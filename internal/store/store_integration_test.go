package store

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/badAkne/hackaton-backend/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newIntegrationStore(t *testing.T) (*Store, *pgxpool.Pool) {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}

	if strings.Trim(parsedURL.Path, "/") != "inventory_test" {
		t.Fatalf("refusing to run tests against database %q; expected inventory_test", parsedURL.Path)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping test database: %v", err)
	}

	_, err = pool.Exec(ctx, `
		TRUNCATE order_items, orders, products RESTART IDENTITY CASCADE
	`)
	if err != nil {
		pool.Close()
		t.Fatalf("clean test database: %v", err)
	}

	t.Cleanup(pool.Close)

	return New(pool), pool
}

func TestCreateOrderSuccess(t *testing.T) {
	store, pool := newIntegrationStore(t)
	ctx := context.Background()

	product, err := store.CreateProduct(ctx, "TEST-001", "Test product", 1500, 10)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	order, err := store.CreateOrder(ctx, domain.CreateOrderInput{
		CustomerEmail: "test@example.com",
		Items: []domain.OrderItemInput{
			{
				ProductID: product.ID,
				Quantity:  2,
			},
		},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	if order.ID <= 0 {
		t.Fatalf("expected order ID to be positive, got %d", order.ID)
	}

	if order.Total != 3000 {
		t.Fatalf("expected total 3000, got %d", order.Total)
	}

	var stock int
	err = pool.QueryRow(ctx,
		`SELECT stock FROM products WHERE id = $1`,
		product.ID,
	).Scan(&stock)
	if err != nil {
		t.Fatalf("get product stock: %v", err)
	}

	if stock != 8 {
		t.Fatalf("expected stock 8, got %d", stock)
	}
}

func TestCreateOrderRollback(t *testing.T) {
	store, pool := newIntegrationStore(t)
	ctx := context.Background()

	product, err := store.CreateProduct(ctx, "TEST-002", "Test product", 1500, 10)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	_, err = store.CreateOrder(ctx, domain.CreateOrderInput{
		CustomerEmail: "rollback@example.com",
		Items: []domain.OrderItemInput{
			{
				ProductID: product.ID,
				Quantity:  2,
			},
			{
				ProductID: 999999,
				Quantity:  1,
			},
		},
	})

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	var stock int
	err = pool.QueryRow(ctx,
		`SELECT stock FROM products WHERE id = $1`,
		product.ID,
	).Scan(&stock)
	if err != nil {
		t.Fatalf("get product stock: %v", err)
	}

	if stock != 10 {
		t.Fatalf("expected stock to be rolled back to 10, got %d", stock)
	}

	var orderCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM orders`,
	).Scan(&orderCount)
	if err != nil {
		t.Fatalf("count orders: %v", err)
	}

	if orderCount != 0 {
		t.Fatalf("expected 0 orders after rollback, got %d", orderCount)
	}
}

func TestCreateOrderConcurrentStock(t *testing.T) {
	store, pool := newIntegrationStore(t)
	ctx := context.Background()

	product, err := store.CreateProduct(ctx, "TEST-003", "Last item", 2000, 1)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	const workers = 2

	start := make(chan struct{})
	results := make(chan error, workers)

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			<-start

			_, err := store.CreateOrder(ctx, domain.CreateOrderInput{
				CustomerEmail: "concurrent@example.com",
				Items: []domain.OrderItemInput{
					{
						ProductID: product.ID,
						Quantity:  1,
					},
				},
			})

			results <- err
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	successCount := 0
	insufficientStockCount := 0

	for err := range results {
		switch {
		case err == nil:
			successCount++
		case errors.Is(err, ErrInsufficientStock):
			insufficientStockCount++
		default:
			t.Fatalf("unexpected error from concurrent order: %v", err)
		}
	}

	if successCount != 1 {
		t.Fatalf("expected 1 successful order, got %d", successCount)
	}

	if insufficientStockCount != 1 {
		t.Fatalf("expected 1 insufficient-stock error, got %d", insufficientStockCount)
	}

	var stock int
	err = pool.QueryRow(ctx,
		`SELECT stock FROM products WHERE id = $1`,
		product.ID,
	).Scan(&stock)
	if err != nil {
		t.Fatalf("get product stock: %v", err)
	}

	if stock != 0 {
		t.Fatalf("expected stock 0, got %d", stock)
	}

	var orderCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM orders`,
	).Scan(&orderCount)
	if err != nil {
		t.Fatalf("count orders: %v", err)
	}

	if orderCount != 1 {
		t.Fatalf("expected 1 order, got %d", orderCount)
	}
}

func TestCancelOrderSuccess(t *testing.T) {
	store, pool := newIntegrationStore(t)
	ctx := context.Background()

	product, err := store.CreateProduct(ctx, "CANCEL-001", "Test product", 1500, 10)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	order, err := store.CreateOrder(ctx, domain.CreateOrderInput{
		CustomerEmail: "cancel@example.com",
		Items: []domain.OrderItemInput{
			{ProductID: product.ID, Quantity: 3},
		},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	if err := store.CancelOrder(ctx, order.ID); err != nil {
		t.Fatalf("cancel order: %v", err)
	}

	var stock int
	err = pool.QueryRow(ctx,
		`SELECT stock FROM products WHERE id = $1`,
		product.ID,
	).Scan(&stock)
	if err != nil {
		t.Fatalf("get stock: %v", err)
	}

	if stock != 10 {
		t.Fatalf("expected stock 10 after cancellation, got %d", stock)
	}

	var status string
	err = pool.QueryRow(ctx,
		`SELECT status FROM orders WHERE id = $1`,
		order.ID,
	).Scan(&status)
	if err != nil {
		t.Fatalf("get order status: %v", err)
	}

	if status != "cancelled" {
		t.Fatalf("expected status cancelled, got %q", status)
	}
}

func TestCancelOrderRepeated(t *testing.T) {
	store, pool := newIntegrationStore(t)
	ctx := context.Background()

	product, err := store.CreateProduct(ctx, "CANCEL-002", "Test product", 1500, 10)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	order, err := store.CreateOrder(ctx, domain.CreateOrderInput{
		CustomerEmail: "repeated@example.com",
		Items: []domain.OrderItemInput{
			{ProductID: product.ID, Quantity: 3},
		},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := store.CancelOrder(ctx, order.ID); err != nil {
			t.Fatalf("cancel order attempt %d: %v", i+1, err)
		}
	}

	var stock int
	err = pool.QueryRow(ctx,
		`SELECT stock FROM products WHERE id = $1`,
		product.ID,
	).Scan(&stock)
	if err != nil {
		t.Fatalf("get stock: %v", err)
	}

	if stock != 10 {
		t.Fatalf("expected stock 10 after repeated cancellation, got %d", stock)
	}
}

func TestCancelOrderConcurrent(t *testing.T) {
	store, pool := newIntegrationStore(t)
	ctx := context.Background()

	product, err := store.CreateProduct(ctx, "CANCEL-003", "Test product", 1500, 10)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	order, err := store.CreateOrder(ctx, domain.CreateOrderInput{
		CustomerEmail: "concurrent-cancel@example.com",
		Items: []domain.OrderItemInput{
			{ProductID: product.ID, Quantity: 3},
		},
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	const workers = 2

	start := make(chan struct{})
	results := make(chan error, workers)

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()
			<-start
			results <- store.CancelOrder(ctx, order.ID)
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	for err := range results {
		if err != nil {
			t.Fatalf("concurrent cancellation failed: %v", err)
		}
	}

	var stock int
	err = pool.QueryRow(ctx,
		`SELECT stock FROM products WHERE id = $1`,
		product.ID,
	).Scan(&stock)
	if err != nil {
		t.Fatalf("get stock: %v", err)
	}

	if stock != 10 {
		t.Fatalf("expected stock 10 after concurrent cancellation, got %d", stock)
	}

	var status string
	err = pool.QueryRow(ctx,
		`SELECT status FROM orders WHERE id = $1`,
		order.ID,
	).Scan(&status)
	if err != nil {
		t.Fatalf("get order status: %v", err)
	}

	if status != "cancelled" {
		t.Fatalf("expected status cancelled, got %q", status)
	}
}
func TestCreateProductSuccess(t *testing.T) {
	store, pool := newIntegrationStore(t)

	product, err := store.CreateProduct(
		context.Background(),
		"SKU-001",
		"Test product",
		1500,
		10,
	)

	if err != nil {
		t.Fatalf("CreateProduct() error = %v", err)
	}

	if product.ID == 0 {
		t.Fatal("expected product ID to be set")
	}

	if product.SKU != "SKU-001" {
		t.Errorf("SKU = %q, want SKU-001", product.SKU)
	}

	if product.Name != "Test product" {
		t.Errorf("Name = %q, want Test product", product.Name)
	}

	if product.Price != 1500 {
		t.Errorf("Price = %d, want 1500", product.Price)
	}

	if product.Stock != 10 {
		t.Errorf("Stock = %d, want 10", product.Stock)
	}

	_ = pool
}

func TestCreateProductInvalidInput(t *testing.T) {
	store, _ := newIntegrationStore(t)

	tests := []struct {
		name  string
		sku   string
		title string
		price int64
		stock int
	}{
		{
			name:  "empty SKU",
			sku:   "",
			title: "Product",
			price: 100,
			stock: 10,
		},
		{
			name:  "empty name",
			sku:   "SKU-001",
			title: "   ",
			price: 100,
			stock: 10,
		},
		{
			name:  "negative price",
			sku:   "SKU-002",
			title: "Product",
			price: -100,
			stock: 10,
		},
		{
			name:  "negative stock",
			sku:   "SKU-003",
			title: "Product",
			price: 100,
			stock: -10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.CreateProduct(
				context.Background(),
				tt.sku,
				tt.title,
				tt.price,
				tt.stock,
			)

			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestCreateProductDuplicateSKU(t *testing.T) {
	store, _ := newIntegrationStore(t)

	ctx := context.Background()

	_, err := store.CreateProduct(ctx, "SKU-001", "Product 1", 100, 10)
	if err != nil {
		t.Fatalf("first CreateProduct() error = %v", err)
	}

	_, err = store.CreateProduct(ctx, "SKU-001", "Product 2", 200, 20)

	if !errors.Is(err, ErrConflict) {
		t.Errorf("error = %v, want ErrConflict", err)
	}
}

func TestCreateProductTrimsWhitespace(t *testing.T) {
	store, _ := newIntegrationStore(t)

	product, err := store.CreateProduct(
		context.Background(),
		" SKU-001 ",
		" Test product ",
		100,
		5,
	)

	if err != nil {
		t.Fatalf("CreateProduct() error = %v", err)
	}

	if product.SKU != "SKU-001" {
		t.Errorf("SKU = %q, want SKU-001", product.SKU)
	}

	if product.Name != "Test product" {
		t.Errorf("Name = %q, want Test product", product.Name)
	}
}

func TestCreateOrderInvalidEmail(t *testing.T) {
	s, _ := newIntegrationStore(t)

	tests := []struct {
		name  string
		email string
	}{
		{
			name:  "missing domain",
			email: "test@",
		},
		{
			name:  "missing local part",
			email: "@example.com",
		},
		{
			name:  "no at symbol",
			email: "test.example.com",
		},
		{
			name:  "invalid email",
			email: "not-an-email",
		},
		{
			name:  "display name",
			email: "Test User <test@example.com>",
		},
		{
			name:  "empty email",
			email: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.CreateOrder(context.Background(), domain.CreateOrderInput{
				CustomerEmail: tt.email,
				Items: []domain.OrderItemInput{
					{
						ProductID: 1,
						Quantity:  1,
					},
				},
			})

			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestDatabaseConstraints(t *testing.T) {
	_, pool := newIntegrationStore(t)

	tests := []struct {
		name  string
		query string
	}{
		{
			name: "negative product price",
			query: `
                INSERT INTO products (sku, name, price_cents, stock)
                VALUES ('SKU-1', 'Product', -100, 10)
            `,
		},
		{
			name: "negative product stock",
			query: `
                INSERT INTO products (sku, name, price_cents, stock)
                VALUES ('SKU-2', 'Product', 100, -10)
            `,
		},
		{
			name: "empty product name",
			query: `
                INSERT INTO products (sku, name, price_cents, stock)
                VALUES ('SKU-3', '   ', 100, 10)
            `,
		},
		{
			name: "invalid order status",
			query: `
                INSERT INTO orders (customer_email, status)
                VALUES ('test@example.com', 'unknown')
            `,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := pool.Exec(context.Background(), tt.query)

			if err == nil {
				t.Fatal("expected database constraint violation")
			}
		})
	}
}
