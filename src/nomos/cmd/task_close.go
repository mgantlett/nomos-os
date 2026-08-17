package cmd

import (
	"context"
	"fmt"
	"os/exec"

	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-commons/src/nomos/core/telemetry"
	"path/filepath"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/gitops"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/mgantlett/nomos-os/src/nomos/modules/verify"
	"github.com/spf13/cobra"
)

// taskCloseCmd handles ticketing backend closures by invoking close API actions.
// It accepts the active task key and an optional resolution comment.
// 1. It parses command arguments to extract the task ID and comment details.
// 2. It connects to the active task tracking backend.
// 3. It transitions the ticket status to Closed in the remote server.
var taskCloseCmd = &cobra.Command{
	Use:   "close [task-key] [comment]",
	Short: "Close a specific task",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, comment, tracker, repoRoot, err := parseTaskArgsAndLoadTracker(args, "Closed by Nomos")
		if err != nil {
			return err
		}

		ctx := context.Background()
		fmt.Printf("Closing task %s...\n", key)
		if err := tracker.Close(ctx, key, comment); err != nil {
			return err
		}

		if stashID, found := DetectStashForTask(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), key); found {
			fmt.Printf("🧹 Auto-pruning orphaned stash %s for closed task %s...\n", stashID, key)
			dropCmd := exec.Command("git", "stash", "drop", stashID)
			dropCmd.Dir = repoRoot
			_ = dropCmd.Run()
		}

		_ = telemetry.EmitEvent(repoRoot, "task_close", fmt.Sprintf("Task ID: %s | Reason: %s", key, comment))
		verify.PruneQualityDebtForTask(repoRoot, key)

		// Transition back to IDLE if the active task was closed
		if state, _ := task.GetPhaseState(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }()); state != nil && state.TaskId == key {
			_ = task.TransitionPhase(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), statepkg.PhaseIdle)
			
			// Teardown orphaned transient worktrees for the closed task
			wtPath := filepath.Join(workspace.MustNewContext(repoRoot).WorktreesDir(), filepath.Base(filepath.Clean(repoRoot))+"-"+key)
			branch := "feature/" + key
			gitops.TeardownWorktree(wtPath, branch, "develop", repoRoot, key)
			
			fmt.Printf("✅ Active task %s closed. Workspace reset to %s phase.\n", key, statepkg.PhaseIdle)
		}

		return nil
	},
}

func init() {
	taskCmd.AddCommand(taskCloseCmd)
}
