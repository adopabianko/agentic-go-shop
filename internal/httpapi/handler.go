// Package httpapi translates HTTP into application calls; it owns no business rules.
package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"time"

	"example.com/agentic-go-shop/internal/contracts"
	"example.com/agentic-go-shop/internal/domain"
	"example.com/agentic-go-shop/internal/inventory"
	"example.com/agentic-go-shop/internal/orders"
	"example.com/agentic-go-shop/internal/ports"
)

const MaxBodyBytes = 16 * 1024

type Handler struct {
	orders *orders.Service
	store  ports.Store
	logger *slog.Logger
}

func New(service *orders.Service, store ports.Store, logger *slog.Logger) http.Handler {
	return newHandler(service, store, nil, nil, logger)
}

// NewWithFeatures builds the active v1.1 router while New remains available to
// baseline callers that only compose the original order and inventory routes.
func NewWithFeatures(service *orders.Service, store ports.Store, adjustments contracts.AdjustmentUseCase, orderList contracts.OrderListUseCase, logger *slog.Logger) http.Handler {
	return newHandler(service, store, adjustments, orderList, logger)
}

func newHandler(service *orders.Service, store ports.Store, adjustments contracts.AdjustmentUseCase, orderList contracts.OrderListUseCase, logger *slog.Logger) http.Handler {
	h := &Handler{orders: service, store: store, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.only(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}))
	mux.HandleFunc("/v1/inventory/{sku}", h.only(http.MethodGet, h.getStock))
	if orderList == nil {
		mux.HandleFunc("/v1/orders", h.only(http.MethodPost, h.createOrder))
	} else {
		mux.HandleFunc("/v1/orders", h.ordersEndpoint(OrderListHandler(orderList, logger)))
	}
	mux.HandleFunc("/v1/orders/{id}", h.only(http.MethodGet, h.getOrder))
	mux.HandleFunc("/v1/orders/{id}/cancel", h.only(http.MethodPost, h.cancelOrder))
	if adjustments != nil {
		mux.HandleFunc("/v1/inventory/{sku}/adjustments", h.only(http.MethodPost, AdjustmentHandler(adjustments, logger)))
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "not_found", "route not found") })
	return h.observe(mux)
}

func (h *Handler) ordersEndpoint(list http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			list(w, r)
		case http.MethodPost:
			h.createOrder(w, r)
		default:
			w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
			fail(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		}
	}
}

func (h *Handler) only(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.Header().Set("Allow", method)
			fail(w, 405, "method_not_allowed", "method not allowed")
			return
		}
		next(w, r)
	}
}

func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		fail(w, 415, "unsupported_media_type", "Content-Type must be application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input domain.CreateOrder
	if err := decoder.Decode(&input); err != nil {
		h.decodeError(w, err)
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			err = domain.ErrInvalid
		}
		h.decodeError(w, err)
		return
	}
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 {
		fail(w, 400, "invalid_input", "one Idempotency-Key is required")
		return
	}
	order, replayed, err := h.orders.Create(r.Context(), keys[0], input)
	if err != nil {
		h.serviceError(w, err)
		return
	}
	w.Header().Set("Location", "/v1/orders/"+order.ID)
	if replayed {
		w.Header().Set("Idempotency-Replayed", "true")
	} else {
		w.Header().Set("Idempotency-Replayed", "false")
	}
	writeJSON(w, http.StatusCreated, order)
}

func (h *Handler) decodeError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		fail(w, 413, "body_too_large", "request body exceeds 16384 bytes")
		return
	}
	fail(w, 400, "invalid_input", "body must be one valid JSON object matching the contract")
}

func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request) {
	order, err := h.orders.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		h.serviceError(w, err)
		return
	}
	writeJSON(w, 200, order)
}

func (h *Handler) cancelOrder(w http.ResponseWriter, r *http.Request) {
	// This operation has no request body in the v1 contract.
	b, err := io.ReadAll(io.LimitReader(r.Body, 1))
	if err != nil || len(b) != 0 {
		fail(w, 400, "invalid_input", "cancel does not accept a body")
		return
	}
	order, err := h.orders.Cancel(r.Context(), r.PathValue("id"))
	if err != nil {
		h.serviceError(w, err)
		return
	}
	writeJSON(w, 200, order)
}

func (h *Handler) getStock(w http.ResponseWriter, r *http.Request) {
	stock, err := inventory.Get(r.Context(), h.store, r.PathValue("sku"))
	if err != nil {
		h.serviceError(w, err)
		return
	}
	writeJSON(w, 200, stock)
}

func (h *Handler) serviceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		fail(w, 400, "invalid_input", "input does not satisfy the contract")
	case errors.Is(err, domain.ErrNotFound):
		fail(w, 404, "not_found", "resource not found")
	case errors.Is(err, domain.ErrInsufficientStock):
		fail(w, 409, "insufficient_stock", "not enough stock")
	case errors.Is(err, domain.ErrIdempotencyConflict):
		fail(w, 409, "idempotency_conflict", "key already used with a different request")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		fail(w, 503, "unavailable", "request interrupted; retry with the same key")
	default:
		h.logger.Error("application failure", "request_id", w.Header().Get("X-Request-ID"), "error", err)
		fail(w, 500, "internal_error", "internal error")
	}
}

func fail(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message, "request_id": w.Header().Get("X-Request-ID")}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value) // The response cannot be replaced after a network write fails.
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *responseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}

func (h *Handler) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var id [16]byte
		_, _ = rand.Read(id[:])
		w.Header().Set("X-Request-ID", hex.EncodeToString(id[:]))
		w.Header().Set("Cache-Control", "no-store")
		rw := &responseWriter{ResponseWriter: w}
		start := time.Now()
		defer func() {
			if recovered := recover(); recovered != nil {
				h.logger.Error("request panic", "request_id", w.Header().Get("X-Request-ID"))
				if rw.status == 0 {
					fail(rw, 500, "internal_error", "internal error")
				}
			}
			h.logger.Info("http request", "request_id", w.Header().Get("X-Request-ID"), "method", r.Method, "route", r.Pattern, "status", rw.status, "duration_ms", time.Since(start).Milliseconds())
		}()
		next.ServeHTTP(rw, r)
	})
}
