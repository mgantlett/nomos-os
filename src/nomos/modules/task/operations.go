// Operations package handles task lifecycle execution.
// It manages phase transitions and workspace states for active sprints.
// Holy ghost context generation is executed here.
// Package task manages the active state, phase discipline, and prompt engineering.
// Operations provides the core CRUD logic for managing the JSON state representation
// of ongoing tasks. It abstracts away direct filesystem interactions to maintain
// compliance with the Data Integrity Gate and prevent state hash mismatches.
package task

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/telemetry"
	nomosexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/mgantlett/nomos-os/src/nomos/modules/schema"
)

// RunHygieneCleanups performs workspace maintenance and resets local locks.
func RunHygieneCleanups(ctx *workspace.WorkspaceContext) error {
	repoRoot := ctx.RepoRoot
	// 1. Unset any stale PO approval locks to prevent ghost approvals from lingering.
	poApprovalPath := filepath.Join(workspace.MustNewContext(repoRoot).TmpDir(), ".po_approval_granted")
	_ = os.Remove(poApprovalPath)
	return nil
}

func writePhaseState(ctx *workspace.WorkspaceContext, key, assignee, agentFlag string) error {
	repoRoot := ctx.RepoRoot
	phaseStatePath := workspace.MustNewContext(repoRoot).NomosStatePath(".phase_state.json")
	agentType := "ide"
	if strings.HasPrefix(agentFlag, "swarm") || strings.HasPrefix(assignee, "swarm:") {
		agentType = "swarm"
	}

	// Read existing phase state to preserve TasksCompletedInSession across task starts
	tasksCompleted := 0
	if existingState, err := GetPhaseState(ctx); err == nil {
		tasksCompleted = existingState.TasksCompletedInSession
	}

	phaseData := map[string]interface{}{
		"agent":                      assignee,
		"agent_type":                 agentType,
		"task_id":                    key,
		"dod_failure_count":          0,
		"session_started_at":         time.Now().Format(time.RFC3339),
		"session_commits":            0,
		"tasks_completed_in_session": tasksCompleted,
	}
	phaseBytes, err := json.MarshalIndent(phaseData, "", "  ")
	if err == nil {
		if _, errStat := os.Stat(phaseStatePath); errStat == nil {
			_ = os.Chmod(phaseStatePath, 0600)
		}
		// In a Zero-Footprint distributed architecture, the CLI Bootstrapper
		// no longer pre-generates the global `.nomos/` directory footprints.
		// We must explicitly ensure that the parent state directory exists
		// before writing the JSON state document, to prevent fatal
		// "no such file or directory" os.WriteFile crashes on fresh setups.
		_ = os.MkdirAll(filepath.Dir(phaseStatePath), 0755)
		_ = os.WriteFile(phaseStatePath, phaseBytes, 0440)
		_ = os.Chmod(phaseStatePath, 0440)
	}
	return err
}

// StartTrackerOnly isolates the tracker update and telemetry emitting without mutating local workspace state.
func StartTrackerOnly(ctx context.Context, wCtx *workspace.WorkspaceContext, tracker Tracker, key string, assignee string) (*Task, error) {
	repoRoot := wCtx.RepoRoot
	t, err := tracker.View(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("failed to load task for tracker update: %w", err)
	}

	fmt.Printf("Starting task %s in tracking backend (JIT Assignee: %s)...\n", key, assignee)
	if err := tracker.Start(ctx, key, assignee); err != nil {
		return nil, err
	}

	detailMsg := fmt.Sprintf("Task ID: %s | Assignee: %s | Summary: %s | Size: %d", key, assignee, t.Title, t.ContextBurden+t.LogicDepth)
	_ = telemetry.EmitEvent(repoRoot, "task_start", detailMsg)

	return t, nil
}

// StartTask centralizes task initialization logic, applying JIT agent routing,
// workspace transitions, and context generation.
func StartTask(ctx context.Context, wCtx *workspace.WorkspaceContext, tracker Tracker, key string, assignee string, agentFlag string, injectExemptionsFunc func(string, string) string) (string, string, error) {
	repoRoot := wCtx.RepoRoot
	_ = RunHygieneCleanups(wCtx)

	t, err := StartTrackerOnly(ctx, wCtx, tracker, key, assignee)
	if err != nil {
		return "", "", err
	}

	if injectExemptionsFunc != nil {
		newBody := injectExemptionsFunc(t.Description, assignee)
		if newBody != t.Description {
			err = tracker.Edit(ctx, t.Key, nil, &newBody, nil, nil, nil, nil, nil, nil)
		}
	}

	taskMdPath := filepath.Join(workspace.MustNewContext(repoRoot).TmpDir(), "task.md")
	taskContent := fmt.Sprintf("# Task %s: %s\n\n%s\n\n%s\n", key, t.Title, schema.DeepReviewChecklistItem, t.Description)

	// Similar to the phase state path above, the temporary data directory
	// may not yet exist on this machine for this specific workspace.
	// We dynamically construct the parent directory tree so the
	// task markdown document can be written safely without errors.
	_ = os.MkdirAll(filepath.Dir(taskMdPath), 0755)
	_ = os.WriteFile(taskMdPath, []byte(taskContent), 0644)

	if err := os.MkdirAll(workspace.MustNewContext(repoRoot).StateDir(), 0755); err != nil {
		return "", "", err
	}

	stateTaskIdPath := workspace.MustNewContext(repoRoot).NomosStatePath(".state_task_id")
	if err := os.WriteFile(stateTaskIdPath, []byte(key), 0644); err != nil {
		return "", "", err
	}

	_ = writePhaseState(wCtx, key, assignee, agentFlag)

	if err := TransitionPhase(wCtx, "PLAN"); err != nil {
		return "", "", err
	}

	dbPath := workspace.MustNewContext(repoRoot).DbPath("cache.db")
	if out, err := nomosexec.GitStashPopByName(dbPath, repoRoot, "nomos-park-task-"+key); err == nil && out != "" {
		fmt.Printf("Restored parked uncommitted changes for task %s from git stash.\n", key)
	}
	_ = GenerateHolyGhostContext(ctx, wCtx, tracker, key)

	return assignee, agentFlag, nil
}

// ResetTask performs local workspace reset by discarding changes or stashing them,
// and transitioning phase to IDLE.
func ResetTask(ctx *workspace.WorkspaceContext, wd string, stash bool) error {
	repoRoot := ctx.RepoRoot
	dbPath := workspace.MustNewContext(repoRoot).DbPath("cache.db")

	// Resolve the active task ID from the local state file cache within this workspace directory.
	stateTaskIdPath := workspace.MustNewContext(wd).NomosStatePath(".state_task_id")
	bytes, err := os.ReadFile(stateTaskIdPath)
	if err != nil {
		return fmt.Errorf("no active task found in this workspace context: %w", err)
	}
	taskID := strings.TrimSpace(string(bytes))
	if taskID == "" {
		return fmt.Errorf("no active task found in this workspace context")
	}

	if stash {
		// Parking changes: stash local work using directory-scoped git command before transitioning phase.
		fmt.Printf("Parking task %s inside workspace %s...\n", taskID, wd)
		_, _ = nomosexec.GitStashSave(dbPath, wd, "nomos-park-task-"+taskID)
	} else {
		// Resetting changes: discard local modifications completely via git checkout and clean.
		fmt.Printf("Resetting task %s inside workspace %s...\n", taskID, wd)
		_, _ = nomosexec.RunCommand(dbPath, "git", "-C", wd, "checkout", ".")
		_, _ = nomosexec.RunCommand(dbPath, "git", "-C", wd, "clean", "-fd", "-e", "*.db", "-e", "."+"nomos/data/*")
	}

	// Transition the workspace phase state locally to IDLE using unified executor helper.
	_ = TransitionPhase(ctx, "IDLE")

	fmt.Printf("✅ Task %s workspace reset completed. Session unlocked.\n", taskID)
	return nil
}
