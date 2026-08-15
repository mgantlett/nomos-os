package verify

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// CheckGoroutineLifecycle audits staged command files for untracked goroutines.
func CheckGoroutineLifecycle(root string) error {
	staged, err := getStagedFiles(root)
	if err != nil {
		return nil
	}

	var violations []string

	for _, f := range staged {
		fSlash := filepath.ToSlash(f)
		// Only inspect Go files inside src/nomos/cmd/
		if !strings.HasSuffix(fSlash, ".go") || !strings.Contains(fSlash, "src/nomos/cmd/") {
			continue
		}

		absPath := filepath.Join(root, f)
		fset := token.NewFileSet()
		fileAST, err := parser.ParseFile(fset, absPath, nil, parser.ParseComments)
		if err != nil {
			continue
		}

		ast.Inspect(fileAST, func(n ast.Node) bool {
			goStmt, ok := n.(*ast.GoStmt)
			if !ok {
				return true
			}

			// Check if the go statement is tracked by a WaitGroup.
			// We look for a call to a function (usually anonymous) that has a deferred call to `.Done()`
			tracked := false
			pos := fset.Position(goStmt.Pos())

			if funcLit, ok := goStmt.Call.Fun.(*ast.FuncLit); ok {
				// Scan statements inside the function literal for defer statement calling Done()
				for _, stmt := range funcLit.Body.List {
					if deferStmt, ok := stmt.(*ast.DeferStmt); ok {
						if selectorExpr, ok := deferStmt.Call.Fun.(*ast.SelectorExpr); ok {
							if selectorExpr.Sel.Name == "Done" {
								tracked = true
								break
							}
						}
					}
				}
			}

			if !tracked {
				violations = append(violations, fmt.Sprintf("%s:%d: raw untracked goroutine found. Use sync.WaitGroup (with defer wg.Done()) or a LifecycleRunner", f, pos.Line))
			}

			return true
		})
	}

	if len(violations) > 0 {
		return fmt.Errorf("goroutine lifecycle check failed:\n - %s", strings.Join(violations, "\n - "))
	}

	return nil
}
