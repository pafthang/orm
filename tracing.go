package orm

import "context"

// TraceObserverOptions configures tracing integration via interceptor layer.
type TraceObserverOptions struct {
	OnStart  func(ctx context.Context, info OperationInfo) context.Context
	OnFinish func(ctx context.Context, info OperationInfo, opErr error)
}

// AttachTraceObserver registers tracing callbacks as global interceptors.
func AttachTraceObserver(opts TraceObserverOptions) {
	if opts.OnStart != nil {
		AddBeforeInterceptor(func(ctx context.Context, info OperationInfo) (context.Context, error) {
			return opts.OnStart(ctx, info), nil
		})
	}
	if opts.OnFinish != nil {
		AddAfterInterceptor(func(ctx context.Context, info OperationInfo, opErr error) error {
			opts.OnFinish(ctx, info, opErr)
			return nil
		})
	}
}
