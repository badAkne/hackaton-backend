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
