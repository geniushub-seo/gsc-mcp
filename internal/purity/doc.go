// Package purity holds compile-time checks that production code does not write
// to stdout. The checks live in external tests; this file exists so the
// package is never an empty go/packages result (golangci-lint 2.12.2 + Go 1.26
// can report "no go files to analyze" for a tests-only directory).
package purity
