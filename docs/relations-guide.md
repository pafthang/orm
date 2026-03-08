# Relations Guide

## Supported Relation Kinds

- `belongs_to`
- `has_many`
- `many_to_many`

## Preload

```go
rows, err := orm.Query[User](db).
	Preload("Profile").
	Preload("Roles").
	All(ctx)
```

Nested preload:

```go
rows, err := orm.Query[User](db).
	Preload("Profile.Organization").
	All(ctx)
```

## Relation Metadata Sources

- inferred by conventions
- `orm` tag overrides
- `ModelConfig.Relations`

Tag keys:
- `rel`
- `local`
- `foreign`
- `join_table`
- `join_local`
- `join_foreign`

## Composite Relation Keys

Composite keys use field lists in `local`/`foreign`:

```go
Items []Item `orm:"rel=has_many,local=TenantID|UserID,foreign=TenantID|UserID"`
```

Use `|` as list separator inside relation option values.

## Preload Options

Examples:
- `PreloadWithDeleted()`
- `PreloadWhereEq/In/Like/...`
- `PreloadOrderByField(...)`
- `PreloadLimit(...)`
- `PreloadConfigure(func(*PreloadScope){...})`

## Join by Relation

```go
rows, err := orm.Query[User](db).
	JoinRelation("Profile").
	WhereRelationEq("Profile.Status", "active").
	All(ctx)
```

Also available: `LeftJoinRelation`.

## Notes

- Preload is explicit (no implicit lazy proxies).
- Large relation graphs are supported, but you should benchmark high-cardinality paths in your environment.
