// Package verify contains the verification gates and rules engines for Nomos.
// These engines are designed to be language-agnostic and analyze files across multiple languages.
// This file focuses on parsing and measuring code complexity metrics.
package verify

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
)

// ComplexityFinding represents a cyclomatic complexity, cognitive complexity, maintainability index, or nesting violation.
type ComplexityFinding struct {
	File    string
	Func    string // Name of the function, empty for non-Go files
	Line    int
	Value   int // Complexity value or score
	IsError bool
	Message string
}

// AnalyzeComplexity scans files for Go complexity metrics or non-Go indentation nesting.
// It parses the file line by line and computes nesting levels based on standard curly braces.
// High complexity functions often correlate with bugs and high churn rates, so we track them carefully.
// It iterates through the project file list and applies specific complexity algorithms
// depending on the file extension and language detected.
func AnalyzeComplexity(root string, stagedOnly bool) ([]ComplexityFinding, error) {
	// Complexity limits are strictly enforced. We use a target line nesting limit of 3,
	// but currently the test suite permits up to 4 for certain legacy patterns.
	// We establish `limit` at 4 to provide a tiny buffer before CI fails.
	var files []string
	var err error

	if stagedOnly {
		// Read staging index to see which files the developer is modifying.
		files, err = getStagedFiles(root)
	} else {
		// Retrieve all files in the project workspace if not in staged mode.
		files, err = getProjectFiles(root)
	}
	if err != nil {
		return nil, err
	}

	// We only want to analyze actual logic files (e.g. .go, .py, .js)
	// We skip over test files to prevent test suites from breaking the build.
	var findings []ComplexityFinding

	for _, f := range files {
		findings = append(findings, analyzeComplexityFile(root, f)...)
	}

	return findings, nil
}

// isInternalTestFile determines if the given path is a test file.
// We strictly exclude test logic from complexity scoring.
func isInternalTestFile(fSlash string) bool {
	return strings.HasSuffix(fSlash, "_test.go") || strings.HasSuffix(fSlash, "_tests.go") || strings.HasSuffix(fSlash, ".test.js")
}

// isIgnoredPath determines if the file resides in an ignored vendor, dist, or UI asset directory.
// We ignore compiled UI paths, embedded UI frontend assets, and third party packages to avoid false positives.
func isIgnoredPath(fSlash string) bool {
	return strings.Contains(fSlash, "/dist/") || strings.HasPrefix(fSlash, "dist/") || strings.Contains(fSlash, "/node_modules/") || strings.HasPrefix(fSlash, "node_modules/") || strings.Contains(fSlash, "/control-plane-ui/") || strings.HasPrefix(fSlash, "src/control-plane-ui/") || strings.Contains(fSlash, "/modules/cockpit/ui/")
}

// analyzeComplexityFile checks specific paths and extensions to see if a file
// should be included in the complexity analysis.
func analyzeComplexityFile(root, f string) []ComplexityFinding {
	fSlash := filepath.ToSlash(f)
	if config.IsInternalSystemDir(fSlash) || isInternalTestFile(fSlash) || isIgnoredPath(fSlash) {
		return nil
	}

	var findings []ComplexityFinding
	ext := filepath.Ext(fSlash)
	absPath := filepath.Join(root, f)

	if isNestingCheckedExtension(ext) {
		var fileFindings []ComplexityFinding
		var err error

		if ext == ".go" {
			contentBytes, readErr := os.ReadFile(absPath)
			if readErr == nil {
				fileFindings, err = analyzeASTComplexity(string(contentBytes), f)
			}
		} else {
			fileFindings, err = analyzeNesting(absPath, f)
		}

		if err == nil {
			findings = append(findings, fileFindings...)
		}
	}
	return findings
}

// isNestingCheckedExtension determines whether a given file extension should be subjected
// to structural nesting complexity checks. Supported extensions cover the majority of
// backend and frontend logic source files.
func isNestingCheckedExtension(ext string) bool {
	validExts := map[string]bool{
		".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
		".py": true, ".rs": true, ".html": true, ".css": true, ".c": true,
		".cpp": true, ".h": true, ".hpp": true, ".java": true, ".sh": true,
		".bash": true, ".nix": true, ".rb": true, ".php": true,
	}
	return validExts[ext]
}

func analyzeNesting(absPath, relPath string) ([]ComplexityFinding, error) {
	file, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return analyzeNestingLines(lines, relPath), nil
}

func analyzeNestingLines(lines []string, relPath string) []ComplexityFinding {
	indentSize := detectIndentSize(lines)
	var findings []ComplexityFinding

	for idx, line := range lines {
		level := lineNestingLevel(line, indentSize)
		if level > 4 {
			findings = append(findings, ComplexityFinding{
				File:    relPath,
				Line:    idx + 1,
				Value:   level,
				IsError: true,
				Message: fmt.Sprintf("line nesting level %d violates maximum allowed of 4", level),
			})
		} else if level > 3 {
			findings = append(findings, ComplexityFinding{
				File:    relPath,
				Line:    idx + 1,
				Value:   level,
				IsError: false,
				Message: fmt.Sprintf("line nesting level %d exceeds recommended level of 3", level),
			})
		}
	}

	return findings
}

func detectIndentSize(lines []string) int {
	for _, l := range lines {
		trimmed := strings.TrimLeft(l, " \t")
		if len(trimmed) == 0 || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}
		leading := l[:len(l)-len(trimmed)]
		if strings.Contains(leading, "\t") {
			return 0 // Tab-based
		}
		spaces := len(leading)
		if spaces > 0 {
			return spaces
		}
	}
	return 4 // Fallback
}

func lineNestingLevel(line string, indentSize int) int {
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) == 0 || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
		return 0
	}
	leading := line[:len(line)-len(trimmed)]
	if indentSize == 0 {
		return strings.Count(leading, "\t")
	}
	return len(leading) / indentSize
}

// DidComplexityWorsen compares a current complexity finding against the HEAD version of the file.
// It returns true if the metric genuinely worsened, or false if it was already that bad in HEAD.
func DidComplexityWorsen(root, relFile string, current ComplexityFinding) bool {
	headContent, err := runGit(root, "show", "HEAD:"+relFile)
	if err != nil {
		return true // New file or error getting HEAD, assume it worsened (strict enforcement)
	}

	ext := filepath.Ext(relFile)
	if ext == ".go" {
		headFindings, _ := analyzeASTComplexity(headContent, relFile)
		return isGoComplexityWorsened(headFindings, current)
	} else {
		// For non-go files, we check if the new max nesting is <= old max nesting overall
		lines := strings.Split(headContent, "\n")
		headFindings := analyzeNestingLines(lines, relFile)

		headMax := 0
		for _, hf := range headFindings {
			if hf.Value > headMax {
				headMax = hf.Value
			}
		}

		if current.Value <= headMax {
			return false // Overall max nesting didn't worsen
		}
		return true
	}
}

// isGoComplexityWorsened checks if the complexity of a Go function has worsened compared to HEAD.
// It iterates through the historical findings to find a match and compares values.
func isGoComplexityWorsened(headFindings []ComplexityFinding, current ComplexityFinding) bool {
	for _, hf := range headFindings {
		if hf.Func == current.Func {
			if current.Value <= hf.Value {
				return false // Did not worsen
			}
			return true // Worsened
		}
	}
	return true // Function not found in HEAD, meaning it's new, so it worsened
}
