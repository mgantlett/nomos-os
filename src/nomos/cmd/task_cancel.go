package cmd

import (
	"context"
	"fmt"

	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-commons/src/nomos/core/telemetry"
	"path/filepath"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/gitops"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/spf13/cobra"
)

var taskCancelCmd = &cobra.Command{
	Use:   "cancel [task-key] [comment]",
	Short: "Cancel a task, moving it to CANCELLED status",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, comment, tracker, repoRoot, err := parseTaskArgsAndLoadTracker(args, "Cancelled by Nomos")
		if err != nil {
			return err
		}

		ctx := context.Background()
		fmt.Printf("Cancelling task %s...\n", key)
		if err := tracker.Cancel(ctx, key, comment); err != nil {
			return err
		}

		_ = telemetry.EmitEvent(repoRoot, "task_cancel", fmt.Sprintf("Task ID: %s | Reason: %s", key, comment))

		// Transition back to IDLE if the active task was cancelled
		if state, _ := task.GetPhaseState(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }()); state != nil && state.TaskId == key {
			_ = task.TransitionPhase(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), statepkg.PhaseIdle)
			
			// Teardown orphaned transient worktrees for the cancelled task
			wtPath := filepath.Join(workspace.MustNewContext(repoRoot).WorktreesDir(), filepath.Base(filepath.Clean(repoRoot))+"-"+key)
			branch := "feature/" + key
			gitops.TeardownWorktree(wtPath, branch, "develop", repoRoot, key, false)
			
			fmt.Printf("✅ Active task %s cancelled. Workspace reset to %s phase.\n", key, statepkg.PhaseIdle)
		}

		return nil
	},
}

func init() {
	taskCmd.AddCommand(taskCancelCmd)
}
