// Package ports defines the transaction boundary shared by inventory and orders.
package ports

import (
	"context"

	"example.com/agentic-go-shop/internal/domain"
)

type RequestRecord struct {
	Fingerprint string
	Response    domain.Order
}

// Tx is valid only during WithinTx. Do not retain it or perform network I/O in it.
// Getters and setters must isolate mutable values from the caller.
// Put methods insert or replace values; business validation belongs in services.
type Tx interface {
	Stock(sku string) (domain.Stock, error)
	PutStock(domain.Stock)
	Order(id string) (domain.Order, error)
	PutOrder(domain.Order)
	Request(key string) (RequestRecord, bool)
	PutRequest(key string, record RequestRecord)
}

// Store executes serializable transactions. Errors, panics, and cancellation
// before commit must roll back every change, including idempotency records.
// Callbacks are executed exactly once; adapters must not silently retry them.
type Store interface {
	WithinTx(context.Context, func(Tx) error) error
}
