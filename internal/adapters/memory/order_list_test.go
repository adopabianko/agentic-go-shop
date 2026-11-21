package memory_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
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

func TestReadOrdersPaginatesByCreatedAtAndID(t *testing.T) {
	store := newOrderListStore(t, nil)
	createdAt := time.Date(2026, time.November, 21, 10, 11, 12, 123456789, time.UTC)
	putOrders(t, store,
		newStoredOrder("order-e", "customer-a", createdAt),
		newStoredOrder("order-c", "customer-a", createdAt),
		newStoredOrder("order-a", "customer-a", createdAt),
		newStoredOrder("order-d", "customer-a", createdAt),
		newStoredOrder("order-b", "customer-a", createdAt),
	)

	first := readOrders(t, store, contracts.OrderQuery{Limit: 2})
	assertOrderIDs(t, first.Items, "order-a", "order-b")
	if first.NextCursor == "" || strings.Contains(first.NextCursor, "=") {
		t.Fatalf("cursor is not unpadded base64url: %q", first.NextCursor)
	}
	assertCursor(t, first.NextCursor, createdAt, "order-b", "")

	// There is deliberately no cross-request snapshot. A later insert after the
	// cursor is eligible for subsequent pages.
	putOrders(t, store, newStoredOrder("order-f", "customer-a", createdAt.Add(time.Second)))
	second := readOrders(t, store, contracts.OrderQuery{Limit: 2, Cursor: first.NextCursor})
	assertOrderIDs(t, second.Items, "order-c", "order-d")
	if second.NextCursor == "" {
		t.Fatal("second page did not advertise remaining orders")
	}
	third := readOrders(t, store, contracts.OrderQuery{Limit: 2, Cursor: second.NextCursor})
	assertOrderIDs(t, third.Items, "order-e", "order-f")
	if third.NextCursor != "" {
		t.Fatalf("last page cursor = %q, want empty", third.NextCursor)
	}
}

func TestReadOrdersFiltersAndBindsCursor(t *testing.T) {
	store := newOrderListStore(t, nil)
	t0 := time.Date(2026, time.November, 21, 1, 0, 0, 0, time.UTC)
	putOrders(t, store,
		newStoredOrder("order-a1", "customer-a", t0),
		newStoredOrder("order-b1", "customer-b", t0.Add(time.Second)),
		newStoredOrder("order-a2", "customer-a", t0.Add(2*time.Second)),
	)

	first := readOrders(t, store, contracts.OrderQuery{CustomerID: "customer-a", Limit: 1})
	assertOrderIDs(t, first.Items, "order-a1")
	assertCursor(t, first.NextCursor, t0, "order-a1", "customer-a")
	second := readOrders(t, store, contracts.OrderQuery{CustomerID: "customer-a", Limit: 1, Cursor: first.NextCursor})
	assertOrderIDs(t, second.Items, "order-a2")
	if second.NextCursor != "" {
		t.Fatalf("last filtered cursor = %q, want empty", second.NextCursor)
	}

	_, err := store.ReadOrders(context.Background(), contracts.OrderQuery{CustomerID: "customer-b", Limit: 1, Cursor: first.NextCursor})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("filter mismatch error = %v, want ErrInvalid", err)
	}
}

func TestReadOrdersRejectsInvalidQueryAndCursor(t *testing.T) {
	store := newOrderListStore(t, nil)
	validTime := "2026-11-21T01:02:03.123456789Z"
	cursor := func(value string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(value))
	}
	cases := []struct {
		name  string
		query contracts.OrderQuery
	}{
		{name: "zero limit is not normalized", query: contracts.OrderQuery{Limit: 0}},
		{name: "limit too large", query: contracts.OrderQuery{Limit: 101}},
		{name: "invalid customer", query: contracts.OrderQuery{CustomerID: "bad customer", Limit: 1}},
		{name: "too long", query: contracts.OrderQuery{Limit: 1, Cursor: strings.Repeat("a", 2049)}},
		{name: "invalid base64", query: contracts.OrderQuery{Limit: 1, Cursor: "***"}},
		{name: "padded base64", query: contracts.OrderQuery{Limit: 1, Cursor: "e30="}},
		{name: "not object", query: contracts.OrderQuery{Limit: 1, Cursor: cursor(`[]`)}},
		{name: "unknown field", query: contracts.OrderQuery{Limit: 1, Cursor: cursor(fmt.Sprintf(`{"version":1,"created_at":%q,"id":"order-a","customer_id":"","extra":true}`, validTime))}},
		{name: "missing field", query: contracts.OrderQuery{Limit: 1, Cursor: cursor(fmt.Sprintf(`{"version":1,"created_at":%q,"id":"order-a"}`, validTime))}},
		{name: "duplicate field", query: contracts.OrderQuery{Limit: 1, Cursor: cursor(fmt.Sprintf(`{"version":1,"version":1,"created_at":%q,"id":"order-a","customer_id":""}`, validTime))}},
		{name: "wrong version", query: contracts.OrderQuery{Limit: 1, Cursor: cursor(fmt.Sprintf(`{"version":2,"created_at":%q,"id":"order-a","customer_id":""}`, validTime))}},
		{name: "invalid timestamp", query: contracts.OrderQuery{Limit: 1, Cursor: cursor(`{"version":1,"created_at":"yesterday","id":"order-a","customer_id":""}`)}},
		{name: "invalid id", query: contracts.OrderQuery{Limit: 1, Cursor: cursor(fmt.Sprintf(`{"version":1,"created_at":%q,"id":"bad id","customer_id":""}`, validTime))}},
		{name: "invalid cursor customer", query: contracts.OrderQuery{CustomerID: "customer-a", Limit: 1, Cursor: cursor(fmt.Sprintf(`{"version":1,"created_at":%q,"id":"order-a","customer_id":"bad customer"}`, validTime))}},
		{name: "null customer", query: contracts.OrderQuery{Limit: 1, Cursor: cursor(fmt.Sprintf(`{"version":1,"created_at":%q,"id":"order-a","customer_id":null}`, validTime))}},
		{name: "trailing value", query: contracts.OrderQuery{Limit: 1, Cursor: cursor(fmt.Sprintf(`{"version":1,"created_at":%q,"id":"order-a","customer_id":""} {}`, validTime))}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.ReadOrders(context.Background(), tc.query)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.ReadOrders(canceled, contracts.OrderQuery{Limit: 1}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled read error = %v", err)
	}
}

func TestReadOrdersReturnsIsolatedValuesAndNonNilEmptyItems(t *testing.T) {
	store := newOrderListStore(t, nil)
	empty := readOrders(t, store, contracts.OrderQuery{Limit: 10})
	if empty.Items == nil || len(empty.Items) != 0 || empty.NextCursor != "" {
		t.Fatalf("empty page = %#v", empty)
	}

	original := newStoredOrder("order-a", "customer-a", time.Now().UTC())
	putOrders(t, store, original)
	page := readOrders(t, store, contracts.OrderQuery{Limit: 10})
	page.Items[0].Items[0].Quantity = 999
	page.Items[0].Status = domain.Cancelled

	again := readOrders(t, store, contracts.OrderQuery{Limit: 10})
	if !reflect.DeepEqual(again.Items[0], original) {
		t.Fatalf("caller mutation leaked into store: got %+v want %+v", again.Items[0], original)
	}
}

func TestReadOrdersConcurrentWithCreate(t *testing.T) {
	const count = 50
	store := newOrderListStore(t, []domain.Stock{{SKU: "SKU-001", Available: count}})
	service := orders.New(store, inventory.Service{})

	var wg sync.WaitGroup
	errs := make(chan error, count+8)
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := service.Create(context.Background(), fmt.Sprintf("request-%03d", i), domain.CreateOrder{
				CustomerID: "customer-a",
				Items:      []domain.Item{{SKU: "SKU-001", Quantity: 1}},
			})
			if err != nil {
				errs <- err
			}
		}(i)
	}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				page, err := store.ReadOrders(context.Background(), contracts.OrderQuery{Limit: 100})
				if err != nil {
					errs <- err
					return
				}
				if !ordersAreSorted(page.Items) {
					errs <- errors.New("concurrent snapshot was not sorted")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	page := readOrders(t, store, contracts.OrderQuery{Limit: 100})
	if len(page.Items) != count || page.NextCursor != "" || !ordersAreSorted(page.Items) {
		t.Fatalf("final page: count=%d cursor=%q sorted=%v", len(page.Items), page.NextCursor, ordersAreSorted(page.Items))
	}
}

func newOrderListStore(t *testing.T, seed []domain.Stock) *memory.Store {
	t.Helper()
	store, err := memory.New(seed)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func newStoredOrder(id, customerID string, createdAt time.Time) domain.Order {
	return domain.Order{
		ID:         id,
		CustomerID: customerID,
		Items:      []domain.Item{{SKU: "SKU-001", Quantity: 1}},
		Status:     domain.Reserved,
		CreatedAt:  createdAt,
		UpdatedAt:  createdAt,
	}
}

func putOrders(t *testing.T, store ports.Store, values ...domain.Order) {
	t.Helper()
	if err := store.WithinTx(context.Background(), func(tx ports.Tx) error {
		for _, order := range values {
			tx.PutOrder(order)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func readOrders(t *testing.T, store *memory.Store, query contracts.OrderQuery) contracts.OrderPage {
	t.Helper()
	page, err := store.ReadOrders(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func assertOrderIDs(t *testing.T, values []domain.Order, want ...string) {
	t.Helper()
	got := make([]string, len(values))
	for i := range values {
		got[i] = values[i].ID
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order ids = %v, want %v", got, want)
	}
}

func assertCursor(t *testing.T, encoded string, createdAt time.Time, id, customerID string) {
	t.Helper()
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Version    int    `json:"version"`
		CreatedAt  string `json:"created_at"`
		ID         string `json:"id"`
		CustomerID string `json:"customer_id"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || got.CreatedAt != createdAt.Format(time.RFC3339Nano) || got.ID != id || got.CustomerID != customerID {
		t.Fatalf("cursor = %+v", got)
	}
}

func ordersAreSorted(values []domain.Order) bool {
	return sort.SliceIsSorted(values, func(i, j int) bool {
		if values[i].CreatedAt.Equal(values[j].CreatedAt) {
			return values[i].ID < values[j].ID
		}
		return values[i].CreatedAt.Before(values[j].CreatedAt)
	})
}
