package verify

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// targetFunctions defines the set of domain-critical functions that we strictly
// monitor to prevent magic strings from being passed instead of typed domain constants.
var targetFunctions = map[string]bool{
	"tracker.Create": true,
	"tracker.Edit":   true,
	"task.Create":    true,
	"task.Edit":      true,
}

// CheckMagicStrings scans modified source code files to ensure domain-critical
// functions are not being called with raw "magic strings" instead of typed domain constants.
// This enforces compile-time type safety over loose string routing.
// It skips all non-Go files and any test files to keep noise down.
func CheckMagicStrings(root string, files []string) []ComplexityFinding {
	var findings []ComplexityFinding

	// Iterate through each staged or modified file in the workspace context.
	for _, f := range files {
		fSlash := filepath.ToSlash(f)

		// Optimization: We only care about standard Go logic files.
		// We explicitly bypass unit test files to avoid false positives during test fixtures.
		if strings.HasSuffix(fSlash, "_test.go") || !strings.HasSuffix(fSlash, ".go") {
			continue
		}

		absPath := filepath.Join(root, f)
		fset := token.NewFileSet()

		// Parse the AST syntax tree for the target source code file.
		node, err := parser.ParseFile(fset, absPath, nil, 0)
		if err != nil {
			continue
		}

		// Traverse the AST tree nodes and collect any violations found within function calls.
		fileFindings := inspectFileForMagicStrings(f, fset, node)
		findings = append(findings, fileFindings...)
	}

	return findings
}

// inspectFileForMagicStrings runs the internal AST node traversal logic
// isolating complexity from the main scanner loop. It specifically looks for CallExpr.
func inspectFileForMagicStrings(f string, fset *token.FileSet, node *ast.File) []ComplexityFinding {
	var fileFindings []ComplexityFinding

	// We use ast.Inspect to dynamically walk down the entire syntax tree.
	ast.Inspect(node, func(n ast.Node) bool {
		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Resolve the fully qualified function identifier string
		funcName := extractFunctionName(callExpr)

		// If this is a monitored function, validate its arguments for raw literals.
		if targetFunctions[funcName] {
			fileFindings = append(fileFindings, validateCallArgs(f, fset, funcName, callExpr)...)
		}
		return true
	})

	return fileFindings
}

// extractFunctionName isolates the logic for unwrapping the AST selector expression
// into a standardized package.Function format string.
func extractFunctionName(callExpr *ast.CallExpr) string {
	if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
		if ident, ok := selExpr.X.(*ast.Ident); ok {
			return fmt.Sprintf("%s.%s", ident.Name, selExpr.Sel.Name)
		}
	}
	return ""
}

// validateCallArgs iterates over the arguments of a function call to determine
// if a raw string literal was mistakenly passed instead of a predefined constant.
func validateCallArgs(f string, fset *token.FileSet, funcName string, callExpr *ast.CallExpr) []ComplexityFinding {
	var argsFindings []ComplexityFinding

	// Loop over every argument supplied to the function call
	for _, arg := range callExpr.Args {
		if basicLit, ok := arg.(*ast.BasicLit); ok {
			// BasicLit encompasses ints, floats, chars, and strings. We only flag strings.
			if basicLit.Kind == token.STRING {
				argsFindings = append(argsFindings, ComplexityFinding{
					File:    f,
					Func:    fmt.Sprintf("raw magic string passed to %s()", funcName),
					Line:    fset.Position(basicLit.Pos()).Line,
					Value:   0,
					IsError: true,
				})
			}
		}
	}

	return argsFindings
}
