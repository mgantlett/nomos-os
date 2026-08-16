package verify

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

// bannedImportsConfig represents the structure of the configuration file.
type bannedImportsConfig struct {
	BannedImports []string `json:"banned_imports"`
	BannedPhrases []string `json:"banned_phrases"`
}

// loadBannedImportsConfig loads configuration rules from .agent/rules/banned_imports.json or falls back to defaults.
// It parses banned import package paths and banned phrase rules used to audit code files.
func loadBannedImportsConfig(ctx *workspace.WorkspaceContext) (bannedImportsConfig, error) {
	configPath := ctx.AgentPath("rules", "banned_imports.json")

	var config bannedImportsConfig

	data, err := os.ReadFile(configPath)
	if err != nil {
		// Fallback defaults
		config.BannedImports = []string{
			"github.com/mgantlett/ado-core",
			"github.com/mgantlett/nomos-commons/src/legacy",
		}
		config.BannedPhrases = []string{
			"os/exec",
		}
		return config, nil
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("failed to parse banned_imports.json: %w", err)
	}

	return config, nil
}

// isFileExemptFromExec defines packages and directory paths that are allowed
// to directly import subprocess execution facilities (like os/exec).
// These packages are excluded from the banned phrases audit checklist.
func isFileExemptFromExec(relPath string) bool {
	return strings.Contains(relPath, "_test.go") ||
		strings.Contains(relPath, "src/nomos/modules/exec/") ||
		strings.Contains(relPath, "src/nomos/utils/") ||
		strings.Contains(relPath, "src/nomos/modules/verify/") ||
		strings.Contains(relPath, "src/nomos/modules/server/") ||
		strings.Contains(relPath, "src/nomos/cmd/") ||
		strings.Contains(relPath, "src/nomos/modules/task/") ||
		strings.Contains(relPath, "src/nomos/core/console/") ||
		strings.Contains(relPath, "src/nomos/core/telemetry/") ||
		strings.Contains(relPath, "src/nomos/core/plugin/") ||
		strings.Contains(relPath, "src/nomos/modules/integration/")
}

// checkImportViolations returns import violation strings for a single import path.
func checkImportViolations(relPath string, impPath string, config bannedImportsConfig, isExemptFromExec bool) []string {
	var violations []string

	// 1. Check banned imports list
	for _, banned := range config.BannedImports {
		if impPath == banned || strings.HasPrefix(impPath, banned+"/") {
			violations = append(violations, fmt.Sprintf("%s: imports forbidden package '%s'", relPath, impPath))
		}
	}

	// 2. Check banned phrases list
	for _, banned := range config.BannedPhrases {
		if impPath == banned {
			if banned == "os/exec" && isExemptFromExec {
				continue
			}
			violations = append(violations, fmt.Sprintf("%s: directly imports forbidden package '%s' (use substrate wrapper)", relPath, impPath))
		}
	}

	return violations
}

var nixImportRegex = regexp.MustCompile(`\b(?:builtins\.)?import\s+("([^"]+)"|'([^']+)'|([^\s;({]+))`)

// parseNixImports parses Nix expression files using regular expressions to find all
// imported expression paths, stripping quotation marks from the matched tokens.
func parseNixImports(content string) []string {
	var imports []string
	matches := nixImportRegex.FindAllStringSubmatch(content, -1)
	for _, m := range matches {
		imp := ""
		if m[2] != "" {
			imp = m[2]
		} else if m[3] != "" {
			imp = m[3]
		} else if m[4] != "" {
			imp = m[4]
		}
		if imp != "" {
			imports = append(imports, strings.Trim(imp, `"'`))
		}
	}
	return imports
}

// parseShellImports scans shell scripts to find sourced file references (e.g. source x, . x).
// It ignores commented lines and extracts the raw target path for import audits.
func parseShellImports(content string) []string {
	var imports []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip commented lines in shell scripts
		if strings.HasPrefix(line, "#") {
			continue
		}

		// Handle 'source path/to/file' patterns
		if strings.HasPrefix(line, "source ") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "source "))
			path = strings.Trim(path, `"'`)
			if path != "" {
				imports = append(imports, path)
			}
		}

		// Handle '. path/to/file' sourcing patterns
		if strings.HasPrefix(line, ". ") {
			path := strings.TrimSpace(strings.TrimPrefix(line, ". "))
			path = strings.Trim(path, `"'`)
			if path != "" {
				imports = append(imports, path)
			}
		}
	}
	return imports
}

// AuditImports checks Go, Nix, and Shell files for prohibited package imports or sources.
// It iterates through all target file paths, loads the active config rules, constructs
// a file set, and delegates auditing tasks to language-specific parser checks.
// It detects the current workspace module, and bypasses the entire import validation check
// if it contains the Sovereign Monorepo (github.com/mgantlett/nomos-sovereign).
func AuditImports(ctx *workspace.WorkspaceContext, files []string) ([]string, error) {
	repoRoot := ctx.RepoRoot
	// Query current module name from go environment.
	modCmd := exec.Command("go", "list", "-m")
	modCmd.Dir = repoRoot
	outBytes, _ := modCmd.Output()
	currentMod := strings.TrimSpace(string(outBytes))

	// Bypass the entire Legacy Code Blocker import check for Sovereign.
	if strings.Contains(currentMod, "github.com/mgantlett/nomos-sovereign") {
		return nil, nil
	}

	var violations []string

	// Load configuration containing banned import targets and banned phrases
	config, err := loadBannedImportsConfig(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())
	if err != nil {
		return nil, err
	}

	// Initialize the token file set for AST compilation and parsing
	fset := token.NewFileSet()
	for _, relPath := range files {
		v := auditSingleFile(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), relPath, config, fset)
		violations = append(violations, v...)
	}

	return violations, nil
}

// auditSingleFile selects the language-specific audit parser for a given file.
// It routes Go files (.go), Nix files (.nix), and Shell scripts (.sh, .bash)
// to their corresponding verification checker logic.
func auditSingleFile(ctx *workspace.WorkspaceContext, relPath string, config bannedImportsConfig, fset *token.FileSet) []string {
	absPath := filepath.Join(ctx.RepoRoot, relPath)

	if strings.HasSuffix(relPath, ".go") {
		return auditGoFile(relPath, absPath, config, fset)
	}
	if strings.HasSuffix(relPath, ".nix") {
		return auditNixFile(relPath, absPath, config)
	}
	if strings.HasSuffix(relPath, ".sh") || strings.HasSuffix(relPath, ".bash") {
		return auditShellFile(relPath, absPath, config)
	}

	return nil
}

func auditGoFile(relPath, absPath string, config bannedImportsConfig, fset *token.FileSet) []string {
	if strings.HasSuffix(relPath, "_test.go") {
		return nil
	}
	file, err := parser.ParseFile(fset, absPath, nil, parser.ImportsOnly)
	if err != nil {
		return nil
	}

	var violations []string
	isExemptFromExec := isFileExemptFromExec(relPath)
	for _, imp := range file.Imports {
		impPath := strings.Trim(imp.Path.Value, `"`)
		v := checkImportViolations(relPath, impPath, config, isExemptFromExec)
		violations = append(violations, v...)
	}
	return violations
}

// auditNixFile scans a Nix expression file for imports and audits them against rules.
// It extracts local/relative references and checks them for banned package boundaries.
func auditNixFile(relPath, absPath string, config bannedImportsConfig) []string {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil
	}
	var violations []string
	imports := parseNixImports(string(content))
	for _, imp := range imports {
		v := checkImportViolations(relPath, imp, config, false)
		violations = append(violations, v...)
	}
	return violations
}

// auditShellFile scans a bash or sh file to detect sourced paths and audits them.
// It detects sourcing commands and verifies they do not cross boundaries.
func auditShellFile(relPath, absPath string, config bannedImportsConfig) []string {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil
	}
	var violations []string
	imports := parseShellImports(string(content))
	for _, imp := range imports {
		v := checkImportViolations(relPath, imp, config, false)
		violations = append(violations, v...)
	}
	return violations
}
