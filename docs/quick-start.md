# Quick Start

This guide shows the shortest path from model definition to real CRUD/query calls.

## 1. Define a Model

```go
type User struct {
	ID    int64  `db:"id,pk"`
	Email string `db:"email"`
	Name  string `db:"name"`
}

func (User) TableName() string { return "users" }
```

## 2. Open DB and Create Schema

```go
db, err := dbx.Open("sqlite", ":memory:")
if err != nil { return err }
defer db.Close()

_, err = db.NewQuery(`
	CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT NOT NULL,
		name TEXT NOT NULL
	);
`).Execute()
if err != nil { return err }
```

## 3. Insert and Fetch by PK

```go
ctx := context.Background()

u := User{Email: "a@example.com", Name: "Alice"}
if err := orm.Insert(ctx, db, &u); err != nil {
	return err
}

row, err := orm.ByPK[User](ctx, db, u.ID)
if err != nil {
	return err
}
_ = row
```

## 4. Query API

```go
rows, err := orm.Query[User](db).
	WhereLike("Email", "%@example.com").
	OrderByDesc("ID").
	Page(1, 20).
	All(ctx)
if err != nil {
	return err
}
_ = rows
```

## 5. Update and Delete

```go
u.Name = "Alice Updated"
if err := orm.Update(ctx, db, &u); err != nil {
	return err
}

if err := orm.DeleteByPK[User](ctx, db, u.ID); err != nil {
	return err
}
```

## Next

- Model and metadata options: [Model Guide](model-guide.md)
- Filters, pagination, bulk operations: [Query Guide](query-guide.md)
- Relations and preload: [Relations Guide](relations-guide.md)
