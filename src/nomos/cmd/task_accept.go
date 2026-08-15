package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// taskAcceptCmd transitions a task from TRIAGE to BACKLOG.
var taskAcceptCmd = &cobra.Command{
	Use:   "accept [key]",
	Short: "Accept a TRIAGE task into the BACKLOG",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]

		ctx := context.Background()
		tracker, t, _, err := loadTrackerAndTask(ctx, key)
		if err != nil {
			return err
		}

		if t.Status != "TRIAGE" {
			return fmt.Errorf("task %s is in status %s, not TRIAGE", key, t.Status)
		}

		err = tracker.Transition(ctx, key, "BACKLOG")
		if err != nil {
			return err
		}

		fmt.Printf("✅ Accepted task %s into BACKLOG\n", key)
		return nil
	},
}

func init() {
	taskCmd.AddCommand(taskAcceptCmd)
}
