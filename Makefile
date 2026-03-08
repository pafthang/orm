.PHONY: test test-matrix bench bench-compare bench-assert

test:
	go test ./...

test-matrix:
	./scripts/test-matrix.sh

bench:
	go test -run '^$$' -bench Benchmark -benchmem ./...

bench-compare:
	go test -run '^$$' -bench BenchmarkCompare -benchmem ./...

bench-assert:
	./scripts/bench-assert.sh
