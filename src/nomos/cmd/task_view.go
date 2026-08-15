package cmd

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
)

// taskViewCmd displays detailed information about a specific task,
// including its title, status, complexity size, assignee, full description,
// and any logged discussion comments from the task tracker backend.
var taskViewCmd = &cobra.Command{
	Use:   "view [task-key]",
	Short: "View details of a specific task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		// Load tracking engine instance and repository root directory path.
		tracker, _, err := loadTrackerAndRoot()
		if err != nil {
			return err
		}

		ctx := context.Background()
		// Query single task record by issue key from tracking engine store.
		t, err := tracker.View(ctx, key)
		if err != nil {
			return err
		}

		// Format and render task metadata headers to standard terminal output stream.
		// We explicitly print out all fields from the Tracker to ensure full visibility,
		// including newly added structural elements like BlockedBy and Sequence boundaries.
		fmt.Printf("\nTask:        %s\n", t.Key)
		fmt.Printf("Summary:     %s\n", t.Title)
		fmt.Printf("Project:     %s\n", t.Project)
		fmt.Printf("Status:      %s\n", t.Status)
		fmt.Printf("Labels:      %s\n", t.Labels)
		fmt.Printf("Size:        %d\n", t.ContextBurden+t.LogicDepth)
		fmt.Printf("Assignee:    %s\n", t.Assignee)
		fmt.Printf("Sequence:    %d\n", t.Sequence)
		fmt.Printf("Blocked By:  %v\n", t.BlockedBy)
		fmt.Printf("Is Spike:    %t\n", t.IsSpike)
		fmt.Printf("Link:        %s\n", "")
		fmt.Printf("\nDescription:\n%s\n", t.Description)

		// Render discussion comments in formatted text blocks if available.
		if len(t.Comments) > 0 {
			fmt.Printf("\nComments:\n")
			for _, c := range t.Comments {
				fmt.Printf("----------------------------------------\n%s\n", c)
			}
			fmt.Printf("----------------------------------------\n\n")
		} else {
			fmt.Println()
		}

		return nil
	},
}

func init() {
	taskCmd.AddCommand(taskViewCmd)
}
