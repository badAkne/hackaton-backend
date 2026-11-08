package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/badAkne/hackaton-backend/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// newTestAPI creates an API instance connected to the test database.
func newTestAPI(t *testing.T) http.Handler {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL_API")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse database config: %v", err)
	}

	if config.ConnConfig.Database != "inventory_api_test" {
		t.Fatalf("expected inventory_api_test, got %s", config.ConnConfig.Database)
	}

	ctx := context.Background()

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}

	t.Cleanup(pool.Close)

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping database: %v", err)
	}

	_, err = pool.Exec(ctx, `
		TRUNCATE order_items, orders, products RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatalf("truncate test database: %v", err)
	}

	logger := log.New(io.Discard, "", 0)
	api := New(store.New(pool), logger)

	return api.Routes()
}

func performRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	return rec
}

func TestCreateProductHandlerSuccess(t *testing.T) {
	handler := newTestAPI(t)

	rec := performRequest(
		handler,
		http.MethodPost,
		"/products",
		`{"sku":"SKU-001","name":"Test product","price_cents":1500,"stock":10}`,
	)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s",
			rec.Code, http.StatusCreated, rec.Body.String())
	}

	var product struct {
		ID    int64  `json:"id"`
		SKU   string `json:"sku"`
		Name  string `json:"name"`
		Price int64  `json:"price_cents"`
		Stock int    `json:"stock"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&product); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if product.ID == 0 {
		t.Error("expected product ID to be set")
	}
	if product.SKU != "SKU-001" {
		t.Errorf("SKU = %q, want SKU-001", product.SKU)
	}
	if product.Price != 1500 {
		t.Errorf("Price = %d, want 1500", product.Price)
	}
	if product.Stock != 10 {
		t.Errorf("Stock = %d, want 10", product.Stock)
	}
}

func TestCreateProductHandlerInvalidPrice(t *testing.T) {
	handler := newTestAPI(t)

	rec := performRequest(
		handler,
		http.MethodPost,
		"/products",
		`{"sku":"SKU-001","name":"Test product","price_cents":-100,"stock":10}`,
	)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d",
			rec.Code, http.StatusBadRequest)
	}
}

func TestCreateProductHandlerDuplicateSKU(t *testing.T) {
	handler := newTestAPI(t)

	body := `{"sku":"SKU-001","name":"Test product","price_cents":1500,"stock":10}`

	first := performRequest(handler, http.MethodPost, "/products", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("first request status = %d, want %d; body = %s",
			first.Code, http.StatusCreated, first.Body.String())
	}

	second := performRequest(handler, http.MethodPost, "/products", body)

	if second.Code != http.StatusConflict {
		t.Errorf("second request status = %d, want %d; body = %s",
			second.Code, http.StatusConflict, second.Body.String())
	}
}

func TestCreateProductHandlerInvalidJSON(t *testing.T) {
	handler := newTestAPI(t)

	rec := performRequest(
		handler,
		http.MethodPost,
		"/products",
		`{"sku": "SKU-001", "name":`,
	)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d",
			rec.Code, http.StatusBadRequest)
	}
}

func TestCreateOrderHandlerSuccess(t *testing.T) {
	handler := newTestAPI(t)

	// Создаём товар.
	product := performRequest(
		handler,
		http.MethodPost,
		"/products",
		`{"sku":"SKU-001","name":"Keyboard","price_cents":1500,"stock":10}`,
	)

	if product.Code != http.StatusCreated {
		t.Fatalf("create product: status = %d, body = %s", product.Code, product.Body.String())
	}

	// Создаём заказ.
	rec := performRequest(
		handler,
		http.MethodPost,
		"/orders",
		`{"customer_email":"test@example.com","items":[{"product_id":1,"quantity":2}]}`,
	)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s",
			rec.Code, http.StatusCreated, rec.Body.String())
	}

	var order struct {
		ID    int64 `json:"id"`
		Total int64 `json:"total_cents"`
	}

	if err := json.NewDecoder(rec.Body).Decode(&order); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if order.ID == 0 {
		t.Error("expected order ID to be set")
	}

	if order.Total != 3000 {
		t.Errorf("total = %d, want 3000", order.Total)
	}
}

func TestCreateOrderHandlerEmptyItems(t *testing.T) {
	handler := newTestAPI(t)

	rec := performRequest(
		handler,
		http.MethodPost,
		"/orders",
		`{"customer_email":"test@example.com","items":[]}`,
	)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d",
			rec.Code, http.StatusBadRequest)
	}
}

func TestCreateOrderHandlerInvalidQuantity(t *testing.T) {
	handler := newTestAPI(t)

	product := performRequest(
		handler,
		http.MethodPost,
		"/products",
		`{"sku":"SKU-001","name":"Keyboard","price_cents":1500,"stock":10}`,
	)

	if product.Code != http.StatusCreated {
		t.Fatalf("create product: status = %d", product.Code)
	}

	tests := []struct {
		name     string
		quantity int
	}{
		{name: "zero quantity", quantity: 0},
		{name: "negative quantity", quantity: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := fmt.Sprintf(
				`{"customer_email":"test@example.com","items":[{"product_id":1,"quantity":%d}]}`,
				tt.quantity,
			)

			rec := performRequest(handler, http.MethodPost, "/orders", body)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d; body = %s",
					rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestCreateOrderHandlerProductNotFound(t *testing.T) {
	handler := newTestAPI(t)

	rec := performRequest(
		handler,
		http.MethodPost,
		"/orders",
		`{"customer_email":"test@example.com","items":[{"product_id":999999,"quantity":1}]}`,
	)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s",
			rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestCreateOrderHandlerInsufficientStock(t *testing.T) {
	handler := newTestAPI(t)

	product := performRequest(
		handler,
		http.MethodPost,
		"/products",
		`{"sku":"SKU-001","name":"Keyboard","price_cents":1500,"stock":2}`,
	)

	if product.Code != http.StatusCreated {
		t.Fatalf("create product: status = %d", product.Code)
	}

	rec := performRequest(
		handler,
		http.MethodPost,
		"/orders",
		`{"customer_email":"test@example.com","items":[{"product_id":1,"quantity":5}]}`,
	)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d; body = %s",
			rec.Code, http.StatusConflict, rec.Body.String())
	}
}
