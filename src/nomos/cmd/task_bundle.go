// Package cmd provides the command-line interface for Nomos.
// The task bundle command is used to consolidate task context and state.
// It bundles artifacts, local configuration, and active phase state into
// a single transportable archive that Tier 2 agents can utilize.
package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-os/src/nomos/modules/schema"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/spf13/cobra"
)

// taskBundleCmd initiates an ephemeral Roll-up Epic containing multiple quality debt tasks.
// This allows engineers or AI agents to group several small tasks together
// under a single temporary umbrella ticket for atomic execution and tracking.
// Note: This does not delete the original tasks, but creates a new Epic that
// tracks their collective completion in its acceptance criteria.
var taskBundleCmd = &cobra.Command{
	Use:   "bundle [task-key-1] [task-key-2]...",
	Short: "Bundle multiple tasks into an ephemeral Roll-up Epic",
	Args:  cobra.MinimumNArgs(2),
	// RunE executes the bundle command by aggregating the requested tasks
	// and generating a new native StorySchema to wrap them.
	RunE: func(cmd *cobra.Command, args []string) error {
		noStartVal, _ := cmd.Flags().GetBool("no-start")

		tracker, repoRoot, err := loadTrackerAndRoot()
		if err != nil {
			return err
		}

		if err := enforceRootZone(repoRoot, "task bundle"); err != nil {
			return err
		}

		ctx := context.Background()
		var bundledTasks []task.Task
		var bundledKeys []string

		for _, key := range args {
			t, err := tracker.View(ctx, key)
			if err != nil {
				return fmt.Errorf("failed to fetch task %s: %w", key, err)
			}

			bundledTasks = append(bundledTasks, *t)
			bundledKeys = append(bundledKeys, key)
		}

		s := &schema.TaskSchema{
			Description:        fmt.Sprintf("Bundle resolution for tasks: %s", strings.Join(bundledKeys, ", ")),
			AcceptanceCriteria: []string{},
			TechnicalNotes:     []string{"Bundled execution to reduce PR overhead."},
			TargetFiles:        []string{"[MODIFY] <files>"},
			QualityDebt:        []string{"monolithic_file_limit: false"},
		}

		for _, t := range bundledTasks {
			s.AcceptanceCriteria = append(s.AcceptanceCriteria, fmt.Sprintf("- [ ] Complete all criteria for %s", t.Key))
		}

		s.Description += fmt.Sprintf("\n\n**Bundled Tasks:** %s", strings.Join(bundledKeys, ", "))
		s.TechnicalNotes = append(s.TechnicalNotes, "Auto-generated bundle by `nomos task bundle`.")

		body := s.GenerateMarkdown("code")

		title := fmt.Sprintf("Bundle: Related Tasks (%d items)", len(bundledKeys))
		labels := []string{"type:epic", "priority:medium"}
		project := filepath.Base(repoRoot)

		newKey, err := tracker.Create(ctx, title, body, labels, task.Unassigned, project, task.TypeBatch, false, task.StatusBacklog)
		if err != nil {
			return fmt.Errorf("failed to create bundle task: %w", err)
		}

		fmt.Printf("✅ Successfully created bundle task: %s\n", newKey)

		if noStartVal {
			return nil
		}
		// Start the new task
		return taskStartCmd.RunE(cmd, []string{newKey})
	},
}

func init() {
	taskBundleCmd.Flags().Bool("no-start", false, "Create the bundle Epic without transitioning the local workspace")
	taskCmd.AddCommand(taskBundleCmd)
}
