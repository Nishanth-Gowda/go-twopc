package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"example.com/twopc/internal/db"
)

const prepareTimeout = 2 * time.Second

type createOrderRequest struct {
	SKU             string `json:"sku"`
	Quantity        int    `json:"quantity"`
	DeliveryAddress string `json:"delivery_address"`
}

type orderResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type orderSummary struct {
	ID              string `json:"id"`
	SKU             string `json:"sku"`
	Quantity        int    `json:"quantity"`
	DeliveryAddress string `json:"delivery_address"`
	Status          string `json:"status"`
}

type reservationRequest struct {
	OrderID         string `json:"order_id"`
	SKU             string `json:"sku,omitempty"`
	Quantity        int    `json:"quantity,omitempty"`
	DeliveryAddress string `json:"delivery_address,omitempty"`
}

type orderService struct {
	db          *sql.DB
	storeURL    string
	deliveryURL string
	client      *http.Client
}

func main() {
	database, err := db.Open()
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	service := orderService{
		db:          database,
		storeURL:    env("STORE_SERVICE_URL", "http://127.0.0.1:8081"),
		deliveryURL: env("DELIVERY_SERVICE_URL", "http://127.0.0.1:8082"),
		client:      &http.Client{},
	}
	http.HandleFunc("POST /orders", service.createOrder)
	http.HandleFunc("GET /orders", service.listOrders)
	http.HandleFunc("GET /orders/{id}", service.getOrder)
	log.Println("OrderService listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func (s orderService) listOrders(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), "SELECT id, sku, quantity, delivery_address, status FROM orders ORDER BY created_at DESC LIMIT 12")
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	orders := []orderSummary{}
	for rows.Next() {
		var order orderSummary
		if err := rows.Scan(&order.ID, &order.SKU, &order.Quantity, &order.DeliveryAddress, &order.Status); err != nil {
			serverError(w, err)
			return
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

func (s orderService) createOrder(w http.ResponseWriter, r *http.Request) {
	var request createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.SKU == "" || request.Quantity < 1 || request.DeliveryAddress == "" {
		http.Error(w, "sku, positive quantity, and delivery_address are required", http.StatusBadRequest)
		return
	}
	orderID, err := newID()
	if err != nil {
		serverError(w, err)
		return
	}
	if _, err := s.db.ExecContext(r.Context(), "INSERT INTO orders (id, sku, quantity, delivery_address, status) VALUES (?, ?, ?, ?, 'PREPARING')", orderID, request.SKU, request.Quantity, request.DeliveryAddress); err != nil {
		serverError(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), prepareTimeout)
	defer cancel()
	storeRequest := reservationRequest{OrderID: orderID, SKU: request.SKU, Quantity: request.Quantity}
	deliveryRequest := reservationRequest{OrderID: orderID, DeliveryAddress: request.DeliveryAddress}

	var storePrepared, deliveryPrepared bool
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		storePrepared = s.prepare(ctx, s.storeURL, storeRequest, "")
	}()
	go func() {
		defer group.Done()
		deliveryPrepared = s.prepare(ctx, s.deliveryURL, deliveryRequest, r.Header.Get("X-Delivery-Prepare-Delay-Ms"))
	}()
	group.Wait()

	status := "ABORTED"
	decision := "abort"
	if storePrepared && deliveryPrepared {
		status = "COMMITTED"
		decision = "commit"
	}
	if _, err := s.db.ExecContext(r.Context(), "UPDATE orders SET status = ? WHERE id = ?", status, orderID); err != nil {
		serverError(w, err)
		return
	}
	// The decision is durable before either participant receives it.
	s.decide(s.storeURL, decision, storeRequest)
	s.decide(s.deliveryURL, decision, deliveryRequest)
	writeJSON(w, http.StatusCreated, orderResponse{ID: orderID, Status: status})
}

func (s orderService) getOrder(w http.ResponseWriter, r *http.Request) {
	var response orderResponse
	err := s.db.QueryRowContext(r.Context(), "SELECT id, status FROM orders WHERE id = ?", r.PathValue("id")).Scan(&response.ID, &response.Status)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s orderService) prepare(ctx context.Context, baseURL string, request reservationRequest, delay string) bool {
	response, err := s.call(ctx, baseURL+"/prepare", request, delay)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func (s orderService) decide(baseURL, decision string, request reservationRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), prepareTimeout)
	defer cancel()
	response, err := s.call(ctx, baseURL+"/"+decision, request, "")
	if err == nil {
		response.Body.Close()
	}
}

func (s orderService) call(ctx context.Context, url string, request reservationRequest, delay string) (*http.Response, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if delay != "" {
		httpRequest.Header.Set("X-Prepare-Delay-Ms", delay)
	}
	return s.client.Do(httpRequest)
}

func newID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes[0:4]) + "-" + hex.EncodeToString(bytes[4:6]) + "-" + hex.EncodeToString(bytes[6:8]) + "-" + hex.EncodeToString(bytes[8:10]) + "-" + hex.EncodeToString(bytes[10:]), nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func serverError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}
