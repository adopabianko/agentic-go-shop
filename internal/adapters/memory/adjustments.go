package memory

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"example.com/agentic-go-shop/internal/contracts"
	"example.com/agentic-go-shop/internal/domain"
	"example.com/agentic-go-shop/internal/ports"
)

var _ contracts.AdjustmentRepository = (*Store)(nil)

// ApplyAdjustment adds validated stock and records its receipt in the same
// copy-on-write transaction used by orders. Adjustment keys have their own ledger.
func (s *Store) ApplyAdjustment(ctx context.Context, key string, input contracts.AdjustStock) (contracts.Adjustment, bool, error) {
	// Guard arithmetic even when the repository is called without the use case.
	if input.Quantity < 1 || input.Quantity > domain.MaxQuantity {
		return contracts.Adjustment{}, false, domain.ErrInvalid
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return contracts.Adjustment{}, false, err
	}
	hash := sha256.Sum256(payload)
	fingerprint := hex.EncodeToString(hash[:])
	var receipt contracts.Adjustment
	var replayed bool
	err = s.WithinTx(ctx, func(tx ports.Tx) error {
		// WithinTx always supplies a private *state clone under Store.mu.
		next := tx.(*state)
		if record, ok := next.adjustments[key]; ok {
			if record.Fingerprint != fingerprint {
				return domain.ErrIdempotencyConflict
			}
			receipt, replayed = record.Response, true
			return nil
		}
		stock, err := next.Stock(input.SKU)
		if err != nil {
			return err
		}
		if input.Quantity > domain.MaxQuantity-stock.Available-stock.Reserved {
			return domain.ErrInvalid
		}
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return err
		}
		receipt = contracts.Adjustment{
			ID: "adj_" + hex.EncodeToString(id[:]), SKU: input.SKU,
			Quantity: input.Quantity, Reason: input.Reason, CreatedAt: time.Now().UTC(),
		}
		stock.Available += input.Quantity
		next.PutStock(stock)
		next.adjustments[key] = contracts.AdjustmentRecord{Fingerprint: fingerprint, Response: receipt}
		return nil
	})
	if err != nil {
		return contracts.Adjustment{}, false, err
	}
	return receipt, replayed, nil
}
