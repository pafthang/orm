# Integration With dbx

## DB Contract

`orm.DB` is a thin interface over `dbx` query primitives.

Compatible executors:
- `*dbx.DB`
- `*dbx.Tx`

## Why This Design

- ORM remains typed and metadata-driven.
- Low-level SQL control remains available through `dbx`.
- Transaction and query lifecycle are shared.

## Observability Bridge

Adapters:
- `AttachDBXObserver`
- `AttachMetricsObserver`
- `AttachRuntimeMetrics`

This gives unified operation context (`operation/model/table`) on top of `dbx` logs.

## Escape Hatches

Use ORM unsafe methods (`UnsafeWhereExpr`, etc.) when needed.
For very custom SQL flows, call `dbx` directly.

## Migration Strategy From Plain dbx

A common path:
1. keep existing `dbx` repositories untouched
2. move simple CRUD entities to `orm`
3. keep complex analytical queries in direct `dbx`
4. standardize errors and observability across both layers
