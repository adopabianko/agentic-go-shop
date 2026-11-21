package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"example.com/agentic-go-shop/internal/contracts"
	"example.com/agentic-go-shop/internal/domain"
	"example.com/agentic-go-shop/internal/httpapi"
)

type fakeOrderListService struct {
	page    contracts.OrderPage
	err     error
	queries []contracts.OrderQuery
}

func (f *fakeOrderListService) List(_ context.Context, query contracts.OrderQuery) (contracts.OrderPage, error) {
	f.queries = append(f.queries, query)
	return f.page, f.err
}

func newOrderListHandler(service contracts.OrderListUseCase) http.HandlerFunc {
	return httpapi.OrderListHandler(service, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestOrderListHandlerParsesQuery(t *testing.T) {
	now := time.Date(2026, time.November, 21, 10, 0, 0, 0, time.UTC)
	service := &fakeOrderListService{page: contracts.OrderPage{
		Items: []domain.Order{{
			ID:         "order-1",
			CustomerID: "customer-1",
			Items:      []domain.Item{{SKU: "SKU-001", Quantity: 1}},
			Status:     domain.Reserved,
			CreatedAt:  now,
			UpdatedAt:  now,
		}},
		NextCursor: "next-token",
	}}
	handler := newOrderListHandler(service)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/orders?customer_id=customer-1&limit=25&cursor=cursor%2Btoken", nil)
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	wantQuery := contracts.OrderQuery{CustomerID: "customer-1", Limit: 25, Cursor: "cursor+token"}
	if !reflect.DeepEqual(service.queries, []contracts.OrderQuery{wantQuery}) {
		t.Fatalf("queries = %#v, want %#v", service.queries, []contracts.OrderQuery{wantQuery})
	}
	var got contracts.OrderPage
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, service.page) {
		t.Fatalf("page = %#v, want %#v", got, service.page)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
}

func TestOrderListHandlerDefaultsAndEmptyPage(t *testing.T) {
	service := &fakeOrderListService{}
	recorder := httptest.NewRecorder()
	newOrderListHandler(service).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/orders", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if !reflect.DeepEqual(service.queries, []contracts.OrderQuery{{}}) {
		t.Fatalf("queries = %#v", service.queries)
	}
	if got, want := recorder.Body.String(), "{\"items\":[],\"next_cursor\":\"\"}\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestOrderListHandlerRejectsInvalidQuery(t *testing.T) {
	tests := []string{
		"?unknown=value",
		"?customer_id=one&customer_id=two",
		"?limit=1&limit=2",
		"?cursor=one&cursor=two",
		"?customer_id=",
		"?limit=",
		"?cursor=",
		"?limit=0",
		"?limit=-1",
		"?limit=101",
		"?limit=1.5",
		"?limit=ten",
		"?limit=999999999999999999999999",
		"?customer_id=ok;cursor=bad",
	}
	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			service := &fakeOrderListService{}
			recorder := httptest.NewRecorder()
			newOrderListHandler(service).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/orders"+query, nil))

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if len(service.queries) != 0 {
				t.Fatalf("service called with %#v", service.queries)
			}
			assertOrderListError(t, recorder, "", "invalid_input")
		})
	}
}

func TestOrderListHandlerAcceptsLimitBounds(t *testing.T) {
	for _, limit := range []string{"1", "100"} {
		t.Run(limit, func(t *testing.T) {
			service := &fakeOrderListService{page: contracts.OrderPage{Items: []domain.Order{}}}
			recorder := httptest.NewRecorder()
			newOrderListHandler(service).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/orders?limit="+limit, nil))

			if recorder.Code != http.StatusOK || len(service.queries) != 1 {
				t.Fatalf("status = %d, queries = %#v", recorder.Code, service.queries)
			}
		})
	}
}

func TestOrderListHandlerMapsServiceErrorsWithRequestID(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "invalid", err: domain.ErrInvalid, status: http.StatusBadRequest, code: "invalid_input"},
		{name: "cancelled", err: context.Canceled, status: http.StatusServiceUnavailable, code: "unavailable"},
		{name: "deadline", err: context.DeadlineExceeded, status: http.StatusServiceUnavailable, code: "unavailable"},
		{name: "unexpected", err: errors.New("boom"), status: http.StatusInternalServerError, code: "internal_error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &fakeOrderListService{err: test.err}
			recorder := httptest.NewRecorder()
			recorder.Header().Set("X-Request-ID", "request-id-123")
			newOrderListHandler(service).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/orders", nil))

			if recorder.Code != test.status {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			assertOrderListError(t, recorder, "request-id-123", test.code)
		})
	}
}

func assertOrderListError(t *testing.T, recorder *httptest.ResponseRecorder, requestID, code string) {
	t.Helper()
	var body struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != code || body.Error.RequestID != requestID {
		t.Fatalf("error = %#v, want code %q request ID %q", body.Error, code, requestID)
	}
}
