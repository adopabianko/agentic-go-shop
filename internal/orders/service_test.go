package orders_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"example.com/agentic-go-shop/internal/adapters/memory"
	"example.com/agentic-go-shop/internal/domain"
	"example.com/agentic-go-shop/internal/inventory"
	"example.com/agentic-go-shop/internal/orders"
	"example.com/agentic-go-shop/internal/ports"
)

func setup(t *testing.T, qty int) (*orders.Service, ports.Store) {
	t.Helper()
	s, err := memory.New([]domain.Stock{{SKU: "A", Available: qty}, {SKU: "B", Available: 2}})
	if err != nil {
		t.Fatal(err)
	}
	return orders.New(s, inventory.Service{}), s
}
func input(qty int) domain.CreateOrder {
	return domain.CreateOrder{CustomerID: "customer-1", Items: []domain.Item{{SKU: "A", Quantity: qty}}}
}
func stock(t *testing.T, s ports.Store) domain.Stock {
	t.Helper()
	v, err := inventory.Get(context.Background(), s, "A")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestConcurrentLastStock(t *testing.T) {
	svc, s := setup(t, 1)
	var wg sync.WaitGroup
	results := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := svc.Create(context.Background(), fmt.Sprintf("request-%03d", i), input(1))
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	success, insufficient := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, domain.ErrInsufficientStock) {
			insufficient++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || insufficient != 31 {
		t.Fatalf("success=%d insufficient=%d", success, insufficient)
	}
	if v := stock(t, s); v.Available != 0 || v.Reserved != 1 {
		t.Fatal(v)
	}
}

func TestConcurrentSameKey(t *testing.T) {
	svc, s := setup(t, 10)
	var wg sync.WaitGroup
	ids := make(chan string, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o, _, err := svc.Create(context.Background(), "same-key-001", input(2))
			if err != nil {
				t.Error(err)
				return
			}
			ids <- o.ID
		}()
	}
	wg.Wait()
	close(ids)
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if id != first {
			t.Fatal("duplicate orders")
		}
	}
	if first == "" {
		t.Fatal("no result")
	}
	if v := stock(t, s); v.Available != 8 || v.Reserved != 2 {
		t.Fatal(v)
	}
}

func TestCanonicalReplayConflictAndCancellation(t *testing.T) {
	svc, s := setup(t, 10)
	ctx := context.Background()
	request := input(2)
	request.Items = append(request.Items, domain.Item{SKU: "B", Quantity: 1})
	original, replayed, err := svc.Create(ctx, "replay-key-001", request)
	if err != nil || replayed {
		t.Fatalf("%v %v", err, replayed)
	}
	request.Items[0], request.Items[1] = request.Items[1], request.Items[0]
	replay, replayed, err := svc.Create(ctx, "replay-key-001", request)
	if err != nil || !replayed || !reflect.DeepEqual(original, replay) {
		t.Fatalf("bad replay: %+v %v", replay, err)
	}
	_, _, err = svc.Create(ctx, "replay-key-001", input(3))
	if !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.Cancel(ctx, original.ID); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	cancelled, err := svc.Get(ctx, original.ID)
	if err != nil || cancelled.Status != domain.Cancelled {
		t.Fatalf("%+v %v", cancelled, err)
	}
	again, err := svc.Cancel(ctx, original.ID)
	if err != nil || !reflect.DeepEqual(cancelled, again) {
		t.Fatal("repeat cancellation changed order")
	}
	if v := stock(t, s); v.Available != 10 || v.Reserved != 0 {
		t.Fatal(v)
	}
	replay, replayed, err = svc.Create(ctx, "replay-key-001", request)
	if err != nil || !replayed || !reflect.DeepEqual(original, replay) {
		t.Fatal("original response not preserved after cancellation")
	}
}

func TestPartialReservationAndFailedKeyRollback(t *testing.T) {
	svc, s := setup(t, 10)
	request := input(2)
	request.Items = append(request.Items, domain.Item{SKU: "B", Quantity: 3})
	if _, _, err := svc.Create(context.Background(), "retry-key-001", request); !errors.Is(err, domain.ErrInsufficientStock) {
		t.Fatal(err)
	}
	if v := stock(t, s); v.Available != 10 || v.Reserved != 0 {
		t.Fatal("partial reservation", v)
	}
	if _, replay, err := svc.Create(context.Background(), "retry-key-001", input(2)); err != nil || replay {
		t.Fatalf("failed key consumed: %v", err)
	}
}

type brokenInventory struct{ inventory.Service }

func (brokenInventory) Reserve(tx ports.Tx, items []domain.Item) error {
	if err := (inventory.Service{}).Reserve(tx, items); err != nil {
		return err
	}
	return errors.New("injected failure after stock mutation")
}

type brokenRelease struct{ inventory.Service }

func (brokenRelease) Release(tx ports.Tx, items []domain.Item) error {
	if err := (inventory.Service{}).Release(tx, items); err != nil {
		return err
	}
	return errors.New("injected release failure")
}

func TestDependencyFailureRollsBack(t *testing.T) {
	svc, s := setup(t, 10)
	broken := orders.New(s, brokenInventory{})
	if _, _, err := broken.Create(context.Background(), "failure-key-001", input(2)); err == nil {
		t.Fatal("expected failure")
	}
	if v := stock(t, s); v.Available != 10 || v.Reserved != 0 {
		t.Fatal(v)
	}
	o, _, err := svc.Create(context.Background(), "failure-key-001", input(2))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := orders.New(s, brokenRelease{}).Cancel(context.Background(), o.ID); err == nil {
		t.Fatal("expected release failure")
	}
	current, _ := svc.Get(context.Background(), o.ID)
	if current.Status != domain.Reserved {
		t.Fatal("status escaped rollback")
	}
	if v := stock(t, s); v.Available != 8 || v.Reserved != 2 {
		t.Fatal("release escaped rollback", v)
	}
}

func TestInvalidInput(t *testing.T) {
	svc, s := setup(t, 10)
	for _, request := range []domain.CreateOrder{
		{}, {CustomerID: "bad id", Items: input(1).Items}, input(0), input(-1), input(domain.MaxQuantity + 1),
		{CustomerID: "c", Items: []domain.Item{{SKU: "A", Quantity: 1}, {SKU: "A", Quantity: 1}}},
	} {
		if _, _, err := svc.Create(context.Background(), "valid-key-001", request); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("%+v: %v", request, err)
		}
	}
	if _, _, err := svc.Create(context.Background(), "short", input(1)); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	if _, _, err := svc.Create(context.Background(), "missing-sku-001", domain.CreateOrder{CustomerID: "c", Items: []domain.Item{{SKU: "missing", Quantity: 1}}}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := svc.Cancel(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := svc.Get(context.Background(), "bad id"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatal(err)
	}
	if v := stock(t, s); v.Available != 10 {
		t.Fatal(v)
	}
}
