package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"example.com/twopc/internal/db"
)

type reservationRequest struct {
	OrderID         string `json:"order_id"`
	DeliveryAddress string `json:"delivery_address"`
}

type deliverySlot struct {
	SlotID  int    `json:"slot_id"`
	OrderID string `json:"order_id,omitempty"`
	Status  string `json:"status"`
}

func main() {
	database, err := db.Open()
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	service := deliveryService{db: database}
	http.HandleFunc("POST /prepare", service.prepare)
	http.HandleFunc("POST /commit", service.commit)
	http.HandleFunc("POST /abort", service.abort)
	http.HandleFunc("GET /state", service.state)
	log.Println("DeliveryService listening on :8082")
	log.Fatal(http.ListenAndServe(":8082", nil))
}

func (s deliveryService) state(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), "SELECT slot_id, COALESCE(order_id, ''), status FROM delivery_reservations ORDER BY slot_id")
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	slots := []deliverySlot{}
	for rows.Next() {
		var slot deliverySlot
		if err := rows.Scan(&slot.SlotID, &slot.OrderID, &slot.Status); err != nil {
			serverError(w, err)
			return
		}
		slots = append(slots, slot)
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, slots)
}

type deliveryService struct{ db *sql.DB }

func (s deliveryService) prepare(w http.ResponseWriter, r *http.Request) {
	if delay, err := strconv.Atoi(r.Header.Get("X-Prepare-Delay-Ms")); err == nil && delay > 0 {
		select {
		case <-time.After(time.Duration(delay) * time.Millisecond):
		case <-r.Context().Done():
			http.Error(w, r.Context().Err().Error(), http.StatusRequestTimeout)
			return
		}
	}
	request, ok := decodeReservation(w, r)
	if !ok {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback()

	var status string
	err = tx.QueryRowContext(r.Context(), "SELECT status FROM delivery_reservations WHERE order_id = ?", request.OrderID).Scan(&status)
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": status})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		serverError(w, err)
		return
	}

	var slotID int
	err = tx.QueryRowContext(r.Context(), "SELECT slot_id FROM delivery_reservations WHERE status = 'AVAILABLE' LIMIT 1 FOR UPDATE").Scan(&slotID)
	if errors.Is(err, sql.ErrNoRows) {
		conflict(w, "no delivery capacity")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(), "UPDATE delivery_reservations SET order_id = ?, delivery_address = ?, status = 'PREPARED' WHERE slot_id = ?", request.OrderID, request.DeliveryAddress, slotID); err != nil {
		serverError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "PREPARED"})
}

func (s deliveryService) commit(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeReservation(w, r)
	if !ok {
		return
	}
	if _, err := s.db.ExecContext(r.Context(), "UPDATE delivery_reservations SET status = 'COMMITTED' WHERE order_id = ? AND status = 'PREPARED'", request.OrderID); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "COMMITTED"})
}

func (s deliveryService) abort(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeReservation(w, r)
	if !ok {
		return
	}
	if _, err := s.db.ExecContext(r.Context(), "UPDATE delivery_reservations SET order_id = NULL, delivery_address = NULL, status = 'AVAILABLE' WHERE order_id = ? AND status = 'PREPARED'", request.OrderID); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ABORTED"})
}

func decodeReservation(w http.ResponseWriter, r *http.Request) (reservationRequest, bool) {
	var request reservationRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.OrderID == "" {
		http.Error(w, "order_id is required", http.StatusBadRequest)
		return request, false
	}
	return request, true
}

func conflict(w http.ResponseWriter, message string) { http.Error(w, message, http.StatusConflict) }
func serverError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
