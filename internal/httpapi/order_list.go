package httpapi

import (
	"log/slog"
	"net/http"
	"net/url"
	"strconv"

	"example.com/agentic-go-shop/internal/contracts"
	"example.com/agentic-go-shop/internal/domain"
)

// OrderListHandler translates the order-list HTTP query into the frozen use-case
// contract. Method routing and the normal HTTP middleware are composed by T09.
func OrderListHandler(service contracts.OrderListUseCase, logger *slog.Logger) http.HandlerFunc {
	h := &Handler{logger: logger}
	return func(w http.ResponseWriter, r *http.Request) {
		query, err := parseOrderListQuery(r.URL.RawQuery)
		if err != nil {
			fail(w, http.StatusBadRequest, "invalid_input", "input does not satisfy the contract")
			return
		}

		page, err := service.List(r.Context(), query)
		if err != nil {
			h.serviceError(w, err)
			return
		}
		if page.Items == nil {
			page.Items = []domain.Order{}
		}
		writeJSON(w, http.StatusOK, page)
	}
}

func parseOrderListQuery(raw string) (contracts.OrderQuery, error) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return contracts.OrderQuery{}, domain.ErrInvalid
	}
	for name, entries := range values {
		switch name {
		case "customer_id", "limit", "cursor":
		default:
			return contracts.OrderQuery{}, domain.ErrInvalid
		}
		if len(entries) != 1 || entries[0] == "" {
			return contracts.OrderQuery{}, domain.ErrInvalid
		}
	}

	query := contracts.OrderQuery{
		CustomerID: values.Get("customer_id"),
		Cursor:     values.Get("cursor"),
	}
	if text := values.Get("limit"); text != "" {
		limit, err := strconv.Atoi(text)
		if err != nil || limit < 1 || limit > 100 {
			return contracts.OrderQuery{}, domain.ErrInvalid
		}
		query.Limit = limit
	}
	return query, nil
}
