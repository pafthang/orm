package orm

import (
	"context"
	"fmt"
	"time"

	"github.com/pafthang/dbx"
)

// InsertBatch inserts multiple models in a single multi-row statement.
// Returns affected rows when available.
func InsertBatch[T any](ctx context.Context, db DB, models []*T) (int64, error) {
	if len(models) == 0 {
		return 0, ErrInvalidQuery.with("insert_batch", "", "", fmt.Errorf("models batch is empty"))
	}

	meta, _, err := modelMetaAndValue(models[0])
	if err != nil {
		return 0, err
	}
	rows := make([]dbx.Params, 0, len(models))

	for i, m := range models {
		if m == nil {
			return 0, ErrInvalidModel.with("insert_batch", meta.Name, "", fmt.Errorf("model at index %d is nil", i))
		}
		_, rv, err := modelMetaAndValue(m)
		if err != nil {
			return 0, err
		}
		now := time.Now().UTC()
		if meta.CreatedAtField != nil {
			setNowIfZero(rv.FieldByIndex(meta.CreatedAtField.Index), now)
		}
		if meta.UpdatedAtField != nil {
			setTimeValue(rv.FieldByIndex(meta.UpdatedAtField.Index), now)
		}
		cols, _, err := insertColumns(meta, rv)
		if err != nil {
			return 0, err
		}
		rows = append(rows, cols)
	}

	info := OperationInfo{Operation: OpInsert, Model: meta.Name, Table: meta.Table}
	var affected int64
	err = withOperationErr(ctx, info, func(ctx context.Context) error {
		q := db.InsertMany(meta.Table, rows)
		if ctx != nil {
			q.WithContext(ctx)
		}
		res, err := q.Execute()
		if err != nil {
			return wrapQueryError(ErrInvalidQuery, "insert_batch", meta.Name, "", err)
		}
		affected, _ = res.RowsAffected()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return affected, nil
}
