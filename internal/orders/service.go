package orders

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"example.com/agentic-go-shop/internal/domain"
	"example.com/agentic-go-shop/internal/ports"
)

// Inventory is consumer-owned. Implementations mutate only the provided Tx.
// Tests can replace it before the inventory implementation is available.
type Inventory interface {
	Reserve(ports.Tx, []domain.Item) error
	Release(ports.Tx, []domain.Item) error
}

type Service struct {
	store     ports.Store
	inventory Inventory
	now       func() time.Time
	newID     func() (string, error)
}

func New(store ports.Store, inventory Inventory) *Service {
	return &Service{store: store, inventory: inventory, now: time.Now, newID: randomID}
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "ord_" + hex.EncodeToString(b[:]), nil
}

func (s *Service) Create(ctx context.Context, key string, input domain.CreateOrder) (domain.Order, bool, error) {
	if !domain.ValidRequestKey(key) || !domain.ValidID(input.CustomerID) {
		return domain.Order{}, false, domain.ErrInvalid
	}
	items, err := domain.NormalizeItems(input.Items)
	if err != nil {
		return domain.Order{}, false, err
	}
	input.Items = items
	canonical, err := json.Marshal(input)
	if err != nil {
		return domain.Order{}, false, err
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(canonical))
	var result domain.Order
	replayed := false
	err = s.store.WithinTx(ctx, func(tx ports.Tx) error {
		if record, ok := tx.Request(key); ok {
			if record.Fingerprint != fingerprint {
				return domain.ErrIdempotencyConflict
			}
			result, replayed = record.Response, true
			return nil
		}
		id, err := s.newID()
		if err != nil {
			return err
		}
		if _, err := tx.Order(id); !errors.Is(err, domain.ErrNotFound) {
			if err != nil {
				return err
			}
			return domain.ErrInvariant
		}
		if err := s.inventory.Reserve(tx, items); err != nil {
			return err
		}
		now := s.now().UTC()
		result = domain.Order{ID: id, CustomerID: input.CustomerID, Items: items, Status: domain.Reserved, CreatedAt: now, UpdatedAt: now}
		tx.PutOrder(result)
		tx.PutRequest(key, ports.RequestRecord{Fingerprint: fingerprint, Response: result})
		return nil
	})
	if err != nil {
		return domain.Order{}, false, err
	}
	return result, replayed, nil
}

func (s *Service) Get(ctx context.Context, id string) (domain.Order, error) {
	if !domain.ValidID(id) {
		return domain.Order{}, domain.ErrInvalid
	}
	var order domain.Order
	err := s.store.WithinTx(ctx, func(tx ports.Tx) error {
		var err error
		order, err = tx.Order(id)
		return err
	})
	return order, err
}

func (s *Service) Cancel(ctx context.Context, id string) (domain.Order, error) {
	if !domain.ValidID(id) {
		return domain.Order{}, domain.ErrInvalid
	}
	var order domain.Order
	err := s.store.WithinTx(ctx, func(tx ports.Tx) error {
		var err error
		order, err = tx.Order(id)
		if err != nil {
			return err
		}
		if order.Status == domain.Cancelled {
			return nil
		}
		if order.Status != domain.Reserved {
			return domain.ErrInvariant
		}
		if err := s.inventory.Release(tx, order.Items); err != nil {
			return err
		}
		order.Status, order.UpdatedAt = domain.Cancelled, s.now().UTC()
		tx.PutOrder(order)
		return nil
	})
	if err != nil {
		return domain.Order{}, err
	}
	return order, nil
}
