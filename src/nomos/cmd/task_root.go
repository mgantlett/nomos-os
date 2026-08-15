package cmd

import (
	"github.com/spf13/cobra"
)

// taskCmd is the parent command for task subcommands
var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage workspace tasks and backlog items",
}

// init registers task commands and their command line flags to the parent cobra RootCmd.
// It sets up the following subcommand options:
// - task list:   Display interactive table of active issues
// - task view:   Inspect details of a specific issue
// - task create: Backlog item initialization command (supports --file, --burden, --depth, and --label)
// - task start:  Branch scaffolding and transition to IN PROGRESS
// - task close:  Merge verification and transition to DONE
func init() {
	RootCmd.AddCommand(taskCmd)
}
