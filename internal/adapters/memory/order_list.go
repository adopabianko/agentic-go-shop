package memory

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"

	"example.com/agentic-go-shop/internal/contracts"
	"example.com/agentic-go-shop/internal/domain"
)

const (
	orderCursorVersion = 1
	maxOrderCursorLen  = 2048
)

type orderCursor struct {
	Version    int    `json:"version"`
	CreatedAt  string `json:"created_at"`
	ID         string `json:"id"`
	CustomerID string `json:"customer_id"`
}

var _ contracts.OrderReader = (*Store)(nil)

// ReadOrders returns one keyset-paginated snapshot of orders. The mutex is the
// same one used by WithinTx, so map iteration and every cloned result belong to
// a single committed state.
func (s *Store) ReadOrders(ctx context.Context, query contracts.OrderQuery) (contracts.OrderPage, error) {
	if query.Limit < 1 || query.Limit > 100 || (query.CustomerID != "" && !domain.ValidID(query.CustomerID)) {
		return contracts.OrderPage{}, domain.ErrInvalid
	}

	var after *orderCursor
	if query.Cursor != "" {
		cursor, err := decodeOrderCursor(query.Cursor, query.CustomerID)
		if err != nil {
			return contracts.OrderPage{}, err
		}
		after = &cursor
	}

	if err := ctx.Err(); err != nil {
		return contracts.OrderPage{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return contracts.OrderPage{}, err
	}

	orders := make([]domain.Order, 0, len(s.state.orders))
	for _, order := range s.state.orders {
		if query.CustomerID != "" && order.CustomerID != query.CustomerID {
			continue
		}
		if after != nil && !orderAfterCursor(order, *after) {
			continue
		}
		orders = append(orders, order.Clone())
	}
	sort.Slice(orders, func(i, j int) bool {
		if orders[i].CreatedAt.Equal(orders[j].CreatedAt) {
			return orders[i].ID < orders[j].ID
		}
		return orders[i].CreatedAt.Before(orders[j].CreatedAt)
	})

	page := contracts.OrderPage{Items: make([]domain.Order, 0, query.Limit)}
	count := min(len(orders), query.Limit)
	page.Items = append(page.Items, orders[:count]...)
	if len(orders) > query.Limit {
		cursor, err := encodeOrderCursor(page.Items[len(page.Items)-1], query.CustomerID)
		if err != nil {
			return contracts.OrderPage{}, err
		}
		page.NextCursor = cursor
	}
	if err := ctx.Err(); err != nil {
		return contracts.OrderPage{}, err
	}
	return page, nil
}

func orderAfterCursor(order domain.Order, cursor orderCursor) bool {
	createdAt, _ := time.Parse(time.RFC3339Nano, cursor.CreatedAt)
	return order.CreatedAt.After(createdAt) || (order.CreatedAt.Equal(createdAt) && order.ID > cursor.ID)
}

func encodeOrderCursor(order domain.Order, customerID string) (string, error) {
	data, err := json.Marshal(orderCursor{
		Version:    orderCursorVersion,
		CreatedAt:  order.CreatedAt.Format(time.RFC3339Nano),
		ID:         order.ID,
		CustomerID: customerID,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeOrderCursor(encoded, customerID string) (orderCursor, error) {
	invalid := func() (orderCursor, error) {
		return orderCursor{}, fmt.Errorf("%w: malformed order cursor", domain.ErrInvalid)
	}
	if len(encoded) > maxOrderCursorLen {
		return invalid()
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || base64.RawURLEncoding.EncodeToString(data) != encoded {
		return invalid()
	}

	cursor, err := decodeOrderCursorJSON(data)
	if err != nil || cursor.Version != orderCursorVersion || !domain.ValidID(cursor.ID) ||
		(cursor.CustomerID != "" && !domain.ValidID(cursor.CustomerID)) || cursor.CustomerID != customerID {
		return invalid()
	}
	if _, err := time.Parse(time.RFC3339Nano, cursor.CreatedAt); err != nil {
		return invalid()
	}
	return cursor, nil
}

// decodeOrderCursorJSON accepts exactly the four frozen cursor fields. Decoding
// one field at a time also rejects duplicate names and null string values.
func decodeOrderCursorJSON(data []byte) (orderCursor, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return orderCursor{}, domain.ErrInvalid
	}

	var cursor orderCursor
	seen := make(map[string]bool, 4)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return orderCursor{}, domain.ErrInvalid
		}
		name, ok := token.(string)
		if !ok || seen[name] {
			return orderCursor{}, domain.ErrInvalid
		}
		seen[name] = true

		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return orderCursor{}, domain.ErrInvalid
		}
		switch name {
		case "version":
			if err := json.Unmarshal(raw, &cursor.Version); err != nil {
				return orderCursor{}, domain.ErrInvalid
			}
		case "created_at":
			if err := decodeCursorString(raw, &cursor.CreatedAt); err != nil {
				return orderCursor{}, err
			}
		case "id":
			if err := decodeCursorString(raw, &cursor.ID); err != nil {
				return orderCursor{}, err
			}
		case "customer_id":
			if err := decodeCursorString(raw, &cursor.CustomerID); err != nil {
				return orderCursor{}, err
			}
		default:
			return orderCursor{}, domain.ErrInvalid
		}
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') || len(seen) != 4 {
		return orderCursor{}, domain.ErrInvalid
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return orderCursor{}, domain.ErrInvalid
	}
	return cursor, nil
}

func decodeCursorString(raw json.RawMessage, dst *string) error {
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, dst) != nil {
		return domain.ErrInvalid
	}
	return nil
}
