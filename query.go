package orm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/pafthang/dbx"
)

// ModelQuery is a typed query builder for model T.
type ModelQuery[T any] struct {
	db      DB
	meta    *ModelMeta
	where   []dbx.Expression
	orderBy []string
	joins   []joinSpec
	limit   int64
	offset  int64

	withDeleted      bool
	onlyDeleted      bool
	hardDelete       bool
	selectCols       []string
	excludeCols      map[string]struct{}
	setCols          dbx.Params
	preloads         []preloadSpec
	preloadChunkSize int

	err      error
	paramSeq int
}

// ListResult is a paginated query response.
type ListResult[T any] struct {
	Items   []T
	Total   int64
	Page    int64
	PerPage int64
}

type preloadSpec struct {
	path preloadPath
	opts preloadOptions
}

type preloadOptions struct {
	withDeleted bool
	orderBy     []string
	orderBySafe []preloadOrderField
	limit       int64
	whereEq     []preloadEqCond
	whereIn     []preloadInCond
	whereLike   []preloadLikeCond
	whereNull   []string
	whereNotNil []string
	whereCmp    []preloadCmpCond
	whereExpr   []dbx.Expression
}

type preloadEqCond struct {
	field string
	value any
}

type joinSpec struct {
	relation string
	left     bool
}

type preloadInCond struct {
	field  string
	values any
}

type preloadLikeCond struct {
	field string
	value any
}

type preloadCmpCond struct {
	field string
	op    string
	value any
}

// SortDirection controls ASC/DESC ordering.
type SortDirection string

const (
	SortAsc  SortDirection = "ASC"
	SortDesc SortDirection = "DESC"
)

type preloadOrderField struct {
	field string
	dir   SortDirection
}

// PreloadOption configures relation preloading behavior.
type PreloadOption func(*preloadOptions)

// PreloadWithDeleted includes soft-deleted rows for the target preload relation.
func PreloadWithDeleted() PreloadOption {
	return func(o *preloadOptions) {
		o.withDeleted = true
	}
}

// PreloadOrderBy adds ORDER BY expression to relation preload query.
// Deprecated: use UnsafePreloadOrderBy to make low-level behavior explicit.
func PreloadOrderBy(orderExpr string) PreloadOption {
	return UnsafePreloadOrderBy(orderExpr)
}

// UnsafePreloadOrderBy adds raw ORDER BY expression to relation preload query.
func UnsafePreloadOrderBy(orderExpr string) PreloadOption {
	return func(o *preloadOptions) {
		orderExpr = strings.TrimSpace(orderExpr)
		if orderExpr != "" {
			o.orderBy = append(o.orderBy, orderExpr)
		}
	}
}

// PreloadOrderByField adds metadata-aware ORDER BY field for preload query.
func PreloadOrderByField(field string, dir SortDirection) PreloadOption {
	return func(o *preloadOptions) {
		field = strings.TrimSpace(field)
		if field == "" {
			return
		}
		if dir != SortDesc {
			dir = SortAsc
		}
		o.orderBySafe = append(o.orderBySafe, preloadOrderField{
			field: field,
			dir:   dir,
		})
	}
}

// PreloadLimit applies LIMIT to relation preload query.
func PreloadLimit(n int64) PreloadOption {
	return func(o *preloadOptions) {
		o.limit = n
	}
}

// PreloadWhereEq adds equality predicate to relation preload query.
func PreloadWhereEq(field string, value any) PreloadOption {
	return func(o *preloadOptions) {
		field = strings.TrimSpace(field)
		if field != "" {
			o.whereEq = append(o.whereEq, preloadEqCond{field: field, value: value})
		}
	}
}

// PreloadWhereIn adds metadata-aware IN predicate for relation preload query.
func PreloadWhereIn(field string, values any) PreloadOption {
	return func(o *preloadOptions) {
		field = strings.TrimSpace(field)
		if field != "" {
			o.whereIn = append(o.whereIn, preloadInCond{field: field, values: values})
		}
	}
}

// PreloadWhereLike adds metadata-aware LIKE predicate for relation preload query.
func PreloadWhereLike(field string, value any) PreloadOption {
	return func(o *preloadOptions) {
		field = strings.TrimSpace(field)
		if field != "" {
			o.whereLike = append(o.whereLike, preloadLikeCond{field: field, value: value})
		}
	}
}

// PreloadWhereNull adds metadata-aware IS NULL predicate for relation preload query.
func PreloadWhereNull(field string) PreloadOption {
	return func(o *preloadOptions) {
		field = strings.TrimSpace(field)
		if field != "" {
			o.whereNull = append(o.whereNull, field)
		}
	}
}

// PreloadWhereNotNull adds metadata-aware IS NOT NULL predicate for relation preload query.
func PreloadWhereNotNull(field string) PreloadOption {
	return func(o *preloadOptions) {
		field = strings.TrimSpace(field)
		if field != "" {
			o.whereNotNil = append(o.whereNotNil, field)
		}
	}
}

func preloadWhereCmp(field, op string, value any) PreloadOption {
	return func(o *preloadOptions) {
		field = strings.TrimSpace(field)
		if field != "" {
			o.whereCmp = append(o.whereCmp, preloadCmpCond{field: field, op: op, value: value})
		}
	}
}

// PreloadWhereGT adds metadata-aware > predicate for relation preload query.
func PreloadWhereGT(field string, value any) PreloadOption { return preloadWhereCmp(field, ">", value) }

// PreloadWhereGTE adds metadata-aware >= predicate for relation preload query.
func PreloadWhereGTE(field string, value any) PreloadOption {
	return preloadWhereCmp(field, ">=", value)
}

// PreloadWhereLT adds metadata-aware < predicate for relation preload query.
func PreloadWhereLT(field string, value any) PreloadOption { return preloadWhereCmp(field, "<", value) }

// PreloadWhereLTE adds metadata-aware <= predicate for relation preload query.
func PreloadWhereLTE(field string, value any) PreloadOption {
	return preloadWhereCmp(field, "<=", value)
}

// PreloadWhereExpr adds low-level SQL expression to relation preload query.
// Deprecated: use UnsafePreloadWhereExpr to make low-level behavior explicit.
func PreloadWhereExpr(sqlExpr string, params dbx.Params) PreloadOption {
	return UnsafePreloadWhereExpr(sqlExpr, params)
}

// UnsafePreloadWhereExpr adds raw SQL expression to relation preload query.
func UnsafePreloadWhereExpr(sqlExpr string, params dbx.Params) PreloadOption {
	return func(o *preloadOptions) {
		sqlExpr = strings.TrimSpace(sqlExpr)
		if sqlExpr != "" {
			o.whereExpr = append(o.whereExpr, newRawExp(sqlExpr, params))
		}
	}
}

// PreloadScope allows callback-style preload configuration.
type PreloadScope struct{ opts *preloadOptions }

func (s *PreloadScope) WithDeleted()        { s.opts.withDeleted = true }
func (s *PreloadScope) OrderBy(expr string) { PreloadOrderBy(expr)(s.opts) }
func (s *PreloadScope) OrderByField(field string, dir SortDirection) {
	PreloadOrderByField(field, dir)(s.opts)
}
func (s *PreloadScope) Limit(n int64)                 { PreloadLimit(n)(s.opts) }
func (s *PreloadScope) WhereEq(field string, v any)   { PreloadWhereEq(field, v)(s.opts) }
func (s *PreloadScope) WhereIn(field string, v any)   { PreloadWhereIn(field, v)(s.opts) }
func (s *PreloadScope) WhereLike(field string, v any) { PreloadWhereLike(field, v)(s.opts) }
func (s *PreloadScope) WhereNull(field string)        { PreloadWhereNull(field)(s.opts) }
func (s *PreloadScope) WhereNotNull(field string)     { PreloadWhereNotNull(field)(s.opts) }
func (s *PreloadScope) WhereGT(field string, v any)   { PreloadWhereGT(field, v)(s.opts) }
func (s *PreloadScope) WhereGTE(field string, v any)  { PreloadWhereGTE(field, v)(s.opts) }
func (s *PreloadScope) WhereLT(field string, v any)   { PreloadWhereLT(field, v)(s.opts) }
func (s *PreloadScope) WhereLTE(field string, v any)  { PreloadWhereLTE(field, v)(s.opts) }
func (s *PreloadScope) WhereExpr(sqlExpr string, params dbx.Params) {
	PreloadWhereExpr(sqlExpr, params)(s.opts)
}

// PreloadConfigure applies callback-style preload config.
func PreloadConfigure(fn func(*PreloadScope)) PreloadOption {
	return func(o *preloadOptions) {
		if fn == nil {
			return
		}
		fn(&PreloadScope{opts: o})
	}
}

// Query creates a typed model query.
func Query[T any](db DB) *ModelQuery[T] {
	meta, err := Meta[T]()
	q := &ModelQuery[T]{
		db:               db,
		meta:             meta,
		limit:            -1,
		offset:           -1,
		preloadChunkSize: 500,
		excludeCols:      map[string]struct{}{},
		setCols:          dbx.Params{},
		err:              err,
	}
	return q
}

func (q *ModelQuery[T]) WithDeleted() *ModelQuery[T] {
	q.withDeleted = true
	q.onlyDeleted = false
	return q
}

// OnlyDeleted limits query to soft-deleted rows only.
func (q *ModelQuery[T]) OnlyDeleted() *ModelQuery[T] {
	q.onlyDeleted = true
	q.withDeleted = false
	return q
}

// Preload eagerly loads an explicit relation by name.
func (q *ModelQuery[T]) Preload(name string, opts ...PreloadOption) *ModelQuery[T] {
	if q.err != nil {
		return q
	}
	name = strings.TrimSpace(name)
	if name == "" {
		q.err = ErrInvalidQuery.with("preload", q.meta.Name, "", fmt.Errorf("relation name is empty"))
		return q
	}
	path, err := parsePreloadPath(name)
	if err != nil {
		q.err = err
		return q
	}
	if _, ok := q.meta.Relations[path[0]]; !ok {
		q.err = ErrRelationNotFound.with("preload", q.meta.Name, path[0], fmt.Errorf("unknown relation"))
		return q
	}
	spec := preloadSpec{
		path: path,
		opts: preloadOptions{
			limit: -1,
		},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&spec.opts)
		}
	}
	q.preloads = append(q.preloads, spec)
	return q
}

// HardDelete forces physical delete in Delete(ctx), even for soft-delete models.
func (q *ModelQuery[T]) HardDelete() *ModelQuery[T] {
	q.hardDelete = true
	return q
}

// PreloadChunkSize customizes chunk size for relation preload IN/OR batches.
func (q *ModelQuery[T]) PreloadChunkSize(n int) *ModelQuery[T] {
	if n > 0 {
		q.preloadChunkSize = n
	}
	return q
}

func (q *ModelQuery[T]) Select(fields ...string) *ModelQuery[T] {
	if q.err != nil {
		return q
	}
	q.selectCols = q.selectCols[:0]
	for _, f := range fields {
		col, err := q.resolveField(f)
		if err != nil {
			q.err = err
			return q
		}
		q.selectCols = append(q.selectCols, col)
	}
	return q
}

func (q *ModelQuery[T]) Exclude(fields ...string) *ModelQuery[T] {
	if q.err != nil {
		return q
	}
	for _, f := range fields {
		col, err := q.resolveField(f)
		if err != nil {
			q.err = err
			return q
		}
		q.excludeCols[col] = struct{}{}
	}
	return q
}

// JoinRelation adds INNER JOIN by relation metadata.
func (q *ModelQuery[T]) JoinRelation(name string) *ModelQuery[T] {
	return q.addRelationJoin(name, false)
}

// LeftJoinRelation adds LEFT JOIN by relation metadata.
func (q *ModelQuery[T]) LeftJoinRelation(name string) *ModelQuery[T] {
	return q.addRelationJoin(name, true)
}

// WhereRelationEq adds metadata-aware equality filter by relation field path: "Relation.Field".
func (q *ModelQuery[T]) WhereRelationEq(path string, value any) *ModelQuery[T] {
	if q.err != nil {
		return q
	}
	col, err := q.resolveRelationColumn(path)
	if err != nil {
		q.err = err
		return q
	}
	q.where = append(q.where, dbx.HashExp{col: value})
	return q
}

// WhereRelationIn adds metadata-aware IN filter by relation field path: "Relation.Field".
func (q *ModelQuery[T]) WhereRelationIn(path string, values any) *ModelQuery[T] {
	if q.err != nil {
		return q
	}
	col, err := q.resolveRelationColumn(path)
	if err != nil {
		q.err = err
		return q
	}
	q.where = append(q.where, dbx.In(col, values))
	return q
}

// WhereRelationLike adds metadata-aware LIKE filter by relation field path: "Relation.Field".
func (q *ModelQuery[T]) WhereRelationLike(path string, value any) *ModelQuery[T] {
	if q.err != nil {
		return q
	}
	col, err := q.resolveRelationColumn(path)
	if err != nil {
		q.err = err
		return q
	}
	q.where = append(q.where, dbx.Like(col, value))
	return q
}

// OrderByRelation adds metadata-aware ASC order by relation field path: "Relation.Field".
func (q *ModelQuery[T]) OrderByRelation(path string) *ModelQuery[T] {
	return q.orderByRelation(path, false)
}

// OrderByRelationDesc adds metadata-aware DESC order by relation field path: "Relation.Field".
func (q *ModelQuery[T]) OrderByRelationDesc(path string) *ModelQuery[T] {
	return q.orderByRelation(path, true)
}

func (q *ModelQuery[T]) WhereEq(field string, value any) *ModelQuery[T] {
	return q.addWhereHash(field, value)
}

func (q *ModelQuery[T]) WhereNotEq(field string, value any) *ModelQuery[T] {
	if q.err != nil {
		return q
	}
	col, err := q.resolveField(field)
	if err != nil {
		q.err = err
		return q
	}
	if value == nil {
		q.where = append(q.where, newRawExp(col+" IS NOT NULL", nil))
		return q
	}
	q.where = append(q.where, newCmpExp(col, "!=", value, q.nextParam()))
	return q
}

func (q *ModelQuery[T]) WhereIn(field string, values any) *ModelQuery[T] {
	if q.err != nil {
		return q
	}
	col, err := q.resolveField(field)
	if err != nil {
		q.err = err
		return q
	}
	q.where = append(q.where, dbx.In(col, values))
	return q
}

func (q *ModelQuery[T]) WhereNull(field string) *ModelQuery[T] {
	if q.err != nil {
		return q
	}
	col, err := q.resolveField(field)
	if err != nil {
		q.err = err
		return q
	}
	q.where = append(q.where, newRawExp(col+" IS NULL", nil))
	return q
}

func (q *ModelQuery[T]) WhereNotNull(field string) *ModelQuery[T] {
	if q.err != nil {
		return q
	}
	col, err := q.resolveField(field)
	if err != nil {
		q.err = err
		return q
	}
	q.where = append(q.where, newRawExp(col+" IS NOT NULL", nil))
	return q
}

func (q *ModelQuery[T]) WhereLike(field string, value any) *ModelQuery[T] {
	if q.err != nil {
		return q
	}
	col, err := q.resolveField(field)
	if err != nil {
		q.err = err
		return q
	}
	q.where = append(q.where, dbx.Like(col, value))
	return q
}

func (q *ModelQuery[T]) WhereGT(field string, value any) *ModelQuery[T] {
	return q.addCmp(field, ">", value)
}

func (q *ModelQuery[T]) WhereGTE(field string, value any) *ModelQuery[T] {
	return q.addCmp(field, ">=", value)
}

func (q *ModelQuery[T]) WhereLT(field string, value any) *ModelQuery[T] {
	return q.addCmp(field, "<", value)
}

func (q *ModelQuery[T]) WhereLTE(field string, value any) *ModelQuery[T] {
	return q.addCmp(field, "<=", value)
}

// WhereExpr adds a low-level SQL expression. Use cautiously.
// Deprecated: use UnsafeWhereExpr to make low-level behavior explicit.
func (q *ModelQuery[T]) WhereExpr(sqlExpr string, params dbx.Params) *ModelQuery[T] {
	return q.UnsafeWhereExpr(sqlExpr, params)
}

// UnsafeWhereExpr adds a raw SQL expression. Use cautiously.
func (q *ModelQuery[T]) UnsafeWhereExpr(sqlExpr string, params dbx.Params) *ModelQuery[T] {
	if q.err != nil {
		return q
	}
	q.where = append(q.where, newRawExp(sqlExpr, params))
	return q
}

// Set adds one field assignment for query-based Update(ctx).
func (q *ModelQuery[T]) Set(field string, value any) *ModelQuery[T] {
	if q.err != nil {
		return q
	}
	fm, err := q.resolveFieldMeta(field)
	if err != nil {
		q.err = err
		return q
	}
	if fm.IsPK || fm.IsReadOnly || fm.IsGenerated || fm.IsCreatedAt || fm.IsSoftDelete {
		q.err = ErrInvalidField.with("set", q.meta.Name, field, fmt.Errorf("field is not updatable"))
		return q
	}
	encoded, err := encodeFieldValue(fm, value)
	if err != nil {
		q.err = ErrInvalidQuery.with("set", q.meta.Name, field, err)
		return q
	}
	q.setCols[fm.DBName] = encoded
	return q
}

func (q *ModelQuery[T]) OrderBy(field string) *ModelQuery[T] {
	return q.addOrder(field, false)
}

func (q *ModelQuery[T]) OrderByDesc(field string) *ModelQuery[T] {
	return q.addOrder(field, true)
}

func (q *ModelQuery[T]) Limit(v int64) *ModelQuery[T] {
	q.limit = v
	return q
}

func (q *ModelQuery[T]) Offset(v int64) *ModelQuery[T] {
	q.offset = v
	return q
}

func (q *ModelQuery[T]) Page(page, perPage int64) *ModelQuery[T] {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	q.limit = perPage
	q.offset = (page - 1) * perPage
	return q
}

// List returns paginated items with total count.
func (q *ModelQuery[T]) List(ctx context.Context, page, perPage int64) (*ListResult[T], error) {
	if q.err != nil {
		return nil, q.err
	}
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}

	totalQ := q.clone()
	total, err := totalQ.Count(ctx)
	if err != nil {
		return nil, err
	}

	itemsQ := q.clone().Page(page, perPage)
	items, err := itemsQ.All(ctx)
	if err != nil {
		return nil, err
	}
	return &ListResult[T]{
		Items:   items,
		Total:   total,
		Page:    page,
		PerPage: perPage,
	}, nil
}

func (q *ModelQuery[T]) All(ctx context.Context) ([]T, error) {
	if q.err != nil {
		return nil, q.err
	}
	info := q.operationInfo(OpQueryAll)
	return withOperationResult(ctx, info, func(ctx context.Context) ([]T, error) {
		sel, err := q.buildSelect(ctx)
		if err != nil {
			return nil, err
		}
		var out []T
		if err := sel.All(&out); err != nil {
			return nil, ErrInvalidQuery.with("query_all", q.meta.Name, "", err)
		}
		for i := range out {
			if err := decodeModelValue(q.meta, reflect.ValueOf(&out[i])); err != nil {
				return nil, err
			}
		}
		emitRowsScanned(OpQueryAll, q.meta.Name, q.meta.Table, int64(len(out)))
		if err := q.applyPreloadsToSlice(ctx, out); err != nil {
			return nil, err
		}
		for i := range out {
			if err := callAfterFindHook(ctx, &out[i]); err != nil {
				return nil, ErrInvalidQuery.with("after_find_hook", q.meta.Name, "", err)
			}
		}
		return out, nil
	})
}

func (q *ModelQuery[T]) One(ctx context.Context) (*T, error) {
	if q.err != nil {
		return nil, q.err
	}
	info := q.operationInfo(OpQueryOne)
	return withOperationResult(ctx, info, func(ctx context.Context) (*T, error) {
		sel, err := q.buildSelect(ctx)
		if err != nil {
			return nil, err
		}
		var out T
		if err := sel.One(&out); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				emitRowsScanned(OpQueryOne, q.meta.Name, q.meta.Table, 0)
				return nil, ErrNotFound.with("query_one", q.meta.Name, "", err)
			}
			return nil, ErrInvalidQuery.with("query_one", q.meta.Name, "", err)
		}
		if err := decodeModelValue(q.meta, reflect.ValueOf(&out)); err != nil {
			return nil, err
		}
		emitRowsScanned(OpQueryOne, q.meta.Name, q.meta.Table, 1)
		if err := q.applyPreloadsToOne(ctx, &out); err != nil {
			return nil, err
		}
		if err := callAfterFindHook(ctx, &out); err != nil {
			return nil, ErrInvalidQuery.with("after_find_hook", q.meta.Name, "", err)
		}
		return &out, nil
	})
}

// FindOne expects exactly one row.
func (q *ModelQuery[T]) FindOne(ctx context.Context) (*T, error) {
	if q.err != nil {
		return nil, q.err
	}
	info := q.operationInfo(OpQueryOne)
	return withOperationResult(ctx, info, func(ctx context.Context) (*T, error) {
		qq := q.clone()
		qq.limit = 2
		sel, err := qq.buildSelect(ctx)
		if err != nil {
			return nil, err
		}
		var rows []T
		if err := sel.All(&rows); err != nil {
			return nil, ErrInvalidQuery.with("query_find_one", q.meta.Name, "", err)
		}
		for i := range rows {
			if err := decodeModelValue(q.meta, reflect.ValueOf(&rows[i])); err != nil {
				return nil, err
			}
		}
		emitRowsScanned(OpQueryOne, q.meta.Name, q.meta.Table, int64(len(rows)))
		switch len(rows) {
		case 0:
			return nil, ErrNotFound.with("query_find_one", q.meta.Name, "", sql.ErrNoRows)
		case 1:
			if err := callAfterFindHook(ctx, &rows[0]); err != nil {
				return nil, ErrInvalidQuery.with("after_find_hook", q.meta.Name, "", err)
			}
			return &rows[0], nil
		default:
			return nil, ErrMultipleRows.with("query_find_one", q.meta.Name, "", fmt.Errorf("expected one row, got %d", len(rows)))
		}
	})
}

// FindAll is an alias for All.
func (q *ModelQuery[T]) FindAll(ctx context.Context) ([]T, error) {
	return q.All(ctx)
}

func (q *ModelQuery[T]) First(ctx context.Context) (*T, error) {
	if q.limit < 0 {
		q.limit = 1
	}
	return q.One(ctx)
}

func (q *ModelQuery[T]) Count(ctx context.Context) (int64, error) {
	if q.err != nil {
		return 0, q.err
	}
	info := q.operationInfo(OpQueryCount)
	return withOperationResult(ctx, info, func(ctx context.Context) (int64, error) {
		sel, err := q.buildSelect(ctx)
		if err != nil {
			return 0, err
		}
		n, err := sel.Count()
		if err != nil {
			return 0, ErrInvalidQuery.with("query_count", q.meta.Name, "", err)
		}
		return n, nil
	})
}

func (q *ModelQuery[T]) Exists(ctx context.Context) (bool, error) {
	n, err := q.Limit(1).Count(ctx)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// Update executes bulk update for filtered rows.
func (q *ModelQuery[T]) Update(ctx context.Context) (int64, error) {
	if q.err != nil {
		return 0, q.err
	}
	info := q.operationInfo(OpQueryUpdate)
	return withOperationResult(ctx, info, func(ctx context.Context) (int64, error) {
		if len(q.setCols) == 0 {
			return 0, ErrInvalidQuery.with("query_update", q.meta.Name, "", fmt.Errorf("no set columns"))
		}
		where := q.buildWhereExpr(ctx, true)
		now := time.Now().UTC()
		updates := dbx.Params{}
		for k, v := range q.setCols {
			updates[k] = v
		}
		if q.meta.UpdatedAtField != nil {
			if _, exists := updates[q.meta.UpdatedAtField.DBName]; !exists {
				updates[q.meta.UpdatedAtField.DBName] = now
			}
		}
		query := q.db.Update(q.meta.Table, updates, where)
		if ctx != nil {
			query.WithContext(ctx)
		}
		res, err := query.Execute()
		if err != nil {
			return 0, wrapQueryError(ErrInvalidQuery, "query_update", q.meta.Name, "", err)
		}
		rows, _ := res.RowsAffected()
		return rows, nil
	})
}

// Delete executes bulk delete for filtered rows.
func (q *ModelQuery[T]) Delete(ctx context.Context) (int64, error) {
	if q.err != nil {
		return 0, q.err
	}
	info := q.operationInfo(OpQueryDelete)
	return withOperationResult(ctx, info, func(ctx context.Context) (int64, error) {
		where := q.buildWhereExpr(ctx, true)
		if q.meta.SoftDeleteField != nil && !q.hardDelete {
			now := time.Now().UTC()
			updates := dbx.Params{q.meta.SoftDeleteField.DBName: now}
			if q.meta.UpdatedAtField != nil {
				updates[q.meta.UpdatedAtField.DBName] = now
			}
			query := q.db.Update(q.meta.Table, updates, where)
			if ctx != nil {
				query.WithContext(ctx)
			}
			res, err := query.Execute()
			if err != nil {
				return 0, wrapQueryError(ErrInvalidQuery, "query_delete", q.meta.Name, "", err)
			}
			rows, _ := res.RowsAffected()
			return rows, nil
		}

		query := q.db.Delete(q.meta.Table, where)
		if ctx != nil {
			query.WithContext(ctx)
		}
		res, err := query.Execute()
		if err != nil {
			return 0, wrapQueryError(ErrInvalidQuery, "query_delete", q.meta.Name, "", err)
		}
		rows, _ := res.RowsAffected()
		return rows, nil
	})
}

func (q *ModelQuery[T]) addWhereHash(field string, value any) *ModelQuery[T] {
	if q.err != nil {
		return q
	}
	col, err := q.resolveField(field)
	if err != nil {
		q.err = err
		return q
	}
	q.where = append(q.where, dbx.HashExp{col: value})
	return q
}

func (q *ModelQuery[T]) addCmp(field, op string, value any) *ModelQuery[T] {
	if q.err != nil {
		return q
	}
	col, err := q.resolveField(field)
	if err != nil {
		q.err = err
		return q
	}
	q.where = append(q.where, newCmpExp(col, op, value, q.nextParam()))
	return q
}

func (q *ModelQuery[T]) addOrder(field string, desc bool) *ModelQuery[T] {
	if q.err != nil {
		return q
	}
	col, err := q.resolveField(field)
	if err != nil {
		q.err = err
		return q
	}
	if desc {
		q.orderBy = append(q.orderBy, col+" DESC")
		return q
	}
	q.orderBy = append(q.orderBy, col+" ASC")
	return q
}

func (q *ModelQuery[T]) addRelationJoin(name string, left bool) *ModelQuery[T] {
	if q.err != nil {
		return q
	}
	name = strings.TrimSpace(name)
	if name == "" {
		q.err = ErrInvalidQuery.with("join_relation", q.meta.Name, "", fmt.Errorf("relation name is empty"))
		return q
	}
	rel := q.meta.Relations[name]
	if rel == nil {
		q.err = ErrRelationNotFound.with("join_relation", q.meta.Name, name, fmt.Errorf("unknown relation"))
		return q
	}
	for _, j := range q.joins {
		if j.relation == name {
			return q
		}
	}
	q.joins = append(q.joins, joinSpec{relation: name, left: left})
	return q
}

func (q *ModelQuery[T]) orderByRelation(path string, desc bool) *ModelQuery[T] {
	if q.err != nil {
		return q
	}
	col, err := q.resolveRelationColumn(path)
	if err != nil {
		q.err = err
		return q
	}
	if desc {
		q.orderBy = append(q.orderBy, col+" DESC")
	} else {
		q.orderBy = append(q.orderBy, col+" ASC")
	}
	return q
}

func (q *ModelQuery[T]) resolveRelationColumn(path string) (string, error) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) != 2 {
		return "", ErrInvalidField.with("relation_field", q.meta.Name, path, fmt.Errorf("expected Relation.Field format"))
	}
	relName := strings.TrimSpace(parts[0])
	fieldName := strings.TrimSpace(parts[1])
	if relName == "" || fieldName == "" {
		return "", ErrInvalidField.with("relation_field", q.meta.Name, path, fmt.Errorf("invalid relation field path"))
	}
	if _, ok := q.meta.Relations[relName]; !ok {
		return "", ErrRelationNotFound.with("relation_field", q.meta.Name, relName, fmt.Errorf("unknown relation"))
	}
	q.addRelationJoin(relName, false)
	if q.err != nil {
		return "", q.err
	}
	rel := q.meta.Relations[relName]
	target, err := DefaultRegistry.Resolve(reflect.New(rel.TargetType).Elem().Interface())
	if err != nil {
		return "", err
	}
	fm, err := findFieldMeta(target, fieldName)
	if err != nil {
		return "", ErrInvalidField.with("relation_field", target.Name, fieldName, err)
	}
	return target.Table + "." + fm.DBName, nil
}

func (q *ModelQuery[T]) buildSelect(ctx context.Context) (*dbx.SelectQuery, error) {
	if q.meta == nil {
		return nil, ErrInvalidModel.with("build_select", "", "", fmt.Errorf("missing metadata"))
	}
	cols := q.columnsForSelect()
	if len(cols) == 0 {
		return nil, ErrInvalidQuery.with("build_select", q.meta.Name, "", fmt.Errorf("no selected columns"))
	}
	if len(q.joins) > 0 {
		cols = qualifyColumns(q.meta.Table, cols)
	}
	sel := q.db.Select(cols...).From(q.meta.Table)
	if ctx != nil {
		sel.WithContext(ctx)
	}
	if err := q.applyRelationJoins(sel); err != nil {
		return nil, err
	}

	where := q.buildWhereExpr(ctx, true)
	if where != nil {
		sel.Where(where)
	}
	if len(q.orderBy) > 0 {
		sel.OrderBy(q.orderBy...)
	}
	if q.limit >= 0 {
		sel.Limit(q.limit)
	}
	if q.offset >= 0 {
		sel.Offset(q.offset)
	}
	return sel, nil
}

func qualifyColumns(table string, cols []string) []string {
	out := make([]string, 0, len(cols))
	for _, c := range cols {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if strings.ContainsAny(c, "(). ") || strings.Contains(c, ".") {
			out = append(out, c)
			continue
		}
		out = append(out, table+"."+c)
	}
	return out
}

func (q *ModelQuery[T]) applyRelationJoins(sel *dbx.SelectQuery) error {
	for _, js := range q.joins {
		rel := q.meta.Relations[js.relation]
		if rel == nil {
			return ErrRelationNotFound.with("join_relation", q.meta.Name, js.relation, fmt.Errorf("unknown relation"))
		}
		target, err := DefaultRegistry.Resolve(reflect.New(rel.TargetType).Elem().Interface())
		if err != nil {
			return ErrInvalidModel.with("join_relation", q.meta.Name, js.relation, err)
		}
		localFields, foreignFields, err := resolveRelationKeyMetas(q.meta, target, rel)
		if err != nil {
			return err
		}

		var on string
		switch rel.Kind {
		case RelationBelongsTo:
			on = buildJoinOn(q.meta.Table, target.Table, localFields, foreignFields)
			if js.left {
				sel.LeftJoin(target.Table, newRawExp(on, nil))
			} else {
				sel.Join(target.Table, newRawExp(on, nil))
			}
		case RelationHasMany:
			on = buildJoinOn(q.meta.Table, target.Table, localFields, foreignFields)
			if js.left {
				sel.LeftJoin(target.Table, newRawExp(on, nil))
			} else {
				sel.Join(target.Table, newRawExp(on, nil))
			}
		case RelationManyToMany:
			if rel.JoinTable == "" || rel.JoinLocalKey == "" || rel.JoinForeignKey == "" {
				return ErrInvalidModel.with("join_relation", q.meta.Name, js.relation, fmt.Errorf("join table configuration is incomplete"))
			}
			joinLocalFields := splitRelationFields(rel.JoinLocalKey)
			joinForeignFields := splitRelationFields(rel.JoinForeignKey)
			if len(joinLocalFields) != len(localFields) || len(joinForeignFields) != len(foreignFields) {
				return ErrInvalidModel.with("join_relation", q.meta.Name, js.relation, fmt.Errorf("join key count mismatch"))
			}
			joinTableOn := buildJoinOnByNames(q.meta.Table, rel.JoinTable, fieldDBNames(localFields), joinLocalFields)
			targetOn := buildJoinOnByNames(rel.JoinTable, target.Table, joinForeignFields, fieldDBNames(foreignFields))
			if js.left {
				sel.LeftJoin(rel.JoinTable, newRawExp(joinTableOn, nil))
				sel.LeftJoin(target.Table, newRawExp(targetOn, nil))
			} else {
				sel.Join(rel.JoinTable, newRawExp(joinTableOn, nil))
				sel.Join(target.Table, newRawExp(targetOn, nil))
			}
		default:
			return ErrInvalidQuery.with("join_relation", q.meta.Name, js.relation, fmt.Errorf("unsupported relation kind"))
		}
	}
	return nil
}

func (q *ModelQuery[T]) columnsForSelect() []string {
	if len(q.selectCols) > 0 {
		if len(q.excludeCols) == 0 {
			return append([]string(nil), q.selectCols...)
		}
		out := make([]string, 0, len(q.selectCols))
		for _, c := range q.selectCols {
			if _, excluded := q.excludeCols[c]; excluded {
				continue
			}
			out = append(out, c)
		}
		return out
	}

	out := make([]string, 0, len(q.meta.Fields))
	for _, f := range q.meta.Fields {
		if f.IsIgnored || f.IsWriteOnly {
			continue
		}
		if _, excluded := q.excludeCols[f.DBName]; excluded {
			continue
		}
		out = append(out, f.DBName)
	}
	sort.Strings(out)
	return out
}

func (q *ModelQuery[T]) resolveField(field string) (string, error) {
	name := strings.TrimSpace(field)
	if name == "" {
		return "", ErrInvalidField.with("resolve_field", q.meta.Name, field, fmt.Errorf("field is empty"))
	}
	if f, ok := q.meta.FieldsByGo[name]; ok {
		if f.IsIgnored {
			return "", ErrInvalidField.with("resolve_field", q.meta.Name, field, fmt.Errorf("field is ignored"))
		}
		return f.DBName, nil
	}
	if f, ok := q.meta.FieldsByDB[name]; ok {
		if f.IsIgnored {
			return "", ErrInvalidField.with("resolve_field", q.meta.Name, field, fmt.Errorf("field is ignored"))
		}
		return f.DBName, nil
	}
	return "", ErrInvalidField.with("resolve_field", q.meta.Name, field, fmt.Errorf("unknown field"))
}

func (q *ModelQuery[T]) resolveFieldMeta(field string) (*FieldMeta, error) {
	name := strings.TrimSpace(field)
	if name == "" {
		return nil, ErrInvalidField.with("resolve_field", q.meta.Name, field, fmt.Errorf("field is empty"))
	}
	if f, ok := q.meta.FieldsByGo[name]; ok {
		if f.IsIgnored {
			return nil, ErrInvalidField.with("resolve_field", q.meta.Name, field, fmt.Errorf("field is ignored"))
		}
		return f, nil
	}
	if f, ok := q.meta.FieldsByDB[name]; ok {
		if f.IsIgnored {
			return nil, ErrInvalidField.with("resolve_field", q.meta.Name, field, fmt.Errorf("field is ignored"))
		}
		return f, nil
	}
	return nil, ErrInvalidField.with("resolve_field", q.meta.Name, field, fmt.Errorf("unknown field"))
}

func (q *ModelQuery[T]) buildWhereExpr(ctx context.Context, includeSoftFilter bool) dbx.Expression {
	allWhere := make([]dbx.Expression, 0, len(q.where)+1)
	allWhere = append(allWhere, q.where...)
	fallbackCol := ""
	if q.meta.TenantField != nil {
		fallbackCol = q.meta.TenantField.DBName
	}
	if tf, ok := tenantFromContextWithDefault(ctx, fallbackCol); ok {
		col := fallbackCol
		if tf.Column != "" {
			if fm, err := q.resolveFieldMeta(tf.Column); err == nil {
				col = fm.DBName
			}
		}
		if col != "" {
			allWhere = append(allWhere, dbx.HashExp{col: tf.Value})
		}
	}
	if includeSoftFilter && q.meta.SoftDeleteField != nil {
		if q.onlyDeleted {
			allWhere = append(allWhere, newRawExp(q.meta.SoftDeleteField.DBName+" IS NOT NULL", nil))
		} else if !q.withDeleted {
			allWhere = append(allWhere, dbx.HashExp{q.meta.SoftDeleteField.DBName: nil})
		}
	}
	if len(allWhere) == 0 {
		return nil
	}
	return dbx.And(allWhere...)
}

func (q *ModelQuery[T]) operationInfo(op Operation) OperationInfo {
	info := OperationInfo{
		Operation: op,
		Model:     q.meta.Name,
		Table:     q.meta.Table,
		HasWhere:  len(q.where) > 0,
		Limit:     q.limit,
		Offset:    q.offset,
		Fields:    q.selectedFieldsForInfo(),
	}
	if len(q.joins) > 0 {
		info.Relations = make([]string, 0, len(q.joins))
		for _, j := range q.joins {
			info.Relations = append(info.Relations, j.relation)
		}
	}
	if len(q.preloads) > 0 {
		info.Preloads = make([]string, 0, len(q.preloads))
		for _, p := range q.preloads {
			info.Preloads = append(info.Preloads, strings.Join([]string(p.path), "."))
		}
	}
	if len(q.setCols) > 0 {
		for k := range q.setCols {
			info.Fields = append(info.Fields, k)
		}
		sort.Strings(info.Fields)
	}
	return info
}

func (q *ModelQuery[T]) selectedFieldsForInfo() []string {
	if len(q.selectCols) > 0 {
		return append([]string(nil), q.selectCols...)
	}
	out := make([]string, 0, len(q.meta.Fields))
	for _, f := range q.meta.Fields {
		if f.IsIgnored || f.IsWriteOnly {
			continue
		}
		out = append(out, f.DBName)
	}
	sort.Strings(out)
	return out
}

func (q *ModelQuery[T]) nextParam() string {
	q.paramSeq++
	return fmt.Sprintf("orm_p%d", q.paramSeq)
}

func (q *ModelQuery[T]) clone() *ModelQuery[T] {
	cp := *q
	if q.where != nil {
		cp.where = append([]dbx.Expression(nil), q.where...)
	}
	if q.orderBy != nil {
		cp.orderBy = append([]string(nil), q.orderBy...)
	}
	if q.joins != nil {
		cp.joins = append([]joinSpec(nil), q.joins...)
	}
	if q.selectCols != nil {
		cp.selectCols = append([]string(nil), q.selectCols...)
	}
	cp.excludeCols = map[string]struct{}{}
	for k, v := range q.excludeCols {
		cp.excludeCols[k] = v
	}
	cp.setCols = dbx.Params{}
	for k, v := range q.setCols {
		cp.setCols[k] = v
	}
	cp.preloadChunkSize = q.preloadChunkSize
	if q.preloads != nil {
		cp.preloads = make([]preloadSpec, 0, len(q.preloads))
		for _, p := range q.preloads {
			cloned := preloadSpec{
				path: append(preloadPath(nil), p.path...),
				opts: preloadOptions{
					withDeleted: p.opts.withDeleted,
					limit:       p.opts.limit,
				},
			}
			if p.opts.orderBy != nil {
				cloned.opts.orderBy = append([]string(nil), p.opts.orderBy...)
			}
			if p.opts.orderBySafe != nil {
				cloned.opts.orderBySafe = append([]preloadOrderField(nil), p.opts.orderBySafe...)
			}
			if p.opts.whereEq != nil {
				cloned.opts.whereEq = append([]preloadEqCond(nil), p.opts.whereEq...)
			}
			if p.opts.whereIn != nil {
				cloned.opts.whereIn = append([]preloadInCond(nil), p.opts.whereIn...)
			}
			if p.opts.whereLike != nil {
				cloned.opts.whereLike = append([]preloadLikeCond(nil), p.opts.whereLike...)
			}
			if p.opts.whereNull != nil {
				cloned.opts.whereNull = append([]string(nil), p.opts.whereNull...)
			}
			if p.opts.whereNotNil != nil {
				cloned.opts.whereNotNil = append([]string(nil), p.opts.whereNotNil...)
			}
			if p.opts.whereCmp != nil {
				cloned.opts.whereCmp = append([]preloadCmpCond(nil), p.opts.whereCmp...)
			}
			if p.opts.whereExpr != nil {
				cloned.opts.whereExpr = append([]dbx.Expression(nil), p.opts.whereExpr...)
			}
			cp.preloads = append(cp.preloads, cloned)
		}
	}
	return &cp
}

func (q *ModelQuery[T]) applyPreloadsToOne(ctx context.Context, out *T) error {
	if len(q.preloads) == 0 || out == nil {
		return nil
	}
	rowVal := reflect.ValueOf(out).Elem()
	if err := q.applyPreloadsReflect(ctx, q.meta, []reflect.Value{rowVal}, q.preloads); err != nil {
		return err
	}
	return nil
}

func (q *ModelQuery[T]) applyPreloadsToSlice(ctx context.Context, rows []T) error {
	if len(q.preloads) == 0 || len(rows) == 0 {
		return nil
	}
	rowVals := make([]reflect.Value, 0, len(rows))
	for i := range rows {
		rowVals = append(rowVals, reflect.ValueOf(&rows[i]).Elem())
	}
	return q.applyPreloadsReflect(ctx, q.meta, rowVals, q.preloads)
}

func (q *ModelQuery[T]) applyPreloadsReflect(ctx context.Context, meta *ModelMeta, rows []reflect.Value, specs []preloadSpec) error {
	if len(specs) == 0 || len(rows) == 0 {
		return nil
	}
	grouped := map[string][]preloadSpec{}
	for _, spec := range specs {
		if len(spec.path) == 0 {
			continue
		}
		head := spec.path[0]
		grouped[head] = append(grouped[head], spec)
	}

	for relName, relSpecs := range grouped {
		rel := meta.Relations[relName]
		if rel == nil {
			return ErrRelationNotFound.with("preload", meta.Name, relName, fmt.Errorf("unknown relation"))
		}
		targetMeta, err := DefaultRegistry.Resolve(reflect.New(rel.TargetType).Elem().Interface())
		if err != nil {
			return err
		}

		relOpts, childSpecs := splitPreloadSpecs(relSpecs)
		var children []reflect.Value
		switch rel.Kind {
		case RelationBelongsTo:
			children, err = q.preloadBelongsToReflect(ctx, meta, rows, rel, targetMeta, relOpts)
		case RelationHasMany:
			children, err = q.preloadHasManyReflect(ctx, meta, rows, rel, targetMeta, relOpts)
		case RelationManyToMany:
			children, err = q.preloadManyToManyReflect(ctx, meta, rows, rel, targetMeta, relOpts)
		default:
			err = ErrInvalidQuery.with("preload", meta.Name, relName, fmt.Errorf("unsupported relation kind"))
		}
		if err != nil {
			return err
		}

		if len(childSpecs) > 0 && len(children) > 0 {
			if err := q.applyPreloadsReflect(ctx, targetMeta, children, childSpecs); err != nil {
				return err
			}
		}
	}
	return nil
}

func (q *ModelQuery[T]) preloadBelongsToReflect(ctx context.Context, meta *ModelMeta, rows []reflect.Value, rel *RelationMeta, target *ModelMeta, opts preloadOptions) ([]reflect.Value, error) {
	localFields, foreignFields, err := resolveRelationKeyMetas(meta, target, rel)
	if err != nil {
		return nil, err
	}
	keys := collectDistinctRelationTuples(rows, localFields)
	if len(keys) == 0 {
		return nil, nil
	}

	var items []reflect.Value
	if len(foreignFields) == 1 {
		scalarKeys := make([]any, 0, len(keys))
		for _, k := range keys {
			scalarKeys = append(scalarKeys, k.values[0])
		}
		items, err = q.fetchRelationRows(ctx, target, foreignFields[0].DBName, scalarKeys, opts)
	} else {
		items, err = q.fetchRelationRowsComposite(ctx, target, foreignFields, keys, opts)
	}
	if err != nil {
		return nil, err
	}
	byKey := make(map[any]reflect.Value, len(items))
	for i := range items {
		key, ok := relationTupleKeyFromRow(items[i], foreignFields)
		if !ok {
			continue
		}
		byKey[key] = items[i]
	}

	children := make([]reflect.Value, 0, len(items))
	for i := range rows {
		rowv := rows[i]
		key, ok := relationTupleKeyFromRow(rowv, localFields)
		if !ok {
			continue
		}
		targetVal, ok := byKey[key]
		if !ok {
			continue
		}
		if err := assignRelationValue(rowv.FieldByIndex(rel.FieldIndex), targetVal); err != nil {
			return nil, err
		}
		children = append(children, targetVal)
	}
	return children, nil
}

func (q *ModelQuery[T]) preloadHasManyReflect(ctx context.Context, meta *ModelMeta, rows []reflect.Value, rel *RelationMeta, target *ModelMeta, opts preloadOptions) ([]reflect.Value, error) {
	localFields, foreignFields, err := resolveRelationKeyMetas(meta, target, rel)
	if err != nil {
		return nil, err
	}
	keys := collectDistinctRelationTuples(rows, localFields)
	if len(keys) == 0 {
		return nil, nil
	}

	var items []reflect.Value
	if len(foreignFields) == 1 {
		scalarKeys := make([]any, 0, len(keys))
		for _, k := range keys {
			scalarKeys = append(scalarKeys, k.values[0])
		}
		items, err = q.fetchRelationRows(ctx, target, foreignFields[0].DBName, scalarKeys, opts)
	} else {
		items, err = q.fetchRelationRowsComposite(ctx, target, foreignFields, keys, opts)
	}
	if err != nil {
		return nil, err
	}
	grouped := make(map[any][]reflect.Value)
	for i := range items {
		key, ok := relationTupleKeyFromRow(items[i], foreignFields)
		if !ok {
			continue
		}
		grouped[key] = append(grouped[key], items[i])
	}

	children := make([]reflect.Value, 0, len(items))
	for i := range rows {
		rowv := rows[i]
		key, ok := relationTupleKeyFromRow(rowv, localFields)
		if !ok {
			continue
		}
		relField := rowv.FieldByIndex(rel.FieldIndex)
		values := grouped[key]
		if err := assignRelationSlice(relField, values); err != nil {
			return nil, err
		}
		for j := 0; j < relField.Len(); j++ {
			elem := relField.Index(j)
			if elem.Kind() == reflect.Pointer {
				if elem.IsNil() {
					continue
				}
				elem = elem.Elem()
			}
			if elem.IsValid() && elem.Kind() == reflect.Struct {
				children = append(children, elem)
			}
		}
	}
	return children, nil
}

func (q *ModelQuery[T]) preloadManyToManyReflect(ctx context.Context, meta *ModelMeta, rows []reflect.Value, rel *RelationMeta, target *ModelMeta, opts preloadOptions) ([]reflect.Value, error) {
	localField, ok := meta.FieldsByGo[rel.LocalField]
	if !ok {
		return nil, ErrInvalidField.with("preload_many_to_many", meta.Name, rel.LocalField, fmt.Errorf("local field not found"))
	}
	if rel.JoinTable == "" || rel.JoinLocalKey == "" || rel.JoinForeignKey == "" {
		return nil, ErrInvalidModel.with("preload_many_to_many", meta.Name, rel.Name, fmt.Errorf("join table configuration is incomplete"))
	}

	localKeys := collectDistinctFieldValuesReflect(rows, localField)
	if len(localKeys) == 0 {
		return nil, nil
	}

	pairs, err := q.fetchJoinPairs(ctx, rel.JoinTable, rel.JoinLocalKey, rel.JoinForeignKey, localKeys)
	if err != nil {
		return nil, err
	}
	if len(pairs) == 0 {
		return nil, nil
	}

	targetForeign, err := findFieldMeta(target, rel.ForeignRef)
	if err != nil {
		return nil, err
	}
	targetKeySet := map[any]struct{}{}
	targetKeys := make([]any, 0, len(pairs))
	for _, p := range pairs {
		if _, seen := targetKeySet[p.foreign]; seen {
			continue
		}
		targetKeySet[p.foreign] = struct{}{}
		targetKeys = append(targetKeys, p.foreign)
	}
	items, err := q.fetchRelationRows(ctx, target, targetForeign.DBName, targetKeys, opts)
	if err != nil {
		return nil, err
	}

	targetByKey := make(map[any]reflect.Value, len(items))
	for i := range items {
		fv := items[i].FieldByIndex(targetForeign.Index)
		if !fv.IsValid() || !fv.CanInterface() {
			continue
		}
		targetByKey[fv.Interface()] = items[i]
	}
	sourceToTargets := make(map[any][]reflect.Value)
	for _, p := range pairs {
		if tv, ok := targetByKey[p.foreign]; ok {
			sourceToTargets[p.local] = append(sourceToTargets[p.local], tv)
		}
	}

	children := make([]reflect.Value, 0, len(items))
	for i := range rows {
		rowv := rows[i]
		localVal := rowv.FieldByIndex(localField.Index)
		if !localVal.IsValid() || !localVal.CanInterface() {
			continue
		}
		relField := rowv.FieldByIndex(rel.FieldIndex)
		values := sourceToTargets[localVal.Interface()]
		if err := assignRelationSlice(relField, values); err != nil {
			return nil, err
		}
		for j := 0; j < relField.Len(); j++ {
			elem := relField.Index(j)
			if elem.Kind() == reflect.Pointer {
				if elem.IsNil() {
					continue
				}
				elem = elem.Elem()
			}
			if elem.IsValid() && elem.Kind() == reflect.Struct {
				children = append(children, elem)
			}
		}
	}
	return children, nil
}

type joinPair struct {
	local   any
	foreign any
}

type relationTuple struct {
	key    string
	values []any
}

func (q *ModelQuery[T]) fetchJoinPairs(ctx context.Context, table, localKey, foreignKey string, localValues []any) ([]joinPair, error) {
	out := make([]joinPair, 0)
	seen := map[string]struct{}{}
	for _, chunk := range chunkValues(localValues, q.effectivePreloadChunkSize(len(localValues))) {
		sel := q.db.Select(localKey, foreignKey).From(table).Where(dbx.In(localKey, chunk))
		if ctx != nil {
			sel.WithContext(ctx)
		}
		rows, err := sel.Rows()
		if err != nil {
			return nil, ErrInvalidQuery.with("preload_join_pairs", table, "", err)
		}
		for rows.Next() {
			var lv any
			var fv any
			if err := rows.Scan(&lv, &fv); err != nil {
				_ = rows.Close()
				return nil, ErrInvalidQuery.with("preload_join_pairs", table, "", err)
			}
			key := fmt.Sprintf("%v::%v", lv, fv)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, joinPair{local: lv, foreign: fv})
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, ErrInvalidQuery.with("preload_join_pairs", table, "", err)
		}
		_ = rows.Close()
	}
	return out, nil
}

func (q *ModelQuery[T]) fetchRelationRows(ctx context.Context, target *ModelMeta, foreignDB string, keys []any, opts preloadOptions) ([]reflect.Value, error) {
	out := make([]reflect.Value, 0)
	for _, chunk := range chunkValues(keys, q.effectivePreloadChunkSize(len(keys))) {
		sel := q.db.Select(selectColumns(target)...).From(target.Table).Where(dbx.In(foreignDB, chunk))
		if target.SoftDeleteField != nil && !opts.withDeleted {
			sel.AndWhere(dbx.HashExp{target.SoftDeleteField.DBName: nil})
		}
		if err := q.applyRelationSelectOptions(sel, target, opts); err != nil {
			return nil, err
		}
		if ctx != nil {
			sel.WithContext(ctx)
		}

		sliceType := reflect.SliceOf(target.Type)
		dest := reflect.New(sliceType)
		if err := sel.All(dest.Interface()); err != nil {
			return nil, ErrInvalidQuery.with("preload_fetch", target.Name, foreignDB, err)
		}
		for i := 0; i < dest.Elem().Len(); i++ {
			v := dest.Elem().Index(i)
			if err := decodeModelValue(target, v); err != nil {
				return nil, err
			}
			out = append(out, v)
		}
	}
	return out, nil
}

func (q *ModelQuery[T]) fetchRelationRowsComposite(ctx context.Context, target *ModelMeta, foreignFields []*FieldMeta, keys []relationTuple, opts preloadOptions) ([]reflect.Value, error) {
	out := make([]reflect.Value, 0)
	for _, chunk := range chunkRelationTuples(keys, q.effectivePreloadChunkSize(len(keys))) {
		sel := q.db.Select(selectColumns(target)...).From(target.Table)

		var ors []string
		params := dbx.Params{}
		for _, tuple := range chunk {
			var ands []string
			for i, fm := range foreignFields {
				p := q.nextParam()
				ands = append(ands, fm.DBName+" = {:"+p+"}")
				params[p] = tuple.values[i]
			}
			ors = append(ors, "("+strings.Join(ands, " AND ")+")")
		}
		if len(ors) > 0 {
			sel.Where(newRawExp(strings.Join(ors, " OR "), params))
		}
		if target.SoftDeleteField != nil && !opts.withDeleted {
			sel.AndWhere(dbx.HashExp{target.SoftDeleteField.DBName: nil})
		}
		if err := q.applyRelationSelectOptions(sel, target, opts); err != nil {
			return nil, err
		}
		if ctx != nil {
			sel.WithContext(ctx)
		}
		sliceType := reflect.SliceOf(target.Type)
		dest := reflect.New(sliceType)
		if err := sel.All(dest.Interface()); err != nil {
			return nil, ErrInvalidQuery.with("preload_fetch", target.Name, strings.Join(fieldDBNames(foreignFields), ","), err)
		}
		for i := 0; i < dest.Elem().Len(); i++ {
			v := dest.Elem().Index(i)
			if err := decodeModelValue(target, v); err != nil {
				return nil, err
			}
			out = append(out, v)
		}
	}
	return out, nil
}

func chunkValues(values []any, size int) [][]any {
	if len(values) == 0 {
		return nil
	}
	if size <= 0 || len(values) <= size {
		return [][]any{values}
	}
	out := make([][]any, 0, (len(values)+size-1)/size)
	for i := 0; i < len(values); i += size {
		j := i + size
		if j > len(values) {
			j = len(values)
		}
		out = append(out, values[i:j])
	}
	return out
}

func chunkRelationTuples(values []relationTuple, size int) [][]relationTuple {
	if len(values) == 0 {
		return nil
	}
	if size <= 0 || len(values) <= size {
		return [][]relationTuple{values}
	}
	out := make([][]relationTuple, 0, (len(values)+size-1)/size)
	for i := 0; i < len(values); i += size {
		j := i + size
		if j > len(values) {
			j = len(values)
		}
		out = append(out, values[i:j])
	}
	return out
}

func (q *ModelQuery[T]) effectivePreloadChunkSize(total int) int {
	size := q.preloadChunkSize
	if size <= 0 {
		size = 500
	}
	// Use larger chunks for very large cardinality preloads.
	if total >= 10000 && size < 2000 {
		size = 2000
	} else if total >= 3000 && size < 1000 {
		size = 1000
	}
	return size
}

func collectDistinctFieldValuesReflect(rows []reflect.Value, field *FieldMeta) []any {
	out := make([]any, 0, len(rows))
	seen := map[any]struct{}{}
	for i := range rows {
		rowv := rows[i]
		if !rowv.IsValid() || rowv.Kind() != reflect.Struct {
			continue
		}
		fv := rowv.FieldByIndex(field.Index)
		if !fv.IsValid() || !fv.CanInterface() {
			continue
		}
		key := fv.Interface()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func collectDistinctRelationTuples(rows []reflect.Value, fields []*FieldMeta) []relationTuple {
	out := make([]relationTuple, 0, len(rows))
	seen := map[string]struct{}{}
	for i := range rows {
		key, vals, ok := relationTupleFromRow(rows[i], fields)
		if !ok {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, relationTuple{key: key, values: vals})
	}
	return out
}

func relationTupleFromRow(row reflect.Value, fields []*FieldMeta) (string, []any, bool) {
	if !row.IsValid() || row.Kind() != reflect.Struct {
		return "", nil, false
	}
	vals := make([]any, 0, len(fields))
	for _, f := range fields {
		fv := row.FieldByIndex(f.Index)
		if !fv.IsValid() || !fv.CanInterface() {
			return "", nil, false
		}
		vals = append(vals, fv.Interface())
	}
	return relationTupleKey(vals), vals, true
}

func relationTupleKeyFromRow(row reflect.Value, fields []*FieldMeta) (string, bool) {
	key, _, ok := relationTupleFromRow(row, fields)
	return key, ok
}

func relationTupleKey(vals []any) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = fmt.Sprintf("%v", v)
	}
	return strings.Join(parts, "|")
}

func resolveRelationKeyMetas(src, target *ModelMeta, rel *RelationMeta) ([]*FieldMeta, []*FieldMeta, error) {
	localNames := splitRelationFields(rel.LocalField)
	foreignNames := splitRelationFields(rel.ForeignRef)
	if len(localNames) == 0 || len(foreignNames) == 0 {
		return nil, nil, ErrInvalidModel.with("relation_keys", src.Name, rel.Name, fmt.Errorf("relation keys are empty"))
	}
	if len(localNames) != len(foreignNames) {
		return nil, nil, ErrInvalidModel.with("relation_keys", src.Name, rel.Name, fmt.Errorf("local/foreign key count mismatch"))
	}
	localFields := make([]*FieldMeta, 0, len(localNames))
	foreignFields := make([]*FieldMeta, 0, len(foreignNames))
	for i := range localNames {
		lf, err := findFieldMeta(src, localNames[i])
		if err != nil {
			return nil, nil, ErrInvalidField.with("relation_keys", src.Name, localNames[i], err)
		}
		ff, err := findFieldMeta(target, foreignNames[i])
		if err != nil {
			return nil, nil, ErrInvalidField.with("relation_keys", target.Name, foreignNames[i], err)
		}
		localFields = append(localFields, lf)
		foreignFields = append(foreignFields, ff)
	}
	return localFields, foreignFields, nil
}

func splitRelationFields(in string) []string {
	parts := strings.FieldsFunc(in, func(r rune) bool { return r == ',' || r == '|' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func buildJoinOn(leftTable, rightTable string, localFields, foreignFields []*FieldMeta) string {
	return buildJoinOnByNames(leftTable, rightTable, fieldDBNames(localFields), fieldDBNames(foreignFields))
}

func buildJoinOnByNames(leftTable, rightTable string, leftCols, rightCols []string) string {
	parts := make([]string, 0, len(leftCols))
	for i := 0; i < len(leftCols) && i < len(rightCols); i++ {
		parts = append(parts, fmt.Sprintf("%s.%s = %s.%s", leftTable, leftCols[i], rightTable, rightCols[i]))
	}
	return strings.Join(parts, " AND ")
}

func fieldDBNames(in []*FieldMeta) []string {
	out := make([]string, 0, len(in))
	for _, f := range in {
		out = append(out, f.DBName)
	}
	return out
}

func (q *ModelQuery[T]) applyRelationSelectOptions(sel *dbx.SelectQuery, target *ModelMeta, opts preloadOptions) error {
	for _, cond := range opts.whereEq {
		fm, err := findFieldMeta(target, cond.field)
		if err != nil {
			return ErrInvalidField.with("preload_where_eq", target.Name, cond.field, err)
		}
		sel.AndWhere(dbx.HashExp{fm.DBName: cond.value})
	}
	for _, cond := range opts.whereIn {
		fm, err := findFieldMeta(target, cond.field)
		if err != nil {
			return ErrInvalidField.with("preload_where_in", target.Name, cond.field, err)
		}
		sel.AndWhere(dbx.In(fm.DBName, cond.values))
	}
	for _, cond := range opts.whereLike {
		fm, err := findFieldMeta(target, cond.field)
		if err != nil {
			return ErrInvalidField.with("preload_where_like", target.Name, cond.field, err)
		}
		sel.AndWhere(dbx.Like(fm.DBName, cond.value))
	}
	for _, field := range opts.whereNull {
		fm, err := findFieldMeta(target, field)
		if err != nil {
			return ErrInvalidField.with("preload_where_null", target.Name, field, err)
		}
		sel.AndWhere(newRawExp(fm.DBName+" IS NULL", nil))
	}
	for _, field := range opts.whereNotNil {
		fm, err := findFieldMeta(target, field)
		if err != nil {
			return ErrInvalidField.with("preload_where_not_null", target.Name, field, err)
		}
		sel.AndWhere(newRawExp(fm.DBName+" IS NOT NULL", nil))
	}
	for _, cond := range opts.whereCmp {
		fm, err := findFieldMeta(target, cond.field)
		if err != nil {
			return ErrInvalidField.with("preload_where_cmp", target.Name, cond.field, err)
		}
		sel.AndWhere(newCmpExp(fm.DBName, cond.op, cond.value, q.nextParam()))
	}
	for _, w := range opts.whereExpr {
		if w != nil {
			sel.AndWhere(w)
		}
	}
	if len(opts.orderBySafe) > 0 {
		for _, ord := range opts.orderBySafe {
			fm, err := findFieldMeta(target, ord.field)
			if err != nil {
				return ErrInvalidField.with("preload_order_by_field", target.Name, ord.field, err)
			}
			sel.OrderBy(fm.DBName + " " + string(ord.dir))
		}
	}
	if len(opts.orderBy) > 0 {
		sel.OrderBy(opts.orderBy...)
	}
	if opts.limit >= 0 {
		sel.Limit(opts.limit)
	}
	return nil
}

type preloadPath []string

func splitPreloadSpecs(specs []preloadSpec) (preloadOptions, []preloadSpec) {
	merged := preloadOptions{limit: -1}
	children := make([]preloadSpec, 0)
	for _, spec := range specs {
		if len(spec.path) == 1 {
			if spec.opts.withDeleted {
				merged.withDeleted = true
			}
			if len(spec.opts.orderBy) > 0 {
				merged.orderBy = append(merged.orderBy, spec.opts.orderBy...)
			}
			if len(spec.opts.orderBySafe) > 0 {
				merged.orderBySafe = append(merged.orderBySafe, spec.opts.orderBySafe...)
			}
			if len(spec.opts.whereEq) > 0 {
				merged.whereEq = append(merged.whereEq, spec.opts.whereEq...)
			}
			if len(spec.opts.whereIn) > 0 {
				merged.whereIn = append(merged.whereIn, spec.opts.whereIn...)
			}
			if len(spec.opts.whereLike) > 0 {
				merged.whereLike = append(merged.whereLike, spec.opts.whereLike...)
			}
			if len(spec.opts.whereNull) > 0 {
				merged.whereNull = append(merged.whereNull, spec.opts.whereNull...)
			}
			if len(spec.opts.whereNotNil) > 0 {
				merged.whereNotNil = append(merged.whereNotNil, spec.opts.whereNotNil...)
			}
			if len(spec.opts.whereCmp) > 0 {
				merged.whereCmp = append(merged.whereCmp, spec.opts.whereCmp...)
			}
			if len(spec.opts.whereExpr) > 0 {
				merged.whereExpr = append(merged.whereExpr, spec.opts.whereExpr...)
			}
			if spec.opts.limit >= 0 {
				merged.limit = spec.opts.limit
			}
			continue
		}
		children = append(children, preloadSpec{
			path: append(preloadPath(nil), spec.path[1:]...),
			opts: spec.opts,
		})
	}
	return merged, children
}

func parsePreloadPath(name string) (preloadPath, error) {
	parts := strings.Split(name, ".")
	out := make(preloadPath, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, ErrInvalidQuery.with("preload", "", name, fmt.Errorf("invalid preload path"))
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, ErrInvalidQuery.with("preload", "", name, fmt.Errorf("invalid preload path"))
	}
	return out, nil
}

func assignRelationValue(field reflect.Value, value reflect.Value) error {
	if !field.CanSet() {
		return ErrInvalidQuery.with("preload_assign", "", "", fmt.Errorf("relation field is not settable"))
	}
	switch field.Kind() {
	case reflect.Pointer:
		ptr := reflect.New(value.Type())
		ptr.Elem().Set(value)
		field.Set(ptr)
		return nil
	case reflect.Struct:
		field.Set(value)
		return nil
	default:
		return ErrInvalidQuery.with("preload_assign", "", "", fmt.Errorf("unsupported relation field kind: %s", field.Kind()))
	}
}

func assignRelationSlice(field reflect.Value, values []reflect.Value) error {
	if !field.CanSet() {
		return ErrInvalidQuery.with("preload_assign", "", "", fmt.Errorf("relation slice is not settable"))
	}
	if field.Kind() != reflect.Slice {
		return ErrInvalidQuery.with("preload_assign", "", "", fmt.Errorf("relation field is not a slice"))
	}
	slice := reflect.MakeSlice(field.Type(), 0, len(values))
	for i := range values {
		v := values[i]
		elemType := field.Type().Elem()
		if elemType.Kind() == reflect.Pointer {
			ptr := reflect.New(v.Type())
			ptr.Elem().Set(v)
			slice = reflect.Append(slice, ptr)
			continue
		}
		slice = reflect.Append(slice, v)
	}
	field.Set(slice)
	return nil
}

type cmpExp struct {
	col   string
	op    string
	value any
	name  string
}

func newCmpExp(col, op string, value any, name string) dbx.Expression {
	return cmpExp{col: col, op: op, value: value, name: name}
}

func (e cmpExp) Build(_ *dbx.DB, params dbx.Params) string {
	if params == nil {
		params = dbx.Params{}
	}
	params[e.name] = e.value
	return e.col + " " + e.op + " {:" + e.name + "}"
}

type rawExp struct {
	sql    string
	params dbx.Params
}

func newRawExp(sql string, params dbx.Params) dbx.Expression {
	return rawExp{sql: sql, params: params}
}

func (e rawExp) Build(_ *dbx.DB, params dbx.Params) string {
	for k, v := range e.params {
		params[k] = v
	}
	return e.sql
}
