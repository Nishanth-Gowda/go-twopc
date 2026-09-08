package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"example.com/twopc/internal/db"
)

type reservationRequest struct {
	OrderID  string `json:"order_id"`
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

type inventoryState struct {
	SKU               string `json:"sku"`
	AvailableQuantity int    `json:"available_quantity"`
}

func main() {
	database, err := db.Open()
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()

	service := storeService{db: database}
	http.HandleFunc("POST /prepare", service.prepare)
	http.HandleFunc("POST /commit", service.commit)
	http.HandleFunc("POST /abort", service.abort)
	http.HandleFunc("GET /state", service.state)
	log.Println("StoreService listening on :8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}

func (s storeService) state(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), "SELECT sku, available_quantity FROM inventory ORDER BY sku")
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	inventory := []inventoryState{}
	for rows.Next() {
		var item inventoryState
		if err := rows.Scan(&item.SKU, &item.AvailableQuantity); err != nil {
			serverError(w, err)
			return
		}
		inventory = append(inventory, item)
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inventory)
}

type storeService struct{ db *sql.DB }

func (s storeService) prepare(w http.ResponseWriter, r *http.Request) {
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
	err = tx.QueryRowContext(r.Context(), "SELECT status FROM store_reservations WHERE order_id = ?", request.OrderID).Scan(&status)
	if err == nil {
		if status == "ABORTED" {
			conflict(w, "reservation was aborted")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": status})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		serverError(w, err)
		return
	}

	var available int
	err = tx.QueryRowContext(r.Context(), "SELECT available_quantity FROM inventory WHERE sku = ? FOR UPDATE", request.SKU).Scan(&available)
	if errors.Is(err, sql.ErrNoRows) {
		conflict(w, "insufficient stock")
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	if available < request.Quantity {
		conflict(w, "insufficient stock")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "UPDATE inventory SET available_quantity = available_quantity - ? WHERE sku = ?", request.Quantity, request.SKU); err != nil {
		serverError(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO store_reservations (order_id, sku, quantity, status) VALUES (?, ?, ?, 'PREPARED')", request.OrderID, request.SKU, request.Quantity); err != nil {
		serverError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "PREPARED"})
}

func (s storeService) commit(w http.ResponseWriter, r *http.Request) {
	request, ok := decodeReservation(w, r)
	if !ok {
		return
	}
	if _, err := s.db.ExecContext(r.Context(), "UPDATE store_reservations SET status = 'COMMITTED' WHERE order_id = ? AND status = 'PREPARED'", request.OrderID); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "COMMITTED"})
}

func (s storeService) abort(w http.ResponseWriter, r *http.Request) {
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
	var sku string
	var quantity int
	err = tx.QueryRowContext(r.Context(), "SELECT sku, quantity FROM store_reservations WHERE order_id = ? AND status = 'PREPARED' FOR UPDATE", request.OrderID).Scan(&sku, &quantity)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ABORTED"})
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(), "UPDATE inventory SET available_quantity = available_quantity + ? WHERE sku = ?", quantity, sku); err != nil {
		serverError(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(), "UPDATE store_reservations SET status = 'ABORTED' WHERE order_id = ?", request.OrderID); err != nil {
		serverError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
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
