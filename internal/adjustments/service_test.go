package adjustments_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"example.com/agentic-go-shop/internal/adjustments"
	"example.com/agentic-go-shop/internal/contracts"
	"example.com/agentic-go-shop/internal/domain"
)

type repositoryFunc func(context.Context, string, contracts.AdjustStock) (contracts.Adjustment, bool, error)

func (f repositoryFunc) ApplyAdjustment(ctx context.Context, key string, input contracts.AdjustStock) (contracts.Adjustment, bool, error) {
	return f(ctx, key, input)
}

func TestAdjustRejectsInvalidInputBeforeRepository(t *testing.T) {
	valid := contracts.AdjustStock{SKU: "SKU-001", Quantity: 1, Reason: "Restock"}
	tests := []struct {
		name  string
		field string
		value string
	}{
		{"missing key", "key", ""},
		{"short key", "key", "1234567"},
		{"long key", "key", strings.Repeat("a", 129)},
		{"key starts with punctuation", "key", "_1234567"},
		{"key contains space", "key", "abcd efgh"},
		{"key contains slash", "key", "abcd/efgh"},
		{"key contains unicode", "key", "abcdéefgh"},
		{"key trailing newline", "key", "abcdefgh\n"},
		{"missing SKU", "sku", ""},
		{"long SKU", "sku", strings.Repeat("a", 65)},
		{"SKU starts with punctuation", "sku", "-SKU"},
		{"SKU contains space", "sku", "SKU 001"},
		{"SKU contains slash", "sku", "SKU/001"},
		{"SKU contains unicode", "sku", "SKU-é"},
		{"SKU trailing newline", "sku", "SKU-001\n"},
		{"missing reason", "reason", ""},
		{"long reason", "reason", strings.Repeat("a", 201)},
		{"unicode reason", "reason", "réstock"},
		{"invalid UTF-8 reason", "reason", "restock\xff"},
	}
	for b := 0; b < 32; b++ {
		tests = append(tests, struct{ name, field, value string }{
			fmt.Sprintf("reason control %02x", b), "reason", "before" + string(rune(b)) + "after",
		})
	}
	tests = append(tests, struct{ name, field, value string }{"reason DEL", "reason", "restock\x7f"})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, input := "adjust-001", valid
			switch tt.field {
			case "key":
				key = tt.value
			case "sku":
				input.SKU = tt.value
			case "reason":
				input.Reason = tt.value
			}
			assertInvalid(t, key, input)
		})
	}
	for _, quantity := range []int{-1, 0, 1_000_001} {
		t.Run(fmt.Sprintf("quantity %d", quantity), func(t *testing.T) {
			input := valid
			input.Quantity = quantity
			assertInvalid(t, "adjust-001", input)
		})
	}
}

func assertInvalid(t *testing.T, key string, input contracts.AdjustStock) {
	t.Helper()
	repository := repositoryFunc(func(context.Context, string, contracts.AdjustStock) (contracts.Adjustment, bool, error) {
		t.Fatal("invalid input reached repository")
		return contracts.Adjustment{}, false, nil
	})
	got, replayed, err := adjustments.New(repository).Adjust(context.Background(), key, input)
	if !errors.Is(err, domain.ErrInvalid) || got != (contracts.Adjustment{}) || replayed {
		t.Fatalf("got (%+v, %v, %v), want zero receipt, false, ErrInvalid", got, replayed, err)
	}
}

func TestAdjustPreservesValidInput(t *testing.T) {
	var printable strings.Builder
	for b := byte(' '); b <= '~'; b++ {
		printable.WriteByte(b)
	}
	tests := []struct {
		name  string
		key   string
		input contracts.AdjustStock
	}{
		{"minimum bounds", "12345678", contracts.AdjustStock{SKU: "0", Quantity: 1, Reason: " "}},
		{"maximum bounds", strings.Repeat("a", 128), contracts.AdjustStock{SKU: strings.Repeat("Z", 64), Quantity: 1_000_000, Reason: strings.Repeat("~", 200)}},
		{"preserve case and spaces", "aB12._-9", contracts.AdjustStock{SKU: "sKu._-9", Quantity: 10, Reason: "  Returned stock  "}},
		{"all printable ASCII", "adjust-001", contracts.AdjustStock{SKU: "SKU-001", Quantity: 2, Reason: printable.String()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			want := contracts.Adjustment{
				ID: "adj_0123456789abcdef0123456789abcdef", SKU: tt.input.SKU,
				Quantity: tt.input.Quantity, Reason: tt.input.Reason,
				CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC),
			}
			repository := repositoryFunc(func(gotCtx context.Context, key string, input contracts.AdjustStock) (contracts.Adjustment, bool, error) {
				calls++
				if gotCtx != ctx || key != tt.key || input != tt.input {
					t.Fatalf("repository received changed context/key/input: %v, %q, %+v", gotCtx, key, input)
				}
				return want, false, nil
			})
			got, replayed, err := adjustments.New(repository).Adjust(ctx, tt.key, tt.input)
			if got != want || replayed || err != nil || calls != 1 {
				t.Fatalf("got (%+v, %v, %v), calls=%d; want (%+v, false, nil), calls=1", got, replayed, err, calls, want)
			}
		})
	}
}

func TestAdjustForwardsRepositoryOutcomesWithoutRetry(t *testing.T) {
	want := contracts.Adjustment{
		ID: "adj_0123456789abcdef0123456789abcdef", SKU: "SKU-001", Quantity: 2, Reason: "Restock",
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC),
	}
	tests := []struct {
		name     string
		response contracts.Adjustment
		replayed bool
		err      error
	}{
		{name: "success", response: want},
		{name: "replay", response: want, replayed: true},
		{name: "unknown SKU", err: domain.ErrNotFound},
		{name: "total exceeds limit", err: domain.ErrInvalid},
		{name: "idempotency conflict", err: domain.ErrIdempotencyConflict},
		{name: "cancelled", err: context.Canceled},
		{name: "deadline", err: context.DeadlineExceeded},
		{name: "wrapped error", err: fmt.Errorf("apply: %w", domain.ErrInvariant)},
		{name: "unexpected error", err: errors.New("repository unavailable")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			repository := repositoryFunc(func(context.Context, string, contracts.AdjustStock) (contracts.Adjustment, bool, error) {
				calls++
				return tt.response, tt.replayed, tt.err
			})
			service := adjustments.New(repository)
			input := contracts.AdjustStock{SKU: "SKU-001", Quantity: 2, Reason: "Restock"}
			// Each repeated call must still consult the repository; the service
			// neither caches responses/errors nor retries repository operations.
			for i := 1; i <= 2; i++ {
				got, replayed, err := service.Adjust(context.Background(), "adjust-001", input)
				if got != tt.response || replayed != tt.replayed || err != tt.err || calls != i {
					t.Fatalf("got (%+v, %v, %v), calls=%d; want (%+v, %v, %v), calls=%d", got, replayed, err, calls, tt.response, tt.replayed, tt.err, i)
				}
			}
		})
	}
}
