package orm

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// FieldMeta describes one model field mapped to DB.
type FieldMeta struct {
	GoName       string
	DBName       string
	JSONName     string
	Type         reflect.Type
	Index        []int
	EmbeddedPath []string

	IsPK           bool
	IsNullable     bool
	IsIgnored      bool
	IsReadOnly     bool
	IsWriteOnly    bool
	HasDefault     bool
	IsGenerated    bool
	IsSoftDelete   bool
	IsCreatedAt    bool
	IsUpdatedAt    bool
	IsRelation     bool
	IsEmbeddedRoot bool
	Attributes     map[string]string
	Codec          Codec
}

// ModelMeta describes one registered model.
type ModelMeta struct {
	Type  reflect.Type
	Name  string
	Table string

	Fields      []*FieldMeta
	FieldsByGo  map[string]*FieldMeta
	FieldsByDB  map[string]*FieldMeta
	PrimaryKeys []*FieldMeta

	SoftDeleteField *FieldMeta
	CreatedAtField  *FieldMeta
	UpdatedAtField  *FieldMeta
	TenantField     *FieldMeta
	RequireTenant   bool
	Relations       map[string]*RelationMeta
}

// RelationKind describes supported relation types.
type RelationKind string

const (
	RelationBelongsTo  RelationKind = "belongs_to"
	RelationHasMany    RelationKind = "has_many"
	RelationManyToMany RelationKind = "many_to_many"
)

// RelationMeta describes relation mapping for explicit preload.
type RelationMeta struct {
	Name           string
	Kind           RelationKind
	FieldIndex     []int
	TargetType     reflect.Type
	LocalField     string
	ForeignRef     string
	JoinTable      string
	JoinLocalKey   string
	JoinForeignKey string
}

func parseModelMeta(modelType reflect.Type, cfg ModelConfig, fallback NamingStrategy) (*ModelMeta, error) {
	if modelType.Kind() != reflect.Struct {
		return nil, ErrInvalidModel.with("parse_model", modelType.String(), "", fmt.Errorf("model must be a struct"))
	}

	naming := fallback
	if cfg.Naming != nil {
		naming = cfg.Naming
	}
	if naming == nil {
		naming = DefaultNamingStrategy{}
	}

	table := cfg.Table
	if table == "" {
		if tname := tableNameFromType(modelType); tname != "" {
			table = tname
		} else {
			table = naming.TableName(modelType.Name())
		}
	}
	if table == "" {
		return nil, ErrInvalidModel.with("parse_model", modelType.String(), "", fmt.Errorf("unable to infer table name"))
	}

	meta := &ModelMeta{
		Type:       modelType,
		Name:       modelType.Name(),
		Table:      table,
		FieldsByGo: make(map[string]*FieldMeta),
		FieldsByDB: make(map[string]*FieldMeta),
		Relations:  make(map[string]*RelationMeta),
	}

	if err := parseStructFields(meta, modelType, nil, nil, naming, cfg.Extenders, cfg.FieldCodecs); err != nil {
		return nil, err
	}

	if len(meta.PrimaryKeys) == 0 {
		return nil, ErrMissingPrimaryKey.with("parse_model", modelType.String(), "", fmt.Errorf("no primary key field found"))
	}
	if err := applyFieldCodecs(meta, cfg.FieldCodecs); err != nil {
		return nil, err
	}
	if err := applyTenantField(meta, cfg.TenantField); err != nil {
		return nil, err
	}
	if cfg.RequireTenant != nil {
		meta.RequireTenant = *cfg.RequireTenant
	}
	inferRelations(meta, modelType)
	if err := applyRelationTagOverrides(meta, modelType); err != nil {
		return nil, err
	}
	if err := applyRelationOverrides(meta, modelType, cfg.Relations); err != nil {
		return nil, err
	}

	return meta, nil
}

func applyFieldCodecs(meta *ModelMeta, in map[string]Codec) error {
	if len(in) == 0 {
		return nil
	}
	for name, codec := range in {
		if codec == nil {
			return ErrInvalidModel.with("parse_model", meta.Name, name, fmt.Errorf("field codec is nil"))
		}
		fm, err := findFieldMeta(meta, name)
		if err != nil {
			return ErrInvalidModel.with("parse_model", meta.Name, name, err)
		}
		fm.Codec = codec
	}
	return nil
}

func applyTenantField(meta *ModelMeta, explicit string) error {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		if f, ok := meta.FieldsByGo[explicit]; ok {
			meta.TenantField = f
			return nil
		}
		if f, ok := meta.FieldsByDB[explicit]; ok {
			meta.TenantField = f
			return nil
		}
		return ErrInvalidModel.with("parse_model", meta.Name, explicit, fmt.Errorf("tenant field not found"))
	}
	if f, ok := meta.FieldsByGo["TenantID"]; ok {
		meta.TenantField = f
		return nil
	}
	if f, ok := meta.FieldsByDB["tenant_id"]; ok {
		meta.TenantField = f
		return nil
	}
	return nil
}

func parseStructFields(meta *ModelMeta, typ reflect.Type, indexPrefix []int, pathPrefix []string, naming NamingStrategy, extenders []FieldExtender, fieldCodecs map[string]Codec) error {
	for i := 0; i < typ.NumField(); i++ {
		sf := typ.Field(i)
		if sf.PkgPath != "" && !sf.Anonymous {
			continue
		}

		idx := append(append([]int{}, indexPrefix...), i)
		dbName, opts, ignore := parseDBTag(sf.Tag.Get("db"))
		if ignore {
			continue
		}

		sfType := sf.Type
		if sf.Anonymous && shouldFlattenEmbedded(sf, dbName, opts) {
			inner := sfType
			if inner.Kind() == reflect.Pointer {
				inner = inner.Elem()
			}
			if inner.Kind() == reflect.Struct {
				innerPath := append(append([]string{}, pathPrefix...), sf.Name)
				if err := parseStructFields(meta, inner, idx, innerPath, naming, extenders, fieldCodecs); err != nil {
					return err
				}
				continue
			}
		}
		if dbName == "" {
			if _, ok := detectBelongsToTarget(sfType); ok {
				continue
			}
			if _, ok := detectHasManyTarget(sfType); ok {
				continue
			}
		}

		if sfType.Kind() == reflect.Struct && sfType.AssignableTo(reflect.TypeOf(sql.NullString{})) {
			// sql.Null* are supported scalar structs and should not be flattened.
		}

		field := &FieldMeta{
			GoName:       sf.Name,
			Type:         sfType,
			Index:        idx,
			EmbeddedPath: append([]string{}, pathPrefix...),
			JSONName:     parseJSONName(sf.Tag.Get("json"), sf.Name),
			DBName:       dbName,
			Attributes:   map[string]string{},
		}
		if !isSupportedPersistenceType(sfType) && !hasCodecHint(sf, fieldCodecs) {
			return ErrUnsupportedType.with("parse_model", meta.Name, sf.Name, fmt.Errorf("unsupported persistence type: %s", sfType.String()))
		}
		if field.DBName == "" {
			field.DBName = naming.ColumnName(sf.Name)
		}

		if _, ok := opts["pk"]; ok {
			field.IsPK = true
		}
		if _, ok := opts["nullable"]; ok {
			field.IsNullable = true
		}
		if _, ok := opts["readonly"]; ok {
			field.IsReadOnly = true
		}
		if _, ok := opts["writeonly"]; ok {
			field.IsWriteOnly = true
		}
		if _, ok := opts["default"]; ok {
			field.HasDefault = true
		}
		if _, ok := opts["generated"]; ok {
			field.IsGenerated = true
		}
		if _, ok := opts["soft_delete"]; ok {
			field.IsSoftDelete = true
		}
		if _, ok := opts["created_at"]; ok {
			field.IsCreatedAt = true
		}
		if _, ok := opts["updated_at"]; ok {
			field.IsUpdatedAt = true
		}
		for _, ext := range extenders {
			if ext == nil {
				continue
			}
			if err := ext.ExtendField(sf, field); err != nil {
				return ErrInvalidModel.with("parse_model", meta.Name, field.GoName, err)
			}
		}

		if !field.IsNullable {
			field.IsNullable = isNullableType(sfType)
		}

		if prev, exists := meta.FieldsByGo[field.GoName]; exists {
			return ErrInvalidModel.with("parse_model", meta.Name, field.GoName, fmt.Errorf("duplicate Go field: %s", prev.GoName))
		}
		if prev, exists := meta.FieldsByDB[field.DBName]; exists {
			return ErrInvalidModel.with("parse_model", meta.Name, field.GoName, fmt.Errorf("duplicate DB column: %s (already %s)", field.DBName, prev.GoName))
		}

		meta.Fields = append(meta.Fields, field)
		meta.FieldsByGo[field.GoName] = field
		meta.FieldsByDB[field.DBName] = field

		if field.IsPK {
			meta.PrimaryKeys = append(meta.PrimaryKeys, field)
		}
		if field.IsSoftDelete {
			if meta.SoftDeleteField != nil {
				return ErrInvalidModel.with("parse_model", meta.Name, field.GoName, fmt.Errorf("multiple soft delete fields"))
			}
			meta.SoftDeleteField = field
		}
		if field.IsCreatedAt {
			if meta.CreatedAtField != nil {
				return ErrInvalidModel.with("parse_model", meta.Name, field.GoName, fmt.Errorf("multiple created_at fields"))
			}
			meta.CreatedAtField = field
		}
		if field.IsUpdatedAt {
			if meta.UpdatedAtField != nil {
				return ErrInvalidModel.with("parse_model", meta.Name, field.GoName, fmt.Errorf("multiple updated_at fields"))
			}
			meta.UpdatedAtField = field
		}
	}
	return nil
}

func hasCodecHint(sf reflect.StructField, fieldCodecs map[string]Codec) bool {
	if len(fieldCodecs) == 0 {
		return false
	}
	if _, ok := fieldCodecs[sf.Name]; ok {
		return true
	}
	dbName, _, ignore := parseDBTag(sf.Tag.Get("db"))
	if !ignore && dbName != "" {
		if _, ok := fieldCodecs[dbName]; ok {
			return true
		}
	}
	return false
}

func shouldFlattenEmbedded(sf reflect.StructField, dbName string, opts map[string]struct{}) bool {
	if dbName != "" {
		return false
	}
	if _, ok := opts["embedded"]; ok {
		return true
	}
	return true
}

func parseDBTag(tag string) (name string, opts map[string]struct{}, ignore bool) {
	opts = make(map[string]struct{})
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

func tableNameFromType(t reflect.Type) string {
	if reflect.PointerTo(t).Implements(reflect.TypeOf((*TableNamer)(nil)).Elem()) {
		v := reflect.New(t).Interface().(TableNamer)
		return strings.TrimSpace(v.TableName())
	}
	if t.Implements(reflect.TypeOf((*TableNamer)(nil)).Elem()) {
		v := reflect.Zero(t).Interface().(TableNamer)
		return strings.TrimSpace(v.TableName())
	}
	return ""
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

func inferRelations(meta *ModelMeta, modelType reflect.Type) {
	for i := 0; i < modelType.NumField(); i++ {
		sf := modelType.Field(i)
		if sf.PkgPath != "" || sf.Anonymous {
			continue
		}
		dbName, _, ignore := parseDBTag(sf.Tag.Get("db"))
		if ignore || dbName != "" {
			continue
		}

		if target, ok := detectBelongsToTarget(sf.Type); ok {
			localField := sf.Name + "ID"
			if _, exists := meta.FieldsByGo[localField]; !exists {
				continue
			}
			meta.Relations[sf.Name] = &RelationMeta{
				Name:       sf.Name,
				Kind:       RelationBelongsTo,
				FieldIndex: sf.Index,
				TargetType: target,
				LocalField: localField,
				ForeignRef: "ID",
			}
			continue
		}

		if target, ok := detectHasManyTarget(sf.Type); ok && len(meta.PrimaryKeys) == 1 {
			meta.Relations[sf.Name] = &RelationMeta{
				Name:       sf.Name,
				Kind:       RelationHasMany,
				FieldIndex: sf.Index,
				TargetType: target,
				LocalField: meta.PrimaryKeys[0].GoName,
				ForeignRef: meta.Name + "ID",
			}
		}
	}
}

func detectBelongsToTarget(t reflect.Type) (reflect.Type, bool) {
	base := t
	for base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	if base.Kind() != reflect.Struct {
		return nil, false
	}
	if base == reflect.TypeOf(time.Time{}) {
		return nil, false
	}
	if reflect.PointerTo(base).Implements(reflect.TypeOf((*sql.Scanner)(nil)).Elem()) {
		return nil, false
	}
	if reflect.PointerTo(base).Implements(reflect.TypeOf((*driver.Valuer)(nil)).Elem()) {
		return nil, false
	}
	if strings.HasPrefix(base.PkgPath(), "database/sql") && strings.HasPrefix(base.Name(), "Null") {
		return nil, false
	}
	return base, true
}

func detectHasManyTarget(t reflect.Type) (reflect.Type, bool) {
	if t.Kind() != reflect.Slice {
		return nil, false
	}
	elem := t.Elem()
	for elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}
	if elem.Kind() != reflect.Struct {
		return nil, false
	}
	if elem == reflect.TypeOf(time.Time{}) {
		return nil, false
	}
	return elem, true
}

func isSupportedPersistenceType(t reflect.Type) bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if hasCodecForType(t) {
		return true
	}

	// Scanner/Valuer custom types are explicitly supported.
	scannerType := reflect.TypeOf((*sql.Scanner)(nil)).Elem()
	valuerType := reflect.TypeOf((*driver.Valuer)(nil)).Elem()
	if t.Implements(scannerType) || reflect.PointerTo(t).Implements(scannerType) {
		return true
	}
	if t.Implements(valuerType) || reflect.PointerTo(t).Implements(valuerType) {
		return true
	}

	// Common scalar kinds.
	switch t.Kind() {
	case reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.String:
		return true
	}

	// Common struct types.
	if t == reflect.TypeOf(time.Time{}) {
		return true
	}
	if strings.HasPrefix(t.PkgPath(), "database/sql") && strings.HasPrefix(t.Name(), "Null") {
		return true
	}

	// JSON/document style fields are allowed for now.
	switch t.Kind() {
	case reflect.Map, reflect.Slice, reflect.Array, reflect.Interface:
		return true
	}

	// Plain structs are allowed only if they implement scanner/valuer (handled above).
	if t.Kind() == reflect.Struct {
		return false
	}
	return false
}

func applyRelationOverrides(meta *ModelMeta, modelType reflect.Type, overrides map[string]RelationConfig) error {
	if len(overrides) == 0 {
		return nil
	}
	for name, cfg := range overrides {
		name = strings.TrimSpace(name)
		if name == "" {
			return ErrInvalidModel.with("parse_model", meta.Name, "", fmt.Errorf("relation override name is empty"))
		}
		rel := meta.Relations[name]
		if rel == nil {
			sf, ok := modelType.FieldByName(name)
			if !ok || sf.PkgPath != "" {
				return ErrInvalidModel.with("parse_model", meta.Name, name, fmt.Errorf("relation field not found"))
			}
			target, ok := detectBelongsToTarget(sf.Type)
			kind := cfg.Kind
			if kind == RelationHasMany || kind == RelationManyToMany {
				if t, ok2 := detectHasManyTarget(sf.Type); ok2 {
					target = t
					ok = true
				}
			}
			if kind == RelationBelongsTo {
				if t, ok2 := detectBelongsToTarget(sf.Type); ok2 {
					target = t
					ok = true
				}
			}
			if kind == "" {
				if ok {
					kind = RelationBelongsTo
				} else if target, ok = detectHasManyTarget(sf.Type); ok {
					kind = RelationHasMany
				}
			}
			if target == nil {
				return ErrInvalidModel.with("parse_model", meta.Name, name, fmt.Errorf("unable to infer relation target type"))
			}
			rel = &RelationMeta{
				Name:           name,
				Kind:           kind,
				FieldIndex:     sf.Index,
				TargetType:     target,
				LocalField:     defaultLocalField(meta, name, kind),
				ForeignRef:     defaultForeignRef(meta, kind),
				JoinTable:      defaultJoinTable(meta, target, kind),
				JoinLocalKey:   defaultJoinLocalKey(meta, kind),
				JoinForeignKey: defaultJoinForeignKey(target, kind),
			}
			meta.Relations[name] = rel
		}
		if cfg.Kind != "" {
			rel.Kind = cfg.Kind
		}
		if cfg.LocalField != "" {
			rel.LocalField = cfg.LocalField
		}
		if cfg.ForeignRef != "" {
			rel.ForeignRef = cfg.ForeignRef
		}
		if cfg.JoinTable != "" {
			rel.JoinTable = cfg.JoinTable
		}
		if cfg.JoinLocalKey != "" {
			rel.JoinLocalKey = cfg.JoinLocalKey
		}
		if cfg.JoinForeignKey != "" {
			rel.JoinForeignKey = cfg.JoinForeignKey
		}
	}
	return nil
}

func applyRelationTagOverrides(meta *ModelMeta, modelType reflect.Type) error {
	for i := 0; i < modelType.NumField(); i++ {
		sf := modelType.Field(i)
		if sf.PkgPath != "" || sf.Anonymous {
			continue
		}
		cfg, ok, err := parseORMRelationTag(sf.Tag.Get("orm"))
		if err != nil {
			return ErrInvalidModel.with("parse_model", meta.Name, sf.Name, err)
		}
		if !ok {
			continue
		}

		rel := meta.Relations[sf.Name]
		if rel == nil {
			target, ok := detectBelongsToTarget(sf.Type)
			kind := cfg.Kind
			if kind == RelationHasMany || kind == RelationManyToMany {
				if t, ok2 := detectHasManyTarget(sf.Type); ok2 {
					target = t
					ok = true
				}
			}
			if kind == RelationBelongsTo {
				if t, ok2 := detectBelongsToTarget(sf.Type); ok2 {
					target = t
					ok = true
				}
			}
			if kind == "" {
				if ok {
					kind = RelationBelongsTo
				} else if target, ok = detectHasManyTarget(sf.Type); ok {
					kind = RelationHasMany
				}
			}
			if target == nil {
				return ErrInvalidModel.with("parse_model", meta.Name, sf.Name, fmt.Errorf("unable to infer relation target type"))
			}
			rel = &RelationMeta{
				Name:           sf.Name,
				Kind:           kind,
				FieldIndex:     sf.Index,
				TargetType:     target,
				LocalField:     defaultLocalField(meta, sf.Name, kind),
				ForeignRef:     defaultForeignRef(meta, kind),
				JoinTable:      defaultJoinTable(meta, target, kind),
				JoinLocalKey:   defaultJoinLocalKey(meta, kind),
				JoinForeignKey: defaultJoinForeignKey(target, kind),
			}
			meta.Relations[sf.Name] = rel
		}
		if cfg.Kind != "" {
			rel.Kind = cfg.Kind
		}
		if cfg.LocalField != "" {
			rel.LocalField = cfg.LocalField
		}
		if cfg.ForeignRef != "" {
			rel.ForeignRef = cfg.ForeignRef
		}
		if cfg.JoinTable != "" {
			rel.JoinTable = cfg.JoinTable
		}
		if cfg.JoinLocalKey != "" {
			rel.JoinLocalKey = cfg.JoinLocalKey
		}
		if cfg.JoinForeignKey != "" {
			rel.JoinForeignKey = cfg.JoinForeignKey
		}
	}
	return nil
}

func defaultLocalField(meta *ModelMeta, relationName string, kind RelationKind) string {
	switch kind {
	case RelationBelongsTo:
		return relationName + "ID"
	case RelationHasMany:
		if len(meta.PrimaryKeys) == 1 {
			return meta.PrimaryKeys[0].GoName
		}
	}
	return ""
}

func defaultForeignRef(meta *ModelMeta, kind RelationKind) string {
	switch kind {
	case RelationBelongsTo:
		return "ID"
	case RelationHasMany:
		return meta.Name + "ID"
	case RelationManyToMany:
		return "ID"
	}
	return ""
}

func defaultJoinTable(src *ModelMeta, targetType reflect.Type, kind RelationKind) string {
	if kind != RelationManyToMany {
		return ""
	}
	srcName := toSnake(src.Name)
	tgtName := toSnake(targetType.Name())
	return srcName + "_" + tgtName
}

func defaultJoinLocalKey(src *ModelMeta, kind RelationKind) string {
	if kind != RelationManyToMany {
		return ""
	}
	return toSnake(src.Name) + "_id"
}

func defaultJoinForeignKey(targetType reflect.Type, kind RelationKind) string {
	if kind != RelationManyToMany {
		return ""
	}
	return toSnake(targetType.Name()) + "_id"
}

func parseORMRelationTag(tag string) (RelationConfig, bool, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return RelationConfig{}, false, nil
	}
	cfg := RelationConfig{}
	parts := strings.Split(tag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return RelationConfig{}, false, fmt.Errorf("invalid orm tag segment: %s", part)
		}
		key := strings.TrimSpace(strings.ToLower(kv[0]))
		val := strings.TrimSpace(kv[1])
		switch key {
		case "rel", "relation", "kind":
			switch strings.ToLower(val) {
			case string(RelationBelongsTo):
				cfg.Kind = RelationBelongsTo
			case string(RelationHasMany):
				cfg.Kind = RelationHasMany
			case string(RelationManyToMany):
				cfg.Kind = RelationManyToMany
			default:
				return RelationConfig{}, false, fmt.Errorf("unsupported relation kind: %s", val)
			}
		case "local", "local_field":
			cfg.LocalField = val
		case "foreign", "foreign_ref":
			cfg.ForeignRef = val
		case "join_table", "through":
			cfg.JoinTable = val
		case "join_local", "join_local_key":
			cfg.JoinLocalKey = val
		case "join_foreign", "join_foreign_key":
			cfg.JoinForeignKey = val
		default:
			return RelationConfig{}, false, fmt.Errorf("unsupported orm relation option: %s", key)
		}
	}
	return cfg, true, nil
}
