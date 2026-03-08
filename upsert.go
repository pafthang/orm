package orm

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/pafthang/dbx"
)

// UpsertOptions controls conflict/update behavior for Upsert.
type UpsertOptions struct {
	ConflictFields []string
	UpdateFields   []string
	DoNothing      bool
}

// Upsert inserts or updates a row based on conflict target.
func Upsert[T any](ctx context.Context, db DB, model *T, opts ...UpsertOptions) error {
	meta, rv, err := modelMetaAndValue(model)
	if err != nil {
		return err
	}
	cfg := UpsertOptions{}
	if len(opts) > 0 {
		cfg = opts[0]
	}

	now := time.Now().UTC()
	if meta.CreatedAtField != nil {
		setNowIfZero(rv.FieldByIndex(meta.CreatedAtField.Index), now)
	}
	if meta.UpdatedAtField != nil {
		setTimeValue(rv.FieldByIndex(meta.UpdatedAtField.Index), now)
	}

	insertCols, _, err := insertColumns(meta, rv)
	if err != nil {
		return err
	}

	conflictCols, err := resolveConflictColumns(meta, cfg.ConflictFields)
	if err != nil {
		return err
	}
	if len(conflictCols) == 0 {
		return ErrInvalidQuery.with("upsert", meta.Name, "", fmt.Errorf("no conflict columns"))
	}

	var conflict dbx.OnConflict
	if cfg.DoNothing {
		conflict = dbx.Conflict(conflictCols...).DoNothing()
	} else {
		updateSet, err := buildUpsertUpdateSet(meta, rv, cfg.UpdateFields)
		if err != nil {
			return err
		}
		if len(updateSet) == 0 {
			return ErrInvalidQuery.with("upsert", meta.Name, "", fmt.Errorf("empty update set"))
		}
		conflict = dbx.Conflict(conflictCols...).DoUpdateSet(updateSet)
	}

	info := OperationInfo{
		Operation:      OpInsert,
		Model:          meta.Name,
		Table:          meta.Table,
		ConflictFields: append([]string(nil), conflictCols...),
	}
	return withOperationErr(ctx, info, func(ctx context.Context) error {
		q := db.UpsertOnConflict(meta.Table, insertCols, conflict)
		if ctx != nil {
			q.WithContext(ctx)
		}
		_, err := q.Execute()
		if err != nil {
			return wrapQueryError(ErrInvalidQuery, "upsert", meta.Name, "", err)
		}
		return nil
	})
}

func resolveConflictColumns(meta *ModelMeta, fields []string) ([]string, error) {
	if len(fields) == 0 {
		if len(meta.PrimaryKeys) == 0 {
			return nil, ErrMissingPrimaryKey.with("upsert", meta.Name, "", nil)
		}
		cols := make([]string, 0, len(meta.PrimaryKeys))
		for _, pk := range meta.PrimaryKeys {
			cols = append(cols, pk.DBName)
		}
		return cols, nil
	}
	cols := make([]string, 0, len(fields))
	for _, f := range fields {
		fm, err := findFieldMeta(meta, f)
		if err != nil {
			return nil, err
		}
		cols = append(cols, fm.DBName)
	}
	return cols, nil
}

func buildUpsertUpdateSet(meta *ModelMeta, rv reflect.Value, fields []string) (dbx.Params, error) {
	set := dbx.Params{}
	if len(fields) > 0 {
		for _, f := range fields {
			fm, err := findFieldMeta(meta, f)
			if err != nil {
				return nil, err
			}
			if !isUpsertUpdatable(fm) {
				return nil, ErrInvalidField.with("upsert", meta.Name, f, fmt.Errorf("field is not updatable on conflict"))
			}
			val, err := encodeFieldValue(fm, rv.FieldByIndex(fm.Index).Interface())
			if err != nil {
				return nil, ErrInvalidQuery.with("upsert", meta.Name, fm.GoName, err)
			}
			set[fm.DBName] = val
		}
	} else {
		for _, fm := range meta.Fields {
			if !isUpsertUpdatable(fm) {
				continue
			}
			val, err := encodeFieldValue(fm, rv.FieldByIndex(fm.Index).Interface())
			if err != nil {
				return nil, ErrInvalidQuery.with("upsert", meta.Name, fm.GoName, err)
			}
			set[fm.DBName] = val
		}
	}
	if meta.UpdatedAtField != nil {
		set[meta.UpdatedAtField.DBName] = rv.FieldByIndex(meta.UpdatedAtField.Index).Interface()
	}
	return set, nil
}

func isUpsertUpdatable(f *FieldMeta) bool {
	if f == nil {
		return false
	}
	if f.IsPK || f.IsReadOnly || f.IsGenerated || f.IsCreatedAt || f.IsSoftDelete || f.IsIgnored {
		return false
	}
	return true
}
