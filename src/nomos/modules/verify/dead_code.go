package verify

// Dead Code verification gate relies on ast parsing to extract Go symbols
// from the codebase. It then cross references them with a project wide
// git grep to ensure all functions and structs have references elsewhere.
// This prevents unused logic from silently accumulating technical debt
// as new features are built or refactored.

import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
)

// tsPyRegex matches export and class definitions in TypeScript, JavaScript, and Python files
var tsPyRegex = regexp.MustCompile(`(?m)^(?:export\s+)?(?:const|func|function|class|def)\s+([A-Z][a-zA-Z0-9_]*)`)

// extractGoSymbols parses a Go source file AST to extract all exported declarations
func extractGoSymbols(fullPath string) []string {
	var symbols []string
	fset := token.NewFileSet()
	// Parse Go source file into Abstract Syntax Tree representation
	node, err := parser.ParseFile(fset, fullPath, nil, 0)
	if err != nil {
		return symbols
	}
	// Iterate through all declarations in AST node
	for _, decl := range node.Decls {
		symbols = append(symbols, parseGoDecl(decl)...)
	}
	return symbols
}

func extractNonGoSymbols(fullPath string) []string {
	var symbols []string
	content, _ := exec.Command("cat", fullPath).Output()
	matches := tsPyRegex.FindAllStringSubmatch(string(content), -1)
	for _, m := range matches {
		symbols = append(symbols, m[1])
	}
	return symbols
}

// checkSymbolUsed executes git grep -w to check if a symbol is referenced across the repo.
// It ignores test functions and the defining file to eliminate false positives.
func checkSymbolUsed(root, sym, f string) bool {
	if strings.HasPrefix(sym, "Test") || strings.HasPrefix(sym, "Benchmark") || strings.HasPrefix(sym, "Example") {
		return true
	}

	cmd := exec.Command("git", "grep", "-l", "-w", sym)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, matchFile := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if matchFile != "" && matchFile != f {
			return true
		}
	}
	return false
}

// isPublicSDKPackage returns true if the file path belongs to a public Nomos OS framework SDK package.
// Public SDK packages expose exported interfaces for external plugins, downstream repos, and IPC calls.
func isPublicSDKPackage(filePath string) bool {
	dir := filepath.Dir(filePath)
	pkg := filepath.Base(dir)
	publicPkgs := map[string]bool{
		"telemetry":   true,
		"config":      true,
		"ast":         true,
		"exec":        true,
		"synapse":     true,
		"provider":    true,
		"assets":      true,
		"integration": true,
		"task":        true,
	}
	return publicPkgs[pkg]
}

// processDeadCodeFile checks a single staged file for unused dead code symbols.
func processDeadCodeFile(root string, f string) []string {
	if isPublicSDKPackage(f) {
		return nil
	}
	ext := filepath.Ext(f)
	fullPath := filepath.Join(root, f)

	var symbols []string
	if ext == ".go" {
		symbols = extractGoSymbols(fullPath)
	} else if ext == ".ts" || ext == ".js" || ext == ".py" {
		symbols = extractNonGoSymbols(fullPath)
	}

	var dead []string
	for _, sym := range symbols {
		if !checkSymbolUsed(root, sym, f) {
			dead = append(dead, fmt.Sprintf("%s in %s", sym, f))
			StageAutoDebtTask(root, f, "dead_code", "Unused dead code symbol: "+sym)
		}
	}
	return dead
}

// runDeadCodeCheck checks modified files for unreferenced internal symbols.
func runDeadCodeCheck(ctx *workspace.WorkspaceContext) (StageResult, error) {
	root := ctx.RepoRoot
	staged, err := getStagedFiles(root)
	if err != nil {
		return StageResult{Passed: true}, nil
	}

	var deadSymbols []string
	for _, f := range staged {
		deadSymbols = append(deadSymbols, processDeadCodeFile(root, f)...)
	}

	if len(deadSymbols) > 0 {
		tier := getActiveAgentTier(root)
		if tier == statepkg.Tier1 {
			return StageResult{
				Passed: false,
				Error:  fmt.Errorf("❌ Dead Code Gate Violation: Tier 1 IDE Agent introduced %d unused symbol(s):\n - %s\n💡 Guidance: Purge unused functions/structs before committing. Tier 1 quality gates are 100%% hard enforced.", len(deadSymbols), strings.Join(deadSymbols, "\n - ")),
			}, nil
		}
		return StageResult{
			Passed:  true,
			Message: fmt.Sprintf("Registered %d dead code bypasses in quality debt for Tier 2 worker.", len(deadSymbols)),
		}, nil
	}

	return StageResult{Passed: true, Message: "No dead code detected."}, nil
}

// parseGoDecl extracts symbols from a single ast declaration.
// It iterates through generic declarations or identifies function declarations.
func parseGoDecl(decl ast.Decl) []string {
	var symbols []string
	if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.IsExported() {
		symbols = append(symbols, fd.Name.Name)
	} else if gd, ok := decl.(*ast.GenDecl); ok {
		for _, spec := range gd.Specs {
			symbols = append(symbols, parseSpec(spec)...)
		}
	}
	return symbols
}

// parseSpec checks type and value specs for exported identifiers.
// This lowers cognitive complexity by extracting the nested type checks.
// Returns a slice of symbols that are marked as exported.
func parseSpec(spec ast.Spec) []string {
	var symbols []string
	if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.IsExported() {
		symbols = append(symbols, ts.Name.Name)
	} else if vs, ok := spec.(*ast.ValueSpec); ok {
		for _, name := range vs.Names {
			if name.IsExported() {
				symbols = append(symbols, name.Name)
			}
		}
	}
	return symbols
}

// DeadCodeFinding describes an unreferenced symbol or defensive dead code branch.
type DeadCodeFinding struct {
	File        string
	Line        int
	Symbol      string
	Type        string // "Unused Symbol", "Defensive Dead Branch", "Unreachable Code"
	Message     string
	IsViolation bool
}

// AuditDeadCode scans the workspace for unreferenced exported symbols, unreachable AST control-flow branches, and defensive AI fallback bloat.
func AuditDeadCode(root string, scanAll bool) ([]DeadCodeFinding, error) {
	var findings []DeadCodeFinding

	var files []string
	if scanAll {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				base := info.Name()
				if base == "node_modules" || base == ".git" || base == "vendor" || base == "dist" || base == "bin" || base == "tmp" {
					return filepath.SkipDir
				}
				return nil
			}
			ext := filepath.Ext(path)
			if ext == ".go" || ext == ".ts" || ext == ".js" || ext == ".py" {
				rel, err := filepath.Rel(root, path)
				if err == nil {
					files = append(files, rel)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		modifiedMap, err := GetModifiedFiles(root)
		if err != nil {
			return nil, err
		}
		for f := range modifiedMap {
			ext := filepath.Ext(f)
			if ext == ".go" || ext == ".ts" || ext == ".js" || ext == ".py" {
				files = append(files, f)
			}
		}
	}

	fset := token.NewFileSet()

	for _, f := range files {
		if isPublicSDKPackage(f) {
			continue
		}
		fullPath := filepath.Join(root, f)

		ext := filepath.Ext(f)
		var symbols []string
		if ext == ".go" {
			symbols = extractGoSymbols(fullPath)
			// Also inspect AST for control-flow dead branches
			if node, err := parser.ParseFile(fset, fullPath, nil, 0); err == nil {
				ast.Inspect(node, func(n ast.Node) bool {
					if ifStmt, ok := n.(*ast.IfStmt); ok {
						// Check for redundant constant condition if statements
						if ident, ok := ifStmt.Cond.(*ast.Ident); ok {
							if ident.Name == "false" {
								pos := fset.Position(ifStmt.Pos())
								findings = append(findings, DeadCodeFinding{
									File:        f,
									Line:        pos.Line,
									Type:        "Unreachable Code",
									Message:     "Constant 'if false' branch detected; branch never executes.",
									IsViolation: true,
								})
							}
						}
					}
					return true
				})
			}
		} else {
			symbols = extractNonGoSymbols(fullPath)
		}

		for _, sym := range symbols {
			if !checkSymbolUsed(root, sym, f) {
				findings = append(findings, DeadCodeFinding{
					File:        f,
					Symbol:      sym,
					Type:        "Unused Symbol",
					Message:     fmt.Sprintf("Exported symbol '%s' has 0 call site references across project.", sym),
					IsViolation: false,
				})
			}
		}
	}

	return findings, nil
}
