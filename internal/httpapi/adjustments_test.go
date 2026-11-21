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
	"strings"
	"testing"
	"time"

	"example.com/agentic-go-shop/internal/contracts"
	"example.com/agentic-go-shop/internal/domain"
	"example.com/agentic-go-shop/internal/httpapi"
)

const adjustmentRequestID = "0123456789abcdef0123456789abcdef"

type adjustmentServiceFunc func(context.Context, string, contracts.AdjustStock) (contracts.Adjustment, bool, error)

func (f adjustmentServiceFunc) Adjust(ctx context.Context, key string, input contracts.AdjustStock) (contracts.Adjustment, bool, error) {
	return f(ctx, key, input)
}

func adjustmentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func serveAdjustment(t *testing.T, service contracts.AdjustmentUseCase, sku, body, mediaType string, keys ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/inventory/"+sku+"/adjustments", strings.NewReader(body))
	req.SetPathValue("sku", sku)
	if mediaType != "" {
		req.Header.Set("Content-Type", mediaType)
	}
	for _, key := range keys {
		req.Header.Add("Idempotency-Key", key)
	}
	recorder := httptest.NewRecorder()
	// T09 mounts this handler inside the existing observation middleware.
	recorder.Header().Set("X-Request-ID", adjustmentRequestID)
	httpapi.AdjustmentHandler(service, adjustmentLogger()).ServeHTTP(recorder, req)
	return recorder
}

func TestAdjustmentHandlerCreatesAndReplaysReceipt(t *testing.T) {
	wantReceipt := contracts.Adjustment{
		ID:        "adj_0123456789abcdef0123456789abcdef",
		SKU:       "SKU-001",
		Quantity:  7,
		Reason:    "supplier restock",
		CreatedAt: time.Date(2026, time.November, 21, 10, 30, 0, 123, time.UTC),
	}
	wantInput := contracts.AdjustStock{SKU: "SKU-001", Quantity: 7, Reason: "supplier restock"}

	for _, replayed := range []bool{false, true} {
		t.Run(map[bool]string{false: "created", true: "replayed"}[replayed], func(t *testing.T) {
			calls := 0
			service := adjustmentServiceFunc(func(_ context.Context, key string, input contracts.AdjustStock) (contracts.Adjustment, bool, error) {
				calls++
				if key != "adjustment-key-001" {
					t.Fatalf("key = %q", key)
				}
				if !reflect.DeepEqual(input, wantInput) {
					t.Fatalf("input = %+v, want %+v", input, wantInput)
				}
				return wantReceipt, replayed, nil
			})

			recorder := serveAdjustment(t, service, "SKU-001", `{"quantity":7,"reason":"supplier restock"}`, "application/json; charset=utf-8", "adjustment-key-001")
			if recorder.Code != http.StatusCreated {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if calls != 1 {
				t.Fatalf("calls = %d, want 1", calls)
			}
			if got := recorder.Header().Get("Idempotency-Replayed"); got != map[bool]string{false: "false", true: "true"}[replayed] {
				t.Fatalf("Idempotency-Replayed = %q", got)
			}
			if got := recorder.Header().Get("Location"); got != "" {
				t.Fatalf("unexpected Location = %q", got)
			}
			if got := recorder.Header().Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q", got)
			}
			var got contracts.Adjustment
			if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, wantReceipt) {
				t.Fatalf("receipt = %+v, want %+v", got, wantReceipt)
			}
		})
	}
}

func TestAdjustmentHandlerRejectsInvalidTransportInput(t *testing.T) {
	validBody := `{"quantity":1,"reason":"restock"}`
	tests := []struct {
		name      string
		body      string
		mediaType string
		keys      []string
		status    int
		code      string
	}{
		{name: "missing media type", body: validBody, keys: []string{"adjustment-key-001"}, status: 415, code: "unsupported_media_type"},
		{name: "wrong media type", body: validBody, mediaType: "text/plain", keys: []string{"adjustment-key-001"}, status: 415, code: "unsupported_media_type"},
		{name: "missing key", body: validBody, mediaType: "application/json", status: 400, code: "invalid_input"},
		{name: "multiple keys", body: validBody, mediaType: "application/json", keys: []string{"adjustment-key-001", "adjustment-key-002"}, status: 400, code: "invalid_input"},
		{name: "unknown field", body: `{"quantity":1,"reason":"restock","sku":"OTHER"}`, mediaType: "application/json", keys: []string{"adjustment-key-001"}, status: 400, code: "invalid_input"},
		{name: "trailing value", body: validBody + ` {}`, mediaType: "application/json", keys: []string{"adjustment-key-001"}, status: 400, code: "invalid_input"},
		{name: "malformed", body: `{`, mediaType: "application/json", keys: []string{"adjustment-key-001"}, status: 400, code: "invalid_input"},
		{name: "wrong JSON type", body: `[]`, mediaType: "application/json", keys: []string{"adjustment-key-001"}, status: 400, code: "invalid_input"},
		{name: "too large", body: strings.Repeat(" ", httpapi.MaxBodyBytes) + validBody, mediaType: "application/json", keys: []string{"adjustment-key-001"}, status: 413, code: "body_too_large"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			service := adjustmentServiceFunc(func(context.Context, string, contracts.AdjustStock) (contracts.Adjustment, bool, error) {
				calls++
				return contracts.Adjustment{}, false, nil
			})
			recorder := serveAdjustment(t, service, "SKU-001", tc.body, tc.mediaType, tc.keys...)
			assertAdjustmentError(t, recorder, tc.status, tc.code)
			if calls != 0 {
				t.Fatalf("invalid transport input reached service %d times", calls)
			}
		})
	}
}

func TestAdjustmentHandlerMapsServiceErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "invalid", err: domain.ErrInvalid, status: 400, code: "invalid_input"},
		{name: "missing SKU", err: domain.ErrNotFound, status: 404, code: "not_found"},
		{name: "key conflict", err: domain.ErrIdempotencyConflict, status: 409, code: "idempotency_conflict"},
		{name: "canceled", err: context.Canceled, status: 503, code: "unavailable"},
		{name: "deadline", err: context.DeadlineExceeded, status: 503, code: "unavailable"},
		{name: "internal", err: errors.New("repository failed"), status: 500, code: "internal_error"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			service := adjustmentServiceFunc(func(context.Context, string, contracts.AdjustStock) (contracts.Adjustment, bool, error) {
				calls++
				return contracts.Adjustment{}, false, tc.err
			})
			recorder := serveAdjustment(t, service, "SKU-001", `{"quantity":1,"reason":"restock"}`, "application/json", "adjustment-key-001")
			assertAdjustmentError(t, recorder, tc.status, tc.code)
			if calls != 1 {
				t.Fatalf("calls = %d, want 1", calls)
			}
		})
	}
}

func assertAdjustmentError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, status, recorder.Body.String())
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != code || body.Error.Message == "" || body.Error.RequestID != adjustmentRequestID {
		t.Fatalf("error body = %+v", body.Error)
	}
}
