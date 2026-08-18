package task

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// AutoBundleTasks scans the backlog and dynamically bundles related tasks
// into Epics up to the provided capacity cap to reduce context exhaustion.
func AutoBundleTasks(ctx context.Context, tracker *LocalTracker, capacity int) error {
	tasks, err := tracker.List(ctx)
	if err != nil {
		return err
	}
	return AutoBundleTasksWith(tasks, capacity, StatusBacklog)
}

// AutoBundleTasksWith groups a provided slice of tasks by status and candidate rules
// into consolidated Epics up to the specified capacity limit to reduce context overhead.
func AutoBundleTasksWith(tasks []Task, capacity int, targetStatus TaskStatus) error {

	candidates := filterCandidatesByStatus(tasks, targetStatus)
	typeGroups := groupCandidatesByType(candidates)

	for _, groupTasks := range typeGroups {
		err := bundleGroup(groupTasks, capacity)
		if err != nil {
			fmt.Printf("⚠️ Error bundling group: %v\n", err)
		}
	}

	return nil
}

func extractBundledKeysFromDescription(desc string, bundledKeys map[string]bool) {
	idx := strings.Index(desc, "**Bundled Tasks:**")
	if idx != -1 {
		line := desc[idx:]
		endIdx := strings.Index(line, "\n")
		if endIdx != -1 {
			line = line[:endIdx]
		}
		parts := strings.Split(strings.TrimPrefix(line, "**Bundled Tasks:**"), ",")
		for _, p := range parts {
			bundledKeys[strings.TrimSpace(p)] = true
		}
	}
}

// getBundledKeys extracts all task keys that have already been bundled into an Epic.
// It parses the Description field of Batch tasks for the "**Bundled Tasks:**" marker.
func getBundledKeys(tasks []Task) map[string]bool {
	bundledKeys := make(map[string]bool)
	for _, t := range tasks {
		if strings.EqualFold(string(t.Type), string(TypeBatch)) {
			extractBundledKeysFromDescription(t.Description, bundledKeys)
		}
	}
	return bundledKeys
}

// filterCandidatesByStatus scans the raw backlog tasks and extracts only those that are OPEN
// or BACKLOG (or targetStatus), ensuring they are not already sub-tasks of an existing Epic.
// This prevents tasks from being double-bundled or pulled when they shouldn't be.
func filterCandidatesByStatus(tasks []Task, targetStatus TaskStatus) []Task {
	bundledKeys := getBundledKeys(tasks)

	var candidates []Task
	for _, t := range tasks {
		hasNeedsSplit := false
		for _, l := range t.Labels {
			if l == "needs-split" {
				hasNeedsSplit = true
				break
			}
		}

		if t.Status == targetStatus && t.ParentKey == "" && t.Type != TypeBatch && !bundledKeys[t.Key] && !hasNeedsSplit {
			candidates = append(candidates, t)
		}
	}
	return candidates
}

// groupCandidatesByType guarantees Type Segregation for bundles.
// It maps the extracted candidates into buckets so that features are not bundled with bugs.
func groupCandidatesByType(candidates []Task) map[string][]Task {
	typeGroups := make(map[string][]Task)
	for _, t := range candidates {
		typ := strings.ToLower(string(t.Type))
		if typ == "" {
			typ = "feature"
		}
		key := t.Project + ":" + typ
		typeGroups[key] = append(typeGroups[key], t)
	}
	return typeGroups
}

// bundleGroup manages the end-to-end bundling flow for a single task type.
// It builds a token map, extracts connected graph clusters, and executes the cap partitioning.
func bundleGroup(tasks []Task, capacity int) error {
	tokensMap := buildTokensMap(tasks)
	clusters := findClusters(tasks, tokensMap)
	partitionAndExecuteClusters(clusters, capacity)
	return nil
}

// buildTokensMap pre-calculates the semantic tokens (files, packages, layers)
// for each individual task to optimize the connected components grouping algorithm.
func buildTokensMap(tasks []Task) map[string]map[string]bool {
	tokensMap := make(map[string]map[string]bool)
	for _, t := range tasks {
		tokensMap[t.Key] = extractSemanticTokens(t)
	}
	return tokensMap
}

func enqueueNeighbors(curr Task, tasks []Task, tokensMap map[string]map[string]bool, visited map[string]bool, queue *[]Task) {
	for _, other := range tasks {
		if !visited[other.Key] && haveOverlap(tokensMap[curr.Key], tokensMap[other.Key]) {
			visited[other.Key] = true
			*queue = append(*queue, other)
		}
	}
}

func buildCluster(startTask Task, tasks []Task, tokensMap map[string]map[string]bool, visited map[string]bool) []Task {
	var cluster []Task
	queue := []Task{startTask}
	visited[startTask.Key] = true

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		cluster = append(cluster, curr)
		enqueueNeighbors(curr, tasks, tokensMap, visited, &queue)
	}
	return cluster
}

// findClusters performs a standard BFS connected components algorithm.
// It groups tasks that share at least one overlapping semantic token.
func findClusters(tasks []Task, tokensMap map[string]map[string]bool) [][]Task {
	var clusters [][]Task
	visited := make(map[string]bool)

	for _, t := range tasks {
		if visited[t.Key] {
			continue
		}
		cluster := buildCluster(t, tasks, tokensMap, visited)
		if len(cluster) > 1 {
			clusters = append(clusters, cluster)
		}
	}
	return clusters
}

// partitionAndExecuteClusters iterates over each distinct semantic cluster of tasks.
// For each cluster, it delegates to partitionSingleCluster.
func partitionAndExecuteClusters(clusters [][]Task, capacity int) {
	for _, cluster := range clusters {
		partitionSingleCluster(cluster, capacity)
	}
}

// partitionSingleCluster processes a single cluster of semantically related tasks,
// ensuring no individual bundle exceeds the absolute JIT max points cap.
// If the cap is exceeded, it splits the cluster into multiple safe bundles.
func partitionSingleCluster(cluster []Task, capacity int) {
	var currentBundle []Task
	var currentCapacity int

	for _, t := range cluster {
		pts := t.ContextBurden + t.LogicDepth
		if pts <= 0 {
			pts = 2 // Assume 2 size for unestimated tasks
		}
		// If adding this task exceeds the limit, flush the current bundle
		if currentCapacity+pts > capacity {
			executeCurrentBundle(currentBundle)
			// Start a fresh bundle with the current task
			currentBundle = []Task{t}
			currentCapacity = pts
		} else {
			// Append the task to the active bundle
			currentBundle = append(currentBundle, t)
			currentCapacity += pts
		}
	}

	// Flush whatever remains at the end
	executeCurrentBundle(currentBundle)
}

// executeCurrentBundle acts as a wrapper to execute a given task array
// only if there are enough tasks to justify bundling (more than 1).
func executeCurrentBundle(bundle []Task) {
	if len(bundle) > 1 {
		if err := executeBundle(bundle); err != nil {
			fmt.Printf("Failed to bundle tasks: %v\n", err)
		}
	}
}

func executeBundle(bundle []Task) error {
	var keys []string
	for _, t := range bundle {
		keys = append(keys, t.Key)
	}

	fmt.Printf("🔧 Auto-bundling %d tasks: %s\n", len(keys), strings.Join(keys, ", "))

	args := []string{"task", "bundle", "--no-start"}
	args = append(args, keys...)

	cmd := exec.Command("nomos", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("bundle command failed: %s - output: %s", err, string(out))
	}
	fmt.Printf("✅ Bundled successfully:\n%s\n", string(out))
	return nil
}

func extractSemanticTokens(t Task) map[string]bool {
	tokens := make(map[string]bool)

	// Add labels
	for _, l := range t.Labels {
		if strings.HasPrefix(l, "layer:") || strings.HasPrefix(l, "package:") || strings.HasPrefix(l, "file:") {
			tokens[l] = true
		}
	}

	// Regex to extract file paths ending in .go, .md, .json etc
	fileRx := regexp.MustCompile(`[a-zA-Z0-9_\-\./\\]+\.(go|md|json|ts|js|yml|yaml)`)

	matches := fileRx.FindAllString(t.Title+" "+t.Description, -1)
	for _, m := range matches {
		tokens[m] = true
	}

	return tokens
}

func haveOverlap(a, b map[string]bool) bool {
	for k := range a {
		if b[k] {
			return true
		}
	}
	return false
}
