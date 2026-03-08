package orm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/pafthang/dbx"
)

// Insert inserts one model row.
func Insert[T any](ctx context.Context, db DB, model *T) error {
	meta, rv, err := modelMetaAndValue(model)
	if err != nil {
		return err
	}
	info := OperationInfo{Operation: OpInsert, Model: meta.Name, Table: meta.Table}
	return withOperationErr(ctx, info, func(ctx context.Context) error {
		if err := callBeforeInsertHook(ctx, model); err != nil {
			return ErrInvalidQuery.with("before_insert_hook", meta.Name, "", err)
		}

		now := time.Now().UTC()
		if meta.CreatedAtField != nil {
			if setNowIfZero(rv.FieldByIndex(meta.CreatedAtField.Index), now) {
				// include in insert payload
			}
		}
		if meta.UpdatedAtField != nil {
			setTimeValue(rv.FieldByIndex(meta.UpdatedAtField.Index), now)
		}

		cols, autoPK, err := insertColumns(meta, rv)
		if err != nil {
			return err
		}

		if autoPK != nil {
			if err := tryInsertReturning(ctx, db, meta, cols, autoPK, rv); err == nil {
				if err := callAfterInsertHook(ctx, model); err != nil {
					return ErrInvalidQuery.with("after_insert_hook", meta.Name, "", err)
				}
				return nil
			}
		}

		q := db.Insert(meta.Table, cols)
		if ctx != nil {
			q.WithContext(ctx)
		}
		_, err = q.Execute()
		if err != nil {
			return wrapQueryError(ErrInvalidQuery, "insert", meta.Name, "", err)
		}
		if err := callAfterInsertHook(ctx, model); err != nil {
			return ErrInvalidQuery.with("after_insert_hook", meta.Name, "", err)
		}
		return nil
	})
}

// Update updates one model row by primary key.
func Update[T any](ctx context.Context, db DB, model *T) error {
	meta, rv, err := modelMetaAndValue(model)
	if err != nil {
		return err
	}
	info := OperationInfo{Operation: OpUpdate, Model: meta.Name, Table: meta.Table, HasWhere: true}
	return withOperationErr(ctx, info, func(ctx context.Context) error {
		if err := callBeforeUpdateHook(ctx, model); err != nil {
			return ErrInvalidQuery.with("before_update_hook", meta.Name, "", err)
		}
		where, err := primaryKeyWhere(meta, rv)
		if err != nil {
			return err
		}
		where = applyTenantWhere(ctx, meta, where)

		now := time.Now().UTC()
		if meta.UpdatedAtField != nil {
			setTimeValue(rv.FieldByIndex(meta.UpdatedAtField.Index), now)
		}

		cols, err := updateColumns(meta, rv)
		if err != nil {
			return err
		}
		if len(cols) == 0 {
			return ErrInvalidQuery.with("update", meta.Name, "", fmt.Errorf("no columns to update"))
		}

		q := db.Update(meta.Table, cols, dbx.HashExp(where))
		if ctx != nil {
			q.WithContext(ctx)
		}
		res, err := q.Execute()
		if err != nil {
			return wrapQueryError(ErrInvalidQuery, "update", meta.Name, "", err)
		}
		if rows, _ := res.RowsAffected(); rows == 0 {
			return ErrNoRowsAffected.with("update", meta.Name, "", nil)
		}
		if err := callAfterUpdateHook(ctx, model); err != nil {
			return ErrInvalidQuery.with("after_update_hook", meta.Name, "", err)
		}
		return nil
	})
}

// UpdateFields updates only explicitly listed fields by primary key.
func UpdateFields[T any](ctx context.Context, db DB, model *T, fields ...string) error {
	meta, rv, err := modelMetaAndValue(model)
	if err != nil {
		return err
	}
	info := OperationInfo{
		Operation: OpUpdateFields,
		Model:     meta.Name,
		Table:     meta.Table,
		HasWhere:  true,
		Fields:    append([]string(nil), fields...),
	}
	return withOperationErr(ctx, info, func(ctx context.Context) error {
		if err := callBeforeUpdateHook(ctx, model); err != nil {
			return ErrInvalidQuery.with("before_update_hook", meta.Name, "", err)
		}
		where, err := primaryKeyWhere(meta, rv)
		if err != nil {
			return err
		}
		where = applyTenantWhere(ctx, meta, where)
		if len(fields) == 0 {
			return ErrInvalidQuery.with("update_fields", meta.Name, "", fmt.Errorf("no fields provided"))
		}

		cols, err := updateColumnsByNames(meta, rv, fields)
		if err != nil {
			return err
		}
		if len(cols) == 0 {
			return ErrInvalidQuery.with("update_fields", meta.Name, "", fmt.Errorf("no columns to update"))
		}

		if meta.UpdatedAtField != nil {
			if _, exists := cols[meta.UpdatedAtField.DBName]; !exists {
				now := time.Now().UTC()
				setTimeValue(rv.FieldByIndex(meta.UpdatedAtField.Index), now)
				cols[meta.UpdatedAtField.DBName] = now
			}
		}

		q := db.Update(meta.Table, cols, dbx.HashExp(where))
		if ctx != nil {
			q.WithContext(ctx)
		}
		res, err := q.Execute()
		if err != nil {
			return wrapQueryError(ErrInvalidQuery, "update_fields", meta.Name, "", err)
		}
		if rows, _ := res.RowsAffected(); rows == 0 {
			return ErrNoRowsAffected.with("update_fields", meta.Name, "", nil)
		}
		if err := callAfterUpdateHook(ctx, model); err != nil {
			return ErrInvalidQuery.with("after_update_hook", meta.Name, "", err)
		}
		return nil
	})
}

// Delete deletes one model row by primary key. Soft delete is applied when configured.
func Delete[T any](ctx context.Context, db DB, model *T) error {
	meta, rv, err := modelMetaAndValue(model)
	if err != nil {
		return err
	}
	info := OperationInfo{Operation: OpDelete, Model: meta.Name, Table: meta.Table, HasWhere: true}
	return withOperationErr(ctx, info, func(ctx context.Context) error {
		if err := callBeforeDeleteHook(ctx, model); err != nil {
			return ErrInvalidQuery.with("before_delete_hook", meta.Name, "", err)
		}
		where, err := primaryKeyWhere(meta, rv)
		if err != nil {
			return err
		}
		where = applyTenantWhere(ctx, meta, where)
		if meta.SoftDeleteField != nil {
			if err := softDeleteByWhere(ctx, db, meta, rv, where); err != nil {
				return err
			}
			if err := callAfterDeleteHook(ctx, model); err != nil {
				return ErrInvalidQuery.with("after_delete_hook", meta.Name, "", err)
			}
			return nil
		}
		if err := hardDeleteByWhere(ctx, db, meta, where); err != nil {
			return err
		}
		if err := callAfterDeleteHook(ctx, model); err != nil {
			return ErrInvalidQuery.with("after_delete_hook", meta.Name, "", err)
		}
		return nil
	})
}

// ByPK fetches a row by primary key.
func ByPK[T any](ctx context.Context, db DB, id any) (*T, error) {
	meta, err := Meta[T]()
	if err != nil {
		return nil, err
	}
	where, err := pkWhereFromKey(meta, id)
	if err != nil {
		return nil, err
	}
	where = applyTenantWhere(ctx, meta, where)
	if meta.SoftDeleteField != nil {
		where[meta.SoftDeleteField.DBName] = nil
	}
	rawWhere, rawWhereErr := pkWhereFromKey(meta, id)
	rawWhere = applyTenantWhere(ctx, meta, rawWhere)

	info := OperationInfo{Operation: OpByPK, Model: meta.Name, Table: meta.Table, HasWhere: true}
	return withOperationResult(ctx, info, func(ctx context.Context) (*T, error) {
		var out T
		q := db.Select(selectColumns(meta)...).From(meta.Table).Where(dbx.HashExp(where))
		if ctx != nil {
			q.WithContext(ctx)
		}
		err = q.One(&out)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				emitRowsScanned(OpByPK, meta.Name, meta.Table, 0)
				if meta.SoftDeleteField != nil && rawWhereErr == nil {
					existsDeleted, derr := existsByWhere(ctx, db, meta, rawWhere)
					if derr == nil && existsDeleted {
						return nil, ErrSoftDeleted.with("by_pk", meta.Name, "", err)
					}
				}
				return nil, ErrNotFound.with("by_pk", meta.Name, "", err)
			}
			return nil, ErrInvalidQuery.with("by_pk", meta.Name, "", err)
		}
		if err := decodeModelValue(meta, reflect.ValueOf(&out)); err != nil {
			return nil, err
		}
		emitRowsScanned(OpByPK, meta.Name, meta.Table, 1)
		if err := callAfterFindHook(ctx, &out); err != nil {
			return nil, ErrInvalidQuery.with("after_find_hook", meta.Name, "", err)
		}
		return &out, nil
	})
}

func existsByWhere(ctx context.Context, db DB, meta *ModelMeta, where map[string]any) (bool, error) {
	col := "*"
	if len(meta.PrimaryKeys) > 0 {
		col = meta.PrimaryKeys[0].DBName
	}
	q := db.Select(col).From(meta.Table).Where(dbx.HashExp(where)).Limit(1)
	if ctx != nil {
		q.WithContext(ctx)
	}
	n, err := q.Count()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// DeleteByPK deletes by primary key. Soft delete is applied when configured.
func DeleteByPK[T any](ctx context.Context, db DB, id any) error {
	meta, err := Meta[T]()
	if err != nil {
		return err
	}
	where, err := pkWhereFromKey(meta, id)
	if err != nil {
		return err
	}
	where = applyTenantWhere(ctx, meta, where)
	info := OperationInfo{Operation: OpDeleteByPK, Model: meta.Name, Table: meta.Table, HasWhere: true}
	return withOperationErr(ctx, info, func(ctx context.Context) error {
		if meta.SoftDeleteField != nil {
			var zero reflect.Value
			return softDeleteByWhere(ctx, db, meta, zero, where)
		}
		return hardDeleteByWhere(ctx, db, meta, where)
	})
}

// ExistsByPK checks existence by primary key.
func ExistsByPK[T any](ctx context.Context, db DB, id any) (bool, error) {
	meta, err := Meta[T]()
	if err != nil {
		return false, err
	}
	baseWhere, err := pkWhereFromKey(meta, id)
	if err != nil {
		return false, err
	}
	baseWhere = applyTenantWhere(ctx, meta, baseWhere)

	info := OperationInfo{Operation: OpExistsByPK, Model: meta.Name, Table: meta.Table, HasWhere: true}
	return withOperationResult(ctx, info, func(ctx context.Context) (bool, error) {
		where := dbx.HashExp(baseWhere)
		if meta.SoftDeleteField != nil {
			where[meta.SoftDeleteField.DBName] = nil
		}
		q := db.Select("*").From(meta.Table).Where(where).Limit(1)
		if ctx != nil {
			q.WithContext(ctx)
		}
		n, err := q.Count()
		if err != nil {
			return false, ErrInvalidQuery.with("exists_by_pk", meta.Name, "", err)
		}
		return n > 0, nil
	})
}

// Count returns total rows for model T, excluding soft-deleted rows by default.
func Count[T any](ctx context.Context, db DB) (int64, error) {
	meta, err := Meta[T]()
	if err != nil {
		return 0, err
	}
	info := OperationInfo{Operation: OpCount, Model: meta.Name, Table: meta.Table}
	return withOperationResult(ctx, info, func(ctx context.Context) (int64, error) {
		q := db.Select("*").From(meta.Table)
		where := map[string]any{}
		where = applyTenantWhere(ctx, meta, where)
		if meta.SoftDeleteField != nil {
			where[meta.SoftDeleteField.DBName] = nil
		}
		if len(where) > 0 {
			q.Where(dbx.HashExp(where))
		}
		if ctx != nil {
			q.WithContext(ctx)
		}
		n, err := q.Count()
		if err != nil {
			return 0, ErrInvalidQuery.with("count", meta.Name, "", err)
		}
		return n, nil
	})
}

func modelMetaAndValue(model any) (*ModelMeta, reflect.Value, error) {
	if model == nil {
		return nil, reflect.Value{}, ErrInvalidModel.with("model_value", "", "", fmt.Errorf("model is nil"))
	}
	rv := reflect.ValueOf(model)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return nil, reflect.Value{}, ErrInvalidModel.with("model_value", "", "", fmt.Errorf("model must be non-nil pointer"))
	}
	rv = rv.Elem()
	if rv.Kind() != reflect.Struct {
		return nil, reflect.Value{}, ErrInvalidModel.with("model_value", rv.Type().String(), "", fmt.Errorf("model must point to struct"))
	}
	meta, err := DefaultRegistry.Resolve(rv.Interface())
	if err != nil {
		return nil, reflect.Value{}, err
	}
	return meta, rv, nil
}

func insertColumns(meta *ModelMeta, rv reflect.Value) (dbx.Params, *FieldMeta, error) {
	cols := dbx.Params{}
	var autoPK *FieldMeta
	for _, f := range meta.Fields {
		if f.IsIgnored || f.IsReadOnly || f.IsGenerated {
			continue
		}
		fv := rv.FieldByIndex(f.Index)
		if f.IsPK && len(meta.PrimaryKeys) == 1 && isAutoPKCandidate(fv) {
			autoPK = f
			continue
		}
		if f.HasDefault && isZeroValue(fv) {
			continue
		}
		val, err := encodeFieldValue(f, fv.Interface())
		if err != nil {
			return nil, nil, ErrInvalidQuery.with("insert", meta.Name, f.GoName, err)
		}
		cols[f.DBName] = val
	}
	if len(cols) == 0 && autoPK == nil {
		return nil, nil, ErrInvalidQuery.with("insert", meta.Name, "", fmt.Errorf("empty insert payload"))
	}
	return cols, autoPK, nil
}

func updateColumns(meta *ModelMeta, rv reflect.Value) (dbx.Params, error) {
	cols := dbx.Params{}
	for _, f := range meta.Fields {
		if f.IsIgnored || f.IsPK || f.IsReadOnly || f.IsGenerated || f.IsCreatedAt || f.IsSoftDelete {
			continue
		}
		val, err := encodeFieldValue(f, rv.FieldByIndex(f.Index).Interface())
		if err != nil {
			return nil, ErrInvalidQuery.with("update", meta.Name, f.GoName, err)
		}
		cols[f.DBName] = val
	}
	return cols, nil
}

func updateColumnsByNames(meta *ModelMeta, rv reflect.Value, fields []string) (dbx.Params, error) {
	cols := dbx.Params{}
	for _, field := range fields {
		fm, err := findFieldMeta(meta, field)
		if err != nil {
			return nil, err
		}
		if fm.IsPK || fm.IsReadOnly || fm.IsGenerated || fm.IsCreatedAt || fm.IsSoftDelete {
			return nil, ErrInvalidField.with("update_fields", meta.Name, field, fmt.Errorf("field is not updatable"))
		}
		val, err := encodeFieldValue(fm, rv.FieldByIndex(fm.Index).Interface())
		if err != nil {
			return nil, ErrInvalidQuery.with("update_fields", meta.Name, fm.GoName, err)
		}
		cols[fm.DBName] = val
	}
	return cols, nil
}

func findFieldMeta(meta *ModelMeta, field string) (*FieldMeta, error) {
	name := strings.TrimSpace(field)
	if name == "" {
		return nil, ErrInvalidField.with("find_field", meta.Name, field, fmt.Errorf("field is empty"))
	}
	if fm, ok := meta.FieldsByGo[name]; ok {
		if fm.IsIgnored {
			return nil, ErrInvalidField.with("find_field", meta.Name, field, fmt.Errorf("field is ignored"))
		}
		return fm, nil
	}
	if fm, ok := meta.FieldsByDB[name]; ok {
		if fm.IsIgnored {
			return nil, ErrInvalidField.with("find_field", meta.Name, field, fmt.Errorf("field is ignored"))
		}
		return fm, nil
	}
	return nil, ErrInvalidField.with("find_field", meta.Name, field, fmt.Errorf("unknown field"))
}

func primaryKeyWhere(meta *ModelMeta, rv reflect.Value) (map[string]any, error) {
	if len(meta.PrimaryKeys) == 0 {
		return nil, ErrMissingPrimaryKey.with("pk_where", meta.Name, "", nil)
	}
	where := make(map[string]any, len(meta.PrimaryKeys))
	for _, pk := range meta.PrimaryKeys {
		where[pk.DBName] = rv.FieldByIndex(pk.Index).Interface()
	}
	return where, nil
}

func pkWhereFromKey(meta *ModelMeta, key any) (map[string]any, error) {
	if len(meta.PrimaryKeys) == 0 {
		return nil, ErrMissingPrimaryKey.with("pk_where_key", meta.Name, "", nil)
	}
	if len(meta.PrimaryKeys) == 1 {
		if m, ok := key.(map[string]any); ok {
			pk := meta.PrimaryKeys[0]
			if v, ok := m[pk.GoName]; ok {
				return map[string]any{pk.DBName: v}, nil
			}
			if v, ok := m[pk.DBName]; ok {
				return map[string]any{pk.DBName: v}, nil
			}
		}
		if rv := reflect.ValueOf(key); rv.IsValid() && rv.Kind() == reflect.Struct {
			if m, err := structToMap(rv); err == nil {
				return pkWhereFromMap(meta, m)
			}
		}
		return map[string]any{meta.PrimaryKeys[0].DBName: key}, nil
	}

	if m, ok := key.(map[string]any); ok {
		return pkWhereFromMap(meta, m)
	}
	rv := reflect.ValueOf(key)
	if rv.IsValid() && (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array) {
		if rv.Len() != len(meta.PrimaryKeys) {
			return nil, ErrInvalidQuery.with("pk_where_key", meta.Name, "", fmt.Errorf("composite key tuple length mismatch"))
		}
		where := make(map[string]any, len(meta.PrimaryKeys))
		for i, pk := range meta.PrimaryKeys {
			elem := rv.Index(i)
			if elem.Kind() == reflect.Pointer {
				if elem.IsNil() {
					return nil, ErrInvalidQuery.with("pk_where_key", meta.Name, pk.GoName, fmt.Errorf("nil tuple key part"))
				}
				elem = elem.Elem()
			}
			where[pk.DBName] = elem.Interface()
		}
		return where, nil
	}
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil, ErrInvalidQuery.with("pk_where_key", meta.Name, "", fmt.Errorf("composite key pointer is nil"))
		}
		rv = rv.Elem()
	}
	if rv.IsValid() && rv.Kind() == reflect.Struct {
		m, err := structToMap(rv)
		if err != nil {
			return nil, err
		}
		return pkWhereFromMap(meta, m)
	}
	return nil, ErrInvalidQuery.with("pk_where_key", meta.Name, "", fmt.Errorf("composite key requires map or struct"))
}

func pkWhereFromMap(meta *ModelMeta, in map[string]any) (map[string]any, error) {
	where := make(map[string]any, len(meta.PrimaryKeys))
	for _, pk := range meta.PrimaryKeys {
		if v, ok := in[pk.GoName]; ok {
			where[pk.DBName] = v
			continue
		}
		if v, ok := in[pk.DBName]; ok {
			where[pk.DBName] = v
			continue
		}
		return nil, ErrInvalidQuery.with("pk_where_key", meta.Name, pk.GoName, fmt.Errorf("missing key part"))
	}
	return where, nil
}

func structToMap(rv reflect.Value) (map[string]any, error) {
	if rv.Kind() != reflect.Struct {
		return nil, ErrInvalidQuery.with("pk_where_key", "", "", fmt.Errorf("key must be struct"))
	}
	out := map[string]any{}
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		if sf.PkgPath != "" {
			continue
		}
		out[sf.Name] = rv.Field(i).Interface()
		dbName, _, ignore := parseDBTag(sf.Tag.Get("db"))
		if !ignore && dbName != "" {
			out[dbName] = rv.Field(i).Interface()
		}
	}
	return out, nil
}

func selectColumns(meta *ModelMeta) []string {
	cols := make([]string, 0, len(meta.Fields))
	for _, f := range meta.Fields {
		if f.IsIgnored || f.IsWriteOnly {
			continue
		}
		cols = append(cols, f.DBName)
	}
	return cols
}

func tryInsertReturning(ctx context.Context, db DB, meta *ModelMeta, cols dbx.Params, pk *FieldMeta, rv reflect.Value) error {
	var id int64
	q := db.InsertReturning(meta.Table, cols, pk.DBName)
	if ctx != nil {
		q.WithContext(ctx)
	}
	if err := q.Row(&id); err != nil {
		return err
	}
	setNumber(rv.FieldByIndex(pk.Index), id)
	return nil
}

func setNumber(v reflect.Value, n int64) {
	if !v.CanSet() {
		return
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if n >= 0 {
			v.SetUint(uint64(n))
		}
	}
}

func setTimeValue(v reflect.Value, ts time.Time) {
	if !v.CanSet() {
		return
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			vv := reflect.New(v.Type().Elem())
			vv.Elem().Set(reflect.ValueOf(ts))
			v.Set(vv)
			return
		}
		v.Elem().Set(reflect.ValueOf(ts))
		return
	}
	if v.Type() == reflect.TypeOf(time.Time{}) {
		v.Set(reflect.ValueOf(ts))
	}
}

func setNowIfZero(v reflect.Value, ts time.Time) bool {
	if isZeroValue(v) {
		setTimeValue(v, ts)
		return true
	}
	return false
}

func isAutoPKCandidate(v reflect.Value) bool {
	if v.Kind() == reflect.Pointer {
		return v.IsNil()
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.IsZero()
	default:
		return false
	}
}

func isZeroValue(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	return v.IsZero()
}

func hardDeleteByWhere(ctx context.Context, db DB, meta *ModelMeta, where map[string]any) error {
	q := db.Delete(meta.Table, dbx.HashExp(where))
	if ctx != nil {
		q.WithContext(ctx)
	}
	res, err := q.Execute()
	if err != nil {
		return wrapQueryError(ErrInvalidQuery, "delete", meta.Name, "", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return ErrNoRowsAffected.with("delete", meta.Name, "", nil)
	}
	return nil
}

func softDeleteByWhere(ctx context.Context, db DB, meta *ModelMeta, rv reflect.Value, where map[string]any) error {
	now := time.Now().UTC()
	updates := dbx.Params{}
	sf := meta.SoftDeleteField
	if sf == nil {
		return ErrInvalidModel.with("soft_delete", meta.Name, "", fmt.Errorf("model has no soft delete field"))
	}

	if rv.IsValid() {
		f := rv.FieldByIndex(sf.Index)
		switch f.Kind() {
		case reflect.Bool:
			f.SetBool(true)
			updates[sf.DBName] = true
		case reflect.Pointer:
			setTimeValue(f, now)
			updates[sf.DBName] = f.Interface()
		default:
			if f.Type() == reflect.TypeOf(time.Time{}) {
				f.Set(reflect.ValueOf(now))
				updates[sf.DBName] = now
			}
		}
	}
	if _, ok := updates[sf.DBName]; !ok {
		// No model value available (DeleteByPK), use conservative default.
		updates[sf.DBName] = now
	}
	if meta.UpdatedAtField != nil {
		updates[meta.UpdatedAtField.DBName] = now
		if rv.IsValid() {
			setTimeValue(rv.FieldByIndex(meta.UpdatedAtField.Index), now)
		}
	}

	q := db.Update(meta.Table, updates, dbx.HashExp(where))
	if ctx != nil {
		q.WithContext(ctx)
	}
	res, err := q.Execute()
	if err != nil {
		return wrapQueryError(ErrInvalidQuery, "soft_delete", meta.Name, "", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return ErrNoRowsAffected.with("soft_delete", meta.Name, "", nil)
	}
	return nil
}
