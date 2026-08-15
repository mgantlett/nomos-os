package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/spf13/cobra"
)

// taskSearchCmd defines the CLI command to perform semantic searches on the backlog.
// It accepts a query string, invokes the semantic embedding API, and prints the top matches.
var taskSearchCmd = &cobra.Command{
	Use:   "search [query...]",
	Short: "Perform a semantic search across the active backlog",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Join all provided arguments into a single continuous query string.
		// This ensures multi-word queries are properly evaluated by the LLM embeddings.
		query := strings.Join(args, " ")

		// Load the backend Tracker which interacts with our JSON datastore
		// and loads the global project roots for semantic context.
		tracker, _, err := loadTrackerAndRoot()
		if err != nil {
			return err
		}

		ctx := context.Background()
		tasks, err := tracker.List(ctx)
		if err != nil {
			return err
		}

		fmt.Printf("🔍 Searching backlog for: %q\n", query)

		// Execute the semantic search logic, which talks to the local LLM embedding daemon
		// on port 8081 to calculate cosine similarities across the workspace backlog.
		results := task.SemanticSearch(query, tasks)

		// Render the CLI headers and table structure for the search results.
		fmt.Printf("\nTop Matches:\n")
		fmt.Println("---------------------------------------------------------------------------------------------")
		fmt.Printf("%-10s | %-15s | %-50s | %s\n", "KEY", "PROJECT", "TITLE", "SCORE")
		fmt.Println("---------------------------------------------------------------------------------------------")

		// Cap the output to the top 5 most relevant results to avoid polluting
		// the terminal window with irrelevant long-tail backlog items.
		limit := 5
		if len(results) < limit {
			limit = len(results)
		}

		for i := 0; i < limit; i++ {
			r := results[i]
			title := r.Task.Title
			if len(title) > 47 {
				title = title[:44] + "..."
			}
			fmt.Printf("%-10s | %-15s | %-50s | %.4f\n", r.Task.Key, r.Task.Project, title, r.Score)
		}

		return nil
	},
}

func init() {
	taskCmd.AddCommand(taskSearchCmd)
}
