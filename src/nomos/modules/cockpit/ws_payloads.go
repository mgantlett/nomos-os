package cockpit

import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
)

// getLogFilename determines the correct local log file name based on the specified source component.
func getLogFilename(source string) string {
	if source == "telemetry" || source == "os" {
		return "telemetry.jsonl"
	}
	if source == "substrate" || source == "server" || source == "control-plane" || source == "all" || source == "" {
		return "nomos.jsonl"
	}
	if strings.HasPrefix(source, "swarm-") || source == "worker" {
		return "nomos.jsonl"
	}
	// fallback for exact task IDs e.g., "301" -> "worker_301.log"
	return fmt.Sprintf("worker_%s.log", source)
}

// getBacklogPayload retrieves active backlog tasks for the given repository root.
func getBacklogPayload(ctx *workspace.WorkspaceContext) []map[string]interface{} {
	repoRoot := ctx.RepoRoot
	cfg := &task.Config{TrackerType: "local", RepoRoot: repoRoot}
	tr, err := task.NewTracker(cfg)
	if err != nil {
		return []map[string]interface{}{}
	}

	tasks, err := tr.ListAll(context.Background())
	if err != nil {
		tasks, _ = tr.List(context.Background())
	}

	tasks = filterTasksForProject(tasks, filepath.Base(repoRoot))

	var taskMaps []map[string]interface{}
	bytes, _ := json.Marshal(tasks)
	_ = json.Unmarshal(bytes, &taskMaps)

	return enrichTaskMaps(taskMaps)
}

func GetFallbackPhaseState() map[string]interface{} {
	return map[string]interface{}{
		"agent":              "unknown",
		"agent_tier":         "low",
		"agent_type":         "unknown",
		"commit_approved":    "false",
		"current_phase":      "IDLE",
		"dod_failure_count":  0,
		"phase_entered_at":   time.Now().Format(time.RFC3339),
		"plan_approved":      "false",
		"prev_phase":         "IDLE",
		"session_commits":    0,
		"session_started_at": time.Now().Format(time.RFC3339),
		"task_id":            "",
		"waiting_on_human":   "false",
		"compact_context":    false,
		"phase_token":        "",
	}
}

// getStatusPayload returns the real-time system status.
func getStatusPayload(ctx *workspace.WorkspaceContext, initialRoot string) interface{} {
	repoRoot := ctx.RepoRoot
	pState, err := task.GetPhaseState(ctx)
	var phaseState interface{}
	if err == nil && pState != nil {
		phaseState = pState
	} else {
		phaseState = GetFallbackPhaseState()
	}

	dbPath := workspace.MustNewContext(repoRoot).DbPath("cache.db")
	cacheActive := false
	if _, err := os.Stat(dbPath); err == nil {
		cacheActive = true
	}

	return map[string]interface{}{
		"status":        "ok",
		"repoRoot":      repoRoot,
		"project":       filepath.Base(repoRoot),
		"workspaceName": "OPEN",
		"phaseState":    phaseState,
		"version":       getCockpitVersion(),
		"buildTime":     "dev",
		"cache": map[string]interface{}{
			"active": cacheActive,
			"path":   dbPath,
		},
	}
}

// getDriftPayload returns the configuration drift telemetry.
func getDriftPayload(ctx *workspace.WorkspaceContext) (interface{}, error) {

	return map[string]interface{}{
		"drift":  []interface{}{},
		"status": "disabled",
		"tier":   "sovereign",
	}, nil
}

// getGraphPayload returns the AST dependency graph topology.
func getGraphPayload(ctx *workspace.WorkspaceContext) (interface{}, error) {

	return map[string]interface{}{
		"nodes":  []interface{}{},
		"edges":  []interface{}{},
		"status": "disabled",
		"tier":   "sovereign",
	}, nil
}
