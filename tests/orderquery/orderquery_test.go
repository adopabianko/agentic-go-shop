package orderquery_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"example.com/agentic-go-shop/internal/adapters/memory"
	"example.com/agentic-go-shop/internal/contracts"
	"example.com/agentic-go-shop/internal/domain"
	"example.com/agentic-go-shop/internal/httpapi"
	"example.com/agentic-go-shop/internal/inventory"
	"example.com/agentic-go-shop/internal/orderquery"
	"example.com/agentic-go-shop/internal/orders"
)

type fixture struct {
	t      *testing.T
	server *httptest.Server
	spec   map[string]any
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	store, err := memory.New([]domain.Stock{{SKU: "SKU-001", Available: 100}})
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	orderService := orders.New(store, inventory.Service{})
	baseline := httpapi.New(orderService, store, logger)
	list := httpapi.OrderListHandler(orderquery.New(store), logger)

	// T09 owns production routing. This acceptance fixture only composes the
	// frozen list handler with the existing create/cancel handler.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/orders" {
			list.ServeHTTP(w, r)
			return
		}
		baseline.ServeHTTP(w, r)
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate acceptance test source")
	}
	document, err := os.ReadFile(filepath.Join(filepath.Dir(filename), "..", "..", "api", "next.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err := json.Unmarshal(document, &spec); err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, server: server, spec: spec}
}

func TestOrderQueryHTTPPaginationLifecycleAndDraftSchema(t *testing.T) {
	f := newFixture(t)

	created := []domain.Order{
		f.create("customer-a", "t08-order-001"),
		f.create("customer-b", "t08-order-002"),
		f.create("customer-a", "t08-order-003"),
		f.create("customer-b", "t08-order-004"),
		f.create("customer-a", "t08-order-005"),
	}
	cancelled := f.cancel(created[2].ID)
	if cancelled.Status != domain.Cancelled {
		t.Fatalf("cancel status = %q, want %q", cancelled.Status, domain.Cancelled)
	}

	all := f.allPages("", 2, "")
	if len(all) != len(created) {
		t.Fatalf("listed %d orders, want %d", len(all), len(created))
	}
	assertUniqueIDs(t, all)
	assertSortedOrders(t, all)
	assertSameIDs(t, all, created)
	for _, order := range all {
		wantStatus := domain.Reserved
		if order.ID == cancelled.ID {
			wantStatus = domain.Cancelled
		}
		if order.Status != wantStatus {
			t.Fatalf("order %s status = %q, want %q", order.ID, order.Status, wantStatus)
		}
	}

	filtered := f.allPages("customer-a", 1, "")
	if len(filtered) != 3 {
		t.Fatalf("customer-a listed %d orders, want 3", len(filtered))
	}
	assertUniqueIDs(t, filtered)
	for _, order := range filtered {
		if order.CustomerID != "customer-a" {
			t.Fatalf("filtered response contains customer %q", order.CustomerID)
		}
	}

	empty := f.list("customer-empty", 10, "", http.StatusOK)
	if empty.Items == nil || len(empty.Items) != 0 || empty.NextCursor != "" {
		t.Fatalf("empty page = %#v", empty)
	}

	first := f.list("customer-a", 1, "", http.StatusOK)
	if first.NextCursor == "" {
		t.Fatal("first filtered page has no cursor")
	}
	values := url.Values{"customer_id": {"customer-b"}, "limit": {"1"}, "cursor": {first.NextCursor}}
	status, raw := f.request(http.MethodGet, "/v1/orders?"+values.Encode(), nil, "")
	if status != http.StatusBadRequest {
		t.Fatalf("cross-filter cursor status = %d, want 400: %s", status, raw)
	}
	var failure struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Error.Code != "invalid_input" {
		t.Fatalf("cross-filter cursor error = %q", failure.Error.Code)
	}
}

func TestOrderQueryHTTPInsertAfterCursorCanAppear(t *testing.T) {
	f := newFixture(t)
	before := []domain.Order{
		f.create("customer-a", "t08-insert-001"),
		f.create("customer-a", "t08-insert-002"),
		f.create("customer-a", "t08-insert-003"),
	}
	first := f.list("customer-a", 2, "", http.StatusOK)
	if first.NextCursor == "" {
		t.Fatal("first page has no cursor")
	}

	// Pagination is not a cross-request snapshot. An order committed after the
	// first read and ordered after its cursor is eligible for later pages.
	inserted := f.create("customer-a", "t08-insert-004")
	got := append([]domain.Order(nil), first.Items...)
	got = append(got, f.allPages("customer-a", 2, first.NextCursor)...)
	assertUniqueIDs(t, got)
	assertSortedOrders(t, got)
	want := append(before, inserted)
	assertSameIDs(t, got, want)
}

func (f *fixture) create(customerID, key string) domain.Order {
	f.t.Helper()
	body, err := json.Marshal(domain.CreateOrder{
		CustomerID: customerID,
		Items:      []domain.Item{{SKU: "SKU-001", Quantity: 1}},
	})
	if err != nil {
		f.t.Fatal(err)
	}
	status, raw := f.request(http.MethodPost, "/v1/orders", body, key)
	if status != http.StatusCreated {
		f.t.Fatalf("create status = %d, want 201: %s", status, raw)
	}
	var order domain.Order
	if err := json.Unmarshal(raw, &order); err != nil {
		f.t.Fatal(err)
	}
	return order
}

func (f *fixture) cancel(id string) domain.Order {
	f.t.Helper()
	status, raw := f.request(http.MethodPost, "/v1/orders/"+id+"/cancel", nil, "")
	if status != http.StatusOK {
		f.t.Fatalf("cancel status = %d, want 200: %s", status, raw)
	}
	var order domain.Order
	if err := json.Unmarshal(raw, &order); err != nil {
		f.t.Fatal(err)
	}
	return order
}

func (f *fixture) allPages(customerID string, limit int, cursor string) []domain.Order {
	f.t.Helper()
	var orders []domain.Order
	seenCursors := map[string]bool{}
	for {
		page := f.list(customerID, limit, cursor, http.StatusOK)
		orders = append(orders, page.Items...)
		if page.NextCursor == "" {
			return orders
		}
		if seenCursors[page.NextCursor] {
			f.t.Fatalf("cursor repeated: %q", page.NextCursor)
		}
		seenCursors[page.NextCursor] = true
		cursor = page.NextCursor
	}
}

func (f *fixture) list(customerID string, limit int, cursor string, wantStatus int) contracts.OrderPage {
	f.t.Helper()
	values := url.Values{"limit": {strconv.Itoa(limit)}}
	if customerID != "" {
		values.Set("customer_id", customerID)
	}
	if cursor != "" {
		values.Set("cursor", cursor)
	}
	status, raw := f.request(http.MethodGet, "/v1/orders?"+values.Encode(), nil, "")
	if status != wantStatus {
		f.t.Fatalf("list status = %d, want %d: %s", status, wantStatus, raw)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		f.t.Fatal(err)
	}
	validateSchema(f.t, f.spec, orderPageResponseSchema(f.t, f.spec), value, "response")

	var page contracts.OrderPage
	if err := json.Unmarshal(raw, &page); err != nil {
		f.t.Fatal(err)
	}
	if page.Items == nil {
		f.t.Fatal("OrderPage.items decoded as nil; contract requires an array")
	}
	return page
}

func (f *fixture) request(method, path string, body []byte, key string) (int, []byte) {
	f.t.Helper()
	request, err := http.NewRequest(method, f.server.URL+path, bytes.NewReader(body))
	if err != nil {
		f.t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	response, err := f.server.Client().Do(request)
	if err != nil {
		f.t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		f.t.Fatal(err)
	}
	if response.Header.Get("Content-Type") != "application/json" {
		f.t.Fatalf("Content-Type = %q", response.Header.Get("Content-Type"))
	}
	return response.StatusCode, raw
}

func orderPageResponseSchema(t *testing.T, spec map[string]any) map[string]any {
	t.Helper()
	paths := spec["paths"].(map[string]any)
	operation := paths["/v1/orders"].(map[string]any)["get"].(map[string]any)
	response := operation["responses"].(map[string]any)["200"].(map[string]any)
	return response["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
}

// validateSchema intentionally supports only the vocabulary used by OrderPage
// and its referenced schemas. Unknown keywords fail so contract changes cannot
// silently weaken this acceptance check.
func validateSchema(t *testing.T, root, schema map[string]any, value any, path string) {
	t.Helper()
	for key := range schema {
		switch key {
		case "$ref", "type", "properties", "required", "additionalProperties", "items", "minItems", "maxItems", "minimum", "maximum", "minLength", "maxLength", "pattern", "enum", "format", "description":
		default:
			t.Fatalf("unsupported schema keyword %s at %s", key, path)
		}
	}
	if reference, ok := schema["$ref"].(string); ok {
		var node any = root
		for _, part := range strings.Split(strings.TrimPrefix(reference, "#/"), "/") {
			node = node.(map[string]any)[part]
		}
		validateSchema(t, root, node.(map[string]any), value, path)
		return
	}
	if enum, ok := schema["enum"].([]any); ok {
		for _, candidate := range enum {
			if reflect.DeepEqual(candidate, value) {
				goto validType
			}
		}
		t.Fatalf("%s invalid enum: %v", path, value)
	}

validType:
	switch schema["type"] {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("%s expected object", path)
		}
		properties, _ := schema["properties"].(map[string]any)
		if required, ok := schema["required"].([]any); ok {
			for _, name := range required {
				if _, exists := object[name.(string)]; !exists {
					t.Fatalf("%s missing %s", path, name)
				}
			}
		}
		for key, item := range object {
			if property, ok := properties[key].(map[string]any); ok {
				validateSchema(t, root, property, item, path+"."+key)
			} else if schema["additionalProperties"] == false {
				t.Fatalf("%s unexpected field %s", path, key)
			}
		}
	case "array":
		array, ok := value.([]any)
		if !ok {
			t.Fatalf("%s expected array", path)
		}
		validateBounds(t, schema, float64(len(array)), "minItems", "maxItems", path)
		for i, item := range array {
			validateSchema(t, root, schema["items"].(map[string]any), item, fmt.Sprintf("%s[%d]", path, i))
		}
	case "string":
		text, ok := value.(string)
		if !ok {
			t.Fatalf("%s expected string", path)
		}
		validateBounds(t, schema, float64(len([]rune(text))), "minLength", "maxLength", path)
		if pattern, ok := schema["pattern"].(string); ok && !regexp.MustCompile(pattern).MatchString(text) {
			t.Fatalf("%s invalid pattern: %q", path, text)
		}
		if format, ok := schema["format"].(string); ok {
			if format != "date-time" {
				t.Fatalf("unsupported format %s", format)
			}
			if _, err := time.Parse(time.RFC3339Nano, text); err != nil {
				t.Fatalf("%s invalid date-time: %v", path, err)
			}
		}
	case "integer":
		number, ok := value.(float64)
		if !ok || number != float64(int64(number)) {
			t.Fatalf("%s expected integer", path)
		}
		validateBounds(t, schema, number, "minimum", "maximum", path)
	default:
		t.Fatalf("unsupported type %v at %s", schema["type"], path)
	}
}

func validateBounds(t *testing.T, schema map[string]any, value float64, minimum, maximum, path string) {
	t.Helper()
	if bound, ok := schema[minimum].(float64); ok && value < bound {
		t.Fatalf("%s below %s", path, minimum)
	}
	if bound, ok := schema[maximum].(float64); ok && value > bound {
		t.Fatalf("%s above %s", path, maximum)
	}
}

func assertUniqueIDs(t *testing.T, orders []domain.Order) {
	t.Helper()
	seen := make(map[string]bool, len(orders))
	for _, order := range orders {
		if seen[order.ID] {
			t.Fatalf("duplicate order ID across pages: %s", order.ID)
		}
		seen[order.ID] = true
	}
}

func assertSortedOrders(t *testing.T, orders []domain.Order) {
	t.Helper()
	if !sort.SliceIsSorted(orders, func(i, j int) bool {
		if orders[i].CreatedAt.Equal(orders[j].CreatedAt) {
			return orders[i].ID < orders[j].ID
		}
		return orders[i].CreatedAt.Before(orders[j].CreatedAt)
	}) {
		t.Fatalf("orders are not sorted by (created_at,id): %#v", orders)
	}
}

func assertSameIDs(t *testing.T, got, want []domain.Order) {
	t.Helper()
	gotIDs := make([]string, len(got))
	wantIDs := make([]string, len(want))
	for i := range got {
		gotIDs[i] = got[i].ID
	}
	for i := range want {
		wantIDs[i] = want[i].ID
	}
	sort.Strings(gotIDs)
	sort.Strings(wantIDs)
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("order IDs = %v, want %v", gotIDs, wantIDs)
	}
}
