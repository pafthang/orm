package orm

import (
	"context"
	"fmt"
	"time"

	"github.com/pafthang/dbx"
)

// UpdateByPK updates selected fields by single/composite PK key.
func UpdateByPK[T any](ctx context.Context, db DB, key any, fields map[string]any) (int64, error) {
	meta, err := Meta[T]()
	if err != nil {
		return 0, err
	}
	if len(fields) == 0 {
		return 0, ErrInvalidQuery.with("update_by_pk", meta.Name, "", fmt.Errorf("no fields to update"))
	}
	where, err := pkWhereFromKey(meta, key)
	if err != nil {
		return 0, err
	}
	where = applyTenantWhere(ctx, meta, where)

	cols := dbx.Params{}
	fieldNames := make([]string, 0, len(fields))
	for name, value := range fields {
		fm, ferr := findFieldMeta(meta, name)
		if ferr != nil {
			return 0, ferr
		}
		if fm.IsPK || fm.IsReadOnly || fm.IsGenerated || fm.IsCreatedAt || fm.IsSoftDelete {
			return 0, ErrInvalidField.with("update_by_pk", meta.Name, name, fmt.Errorf("field is not updatable"))
		}
		enc, eerr := encodeFieldValue(fm, value)
		if eerr != nil {
			return 0, ErrInvalidQuery.with("update_by_pk", meta.Name, name, eerr)
		}
		cols[fm.DBName] = enc
		fieldNames = append(fieldNames, fm.GoName)
	}
	if meta.UpdatedAtField != nil {
		cols[meta.UpdatedAtField.DBName] = time.Now().UTC()
	}

	info := OperationInfo{
		Operation: OpUpdateFields,
		Model:     meta.Name,
		Table:     meta.Table,
		HasWhere:  true,
		Fields:    fieldNames,
	}
	var affected int64
	err = withOperationErr(ctx, info, func(ctx context.Context) error {
		q := db.Update(meta.Table, cols, dbx.HashExp(where))
		if ctx != nil {
			q.WithContext(ctx)
		}
		res, err := q.Execute()
		if err != nil {
			return wrapQueryError(ErrInvalidQuery, "update_by_pk", meta.Name, "", err)
		}
		affected, _ = res.RowsAffected()
		if affected == 0 {
			return ErrNoRowsAffected.with("update_by_pk", meta.Name, "", nil)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return affected, nil
}
