package cmd

import (
	"context"
	"fmt"
	"os"

	"path/filepath"

	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-commons/src/nomos/core/telemetry"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/gitops"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/mgantlett/nomos-os/src/nomos/modules/verify"
	"github.com/spf13/cobra"
)

var syncFile string

var taskSyncCmd = &cobra.Command{
	Use:   "sync [targetEnv]",
	Short: "Natively execute AI-AI DDP Direct Sync",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetEnv := "develop"
		if len(args) > 0 {
			targetEnv = args[0]
		}

		wd, err := os.Getwd()
		if err != nil {
			return err
		}

		repoRoot := findRepoRoot(wd)

		if err := enforceRootZone(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), "task sync"); err != nil {
			return err
		}

		taskID := verify.GetActiveTaskId(repoRoot)
		if taskID == "" {
			return fmt.Errorf("no active task found to sync")
		}

		wtPath := filepath.Join(workspace.MustNewContext(repoRoot).WorktreesDir(), filepath.Base(repoRoot)+"-"+taskID)
		if _, err := os.Stat(wtPath); os.IsNotExist(err) {
			return fmt.Errorf("active task worktree not found at %s", wtPath)
		}

		fmt.Printf("🚀 Natively executing direct sync from worktree %s into %s...\n", wtPath, targetEnv)
		if err := gitops.DirectMerge(wtPath, func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), targetEnv, syncFile); err != nil {
			return fmt.Errorf("direct sync failed: %w", err)
		}

		if taskID != "" {
			tracker, err := loadTrackerForRoot(repoRoot)
			if err != nil {
				fmt.Printf("⚠️  Warning: Failed to load task tracker to close task %s: %v\n", taskID, err)
			} else {
				ctx := context.Background()
				comment := "Synced natively via nomos task sync"
				fmt.Printf("Closing task %s...\n", taskID)
				if err := tracker.Close(ctx, taskID, comment); err != nil {
					fmt.Printf("⚠️  Warning: Failed to close task %s: %v\n", taskID, err)
				} else {
					_ = telemetry.EmitEvent(repoRoot, "task_close", fmt.Sprintf("Task ID: %s | Reason: %s", taskID, comment))
					verify.PruneQualityDebtForTask(repoRoot, taskID)

					if state, _ := task.GetPhaseState(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }()); state != nil && state.TaskId == taskID {
						_ = task.TransitionPhase(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), statepkg.PhaseIdle)
						fmt.Printf("✅ Active task %s closed. Workspace reset to %s phase.\n", taskID, statepkg.PhaseIdle)
					}
				}
			}
		}

		return nil
	},
}

func init() {
	taskSyncCmd.Flags().StringVarP(&syncFile, "file", "F", "", "Path to walkthrough markdown file to use as commit message")
	taskCmd.AddCommand(taskSyncCmd)
}
