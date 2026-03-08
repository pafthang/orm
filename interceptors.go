package orm

import (
	"context"
	"errors"
	"sync"
)

// Operation identifies high-level ORM operations.
type Operation string

const (
	OpInsert       Operation = "insert"
	OpUpdate       Operation = "update"
	OpUpdateFields Operation = "update_fields"
	OpDelete       Operation = "delete"
	OpByPK         Operation = "by_pk"
	OpDeleteByPK   Operation = "delete_by_pk"
	OpExistsByPK   Operation = "exists_by_pk"
	OpCount        Operation = "count"
	OpQueryAll     Operation = "query_all"
	OpQueryOne     Operation = "query_one"
	OpQueryCount   Operation = "query_count"
	OpQueryUpdate  Operation = "query_update"
	OpQueryDelete  Operation = "query_delete"
)

// OperationInfo describes one ORM operation for interceptor hooks.
type OperationInfo struct {
	Operation      Operation
	Model          string
	Table          string
	Fields         []string
	ConflictFields []string
	Relations      []string
	Preloads       []string
	HasWhere       bool
	Limit          int64
	Offset         int64
}

// BeforeInterceptor can mutate context and abort execution.
type BeforeInterceptor func(ctx context.Context, info OperationInfo) (context.Context, error)

// AfterInterceptor runs after operation execution (success or failure).
type AfterInterceptor func(ctx context.Context, info OperationInfo, opErr error) error

var interceptorsState struct {
	mu     sync.RWMutex
	before []BeforeInterceptor
	after  []AfterInterceptor
}

// AddBeforeInterceptor registers a global before interceptor.
func AddBeforeInterceptor(fn BeforeInterceptor) {
	if fn == nil {
		return
	}
	interceptorsState.mu.Lock()
	defer interceptorsState.mu.Unlock()
	interceptorsState.before = append(interceptorsState.before, fn)
}

// AddAfterInterceptor registers a global after interceptor.
func AddAfterInterceptor(fn AfterInterceptor) {
	if fn == nil {
		return
	}
	interceptorsState.mu.Lock()
	defer interceptorsState.mu.Unlock()
	interceptorsState.after = append(interceptorsState.after, fn)
}

// ResetInterceptors removes all global interceptors.
func ResetInterceptors() {
	interceptorsState.mu.Lock()
	defer interceptorsState.mu.Unlock()
	interceptorsState.before = nil
	interceptorsState.after = nil
}

func withOperationErr(ctx context.Context, info OperationInfo, fn func(context.Context) error) (err error) {
	ctx, err = runBeforeInterceptors(ctx, info)
	if err != nil {
		return err
	}
	ctx = withOperationContext(ctx, info)
	err = fn(ctx)
	afterErr := runAfterInterceptors(ctx, info, err)
	if afterErr != nil {
		if err != nil {
			return errors.Join(err, afterErr)
		}
		return afterErr
	}
	return err
}

func withOperationResult[T any](ctx context.Context, info OperationInfo, fn func(context.Context) (T, error)) (out T, err error) {
	ctx, err = runBeforeInterceptors(ctx, info)
	if err != nil {
		return out, err
	}
	ctx = withOperationContext(ctx, info)
	out, err = fn(ctx)
	afterErr := runAfterInterceptors(ctx, info, err)
	if afterErr != nil {
		if err != nil {
			return out, errors.Join(err, afterErr)
		}
		return out, afterErr
	}
	return out, err
}

func runBeforeInterceptors(ctx context.Context, info OperationInfo) (context.Context, error) {
	interceptorsState.mu.RLock()
	before := append([]BeforeInterceptor(nil), interceptorsState.before...)
	interceptorsState.mu.RUnlock()

	for _, fn := range before {
		var err error
		ctx, err = fn(ctx, info)
		if err != nil {
			return ctx, ErrInvalidQuery.with("before_interceptor", info.Model, "", err)
		}
	}
	return ctx, nil
}

func runAfterInterceptors(ctx context.Context, info OperationInfo, opErr error) error {
	interceptorsState.mu.RLock()
	after := append([]AfterInterceptor(nil), interceptorsState.after...)
	interceptorsState.mu.RUnlock()

	var err error
	for _, fn := range after {
		if e := fn(ctx, info, opErr); e != nil {
			if err == nil {
				err = ErrInvalidQuery.with("after_interceptor", info.Model, "", e)
			} else {
				err = errors.Join(err, ErrInvalidQuery.with("after_interceptor", info.Model, "", e))
			}
		}
	}
	return err
}
