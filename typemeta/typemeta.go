package typemeta

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// FieldMeta is transport-neutral field metadata.
type FieldMeta struct {
	GoName       string
	DBName       string
	JSONName     string
	Type         reflect.Type
	Index        []int
	EmbeddedPath []string

	IsPK       bool
	IsNullable bool
	IsIgnored  bool

	Attributes map[string]string
}

// TypeMeta is transport-neutral metadata for a Go struct type.
type TypeMeta struct {
	Type       reflect.Type
	Name       string
	Fields     []*FieldMeta
	FieldsByGo map[string]*FieldMeta
	FieldsByDB map[string]*FieldMeta
}

// Registry caches parsed metadata.
type Registry struct {
	mu    sync.RWMutex
	cache map[reflect.Type]*TypeMeta
}

// NewRegistry creates metadata registry.
func NewRegistry() *Registry {
	return &Registry{cache: map[reflect.Type]*TypeMeta{}}
}

// Resolve returns metadata for model type and caches it.
func (r *Registry) Resolve(model any) (*TypeMeta, error) {
	t, err := modelType(model)
	if err != nil {
		return nil, err
	}

	r.mu.RLock()
	if meta, ok := r.cache[t]; ok {
		r.mu.RUnlock()
		return meta, nil
	}
	r.mu.RUnlock()

	meta, err := parseTypeMeta(t)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.cache[t] = meta
	r.mu.Unlock()
	return meta, nil
}

func modelType(model any) (reflect.Type, error) {
	if model == nil {
		return nil, fmt.Errorf("model is nil")
	}
	t := reflect.TypeOf(model)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("model must be struct or pointer to struct")
	}
	return t, nil
}

func parseTypeMeta(t reflect.Type) (*TypeMeta, error) {
	meta := &TypeMeta{
		Type:       t,
		Name:       t.Name(),
		FieldsByGo: map[string]*FieldMeta{},
		FieldsByDB: map[string]*FieldMeta{},
	}
	if err := parseStruct(meta, t, nil, nil); err != nil {
		return nil, err
	}
	return meta, nil
}

func parseStruct(meta *TypeMeta, t reflect.Type, indexPrefix []int, pathPrefix []string) error {
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" && !sf.Anonymous {
			continue
		}

		idx := append(append([]int{}, indexPrefix...), i)
		dbName, opts, ignore := parseDBTag(sf.Tag.Get("db"))
		if ignore {
			continue
		}

		if sf.Anonymous && dbName == "" {
			inner := sf.Type
			for inner.Kind() == reflect.Pointer {
				inner = inner.Elem()
			}
			if inner.Kind() == reflect.Struct {
				innerPath := append(append([]string{}, pathPrefix...), sf.Name)
				if err := parseStruct(meta, inner, idx, innerPath); err != nil {
					return err
				}
				continue
			}
		}

		field := &FieldMeta{
			GoName:       sf.Name,
			DBName:       dbName,
			JSONName:     parseJSONName(sf.Tag.Get("json"), sf.Name),
			Type:         sf.Type,
			Index:        idx,
			EmbeddedPath: append([]string{}, pathPrefix...),
			Attributes:   map[string]string{},
		}
		if field.DBName == "" {
			field.DBName = toSnake(sf.Name)
		}
		if _, ok := opts["pk"]; ok {
			field.IsPK = true
		}
		if _, ok := opts["nullable"]; ok {
			field.IsNullable = true
		}
		if !field.IsNullable {
			field.IsNullable = isNullableType(sf.Type)
		}
		for k := range opts {
			field.Attributes[k] = "true"
		}

		if _, exists := meta.FieldsByGo[field.GoName]; exists {
			return fmt.Errorf("duplicate Go field: %s", field.GoName)
		}
		if _, exists := meta.FieldsByDB[field.DBName]; exists {
			return fmt.Errorf("duplicate DB field: %s", field.DBName)
		}

		meta.Fields = append(meta.Fields, field)
		meta.FieldsByGo[field.GoName] = field
		meta.FieldsByDB[field.DBName] = field
	}
	return nil
}

func parseDBTag(tag string) (name string, opts map[string]struct{}, ignore bool) {
	opts = map[string]struct{}{}
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "", opts, false
	}
	if tag == "-" {
		return "", opts, true
	}
	parts := strings.Split(tag, ",")
	first := strings.TrimSpace(parts[0])
	if first != "" {
		name = first
	}
	for _, p := range parts[1:] {
		p = strings.TrimSpace(strings.ToLower(p))
		if p == "" {
			continue
		}
		opts[p] = struct{}{}
	}
	return name, opts, false
}

func parseJSONName(tag, fallback string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return toSnake(fallback)
	}
	if tag == "-" {
		return ""
	}
	parts := strings.Split(tag, ",")
	if parts[0] == "" {
		return toSnake(fallback)
	}
	return parts[0]
}

func isNullableType(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		return true
	}
	switch t.Kind() {
	case reflect.Interface, reflect.Map, reflect.Slice:
		return true
	default:
		return false
	}
}

func toSnake(in string) string {
	if in == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(in) + 4)
	for i, r := range in {
		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := rune(in[i-1])
			if prev != '_' && (prev < 'A' || prev > 'Z') {
				b.WriteByte('_')
			}
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}
