package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// taskStashCmd defines the parent command for git stash management within Nomos tasks.
// This command group allows autonomous AI agents and users to effectively track,
// audit, and prune work-in-progress code that was parked in the git stash.
// Because agents operate in highly constrained contexts, ensuring the stash
// remains hygienic prevents complex state divergence and lost work.
// By mapping stash messages back to canonical task identifiers, Nomos can
// deterministically assert whether a stash is orphaned or actively being worked on.
var taskStashCmd = &cobra.Command{
	Use:   "stash",
	Short: "Manage and audit git stashes linked to tasks",
}

// taskStashAuditCmd defines the subcommand for auditing and cross-referencing stashes.
var taskStashAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Audit git stashes and cross-reference with backlog",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Retrieve all active and closed tasks from the repository database.
		ctx := context.Background()
		_, repoRoot, tasks, err := loadTrackerAndListTasks(ctx, false)
		if err != nil {
			return err
		}

		// Filter out tasks that do not belong to the current project context
		tasks = FilterTasksByProject(tasks, repoRoot)
		// Build a map of task key to closed status for O(1) lookups during audit
		taskMap := make(map[string]bool)
		for _, t := range tasks {
			taskMap[t.Key] = t.IsClosed()
		}

		// Execute the git stash list command to parse raw stashes
		stashCmd := exec.Command("git", "stash", "list")
		stashCmd.Dir = repoRoot
		out, err := stashCmd.Output()
		if err != nil {
			return fmt.Errorf("failed to list stashes: %w", err)
		}

		// Split output into discrete stash entries based on newlines
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
			fmt.Println("No stashes found.")
			return nil
		}

		// Iterate and classify each stash item against the backlog map
		fmt.Println("📊 Git Stash Audit Report:")
		// Validate and unpack output lines safely
		for _, line := range lines {
			// Skip blank lines returned by git stash list output
			if line == "" {
				continue
			}
			// Stash formats are usually 'stash@{N}: On branch...'
			parts := strings.SplitN(line, ": ", 2)
			if len(parts) < 2 {
				// Malformed stash string, ignore it
				continue
			}
			// Extract the stash ID (e.g., stash@{0})
			stashID := parts[0]
			// Extract the rest of the stash message
			msg := parts[1]

			// This variable will hold the task key if a match is successfully identified
			var matchedTask string
			// Iterate through all known tasks to cross-reference with the stash message
			for key := range taskMap {
				// The stash message could contain various patterns inserted by Nomos or the user
				if strings.Contains(msg, "Task "+key) || strings.Contains(msg, "task-"+key) || strings.Contains(msg, "Task: "+key) || strings.Contains(msg, "nomos-park-task-"+key) {
					// We found a matching task in the backlog
					matchedTask = key
					break
				}
			}

			// If a match was found, determine its operational state
			if matchedTask != "" {
				isClosed := taskMap[matchedTask]
				if isClosed {
					// Task is closed, therefore the stash is orphaned and can be safely dropped
					fmt.Printf("- %s: [ORPHANED] Task %s is Closed. Safe to drop. (Msg: %s)\n", stashID, matchedTask, msg)
				} else {
					// Task is still active, stash represents valid work in progress
					fmt.Printf("- %s: [ACTIVE] Task %s is Open. (Msg: %s)\n", stashID, matchedTask, msg)
				}
			} else {
				// No task mapping could be established from the stash message
				fmt.Printf("- %s: [UNMAPPED] No task mapping found. (Msg: %s)\n", stashID, msg)
			}
		}

		// Execution complete
		return nil
	},
}

// init registers the stash commands into the cobra command tree
func init() {
	taskStashCmd.AddCommand(taskStashAuditCmd)
	taskCmd.AddCommand(taskStashCmd)
}
