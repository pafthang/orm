package orm

import (
	"context"
	"errors"
	"testing"
)

func TestWithTxCommit(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	ctx := context.Background()

	err := WithTx(ctx, db, func(tx *Tx) error {
		u := crudUser{Email: "tx@example.com", Name: "Tx"}
		return Insert(ctx, tx, &u)
	})
	if err != nil {
		t.Fatalf("with tx commit: %v", err)
	}

	n, err := Count[crudUser](ctx, db)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row after commit, got %d", n)
	}
}

func TestWithTxRollbackOnError(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	ctx := context.Background()

	err := WithTx(ctx, db, func(tx *Tx) error {
		u := crudUser{Email: "tx2@example.com", Name: "Tx2"}
		if err := Insert(ctx, tx, &u); err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatalf("expected rollback error")
	}

	n, err := Count[crudUser](ctx, db)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 rows after rollback, got %d", n)
	}
}

func TestWithSavepointRollbackInnerOnly(t *testing.T) {
	withFreshRegistry(t)
	db := setupSQLiteDB(t)
	ctx := context.Background()

	err := WithTx(ctx, db, func(tx *Tx) error {
		u1 := crudUser{Email: "sp1@example.com", Name: "SP1"}
		if err := Insert(ctx, tx, &u1); err != nil {
			return err
		}

		_ = WithSavepoint(tx, "inner1", func() error {
			u2 := crudUser{Email: "sp2@example.com", Name: "SP2"}
			if err := Insert(ctx, tx, &u2); err != nil {
				return err
			}
			return errors.New("force rollback to savepoint")
		})

		u3 := crudUser{Email: "sp3@example.com", Name: "SP3"}
		if err := Insert(ctx, tx, &u3); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("with tx/savepoint: %v", err)
	}

	n, err := Count[crudUser](ctx, db)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 persisted rows, got %d", n)
	}
}
