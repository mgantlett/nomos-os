package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/spf13/cobra"
)

var (
	// listJSONFlag instructs the command to bypass tabular output and print raw JSON payload instead.
	listJSONFlag bool

	// listSortCLIFlag triggers an ascending sort based on Cognitive Load Index (LOW -> MED -> HIGH).
	listSortCLIFlag bool

	// listTierFlag filters tasks by their designated intelligence tier boundary (1 or 2).
	listTierFlag int

	// listShowClosedFlag overrides the default behavior to hide terminal tasks (DONE/CANCELLED).
	listShowClosedFlag bool
)

// taskListCmd queries the ticketing engine backend, formats the retrieved list of
// tasks, and prints them in tabular layout or returns raw JSON depending on flags.
var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all active tasks and backlog items",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Resolve credentials configuration and construct the tracker object.
		ctx := context.Background()
		_, _, tasks, err := loadTrackerAndListTasks(ctx)
		if err != nil {
			return err
		}

		// Filter out closed/canceled tasks by default unless explicitly requested
		if !listShowClosedFlag {
			var activeTasks []task.Task
			for _, t := range tasks {
				if !t.IsClosed() {
					activeTasks = append(activeTasks, t)
				}
			}
			tasks = activeTasks
		}

		getCliInfo := func(t task.Task) (string, int) {
			score := t.CognitiveLoadIndex()
			if score >= 5 {
				return "HIGH", score
			}
			if score >= 3 {
				return "MED", score
			}
			return "LOW", score
		}

		// getTierStr maps a task to its minimum required intelligence Tier.
		getTierStr := func(t task.Task) string {
			if t.GetIntelligenceTier() == 1 {
				return "T1"
			}
			return "T2"
		}

		// Filter by --tier if specified
		if listTierFlag > 0 {
			var filtered []task.Task
			for _, t := range tasks {
				tTier := 2
				if getTierStr(t) == "T1" {
					tTier = 1
				}
				if tTier == listTierFlag {
					filtered = append(filtered, t)
				}
			}
			tasks = filtered
		}

		// Remove Project filtering since tasks are fully global

		// Sort by CLI if --sort-cli flag is provided
		if listSortCLIFlag {
			sort.Slice(tasks, func(i, j int) bool {
				_, scoreI := getCliInfo(tasks[i])
				_, scoreJ := getCliInfo(tasks[j])
				if scoreI != scoreJ {
					return scoreI < scoreJ // Low LLM demand first -> High LLM demand
				}
				return tasks[i].Key < tasks[j].Key
			})
		}

		// Format and print as raw JSON payload if --json flag is provided.
		if listJSONFlag {
			bytes, err := json.MarshalIndent(tasks, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(bytes))
			return nil
		}

		if len(tasks) == 0 {
			fmt.Println("No active tasks found in the backlog.")
			return nil
		}

		// Organize tasks by parent
		var rootTasks []task.Task
		childrenMap := make(map[string][]task.Task)
		taskKeys := make(map[string]bool)
		for _, t := range tasks {
			taskKeys[t.Key] = true
		}

		for _, t := range tasks {
			if t.ParentKey == "" || !taskKeys[t.ParentKey] {
				rootTasks = append(rootTasks, t)
			} else {
				childrenMap[t.ParentKey] = append(childrenMap[t.ParentKey], t)
			}
		}

		// Format and print a clean developer table dashboard.
		fmt.Printf("\n%-8s  %-16s  %-8s  %-10s  %-5s  %-6s  %-6s  %-18s  %-34s  %-12s\n", "Key", "Project", "Type", "Status", "Tier", "CLI", "Size", "Labels", "Summary", "Assignee")
		fmt.Println(strings.Repeat("-", 136))

		// printTask is a recursive closure to draw tasks as a visual tree hierarchy
		var printTask func(t task.Task, depth int)
		printTask = func(t task.Task, depth int) {
			summary := t.Title
			prefix := strings.Repeat("  ", depth)
			if depth > 0 {
				prefix += "└─ "
			}
			summary = prefix + summary
			if len(summary) > 34 {
				summary = summary[:31] + "..."
			}
			labels := strings.Join(t.Labels, ",")
			if len(labels) > 16 {
				labels = labels[:13] + "..."
			}

			tierStr := getTierStr(t)
			cliStr, _ := getCliInfo(t)

			fmt.Printf("%-8s  %-16s  %-8s  %-10s  %-5s  %-6s  %-6d  %-18s  %-34s  %-12s\n", t.Key, t.Project, t.Type, t.Status, tierStr, cliStr, t.ContextBurden+t.LogicDepth, labels, summary, t.Assignee)

			for _, child := range childrenMap[t.Key] {
				printTask(child, depth+1)
			}
		}

		for _, t := range rootTasks {
			printTask(t, 0)
		}
		fmt.Println()

		return nil
	},
}

func init() {
	taskListCmd.Flags().BoolVar(&listJSONFlag, "json", false, "Output list of tasks in JSON format")
	taskListCmd.Flags().BoolVar(&listSortCLIFlag, "sort-cli", false, "Sort tasks by Cognitive Load Index (LOW -> MED -> HIGH)")
	taskListCmd.Flags().IntVar(&listTierFlag, "tier", 0, "Filter tasks by intelligence tier (1 or 2)")
	taskListCmd.Flags().BoolVarP(&listShowClosedFlag, "show-closed", "c", false, "Include closed and canceled tasks in the output")
	taskCmd.AddCommand(taskListCmd)
}
