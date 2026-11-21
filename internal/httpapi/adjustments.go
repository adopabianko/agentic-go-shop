package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"

	"example.com/agentic-go-shop/internal/contracts"
	"example.com/agentic-go-shop/internal/domain"
)

type adjustmentInput struct {
	Quantity int    `json:"quantity"`
	Reason   string `json:"reason"`
}

// AdjustmentHandler translates a positive inventory adjustment request into
// the frozen adjustment use-case contract. Routing, method checks, and request
// observation are applied by the composition task when this handler is mounted.
func AdjustmentHandler(service contracts.AdjustmentUseCase, logger *slog.Logger) http.HandlerFunc {
	h := &Handler{logger: logger}
	return func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			fail(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var body adjustmentInput
		if err := decoder.Decode(&body); err != nil {
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
			fail(w, http.StatusBadRequest, "invalid_input", "one Idempotency-Key is required")
			return
		}

		receipt, replayed, err := service.Adjust(r.Context(), keys[0], contracts.AdjustStock{
			SKU:      r.PathValue("sku"),
			Quantity: body.Quantity,
			Reason:   body.Reason,
		})
		if err != nil {
			h.serviceError(w, err)
			return
		}

		w.Header().Set("Idempotency-Replayed", strconv.FormatBool(replayed))
		writeJSON(w, http.StatusCreated, receipt)
	}
}
