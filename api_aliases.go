package orm

import "context"

// QueryScope mutates/returns query chain for top-level helpers.
type QueryScope[T any] func(*ModelQuery[T]) *ModelQuery[T]

// FindByPK is an alias for ByPK.
func FindByPK[T any](ctx context.Context, db DB, id any) (*T, error) {
	return ByPK[T](ctx, db, id)
}

// GetByPK is an alias for ByPK.
func GetByPK[T any](ctx context.Context, db DB, id any) (*T, error) {
	return ByPK[T](ctx, db, id)
}

// FindAll fetches all rows for model T (excluding soft-deleted by default).
func FindAll[T any](ctx context.Context, db DB) ([]T, error) {
	return Query[T](db).All(ctx)
}

// List is a top-level helper for paginated query results.
func List[T any](ctx context.Context, db DB, page, perPage int64) (*ListResult[T], error) {
	return Query[T](db).List(ctx, page, perPage)
}

// Exists checks whether at least one row exists for model T with optional query scopes.
func Exists[T any](ctx context.Context, db DB, scopes ...QueryScope[T]) (bool, error) {
	q := Query[T](db)
	q = applyScopes(q, scopes...)
	return q.Exists(ctx)
}

// UpdateAll performs bulk update with optional query scopes and returns affected rows.
func UpdateAll[T any](ctx context.Context, db DB, set map[string]any, scopes ...QueryScope[T]) (int64, error) {
	q := Query[T](db)
	q = applyScopes(q, scopes...)
	for field, value := range set {
		q = q.Set(field, value)
	}
	return q.Update(ctx)
}

// DeleteAll performs bulk delete with optional query scopes and returns affected rows.
func DeleteAll[T any](ctx context.Context, db DB, scopes ...QueryScope[T]) (int64, error) {
	q := Query[T](db)
	q = applyScopes(q, scopes...)
	return q.Delete(ctx)
}

func applyScopes[T any](q *ModelQuery[T], scopes ...QueryScope[T]) *ModelQuery[T] {
	for _, scope := range scopes {
		if scope == nil {
			continue
		}
		next := scope(q)
		if next != nil {
			q = next
		}
	}
	return q
}
