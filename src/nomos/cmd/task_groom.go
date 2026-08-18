package cmd

import (
	"context"
	"path/filepath"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/spf13/cobra"
)

// Task Groom Package
//
// This module provides the `nomos task groom` CLI command.
// It acts as the operational entry point for the automated heuristic backlog groomer.
// The groomer scans the current project backlog and intelligently bundles smaller
// or duplicate tasks into unified Epics based on semantic similarity.
//
// 1. Invokes semantic bundling logic through the task module.
// 2. Prunes old resolved stories and archives completed tasks automatically.
// 3. Flags interactive mode for user confirmation (or dry run analysis).
//
// Usage:
//   nomos task groom --project my_project

var taskGroomProjectFlag string
var taskGroomYesFlag bool

// taskGroomCmd runs the heuristic backlog groomer to auto-bundle tasks
var taskGroomCmd = &cobra.Command{
	Use:   "groom",
	Short: "Run the heuristic backlog groomer to automatically bundle tasks",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		tracker, repoRoot, err := loadTrackerAndRoot()
		if err != nil {
			return err
		}
		ctx := context.Background()

		projSettings, _ := config.LoadProjectSettings(repoRoot)
		if projSettings != nil && projSettings.Capacity != nil && !cmd.Flags().Changed("capacity") {
			taskGroomCapacityFlag = *projSettings.Capacity
		}

		// Resolve the target project filter for the groomer execution.
		// If no specific project is passed via the --project flag, we default
		// to the base name of the current working repository directory.
		// If the user specifies "all", we empty the filter to allow global grooming.
		projectFilter := taskGroomProjectFlag
		if projectFilter == "" {
			projectFilter = filepath.Base(repoRoot) // default to current directory project
			if projSettings != nil && projSettings.DefaultProject != "" {
				projectFilter = projSettings.DefaultProject
			}
		} else if projectFilter == "all" {
			projectFilter = ""
		}

		// Execute the core semantic grouping heuristic across the filtered project backlog.
		// We enforce a hard ceiling of 13.0 complexity size for any single generated Epic bundle,
		// ensuring that the resulting AI task dispatch does not exceed Tier 1 cognitive
		// processing limits (context window caps and max iteration limits).
		return task.GroomBacklog(ctx, func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), tracker, taskGroomCapacityFlag, projectFilter, taskGroomYesFlag)
	},
}

var taskGroomCapacityFlag int

func init() {
	taskGroomCmd.Flags().StringVar(&taskGroomProjectFlag, "project", "", "Override the project assignment (defaults to current directory name, use 'all' for all projects)")
	taskGroomCmd.Flags().BoolVarP(&taskGroomYesFlag, "yes", "y", false, "Automatically approve bundling without prompt")
	taskGroomCmd.Flags().IntVar(&taskGroomCapacityFlag, "capacity", 13, "Maximum combined ContextBurden + LogicDepth for a single bundle or Sprint cycle")
	taskCmd.AddCommand(taskGroomCmd)
}
