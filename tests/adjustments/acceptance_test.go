package adjustments_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"example.com/agentic-go-shop/internal/adapters/memory"
	"example.com/agentic-go-shop/internal/adjustments"
	"example.com/agentic-go-shop/internal/contracts"
	"example.com/agentic-go-shop/internal/domain"
	"example.com/agentic-go-shop/internal/httpapi"
	"example.com/agentic-go-shop/internal/inventory"
	"example.com/agentic-go-shop/internal/orders"
)

const acceptanceSKU = "SKU-001"

type fixture struct {
	store             *memory.Store
	orders            *orders.Service
	adjustmentHandler http.HandlerFunc
}

func newFixture(t *testing.T, available int) *fixture {
	t.Helper()
	store, err := memory.New([]domain.Stock{{SKU: acceptanceSKU, Available: available}})
	if err != nil {
		t.Fatal(err)
	}
	adjustmentService := adjustments.New(store)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &fixture{
		store:             store,
		orders:            orders.New(store, inventory.Service{}),
		adjustmentHandler: httpapi.AdjustmentHandler(adjustmentService, logger),
	}
}

func (f *fixture) adjust(key string, quantity int, reason string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	_ = json.NewEncoder(&body).Encode(struct {
		Quantity int    `json:"quantity"`
		Reason   string `json:"reason"`
	}{Quantity: quantity, Reason: reason})
	req := httptest.NewRequest(http.MethodPost, "/v1/inventory/"+acceptanceSKU+"/adjustments", &body)
	req.SetPathValue("sku", acceptanceSKU)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
	recorder := httptest.NewRecorder()
	f.adjustmentHandler.ServeHTTP(recorder, req)
	return recorder
}

func (f *fixture) stock(t *testing.T) domain.Stock {
	t.Helper()
	stock, err := inventory.Get(context.Background(), f.store, acceptanceSKU)
	if err != nil {
		t.Fatal(err)
	}
	return stock
}

func decodeReceipt(t *testing.T, recorder *httptest.ResponseRecorder) contracts.Adjustment {
	t.Helper()
	var receipt contracts.Adjustment
	if err := json.Unmarshal(recorder.Body.Bytes(), &receipt); err != nil {
		t.Fatalf("decode receipt: %v; body=%s", err, recorder.Body.String())
	}
	return receipt
}

func errorCode(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var response struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error: %v; body=%s", err, recorder.Body.String())
	}
	return response.Error.Code
}

func TestOrderAdjustmentCancellationPreservesUniqueStockTotal(t *testing.T) {
	const initialStock = 10
	f := newFixture(t, initialStock)
	ctx := context.Background()
	order, replayed, err := f.orders.Create(ctx, "order-lifecycle-001", domain.CreateOrder{
		CustomerID: "customer-001",
		Items:      []domain.Item{{SKU: acceptanceSKU, Quantity: 4}},
	})
	if err != nil || replayed {
		t.Fatalf("create order: replayed=%v err=%v", replayed, err)
	}

	created := f.adjust("adjust-lifecycle-001", 3, "supplier restock")
	if created.Code != http.StatusCreated || created.Header().Get("Idempotency-Replayed") != "false" {
		t.Fatalf("create adjustment: status=%d replay=%q body=%s", created.Code, created.Header().Get("Idempotency-Replayed"), created.Body.String())
	}
	createdReceipt := decodeReceipt(t, created)

	replayedResponse := f.adjust("adjust-lifecycle-001", 3, "supplier restock")
	if replayedResponse.Code != http.StatusCreated || replayedResponse.Header().Get("Idempotency-Replayed") != "true" {
		t.Fatalf("replay adjustment: status=%d replay=%q body=%s", replayedResponse.Code, replayedResponse.Header().Get("Idempotency-Replayed"), replayedResponse.Body.String())
	}
	if replayedReceipt := decodeReceipt(t, replayedResponse); !reflect.DeepEqual(replayedReceipt, createdReceipt) {
		t.Fatalf("replay receipt=%+v, want original=%+v", replayedReceipt, createdReceipt)
	}

	if _, err := f.orders.Cancel(ctx, order.ID); err != nil {
		t.Fatalf("cancel order: %v", err)
	}
	if got := f.stock(t); got.Available != initialStock+createdReceipt.Quantity || got.Reserved != 0 {
		t.Fatalf("final stock=%+v, want available=%d reserved=0", got, initialStock+createdReceipt.Quantity)
	}
}

func TestConcurrentAdjustmentKeyAddsStockOnceAndConflicts(t *testing.T) {
	const workers = 32
	f := newFixture(t, 10)
	start := make(chan struct{})
	type result struct {
		receipt contracts.Adjustment
		replay  string
		status  int
		body    string
		err     error
	}
	results := make(chan result, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			recorder := f.adjust("concurrent-adjustment-001", 2, "concurrent restock")
			got := result{status: recorder.Code, replay: recorder.Header().Get("Idempotency-Replayed"), body: recorder.Body.String()}
			got.err = json.Unmarshal(recorder.Body.Bytes(), &got.receipt)
			results <- got
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	created, replayed := 0, 0
	var original contracts.Adjustment
	for got := range results {
		if got.status != http.StatusCreated || got.err != nil {
			t.Fatalf("concurrent adjustment: status=%d decode=%v body=%s", got.status, got.err, got.body)
		}
		if original.ID == "" {
			original = got.receipt
		} else if !reflect.DeepEqual(got.receipt, original) {
			t.Fatalf("receipt=%+v, want original=%+v", got.receipt, original)
		}
		switch got.replay {
		case "false":
			created++
		case "true":
			replayed++
		default:
			t.Fatalf("unexpected Idempotency-Replayed=%q", got.replay)
		}
	}
	if created != 1 || replayed != workers-1 {
		t.Fatalf("created=%d replayed=%d, want 1/%d", created, replayed, workers-1)
	}
	if got := f.stock(t); got.Available != 12 || got.Reserved != 0 {
		t.Fatalf("concurrent replay changed stock more than once: %+v", got)
	}

	conflict := f.adjust("concurrent-adjustment-001", 3, "concurrent restock")
	if conflict.Code != http.StatusConflict || errorCode(t, conflict) != "idempotency_conflict" {
		t.Fatalf("conflict: status=%d body=%s", conflict.Code, conflict.Body.String())
	}
	if got := f.stock(t); got.Available != 12 || got.Reserved != 0 {
		t.Fatalf("conflict mutated stock: %+v", got)
	}
}

func TestAdjustmentLimitIncludesReservedStockAndFailedKeyCanRetry(t *testing.T) {
	f := newFixture(t, domain.MaxQuantity-1)
	order, replayed, err := f.orders.Create(context.Background(), "order-limit-001", domain.CreateOrder{
		CustomerID: "customer-001",
		Items:      []domain.Item{{SKU: acceptanceSKU, Quantity: 2}},
	})
	if err != nil || replayed {
		t.Fatalf("create order: replayed=%v err=%v", replayed, err)
	}

	overLimit := f.adjust("adjust-limit-001", 2, "too much while reserved")
	if overLimit.Code != http.StatusBadRequest || errorCode(t, overLimit) != "invalid_input" {
		t.Fatalf("over-limit adjustment: status=%d body=%s", overLimit.Code, overLimit.Body.String())
	}
	if got := f.stock(t); got.Available != domain.MaxQuantity-3 || got.Reserved != 2 {
		t.Fatalf("failed adjustment mutated stock: %+v", got)
	}

	retry := f.adjust("adjust-limit-001", 1, "fits exactly while reserved")
	if retry.Code != http.StatusCreated || retry.Header().Get("Idempotency-Replayed") != "false" {
		t.Fatalf("retry adjustment: status=%d replay=%q body=%s", retry.Code, retry.Header().Get("Idempotency-Replayed"), retry.Body.String())
	}
	if got := f.stock(t); got.Available != domain.MaxQuantity-2 || got.Reserved != 2 {
		t.Fatalf("retry stock=%+v", got)
	}
	if _, err := f.orders.Cancel(context.Background(), order.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.stock(t); got.Available != domain.MaxQuantity || got.Reserved != 0 {
		t.Fatalf("cancel stock=%+v", got)
	}
}

func TestAdjustmentConcurrentWithOrderLifecycleSharesSerialization(t *testing.T) {
	const orderCount = 24
	f := newFixture(t, orderCount)
	start := make(chan struct{})
	errs := make(chan error, orderCount+1)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		recorder := f.adjust("adjust-concurrent-order-001", 1, "concurrent delivery")
		if recorder.Code != http.StatusCreated || recorder.Header().Get("Idempotency-Replayed") != "false" {
			errs <- fmt.Errorf("adjustment status=%d replay=%q body=%s", recorder.Code, recorder.Header().Get("Idempotency-Replayed"), recorder.Body.String())
		}
	}()
	for i := range orderCount {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			order, replayed, err := f.orders.Create(context.Background(), fmt.Sprintf("order-concurrent-%03d", i), domain.CreateOrder{
				CustomerID: "customer-001",
				Items:      []domain.Item{{SKU: acceptanceSKU, Quantity: 1}},
			})
			if err != nil {
				errs <- fmt.Errorf("create order %d: %w", i, err)
				return
			}
			if replayed {
				errs <- fmt.Errorf("create order %d unexpectedly replayed", i)
				return
			}
			if _, err := f.orders.Cancel(context.Background(), order.ID); err != nil {
				errs <- fmt.Errorf("cancel order %d: %w", i, err)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
	if got := f.stock(t); got.Available != orderCount+1 || got.Reserved != 0 {
		t.Fatalf("final stock=%+v, want available=%d reserved=0", got, orderCount+1)
	}

	_, replayed, err := f.orders.Create(context.Background(), "order-concurrent-000", domain.CreateOrder{
		CustomerID: "customer-001",
		Items:      []domain.Item{{SKU: acceptanceSKU, Quantity: 2}},
	})
	if !errors.Is(err, domain.ErrIdempotencyConflict) || replayed {
		t.Fatalf("order key conflict after concurrency: replayed=%v err=%v", replayed, err)
	}
}
