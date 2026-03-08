package orm

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pafthang/dbx"
)

// Tx is an alias for dbx transaction wrapper.
type Tx = dbx.Tx

// WithTx runs fn inside a transaction.
// When fn returns error, transaction is rolled back.
func WithTx(ctx context.Context, db *dbx.DB, fn func(tx *Tx) error) error {
	if db == nil {
		return ErrInvalidQuery.with("with_tx", "", "", fmt.Errorf("db is nil"))
	}
	if fn == nil {
		return ErrInvalidQuery.with("with_tx", "", "", fmt.Errorf("fn is nil"))
	}
	emitTxCount("begin")
	wrapped := func(tx *Tx) error {
		return fn(tx)
	}
	var err error
	if ctx == nil {
		err = db.Transactional(wrapped)
	} else {
		err = db.TransactionalContext(ctx, &sql.TxOptions{}, wrapped)
	}
	if err != nil {
		emitTxCount("rollback")
		return err
	}
	emitTxCount("commit")
	return nil
}

// WithSavepoint runs fn inside a named savepoint.
// On fn error, it rolls back to savepoint and releases it.
func WithSavepoint(tx *Tx, name string, fn func() error) error {
	if tx == nil {
		return ErrInvalidQuery.with("with_savepoint", "", "", fmt.Errorf("tx is nil"))
	}
	if fn == nil {
		return ErrInvalidQuery.with("with_savepoint", "", "", fmt.Errorf("fn is nil"))
	}
	if err := tx.Savepoint(name); err != nil {
		return ErrInvalidQuery.with("with_savepoint", "", "", err)
	}
	if err := fn(); err != nil {
		_ = tx.RollbackTo(name)
		_ = tx.Release(name)
		return err
	}
	if err := tx.Release(name); err != nil {
		return ErrInvalidQuery.with("with_savepoint", "", "", err)
	}
	return nil
}
