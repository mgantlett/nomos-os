// Package task provides lifecycle management for local agile tasks and state transitions.
// This handles the loading, saving, and verifying of JSON task definitions.
// It interfaces with GitBrain and the Phase state engine to enforce Definition of Done rules.
package task

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/plugin"
	"github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-commons/src/nomos/core/telemetry"
)

// EscalationEvaluatorFunc is a pluggable callback hook for evaluating closed-loop Swarm escalation.
// This decouples the task lifecycle package from the engine package to prevent Go import cycles.
var EscalationEvaluatorFunc func(ctx *workspace.WorkspaceContext, key string, failCount int, detail string) (bool, string, error)

// PostPhaseComment posts a comment to the issue tracker when local phase transitions occur.
// It formats markdown updates for PLAN, EDIT, and REVIEW phases and submits them to the tracker.
func PostPhaseComment(ctx *workspace.WorkspaceContext, key string, phase state.WorkspacePhase) {
	repoRoot := ctx.RepoRoot
	// Guard against empty key or phase strings
	if key == "" || phase == "" {
		// Log debug warning if key or phase is empty
		return
	}

	// Instantiate task tracker for the specified repository root path
	tracker, err := getTracker(ctx)
	if err != nil {
		// Return gracefully if tracker instance initialization fails
		return
	}

	// Parse CSV task keys string if multiple tasks are bound
	keys := strings.Split(key, ",")
	for _, k := range keys {
		// Clean leading and trailing whitespace from task key string
		k = strings.TrimSpace(k)
		if k == "" {
			// Skip empty key string entry
			continue
		}

		// Determine formatted comment body based on target phase
		var comment string
		switch phase {
		case state.PhasePlan:
			// Scaffolding implementation plan update comment
			comment = fmt.Sprintf("🚀 Task %s started by AI agent. Workspace initialized in PLAN phase. Scaffolding implementation plan...", k)
		case state.PhaseEdit:
			// Implementation phase update comment
			comment = fmt.Sprintf("🛠️  Task %s implementation in progress (EDIT phase). Executing code changes and TDD verification.", k)
		case state.PhaseReview:
			// Review stage update comment
			comment = fmt.Sprintf("🔍 Task %s implementation completed (REVIEW phase). Staged changes awaiting DoD verification and PO review.", k)
			// Trigger automated artifact sync to copy brain documents to .nomos workspace storage
			SyncWorkspaceArtifacts(repoRoot, k)
		default:
			// Do nothing for other intermediate phases or IDLE state
			return
		}

		// Submit comment to issue tracker with 10-second timeout context
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		// Post comment payload via issue tracker client interface
		_ = tracker.Comment(ctx, k, comment)
		cancel()
	}
}

// PostDoDFailure posts a detailed warning comment when DoD verification fails in an active AI session.
// It formats the breakdown of failing DoD verification gates as a markdown list.
// Swarm telemetry failure metrics are recorded to track consecutive failures per task.
// Closed-loop escalation callbacks evaluate whether tier advancement is required.
// Comments are published to local JSON tasks or remote tracker backends.
func PostDoDFailure(ctx *workspace.WorkspaceContext, key string, failMsgs []string) {
	// Return early if key is empty or no failure messages are provided
	if key == "" || len(failMsgs) == 0 {
		return
	}

	// Retrieve active task tracker instance from repo root configuration
	tracker, err := getTracker(ctx)
	if err != nil {
		return
	}

	// Format list of failed verification gates into markdown list items for tracking comment
	var gateLines []string
	for _, m := range failMsgs {
		// Append each failure message as a bullet point entry
		gateLines = append(gateLines, fmt.Sprintf("- %s", m))
	}

	// Join failure messages into detailed string payload for telemetry logging
	detailStr := strings.Join(failMsgs, "; ")

	// Iterate over target task keys when handling comma-separated tasks
	keys := strings.Split(key, ",")
	for _, k := range keys {
		// Clean task key string by removing leading/trailing spaces
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}

		// Record failure count in GlobalSwarmAggregator telemetry instance
		failCount := telemetry.GlobalSwarmAggregator.RecordDoDFailure(k, "swarm", detailStr)

		// Evaluate closed-loop auto-escalation via pluggable callback hook
		if EscalationEvaluatorFunc != nil {
			// Trigger escalation logic if threshold limit is reached
			_, _, _ = EscalationEvaluatorFunc(ctx, k, failCount, detailStr)
		}

		// Construct formatted warning comment body with failure breakdown
		comment := fmt.Sprintf("⚠️  Definition of Done (DoD) verification failed for task %s:\n%s\n\nAI progress is temporarily blocked until these quality gates pass.", k, strings.Join(gateLines, "\n"))

		// Post warning comment to tracker backend using 10-second timeout context
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_ = tracker.Comment(ctx, k, comment)
		cancel()
	}
}

// getTracker resolves credentials configuration and constructs the tracker object for a repo root.
func getTracker(ctx *workspace.WorkspaceContext) (Tracker, error) {
	// Load configuration settings for active repository workspace
	cfg, err := LoadConfig(ctx)
	if err != nil {
		// Return error if config failed to load
		return nil, err
	}
	// Construct new tracker instance from loaded workspace configuration settings
	return NewTracker(cfg)
}

// findLatestBrainArtifact determines the most recently modified file from a list of glob matches.
// It iterates through all matched artifacts and stats them to compare modification times.
// Returns the path to the artifact with the highest UnixNano timestamp.
func findLatestBrainArtifact(matches []string) string {
	latest := ""
	var latestTime int64 = 0
	for _, match := range matches {
		if fi, err := os.Stat(match); err == nil && fi.ModTime().UnixNano() > latestTime {
			latestTime = fi.ModTime().UnixNano()
			latest = match
		}
	}
	return latest
}

// syncArtifactType handles the discovery and copying of a specific artifact type from brain to workspace.
// It locates the brain pattern, resolves the latest artifact, and saves it into the correct directory.
func syncArtifactType(repoRoot string, homeDir string, name string, dir string, filename string) {
	// Construct glob search string for brain artifacts
	brainPattern := filepath.Join(homeDir, ".gemini", "antigravity-ide", "brain", "*", name)
	matches, _ := filepath.Glob(brainPattern)
	if len(matches) == 0 {
		// Check local repository root if no brain artifact was found
		rootFile := filepath.Join(repoRoot, name)
		if _, err := os.Stat(rootFile); err == nil {
			matches = []string{rootFile}
		}
	}

	// Copy discovered artifact content into destination directory
	if len(matches) > 0 {
		// Extract most recently modified conversation artifact instead of alphabetically last
		latest := findLatestBrainArtifact(matches)
		if latest == "" {
			latest = matches[len(matches)-1] // Fallback to lexicographically last if stat fails
		}
		data, err := os.ReadFile(latest)
		if err == nil {
			// Destination storage path within repository state tree
			targetDir := filepath.Join(workspace.MustNewContext(repoRoot).DataDir(), dir)
			_ = os.MkdirAll(targetDir, 0755)
			_ = os.WriteFile(filepath.Join(targetDir, filename), data, 0644)

			// Duplicate walkthrough file to temporary directory for parity validation
			if name == workspace.WalkthroughFileName {
				// We also keep a cached working copy of the walkthrough in the local tmp dir
				// for instantaneous commits without re-polling the brain
				stagingPath := workspace.MustNewContext(repoRoot).WalkthroughStagingPath()
				_ = os.MkdirAll(filepath.Dir(stagingPath), 0755)
				_ = os.WriteFile(stagingPath, data, 0644)
			}
		}
	}
}

// SyncWorkspaceArtifacts discovers generated brain artifacts and syncs them to workspace SSoT paths.
func SyncWorkspaceArtifacts(repoRoot string, taskID string) {
	// Terminate execution immediately if essential parameters are blank
	if taskID == "" || repoRoot == "" {
		return
	}
	// Fetch target operating system user profile directory
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		return
	}

	// Catalog of primary markdown documentation types generated by AI agents during task lifecycle
	artifactTypes := []struct {
		name     string
		dir      string
		filename string
	}{
		// Architectural design and implementation strategy document
		{"implementation_plan.md", "plans", fmt.Sprintf("%s.md", taskID)},
		// Execution summary and verification results artifact
		{workspace.WalkthroughFileName, "walkthroughs", fmt.Sprintf("%s.md", taskID)},
		// Educational deep-dive explaining technical decisions
		{"explainer.md", "explainers", fmt.Sprintf("%s.md", taskID)},
		// Self-assessment comprehension testing questions
		{"quiz.md", "quizzes", fmt.Sprintf("%s.md", taskID)},
	}

	// Scan conversation brain artifacts directory for matching files
	for _, art := range artifactTypes {
		syncArtifactType(repoRoot, homeDir, art.name, art.dir, art.filename)
	}
}

// IndexArtifactsToGitBrain executes the enterprise GitBrain module via IPC to index notes/code.
func IndexArtifactsToGitBrain(repoRoot string, taskID string) {
	plugins, err := plugin.DiscoverPlugins(repoRoot)
	if err != nil {
		return
	}

	for _, p := range plugins {
		if filepath.Base(p) == "nomos-plugin-gitbrain" {
			_, _ = plugin.CallPlugin(p, "index", map[string]string{
				"repoRoot": repoRoot,
				"taskID":   taskID,
			})
			return
		}
	}
}

// ValidateWorkspaceTaskContext compares the target task's project name against the active repository root directory.
// It returns a hard error if an agent attempts to initiate a task belonging to a different project root context.
func ValidateWorkspaceTaskContext(root string, taskKey string, taskProject string) error {
	if root == "" || taskProject == "" {
		return nil
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	currentProjectBase := filepath.Base(filepath.Clean(absRoot))

	normCurrent := strings.ToLower(strings.TrimSpace(currentProjectBase))
	normProject := strings.ToLower(strings.TrimSpace(taskProject))

	isMatch := normCurrent == normProject
	if !isMatch && normProject == "nomos" && strings.HasPrefix(normCurrent, "nomos-") {
		isMatch = true
	}

	if !isMatch {
		return fmt.Errorf("Workspace Context Mismatch: task %s belongs to project '%s', but active workspace root is '%s'. Please switch to the '%s' workspace root before initiating this task.", taskKey, taskProject, currentProjectBase, taskProject)
	}

	return nil
}
