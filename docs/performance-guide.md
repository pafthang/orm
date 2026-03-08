# Performance Guide

## Benchmark Commands

```bash
make bench
make bench-compare
make bench-assert
```

## Compare Suite

Compare benchmarks run across:
- `orm`
- `dbx`
- `database/sql`

Covered scenarios:
- by primary key
- insert one row
- list query
- update one row
- batch insert

## Assertions and Baseline

`bench-assert` validates benchmark output against:
- `perf/baseline.tsv`

If a benchmark exceeds threshold, command fails.

## Interpreting Results

- Focus on trend, not single run variance.
- Run on stable machine class when updating baseline.
- Keep schema/data shape representative for your services.

## Tuning Pointers

- Minimize preload graph width for high-cardinality lists.
- Use `PreloadChunkSize` for relation-heavy paths.
- Prefer metadata-safe filters before unsafe expressions.
- Keep transactions short.
