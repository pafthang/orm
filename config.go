package orm

import "reflect"

// TableNamer allows model types to override table name.
type TableNamer interface {
	TableName() string
}

// FieldExtender allows custom tags/field behavior during metadata parsing.
type FieldExtender interface {
	ExtendField(sf reflect.StructField, field *FieldMeta) error
}

// ModelConfig overrides inferred model metadata.
type ModelConfig struct {
	Table         string
	Naming        NamingStrategy
	Relations     map[string]RelationConfig
	Extenders     []FieldExtender
	TenantField   string
	FieldCodecs   map[string]Codec
	RequireTenant *bool
}

// RelationConfig overrides inferred relation metadata.
type RelationConfig struct {
	Kind           RelationKind
	LocalField     string
	ForeignRef     string
	JoinTable      string
	JoinLocalKey   string
	JoinForeignKey string
}

// RegistryOption mutates registry defaults.
type RegistryOption func(*Registry)

// WithNamingStrategy sets default naming strategy for a registry.
func WithNamingStrategy(n NamingStrategy) RegistryOption {
	return func(r *Registry) {
		if n != nil {
			r.naming = n
		}
	}
}

// WithFieldExtender adds global metadata field extender for this registry.
func WithFieldExtender(ext FieldExtender) RegistryOption {
	return func(r *Registry) {
		if ext != nil {
			r.extenders = append(r.extenders, ext)
		}
	}
}

// WithTenantRequiredDefault sets default tenant-required policy for all models in registry.
func WithTenantRequiredDefault(required bool) RegistryOption {
	return func(r *Registry) {
		v := required
		r.tenantRequiredDefault = &v
	}
}
