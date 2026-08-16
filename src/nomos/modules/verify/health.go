package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/db"
	"github.com/mgantlett/nomos-commons/src/nomos/core/telemetry"
	"github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/mgantlett/nomos-os/src/nomos/modules/schema"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
)

// HealthStatus encapsulates the results of all health checks.
// It stores boolean indicators of daemon statuses, hook integrity,
// list of cleared database locks, and detailed error messages.
// This structure is often serialized and emitted over websockets to the control plane.
// We keep it extremely flat so the frontend UI does not have to parse nested objects.
type HealthStatus struct {
	Timestamp         string   `json:"timestamp"`
	LlamaAlive        bool     `json:"llama_alive"`
	CockpitAlive      bool     `json:"cockpit_alive"`
	GitHooksHealthy   bool     `json:"git_hooks_healthy"`
	StaleLocksCleared []string `json:"stale_locks_cleared"`
	Failures          []string `json:"failures"`
}

// checkPort resolves TCP connection health for the given network address.
// Returns true if port responds within timeout boundary, false otherwise.
func checkPort(addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// checkGitHooks verifies all configured hook symlinks exist in the Git workspace.
// If missing, it attempts to self-heal hook pointers back to standard targets.
func checkGitHooks(root string, status *HealthStatus) {
	gitDir := filepath.Join(root, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		status.GitHooksHealthy = true
		return
	}

	source := filepath.Join(root, ".nomos-commons", ".agent", "config", "hooks.json")
	if _, err := os.Stat(source); os.IsNotExist(err) {
		source = filepath.Join(root, ".agent", "config", "hooks.json")
		if _, err := os.Stat(source); os.IsNotExist(err) {
			status.GitHooksHealthy = true
			return
		}
	}

	hooks := []string{"pre-commit", "commit-msg", "pre-push"}
	gitHooksHealthy := true
	for _, h := range hooks {
		hPath := filepath.Join(root, ".git", "hooks", h)
		if _, err := os.Stat(hPath); os.IsNotExist(err) {
			gitHooksHealthy = false
			status.Failures = append(status.Failures, fmt.Sprintf("Git hook symlink missing: %s", h))
		}
	}
	status.GitHooksHealthy = gitHooksHealthy

	if !gitHooksHealthy {
		_ = os.MkdirAll(filepath.Join(root, ".git", "hooks"), 0755)
		for _, h := range hooks {
			target := filepath.Join(root, ".git", "hooks", h)
			_ = os.Remove(target)
			_ = os.Symlink(source, target)
		}
	}
}

// checkAndClearDatabaseLocks queries the local cache database locks table.
// If process check identifies dead holding PIDs, it performs transaction cleanup.
func checkAndClearDatabaseLocks(root string, status *HealthStatus) {
	dbPath := workspace.MustNewContext(root).DbPath("cache.db")
	if _, err := os.Stat(dbPath); err != nil {
		return
	}
	db, err := db.Open(dbPath)
	if err != nil {
		return
	}

	rows, err := db.Query("SELECT lock_key, owner, pid FROM locks;")
	if err != nil {
		return
	}
	defer rows.Close()

	var toDelete []struct {
		Key   string
		Owner string
	}
	// Scan through all active locks to identify abandoned processes.
	// We use an early-continue pattern to avoid deep nesting that trips the complexity gates.
	for rows.Next() {
		var lockKey, owner string
		var pid int

		// Skip rows that we fail to parse
		if err := rows.Scan(&lockKey, &owner, &pid); err != nil {
			continue
		}

		// Skip processes that are still actively running
		if exec.IsProcessAlive(pid) {
			continue
		}

		// Process is dead, mark this lock for deletion
		toDelete = append(toDelete, struct {
			Key   string
			Owner string
		}{lockKey, owner})
	}
	rows.Close() // Explicitly close to release the single connection before Exec

	for _, d := range toDelete {
		_, err = db.Exec("DELETE FROM locks WHERE lock_key = ? AND owner = ?;", d.Key, d.Owner)
		if err == nil {
			status.StaleLocksCleared = append(status.StaleLocksCleared, fmt.Sprintf("Cleared stale lock %s owned by process %s", d.Key, d.Owner))
		}
	}
}

// persistFailureHistory records ongoing diagnostics telemetry files.
// When consecutive error events hit threshold limits, it auto-files backlog tickets.
// It also cleans up the failures if the recent check succeeds.
func persistFailureHistory(root string, status *HealthStatus) {
	failuresFile := filepath.Join(workspace.MustNewContext(root).TmpDir(), "health_failures.json")
	var state struct {
		Failures []string `json:"failures"`
		Filed    bool     `json:"filed"`
	}
	if data, err := os.ReadFile(failuresFile); err == nil {
		_ = json.Unmarshal(data, &state)
	}

	if len(status.Failures) > 0 {
		state.Failures = append(state.Failures, status.Timestamp+": "+status.Failures[0])
		if len(state.Failures) >= 3 && !state.Filed {
			_ = logTriageIssue(root, status.Failures, state.Failures)
			state.Filed = true
		}
	} else {
		state.Failures = nil
		state.Filed = false
	}

	failureData, _ := json.Marshal(state)
	_ = os.MkdirAll(workspace.MustNewContext(root).TmpDir(), 0755)
	_ = os.WriteFile(failuresFile, failureData, 0644)
}

// readCachedHealth reads the health status from a JSON file.
// Returns status pointer and true if cache exists and is newer than 2 minutes.
func readCachedHealth(root string) (*HealthStatus, bool) {
	cachePath := filepath.Join(workspace.MustNewContext(root).TmpDir(), ".health_cache.json")
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, false
	}
	var status HealthStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, false
	}

	lastChecked, err := time.Parse(time.RFC3339, status.Timestamp)
	if err != nil || time.Since(lastChecked) >= 2*time.Minute {
		return nil, false
	}
	return &status, true
}

// writeCachedHealth saves the health status to a JSON file.
func writeCachedHealth(root string, status HealthStatus) {
	cachePath := filepath.Join(workspace.MustNewContext(root).TmpDir(), ".health_cache.json")
	_ = os.MkdirAll(filepath.Dir(cachePath), 0755)
	data, _ := json.Marshal(status)
	_ = os.WriteFile(cachePath, data, 0644)
}

// AuditHealth checks ports, DB locks, and git hooks, attempting self-healing.
// It coordinates modular checks and triggers persistence loops.
func AuditHealth(root string) (HealthStatus, error) {
	if cached, ok := readCachedHealth(root); ok {
		return *cached, nil
	}

	status := HealthStatus{
		Timestamp: time.Now().Format(time.RFC3339),
	}

	// Audit LlamaServer inference port health status
	llamaAddr := "127.0.0.1:8082"
	status.LlamaAlive = checkPort(llamaAddr, 1*time.Second)

	// Audit Cockpit observability server health status
	cockpitAddr := "127.0.0.1:8089"
	status.CockpitAlive = checkPort(cockpitAddr, 1*time.Second)

	// Execute git hooks check loop and DB locks self-healing routine
	checkGitHooks(root, &status)
	checkAndClearDatabaseLocks(root, &status)

	// Persist failure logs and check for auto-filing thresholds
	persistFailureHistory(root, &status)

	// Write to the flat file cache
	writeCachedHealth(root, status)

	return status, nil
}
func isActive(status task.TaskStatus) bool {
	return status == task.StatusBacklog || status == task.StatusInProgress
}

// logTriageIssue creates a triage issue in the backlog after persistent health check failures.
// It leverages standard ticketing tracker interfaces to auto-file issues.
func logTriageIssue(root string, activeFailures []string, history []string) error {
	cfg, err := func() (*task.Config, error) { c, _ := workspace.NewContext(root); return task.LoadConfig(c) }()
	if err != nil {
		return err
	}
	tracker, err := task.NewTracker(cfg)
	if err != nil {
		return err
	}

	title := "System Health Alert: Persistent health failures detected in workspace"

	// Deduplicate: Check if an open issue with the same title already exists
	ctx := context.Background()
	existingTasks, err := tracker.List(ctx)
	if err != nil {
		return err
	}

	for _, t := range existingTasks {
		if t.Title == title && isActive(t.Status) {
			synapse.Info("Health check daemon: deduplicating issue, task %s already exists\n", t.Key)
			return nil
		}
	}

	s := &schema.IncidentTriageSchema{
		CurrentFailures: []string{activeFailures[0]},
		FailureHistory:  []string{history[0]},
	}
	body := s.GenerateMarkdown()

	labels := []string{"nomos:system:health", "bug"}
	project := filepath.Base(root)
	_, err = tracker.Create(ctx, title, body, labels, task.Unassigned, project, task.TypeBug, false, task.StatusBacklog)
	if err != nil {
		return err
	}

	_ = telemetry.EmitEvent(root, "system_health_failure", activeFailures[0])
	return nil
}
