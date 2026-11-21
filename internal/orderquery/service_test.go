package orderquery_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"example.com/agentic-go-shop/internal/contracts"
	"example.com/agentic-go-shop/internal/domain"
	"example.com/agentic-go-shop/internal/orderquery"
)

type fakeReader struct {
	page  contracts.OrderPage
	err   error
	query contracts.OrderQuery
	calls int
}

func (f *fakeReader) ReadOrders(_ context.Context, query contracts.OrderQuery) (contracts.OrderPage, error) {
	f.calls++
	f.query = query
	return f.page, f.err
}

func TestListNormalizesDefaultLimitAndDelegates(t *testing.T) {
	wantPage := contracts.OrderPage{
		Items: []domain.Order{
			{ID: "ord-2", CustomerID: "customer-1", CreatedAt: time.Unix(2, 0)},
			{ID: "ord-1", CustomerID: "another-customer", CreatedAt: time.Unix(1, 0)},
		},
		NextCursor: "opaque-cursor",
	}
	reader := &fakeReader{page: wantPage}
	service := orderquery.New(reader)

	got, err := service.List(context.Background(), contracts.OrderQuery{
		CustomerID: "customer-1",
		Cursor:     "cursor-from-client",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reader.calls != 1 {
		t.Fatalf("reader calls = %d, want 1", reader.calls)
	}
	wantQuery := contracts.OrderQuery{
		CustomerID: "customer-1",
		Limit:      50,
		Cursor:     "cursor-from-client",
	}
	if reader.query != wantQuery {
		t.Fatalf("reader query = %+v, want %+v", reader.query, wantQuery)
	}
	if !reflect.DeepEqual(got, wantPage) {
		t.Fatalf("page = %+v, want unmodified page %+v", got, wantPage)
	}
}

func TestListAcceptsValidBoundsAndAllCustomers(t *testing.T) {
	reader := &fakeReader{}
	service := orderquery.New(reader)

	query := contracts.OrderQuery{Limit: 100, Cursor: strings.Repeat("x", 2048)}
	if _, err := service.List(context.Background(), query); err != nil {
		t.Fatal(err)
	}
	if reader.calls != 1 || reader.query != query {
		t.Fatalf("reader calls = %d, query = %+v", reader.calls, reader.query)
	}
}

func TestListRejectsInvalidQueryBeforeReading(t *testing.T) {
	tests := []struct {
		name  string
		query contracts.OrderQuery
	}{
		{name: "invalid customer", query: contracts.OrderQuery{CustomerID: "bad customer"}},
		{name: "negative limit", query: contracts.OrderQuery{Limit: -1}},
		{name: "limit too large", query: contracts.OrderQuery{Limit: 101}},
		{name: "cursor too large", query: contracts.OrderQuery{Cursor: strings.Repeat("x", 2049)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeReader{}
			_, err := orderquery.New(reader).List(context.Background(), tt.query)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
			if reader.calls != 0 {
				t.Fatalf("reader called %d times", reader.calls)
			}
		})
	}
}

func TestListPropagatesReaderError(t *testing.T) {
	wantErr := errors.New("reader failed")
	reader := &fakeReader{err: wantErr}

	page, err := orderquery.New(reader).List(context.Background(), contracts.OrderQuery{Limit: 1})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if !reflect.DeepEqual(page, contracts.OrderPage{}) {
		t.Fatalf("page = %+v, want zero page", page)
	}
	if reader.calls != 1 {
		t.Fatalf("reader calls = %d, want 1", reader.calls)
	}
}
