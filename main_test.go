package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlaceOrderCommitsWhenAllServicesPrepare(t *testing.T) {
	dir := t.TempDir()
	store := participant(dir, "StoreService", true, 0)
	delivery := participant(dir, "DeliveryService", true, 0)
	order := OrderService{
		participants:   []*Participant{store, delivery},
		logPath:        filepath.Join(dir, "order-service.log"),
		prepareTimeout: time.Second,
	}

	if err := order.PlaceOrder("order-100"); err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}

	assertLogContains(t, order.logPath, "COMMIT order-100")
	assertLogContains(t, store.logPath, "COMMIT order-100")
	assertLogContains(t, delivery.logPath, "COMMIT order-100")
}

func TestPlaceOrderAbortsWhenDeliveryRejects(t *testing.T) {
	dir := t.TempDir()
	store := participant(dir, "StoreService", true, 0)
	delivery := participant(dir, "DeliveryService", false, 0)
	order := OrderService{
		participants:   []*Participant{store, delivery},
		logPath:        filepath.Join(dir, "order-service.log"),
		prepareTimeout: time.Second,
	}

	if err := order.PlaceOrder("order-101"); err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}

	assertLogContains(t, order.logPath, "ABORT order-101")
	assertLogContains(t, store.logPath, "ABORT order-101")
	assertLogContains(t, delivery.logPath, "ABORT order-101")
}

func TestPlaceOrderUsesOneSharedPrepareTimeout(t *testing.T) {
	dir := t.TempDir()
	store := participant(dir, "StoreService", true, 75*time.Millisecond)
	delivery := participant(dir, "DeliveryService", true, 200*time.Millisecond)
	order := OrderService{
		participants:   []*Participant{store, delivery},
		logPath:        filepath.Join(dir, "order-service.log"),
		prepareTimeout: 100 * time.Millisecond,
	}

	started := time.Now()
	if err := order.PlaceOrder("order-102"); err != nil {
		t.Fatalf("PlaceOrder() error = %v", err)
	}
	elapsed := time.Since(started)

	if elapsed >= 150*time.Millisecond {
		t.Fatalf("PlaceOrder() took %s; expected one shared timeout", elapsed)
	}
	assertLogContains(t, order.logPath, "ABORT order-102")
	assertLogContains(t, store.logPath, "ABORT order-102")
	assertLogContains(t, delivery.logPath, "ABORT order-102")
}

func assertLogContains(t *testing.T, path, want string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if !strings.Contains(string(contents), want) {
		t.Fatalf("log %q = %q; want %q", path, contents, want)
	}
}
