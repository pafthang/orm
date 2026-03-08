package orm

import (
	"context"
	"database/sql"
	"time"

	"github.com/pafthang/dbx"
)

type ormCtxKey string

const (
	ormCtxOperationKey ormCtxKey = "orm.operation"
	ormCtxModelKey     ormCtxKey = "orm.model"
	ormCtxTableKey     ormCtxKey = "orm.table"
	ormCtxInfoKey      ormCtxKey = "orm.info"
)

func withOperationContext(ctx context.Context, info OperationInfo) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithValue(ctx, ormCtxOperationKey, info.Operation)
	ctx = context.WithValue(ctx, ormCtxModelKey, info.Model)
	ctx = context.WithValue(ctx, ormCtxTableKey, info.Table)
	ctx = context.WithValue(ctx, ormCtxInfoKey, info)
	return ctx
}

func operationInfoFromContext(ctx context.Context) OperationInfo {
	if ctx == nil {
		return OperationInfo{}
	}
	if v := ctx.Value(ormCtxInfoKey); v != nil {
		if info, ok := v.(OperationInfo); ok {
			return info
		}
	}
	info := OperationInfo{}
	if v := ctx.Value(ormCtxOperationKey); v != nil {
		if op, ok := v.(Operation); ok {
			info.Operation = op
		}
	}
	if v := ctx.Value(ormCtxModelKey); v != nil {
		if model, ok := v.(string); ok {
			info.Model = model
		}
	}
	if v := ctx.Value(ormCtxTableKey); v != nil {
		if table, ok := v.(string); ok {
			info.Table = table
		}
	}
	return info
}

// QueryEvent is a single DB query execution event.
type QueryEvent struct {
	Kind         string
	Operation    Operation
	Model        string
	Table        string
	SQL          string
	Duration     time.Duration
	RowsAffected int64
	Err          error
}

// DBXObserverOptions configures DB-level ORM-aware observability hooks.
type DBXObserverOptions struct {
	RedactSQL func(sql string) string
	OnEvent   func(QueryEvent)
}

// AttachDBXObserver installs ORM-aware hooks on a dbx DB instance.
// Existing dbx log hooks are preserved and called before observer hooks.
func AttachDBXObserver(db *dbx.DB, opts DBXObserverOptions) {
	if db == nil || opts.OnEvent == nil {
		return
	}

	prevExec := db.ExecLogFunc
	prevQuery := db.QueryLogFunc

	db.ExecLogFunc = func(ctx context.Context, d time.Duration, sqlText string, result sql.Result, err error) {
		if prevExec != nil {
			prevExec(ctx, d, sqlText, result, err)
		}
		rows := int64(-1)
		if result != nil {
			if n, e := result.RowsAffected(); e == nil {
				rows = n
			}
		}
		info := operationInfoFromContext(ctx)
		safeSQL := sqlText
		if opts.RedactSQL != nil {
			safeSQL = opts.RedactSQL(sqlText)
		}
		opts.OnEvent(QueryEvent{
			Kind:         "exec",
			Operation:    info.Operation,
			Model:        info.Model,
			Table:        info.Table,
			SQL:          safeSQL,
			Duration:     d,
			RowsAffected: rows,
			Err:          err,
		})
	}

	db.QueryLogFunc = func(ctx context.Context, d time.Duration, sqlText string, rows *sql.Rows, err error) {
		if prevQuery != nil {
			prevQuery(ctx, d, sqlText, rows, err)
		}
		info := operationInfoFromContext(ctx)
		safeSQL := sqlText
		if opts.RedactSQL != nil {
			safeSQL = opts.RedactSQL(sqlText)
		}
		opts.OnEvent(QueryEvent{
			Kind:         "query",
			Operation:    info.Operation,
			Model:        info.Model,
			Table:        info.Table,
			SQL:          safeSQL,
			Duration:     d,
			RowsAffected: -1,
			Err:          err,
		})
	}
}
