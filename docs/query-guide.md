# Query Guide

## Basic Query

```go
rows, err := orm.Query[User](db).
	WhereEq("status", "active").
	OrderByDesc("id").
	Page(1, 20).
	All(ctx)
```

## Filters

Supported filters:
- `WhereEq`
- `WhereNotEq`
- `WhereIn`
- `WhereNull`
- `WhereNotNull`
- `WhereLike`
- `WhereGT`
- `WhereGTE`
- `WhereLT`
- `WhereLTE`

Relation-aware filters:
- `WhereRelationEq`
- `WhereRelationIn`
- `WhereRelationLike`

## Sorting and Pagination

- `OrderBy`
- `OrderByDesc`
- `Limit`
- `Offset`
- `Page`
- `List`

## Selection

- `Select("FieldA", "FieldB")`
- `Exclude("FieldX")`

## Read APIs

- `All(ctx)`
- `One(ctx)`
- `First(ctx)`
- `FindOne(ctx)` (strict one-row expectation)
- `Count(ctx)`
- `Exists(ctx)`

## Bulk Write via Query

```go
affected, err := orm.Query[User](db).
	WhereEq("status", "pending").
	Set("status", "active").
	Update(ctx)
```

Bulk delete:

```go
affected, err := orm.Query[User](db).
	WhereEq("status", "inactive").
	Delete(ctx)
```

## Unsafe Boundary

Use raw SQL predicates only when metadata-aware API is insufficient:

- `UnsafeWhereExpr(...)`

Keep unsafe expressions localized and covered by tests.

## Soft Delete Behavior

Default query excludes soft-deleted rows.

Use:
- `WithDeleted()` to include all
- `OnlyDeleted()` to fetch only deleted rows
