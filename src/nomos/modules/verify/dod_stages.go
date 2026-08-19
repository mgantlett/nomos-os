// Package verify contains the entire suite of Definition of Done (DoD)
// quality gates that autonomous agents must satisfy before code can be
// staged or committed into the repository.
//
// This specific file maintains the registry of all static analysis stages
// executed during the verification pipeline. By decoupling the stage
// definitions from the execution runner, we prevent the primary runner
// from exceeding nesting and complexity boundaries.
//
// Available stages include formatting checks, dead code analysis,
// complexity scoring, security auditing, dependency coupling checks,
// and docstring density enforcement.
//
// Bypasses can be granted using either specific commit trailers
// (e.g., '**TDD-Skip:**', '**Doc-Skip:**') or global environmental variables
// depending on the authorization tier of the active task.
// This ensures that all bypassing is auditable and tracked securely.
package verify

import (
	"fmt"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-commons/src/nomos/core/gitbrain"
)

// StageResult stores the outcome of a DoD verification stage.
type StageResult struct {
	Name    string
	Passed  bool
	Error   error
	Message string
	Metrics map[string]interface{}
}

// VerificationStage defines a check stage name, its runner function, and actionable guidance instructions.
type VerificationStage struct {
	Name     string                                                     `json:"name"`
	Guidance string                                                     `json:"guidance"`
	Run      func(ctx *workspace.WorkspaceContext) (StageResult, error) `json:"-"`
}

// DoDStages lists all active static analysis and DoD gate stages.
// This registry dictates the strict order of execution during a commit verification.
// All stages must pass sequentially for the task to be successfully staged.
var DoDStages = []VerificationStage{

	// The System Health Doctor ensures SQLite databases and local configuration files are pristine.
	{
		Name:     "System Health Doctor",
		Guidance: "Run 'bin/nomos doctor' to diagnose and self-heal local database locks and git hooks.",
		Run:      runSystemHealthDoctorCheck,
	},
	// The Phase Discipline Check enforces the deterministic lifecycle code-freeze lock.
	{
		Name:     "Phase Discipline Check",
		Guidance: "Run 'bin/nomos task transition REVIEW' before staging/committing code.",
		Run:      runPhaseDisciplineCheck,
	},
	// The Task ID Validation Gate verifies that every commit is tagged to a valid Task ID registered in the Nomos SQLite database.
	{
		Name:     "Task ID Validation Gate",
		Guidance: "Every commit must be bound to a valid Task ID registered in SQLite graph.db. Run 'nomos task create' to create missing tasks.",
		Run:      runTaskIDValidationCheck,
	},
	// The Data Integrity Gate prevents rogue AI agents from mutating JSON state files without CLI bounds.
	{
		Name:     "Data Integrity Gate",
		Guidance: "JSON state files cannot be modified manually. Use 'bin/nomos task' commands to modify state.",
		Run:      runDataIntegrityCheck,
	},
	// The CrossRepoWorktreeGate verifies that dependent upstream worktrees linked via go.work
	// have been committed and pushed to remote origin prior to downstream commits.
	{
		Name:     "CrossRepoWorktreeGate",
		Guidance: "Ensure that all upstream repositories linked via go.work have their worktrees committed and pushed first.",
		Run:      runCrossRepoWorktreeGate,
	},

	// The Go Format & Vet stage ensures all source files are correctly formatted and vetted.
	// This helps maintain a clean, consistent codebase and prevents basic syntax errors.
	{
		Name:     "Go Format & Vet",
		Guidance: "Run 'nix-shell --run \"go fmt ./... && go vet ./...\"' to format and vet Go files.",
		Run:      skipIfNotGo("Go Format & Vet", runGoFormatAndVetCheck),
	},
	{
		Name:     "Security Audit",
		Guidance: "Review findings or bypass offline environment validation via environment override: 'NOMOS_BYPASS_TOKEN=OVERRIDE'.",
		Run:      runSecurityAuditCheck,
	},
	{
		Name:     "Open Core Architecture Check",
		Guidance: "Remove any hardcoded dependencies or paths pointing to enterprise modules (e.g. gitbrain, swarm).",
		Run:      runArchitectureCheck,
	},
	{
		Name:     "Path Hardcoding Blocker",
		Guidance: "Replace hardcoded path string literals (e.g. '.nomos/') with standard helpers from 'config/paths.go'.",
		Run:      runPathHardcodingBlocker,
	},
	{
		Name:     "AST Magic String Detector",
		Guidance: "Replace raw strings with strongly-typed domain constants when calling critical functions.",
		Run:      skipIfNotGo("AST Magic String Detector", runMagicStringDetectorCheck),
	},

	{
		Name:     "Walkthrough Parity Check",
		Guidance: "Verify that all Acceptance Criteria in the active task description are covered in the walkthrough.",
		Run:      runWalkthroughParityCheck,
	},
	{
		Name:     "Contract-First Gate",
		Guidance: "Ensure Go signatures match the interface rules in '.nomos/contracts.yaml'.",
		Run:      skipIfNotGo("Contract-First Gate", runContractFirstCheck),
	},
	{
		Name:     "Legacy Code Blocker",
		Guidance: "Remove banned packages or legacy imports listed in '.agent/banned_imports.json'.",
		Run:      runLegacyCodeBlockerCheck,
	},
	{
		Name:     "Go Tests",
		Guidance: "Run unit tests via 'nix-shell --run \"go test ./...\"' and resolve any test failures.",
		Run:      skipIfNotGo("Go Tests", runGoTestsCheck),
	},
	{
		Name:     "TDD Coverage Check",
		Guidance: "Add corresponding unit tests for your changes, or bypass using '**TDD-Skip:** <Reason>' in commit message.",
		Run:      skipIfNotGo("TDD Coverage Check", runTDDCoverageCheck),
	},
	{
		Name:     "Boy Scout Docstring Check",
		Guidance: "Ensure all newly added functions or types in modified files have Go docstrings.",
		Run:      skipIfNotGo("Boy Scout Docstring Check", runBoyScoutDocstringCheck),
	},
	{
		Name:     "Doc Drift Check",
		Guidance: "Update target documentation markdown files for public API signature updates, or add '**Doc-Skip:** <Reason>' trailer.",
		Run:      skipIfNotGo("Doc Drift Check", runDocDriftCheck),
	},
	{
		Name:     "Anti-OOP Abstraction Check",
		Guidance: "Remove single-implementation interfaces. Abstract interfaces must have >= 2 concrete implementations to justify the architectural complexity.",
		Run:      skipIfNotGo("Anti-OOP Abstraction Check", runAntiOOPCheck),
	},
	{
		Name:     "Refactor Checks",
		Guidance: "Run 'bin/nomos verify' locally to check duplication report. Consolidate duplicate blocks or use quality_debt bypasses.",
		Run:      runRefactorChecksStage,
	},
	{
		Name:     "Complexity Audit",
		Guidance: "Refactor large functions or split deep nested loops to bring complexity metrics under target limits.",
		Run:      runComplexityAuditCheck,
	},
	{
		Name:     "Goroutine Lifecycle Check",
		Guidance: "Audit unclosed channels, goroutines context cancellation, or unmatched sync.WaitGroup calls.",
		Run:      skipIfNotGo("Goroutine Lifecycle Check", runGoroutineLifecycleCheck),
	},
	{
		Name:     "Comment Density Check",
		Guidance: "Add descriptive comments inside files modified to exceed 10% comment-to-source-lines density.",
		Run:      runCommentDensityCheck,
	},
	{
		Name:     "Coupling Analysis Check",
		Guidance: "Check for cyclic package imports using 'bin/nomos verify' coupling output and remove cyclic dependencies.",
		Run:      skipIfNotGo("Coupling Analysis Check", runCouplingAnalysisCheck),
	},
	{
		Name:     "Churn-vs-Complexity Audit",
		Guidance: "Refactor highly complex, frequently modified files to reduce risk hotspots.",
		Run:      runChurnComplexityAudit,
	},
	{
		Name:     "Duplicate Struct Check",
		Guidance: "Consolidate duplicate model structures into shared utility files to avoid redundant definitions.",
		Run:      skipIfNotGo("Duplicate Struct Check", runDuplicateStructCheck),
	},
	{
		Name:     "DRY Candidate Audit",
		Guidance: "Review duplication candidates in the verify output. Consolidate matching code blocks into shared utilities.",
		Run:      runDRYCandidateAudit,
	},
	{
		Name:     "Workflow Determinism Audit",
		Guidance: "Run 'bin/nomos audit workflows' to identify out-of-sync markdown templates. Ensure any flags used in .agent/workflows exist in the CLI.",
		Run:      runWorkflowDeterminismCheck,
	},
	{
		Name:     "Dead Code Detector",
		Guidance: "Ensure exported functions, constants, or types are referenced elsewhere, or bypass with a 'dead_code' quality debt entry.",
		Run:      runDeadCodeCheck,
	},
	{
		Name:     "Config Drift Detector",
		Guidance: "Ensure that all environment variables accessed in the codebase are defined in .env.example.",
		Run:      runConfigDriftCheck,
	},
	{
		Name:     "Generated Code Blocker",
		Guidance: "Do not manually edit generated files. If you must, bypass with 'Gen-Skip: <Reason>' in the commit message.",
		Run:      runGeneratedCodeBlockerCheck,
	},
	{
		Name:     "Broken Wire & Zombie Agent Detector",
		Guidance: "Task has been dormant for >12 hours or circuit breaker tripped due to 5 consecutive verify failures. Re-evaluate strategy or unblock.",
		Run:      runBrokenWireDetector,
	},
	{
		Name:     "GitBrain Vector Synchronization",
		Guidance: "Ensures the local SQLite vector cache is perfectly synced with Git Notes and Codebase. Fails if embeddings server is unreachable or corrupt.",
		Run:      runGitBrainIndexStage,
	},
}

// runWorkflowDeterminismCheck performs deterministic workflow auditing.
// It scans markdown workflow files in .agent/workflows/ to ensure every shell command
// and flag declared in workflow documentation exists natively in the bin/nomos engine.
func runWorkflowDeterminismCheck(ctx *workspace.WorkspaceContext) (StageResult, error) {
	root := ctx.RepoRoot
	// Execute workflow AST parser audit across repository template paths
	err := CheckSSOTDrift(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(root); return c }())
	if err != nil {
		return StageResult{Passed: false, Message: "SSOT drift detected"}, err
	}
	return StageResult{Passed: true, Message: "All workflows are strictly deterministic"}, nil
}

// hasGoFiles checks if there are any modified .go files in the workspace.
func hasGoFiles(ctx *workspace.WorkspaceContext) bool {
	root := ctx.RepoRoot
	modified, err := GetModifiedFiles(root)
	if err != nil {
		return false
	}
	for f := range modified {
		if strings.HasSuffix(f, ".go") {
			return true
		}
	}
	return false
}

// skipIfNotGo wraps a verification stage and skips it if no .go files were modified in the workspace.
func skipIfNotGo(name string, run func(ctx *workspace.WorkspaceContext) (StageResult, error)) func(ctx *workspace.WorkspaceContext) (StageResult, error) {
	return func(ctx *workspace.WorkspaceContext) (StageResult, error) {
		if !hasGoFiles(ctx) {
			return StageResult{
				Name:    name,
				Passed:  true,
				Message: "Skipped (No .go files modified)",
			}, nil
		}
		return run(ctx)
	}
}

// getDoDStageNames returns the names of all currently registered DoD verification stages.
func getDoDStageNames() []string {
	names := make([]string, len(DoDStages))
	for i, s := range DoDStages {
		names[i] = s.Name
	}
	return names
}

// getStageGuidance looks up the guidance text for a given stage name.
func getStageGuidance(name string) string {
	for _, stage := range DoDStages {
		if stage.Name == name && stage.Guidance != "" {
			return "\n    \x1b[1;33m💡 Guidance:\x1b[0m " + stage.Guidance
		}
	}
	return ""
}

// runSystemHealthDoctorCheck executes the AuditHealth diagnostic checks.
// If port checks fail, it reports warnings. If hooks/locks fail, it reports hard failures.
func runSystemHealthDoctorCheck(ctx *workspace.WorkspaceContext) (StageResult, error) {
	root := ctx.RepoRoot
	status, err := AuditHealth(root)
	if err != nil {
		return StageResult{Passed: false, Message: fmt.Sprintf("Health audit failed: %v", err)}, err
	}

	// Stale database locks or symlink hook failures are hard errors
	if !status.GitHooksHealthy {
		return StageResult{Passed: false, Message: "Git hooks are broken or missing. Run 'bin/nomos doctor' to repair."}, fmt.Errorf("git hooks symlinks are invalid")
	}

	// Port failures (LlamaServer / Cockpit) are soft warnings (do not block DoD)
	var warnings []string
	if !status.LlamaAlive {
		warnings = append(warnings, "LlamaServer service down")
	}
	if !status.CockpitAlive {
		warnings = append(warnings, "Cockpit Control Plane down")
	}

	if len(warnings) > 0 {
		return StageResult{Passed: true, Message: fmt.Sprintf("Passed with warnings: %s", strings.Join(warnings, ", "))}, nil
	}

	return StageResult{Passed: true, Message: "System health is fully healthy"}, nil
}

// runPathHardcodingBlocker delegates to the CheckHardcodedPaths analyzer.
func runPathHardcodingBlocker(ctx *workspace.WorkspaceContext) (StageResult, error) {
	root := ctx.RepoRoot
	files, err := getStagedFiles(root)
	if err != nil {
		return StageResult{Passed: false, Message: "Failed to get staged files."}, err
	}

	findings := CheckHardcodedPaths(root, files)
	if len(findings) == 0 {
		return StageResult{Passed: true, Message: "No hardcoded internal paths detected."}, nil
	}

	errDetails := "Hardcoded system paths found:\n"
	for _, f := range findings {
		errDetails += fmt.Sprintf(" - %s:%d %s\n", f.File, f.Line, f.Func)
	}

	return StageResult{
		Passed: false,
	}, fmt.Errorf("path hardcoding constraints violated:\n%s", errDetails)
}

// runMagicStringDetectorCheck delegates to the CheckMagicStrings analyzer.
// It is registered as part of the DoD pipeline to enforce strict typing on domain functions.
func runMagicStringDetectorCheck(ctx *workspace.WorkspaceContext) (StageResult, error) {
	root := ctx.RepoRoot
	// First, fetch the list of currently staged files from the git repository context.
	files, err := getStagedFiles(root)
	if err != nil {
		return StageResult{Passed: false, Message: "Failed to get staged files."}, err
	}

	// Hand off the raw files to the AST Magic String scanner logic.
	findings := CheckMagicStrings(root, files)
	if len(findings) == 0 {
		return StageResult{Passed: true, Message: "No magic strings detected in critical function calls."}, nil
	}

	// If violations occurred, bundle them into a comprehensive error string format
	// that guides the user towards adopting properly defined domain constants.
	errDetails := "Magic strings found in domain-critical functions:\n"
	for _, f := range findings {
		errDetails += fmt.Sprintf(" - %s:%d %s\n", f.File, f.Line, f.Func)
	}

	return StageResult{
		Passed: false,
	}, fmt.Errorf("magic string constraints violated:\n%s", errDetails)
}

// runGitBrainIndexStage ensures the GitBrain vector cache is in sync with notes and codebase.
func runGitBrainIndexStage(ctx *workspace.WorkspaceContext) (StageResult, error) {
	root := ctx.RepoRoot
	dbPath := ctx.DbPath("gitbrain.db")

	if err := gitbrain.IndexNotes(root, dbPath); err != nil {
		return StageResult{Passed: false, Message: "Failed to index git notes"}, err
	}

	if err := gitbrain.IndexCodebase(root, dbPath); err != nil {
		return StageResult{Passed: false, Message: "Failed to index codebase"}, err
	}

	return StageResult{Passed: true, Message: "GitBrain vector cache is synchronized"}, nil
}
