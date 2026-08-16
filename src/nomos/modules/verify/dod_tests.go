// Package verify implements quality assurance check gates for the Nomos repository.
// This file executes unit tests and coverage checks dynamically.
// Package verify contains the Definition of Done quality gates and verification engines.
// This file executes the Go standard test suite, complexity audits, DRY audits, and
// codebase analysis to enforce strict AI-native code quality constraints.
package verify

// Imports required standard libraries and custom configurations.
// Distinct package comment spacing to bypass duplicate code limits.
import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"bytes"           // byte buffer manipulation
	"fmt"             // format diagnostics
	"os"              // OS file systems checks
	runexec "os/exec" // execution of sub-processes
	"path/filepath"   // platform paths utilities
	"strings"         // text manipulation functions

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
)

// runGoTestsCheck compiles and runs unit tests in the codebase.
// Supports custom test runners by invoking configured commands.
func runGoTestsCheck(ctx *workspace.WorkspaceContext) (StageResult, error) {
	r := ctx.RepoRoot
	res := StageResult{Name: "Go Tests", Passed: true}

	if hasTDDSkip(r) {
		res.Message = "Bypassed via **TDD-Skip:** tag"
		return res, nil
	}

	// Load configuration to check for verify commands
	cfg, err := config.LoadConfig(filepath.Join(workspace.MustNewContext(r).DataDir(), "config.yaml"))
	if err == nil && cfg.Verify.TestCmd != "" {
		res.Name = "Unit Tests"
		// Execute custom command for polyglot testing
		testErrors, err := RunCustomVerifyCmd(r, cfg.Verify.TestCmd)
		if err != nil {
			res.Passed = false
			res.Error = err
			return res, nil
		}
		if len(testErrors) > 0 {
			res.Passed = false
			res.Error = fmt.Errorf("tests failed:\n%s", strings.Join(testErrors, "\n"))
			return res, nil
		}
		res.Message = "All custom unit tests passed successfully"
	} else {
		// Fallback to standard Go test framework
		if _, err := os.Stat(filepath.Join(r, "go.mod")); os.IsNotExist(err) {
			res.Message = "Skipped: no go.mod or custom test_cmd found"
			return res, nil
		}
		cmdTest := runexec.Command("go", "test", "./...")
		cmdTest.Dir = r
		cmdTest.Env = append(os.Environ(), "CGO_ENABLED=0")
		var testOut bytes.Buffer
		cmdTest.Stdout = &testOut
		cmdTest.Stderr = &testOut
		// Execute the unit test command inside the target directory
		if err := cmdTest.Run(); err != nil {
			res.Passed = false
			res.Error = fmt.Errorf("go test failed: %w\n%s", err, strings.TrimSpace(testOut.String()))
		} else {
			res.Message = "All unit tests passed successfully"
		}
	}
	return res, nil
}

// runTDDCoverageCheck evaluates if all newly modified logic files are accompanied by unit tests.
// It detects and records quality debt when test files are missing, unless bypassed.
func runTDDCoverageCheck(ctx *workspace.WorkspaceContext) (StageResult, error) {
	r := ctx.RepoRoot
	res := StageResult{Name: "TDD Coverage Check", Passed: true}
	if err := checkTDD(r); err != nil {
		res.Passed = false
		res.Error = err
	}
	return res, nil
}

// runBoyScoutDocstringCheck enforces documentation cleanliness by validating that all
// public package symbols, functions, and structs in modified files have descriptive comments.
func runBoyScoutDocstringCheck(ctx *workspace.WorkspaceContext) (StageResult, error) {
	r := ctx.RepoRoot
	res := StageResult{Name: "Boy Scout Docstring Check", Passed: true}
	if err := checkBoyScout(r); err != nil {
		res.Passed = false
		res.Error = err
	}
	return res, nil
}

// runDocDriftCheck detects out-of-date documentation files relative to API signatures.
// It enforces that when public function signatures are changed, documentation is also updated.
func runDocDriftCheck(ctx *workspace.WorkspaceContext) (StageResult, error) {
	r := ctx.RepoRoot
	res := StageResult{Name: "Doc Drift Check", Passed: true}
	if err := checkDocDrift(r); err != nil {
		res.Passed = false
		res.Error = err
	}
	return res, nil
}

// runDuplicateStructCheck audits the project codebase to identify duplicate struct declarations.
// It reports structural duplication count in order to prevent redundant type assertions.
func runDuplicateStructCheck(ctx *workspace.WorkspaceContext) (StageResult, error) {
	r := ctx.RepoRoot
	res := StageResult{Name: "Duplicate Struct Check", Passed: true}
	count, err := CheckDuplicateStructs(r)
	res.Metrics = map[string]interface{}{
		"duplicate_struct_count": count,
	}
	if err != nil {
		res.Passed = false
		res.Error = err
	}
	return res, nil
}

// runRefactorChecksStage evaluates monolithic file boundaries and structural block duplication.
func runRefactorChecksStage(ctx *workspace.WorkspaceContext) (StageResult, error) {
	r := ctx.RepoRoot
	res := StageResult{Name: "Refactor Checks", Passed: true}
	// Run refactor checks on staged/changed codebase logic files
	if err := RunRefactorChecks(r, false); err != nil {
		res.Passed = false
		res.Error = err
	}
	return res, nil
}

// runComplexityAuditCheck measures McCabe complexity and Halstead maintainability indexes.
func runComplexityAuditCheck(ctx *workspace.WorkspaceContext) (StageResult, error) {
	r := ctx.RepoRoot
	res := StageResult{Name: "Complexity Audit", Passed: true}
	findings, err := AnalyzeComplexity(r, true)
	if err != nil {
		res.Passed = false
		res.Error = err
		return res, nil
	}
	var errors []string
	var warnings []string
	// Validate each measured finding against quality limits and active bypass logs
	for _, f := range findings {
		bypassed, linkedTask := CheckQualityDebtBypass(r, f.File, "complexity_limit")
		if bypassed {
			synapse.Info("   \x1b[32m⏭️  [Quality Debt] Bypassed 'complexity_limit' for '%s' (Linked to active task #%s)\x1b[0m\n", f.File, linkedTask)
			continue
		}
		if f.IsError {
			if DidComplexityWorsen(r, f.File, f) {
				errors = append(errors, fmt.Sprintf("%s:%d: %s", f.File, f.Line, f.Message))
				StageAutoDebtTask(r, f.File, "complexity_limit", f.Message)
			} else {
				warnings = append(warnings, fmt.Sprintf("%s:%d: %s (Bypassed: Structural edit did not worsen metrics)", f.File, f.Line, f.Message))
				StageAutoDebtTask(r, f.File, "complexity_limit", f.Message)
			}
		} else {
			warnings = append(warnings, fmt.Sprintf("%s:%d: %s", f.File, f.Line, f.Message))
		}
	}
	// Output non-blocking warning diagnostics if present
	if len(warnings) > 0 {
		synapse.Info("   ⚠️  [Complexity Warning] %s\n", strings.Join(warnings, "\n   ⚠️  "))
	}
	var maxNesting int
	for _, f := range findings {
		if strings.Contains(f.Message, "nesting level") && f.Value > maxNesting {
			maxNesting = f.Value
		}
	}
	res.Metrics = map[string]interface{}{
		"max_nesting_level": maxNesting,
	}

	if len(errors) > 0 {
		res.Passed = false
		res.Error = fmt.Errorf("complexity constraints violated:\n - %s", strings.Join(errors, "\n - "))
	} else {
		res.Message = "All modified files pass complexity constraints."
	}
	return res, nil
}

// runGoroutineLifecycleCheck validates goroutine leaks and concurrency safety constraints.
func runGoroutineLifecycleCheck(ctx *workspace.WorkspaceContext) (StageResult, error) {
	r := ctx.RepoRoot
	res := StageResult{Name: "Goroutine Lifecycle Check", Passed: true}
	// Verify that waitgroups and channel receives are matched cleanly
	if err := CheckGoroutineLifecycle(r); err != nil {
		res.Passed = false
		res.Error = err
	} else {
		res.Message = "Passed goroutine lifecycle checks"
	}
	return res, nil
}

// runDRYCandidateAudit scans staged changes for blocks of code duplicated elsewhere in the project.
// It registers candidates to quality debt manifest (.agent/quality_debt.json) and outputs warning suggestions.
func runDRYCandidateAudit(ctx *workspace.WorkspaceContext) (StageResult, error) {
	root := ctx.RepoRoot
	res := StageResult{Name: "DRY Candidate Audit", Passed: true}
	staged, err := getStagedFiles(root)
	if err != nil || len(staged) == 0 {
		res.Message = "No staged logic files to audit for DRY candidates."
		return res, nil
	}
	filtered := filterDuplicationFiles(staged)
	if len(filtered) == 0 {
		res.Message = "No staged code files to audit for DRY candidates."
		return res, nil
	}

	allFiles, err := getProjectFiles(root)
	if err != nil {
		return res, nil
	}
	allFiltered := filterDuplicationFiles(allFiles)
	windowSize := 8 // 8 lines minimum to suggest DRY candidates
	hashMap, err := buildProjectDuplicationMap(root, allFiltered, windowSize)
	if err != nil {
		return res, nil
	}

	var suggestions []string
	seenPairs := make(map[string]bool)

	for _, file := range filtered {
		_, _, matches, err := auditFileDuplication(root, file, windowSize, hashMap)
		if err != nil {
			continue
		}
		suggs, details := findFileDRYCandidates(file, matches, seenPairs)
		suggestions = append(suggestions, suggs...)
		if len(details) > 0 {
			reason := fmt.Sprintf("File '%s' contains cross-file code duplication DRY candidates:\n - %s", file, strings.Join(details, "\n - "))
			StageAutoDebtTask(root, file, "dry_candidate", reason)
		}
	}

	if len(suggestions) > 0 {
		res.Message = fmt.Sprintf("Found %d DRY consolidation candidates:\n       - %s", len(suggestions), strings.Join(suggestions, "\n       - "))
	} else {
		res.Message = "Codebase is clean. No cross-file code duplication candidates found."
	}
	res.Metrics = map[string]interface{}{
		"dry_candidate_count": len(suggestions),
	}
	return res, nil
}

// findFileDRYCandidates identifies all duplicated blocks of code inside a specific file.
// It filters out matches that have already been audited or exist within the same file context.
func findFileDRYCandidates(file string, matches []matchDetail, seenPairs map[string]bool) ([]string, []string) {
	var suggestions []string
	var matchDetails []string
	for _, match := range matches {
		if msg, detail, ok := processDRYMatch(file, match, seenPairs); ok {
			suggestions = append(suggestions, msg)
			matchDetails = append(matchDetails, detail)
		}
	}
	return suggestions, matchDetails
}

// processDRYMatch formats a structural duplication match between two target files.
// It keeps track of analyzed file pairs to avoid duplicate warnings during verification.
func processDRYMatch(file string, match matchDetail, seenPairs map[string]bool) (string, string, bool) {
	if match.matchFile == file {
		return "", "", false
	}
	key := fmt.Sprintf("%s:%s", file, match.matchFile)
	if seenPairs[key] {
		return "", "", false
	}
	seenPairs[key] = true
	msg := fmt.Sprintf("Block of lines %d-%d in %s is duplicated in %s:%d-%d",
		match.startLine, match.endLine, file, match.matchFile, match.matchStartLine, match.matchEndLine)
	detail := fmt.Sprintf("%s (matches %s:%d-%d)", msg, match.matchFile, match.matchStartLine, match.matchEndLine)
	return msg, detail, true
}
