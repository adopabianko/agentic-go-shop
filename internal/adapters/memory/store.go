// Package memory is a single-process reference adapter, not durable storage.
package memory

import (
	"context"
	"sync"

	"example.com/agentic-go-shop/internal/contracts"
	"example.com/agentic-go-shop/internal/domain"
	"example.com/agentic-go-shop/internal/ports"
)

type state struct {
	stocks   map[string]domain.Stock
	orders   map[string]domain.Order
	requests map[string]ports.RequestRecord
	// Reserved foundation for T02. No adjustment endpoint is exposed yet.
	adjustments map[string]contracts.AdjustmentRecord
}

type Store struct {
	mu    sync.Mutex
	state *state
}

var _ ports.Store = (*Store)(nil)

func New(seed []domain.Stock) (*Store, error) {
	s := &state{stocks: map[string]domain.Stock{}, orders: map[string]domain.Order{}, requests: map[string]ports.RequestRecord{}, adjustments: map[string]contracts.AdjustmentRecord{}}
	for _, stock := range seed {
		_, duplicate := s.stocks[stock.SKU]
		if !domain.ValidID(stock.SKU) || stock.Available < 0 || stock.Available > domain.MaxQuantity || stock.Reserved != 0 || duplicate {
			return nil, domain.ErrInvalid
		}
		s.stocks[stock.SKU] = stock
	}
	return &Store{state: s}, nil
}

func (s *Store) WithinTx(ctx context.Context, fn func(ports.Tx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	next := s.state.clone()
	if err := fn(next); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.state = next
	return nil
}

func (s *state) clone() *state {
	out := &state{stocks: map[string]domain.Stock{}, orders: map[string]domain.Order{}, requests: map[string]ports.RequestRecord{}, adjustments: map[string]contracts.AdjustmentRecord{}}
	for k, v := range s.stocks {
		out.stocks[k] = v
	}
	for k, v := range s.orders {
		out.orders[k] = v.Clone()
	}
	for k, v := range s.requests {
		v.Response = v.Response.Clone()
		out.requests[k] = v
	}
	for k, v := range s.adjustments {
		out.adjustments[k] = v
	}
	return out
}

func (s *state) Stock(sku string) (domain.Stock, error) {
	v, ok := s.stocks[sku]
	if !ok {
		return domain.Stock{}, domain.ErrNotFound
	}
	return v, nil
}
func (s *state) PutStock(v domain.Stock) { s.stocks[v.SKU] = v }
func (s *state) Order(id string) (domain.Order, error) {
	v, ok := s.orders[id]
	if !ok {
		return domain.Order{}, domain.ErrNotFound
	}
	return v.Clone(), nil
}
func (s *state) PutOrder(v domain.Order) { s.orders[v.ID] = v.Clone() }
func (s *state) Request(key string) (ports.RequestRecord, bool) {
	v, ok := s.requests[key]
	v.Response = v.Response.Clone()
	return v, ok
}
func (s *state) PutRequest(key string, v ports.RequestRecord) {
	v.Response = v.Response.Clone()
	s.requests[key] = v
}
