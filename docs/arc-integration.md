# Integration With arc

`orm` is designed to work near `arc`, but not depend on it.

## Goals

- reuse one model type in persistence (`orm`) and HTTP/API layer (`arc`)
- keep nullable/optional behavior consistent
- map ORM errors to HTTP errors predictably
- avoid ORM package coupling to HTTP semantics

## Shared Model Types

Use one struct definition with stable tags and metadata conventions:

```go
type User struct {
	ID        int64      `db:"id,pk" json:"id"`
	Email     string     `db:"email" json:"email"`
	Name      string     `db:"name" json:"name"`
	DeletedAt *time.Time `db:"deleted_at,soft_delete" json:"-"`
	CreatedAt time.Time  `db:"created_at,created_at" json:"created_at"`
	UpdatedAt time.Time  `db:"updated_at,updated_at" json:"updated_at"`
}
```

`orm` reads `db` metadata for persistence.
`arc` can use `json` metadata for transport contracts.

## Shared Type Metadata

For tooling that needs transport-neutral model introspection, use:

```go
meta, err := orm.TypeMetadata[User]()
if err != nil {
	return err
}
_ = meta
```

This keeps reflection logic centralized and avoids duplicated parsers in adjacent layers.

## Error Mapping Boundary

Keep HTTP mapping outside `orm` (in `arc` adapter/service layer):

```go
func mapORMError(err error) int {
	switch {
	case errors.Is(err, orm.ErrNotFound):
		return 404
	case errors.Is(err, orm.ErrConflict):
		return 409
	case errors.Is(err, orm.ErrInvalidModel),
		errors.Is(err, orm.ErrInvalidField),
		errors.Is(err, orm.ErrInvalidQuery):
		return 400
	default:
		return 500
	}
}
```

This preserves `orm` as transport-agnostic and keeps API concerns in `arc`.

## Repository + Handler Split

Recommended boundary:

1. repository/service uses `orm` (`Insert`, `Update`, `Query`, `WithTx`)
2. HTTP handlers/controllers convert request/response DTOs
3. error-to-status mapping happens only in API layer

## Migration Path

1. keep existing `arc` handlers
2. move persistence logic from ad-hoc SQL to typed `orm` repositories
3. standardize error mapping with `errors.Is` checks
4. optionally expose `TypeMetadata[T]` to codegen/schema tooling
