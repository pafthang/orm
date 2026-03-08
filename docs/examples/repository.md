# Repository Example

```go
package repository

import (
	"context"

	"github.com/pafthang/orm"
)

type User struct {
	ID    int64  `db:"id,pk"`
	Email string `db:"email"`
	Name  string `db:"name"`
}

type UserRepo struct {
	db orm.DB
}

func NewUserRepo(db orm.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) ByID(ctx context.Context, id int64) (*User, error) {
	return orm.ByPK[User](ctx, r.db, id)
}

func (r *UserRepo) Create(ctx context.Context, u *User) error {
	return orm.Insert(ctx, r.db, u)
}

func (r *UserRepo) UpdateName(ctx context.Context, id int64, name string) error {
	affected, err := orm.UpdateByPK[User](ctx, r.db, id, map[string]any{"name": name})
	if err != nil {
		return err
	}
	if affected == 0 {
		return orm.ErrNoRowsAffected
	}
	return nil
}

func (r *UserRepo) ActiveList(ctx context.Context, page, perPage int64) ([]User, error) {
	rows, err := orm.Query[User](r.db).
		WhereEq("status", "active").
		Page(page, perPage).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return rows, nil
}
```

Notes:
- Repositories stay thin; keep business orchestration in service layer.
- For high-complexity SQL, call `dbx` directly and keep repository contract stable.
