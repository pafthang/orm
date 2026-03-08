# Hooks Guide

## Model Lifecycle Hooks

Supported model hooks:
- `BeforeInsert`
- `AfterInsert`
- `BeforeUpdate`
- `AfterUpdate`
- `BeforeDelete`
- `AfterDelete`
- `AfterFind`

If a hook returns error, operation fails.

## Global Interceptors

Use interceptors for cross-cutting policies:

- `AddBeforeInterceptor`
- `AddAfterInterceptor`
- `ResetInterceptors`

Typical use cases:
- auditing
- request policy checks
- tenant constraints
- security rules

## Plugins

Plugins install interceptors and other global behaviors in one unit:

- `RegisterPlugin`
- `RegisteredPlugins`
- `ResetPlugins`

Example plugin domains:
- tenant enforcement
- tracing wiring
- operation-level validation

## Tracing Integration

`AttachTraceObserver` wraps tracing start/finish callbacks through interceptor layer.

## Error Handling Strategy

Treat hook/interceptor errors as business control-flow:
- return typed errors where possible
- avoid panics
- keep messages machine-parseable for external adapters
