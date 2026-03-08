package orm

import (
	"context"
	"fmt"
)

type tenantCtxKey struct{}

type tenantFilter struct {
	Column string
	Value  any
}

// WithTenant returns context with tenant filter for query/update/delete operations.
func WithTenant(ctx context.Context, column string, value any) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, tenantCtxKey{}, tenantFilter{Column: column, Value: value})
}

// WithTenantValue sets tenant value and lets ORM resolve tenant column from model metadata.
func WithTenantValue(ctx context.Context, value any) context.Context {
	return WithTenant(ctx, "", value)
}

func tenantFromContext(ctx context.Context) (tenantFilter, bool) {
	if ctx == nil {
		return tenantFilter{}, false
	}
	v := ctx.Value(tenantCtxKey{})
	tf, ok := v.(tenantFilter)
	if !ok {
		return tenantFilter{}, false
	}
	return tf, true
}

func tenantFromContextWithDefault(ctx context.Context, fallbackColumn string) (tenantFilter, bool) {
	tf, ok := tenantFromContext(ctx)
	if !ok {
		return tenantFilter{}, false
	}
	if tf.Column == "" {
		tf.Column = fallbackColumn
	}
	if tf.Column == "" {
		return tenantFilter{}, false
	}
	return tf, true
}

func applyTenantWhere(ctx context.Context, meta *ModelMeta, where map[string]any) map[string]any {
	if meta == nil || meta.TenantField == nil || where == nil {
		return where
	}
	tf, ok := tenantFromContextWithDefault(ctx, meta.TenantField.DBName)
	if !ok {
		return where
	}
	if _, exists := where[meta.TenantField.DBName]; exists {
		return where
	}
	where[meta.TenantField.DBName] = tf.Value
	return where
}

// TenantPlugin validates tenant context presence for selected operations.
type TenantPlugin struct {
	NameStr     string
	Required    bool
	Column      string
	PerModel    bool
	RegistryRef *Registry
}

func (p TenantPlugin) Name() string {
	if p.NameStr != "" {
		return p.NameStr
	}
	return "tenant"
}

func (p TenantPlugin) Install(host *PluginHost) error {
	if host == nil {
		return ErrInvalidQuery.with("tenant_plugin_install", "", "", fmt.Errorf("plugin host is nil"))
	}
	if p.Required && p.Column == "" {
		return ErrInvalidQuery.with("tenant_plugin_install", "", "", fmt.Errorf("column is required when Required=true"))
	}
	host.AddBeforeInterceptor(func(ctx context.Context, info OperationInfo) (context.Context, error) {
		required := p.Required
		reg := p.RegistryRef
		if reg == nil {
			reg = DefaultRegistry
		}
		if p.PerModel && info.Model != "" {
			if meta, ok := reg.ResolveByName(info.Model); ok && meta.RequireTenant {
				required = true
			}
		}
		if !required {
			return ctx, nil
		}
		switch info.Operation {
		case OpInsert, OpUpdate, OpUpdateFields, OpDelete, OpByPK, OpDeleteByPK, OpExistsByPK, OpCount, OpQueryAll, OpQueryOne, OpQueryCount, OpQueryUpdate, OpQueryDelete:
		default:
			return ctx, nil
		}
		defaultCol := p.Column
		if defaultCol == "" && info.Model != "" {
			if meta, ok := reg.ResolveByName(info.Model); ok && meta.TenantField != nil {
				defaultCol = meta.TenantField.DBName
			}
		}
		tf, ok := tenantFromContextWithDefault(ctx, defaultCol)
		if !ok {
			return ctx, ErrInvalidQuery.with("tenant_plugin", info.Model, "", fmt.Errorf("tenant context is required"))
		}
		if defaultCol != "" && tf.Column != defaultCol {
			return ctx, ErrInvalidQuery.with("tenant_plugin", info.Model, "", fmt.Errorf("tenant column mismatch"))
		}
		return ctx, nil
	})
	return nil
}
