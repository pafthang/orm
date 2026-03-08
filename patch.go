package orm

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/pafthang/dbx"
	"github.com/pafthang/orm/optional"
)

// Optional represents patch field with explicit presence marker.
type Optional[T any] = optional.Value[T]
type Nullable[T any] = optional.Nullable[T]

// Some creates Optional with explicit value.
func Some[T any](v T) Optional[T] { return optional.Some(v) }

// None creates Optional without value.
func None[T any]() Optional[T]            { return optional.None[T]() }
func SomeNullable[T any](v T) Nullable[T] { return optional.SomeNullable(v) }
func Null[T any]() Nullable[T]            { return optional.Null[T]() }
func Unset[T any]() Nullable[T]           { return optional.Unset[T]() }

type optionalValue interface {
	IsSet() bool
	ValueAny() any
}

type nullableOptionalValue interface {
	optionalValue
	IsNull() bool
}

// UpdatePatchByPK applies partial update by composite/single PK using Optional fields from patch struct.
func UpdatePatchByPK[T any, P any](ctx context.Context, db DB, key any, patch *P) error {
	if patch == nil {
		return ErrInvalidQuery.with("update_patch", "", "", fmt.Errorf("patch is nil"))
	}
	meta, err := Meta[T]()
	if err != nil {
		return err
	}
	where, err := pkWhereFromKey(meta, key)
	if err != nil {
		return err
	}
	cols, err := extractPatchColumns(meta, patch)
	if err != nil {
		return err
	}
	if len(cols) == 0 {
		return ErrInvalidQuery.with("update_patch", meta.Name, "", fmt.Errorf("patch has no set fields"))
	}
	if meta.UpdatedAtField != nil {
		if _, ok := cols[meta.UpdatedAtField.DBName]; !ok {
			cols[meta.UpdatedAtField.DBName] = time.Now().UTC()
		}
	}

	info := OperationInfo{Operation: OpUpdateFields, Model: meta.Name, Table: meta.Table, HasWhere: true}
	return withOperationErr(ctx, info, func(ctx context.Context) error {
		q := db.Update(meta.Table, cols, dbx.HashExp(where))
		if ctx != nil {
			q.WithContext(ctx)
		}
		res, err := q.Execute()
		if err != nil {
			return wrapQueryError(ErrInvalidQuery, "update_patch", meta.Name, "", err)
		}
		if rows, _ := res.RowsAffected(); rows == 0 {
			return ErrNoRowsAffected.with("update_patch", meta.Name, "", nil)
		}
		return nil
	})
}

func extractPatchColumns(meta *ModelMeta, patch any) (dbx.Params, error) {
	rv := reflect.ValueOf(patch)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, ErrInvalidQuery.with("update_patch", meta.Name, "", fmt.Errorf("patch is nil"))
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return nil, ErrInvalidQuery.with("update_patch", meta.Name, "", fmt.Errorf("patch must be struct"))
	}
	cols := dbx.Params{}
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		if sf.PkgPath != "" {
			continue
		}
		fv := rv.Field(i)
		if !fv.CanInterface() {
			continue
		}
		ov, ok := fv.Interface().(optionalValue)
		if !ok || !ov.IsSet() {
			continue
		}

		name := patchFieldName(sf)
		fm, err := findFieldMeta(meta, name)
		if err != nil {
			return nil, ErrInvalidField.with("update_patch", meta.Name, name, err)
		}
		if fm.IsPK || fm.IsReadOnly || fm.IsGenerated || fm.IsCreatedAt || fm.IsSoftDelete {
			return nil, ErrInvalidField.with("update_patch", meta.Name, name, fmt.Errorf("field is not patch-updatable"))
		}
		if nov, ok := ov.(nullableOptionalValue); ok && nov.IsNull() {
			cols[fm.DBName] = nil
			continue
		}
		encoded, err := encodeFieldValue(fm, ov.ValueAny())
		if err != nil {
			return nil, ErrInvalidQuery.with("update_patch", meta.Name, name, err)
		}
		cols[fm.DBName] = encoded
	}
	return cols, nil
}

func patchFieldName(sf reflect.StructField) string {
	if tag := strings.TrimSpace(sf.Tag.Get("db")); tag != "" && tag != "-" {
		parts := strings.Split(tag, ",")
		if parts[0] != "" {
			return strings.TrimSpace(parts[0])
		}
	}
	return sf.Name
}
