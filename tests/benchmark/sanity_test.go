package benchmark

import "testing"

// TestBenchmarkSanity exists so the package participates in the default
// `go test` run — every other file here only contains `Benchmark*`
// functions, which `go test` skips without `-bench`. Without this test
// `go test ./tests/benchmark/...` reports "[no tests to run]" and gives
// the false impression the benchmark harness is missing entirely.
//
// The actual benchmarks run via `bash scripts/bench.sh` (or `go test
// -bench=. ./tests/benchmark/`). Keeping a trivial Test here also
// ensures the benchmark code stays compilable under the default test
// runner — catches a refactor that accidentally removes a type used
// only by benchmarks.
func TestBenchmarkSanity(t *testing.T) {
	t.Helper()
	// If the package compiles and the test binary is produced, we're good.
}
