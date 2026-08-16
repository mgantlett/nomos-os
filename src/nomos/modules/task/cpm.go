// Package task provides models and functions for manipulating Agile tasks.
// The cpm.go file contains the Critical Path Method (CPM) flow calculations
// replacing the old manual Agile story point estimations.
package task

import (
	"context"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/db"
)

// SyncAgentVelocities syncs all closed tasks to a SQLite table and computes rolling averages
// of execution duration (AgentCycles) grouped by Layer Tag, excluding extreme outliers.
func SyncAgentVelocities(ctx context.Context, wCtx *workspace.WorkspaceContext, tasks []Task) error {
	repoRoot := wCtx.RepoRoot
	dbPath := workspace.MustNewContext(repoRoot).DbPath("state.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		return err
	}

	_, err = conn.Exec(`
		CREATE TABLE IF NOT EXISTS agent_velocities (
			task_key TEXT PRIMARY KEY,
			layer TEXT,
			agent_cycles INTEGER
		);
	`)
	if err != nil {
		return err
	}

	// Initialize a database transaction to efficiently insert multiple records.
	// We use INSERT OR REPLACE to keep the database fully up-to-date with
	// the latest agent execution times for the given task keys.
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Prepare the statement for batch inserting.
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO agent_velocities (task_key, layer, agent_cycles) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, t := range tasks {
		if t.Status == StatusDone {
			layer := extractLayer(t.Labels)
			if layer != "" {
				_, err = stmt.Exec(t.Key, layer, t.AgentCycles)
				if err != nil {
					return err
				}
			}
		}
	}
	return tx.Commit()
}

// GetRollingAverages retrieves the average AgentCycles per Layer, excluding outliers (e.g. > 100 cycles).
func GetRollingAverages(ctx *workspace.WorkspaceContext) (map[string]float64, error) {
	repoRoot := ctx.RepoRoot
	dbPath := workspace.MustNewContext(repoRoot).DbPath("state.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		return nil, err
	}

	// Calculate rolling average using a window or simply the average excluding top 5% outliers
	// For simplicity and robustness in SQLite, we exclude AgentCycles > 50 which represent blocked outliers
	rows, err := conn.Query(`
		SELECT layer, AVG(agent_cycles) 
		FROM agent_velocities 
		WHERE agent_cycles <= 50 
		GROUP BY layer
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	averages := make(map[string]float64)
	for rows.Next() {
		var layer string
		var avg float64
		if err := rows.Scan(&layer, &avg); err == nil {
			averages[layer] = avg
		}
	}
	return averages, nil
}

// extractLayer parses the array of labels attached to a task and identifies
// the semantic layer it belongs to (e.g. frontend, backend, telemetry).
// If no layer label is found, it returns 'default'.
func extractLayer(labels []string) string {
	for _, l := range labels {
		if strings.HasPrefix(l, "layer:") {
			return strings.TrimPrefix(l, "layer:")
		}
	}
	return "default"
}

// CalculateCriticalPath computes the longest duration path in a task DAG for an Epic.
// It iterates through the tasks and recursively resolves their BlockedBy dependencies,
// calculating the longest chain of execution using memoization to avoid redundant traversals.
func CalculateCriticalPath(tasks []Task, averages map[string]float64) (float64, []string) {
	// Create lookup maps
	taskMap := make(map[string]Task)
	for _, t := range tasks {
		taskMap[t.Key] = t
	}

	// Memoization for longest path from a node
	memo := make(map[string]float64)
	var getLongestPath func(key string) float64

	getLongestPath = func(key string) float64 {
		if val, exists := memo[key]; exists {
			return val
		}
		t, exists := taskMap[key]
		if !exists {
			return 0
		}
		layer := extractLayer(t.Labels)
		dur, exists := averages[layer]
		if !exists {
			dur = 5.0 // fallback default average cycles
		}

		maxBlockDur := 0.0
		for _, b := range t.BlockedBy {
			d := getLongestPath(b)
			if d > maxBlockDur {
				maxBlockDur = d
			}
		}
		memo[key] = dur + maxBlockDur
		return memo[key]
	}

	maxTotal := 0.0
	for _, t := range tasks {
		dur := getLongestPath(t.Key)
		if dur > maxTotal {
			maxTotal = dur
		}
	}

	return maxTotal, nil
}
