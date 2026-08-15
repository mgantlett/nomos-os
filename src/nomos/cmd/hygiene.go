package cmd

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/mgantlett/nomos-os/src/nomos/modules/verify"

	"github.com/mgantlett/nomos-commons/src/nomos/core/db"
)

// RunHygieneCleanups executes central workspace maintenance activities,
// delegating to specialized helper functions to keep cyclomatic complexity low.
// This orchestration ensures that standard databases are vacuumed, stale worktrees
// are gracefully pruned along with their associated data folders, active processes
// are monitored, and transient tracking states are cleaned up.
// It is critical to run this daily to prevent the <repoRoot>/.nomos/data/ folder from becoming bloated.
func RunHygieneCleanups(repoRoot string) error {
	synapse.Info("%s", fmt.Sprint("🧹 Starting Nomos workspace hygiene cleanup..."))

	// Resolve the list of standard databases.
	dbFiles := []string{
		config.ResolveCacheDbPath(repoRoot),
		config.ResolveGitBrainDbPath(repoRoot),
	}

	// 1. SQLite Database Vacuum.
	vacuumDatabases(dbFiles)

	// Resolve active task tracker config.
	cfg, err := task.LoadConfig(repoRoot)
	var tracker task.Tracker
	if err == nil {
		tracker, _ = task.NewTracker(cfg)
	}

	// 2. Worktree Cleanup.
	cleanupWorktrees(repoRoot, tracker)

	// 3. Process Hygiene.
	hygieneProcesses(dbFiles)

	// 4. Expired Quality Debt Check.
	checkExpiredQualityDebt(repoRoot)

	// 5. Temp File Pruning.
	pruneTempFiles(repoRoot, tracker)

	// 6. Transient State File Pruning.
	pruneStateFiles(repoRoot)

	// 7. Refactoring Stories Pruning.
	pruneRefactorStories(repoRoot)

	synapse.Info("%s", fmt.Sprint("✅ Nomos workspace hygiene cleanup complete!"))
	return nil
}

// vacuumDatabases executes SQLite VACUUM and ANALYZE operations.
// This defragments the database pages on disk, reducing file size and improving
// query planning. It operates sequentially over the provided array of database
// paths, gracefully skipping any databases that do not exist in the filesystem.
// It also ensures the database is operating in Write-Ahead Log (WAL) mode for
// enhanced concurrency during active developer sessions.
func vacuumDatabases(dbFiles []string) {
	for _, dbPath := range dbFiles {
		if fi, err := os.Stat(dbPath); err == nil && !fi.IsDir() {
			db, err := db.Open(dbPath)
			if err == nil {
				_, _ = db.Exec("PRAGMA journal_mode=WAL;")
				_, _ = db.Exec("VACUUM;")
				_, _ = db.Exec("ANALYZE;")
				synapse.Info("   ↳ Vacuumed database: %s\n", filepath.Base(dbPath))
			}
		}
	}
}

// cleanupWorktrees audits the local worktrees directory and prunes closed tasks.
func cleanupWorktrees(repoRoot string, tracker task.Tracker) {
	if tracker == nil {
		return
	}
	worktreesDir := config.WorktreesDir(repoRoot)
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			wtPath := filepath.Join(worktreesDir, entry.Name())

			// Extract taskID from .nomos_parent_task instead of parsing the directory name
			taskIDBytes, err := os.ReadFile(filepath.Join(wtPath, ".nomos_parent_task"))
			if err != nil {
				continue // Skip if not a valid nomos worktree or parent task cannot be read
			}
			taskID := strings.TrimSpace(string(taskIDBytes))

			if taskID != "" && isTaskClosed(tracker, taskID) {
				synapse.Info("   ↳ Pruning stale worktree for task %s at: %s\n", taskID, wtPath)

				// Run git commands.
				cmdRemove := exec.Command("git", "worktree", "remove", "-f", wtPath)
				cmdRemove.Dir = repoRoot
				_ = cmdRemove.Run()

				cmdPrune := exec.Command("git", "worktree", "prune")
				cmdPrune.Dir = repoRoot
				_ = cmdPrune.Run()

				_ = ensureWritableAndRemove(wtPath)

				// Clean up the associated nomos data folder for this worktree (e.g., <repoRoot>/.nomos/data/<entry.Name()>)
				dataDir := filepath.Join(filepath.Dir(config.GlobalDataDir(repoRoot)), entry.Name())
				if _, err := os.Stat(dataDir); err == nil {
					synapse.Info("   ↳ Pruning associated data folder: %s\n", dataDir)
					_ = ensureWritableAndRemove(dataDir)
				}
			}
		}
	}
}

// isTaskClosed queries task tracker state to check if the issue is closed.
// This involves checking the persistent issue tracker configuration, establishing
// a network request with a context timeout, and determining if the task's status
// satisfies the criteria for closure. Stale network calls are aggressively timed out
// to ensure that bulk pruning operations do not hang the developer's terminal.
func isTaskClosed(tracker task.Tracker, taskID string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tDetail, err := tracker.View(ctx, taskID)
	if err != nil {
		return strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "404")
	}
	if tDetail == nil {
		return false
	}
	return tDetail.IsClosed()
}

// hygieneProcesses cleans up unregistered active processes.
func hygieneProcesses(dbFiles []string) {
	for _, dbPath := range dbFiles {
		if fi, err := os.Stat(dbPath); err == nil && !fi.IsDir() {
			db, err := db.Open(dbPath)
			if err == nil {
				pruneDeadProcesses(db)
			}
		}
	}
}

// pruneDeadProcesses queries the active_processes table and deletes dead PIDs.
func pruneDeadProcesses(db *sql.DB) {
	rows, err := db.Query("SELECT pid, command FROM active_processes;")
	if err != nil {
		return
	}
	defer rows.Close()

	type activeProc struct {
		pid int
		cmd string
	}
	var procs []activeProc
	for rows.Next() {
		var ap activeProc
		if errScan := rows.Scan(&ap.pid, &ap.cmd); errScan == nil {
			procs = append(procs, ap)
		}
	}

	for _, ap := range procs {
		if !isProcessAlive(ap.pid) {
			_, _ = db.Exec("DELETE FROM active_processes WHERE pid = ?;", ap.pid)
			synapse.Info("   ↳ Cleaned up dead active process: PID %d (%s)\n", ap.pid, ap.cmd)
		}
	}
}

// isProcessAlive queries signal status to check process health.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// checkExpiredQualityDebt prints warnings for expired quality debt bypasses.
func checkExpiredQualityDebt(repoRoot string) {
	manifestPath := filepath.Join(config.GlobalDataDir(repoRoot), "state", "quality_debt.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return
	}

	var manifest verify.QualityDebtManifest
	if errUnmarshal := json.Unmarshal(data, &manifest); errUnmarshal == nil {
		for _, item := range manifest.ActiveDebt {
			if isDebtExpired(item) {
				synapse.Info("   ⚠️  [Quality Debt Expired] Bypass for '%s' (gate: %s) expired on %s\n", item.File, item.Gate, item.ExpiresAt)
			}
		}
	}
}

// isDebtExpired parses and checks if a QualityDebtItem has expired.
func isDebtExpired(item verify.QualityDebtItem) bool {
	expiry, err := time.Parse(time.RFC3339, item.ExpiresAt)
	if err != nil {
		expiry, err = time.Parse("2006-01-02T15:04:05Z", item.ExpiresAt)
		if err != nil {
			expiry, _ = time.Parse("2006-01-02", item.ExpiresAt)
		}
	}
	return time.Now().After(expiry)
}

// pruneTempFiles deletes old run logs, commit_msg, and task-specific temp files.
// It iterates through the workspace's designated temporary directory and applies
// a retention policy to each file. Temporary files that have exceeded their
// time-to-live (e.g. 3 days for logs, 7 days for stories) are forcefully removed
// to maintain a pristine directory structure.
func pruneTempFiles(repoRoot string, tracker task.Tracker) {
	tmpDir := config.TmpDir(repoRoot)
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			filePath := filepath.Join(tmpDir, entry.Name())
			if shouldPruneFile(entry.Name(), filePath, tracker) {
				_ = os.Remove(filePath)
			}
		}
	}
	synapse.Info("%s", fmt.Sprint("   ↳ Temp files and outdated logs pruned."))
}

// shouldPruneFile decides if a temporary log or story file should be deleted.
func shouldPruneFile(name string, path string, tracker task.Tracker) bool {
	// Delete old commit message logs.
	if strings.HasPrefix(name, "commit_msg_") && (strings.HasSuffix(name, ".txt") || strings.HasSuffix(name, ".md")) {
		return true
	}

	// Delete log files older than 3 days.
	if strings.HasSuffix(name, ".log") {
		if info, errStat := os.Stat(path); errStat == nil {
			return time.Since(info.ModTime()) > 3*24*time.Hour
		}
	}

	// Delete story files belonging to closed tasks or older than 7 days.
	if shouldPruneStoryFile(name, path, tracker) {
		return true
	}

	return false
}

// shouldPruneStoryFile decides if a story file belongs to a closed task or is stale.
func shouldPruneStoryFile(name string, path string, tracker task.Tracker) bool {
	if !strings.HasPrefix(name, "story_") || !strings.HasSuffix(name, ".md") {
		return false
	}
	if tracker != nil {
		trimmed := strings.TrimPrefix(name, "story_")
		trimmed = strings.TrimSuffix(trimmed, ".md")
		parts := strings.Split(trimmed, "_")
		taskID := parts[0]
		if taskID != "" && isTaskClosed(tracker, taskID) {
			return true
		}
	}
	if info, errStat := os.Stat(path); errStat == nil {
		return time.Since(info.ModTime()) > 7*24*time.Hour
	}
	return false
}

// pruneStateFiles deletes old transient pipeline tracking files in state directory.
func pruneStateFiles(repoRoot string) {
	stateDir := config.StateDir(repoRoot)
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		filePath := filepath.Join(stateDir, entry.Name())
		if info, err := os.Stat(filePath); err == nil && shouldPruneStateFile(info, entry.Name()) {
			_ = os.Remove(filePath)
		}
	}
}

// shouldPruneStateFile checks if transient state file is stale.
func shouldPruneStateFile(info os.FileInfo, name string) bool {
	if !strings.HasPrefix(name, ".dor_re_satisfied_") && !strings.HasPrefix(name, ".last_phase_comment_") {
		return false
	}
	return time.Since(info.ModTime()) > 7*24*time.Hour
}

// ensureWritableAndRemove recursively makes files under path writable and removes them.
func ensureWritableAndRemove(path string) error {
	_ = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err == nil {
			_ = os.Chmod(p, 0777)
		}
		return nil
	})
	return os.RemoveAll(path)
}

// pruneRefactorStories deletes draft story files under tmp/refactor_stories/ that are no longer active in quality debt.
func pruneRefactorStories(repoRoot string) {
	activeIDs := loadActiveStoryIDs(repoRoot)
	storiesDir := filepath.Join(config.GlobalDataDir(repoRoot), "tmp", "refactor_stories")
	entries, err := os.ReadDir(storiesDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if shouldPruneRefactorFile(entry.Name(), activeIDs) {
			_ = os.Remove(filepath.Join(storiesDir, entry.Name()))
		}
	}
}

// loadActiveStoryIDs parses quality_debt.json and extracts active story IDs.
func loadActiveStoryIDs(repoRoot string) map[string]bool {
	manifestPath := filepath.Join(config.GlobalDataDir(repoRoot), "state", "quality_debt.json")
	var manifest struct {
		ActiveDebt []struct {
			File string `json:"file"`
			Gate string `json:"gate"`
		} `json:"active_debt"`
	}

	activeIDs := make(map[string]bool)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return activeIDs
	}
	_ = json.Unmarshal(data, &manifest)

	for _, item := range manifest.ActiveDebt {
		relFile := item.File
		if filepath.IsAbs(relFile) {
			if rel, err := filepath.Rel(repoRoot, relFile); err == nil {
				relFile = rel
			}
		}
		storyHash := md5.Sum([]byte(relFile + "_" + item.Gate))
		storyID := hex.EncodeToString(storyHash[:])[:8]
		activeIDs[storyID] = true
	}
	return activeIDs
}

// shouldPruneRefactorFile checks if the refactor story file should be pruned.
func shouldPruneRefactorFile(name string, activeIDs map[string]bool) bool {
	if !strings.HasPrefix(name, "refactor_") || !strings.HasSuffix(name, ".md") {
		return false
	}
	trimmed := strings.TrimPrefix(name, "refactor_")
	storyID := strings.TrimSuffix(trimmed, ".md")
	return !activeIDs[storyID]
}
