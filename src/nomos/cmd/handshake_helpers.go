// Package cmd implements the command line interface subcommands for Nomos.
// This specific file contains helper functions extracted from the original
// monolithic handshake.go implementation to resolve high cyclomatic complexity.
//
// The functions herein handle:
// - Migrating legacy agent configuration directories safely.
// - Checking the active workspace git branch.
// - Identifying modified/dirty files.
// - Discovering active lock claims from the SQLite subsystem.
// - Fetching gitbrain architectural memories.
// - Determining the active developer task key.
//
// By breaking these out into standalone functions, the main RunE
// operation is greatly simplified and easier to audit for Definition
// of Done verification loops.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/db"

	nomosexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
)

// migrateLegacyDirectories ensures that any legacy agent directories
// are gracefully moved to their new locations under the .nomos state directory.
func migrateLegacyDirectories(repoRoot string) {
	globalDir := config.GlobalDataDir(repoRoot)
	_ = os.MkdirAll(globalDir, 0755)

	dirsToMove := []string{"state", "tmp", "walkthroughs", "tasks", "locks"}
	for _, dir := range dirsToMove {
		legacyPath := filepath.Join(repoRoot, ".nomos", dir)
		newPath := filepath.Join(globalDir, dir)
		if _, err := os.Stat(legacyPath); err == nil {
			_ = os.Rename(legacyPath, newPath)
		}
	}

	// Remove the legacy .nomos directory completely if it exists
	_ = os.RemoveAll(filepath.Join(repoRoot, ".nomos"))
}

// getBranch retrieves the current active git branch using the git CLI.
// It returns 'unknown' if the command fails.
func getBranch(repoRoot string) string {
	branch := "unknown"
	branchCmd := exec.Command("git", "branch", "--show-current")
	branchCmd.Dir = repoRoot
	if branchBytes, err := branchCmd.Output(); err == nil {
		branch = strings.TrimSpace(string(branchBytes))
	}
	return branch
}

// getDirtyFiles returns a list of modified files in the workspace
// by parsing the output of git status --porcelain.
func getDirtyFiles(repoRoot string) []string {
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = repoRoot
	var dirtyFiles []string
	if statusBytes, err := statusCmd.Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(statusBytes)), "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				dirtyFiles = append(dirtyFiles, trimmed)
			}
		}
	}
	return dirtyFiles
}

// getClaims inspects the SQLite locks table and local file locks
// to determine which autonomous processes currently hold mutation rights.
func getClaims(repoRoot string) []string {
	claims := []string{}
	// Open sqlite connection using the cached database file path.
	db, err := db.Open(cacheDbPath)
	if err == nil {
		// Fetch all claims registered in the locks table.
		rows, err := db.Query("SELECT lock_key, owner, pid, acquired_at FROM locks;")
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var lockKey, owner, acquiredAt string
				var pid int
				// Scan table row values.
				if err := rows.Scan(&lockKey, &owner, &pid, &acquiredAt); err == nil {
					// Filter to include only currently alive process IDs.
					if nomosexec.IsProcessAlive(pid) {
						claims = append(claims, fmt.Sprintf("%s (claimed by %s, PID %d, acquired at %s)", lockKey, owner, pid, acquiredAt))
					}
				}
			}
		}
	}

	// Inspect local file locks in .nomos/locks/ folder for multi-environment safety.
	locksDir := filepath.Join(config.GlobalDataDir(repoRoot), "locks")
	if files, err := os.ReadDir(locksDir); err == nil {
		for _, f := range files {
			if !f.IsDir() {
				claims = append(claims, fmt.Sprintf("File lock: %s", f.Name()))
			}
		}
	}
	return claims
}

// getMemories executes the gitbrain CLI to fetch semantic memories if available.
func getMemories(repoRoot string, taskDesc string) ([]MemoryInsight, []string) {
	_, err := exec.LookPath("nomos-gitbrain")
	if err != nil {
		return nil, []string{"Semantic memory disabled: nomos-gitbrain enterprise module not installed."}
	}

	gbCmd := exec.Command("nomos-gitbrain", "search", taskDesc)
	out, err := gbCmd.Output()
	if err != nil {
		return nil, []string{"Semantic memory failed: " + err.Error()}
	}

	var results struct {
		Notes []struct {
			Hash    string  `json:"hash"`
			Content string  `json:"content"`
			Score   float64 `json:"score"`
		} `json:"notes"`
	}

	if err := json.Unmarshal(out, &results); err != nil {
		return nil, []string{"Semantic memory parse failed: " + err.Error()}
	}

	var insights []MemoryInsight
	for _, n := range results.Notes {
		insights = append(insights, MemoryInsight{
			Insight:    n.Content,
			CommitHash: n.Hash,
		})
	}

	if len(insights) > 0 {
		return insights, []string{"Loaded semantic memory insights from GitBrain."}
	}
	return nil, nil
}

// getActiveTaskKey reads the current phase state document to
// identify the active task ID the workspace is bound to.
func getActiveTaskKey(repoRoot string) string {
	phaseStatePath := config.PhaseStatePath(repoRoot)
	if stateBytes, err := os.ReadFile(phaseStatePath); err == nil {
		var state map[string]interface{}
		if err := json.Unmarshal(stateBytes, &state); err == nil {
			if tId, ok := state["task_id"].(string); ok && tId != "" {
				return tId
			}
		}
	}
	return ""
}

// populateTrackerTasks loads the tracker and populates tasks in the payload
func populateTrackerTasks(ctx context.Context, payload *HandshakePayload, repoRoot string) {
	tracker, _, errTracker := loadTrackerAndRoot()
	if errTracker != nil {
		payload.Errors = append(payload.Errors, fmt.Sprintf("unable to load tracker: %v", errTracker))
		return
	}

	tasks, err := tracker.List(ctx)
	if err != nil {
		payload.Errors = append(payload.Errors, fmt.Sprintf("unable to retrieve tasks: %v", err))
	} else {
		payload.OpenTasks = FilterTasksByProject(tasks, repoRoot)
	}

	payload.ActiveTaskKey = getActiveTaskKey(repoRoot)
	if payload.ActiveTaskKey != "" {
		if t, errView := tracker.View(ctx, payload.ActiveTaskKey); errView == nil {
			payload.ActiveTaskName = t.Title
		}
	}
}

// createHandshakePayload initializes the default payload structure
func createHandshakePayload(repoRoot string) HandshakePayload {
	return HandshakePayload{
		Timestamp: time.Now().Format("2006-01-02 15:04:05 MST"),
		Errors:    []string{},
		SuggestedActions: []string{
			"nomos help        : Display the primary Nomos engine capabilities",
			"nomos task list   : View your active workspace backlog and sprint items",
			"nomos search      : Search the GitBrain and workspace for context",
			"/nomos-start      : Workflow to initiate a sprint task and claim it",
			"/nomos-verify     : Workflow to validate the Definition of Done",
		},
	}
}
