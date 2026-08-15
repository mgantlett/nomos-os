package task

import (
	"context"
	"fmt"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

// GroomBacklog orchestrates the backlog grooming process, detecting dependency cycles,
// flagging duplicate tasks using cosine similarity, and automatically bundling them.
func GroomBacklog(ctx context.Context, wCtx *workspace.WorkspaceContext, tracker Tracker, capacity int, projectFilter string, autoApprove bool) error {
	allTasks, err := tracker.List(ctx)
	if err != nil {
		return err
	}

	tasks := filterTasks(allTasks, projectFilter)

	cycles := detectCycles(tasks)
	for _, cycle := range cycles {
		fmt.Printf("⚠️  Cyclic Dependency Detected! Path: %s\n", strings.Join(cycle, " -> "))
	}

	duplicates := detectDuplicates(tasks)
	for _, dup := range duplicates {
		fmt.Printf("⚠️  Possible Duplicate Tasks Detected! %s and %s have a high cosine similarity.\n", dup[0], dup[1])
	}

	// Monolithic task detection delegates the cognitive splitting to the Orchestrator LLM
	detectAndTagMonolithicTasks(ctx, tracker, tasks)

	if !autoApprove {
		fmt.Print("\nDo you want to proceed with backlog auto-bundling and pruning? (y/N): ")
		var resp string
		fmt.Scanln(&resp)
		if strings.ToLower(strings.TrimSpace(resp)) != "y" {
			fmt.Println("Grooming aborted by Product Owner.")
			return nil
		}
	}

	// Sweep TRIAGE debt items first
	// This ensures that all quality debt items are dynamically grouped together
	// by their module boundaries and consolidated into actionable Epics within
	// the standard BACKLOG before we process regular backlog items.
	err = AutoBundleTasksWith(tasks, capacity, StatusTriage)
	if err != nil {
		fmt.Printf("⚠️ Error bundling TRIAGE tasks: %v\n", err)
	}

	// Then sweep BACKLOG
	// After the Triage scope is clear, we perform the standard feature
	// bundling for regular unassigned backlog stories.
	err = AutoBundleTasksWith(tasks, capacity, StatusBacklog)
	if err != nil {
		return err
	}

	return nil
}

// detectCycles performs a Depth First Search (DFS) on the task dependency graph
// to identify any cyclic dependencies between tasks (e.g. Task A blocks Task B blocks Task A).
// Returns a list of identified cycles represented as arrays of task keys.
func detectCycles(tasks []Task) [][]string {
	var cycles [][]string
	adj := make(map[string][]string)
	for _, t := range tasks {
		adj[t.Key] = t.BlockedBy
	}

	visited := make(map[string]bool)
	visiting := make(map[string]bool)

	var dfs func(node string, path []string)
	dfs = func(node string, path []string) {
		if visiting[node] {
			cyclePath := make([]string, len(path))
			copy(cyclePath, path)
			cyclePath = append(cyclePath, node)
			cycles = append(cycles, cyclePath)
			return
		}
		if visited[node] {
			return
		}
		visiting[node] = true
		path = append(path, node)

		for _, neighbor := range adj[node] {
			dfs(neighbor, path)
		}

		visiting[node] = false
		visited[node] = true
	}

	for _, t := range tasks {
		if !visited[t.Key] {
			dfs(t.Key, []string{})
		}
	}
	return cycles
}

// filterTasks iterates over all raw tracker tasks and applies the given project
// filter. If the projectFilter is empty, it returns all tasks.
// This is necessary because global backlog queries might fetch across worktrees.
func filterTasks(allTasks []Task, projectFilter string) []Task {
	var tasks []Task
	for _, t := range allTasks {
		if t.IsClosed() {
			continue
		}
		if projectFilter == "" || t.Project == projectFilter {
			tasks = append(tasks, t)
		}
	}
	return tasks
}

// detectAndTagMonolithicTasks iterates through the tasks and calculates their
// total size (ContextBurden + LogicDepth). If a task exceeds the micro-task
// threshold of 5, it prints an Orchestrator Directive so the LLM can
// semantically split it. It also automatically tags the task with 'needs-split'
// to ensure it is not incorrectly bundled before the split occurs.
func detectAndTagMonolithicTasks(ctx context.Context, tracker Tracker, tasks []Task) {
	for _, t := range tasks {
		if t.ContextBurden+t.LogicDepth > 5 {
			fmt.Printf("⚠️  [ORCHESTRATOR DIRECTIVE] Task %s is monolithic (Size: %d). You must invoke cognitive splitting. Read the task and spawn micro-tasks.\n", t.Key, t.ContextBurden+t.LogicDepth)

			hasNeedsSplit := false
			for _, l := range t.Labels {
				if l == "needs-split" {
					hasNeedsSplit = true
					break
				}
			}
			if !hasNeedsSplit {
				_ = tracker.Edit(ctx, t.Key, nil, nil, append(t.Labels, "needs-split"), nil, nil, nil, nil, nil)
			}
		}
	}
}
