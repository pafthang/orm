package orm

import (
	"sync"
	"time"

	"github.com/pafthang/dbx"
)

// MetricsObserverOptions defines callbacks for high-level query metrics.
type MetricsObserverOptions struct {
	OnQueryCount   func(kind string, op Operation, model, table string)
	OnError        func(kind string, op Operation, model, table string, err error)
	OnLatency      func(kind string, op Operation, model, table string, d time.Duration)
	OnRowsAffected func(op Operation, model, table string, rows int64)
	RedactSQL      func(sql string) string
}

// RuntimeMetricsOptions defines callbacks emitted by ORM runtime paths.
type RuntimeMetricsOptions struct {
	OnRowsScanned func(op Operation, model, table string, rows int64)
	OnTxCount     func(event string)
}

var runtimeMetricsState struct {
	mu   sync.RWMutex
	opts RuntimeMetricsOptions
}

// AttachMetricsObserver attaches metric callbacks using ORM-aware query events.
func AttachMetricsObserver(db *dbx.DB, opts MetricsObserverOptions) {
	if db == nil {
		return
	}
	AttachDBXObserver(db, DBXObserverOptions{
		RedactSQL: opts.RedactSQL,
		OnEvent: func(e QueryEvent) {
			if opts.OnQueryCount != nil {
				opts.OnQueryCount(e.Kind, e.Operation, e.Model, e.Table)
			}
			if opts.OnLatency != nil {
				opts.OnLatency(e.Kind, e.Operation, e.Model, e.Table, e.Duration)
			}
			if e.Err != nil && opts.OnError != nil {
				opts.OnError(e.Kind, e.Operation, e.Model, e.Table, e.Err)
			}
			if e.Kind == "exec" && e.RowsAffected >= 0 && opts.OnRowsAffected != nil {
				opts.OnRowsAffected(e.Operation, e.Model, e.Table, e.RowsAffected)
			}
		},
	})
}

// AttachRuntimeMetrics configures callbacks for runtime-level metrics.
func AttachRuntimeMetrics(opts RuntimeMetricsOptions) {
	runtimeMetricsState.mu.Lock()
	runtimeMetricsState.opts = opts
	runtimeMetricsState.mu.Unlock()
}

// ResetRuntimeMetrics clears runtime-level metric callbacks.
func ResetRuntimeMetrics() {
	runtimeMetricsState.mu.Lock()
	runtimeMetricsState.opts = RuntimeMetricsOptions{}
	runtimeMetricsState.mu.Unlock()
}

func emitRowsScanned(op Operation, model, table string, rows int64) {
	runtimeMetricsState.mu.RLock()
	fn := runtimeMetricsState.opts.OnRowsScanned
	runtimeMetricsState.mu.RUnlock()
	if fn != nil {
		fn(op, model, table, rows)
	}
}

func emitTxCount(event string) {
	runtimeMetricsState.mu.RLock()
	fn := runtimeMetricsState.opts.OnTxCount
	runtimeMetricsState.mu.RUnlock()
	if fn != nil {
		fn(event)
	}
}
