// Package cmd defines the command-line interface for the nomos binary.
// This file implements the 'nomos task edit' command which provides
// the primary mutation mechanism for modifying tracking payloads.
package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/spf13/cobra"
)

// taskEditCmd allows modifying existing backlog tasks.
// It parses the given task key and selectively updates fields such as:
// - Title
// - Markdown body content
// - Context burden and logic depth
// - Associated labels
// It relies on the active task.Tracker implementation to persist changes.
var taskEditCmd = &cobra.Command{
	Use:   "edit [task-key]",
	Short: "Edit metadata and body of an existing task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		titleVal, _ := cmd.Flags().GetString("title")
		fileVal, _ := cmd.Flags().GetString("file")
		labelVal, _ := cmd.Flags().GetString("label")
		blockedByVal, _ := cmd.Flags().GetString("blocked-by")
		sequenceVal, _ := cmd.Flags().GetInt("sequence")

		var titlePtr *string
		if cmd.Flags().Changed("title") {
			titlePtr = &titleVal
		}

		var bodyPtr *string
		if cmd.Flags().Changed("file") {
			data, err := os.ReadFile(fileVal)
			if err != nil {
				return fmt.Errorf("failed to read description file: %w", err)
			}
			bodyStr := string(data)
			bodyPtr = &bodyStr
		}

		var labels []string
		if cmd.Flags().Changed("label") {
			if labelVal != "" {
				parts := strings.Split(labelVal, ",")
				for _, p := range parts {
					labels = append(labels, strings.TrimSpace(p))
				}
			} else {
				labels = []string{} // Explicitly empty to clear labels if needed
			}
		} else {
			labels = nil // Explicitly nil to mean no-change
		}

		var blockedBy []string
		if cmd.Flags().Changed("blocked-by") {
			if blockedByVal != "" {
				parts := strings.Split(blockedByVal, ",")
				for _, p := range parts {
					blockedBy = append(blockedBy, strings.TrimSpace(p))
				}
			} else {
				blockedBy = []string{}
			}
		} else {
			blockedBy = nil
		}

		var sequencePtr *int
		if cmd.Flags().Changed("sequence") {
			sequencePtr = &sequenceVal
		}

		// Load the configured task tracker and locate the repository root.
		// This establishes the persistence backend needed to view or edit tasks.
		tracker, repoRoot, err := loadTrackerAndRoot()
		if err != nil {
			return err
		}

		// Enforce Tier 2 atomic rigidity logic.
		// Low-tier execution swarm agents are explicitly prevented from modifying
		// tasks outside their active lock to prevent rogue scope changes.
		if pState, err := task.GetPhaseState(repoRoot); err == nil && pState.AgentTier == statepkg.Tier2 {
			if key != pState.TaskId {
				return fmt.Errorf("Tier 2 atomic rigidity violation: agents cannot mutate tasks outside their active lock (locked to %s, tried to edit %s)", pState.TaskId, key)
			}
		}

		burdenVal, _ := cmd.Flags().GetInt("burden")
		depthVal, _ := cmd.Flags().GetInt("depth")

		var burdenPtr, depthPtr *int
		if cmd.Flags().Changed("burden") {
			burdenPtr = &burdenVal
		}
		if cmd.Flags().Changed("depth") {
			depthPtr = &depthVal
		}

		var projectPtr *string
		if cmd.Flags().Changed("project") {
			projectVal, _ := cmd.Flags().GetString("project")
			projectPtr = &projectVal
		}

		ctx := context.Background()
		if err := tracker.Edit(ctx, key, titlePtr, bodyPtr, labels, burdenPtr, depthPtr, blockedBy, sequencePtr, projectPtr); err != nil {
			return err
		}

		fmt.Printf("Successfully updated task %s\n", key)
		return nil
	},
}

// init registers the edit command and configures all supported CLI flags
// including title, body, burden, depth, labels, and the dependency sequence flags.
func init() {
	taskEditCmd.Flags().String("title", "", "New title for the task")
	taskEditCmd.Flags().StringP("file", "F", "", "Path to markdown file containing description/body")
	taskEditCmd.Flags().Int("burden", 0, "Context Burden estimation")
	taskEditCmd.Flags().Int("depth", 0, "Logic Depth estimation")
	taskEditCmd.Flags().String("label", "", "Comma-separated list of labels to set")
	taskEditCmd.Flags().String("blocked-by", "", "Comma-separated list of task keys that block this task")
	taskEditCmd.Flags().Int("sequence", -1, "Execution sequence order")
	taskEditCmd.Flags().String("project", "", "Override the project assignment")
	taskCmd.AddCommand(taskEditCmd)
}
