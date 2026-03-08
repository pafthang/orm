# Model Guide

## Field Mapping

Use `db` tag to map fields:

```go
Field string `db:"column_name"`
```

Common flags:
- `pk`
- `nullable`
- `readonly`
- `writeonly`
- `default`
- `generated`
- `created_at`
- `updated_at`
- `soft_delete`

Example:

```go
type User struct {
	ID        int64      `db:"id,pk"`
	Email     string     `db:"email"`
	Password  string     `db:"password,writeonly"`
	DeletedAt *time.Time `db:"deleted_at,soft_delete"`
	CreatedAt time.Time  `db:"created_at,created_at"`
	UpdatedAt time.Time  `db:"updated_at,updated_at"`
}
```

## Table Name

Priority order:
1. `ModelConfig.Table`
2. `TableName() string`
3. naming strategy fallback

## Registration

Lazy registration happens on first usage. You can register explicitly:

```go
_, err := orm.Register[User](orm.ModelConfig{Table: "app_users"})
```

## Naming and Extenders

- Global naming via `NewRegistry(WithNamingStrategy(...))`
- Global custom metadata behavior via `WithFieldExtender(...)`
- Per-model custom metadata behavior via `ModelConfig.Extenders`

## Field-Level Codecs

Use `ModelConfig.FieldCodecs` for custom field conversion when writing and reading:

```go
_, err := orm.Register[User](orm.ModelConfig{
	FieldCodecs: map[string]orm.Codec{
		"Secret": myCodec,
	},
})
```

Keys can be Go field name or DB column name.

## Tenant-Aware Metadata

`ModelConfig` supports tenant metadata:
- `TenantField`
- `RequireTenant`

These are used by tenant plugin and routing policies.

## Shared Type Metadata

`orm` exposes transport-neutral metadata through:

```go
meta, err := orm.TypeMetadata[User]()
```

This data is useful for external tooling without coupling to ORM internals.

## Advanced Patch Types

- `Optional[T]`: set/absent
- `Nullable[T]`: set(value)/set(NULL)/absent

Use with `UpdatePatchByPK`.
