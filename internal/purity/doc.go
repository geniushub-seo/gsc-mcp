// Package purity holds repo-hygiene checks (live doc byte budgets). The checks
// live in external tests; this file exists so the package is never an empty
// go/packages result (golangci-lint 2.12.2 + Go 1.26 can report "no go files to
// analyze" for a tests-only directory). Stdout purity is enforced at runtime by
// cmd/gsc-mcp/stdio_purity_test.go, which spawns the binary.
package purity
