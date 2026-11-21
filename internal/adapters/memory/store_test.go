package memory_test

import (
	"example.com/agentic-go-shop/internal/adapters/memory"
	"example.com/agentic-go-shop/internal/domain"
	"example.com/agentic-go-shop/internal/ports"
	"example.com/agentic-go-shop/internal/ports/storetest"
	"testing"
)

func TestStoreContract(t *testing.T) {
	storetest.Run(t, func(t *testing.T) ports.Store {
		t.Helper()
		s, err := memory.New(nil)
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}

func TestRejectInvalidSeed(t *testing.T) {
	for _, seed := range [][]domain.Stock{
		{{SKU: "x", Available: -1}}, {{SKU: "x", Available: domain.MaxQuantity + 1}},
		{{SKU: "x", Reserved: 1}}, {{SKU: "bad id"}}, {{SKU: "x"}, {SKU: "x"}},
	} {
		if _, err := memory.New(seed); err == nil {
			t.Fatalf("accepted %+v", seed)
		}
	}
}
