/*
Package cmd provides the CLI commands for the Nomos orchestrator.
This file houses the 'task batch' command which visualizes the progress of a parent task.
*/
package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// taskBatchCmd visually tracks the completion percentage of a batch based on child tasks.
var taskBatchCmd = &cobra.Command{
	Use:   "batch [key]",
	Short: "View the progress rollup of a batch based on its child tasks",
	Args:  cobra.ExactArgs(1),
	// Execute handles the core logic for the 'batch' subcommand.
	// It parses the CLI arguments to identify the target batch key,
	// validates the input, and delegates to the Task API to fetch rollup data.
	// The function also supports outputting raw JSON via the --json flag.
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]

		// Call the application layer to gather batch statistics and tasks.
		// Returns a formatted output struct with percent completion and metadata.
		ctx := context.Background()
		tracker, parent, _, err := loadTrackerAndTask(ctx, key)
		if err != nil {
			return err
		}

		// Retrieve all tracked task items from the underlying data store.
		tasks, err := tracker.List(ctx)
		if err != nil {
			return err
		}

		total := 0
		closed := 0

		// Iterate over all tasks to find children of this batch.
		// We count both total child tasks and closed tasks to compute the rollup progress.
		for _, t := range tasks {
			if t.ParentKey == key {
				total++
				if t.IsClosed() {
					closed++
				}
			}
		}

		if total == 0 {
			fmt.Printf("Batch %s (%s) has no child tasks.\n", key, parent.Title)
			return nil
		}

		percent := float64(closed) / float64(total) * 100.0
		barBlocks := int(percent / 10)
		bar := strings.Repeat("█", barBlocks) + strings.Repeat("░", 10-barBlocks)

		fmt.Printf("Batch: %s (%s)\n", key, parent.Title)
		fmt.Printf("Progress: [%s] %.1f%% (%d/%d tasks completed)\n", bar, percent, closed, total)

		return nil
	},
}

func init() {
	taskCmd.AddCommand(taskBatchCmd)
}
