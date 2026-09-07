package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Participant represents a downstream service taking part in an order transaction.
// Its log is deliberately persisted before it answers YES or applies a decision.
type Participant struct {
	name         string
	canPrepare   bool
	prepareDelay time.Duration
	state        string
	logPath      string
}

func (p *Participant) record(state string) error {
	p.state = state
	return os.WriteFile(p.logPath, []byte(state+"\n"), 0o600)
}

func (p *Participant) Prepare(ctx context.Context, txID string) (bool, error) {
	if p.prepareDelay > 0 {
		select {
		case <-time.After(p.prepareDelay):
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
	if !p.canPrepare {
		fmt.Printf("%s: NO  (%s cannot be reserved)\n", p.name, txID)
		return false, nil
	}
	if err := p.record("PREPARED " + txID); err != nil {
		return false, err
	}
	fmt.Printf("%s: YES (durably prepared)\n", p.name)
	return true, nil
}

func (p *Participant) Decide(decision, txID string) error {
	if err := p.record(decision + " " + txID); err != nil {
		return err
	}
	fmt.Printf("%s: %s\n", p.name, decision)
	return nil
}

// OrderService coordinates 2PC: every YES means COMMIT; one NO means ABORT.
// It persists the global decision before telling downstream services.
type OrderService struct {
	participants   []*Participant
	logPath        string
	prepareTimeout time.Duration
}

func (s *OrderService) PlaceOrder(orderID string) error {
	fmt.Printf("\norder %s: phase 1 (reserve resources)\n", orderID)
	decision := "COMMIT"
	type vote struct {
		participant *Participant
		yes         bool
		err         error
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.prepareTimeout)
	defer cancel()
	votes := make(chan vote, len(s.participants))
	for _, participant := range s.participants {
		go func(participant *Participant) {
			yes, err := participant.Prepare(ctx, orderID)
			votes <- vote{participant: participant, yes: yes, err: err}
		}(participant)
	}

	for range s.participants {
		vote := <-votes
		if errors.Is(vote.err, context.DeadlineExceeded) {
			fmt.Printf("%s: timeout after %s\n", vote.participant.name, s.prepareTimeout)
			decision = "ABORT"
			continue
		}
		if vote.err != nil {
			return vote.err
		}
		if !vote.yes {
			decision = "ABORT"
		}
	}

	if err := os.WriteFile(s.logPath, []byte(decision+" "+orderID+"\n"), 0o600); err != nil {
		return err
	}
	fmt.Printf("phase 2 (decision): OrderService durably recorded %s\n", decision)
	for _, participant := range s.participants {
		if err := participant.Decide(decision, orderID); err != nil {
			return err
		}
	}
	return nil
}

func participant(dir, name string, canPrepare bool, delay time.Duration) *Participant {
	return &Participant{name: name, canPrepare: canPrepare, prepareDelay: delay, state: "NEW", logPath: filepath.Join(dir, name+".log")}
}

func main() {
	dir, err := os.MkdirTemp("", "twopc-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)

	// StoreService reserves stock and DeliveryService reserves a delivery slot.
	// Both say YES, so OrderService commits the order everywhere.
	commit := OrderService{
		participants:   []*Participant{participant(dir, "StoreService", true, 0), participant(dir, "DeliveryService", true, 0)},
		logPath:        filepath.Join(dir, "order-service-commit.log"),
		prepareTimeout: 100 * time.Millisecond,
	}
	if err := commit.PlaceOrder("order-100"); err != nil {
		panic(err)
	}

	// DeliveryService has no delivery capacity, so OrderService aborts the order.
	abort := OrderService{
		participants:   []*Participant{participant(dir, "StoreService", true, 0), participant(dir, "DeliveryService", false, 0)},
		logPath:        filepath.Join(dir, "order-service-abort.log"),
		prepareTimeout: 100 * time.Millisecond,
	}
	if err := abort.PlaceOrder("order-101"); err != nil {
		panic(err)
	}

	// A missing response from DeliveryService also makes OrderService abort.
	timeout := OrderService{
		participants:   []*Participant{participant(dir, "StoreService", true, 0), participant(dir, "DeliveryService", true, 200*time.Millisecond)},
		logPath:        filepath.Join(dir, "order-service-timeout.log"),
		prepareTimeout: 100 * time.Millisecond,
	}
	if err := timeout.PlaceOrder("order-102"); err != nil {
		panic(err)
	}
}
