// Package verify contains the verification gates and rules engines for Nomos.
// These engines enforce code quality, architectural boundaries, and security rules.
// This specific file introduces an AST-based complexity parser for Go code.
package verify

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

// maxASTComplexity is the maximum allowed cyclomatic complexity per function before it triggers an error.
const maxASTComplexity = 10

// analyzeASTComplexity parses a Go source string and computes cyclomatic complexity per function.
func analyzeASTComplexity(content, relPath string) ([]ComplexityFinding, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, relPath, content, 0)
	if err != nil {
		return nil, err
	}

	var findings []ComplexityFinding

	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}

		// Initialize base complexity
		// Every function starts with a complexity of 1 (the function itself).

		complexity := 1 // Base complexity is 1

		ast.Inspect(fn.Body, func(bodyNode ast.Node) bool {
			checkNodeComplexity(bodyNode, &complexity)
			return true
		})

		pos := fset.Position(fn.Pos())

		// If complexity exceeds the max threshold, we generate an error finding.
		// If it's approaching the threshold, we generate a non-error warning finding.
		if complexity > maxASTComplexity {
			findings = append(findings, ComplexityFinding{
				File:    relPath,
				Func:    fn.Name.Name,
				Line:    pos.Line,
				Value:   complexity,
				IsError: true,
				Message: fmt.Sprintf("cyclomatic complexity %d for function '%s' violates maximum allowed of %d", complexity, fn.Name.Name, maxASTComplexity),
			})
		} else if complexity > maxASTComplexity-3 {
			findings = append(findings, ComplexityFinding{
				File:    relPath,
				Func:    fn.Name.Name,
				Line:    pos.Line,
				Value:   complexity,
				IsError: false,
				Message: fmt.Sprintf("cyclomatic complexity %d for function '%s' is approaching the limit of %d", complexity, fn.Name.Name, maxASTComplexity),
			})
		}

		return true
	})

	return findings, nil
}

// checkNodeComplexity inspects a single AST node and increments the complexity counter
// if the node represents a branching or looping construct.
// This helper reduces nesting within the main AST inspect function.
func checkNodeComplexity(bodyNode ast.Node, complexity *int) {
	switch node := bodyNode.(type) {
	case *ast.IfStmt:
		// If statements add a branching path
		*complexity++
	case *ast.ForStmt:
		// For loops add a cyclic path
		*complexity++
	case *ast.RangeStmt:
		// Range loops add a cyclic path
		*complexity++
	case *ast.CaseClause:
		// Case clauses add a branching path, but we ignore the default case
		if node.List != nil {
			*complexity++
		}
	case *ast.CommClause:
		// Select communication clauses add a branching path, but we ignore default
		if node.Comm != nil {
			*complexity++
		}
	case *ast.BinaryExpr:
		// Logical AND/OR add implicit branching paths
		if node.Op == token.LAND || node.Op == token.LOR {
			*complexity++
		}
	}
}
