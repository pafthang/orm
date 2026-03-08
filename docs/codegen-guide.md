# Codegen Guide

`orm` provides two codegen modes:

- single artifact (`ormtool codegen columns|repo`)
- pipeline mode from config (`ormtool codegen run`)

## Single Artifact

```bash
go run ./cmd/ormtool codegen columns --name User --columns id,email,name --output ./user_columns_gen.go
go run ./cmd/ormtool codegen repo --name User --pk-type int64 --output ./user_repo_gen.go
```

## Pipeline Mode

Create config `orm.codegen.json`:

```json
{
  "package": "repository",
  "output_dir": "./internal/repository/generated",
  "orm_import": "github.com/pafthang/orm",
  "header": "Repository artifacts for service X",
  "generate_index": true,
  "models": [
    {
      "name": "User",
      "columns": ["id", "email", "name", "created_at"],
      "pk_type": "int64",
      "generate_columns": true,
      "generate_repository": true
    }
  ]
}
```

Run:

```bash
go run ./cmd/ormtool codegen run --config ./orm.codegen.json
```

Generated files:

- `<model>_columns_gen.go`
- `<model>_repo_gen.go`
- `orm_codegen_index_gen.go` (when `generate_index=true`)

## Programmatic API

```go
cfg, err := orm.LoadCodegenConfig("./orm.codegen.json")
if err != nil { return err }

_ = cfg
err = orm.RunCodegenPipeline("./orm.codegen.json")
```
