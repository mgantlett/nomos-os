package cmd

import (
	"fmt"
	"os"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/spf13/cobra"
)

// executeLocalReset performs local workspace reset by discarding changes or stashing them, and transitioning phase to IDLE.
// It resolves the current directory context, reads the active task ID, runs git cleanups, and resets state.
func executeLocalReset(stash bool) error {
	// Retrieve the current working directory to identify the active workspace.
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Locate repository root.
	repoRoot := findRepoRoot(wd)

	return task.ResetTask(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), wd, stash)
}

// taskResetCmd abandons active work on a task locally, cleaning local modifications and transitioning phase to IDLE.
// If a task ID is provided, it resets that task in the backend to BACKLOG status and unassigns it.
var taskResetCmd = &cobra.Command{
	Use:   "reset [task-key]",
	Short: "Abandon local work on the active task, or reset a specific task to BACKLOG",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			// Run local reset without stashing (discards progress)
			err := executeLocalReset(false)
			if err == nil {
				fmt.Println("✅ Task reset successfully.")
			}
			return err
		}

		key, _, tracker, _, err := parseTaskArgsAndLoadTracker(args, "")
		if err != nil {
			return err
		}

		ctx := cmd.Context()
		fmt.Printf("Resetting task %s back to BACKLOG...\n", key)
		if err := tracker.ResetBackend(ctx, key); err != nil {
			return err
		}
		fmt.Println("✅ Task reset to BACKLOG successfully.")
		return nil
	},
}

// taskParkCmd pauses active work on a task locally, stashing changes and transitioning phase to IDLE.
var taskParkCmd = &cobra.Command{
	Use:   "park",
	Short: "Park/stash local work on the active task and transition phase to IDLE",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Run local reset with stashing (parks progress)
		err := executeLocalReset(true)
		if err == nil {
			fmt.Println("✅ Task parked successfully.")
		}
		return err
	},
}

// init registers the task reset and park commands to the parent taskCmd.
func init() {
	taskCmd.AddCommand(taskResetCmd)
	taskCmd.AddCommand(taskParkCmd)
}
