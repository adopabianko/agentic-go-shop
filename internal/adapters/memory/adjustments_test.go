package memory_test

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"testing"
	"time"

	"example.com/agentic-go-shop/internal/adapters/memory"
	"example.com/agentic-go-shop/internal/contracts"
	"example.com/agentic-go-shop/internal/domain"
	"example.com/agentic-go-shop/internal/inventory"
	"example.com/agentic-go-shop/internal/orders"
	"example.com/agentic-go-shop/internal/ports"
)

func newAdjustmentStore(t *testing.T, seed ...domain.Stock) *memory.Store {
	t.Helper()
	store, err := memory.New(seed)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func readStock(t *testing.T, store *memory.Store, sku string) domain.Stock {
	t.Helper()
	var got domain.Stock
	if err := store.WithinTx(context.Background(), func(tx ports.Tx) error {
		var err error
		got, err = tx.Stock(sku)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestApplyAdjustmentCreatesAndReplaysOriginalReceipt(t *testing.T) {
	store := newAdjustmentStore(t, domain.Stock{SKU: "SKU-001", Available: 4})
	input := contracts.AdjustStock{SKU: "SKU-001", Quantity: 3, Reason: "  restock: shelf A  "}

	first, replayed, err := store.ApplyAdjustment(context.Background(), "adjustment-key-001", input)
	if err != nil || replayed {
		t.Fatalf("first call: replayed=%v err=%v", replayed, err)
	}
	if !regexp.MustCompile(`^adj_[a-f0-9]{32}$`).MatchString(first.ID) {
		t.Fatalf("invalid receipt ID %q", first.ID)
	}
	if first.SKU != input.SKU || first.Quantity != input.Quantity || first.Reason != input.Reason {
		t.Fatalf("receipt did not preserve request: %+v", first)
	}
	if first.CreatedAt.IsZero() || first.CreatedAt.Location() != time.UTC {
		t.Fatalf("receipt timestamp must be UTC: %v (%v)", first.CreatedAt, first.CreatedAt.Location())
	}

	replay, replayed, err := store.ApplyAdjustment(context.Background(), "adjustment-key-001", input)
	if err != nil || !replayed || replay != first {
		t.Fatalf("replay=%+v replayed=%v err=%v; want original %+v", replay, replayed, err, first)
	}
	if got := readStock(t, store, input.SKU); got.Available != 7 || got.Reserved != 0 {
		t.Fatalf("replay added stock more than once: %+v", got)
	}

	changed := input
	changed.Reason = "restock: shelf A"
	if _, replayed, err := store.ApplyAdjustment(context.Background(), "adjustment-key-001", changed); !errors.Is(err, domain.ErrIdempotencyConflict) || replayed {
		t.Fatalf("different payload: replayed=%v err=%v", replayed, err)
	}
	if got := readStock(t, store, input.SKU); got.Available != 7 {
		t.Fatalf("conflict changed stock: %+v", got)
	}
}

func TestApplyAdjustmentFailureDoesNotConsumeKeyOrMutateStock(t *testing.T) {
	store := newAdjustmentStore(t, domain.Stock{SKU: "SKU-001", Available: domain.MaxQuantity - 1})
	key := "retry-adjustment-001"

	input := contracts.AdjustStock{SKU: "MISSING", Quantity: 1, Reason: "restock"}
	if _, replayed, err := store.ApplyAdjustment(context.Background(), key, input); !errors.Is(err, domain.ErrNotFound) || replayed {
		t.Fatalf("unknown SKU: replayed=%v err=%v", replayed, err)
	}
	if err := store.WithinTx(context.Background(), func(tx ports.Tx) error {
		tx.PutStock(domain.Stock{SKU: input.SKU, Available: 0})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	input.SKU = "SKU-001"
	input.Quantity = 2
	if _, replayed, err := store.ApplyAdjustment(context.Background(), key, input); !errors.Is(err, domain.ErrInvalid) || replayed {
		t.Fatalf("over-limit total: replayed=%v err=%v", replayed, err)
	}
	if got := readStock(t, store, "SKU-001"); got.Available != domain.MaxQuantity-1 || got.Reserved != 0 {
		t.Fatalf("failed adjustment partially mutated stock: %+v", got)
	}

	input.Quantity = 1
	if _, replayed, err := store.ApplyAdjustment(context.Background(), key, input); err != nil || replayed {
		t.Fatalf("retry after failure: replayed=%v err=%v", replayed, err)
	}
	if got := readStock(t, store, "SKU-001"); got.Available != domain.MaxQuantity || got.Reserved != 0 {
		t.Fatalf("successful retry has wrong stock: %+v", got)
	}
}

func TestApplyAdjustmentUsesSeparateKeyNamespaceFromOrders(t *testing.T) {
	store := newAdjustmentStore(t, domain.Stock{SKU: "SKU-001", Available: 5})
	key := "shared-key-001"
	input := contracts.AdjustStock{SKU: "SKU-001", Quantity: 2, Reason: "restock"}
	if _, _, err := store.ApplyAdjustment(context.Background(), key, input); err != nil {
		t.Fatal(err)
	}
	orderService := orders.New(store, inventory.Service{})
	order, replayed, err := orderService.Create(context.Background(), key, domain.CreateOrder{
		CustomerID: "customer-1", Items: []domain.Item{{SKU: "SKU-001", Quantity: 1}},
	})
	if err != nil || replayed {
		t.Fatalf("order key collided with adjustment namespace: order=%+v replayed=%v err=%v", order, replayed, err)
	}
	if got := readStock(t, store, "SKU-001"); got.Available != 6 || got.Reserved != 1 {
		t.Fatalf("unexpected stock after adjustment and order: %+v", got)
	}
}

func TestConcurrentAdjustmentReplayAddsStockOnce(t *testing.T) {
	store := newAdjustmentStore(t, domain.Stock{SKU: "SKU-001", Available: 10})
	input := contracts.AdjustStock{SKU: "SKU-001", Quantity: 2, Reason: "restock"}
	const workers = 32
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	results := make(chan struct {
		receipt  contracts.Adjustment
		replayed bool
	}, workers)
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			receipt, replayed, err := store.ApplyAdjustment(context.Background(), "same-adjustment-key", input)
			if err != nil {
				errs <- err
				return
			}
			results <- struct {
				receipt  contracts.Adjustment
				replayed bool
			}{receipt, replayed}
		}()
	}
	wg.Wait()
	close(errs)
	close(results)
	for err := range errs {
		t.Errorf("ApplyAdjustment: %v", err)
	}
	var first contracts.Adjustment
	created, replayedCount := 0, 0
	for got := range results {
		if first.ID == "" {
			first = got.receipt
		} else if got.receipt != first {
			t.Errorf("concurrent replay returned a different receipt: %+v vs %+v", got.receipt, first)
		}
		if got.replayed {
			replayedCount++
		} else {
			created++
		}
	}
	if created != 1 || replayedCount != workers-1 {
		t.Fatalf("created=%d replayed=%d", created, replayedCount)
	}
	if got := readStock(t, store, "SKU-001"); got.Available != 12 || got.Reserved != 0 {
		t.Fatalf("concurrent replays added stock more than once: %+v", got)
	}
}

func TestConcurrentAdjustmentsOrdersAndCancellationShareSerialization(t *testing.T) {
	const orderCount = 24
	store := newAdjustmentStore(t, domain.Stock{SKU: "SKU-001", Available: orderCount})
	service := orders.New(store, inventory.Service{})
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, orderCount+1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		if _, replayed, err := store.ApplyAdjustment(context.Background(), "concurrent-adjust-key", contracts.AdjustStock{
			SKU: "SKU-001", Quantity: 1, Reason: "restock",
		}); err != nil || replayed {
			errs <- fmt.Errorf("adjustment replayed=%v: %w", replayed, err)
		}
	}()
	for i := range orderCount {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			order, _, err := service.Create(context.Background(), fmt.Sprintf("order-concurrent-%03d", i), domain.CreateOrder{
				CustomerID: "customer-1", Items: []domain.Item{{SKU: "SKU-001", Quantity: 1}},
			})
			if err != nil {
				errs <- fmt.Errorf("create %d: %w", i, err)
				return
			}
			if _, err := service.Cancel(context.Background(), order.ID); err != nil {
				errs <- fmt.Errorf("cancel %d: %w", i, err)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if got := readStock(t, store, "SKU-001"); got.Available != orderCount+1 || got.Reserved != 0 {
		t.Fatalf("concurrent adjustment/order/cancel lost stock: %+v", got)
	}
}

type cancelBeforeCommitContext struct {
	context.Context
	checks int
}

func (ctx *cancelBeforeCommitContext) Err() error {
	ctx.checks++
	if ctx.checks == 3 {
		return context.Canceled
	}
	return nil
}

func TestApplyAdjustmentRollsBackWhenContextCancelsBeforeCommit(t *testing.T) {
	store := newAdjustmentStore(t, domain.Stock{SKU: "SKU-001", Available: 2})
	input := contracts.AdjustStock{SKU: "SKU-001", Quantity: 3, Reason: "restock"}
	ctx := &cancelBeforeCommitContext{Context: context.Background()}
	if _, replayed, err := store.ApplyAdjustment(ctx, "cancelled-adjustment-001", input); !errors.Is(err, context.Canceled) || replayed {
		t.Fatalf("cancelled transaction: replayed=%v err=%v", replayed, err)
	}
	if got := readStock(t, store, "SKU-001"); got.Available != 2 || got.Reserved != 0 {
		t.Fatalf("cancelled transaction committed stock: %+v", got)
	}
	receipt, replayed, err := store.ApplyAdjustment(context.Background(), "cancelled-adjustment-001", input)
	if err != nil || replayed || receipt.ID == "" {
		t.Fatalf("failed transaction consumed idempotency key: receipt=%+v replayed=%v err=%v", receipt, replayed, err)
	}
	if got := readStock(t, store, "SKU-001"); got.Available != 5 || got.Reserved != 0 {
		t.Fatalf("retry did not commit exactly once: %+v", got)
	}
}
