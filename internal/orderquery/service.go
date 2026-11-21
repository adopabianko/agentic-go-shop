// Package orderquery validates order-list queries and delegates pagination to
// the order reader.
package orderquery

import (
	"context"

	"example.com/agentic-go-shop/internal/contracts"
	"example.com/agentic-go-shop/internal/domain"
)

const (
	defaultLimit   = 50
	maxLimit       = 100
	maxCursorBytes = 2048
)

type Service struct {
	reader contracts.OrderReader
}

func New(reader contracts.OrderReader) *Service {
	return &Service{reader: reader}
}

func (s *Service) List(ctx context.Context, query contracts.OrderQuery) (contracts.OrderPage, error) {
	if query.CustomerID != "" && !domain.ValidID(query.CustomerID) {
		return contracts.OrderPage{}, domain.ErrInvalid
	}
	if query.Limit < 0 || query.Limit > maxLimit || len(query.Cursor) > maxCursorBytes {
		return contracts.OrderPage{}, domain.ErrInvalid
	}
	if query.Limit == 0 {
		query.Limit = defaultLimit
	}
	return s.reader.ReadOrders(ctx, query)
}

var _ contracts.OrderListUseCase = (*Service)(nil)
