package domain

import "time"

type Product struct {
	ID        int64     `json:"id"`
	SKU       string    `json:"sku"`
	Name      string    `json:"name"`
	Price     int64     `json:"price_cents"`
	Stock     int       `json:"stock"`
	CreatedAt time.Time `json:"created_at"`
}

type OrderItemInput struct {
	ProductID int64 `json:"product_id"`
	Quantity  int   `json:"quantity"`
}

type CreateOrderInput struct {
	CustomerEmail string          `json:"customer_email"`
	Items         []OrderItemInput `json:"items"`
}

type Order struct {
	ID            int64     `json:"id"`
	CustomerEmail string    `json:"customer_email"`
	Status        string    `json:"status"`
	Total         int64     `json:"total_cents"`
	CreatedAt     time.Time `json:"created_at"`
	Items         []OrderItem `json:"items,omitempty"`
}

type OrderItem struct {
	ProductID   int64  `json:"product_id"`
	ProductName string `json:"product_name"`
	Quantity    int    `json:"quantity"`
	UnitPrice   int64  `json:"unit_price_cents"`
}
