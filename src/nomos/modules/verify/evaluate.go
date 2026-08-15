package verify

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
)

// getComplexityViolationsMap analyzes the complexity of the repository
// using AnalyzeComplexity and maps files with active errors (complexity violations)
// to helper status flags.
func getComplexityViolationsMap(repoRoot string) map[string]bool {
	complexityFindings, _ := AnalyzeComplexity(repoRoot, false)
	hasComplexityViolation := make(map[string]bool)

	// Loop over all findings and register files having complexity errors
	for _, f := range complexityFindings {
		if f.IsError {
			rel := getRelativePath(repoRoot, f.File)
			hasComplexityViolation[rel] = true
		}
	}
	return hasComplexityViolation
}

// isSpecDocumented evaluates if a Go AST specification spec (e.g. variable, constant,
// or type definition) is adequately documented if it is exported.
func isSpecDocumented(spec ast.Spec, doc *ast.CommentGroup) bool {
	switch s := spec.(type) {
	case *ast.TypeSpec:
		// If type spec is exported and has no associated docstrings
		if ast.IsExported(s.Name.Name) && doc == nil {
			return false
		}
	case *ast.ValueSpec:
		// Check all names in a multi-variable value specification
		for _, name := range s.Names {
			if ast.IsExported(name.Name) && doc == nil {
				return false
			}
		}
	}
	return true
}

// isDeclDocumented evaluates if a Go AST declaration (function or general declaration)
// is documented if it exposes any public exported API symbol.
func isDeclDocumented(decl ast.Decl) bool {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		// If function decl is exported and doc comments are missing
		if ast.IsExported(d.Name.Name) && d.Doc == nil {
			return false
		}
	case *ast.GenDecl:
		// Scan through all sub-specs (types, values, imports)
		for _, spec := range d.Specs {
			if !isSpecDocumented(spec, d.Doc) {
				return false
			}
		}

	}
	return true
}

func isBoyScoutDocstringResolved(absPath string) bool {
	fset := token.NewFileSet()
	fileAST, err := parser.ParseFile(fset, absPath, nil, parser.ParseComments)
	if err != nil {
		return false
	}
	for _, decl := range fileAST.Decls {
		if !isDeclDocumented(decl) {
			return false
		}
	}
	return true
}

// isTddCoverageBypassResolved checks if the TDD coverage bypass for a file has been resolved.
// It verifies whether any of the matching test files exist on disk.
func isTddCoverageBypassResolved(repoRoot, relFile string) bool {
	for _, pair := range getTestPairsFor(relFile) {
		if _, err := os.Stat(filepath.Join(repoRoot, pair)); err == nil {
			return true
		}
	}
	return false
}

// isDuplicationLimitBypassResolved checks if the duplication limit bypass has been resolved.
// It builds a project-wide duplication map and calculates the duplication density for the file.
func isDuplicationLimitBypassResolved(repoRoot, relFile string) bool {
	allFiles, err := getProjectFiles(repoRoot)
	if err != nil {
		return false
	}
	allFiltered := filterDuplicationFiles(allFiles)
	windowSize := 10
	hashMap, err := buildProjectDuplicationMap(repoRoot, allFiltered, windowSize)
	if err != nil {
		return false
	}
	dupCount, dupDensity, _, err := auditFileDuplication(repoRoot, relFile, windowSize, hashMap)
	if err != nil {
		return true
	}
	return dupDensity <= 5.0 || dupCount == 0
}

// isDocDriftBypassResolved checks if the document drift bypass has been resolved.
// It checks if the file is staged, and if so, checks if documentation was modified.
func isDocDriftBypassResolved(repoRoot, relFile string) bool {
	staged, err := getStagedFiles(repoRoot)
	if err != nil {
		return false
	}
	isStaged := false
	for _, f := range staged {
		if getRelativePath(repoRoot, f) == relFile {
			isStaged = true
			break
		}
	}
	if !isStaged {
		return true
	}
	_, docsModified := classifyStagedFiles(staged)
	return docsModified
}

// isBasicBypassResolved evaluates formatting, compilation, and complexity bounds.
func isBasicBypassResolved(absPath, relFile, gate string, hasComplexityViolation map[string]bool) bool {
	switch gate {
	case "go_format":
		cmd := exec.Command("gofmt", "-l", absPath)
		var out bytes.Buffer
		cmd.Stdout = &out
		return cmd.Run() == nil && out.Len() == 0
	case "go_vet":
		return exec.Command("go", "vet", absPath).Run() == nil
	case "complexity_limit":
		return !hasComplexityViolation[relFile]
	}
	return false
}

// isMetricBypassResolved evaluates file length and comment density constraints.
func isMetricBypassResolved(absPath, gate string) bool {
	switch gate {
	case "monolithic_file_limit":
		content, err := os.ReadFile(absPath)
		if err == nil {
			lines := strings.Split(string(content), "\n")
			return len(lines) <= 2000
		}
	case "comment_density_limit":
		comments, code, err := calculateCommentDensity(absPath, make(map[string]int))
		if err == nil && code > 0 {
			density := (float64(comments) / float64(code)) * 100.0
			return density >= 10.0
		}
	}
	return false
}

// isBypassResolved evaluates whether the target file has been updated to satisfy the quality gate.
// If resolved, returns true so the bypass is automatically pruned.
func isBypassResolved(repoRoot, absPath, relFile, gate string, hasComplexityViolation map[string]bool) bool {
	if gate == "go_format" || gate == "go_vet" || gate == "complexity_limit" {
		return isBasicBypassResolved(absPath, relFile, gate, hasComplexityViolation)
	}
	if gate == "monolithic_file_limit" || gate == "comment_density_limit" {
		return isMetricBypassResolved(absPath, gate)
	}

	switch gate {
	case "boy_scout_docstring":
		// Check docstrings: if all exported symbols are documented, it's resolved.
		return isBoyScoutDocstringResolved(absPath)
	case "dry_candidate":
		// Delegate DRY candidate check resolution.
		return isDRYCandidateResolved(repoRoot, relFile)
	case "tdd_coverage":
		// Delegate TDD coverage check resolution.
		return isTddCoverageBypassResolved(repoRoot, relFile)
	case "duplication_limit":
		// Delegate duplication limit check resolution.
		return isDuplicationLimitBypassResolved(repoRoot, relFile)
	case "doc_drift":
		// Delegate doc drift check resolution.
		return isDocDriftBypassResolved(repoRoot, relFile)
	}
	return false
}

// isBypassExpired checks if the expiration date/time configured on a technical
// debt item is in the past. It handles multiple time layouts to tolerate manually
// declared expiration dates.
func isBypassExpired(repoRoot string, item QualityDebtItem) bool {
	// 1. Attempt parsing using RFC3339 layout with offset (e.g. 2006-01-02T15:04:05Z)
	expiry, err := time.Parse(time.RFC3339, item.ExpiresAt)
	if err != nil {
		// 2. Fall back to standard ISO-8601 date-time layout without timezone offset
		expiry, err = time.Parse("2006-01-02T15:04:05Z", item.ExpiresAt)
		if err != nil {
			// 3. Fall back to a simple date format (e.g. YYYY-MM-DD)
			expiry, err = time.Parse("2006-01-02", item.ExpiresAt)
		}
	}

	// If parsing succeeds and the current local system time has passed the target expiration
	if err == nil && time.Now().After(expiry) {
		relFile := getRelativePath(repoRoot, item.File)
		// Output red terminal error warning that the debt has officially expired
		synapse.Info("\x1b[31m❌ [Quality Debt Expired] Bypass for '%s' (gate: %s) expired on %s\x1b[0m\n", relFile, item.Gate, item.ExpiresAt)
		return true
	}
	return false
}

func isDRYCandidateResolved(repoRoot, relFile string) bool {
	allFiles, err := getProjectFiles(repoRoot)
	if err != nil {
		return false
	}
	allFiltered := filterDuplicationFiles(allFiles)
	windowSize := 8
	hashMap, err := buildProjectDuplicationMap(repoRoot, allFiltered, windowSize)
	if err != nil {
		return false
	}
	_, _, matches, err := auditFileDuplication(repoRoot, relFile, windowSize, hashMap)
	if err != nil {
		return true // Assume resolved if audit fails
	}
	for _, m := range matches {
		if m.matchFile != relFile {
			return false // Still has cross-file duplicates
		}
	}
	return true
}
