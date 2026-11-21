package inventory_test

import (
	"context"
	"errors"
	"example.com/agentic-go-shop/internal/adapters/memory"
	"example.com/agentic-go-shop/internal/domain"
	"example.com/agentic-go-shop/internal/inventory"
	"example.com/agentic-go-shop/internal/ports"
	"testing"
)

func TestReleaseCannotInventStock(t *testing.T) {
	s, err := memory.New([]domain.Stock{{SKU: "A", Available: 2}})
	if err != nil {
		t.Fatal(err)
	}
	err = s.WithinTx(context.Background(), func(tx ports.Tx) error {
		return (inventory.Service{}).Release(tx, []domain.Item{{SKU: "A", Quantity: 1}})
	})
	if !errors.Is(err, domain.ErrInvariant) {
		t.Fatal(err)
	}
	got, err := inventory.Get(context.Background(), s, "A")
	if err != nil || got.Available != 2 || got.Reserved != 0 {
		t.Fatalf("%+v %v", got, err)
	}
}
