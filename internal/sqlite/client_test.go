package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func TestTransactionsPersistAndRollbackAcrossConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "insight.db")
	a, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 60)
	for i := 0; i < 60; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			client := a
			if i%2 == 0 {
				client = b
			}
			errs <- client.update(ctx, func(tx *sql.Tx) error {
				count, _, err := get[int](ctx, tx, "counter", "workspace", "value")
				if err != nil {
					return err
				}
				return put(ctx, tx, "counter", "workspace", "value", count+1)
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	abort := errors.New("abort")
	if err := a.update(ctx, func(tx *sql.Tx) error {
		if err := put(ctx, tx, "counter", "workspace", "value", 0); err != nil {
			return err
		}
		return abort
	}); !errors.Is(err, abort) {
		t.Fatalf("rollback error: %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	a, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	count, found, err := get[int](ctx, a.db, "counter", "workspace", "value")
	if err != nil || !found || count != 60 {
		t.Fatal(fmt.Sprintf("persisted count=%d found=%v err=%v", count, found, err))
	}
	_, otherFound, err := get[int](ctx, a.db, "counter", "other-workspace", "value")
	if err != nil || otherFound {
		t.Fatal("workspace isolation failed")
	}
}
