// Package contracts freezes the ports for the example's NEXT sprint.
// These are real compile-time contracts, not implemented or routed features.
package contracts

import (
	"context"
	"time"

	"example.com/agentic-go-shop/internal/domain"
)

type AdjustStock struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
	Reason   string `json:"reason"`
}

type Adjustment struct {
	ID        string    `json:"id"`
	SKU       string    `json:"sku"`
	Quantity  int       `json:"quantity"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// AdjustmentRecord is the transactionally stored result of a successful request.
type AdjustmentRecord struct {
	Fingerprint string
	Response    Adjustment
}

// AdjustmentRepository atomically applies a validated positive adjustment and
// records its response. It shares inventory serialization with order reservations.
// The key namespace is separate from order creation. See docs/contracts/next.md.
type AdjustmentRepository interface {
	ApplyAdjustment(context.Context, string, AdjustStock) (Adjustment, bool, error)
}

type AdjustmentUseCase interface {
	Adjust(context.Context, string, AdjustStock) (Adjustment, bool, error)
}

type OrderQuery struct {
	CustomerID string
	Limit      int
	Cursor     string
}

type OrderPage struct {
	Items      []domain.Order `json:"items"`
	NextCursor string         `json:"next_cursor"`
}

// OrderReader uses keyset pagination by (created_at, id), ascending, and returns
// isolated order values. Cursors bind the customer filter; see next.md.
type OrderReader interface {
	ReadOrders(context.Context, OrderQuery) (OrderPage, error)
}

type OrderListUseCase interface {
	List(context.Context, OrderQuery) (OrderPage, error)
}
