// Package domain contains business values and errors, without transport or storage dependencies.
package domain

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"
)

var (
	ErrInvalid             = errors.New("invalid input")
	ErrNotFound            = errors.New("not found")
	ErrInsufficientStock   = errors.New("insufficient stock")
	ErrIdempotencyConflict = errors.New("idempotency key already used for another request")
	ErrInvariant           = errors.New("storage invariant violated")
)

const (
	Reserved    = "reserved"
	Cancelled   = "cancelled"
	MaxQuantity = 1_000_000
	MaxItems    = 100
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
var requestKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{7,127}$`)

func ValidID(s string) bool         { return identifier.MatchString(s) }
func ValidRequestKey(s string) bool { return requestKey.MatchString(s) }

type Item struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

type Stock struct {
	SKU       string `json:"sku"`
	Available int    `json:"available"`
	Reserved  int    `json:"reserved"`
}

type Order struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customer_id"`
	Items      []Item    `json:"items"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Clone prevents callers from mutating committed state through a slice alias.
func (o Order) Clone() Order {
	o.Items = append([]Item(nil), o.Items...)
	return o
}

type CreateOrder struct {
	CustomerID string `json:"customer_id"`
	Items      []Item `json:"items"`
}

// NormalizeItems rejects duplicates and sorts a copy for stable request fingerprints.
func NormalizeItems(items []Item) ([]Item, error) {
	if len(items) == 0 || len(items) > MaxItems {
		return nil, fmt.Errorf("%w: items must contain 1..100 entries", ErrInvalid)
	}
	out := append([]Item(nil), items...)
	seen := make(map[string]bool, len(items))
	for _, item := range out {
		if !ValidID(item.SKU) || item.Quantity < 1 || item.Quantity > MaxQuantity || seen[item.SKU] {
			return nil, fmt.Errorf("%w: invalid or duplicate item", ErrInvalid)
		}
		seen[item.SKU] = true
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SKU < out[j].SKU })
	return out, nil
}
