package orm

import (
	"fmt"
	"reflect"
	"sort"
	"sync"
)

// Registry holds parsed model metadata.
type Registry struct {
	mu                    sync.RWMutex
	models                map[reflect.Type]*ModelMeta
	naming                NamingStrategy
	extenders             []FieldExtender
	tenantRequiredDefault *bool
}

// NewRegistry creates a metadata registry.
func NewRegistry(opts ...RegistryOption) *Registry {
	r := &Registry{
		models: make(map[reflect.Type]*ModelMeta),
		naming: DefaultNamingStrategy{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
	if r.naming == nil {
		r.naming = DefaultNamingStrategy{}
	}
	return r
}

// RegisterType registers a model type using optional config override.
func (r *Registry) RegisterType(model any, cfg ...ModelConfig) (*ModelMeta, error) {
	t, err := modelType(model)
	if err != nil {
		return nil, err
	}
	var config ModelConfig
	if len(cfg) > 0 {
		config = cfg[0]
	}
	if len(r.extenders) > 0 {
		config.Extenders = append(append([]FieldExtender{}, r.extenders...), config.Extenders...)
	}
	if config.RequireTenant == nil && r.tenantRequiredDefault != nil {
		v := *r.tenantRequiredDefault
		config.RequireTenant = &v
	}

	meta, err := parseModelMeta(t, config, r.naming)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.models[t] = meta
	return meta, nil
}

// ResolveByName resolves metadata by model name.
func (r *Registry) ResolveByName(name string) (*ModelMeta, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, m := range r.models {
		if m != nil && m.Name == name {
			return m, true
		}
	}
	return nil, false
}

// Resolve returns model metadata, lazy-registering when needed.
func (r *Registry) Resolve(model any) (*ModelMeta, error) {
	t, err := modelType(model)
	if err != nil {
		return nil, err
	}

	r.mu.RLock()
	if meta, ok := r.models[t]; ok {
		r.mu.RUnlock()
		return meta, nil
	}
	r.mu.RUnlock()

	return r.RegisterType(reflect.New(t).Elem().Interface())
}

// Models returns a stable snapshot of all currently registered model metadata.
func (r *Registry) Models() []*ModelMeta {
	r.mu.RLock()
	out := make([]*ModelMeta, 0, len(r.models))
	for _, m := range r.models {
		if m != nil {
			out = append(out, m)
		}
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Validate checks that all registered models are internally consistent.
func (r *Registry) Validate() error {
	r.mu.RLock()
	models := make([]*ModelMeta, 0, len(r.models))
	for _, m := range r.models {
		models = append(models, m)
	}
	r.mu.RUnlock()

	for _, meta := range models {
		if len(meta.PrimaryKeys) == 0 {
			return ErrMissingPrimaryKey.with("validate_registry", meta.Name, "", fmt.Errorf("no primary key"))
		}
		for relName, rel := range meta.Relations {
			if rel == nil {
				return ErrInvalidModel.with("validate_registry", meta.Name, relName, fmt.Errorf("nil relation meta"))
			}
			if _, ok := meta.FieldsByGo[rel.LocalField]; !ok {
				localNames := splitRelationFields(rel.LocalField)
				for _, lf := range localNames {
					if _, ok := meta.FieldsByGo[lf]; !ok {
						return ErrInvalidModel.with("validate_registry", meta.Name, relName, fmt.Errorf("local field not found: %s", lf))
					}
				}
			}
			targetMeta, err := r.Resolve(reflect.New(rel.TargetType).Elem().Interface())
			if err != nil {
				return ErrInvalidModel.with("validate_registry", meta.Name, relName, fmt.Errorf("target model resolve failed: %w", err))
			}
			foreignNames := splitRelationFields(rel.ForeignRef)
			for _, ff := range foreignNames {
				if _, ok := targetMeta.FieldsByGo[ff]; !ok {
					return ErrInvalidModel.with("validate_registry", meta.Name, relName, fmt.Errorf("target foreign field not found: %s", ff))
				}
			}
			if err := validateRelationFieldShape(meta, rel); err != nil {
				return err
			}
			if err := validateRelationKeyCompatibility(meta, targetMeta, rel); err != nil {
				return err
			}
			if rel.Kind == RelationManyToMany {
				if rel.JoinTable == "" {
					return ErrInvalidModel.with("validate_registry", meta.Name, relName, fmt.Errorf("join table is required for many_to_many"))
				}
				if rel.JoinLocalKey == "" {
					return ErrInvalidModel.with("validate_registry", meta.Name, relName, fmt.Errorf("join local key is required for many_to_many"))
				}
				if rel.JoinForeignKey == "" {
					return ErrInvalidModel.with("validate_registry", meta.Name, relName, fmt.Errorf("join foreign key is required for many_to_many"))
				}
			}
			if rel.Kind != RelationBelongsTo && rel.Kind != RelationHasMany && rel.Kind != RelationManyToMany {
				return ErrInvalidModel.with("validate_registry", meta.Name, relName, fmt.Errorf("unsupported relation kind: %s", rel.Kind))
			}
		}
	}
	return nil
}

func validateRelationFieldShape(meta *ModelMeta, rel *RelationMeta) error {
	sf := meta.Type.FieldByIndex(rel.FieldIndex)
	t := sf.Type
	switch rel.Kind {
	case RelationBelongsTo:
		base := t
		for base.Kind() == reflect.Pointer {
			base = base.Elem()
		}
		if base.Kind() != reflect.Struct {
			return ErrInvalidModel.with("validate_registry", meta.Name, rel.Name, fmt.Errorf("belongs_to relation field must be struct or pointer to struct"))
		}
	case RelationHasMany, RelationManyToMany:
		if t.Kind() != reflect.Slice {
			return ErrInvalidModel.with("validate_registry", meta.Name, rel.Name, fmt.Errorf("%s relation field must be slice", rel.Kind))
		}
	default:
		return ErrInvalidModel.with("validate_registry", meta.Name, rel.Name, fmt.Errorf("unsupported relation kind: %s", rel.Kind))
	}
	return nil
}

func validateRelationKeyCompatibility(src, target *ModelMeta, rel *RelationMeta) error {
	locals := splitRelationFields(rel.LocalField)
	foreigns := splitRelationFields(rel.ForeignRef)
	if len(locals) != len(foreigns) {
		return ErrInvalidModel.with("validate_registry", src.Name, rel.Name, fmt.Errorf("local/foreign key count mismatch"))
	}
	for i := range locals {
		local, ok := src.FieldsByGo[locals[i]]
		if !ok {
			return ErrInvalidModel.with("validate_registry", src.Name, rel.Name, fmt.Errorf("local field not found: %s", locals[i]))
		}
		foreign, ok := target.FieldsByGo[foreigns[i]]
		if !ok {
			return ErrInvalidModel.with("validate_registry", src.Name, rel.Name, fmt.Errorf("target field not found: %s", foreigns[i]))
		}
		lt := normalizeType(local.Type)
		ft := normalizeType(foreign.Type)
		if !relationTypesCompatible(lt, ft) {
			return ErrInvalidModel.with("validate_registry", src.Name, rel.Name, fmt.Errorf("key type mismatch: %s vs %s", lt, ft))
		}
	}
	return nil
}

func normalizeType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func relationTypesCompatible(a, b reflect.Type) bool {
	if a == b {
		return true
	}
	if a.AssignableTo(b) || b.AssignableTo(a) {
		return true
	}
	ak, bk := a.Kind(), b.Kind()
	return isIntegerKind(ak) && isIntegerKind(bk)
}

func isIntegerKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	default:
		return false
	}
}

func modelType(model any) (reflect.Type, error) {
	if model == nil {
		return nil, ErrInvalidModel.with("model_type", "", "", fmt.Errorf("model is nil"))
	}
	t := reflect.TypeOf(model)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, ErrInvalidModel.with("model_type", t.String(), "", fmt.Errorf("model must be struct or pointer to struct"))
	}
	return t, nil
}

var DefaultRegistry = NewRegistry()

// Register registers model T in global registry.
func Register[T any](cfg ...ModelConfig) (*ModelMeta, error) {
	var zero T
	return DefaultRegistry.RegisterType(zero, cfg...)
}

// Meta returns model metadata for T from global registry.
func Meta[T any]() (*ModelMeta, error) {
	var zero T
	return DefaultRegistry.Resolve(zero)
}

// Validate validates global registry.
func Validate() error {
	return DefaultRegistry.Validate()
}
