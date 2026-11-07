package store

import (
	"math"
	"testing"
)

func TestAddOrderItemTotal(t *testing.T) {
	tests := []struct {
		name     string
		total    int64
		price    int64
		quantity int
		want     int64
		wantErr  bool
	}{
		{
			name:     "normal calculation",
			total:    100,
			price:    200,
			quantity: 3,
			want:     700,
		},
		{
			name:     "zero price",
			total:    100,
			price:    0,
			quantity: 3,
			want:     100,
		},
		{
			name:     "overflow",
			total:    math.MaxInt64 - 10,
			price:    100,
			quantity: 1,
			wantErr:  true,
		},
		{
			name:     "negative price",
			total:    0,
			price:    -1,
			quantity: 1,
			wantErr:  true,
		},
		{
			name:     "zero quantity",
			total:    0,
			price:    100,
			quantity: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := addOrderItemTotal(tt.total, tt.price, tt.quantity)

			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}

			if !tt.wantErr && got != tt.want {
				t.Errorf("got = %d, want %d", got, tt.want)
			}
		})
	}
}
