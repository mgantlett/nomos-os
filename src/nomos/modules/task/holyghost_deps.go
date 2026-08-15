package task

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/ast"
	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
)

// writeDependencySignaturesIfCompactEnabled writes structural signatures for dependency packages
// if context compaction is enabled in phase state.
func writeDependencySignaturesIfCompactEnabled(f *strings.Builder, repoRoot string, taskKey string) {
	// 1. Check if compact context is enabled in phase state
	if !isCompactEnabled(repoRoot) {
		return
	}

	// 2. Identify dependency package directories
	depDirs := getDependencyDirs(repoRoot, taskKey)
	if len(depDirs) == 0 {
		return
	}

	// 3. Document the dependency signatures block header
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "## Dependency Package Signatures")
	fmt.Fprintln(f, "The structural interface signatures and type specifications for local package dependencies:")

	// 4. Iterate over dependency packages and extract AST declarations
	for depDir := range depDirs {
		writePkgSigs(f, repoRoot, depDir)
	}
}

// isCompactEnabled reads phase state to check if compact_context is set.
func isCompactEnabled(repoRoot string) bool {
	phaseStatePath := config.PhaseStatePath(repoRoot)
	if data, err := os.ReadFile(phaseStatePath); err == nil {
		var state struct {
			CompactContext bool `json:"compact_context"`
		}
		if err := json.Unmarshal(data, &state); err == nil {
			return state.CompactContext
		}
	}
	return false
}

// getDependencyDirs parses implementation plan files and extracts local dependency package directories.
func getDependencyDirs(repoRoot string, taskKey string) map[string]bool {
	planPath := filepath.Join(repoRoot, ".agent", "specs", taskKey, "implementation_plan.md")
	plannedFiles := parseSpecFilesLocal(planPath, repoRoot)
	if len(plannedFiles) == 0 {
		return nil
	}

	// Group planned files by package directory
	activeDirs := make(map[string]bool)
	for _, pf := range plannedFiles {
		if strings.HasSuffix(pf, ".go") {
			activeDirs[filepath.Dir(pf)] = true
		}
	}

	// Extract local dependencies from Go file imports
	dependencyDirs := make(map[string]bool)
	for _, pf := range plannedFiles {
		if strings.HasSuffix(pf, ".go") {
			absFile := filepath.Join(repoRoot, pf)
			extractFileDeps(absFile, activeDirs, dependencyDirs)
		}
	}
	return dependencyDirs
}

// extractFileDeps reads imports of the file and marks non-active local package dependencies.
func extractFileDeps(absFile string, activeDirs map[string]bool, dependencyDirs map[string]bool) {
	imports, err := ast.ParseImports(absFile)
	if err != nil {
		return
	}
	const prefix = "github.com/mgantlett/nomos-commons/"
	for _, imp := range imports {
		if strings.HasPrefix(imp, prefix) {
			relDir := strings.TrimPrefix(imp, prefix)
			if !activeDirs[relDir] {
				dependencyDirs[relDir] = true
			}
		}
	}
}

// writePkgSigs parses package files and writes signature declarations to output buffer.
func writePkgSigs(f *strings.Builder, repoRoot string, depDir string) {
	absDepDir := filepath.Join(repoRoot, depDir)
	entries, err := os.ReadDir(absDepDir)
	if err != nil {
		return
	}
	pkgName := filepath.Base(depDir)
	fmt.Fprintf(f, "\n### Package %s\n", pkgName)

	// Scan through Go source files, omitting test code files
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		absFile := filepath.Join(absDepDir, entry.Name())
		sig, err := ast.ExtractSignatures(absFile)
		if err != nil || strings.TrimSpace(sig) == "" {
			continue
		}
		fmt.Fprintf(f, "\n#### File: %s\n", filepath.Join(depDir, entry.Name()))
		fmt.Fprintf(f, "```go\n%s\n```\n", strings.TrimSpace(sig))
	}
}

// parseSpecFilesLocal scans the spec implementation plan for active task changes.
func parseSpecFilesLocal(planPath string, repoRoot string) []string {
	file, err := os.Open(planPath)
	if err != nil {
		return nil
	}
	defer file.Close()

	var files []string
	scanner := bufio.NewScanner(file)
	inProposed := false

	// Scan spec file line-by-line using regular expressions
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Track if current line is inside the proposed changes section block
		inProposed = updateProposedSectionFlag(line, inProposed)
		if !inProposed {
			continue
		}

		if p := extractSpecPathFromLine(line, repoRoot); p != "" {
			files = append(files, p)
		}
	}
	return files
}

// updateProposedSectionFlag toggles parsing context state when matching section headers.
func updateProposedSectionFlag(line string, current bool) bool {
	rxProposedChanges := regexp.MustCompile(`(?i)^##\s+Proposed\s+Changes`)
	rxSectionHeader := regexp.MustCompile(`(?i)^##\s+`)
	if rxProposedChanges.MatchString(line) {
		return true
	} else if rxSectionHeader.MatchString(line) && current {
		// Keep parsing sub-sections like ### [Component], but exit on new main sections
		if !strings.HasPrefix(line, "###") {
			return false
		}
	}
	return current
}

// extractSpecPathFromLine parses file paths out of markdown proposed changes list items.
func extractSpecPathFromLine(line string, repoRoot string) string {
	rxActionLink := regexp.MustCompile(`(?i)(?:-|\*|####)\s*\[(?:NEW|MODIFY|DELETE)\]\s*\[[^\]]+\]\(([^)]+)\)`)
	rxActionPlain := regexp.MustCompile(`(?i)(?:-|\*|####)\s*\[(?:NEW|MODIFY|DELETE)\]\s*(.+)`)

	var rawPath string
	if m := rxActionLink.FindStringSubmatch(line); len(m) > 1 {
		rawPath = m[1]
	} else if m := rxActionPlain.FindStringSubmatch(line); len(m) > 1 {
		rawPath = strings.TrimSpace(m[1])
	}

	if rawPath == "" {
		return ""
	}

	if strings.HasPrefix(rawPath, "file://") {
		rawPath = strings.TrimPrefix(rawPath, "file://")
	}
	if filepath.IsAbs(rawPath) {
		if rel, errRel := filepath.Rel(repoRoot, rawPath); errRel == nil {
			rawPath = rel
		}
	}
	return filepath.Clean(filepath.ToSlash(rawPath))
}
