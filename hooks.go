package orm

import "context"

// BeforeInsertHook runs before Insert.
type BeforeInsertHook interface {
	BeforeInsert(ctx context.Context) error
}

// AfterInsertHook runs after successful Insert.
type AfterInsertHook interface {
	AfterInsert(ctx context.Context) error
}

// BeforeUpdateHook runs before Update/UpdateFields.
type BeforeUpdateHook interface {
	BeforeUpdate(ctx context.Context) error
}

// AfterUpdateHook runs after successful Update/UpdateFields.
type AfterUpdateHook interface {
	AfterUpdate(ctx context.Context) error
}

// BeforeDeleteHook runs before Delete.
type BeforeDeleteHook interface {
	BeforeDelete(ctx context.Context) error
}

// AfterDeleteHook runs after successful Delete.
type AfterDeleteHook interface {
	AfterDelete(ctx context.Context) error
}

// AfterFindHook runs after a model is loaded from DB.
type AfterFindHook interface {
	AfterFind(ctx context.Context) error
}

func callBeforeInsertHook(ctx context.Context, model any) error {
	h, ok := model.(BeforeInsertHook)
	if !ok {
		return nil
	}
	return h.BeforeInsert(ctx)
}

func callAfterInsertHook(ctx context.Context, model any) error {
	h, ok := model.(AfterInsertHook)
	if !ok {
		return nil
	}
	return h.AfterInsert(ctx)
}

func callBeforeUpdateHook(ctx context.Context, model any) error {
	h, ok := model.(BeforeUpdateHook)
	if !ok {
		return nil
	}
	return h.BeforeUpdate(ctx)
}

func callAfterUpdateHook(ctx context.Context, model any) error {
	h, ok := model.(AfterUpdateHook)
	if !ok {
		return nil
	}
	return h.AfterUpdate(ctx)
}

func callBeforeDeleteHook(ctx context.Context, model any) error {
	h, ok := model.(BeforeDeleteHook)
	if !ok {
		return nil
	}
	return h.BeforeDelete(ctx)
}

func callAfterDeleteHook(ctx context.Context, model any) error {
	h, ok := model.(AfterDeleteHook)
	if !ok {
		return nil
	}
	return h.AfterDelete(ctx)
}

func callAfterFindHook(ctx context.Context, model any) error {
	h, ok := model.(AfterFindHook)
	if !ok {
		return nil
	}
	return h.AfterFind(ctx)
}
