package verify

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
)

// ParseSpecFiles extracts planned file paths from implementation_plan.md
func ParseSpecFiles(planPath string, root string) (map[string]bool, error) {
	planned := make(map[string]bool)
	file, err := os.Open(planPath)
	if err != nil {
		return planned, err
	}
	defer file.Close()

	rxProposedChanges := regexp.MustCompile(`(?i)^##\s+Proposed\s+Changes`)
	rxSectionHeader := regexp.MustCompile(`(?i)^##\s+`)
	rxActionLink := regexp.MustCompile(`(?i)(?:-|\*|####)\s*\[(?:NEW|MODIFY|DELETE)\]\s*\[[^\]]+\]\(([^)]+)\)`)
	rxActionPlain := regexp.MustCompile(`(?i)(?:-|\*|####)\s*\[(?:NEW|MODIFY|DELETE)\]\s*(.+)`)

	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	inProposedChanges := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if rxProposedChanges.MatchString(line) {
			inProposedChanges = true
			continue
		} else if rxSectionHeader.MatchString(line) {
			inProposedChanges = false
			continue
		}

		if !inProposedChanges {
			continue
		}

		rawPath := parseProposedChangePath(line, rxActionLink, rxActionPlain)
		if rawPath == "" {
			continue
		}

		normPath := normalizeSpecPath(rawPath, root, absRoot)
		if !strings.HasPrefix(normPath, "..") {
			planned[normPath] = true
		}
	}

	return planned, scanner.Err()
}

// parseProposedChangePath inspects a single line from implementation_plan.md to extract file target URLs.
func parseProposedChangePath(line string, rxLink, rxPlain *regexp.Regexp) string {
	// Match markdown link pattern [LABEL](file://path)
	if match := rxLink.FindStringSubmatch(line); len(match) > 1 {
		return match[1]
	} else if match := rxPlain.FindStringSubmatch(line); len(match) > 1 {
		// Fall back to plain text file pattern [LABEL] path
		return match[1]
	}
	return ""
}

// normalizeSpecPath cleans and normalizes a relative file path extracted from plan markdown.
func normalizeSpecPath(rawPath, root, absRoot string) string {
	// Strip file:// scheme and host prefixes
	rawPath = stripSpecPathPrefixes(rawPath, absRoot)
	// Resolve path relative to workspace root directory
	relPath := resolveSpecRelPath(rawPath, root, absRoot)
	// Clean and normalize slash direction for OS compatibility
	normPath := filepath.ToSlash(filepath.Clean(relPath))
	return strings.TrimPrefix(normPath, "./")
}

// stripSpecPathPrefixes removes file URI scheme, host components, and root directory prefixes.
func stripSpecPathPrefixes(rawPath, absRoot string) string {
	if strings.HasPrefix(rawPath, "file://") {
		rawPath = strings.TrimPrefix(rawPath, "file://")
		if strings.HasPrefix(rawPath, "localhost") {
			rawPath = strings.TrimPrefix(rawPath, "localhost")
		}
	}

	// Match workspace root base name to extract clean relative path
	rootBase := filepath.Base(absRoot)
	idx := strings.Index(filepath.ToSlash(rawPath), "/"+rootBase+"/")
	if idx != -1 {
		rawPath = rawPath[idx+len(rootBase)+2:]
	}
	return rawPath
}

func resolveSpecRelPath(rawPath, root, absRoot string) string {
	if strings.HasPrefix(rawPath, "/") && len(rawPath) > 1 {
		stripped := rawPath[1:]
		if _, errS := os.Stat(filepath.Join(root, stripped)); errS == nil {
			rawPath = stripped
		}
	}

	if filepath.IsAbs(rawPath) {
		if relPath, err := filepath.Rel(absRoot, rawPath); err == nil {
			return relPath
		}
		return rawPath
	}
	absPath := filepath.Join(absRoot, rawPath)
	if relPath, err := filepath.Rel(absRoot, absPath); err == nil {
		return relPath
	}
	return rawPath
}

// GetModifiedFiles gets the set of modified/untracked files using git commands.
func GetModifiedFiles(root string) (map[string]bool, error) {
	modified := make(map[string]bool)

	runGitHelper := func(args ...string) []string {
		out, err := runGit(root, args...)
		if err != nil {
			return nil
		}
		var result []string
		for _, l := range strings.Split(out, "\n") {
			if t := strings.TrimSpace(l); t != "" {
				result = append(result, t)
			}
		}
		return result
	}

	parseGitDiffFiles(runGitHelper, modified)
	parseGitStatusFiles(runGitHelper, modified, root)

	return modified, nil
}

// parseGitDiffFiles runs git diff commands to find files changed in the git commit logs
// compared to develop branch or local HEAD state, filtering out internal state files.
func parseGitDiffFiles(runGitHelper func(...string) []string, modified map[string]bool) {
	diffFiles := append(runGitHelper("diff", "--name-only", "HEAD"), runGitHelper("diff", "--name-only", "origin/develop...HEAD")...)
	for _, f := range diffFiles {
		f = strings.TrimSpace(f)
		if !isAgentStateFile(f) {
			modified[filepath.ToSlash(f)] = true
		}
	}
}

func isUntrackedDir(root, f string) bool {
	fi, err := os.Stat(filepath.Join(root, f))
	return err == nil && fi.IsDir()
}

// parseGitStatusFiles parses the porcelain status lines to find local unstaged,
// staged, and untracked changes, using precise 2-character slicing prefix rules.
func parseGitStatusFiles(runGitHelper func(...string) []string, modified map[string]bool, root string) {
	for _, l := range runGitHelper("status", "--porcelain", "-u") {
		if len(l) <= 3 {
			continue
		}
		statusSymbol := l[:2]
		f := strings.TrimSpace(l[2:])
		if isAgentStateFile(f) {
			continue
		}
		if strings.Contains(statusSymbol, "??") && isUntrackedDir(root, f) {
			continue
		}
		modified[filepath.ToSlash(f)] = true
	}
}

// isAgentStateFile detects if the given relative file path belongs to the
// internal agent specifications, story cards, quality debt files, or state caches.
func isAgentStateFile(f string) bool {
	return config.IsAgentStateFile(f)
}

// CheckSpecParity executes the spec parity logic and writes the markdown report.
// It detects the active Task ID, loads the implementation plan, queries git for
// the modified files, calculates drift and parity scores, and generates the report.
func CheckSpecParity(root string, taskId string) (driftScore float64, parityScore float64, err error) {
	// Detect active task ID if not explicitly passed
	if taskId == "" {
		taskId = GetActiveTaskId(root)
	}
	if taskId == "" {
		return 0, 0, fmt.Errorf("active Task ID could not be detected")
	}

	// Resolve target paths for specifications, plans, and output reports
	specsDir, planPath, reportPath := resolveSpecPaths(root, taskId)

	// Ensure the implementation plan exists on disk
	if _, err := os.Stat(planPath); os.IsNotExist(err) {
		return 0, 0, fmt.Errorf("spec plan not found at %s", planPath)
	}

	// Parse the implementation plan to find the planned files list
	planned, err := ParseSpecFiles(planPath, root)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse spec file: %w", err)
	}

	// Tokenize plan text for AST symbol parity verification
	planTokens, err := tokenizePlanText(planPath, root)
	if err != nil {
		planTokens = make(map[string]map[string]bool)
	}

	// Fetch actual modified files using git status and diff commands
	actual, err := GetModifiedFiles(root)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to list modified files: %w", err)
	}

	// Check AST-level symbol parity for modified Go files
	undeclaredSymbols, totalUndeclaredSymbolsCount := checkASTParity(root, actual, planTokens)

	// Compute matching, unfulfilled, and undocumented modification maps
	unfulfilled, undocumented, matching := computeParityMaps(planned, actual)

	// Calculate the drift and spec parity score values, incorporating AST drift
	totalFiles := len(planned) + len(undocumented) + totalUndeclaredSymbolsCount
	if totalFiles > 0 {
		driftScore = float64(len(unfulfilled)+len(undocumented)+totalUndeclaredSymbolsCount) / float64(totalFiles) * 100.0
	}
	parityScore = 100.0 - driftScore

	// Check if any logic files were modified without corresponding test file changes
	missingTests := getMissingTests(actual)

	// Scaffold directory, write parity report, and update terminal dashboard
	if err := writeAndPrintParity(specsDir, reportPath, taskId, driftScore, parityScore, planned, actual, matching, unfulfilled, undocumented, missingTests, undeclaredSymbols); err != nil {
		return driftScore, parityScore, err
	}

	return driftScore, parityScore, nil
}

// writeAndPrintParity handles report directory creation, writing the markdown report, and printing CLI dashboard.
func writeAndPrintParity(specsDir, reportPath, taskId string, driftScore, parityScore float64, planned, actual, matching, unfulfilled, undocumented map[string]bool, missingTests []string, undeclared map[string][]string) error {
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		return fmt.Errorf("failed to create specs dir: %w", err)
	}
	if err := writeParityReport(reportPath, taskId, driftScore, parityScore, planned, actual, matching, unfulfilled, undocumented, missingTests, undeclared); err != nil {
		return fmt.Errorf("failed to write parity report: %w", err)
	}
	printParityDashboard(taskId, driftScore, parityScore, planned, actual, matching, unfulfilled, undocumented, missingTests, reportPath, undeclared)
	return nil
}

// resolveSpecPaths constructs absolute directories and files paths for task specifications.
// It checks primary .nomos specs, temporary implementation plans, and legacy locations.
func resolveSpecPaths(root, taskId string) (specsDir, planPath, reportPath string) {
	specsDir = filepath.Join(config.PlansDir(root), taskId)
	planPath = filepath.Join(specsDir, "implementation_plan.md")
	if _, err := os.Stat(planPath); err == nil {
		reportPath = filepath.Join(specsDir, "parity_report.md")
		return
	}

	altPlan := filepath.Join(config.TmpDir(root), "implementation_plan.md")
	if _, errAlt := os.Stat(altPlan); errAlt == nil {
		planPath = altPlan
		reportPath = filepath.Join(specsDir, "parity_report.md")
		return
	}

	legacyPlan := filepath.Join(root, ".agent", "specs", taskId, "implementation_plan.md")
	if _, errLeg := os.Stat(legacyPlan); errLeg == nil {
		planPath = legacyPlan
	}
	reportPath = filepath.Join(specsDir, "parity_report.md")
	return
}

// calculateParityScores calculates the self-drift and spec parity scores based on file maps.
func calculateParityScores(planned, unfulfilled, undocumented map[string]bool) (drift, parity float64) {
	totalFiles := len(planned) + len(undocumented)
	if totalFiles > 0 {
		drift = float64(len(unfulfilled)+len(undocumented)) / float64(totalFiles) * 100.0
	}
	parity = 100.0 - drift
	return
}

// getMissingTests returns logic files that lack matching test edits in the change delta.
func getMissingTests(actual map[string]bool) []string {
	modifiedLogic, modifiedTests := classifyModifiedFiles(actual)
	if len(modifiedLogic) > 0 && len(modifiedTests) == 0 {
		return modifiedLogic
	}
	return nil
}

// computeParityMaps evaluates files from planned and actual maps to group them
// into unfulfilled intents, undocumented mutations, and matching intent maps.
func computeParityMaps(planned, actual map[string]bool) (unfulfilled, undocumented, matching map[string]bool) {
	// Initialize result sets for spec tracking
	unfulfilled = make(map[string]bool)
	undocumented = make(map[string]bool)
	matching = make(map[string]bool)

	// Check planned files against actual modified files
	for p := range planned {
		if actual[p] {
			// File planned and modified: matching intent
			matching[p] = true
		} else {
			// File planned but not modified: unfulfilled intent
			unfulfilled[p] = true
		}
	}
	// Identify unrecorded modifications
	for a := range actual {
		if !planned[a] {
			// File modified without specification plan entry
			undocumented[a] = true
		}
	}
	return
}

// totalFilesCount computes the union count of planned files and undocumented mutations.
func totalFilesCount(planned, undocumented map[string]bool) int {
	// Calculate total unique files participating in parity calculation
	return len(planned) + len(undocumented)
}

func isLogicExtension(f string, logicExtensions []string) bool {
	for _, ext := range logicExtensions {
		if strings.HasSuffix(f, ext) {
			return true
		}
	}
	return false
}

// classifyModifiedFiles filters the actual files map and groups them into logic files
// and corresponding unit/integration test files based on extensions and naming patterns.
func classifyModifiedFiles(actual map[string]bool) ([]string, []string) {
	logicExtensions := []string{".sh", ".py", ".go", ".js", ".ts", ".jsx", ".tsx"}
	var modifiedLogic []string
	var modifiedTests []string

	for f := range actual {
		if !isLogicExtension(f, logicExtensions) {
			continue
		}
		lower := strings.ToLower(f)
		if strings.Contains(lower, "test") || strings.HasSuffix(lower, ".bats") || strings.HasSuffix(lower, "_test.go") || strings.HasPrefix(filepath.Base(f), "test_") {
			modifiedTests = append(modifiedTests, f)
		} else {
			modifiedLogic = append(modifiedLogic, f)
		}
	}
	return modifiedLogic, modifiedTests
}

// writeParityReport serializes the spec parity check metrics and results into a structured
// markdown format on disk, listing matches, omissions, and missing test coverage indicators.
func writeParityReport(reportPath, taskId string, driftScore, parityScore float64, planned, actual, matching, unfulfilled, undocumented map[string]bool, missingTests []string, undeclaredSymbols map[string][]string) error {
	// Create the report file or overwrite if it already exists
	reportFile, err := os.Create(reportPath)
	if err != nil {
		return err
	}
	defer reportFile.Close()

	w := bufio.NewWriter(reportFile)
	// Write header details
	fmt.Fprintf(w, "# Spec-to-Code Parity Report (%s)\n\n", taskId)
	fmt.Fprintf(w, "Mathematically calculates code modification alignment with proposed specifications.\n\n")

	// Write telemetry statistics block
	fmt.Fprintln(w, "## Telemetry Metrics")
	fmt.Fprintf(w, "- **Self-Drift Score**: %.1f%%\n", driftScore)
	fmt.Fprintf(w, "- **Spec Parity Score**: %.1f%%\n", parityScore)
	fmt.Fprintf(w, "- **Planned Files**: %d\n", len(planned))
	fmt.Fprintf(w, "- **Actually Modified**: %d\n", len(actual))
	fmt.Fprintf(w, "- **Matching Coverage**: %d\n\n", len(matching))

	fmt.Fprintf(w, "## Detailed Analysis\n\n")

	// Print matching, unfulfilled, and undocumented file categories
	writeSectionList(w, "### 🟢 Matching Intent", "_No matching files._", "- `%s` (Aligned with Spec)\n", matching)
	writeSectionList(w, "### 🔴 Unfulfilled Intent", "_None._", "- `%s` (Specified but never modified)\n", unfulfilled)
	writeSectionList(w, "### 🟡 Undocumented Mutations", "_None._", "- `%s` (Code modified but omitted from Spec)\n", undocumented)

	// Print undeclared AST symbols
	writeReportUndeclared(w, undeclaredSymbols)

	// Check and output test file modifications delta warnings
	writeReportWarnings(w, missingTests)

	return w.Flush()
}

func writeReportUndeclared(w *bufio.Writer, undeclaredSymbols map[string][]string) {
	fmt.Fprintln(w, "### ❌ Undeclared Public Symbols")
	if len(undeclaredSymbols) == 0 {
		fmt.Fprintln(w, "_No undeclared public exports detected._")
		return
	}
	var files []string
	for f := range undeclaredSymbols {
		files = append(files, f)
	}
	sort.Strings(files)
	for _, f := range files {
		fmt.Fprintf(w, "- File `%s` contains undeclared public exports:\n", f)
		syms := undeclaredSymbols[f]
		sort.Strings(syms)
		for _, sym := range syms {
			fmt.Fprintf(w, "  - `%s` (Exported but omitted from plan details)\n", sym)
		}
	}
	fmt.Fprintln(w, "")
}

func writeReportWarnings(w *bufio.Writer, missingTests []string) {
	fmt.Fprintln(w, "### ⚠️  Test Coverage Warnings")
	if len(missingTests) > 0 {
		fmt.Fprintln(w, "The following logic files were modified, but no test files were detected in the change delta:")
		sort.Strings(missingTests)
		for _, m := range missingTests {
			fmt.Fprintf(w, "- `%s` (Lacks test changes)\n", m)
		}
	} else {
		fmt.Fprintln(w, "_All logic modifications are covered by corresponding test files._")
	}
}

// writeSectionList formats a mapped list of file paths into an ordered markdown bullet list.
func writeSectionList(w *bufio.Writer, title, emptyMsg, format string, items map[string]bool) {
	fmt.Fprintln(w, title)
	if len(items) > 0 {
		var keys []string
		for k := range items {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(w, format, k)
		}
	} else {
		fmt.Fprintln(w, emptyMsg)
	}
	fmt.Fprintln(w, "")
}

// printParityDashboard emits the final spec parity check statistics via Synapse JSON-L.
func printParityDashboard(taskId string, driftScore, parityScore float64, planned, actual, matching, unfulfilled, undocumented map[string]bool, missingTests []string, reportPath string, undeclaredSymbols map[string][]string) {
	totalUndeclared := 0
	for _, syms := range undeclaredSymbols {
		totalUndeclared += len(syms)
	}

	payload := map[string]interface{}{
		"task_id":                      taskId,
		"drift_score":                  driftScore,
		"parity_score":                 parityScore,
		"planned_files_count":          len(planned),
		"actually_modified_count":      len(actual),
		"matching_intent_count":        len(matching),
		"unfulfilled_intent_count":     len(unfulfilled),
		"undocumented_mutations_count": len(undocumented),
		"total_undeclared_symbols":     totalUndeclared,
		"undeclared_symbols":           undeclaredSymbols,
		"missing_tests":                missingTests,
		"report_path":                  reportPath,
	}

	synapse.Emit("SpecParityResult", payload)
}

// checkASTParity checks AST-level symbol parity for modified Go files and returns a map of undeclared symbols.
func checkASTParity(root string, actual map[string]bool, planTokens map[string]map[string]bool) (map[string][]string, int) {
	// Initialize map of undeclared symbols grouped by Go file path
	undeclaredSymbols := make(map[string][]string)
	totalUndeclaredSymbolsCount := 0

	// Loop through all modified files to run the AST checks
	for file := range actual {
		// Only check Go files as python/js/ts AST checking is handled out-of-band
		if filepath.Ext(file) != ".go" {
			continue
		}
		syms, count := checkFileASTParity(root, file, planTokens[file])
		if len(syms) > 0 {
			undeclaredSymbols[file] = syms
			totalUndeclaredSymbolsCount += count
		}
	}
	return undeclaredSymbols, totalUndeclaredSymbolsCount
}

// checkFileASTParity runs the AST checks for a single Go file.
// It parses exported AST symbols, checks git diff line ranges, and verifies token inclusion.
func checkFileASTParity(root, file string, tokens map[string]bool) ([]string, int) {
	var undeclared []string
	count := 0

	// Parse all AST symbols from the Go source file
	symbols, err := parseGoSymbols(filepath.Join(root, file))
	if err != nil {
		return nil, 0
	}

	// Fetch set of line numbers added or modified in git diff
	addedLines, err := getGitAddedLines(root, file)
	if err != nil {
		return nil, 0
	}

	// Ensure token map is initialized
	if tokens == nil {
		tokens = make(map[string]bool)
	}

	// Check each AST symbol against modified line numbers and plan tokens
	for _, sym := range symbols {
		// Filter out non-exported internal AST symbols (lowercase identifiers)
		if len(sym.Name) == 0 || !unicode.IsUpper(rune(sym.Name[0])) {
			continue
		}
		// If exported symbol was modified but missing from plan tokens, flag as undeclared
		if isSymbolModified(sym, addedLines) && !tokens[sym.Name] {
			undeclared = append(undeclared, sym.Name)
			count++
		}
	}
	return undeclared, count
}

// isSymbolModified checks if any line of the symbol declaration is in the added/modified set.
func isSymbolModified(sym SymbolInfo, addedLines map[int]bool) bool {
	// Scan line range spanned by the AST symbol declaration
	for ln := sym.LineStart; ln <= sym.LineEnd; ln++ {
		// Check if line was modified in git diff
		if addedLines[ln] {
			return true
		}
	}
	return false
}
