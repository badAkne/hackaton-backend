package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/hackathon/inventory-order-management/internal/domain"
	"github.com/hackathon/inventory-order-management/internal/store"
)

type API struct {
	store  *store.Store
	logger *log.Logger
}

func New(s *store.Store, logger *log.Logger) *API {
	return &API{store: s, logger: logger}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", a.health)
	mux.HandleFunc("GET /products", a.listProducts)
	mux.HandleFunc("POST /products", a.createProduct)
	mux.HandleFunc("POST /orders", a.createOrder)
	mux.HandleFunc("GET /orders/{id}", a.getOrder)
	mux.HandleFunc("POST /orders/{id}/cancel", a.cancelOrder)
	return a.requestLog(mux)
}

func (a *API) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.logger.Printf("method=%s path=%s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) listProducts(w http.ResponseWriter, r *http.Request) {
	products, err := a.store.ListProducts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, products)
}

func (a *API) createProduct(w http.ResponseWriter, r *http.Request) {
	var input struct {
		SKU   string `json:"sku"`
		Name  string `json:"name"`
		Price int64  `json:"price_cents"`
		Stock int    `json:"stock"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if strings.TrimSpace(input.SKU) == "" || strings.TrimSpace(input.Name) == "" || input.Price < 0 || input.Stock < 0 {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	product, err := a.store.CreateProduct(r.Context(), input.SKU, input.Name, input.Price, input.Stock)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusCreated, product)
}

func (a *API) createOrder(w http.ResponseWriter, r *http.Request) {
	var input domain.CreateOrderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json")
		return
	}
	order, err := a.store.CreateOrder(r.Context(), input)
	switch {
	case errors.Is(err, store.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "invalid_input")
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "product_not_found")
	case errors.Is(err, store.ErrInsufficientStock):
		writeError(w, http.StatusConflict, "insufficient_stock")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal_error")
	default:
		writeJSON(w, http.StatusCreated, order)
	}
}

func (a *API) getOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	order, err := a.store.GetOrder(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "order_not_found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (a *API) cancelOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_id")
		return
	}
	if err := a.store.CancelOrder(r.Context(), id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "order_not_found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}
