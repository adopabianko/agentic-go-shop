package inventory

import (
	"context"
	"fmt"

	"example.com/agentic-go-shop/internal/domain"
	"example.com/agentic-go-shop/internal/ports"
)

type Service struct{}

// Reserve participates in the caller's transaction; it does not start another one.
func (Service) Reserve(tx ports.Tx, items []domain.Item) error {
	items, err := domain.NormalizeItems(items)
	if err != nil {
		return err
	}
	stocks := make([]domain.Stock, len(items))
	for i, item := range items {
		stock, err := tx.Stock(item.SKU)
		if err != nil {
			return err
		}
		if stock.Available < item.Quantity {
			return fmt.Errorf("%w: %s", domain.ErrInsufficientStock, item.SKU)
		}
		stocks[i] = stock
	}
	for i, item := range items {
		stock := stocks[i]
		stock.Available -= item.Quantity
		stock.Reserved += item.Quantity
		tx.PutStock(stock)
	}
	return nil
}

// Release is called once per order state transition. Order cancellation owns
// idempotency; a raw Release call is deliberately not an independently exposed API.
func (Service) Release(tx ports.Tx, items []domain.Item) error {
	items, err := domain.NormalizeItems(items)
	if err != nil {
		return err
	}
	stocks := make([]domain.Stock, len(items))
	for i, item := range items {
		stock, err := tx.Stock(item.SKU)
		if err != nil {
			return err
		}
		if stock.Reserved < item.Quantity {
			return domain.ErrInvariant
		}
		stocks[i] = stock
	}
	for i, item := range items {
		stock := stocks[i]
		stock.Reserved -= item.Quantity
		stock.Available += item.Quantity
		tx.PutStock(stock)
	}
	return nil
}

func Get(ctx context.Context, store ports.Store, sku string) (domain.Stock, error) {
	if !domain.ValidID(sku) {
		return domain.Stock{}, domain.ErrInvalid
	}
	var stock domain.Stock
	err := store.WithinTx(ctx, func(tx ports.Tx) error {
		var err error
		stock, err = tx.Stock(sku)
		return err
	})
	return stock, err
}
