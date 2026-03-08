package orm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GenerateColumnsConst returns Go snippet with sorted DB columns for model.
func GenerateColumnsConst(meta *ModelMeta) (string, error) {
	if meta == nil {
		return "", ErrInvalidModel.with("codegen_columns", "", "", fmt.Errorf("meta is nil"))
	}
	cols := make([]string, 0, len(meta.Fields))
	for _, f := range meta.Fields {
		if f.IsIgnored {
			continue
		}
		cols = append(cols, f.DBName)
	}
	sort.Strings(cols)
	var b strings.Builder
	fmt.Fprintf(&b, "// %sColumns is a generated column list.\n", meta.Name)
	fmt.Fprintf(&b, "var %sColumns = []string{\n", meta.Name)
	for _, c := range cols {
		fmt.Fprintf(&b, "\t%q,\n", c)
	}
	b.WriteString("}\n")
	return b.String(), nil
}

// WriteColumnsConstFile writes generated columns snippet into a file.
func WriteColumnsConstFile(path string, meta *ModelMeta) error {
	src, err := GenerateColumnsConst(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(src), 0o644)
}

// GenerateRepositoryStub generates a typed repository skeleton.
func GenerateRepositoryStub(meta *ModelMeta) (string, error) {
	if meta == nil {
		return "", ErrInvalidModel.with("codegen_repo", "", "", fmt.Errorf("meta is nil"))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "type %sRepo struct {\n\tdb orm.DB\n}\n\n", meta.Name)
	fmt.Fprintf(&b, "func New%sRepo(db orm.DB) *%sRepo { return &%sRepo{db: db} }\n\n", meta.Name, meta.Name, meta.Name)
	if len(meta.PrimaryKeys) == 1 {
		pk := meta.PrimaryKeys[0]
		fmt.Fprintf(&b, "func (r *%sRepo) ByID(ctx context.Context, id %s) (*%s, error) {\n", meta.Name, pk.Type.String(), meta.Name)
		fmt.Fprintf(&b, "\treturn orm.ByPK[%s](ctx, r.db, id)\n}\n", meta.Name)
	}
	return b.String(), nil
}

// WriteRepositoryStubFile writes repository scaffold source into file.
func WriteRepositoryStubFile(path string, meta *ModelMeta) error {
	src, err := GenerateRepositoryStub(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(src), 0o644)
}

// GenerateColumnsConstFromSpec generates columns const snippet without reflection metadata.
func GenerateColumnsConstFromSpec(modelName string, columns []string) (string, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", ErrInvalidQuery.with("codegen_columns_spec", "", "", fmt.Errorf("model name is empty"))
	}
	clean := make([]string, 0, len(columns))
	for _, c := range columns {
		c = strings.TrimSpace(c)
		if c != "" {
			clean = append(clean, c)
		}
	}
	sort.Strings(clean)
	var b strings.Builder
	fmt.Fprintf(&b, "// %sColumns is a generated column list.\n", modelName)
	fmt.Fprintf(&b, "var %sColumns = []string{\n", modelName)
	for _, c := range clean {
		fmt.Fprintf(&b, "\t%q,\n", c)
	}
	b.WriteString("}\n")
	return b.String(), nil
}

// GenerateRepositoryStubFromSpec generates repository skeleton from explicit names.
func GenerateRepositoryStubFromSpec(modelName, pkType string) (string, error) {
	modelName = strings.TrimSpace(modelName)
	pkType = strings.TrimSpace(pkType)
	if modelName == "" || pkType == "" {
		return "", ErrInvalidQuery.with("codegen_repo_spec", "", "", fmt.Errorf("modelName and pkType are required"))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "type %sRepo struct {\n\tdb orm.DB\n}\n\n", modelName)
	fmt.Fprintf(&b, "func New%sRepo(db orm.DB) *%sRepo { return &%sRepo{db: db} }\n\n", modelName, modelName, modelName)
	fmt.Fprintf(&b, "func (r *%sRepo) ByID(ctx context.Context, id %s) (*%s, error) {\n", modelName, pkType, modelName)
	fmt.Fprintf(&b, "\treturn orm.ByPK[%s](ctx, r.db, id)\n}\n", modelName)
	return b.String(), nil
}

// CodegenModelSpec defines one model entry for pipeline generation.
type CodegenModelSpec struct {
	Name               string   `json:"name"`
	Columns            []string `json:"columns"`
	PKType             string   `json:"pk_type"`
	GenerateColumns    bool     `json:"generate_columns"`
	GenerateRepository bool     `json:"generate_repository"`
}

// CodegenConfig is an input file for multi-file code generation.
type CodegenConfig struct {
	Package       string             `json:"package"`
	OutputDir     string             `json:"output_dir"`
	ORMImport     string             `json:"orm_import"`
	Header        string             `json:"header"`
	Models        []CodegenModelSpec `json:"models"`
	GenerateIndex bool               `json:"generate_index"`
}

// LoadCodegenConfig loads JSON codegen config from file.
func LoadCodegenConfig(path string) (*CodegenConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg CodegenConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, ErrInvalidQuery.with("codegen_config", "", "", err)
	}
	if err := validateCodegenConfig(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// RunCodegenPipeline generates all files described by JSON config.
func RunCodegenPipeline(configPath string) error {
	cfg, err := LoadCodegenConfig(configPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil {
		return err
	}
	for _, m := range cfg.Models {
		base := toSnake(m.Name)
		if m.GenerateColumns {
			src, err := renderColumnsFile(cfg, m)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(cfg.OutputDir, base+"_columns_gen.go"), []byte(src), 0o644); err != nil {
				return err
			}
		}
		if m.GenerateRepository {
			src, err := renderRepoFile(cfg, m)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(cfg.OutputDir, base+"_repo_gen.go"), []byte(src), 0o644); err != nil {
				return err
			}
		}
	}
	if cfg.GenerateIndex {
		src, err := renderIndexFile(cfg)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(cfg.OutputDir, "orm_codegen_index_gen.go"), []byte(src), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func validateCodegenConfig(cfg *CodegenConfig) error {
	if cfg == nil {
		return ErrInvalidQuery.with("codegen_config", "", "", fmt.Errorf("config is nil"))
	}
	cfg.Package = strings.TrimSpace(cfg.Package)
	cfg.OutputDir = strings.TrimSpace(cfg.OutputDir)
	cfg.ORMImport = strings.TrimSpace(cfg.ORMImport)
	if cfg.Package == "" {
		return ErrInvalidQuery.with("codegen_config", "", "", fmt.Errorf("package is required"))
	}
	if cfg.OutputDir == "" {
		return ErrInvalidQuery.with("codegen_config", "", "", fmt.Errorf("output_dir is required"))
	}
	if cfg.ORMImport == "" {
		cfg.ORMImport = "github.com/pafthang/orm"
	}
	if len(cfg.Models) == 0 {
		return ErrInvalidQuery.with("codegen_config", "", "", fmt.Errorf("models are required"))
	}
	seen := map[string]struct{}{}
	for i := range cfg.Models {
		m := &cfg.Models[i]
		m.Name = strings.TrimSpace(m.Name)
		m.PKType = strings.TrimSpace(m.PKType)
		if m.Name == "" {
			return ErrInvalidQuery.with("codegen_config", "", "", fmt.Errorf("model name is required"))
		}
		if _, ok := seen[m.Name]; ok {
			return ErrConflict.with("codegen_config", "", m.Name, fmt.Errorf("duplicate model"))
		}
		seen[m.Name] = struct{}{}
		if m.PKType == "" {
			m.PKType = "int64"
		}
		if !m.GenerateColumns && !m.GenerateRepository {
			m.GenerateColumns = true
			m.GenerateRepository = true
		}
		clean := make([]string, 0, len(m.Columns))
		for _, c := range m.Columns {
			c = strings.TrimSpace(c)
			if c != "" {
				clean = append(clean, c)
			}
		}
		m.Columns = clean
	}
	return nil
}

func renderColumnsFile(cfg *CodegenConfig, m CodegenModelSpec) (string, error) {
	body, err := GenerateColumnsConstFromSpec(m.Name, m.Columns)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	writeCodegenHeader(&b, cfg)
	fmt.Fprintf(&b, "package %s\n\n", cfg.Package)
	b.WriteString(body)
	return b.String(), nil
}

func renderRepoFile(cfg *CodegenConfig, m CodegenModelSpec) (string, error) {
	stub, err := GenerateRepositoryStubFromSpec(m.Name, m.PKType)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	writeCodegenHeader(&b, cfg)
	fmt.Fprintf(&b, "package %s\n\n", cfg.Package)
	b.WriteString("import (\n")
	b.WriteString("\t\"context\"\n")
	fmt.Fprintf(&b, "\torm %q\n", cfg.ORMImport)
	b.WriteString(")\n\n")
	b.WriteString(stub)
	return b.String(), nil
}

func renderIndexFile(cfg *CodegenConfig) (string, error) {
	var b strings.Builder
	writeCodegenHeader(&b, cfg)
	fmt.Fprintf(&b, "package %s\n\n", cfg.Package)
	b.WriteString("// GeneratedModelColumns maps model type to generated column list.\n")
	b.WriteString("var GeneratedModelColumns = map[string][]string{\n")
	for _, m := range cfg.Models {
		if !m.GenerateColumns {
			continue
		}
		fmt.Fprintf(&b, "\t%q: %sColumns,\n", m.Name, m.Name)
	}
	b.WriteString("}\n")
	return b.String(), nil
}

func writeCodegenHeader(b *strings.Builder, cfg *CodegenConfig) {
	b.WriteString("// Code generated by ormtool. DO NOT EDIT.\n")
	if s := strings.TrimSpace(cfg.Header); s != "" {
		b.WriteString("// ")
		b.WriteString(s)
		b.WriteString("\n")
	}
	b.WriteString("\n")
}
