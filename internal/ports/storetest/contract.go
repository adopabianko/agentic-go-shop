// Package storetest contains the reusable contract every storage adapter must pass.
package storetest

import (
	"context"
	"errors"
	"sync"
	"testing"

	"example.com/agentic-go-shop/internal/domain"
	"example.com/agentic-go-shop/internal/ports"
)

type Factory func(t *testing.T) ports.Store

func Run(t *testing.T, factory Factory) {
	t.Helper()
	t.Run("rollback_all_writes", func(t *testing.T) {
		s := factory(t)
		failure := errors.New("abort")
		err := s.WithinTx(context.Background(), func(tx ports.Tx) error {
			tx.PutStock(domain.Stock{SKU: "x", Available: 9})
			tx.PutOrder(domain.Order{ID: "o", Items: []domain.Item{{SKU: "x", Quantity: 1}}})
			tx.PutRequest("key", ports.RequestRecord{Fingerprint: "f"})
			return failure
		})
		if !errors.Is(err, failure) {
			t.Fatalf("got %v", err)
		}
		check(t, s, func(tx ports.Tx) {
			if _, err := tx.Stock("x"); !errors.Is(err, domain.ErrNotFound) {
				t.Fatal("stock escaped rollback")
			}
			if _, err := tx.Order("o"); !errors.Is(err, domain.ErrNotFound) {
				t.Fatal("order escaped rollback")
			}
			if _, ok := tx.Request("key"); ok {
				t.Fatal("request escaped rollback")
			}
		})
	})
	t.Run("copy_isolation", func(t *testing.T) {
		s := factory(t)
		value := domain.Order{ID: "o", Items: []domain.Item{{SKU: "x", Quantity: 2}}}
		check(t, s, func(tx ports.Tx) {
			tx.PutOrder(value)
			tx.PutRequest("key", ports.RequestRecord{Response: value})
			value.Items[0].Quantity = 90
		})
		var held domain.Order
		check(t, s, func(tx ports.Tx) {
			o, err := tx.Order("o")
			if err != nil {
				t.Fatal(err)
			}
			if o.Items[0].Quantity != 2 {
				t.Fatal("setter leaked alias")
			}
			held = o
			o.Items[0].Quantity = 50
			r, _ := tx.Request("key")
			if r.Response.Items[0].Quantity != 2 {
				t.Fatal("request setter leaked alias")
			}
			r.Response.Items[0].Quantity = 30
		})
		held.Items[0].Quantity = 70
		check(t, s, func(tx ports.Tx) {
			o, _ := tx.Order("o")
			r, _ := tx.Request("key")
			if o.Items[0].Quantity != 2 || r.Response.Items[0].Quantity != 2 {
				t.Fatal("getter leaked alias")
			}
		})
	})
	t.Run("cancel_before_and_during_transaction", func(t *testing.T) {
		s := factory(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		called := false
		err := s.WithinTx(ctx, func(tx ports.Tx) error { called = true; return nil })
		if called || !errors.Is(err, context.Canceled) {
			t.Fatalf("called=%v err=%v", called, err)
		}
		ctx, cancel = context.WithCancel(context.Background())
		defer cancel()
		err = s.WithinTx(ctx, func(tx ports.Tx) error { tx.PutStock(domain.Stock{SKU: "x", Available: 9}); cancel(); return nil })
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		check(t, s, func(tx ports.Tx) {
			if _, err := tx.Stock("x"); !errors.Is(err, domain.ErrNotFound) {
				t.Fatal("cancel committed")
			}
		})
	})
	t.Run("panic_rolls_back_and_unlocks", func(t *testing.T) {
		s := factory(t)
		func() {
			defer func() {
				if recover() == nil {
					t.Error("expected panic")
				}
			}()
			_ = s.WithinTx(context.Background(), func(tx ports.Tx) error { tx.PutStock(domain.Stock{SKU: "x"}); panic("abort") })
		}()
		check(t, s, func(tx ports.Tx) {
			if _, err := tx.Stock("x"); !errors.Is(err, domain.ErrNotFound) {
				t.Fatal("panic committed")
			}
		})
	})
	t.Run("serializable_updates", func(t *testing.T) {
		s := factory(t)
		check(t, s, func(tx ports.Tx) { tx.PutStock(domain.Stock{SKU: "x"}) })
		const n = 50
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := s.WithinTx(context.Background(), func(tx ports.Tx) error {
					v, err := tx.Stock("x")
					if err != nil {
						return err
					}
					v.Available++
					tx.PutStock(v)
					return nil
				})
				if err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		check(t, s, func(tx ports.Tx) {
			v, _ := tx.Stock("x")
			if v.Available != n {
				t.Fatalf("lost updates: %d", v.Available)
			}
		})
	})
}

func check(t *testing.T, s ports.Store, fn func(ports.Tx)) {
	t.Helper()
	if err := s.WithinTx(context.Background(), func(tx ports.Tx) error { fn(tx); return nil }); err != nil {
		t.Fatal(err)
	}
}
