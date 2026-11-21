// Package adjustments validates positive stock adjustments before handing them
// to the repository that owns atomic stock and idempotency updates.
package adjustments

import (
	"context"

	"example.com/agentic-go-shop/internal/contracts"
	"example.com/agentic-go-shop/internal/domain"
)

type Service struct {
	repository contracts.AdjustmentRepository
}

var _ contracts.AdjustmentUseCase = (*Service)(nil)

func New(repository contracts.AdjustmentRepository) *Service {
	return &Service{repository: repository}
}

// Adjust delegates each valid request exactly once, including replays. The
// repository owns receipt generation, stock limits, and transaction handling.
func (s *Service) Adjust(ctx context.Context, key string, input contracts.AdjustStock) (contracts.Adjustment, bool, error) {
	if !domain.ValidRequestKey(key) || !domain.ValidID(input.SKU) ||
		input.Quantity < 1 || input.Quantity > domain.MaxQuantity ||
		len(input.Reason) < 1 || len(input.Reason) > 200 {
		return contracts.Adjustment{}, false, domain.ErrInvalid
	}
	for i := 0; i < len(input.Reason); i++ {
		if input.Reason[i] < ' ' || input.Reason[i] > '~' {
			return contracts.Adjustment{}, false, domain.ErrInvalid
		}
	}
	return s.repository.ApplyAdjustment(ctx, key, input)
}
