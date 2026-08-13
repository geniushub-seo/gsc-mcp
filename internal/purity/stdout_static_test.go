package purity_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Whitelist of files allowed to reference os.Stdout or fmt.Print* for MCP transport
// or intentional non-MCP CLI output (setup subcommand writes to stderr only — still
// flag fmt.Print to stdout if any).
var allowStdoutFiles = map[string]bool{
	// MCP SDK transport is external; our code must not write stdout except never.
}

func TestNoStdoutPollutionInProductionCode(t *testing.T) {
	root := findModuleRoot(t)
	var offenders []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == "testdata" || base == "vendor" || base == "bin" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if allowStdoutFiles[rel] {
			return nil
		}

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			name := sel.Sel.Name
			// fmt.Print* / println
			if pkg.Name == "fmt" && (strings.HasPrefix(name, "Print") || name == "Println") {
				// Allow Fprint* to non-stdout (checked loosely: first arg not os.Stdout)
				if strings.HasPrefix(name, "Fprint") {
					if writesStdout(call) {
						offenders = append(offenders, rel+": fmt."+name+" to os.Stdout")
					}
					return true
				}
				offenders = append(offenders, rel+": fmt."+name)
			}
			if pkg.Name == "os" && name == "Stdout" {
				// bare reference — catch os.Stdout usage
				offenders = append(offenders, rel+": os.Stdout")
			}
			return true
		})
		// also catch println builtin via Decl — parser sees CallExpr Ident println
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "println" {
				offenders = append(offenders, rel+": println")
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(offenders) > 0 {
		t.Fatalf("stdout pollution risks:\n  %s", strings.Join(offenders, "\n  "))
	}
}

func writesStdout(call *ast.CallExpr) bool {
	if len(call.Args) == 0 {
		return false
	}
	sel, ok := call.Args[0].(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "os" && sel.Sel.Name == "Stdout"
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
