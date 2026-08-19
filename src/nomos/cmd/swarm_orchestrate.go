package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/db"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/spf13/cobra"
)

var (
	swarmOrchestrateLimit int
	swarmOrchestrateAgent string
)

func parsePriorityScore(labels []string) int {
	for _, l := range labels {
		lower := strings.ToLower(l)
		if strings.Contains(lower, "critical") {
			return 3
		}
		if strings.Contains(lower, "high") {
			return 2
		}
		if strings.Contains(lower, "medium") {
			return 1
		}
	}
	return 0
}

func getActiveSwarmWorkerCount(repoRoot string) (int, error) {
	dbPath := workspace.MustNewContext(repoRoot).DbPath("cache.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		return 0, err
	}
	var count int
	err = conn.QueryRow("SELECT COUNT(*) FROM active_processes WHERE command LIKE '%ncode%' OR command LIKE '%swarm delegate%'").Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

var swarmOrchestrateCmd = &cobra.Command{
	Use:   "orchestrate",
	Short: "Tier 1 Autonomous Pool: Auto-claim top backlog tasks and spawn Swarm workers",
	Long:  "Scans global tasks folder for tasks in BACKLOG status, sorts by priority, and claims up to --limit tasks.",
	RunE: func(cmd *cobra.Command, args []string) error {
		tracker, repoRoot, err := loadTrackerAndRoot()
		if err != nil {
			return err
		}

		ctx := context.Background()
		tasks, err := tracker.List(ctx)
		if err != nil {
			return fmt.Errorf("failed to list tasks for swarm orchestrate: %w", err)
		}

		var backlogTasks []*task.Task
		for i := range tasks {
			if tasks[i].Status == task.StatusBacklog || tasks[i].Status == task.StatusInProgress {
				hasLowCLI := false
				hasLowBlast := false
				for _, l := range tasks[i].Labels {
					if strings.ToLower(l) == "cli:low" {
						hasLowCLI = true
					}
					if strings.ToLower(l) == "blast:low" {
						hasLowBlast = true
					}
				}

				if hasLowCLI && hasLowBlast {
					backlogTasks = append(backlogTasks, &tasks[i])
				}
			}
		}

		if len(backlogTasks) == 0 {
			fmt.Println("ℹ️  No tasks currently in BACKLOG status. Autonomous pool idle.")
			return nil
		}

		sort.Slice(backlogTasks, func(i, j int) bool {
			pi := parsePriorityScore(backlogTasks[i].Labels)
			pj := parsePriorityScore(backlogTasks[j].Labels)
			if pi != pj {
				return pi > pj
			}
			return backlogTasks[i].CreatedAt.Before(backlogTasks[j].CreatedAt)
		})

		if swarmOrchestrateLimit <= 0 {
			swarmOrchestrateLimit = 2
		}

		activeCount, err := getActiveSwarmWorkerCount(repoRoot)
		if err != nil {
			fmt.Printf("⚠️  Could not determine active worker count: %v\n", err)
			activeCount = 0
		}

		availableSlots := swarmOrchestrateLimit - activeCount
		if availableSlots <= 0 {
			fmt.Printf("⚠️  Swarm worker pool is full (%d active workers, limit %d). Waiting for active workers to complete.\n", activeCount, swarmOrchestrateLimit)
			return nil
		}

		toSpawn := len(backlogTasks)
		if toSpawn > availableSlots {
			toSpawn = availableSlots
		}

		fmt.Printf("🚀 [Tier 1 Autonomous Swarm Pool] Claiming %d backlog task(s) (limit: %d, active: %d)...\n", toSpawn, swarmOrchestrateLimit, activeCount)

		for i := 0; i < toSpawn; i++ {
			t := backlogTasks[i]
			assignee := "swarm:" + swarmOrchestrateAgent
			fmt.Printf("  ▶ Starting Task %s (%s)...\n", t.Key, t.Title)

			_, errStart := task.StartTrackerOnly(ctx, workspace.MustNewContext(repoRoot), tracker, t.Key, assignee)
			if errStart != nil {
				fmt.Printf("  ⚠️  Failed to start task %s: %v\n", t.Key, errStart)
				continue
			}

			// Shell out to nomos swarm delegate asynchronously
			delegateCmd := exec.Command("nomos", "swarm", "delegate", swarmOrchestrateAgent, t.Key)
			delegateCmd.Stdout = os.Stdout
			delegateCmd.Stderr = os.Stderr
			
			// We just start it and let it run in the background
			if errDispatch := delegateCmd.Start(); errDispatch != nil {
				fmt.Printf("  ⚠️  Failed to dispatch Swarm worker for task %s: %v\n", t.Key, errDispatch)
			} else {
				// Record the active process so it counts against the limit
				conn, _ := db.Open(workspace.MustNewContext(repoRoot).DbPath("cache.db"))
				if conn != nil {
					conn.Exec("INSERT INTO active_processes (pid, command) VALUES (?, ?)", delegateCmd.Process.Pid, strings.Join(delegateCmd.Args, " "))
				}
			}
		}

		return nil
	},
}

func init() {
	swarmOrchestrateCmd.Flags().IntVarP(&swarmOrchestrateLimit, "limit", "l", 2, "Maximum concurrent swarm workers")
	swarmOrchestrateCmd.Flags().StringVarP(&swarmOrchestrateAgent, "agent", "a", "ncode", "Swarm agent implementation to dispatch")
	swarmCmd.AddCommand(swarmOrchestrateCmd)
}
