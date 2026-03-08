# Transactions Guide

## WithTx

```go
err := orm.WithTx(ctx, db, func(tx *orm.Tx) error {
	u := User{Email: "a@example.com", Name: "Alice"}
	if err := orm.Insert(ctx, tx, &u); err != nil {
		return err
	}
	return nil
})
```

If callback returns error, transaction is rolled back.

## Savepoints

```go
err := orm.WithTx(ctx, db, func(tx *orm.Tx) error {
	return orm.WithSavepoint(tx, "sp1", func() error {
		// nested unit of work
		return nil
	})
})
```

## Best Practices

- Keep transaction scope small.
- Avoid network calls inside transaction blocks.
- Use savepoints for partial rollback in long workflows.
- Test both commit and rollback paths.

## Metrics

Runtime metrics can track tx events via `AttachRuntimeMetrics` (`begin/commit/rollback`).
