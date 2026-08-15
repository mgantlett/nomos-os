package cockpit

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-playground/form/v4"
	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/db"
	nomosexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
)

var formDecoder = form.NewDecoder()

// GitFile represents the status details of a single modified file.
type GitFile struct {
	File   string `json:"file"`
	Status string `json:"status"`
}

// parseGitStatusLine processes a single line from git status --porcelain.
func parseGitStatusLine(line string) (string, string, bool) {
	if len(line) < 4 {
		return "", "", false
	}
	statusSymbol := line[:2]
	fileStr := strings.TrimSpace(line[2:])

	if strings.HasPrefix(fileStr, "\"") && strings.HasSuffix(fileStr, "\"") {
		if unq, err := strconv.Unquote(fileStr); err == nil {
			fileStr = unq
		}
	}

	if config.IsInternalSystemDir(fileStr) {
		return "", "", false
	}

	return fileStr, statusSymbol, true
}

func classifyAndAppend(fileStr, statusSymbol string, staged, unstaged, untracked *[]GitFile) {
	if statusSymbol == "??" {
		*untracked = append(*untracked, GitFile{File: fileStr, Status: "untracked"})
		return
	}
	if statusSymbol[0] != ' ' && statusSymbol[0] != '?' {
		*staged = append(*staged, GitFile{File: fileStr, Status: string(statusSymbol[0])})
	}
	if statusSymbol[1] != ' ' && statusSymbol[1] != '?' {
		*unstaged = append(*unstaged, GitFile{File: fileStr, Status: string(statusSymbol[1])})
	}
}

// HandleGitStatusRoute executes the underlying `git status --porcelain` command for the
// specified directory and parses its output into structured JSON format.
// It scopes the command to a target path passed in via the 'path' query parameter,
// allowing callers to inspect the status of different worktrees dynamically.
func HandleGitStatusRoute(w http.ResponseWriter, r *http.Request, defaultRepoRoot string) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	targetPath := r.URL.Query().Get("path")
	if targetPath == "" || targetPath == "ALL" {
		targetPath = defaultRepoRoot
	}

	cmd := exec.Command("git", "status", "--porcelain", "-u")
	cmd.Dir = targetPath
	out, err := cmd.Output()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"success":false,"error":"git status failed: %v"}`, err), http.StatusInternalServerError)
		return
	}

	staged := []GitFile{}
	unstaged := []GitFile{}
	untracked := []GitFile{}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		fileStr, statusSymbol, ok := parseGitStatusLine(line)
		if ok {
			classifyAndAppend(fileStr, statusSymbol, &staged, &unstaged, &untracked)
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"staged":    staged,
		"unstaged":  unstaged,
		"untracked": untracked,
	})
}

// HandleGitDiffRoute executes `git diff` for a requested file.
// It handles both staged and unstaged diffs via the 'staged' query parameter,
// and scopes the command execution to the requested worktree path.
func HandleGitDiffRoute(w http.ResponseWriter, r *http.Request, defaultRepoRoot string) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	fileParam := r.URL.Query().Get("file")
	stagedParam := r.URL.Query().Get("staged")
	targetPath := r.URL.Query().Get("path")

	if fileParam == "" {
		http.Error(w, `{"success":false,"error":"file parameter is required"}`, http.StatusBadRequest)
		return
	}

	if targetPath == "" || targetPath == "ALL" {
		targetPath = defaultRepoRoot
	}

	var args []string
	if stagedParam == "true" {
		args = append(args, "diff", "--cached", "--", fileParam)
	} else {
		args = append(args, "diff", "--", fileParam)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = targetPath
	out, err := cmd.Output()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"success":false,"error":"git diff failed: %v"}`, err), http.StatusInternalServerError)
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"diff":    string(out),
	})
}

// HandleGitStageRoute exposes an HTTP POST endpoint for modifying the git index.
// It supports both staging changes (`git add`) and unstaging them (`git restore --staged`).
// This allows the IDE/cockpit to seamlessly transition individual files into a commit payload.
func HandleGitStageRoute(w http.ResponseWriter, r *http.Request, defaultRepoRoot string) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		http.Error(w, `{"success":false,"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		File  string `json:"file"`
		Stage bool   `json:"stage"`
		Path  string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf(`{"success":false,"error":"invalid body: %v"}`, err), http.StatusBadRequest)
		return
	}

	if req.File == "" {
		http.Error(w, `{"success":false,"error":"file is required"}`, http.StatusBadRequest)
		return
	}

	targetPath := req.Path
	if targetPath == "" || targetPath == "ALL" {
		targetPath = defaultRepoRoot
	}

	var cmd *exec.Cmd
	if req.Stage {
		cmd = exec.Command("git", "add", req.File)
	} else {
		cmd = exec.Command("git", "restore", "--staged", req.File)
	}
	cmd.Dir = targetPath

	err := cmd.Run()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"success":false,"error":"git command failed: %v"}`, err), http.StatusInternalServerError)
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

// HandleApiTaskTransition routes phase transition requests (e.g. EDIT, REVIEW, DONE).
// It verifies that the task being transitioned matches the active workspace context,
// and enforces phase constraints to ensure that tasks move linearly through the pipeline.
func HandleApiTaskTransition(w http.ResponseWriter, r *http.Request, repoRoot string) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	var req struct {
		ID     string `form:"id"`
		Column string `form:"column"`
	}
	if err := formDecoder.Decode(&req, r.URL.Query()); err != nil {
		http.Error(w, `{"success":false,"error":"invalid form"}`, http.StatusBadRequest)
		return
	}
	taskID := req.ID
	column := req.Column

	if taskID == "" {
		http.Error(w, `{"success":false,"error":"id is required"}`, http.StatusBadRequest)
		return
	}

	taskID = strings.TrimPrefix(taskID, "#")
	dbPath := config.ResolveCacheDbPath(repoRoot)

	if err := validateTransitionTask(config.PhaseStatePath(repoRoot), taskID); err != nil {
		http.Error(w, fmt.Sprintf(`{"success":false,"error":"%v"}`, err), http.StatusBadRequest)
		return
	}

	if column == "EDIT" {
		if err := task.TransitionPhase(repoRoot, "EDIT"); err != nil {
			http.Error(w, fmt.Sprintf(`{"success":false,"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}
	} else if column == "DONE" {
		if err := executeDoneTransition(repoRoot, dbPath, taskID); err != nil {
			http.Error(w, fmt.Sprintf(`{"success":false,"error":"%v"}`, err), http.StatusBadRequest)
			return
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

// HandleApiTaskReset performs a hard reset of a task environment.
// It terminates all active processes associated with the task's worktree,
// force-prunes the git worktree, forcefully deletes the tracking branch,
// and resets the main repository to an IDLE phase.
func HandleApiTaskReset(w http.ResponseWriter, r *http.Request, repoRoot string) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	dbPath := config.ResolveCacheDbPath(repoRoot)

	var req struct {
		ID string `form:"id"`
	}
	if err := formDecoder.Decode(&req, r.URL.Query()); err != nil {
		http.Error(w, `{"success":false,"error":"invalid form"}`, http.StatusBadRequest)
		return
	}
	taskID := req.ID
	if taskID == "" {
		http.Error(w, `{"success":false,"error":"id is required"}`, http.StatusBadRequest)
		return
	}

	taskID = strings.TrimPrefix(taskID, "#")
	wtDir := filepath.Join(config.WorktreesDir(repoRoot), "task-"+taskID)

	PerformTaskReset(repoRoot, dbPath, taskID, wtDir)

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

// HandleApiWorktreesPrune performs cleanup on stale git worktrees.
// It executes git commands to remove the specified worktree and force prune
// any unlinked directories. It includes fallback logic to clean up the directory
// using OS-level commands if the native git cleanup fails.
func HandleApiWorktreesPrune(w http.ResponseWriter, r *http.Request, repoRoot string) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != "POST" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "method must be POST"})
		return
	}

	var req struct {
		Path string `json:"path"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	dbPath := config.ResolveCacheDbPath(repoRoot)

	var pruned []string
	if req.Path != "" {
		_, _ = nomosexec.RunCommand(dbPath, "git", "worktree", "remove", "-f", req.Path)
		_, _ = nomosexec.RunCommand(dbPath, "git", "worktree", "prune")

		err := ensureWritableAndRemove(req.Path)
		if err != nil && !os.IsNotExist(err) {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("failed to clean up worktree directory: %v", err),
			})
			return
		}
		pruned = append(pruned, req.Path)
	} else {
		out, err := nomosexec.RunCommand(dbPath, "git", "worktree", "prune")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("failed to prune worktrees: %v (output: %s)", err, out),
			})
			return
		}
		pruned = append(pruned, "all-stale")
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "pruned": pruned})
}

// getActiveProcesses queries the SQLite task database to retrieve a list
// of all currently active background processes and their respective PIDs.
// This is critical for lifecycle management of long-running operations.
func getActiveProcesses(dbPath string) ([]struct {
	Pid     int
	Command string
}, error) {
	dbConn, err := db.Open(dbPath)
	if err != nil {
		return nil, err
	}
	defer dbConn.Close()

	rows, err := dbConn.Query("SELECT pid, command FROM active_processes;")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []struct {
		Pid     int
		Command string
	}
	for rows.Next() {
		var ap struct {
			Pid     int
			Command string
		}
		if err := rows.Scan(&ap.Pid, &ap.Command); err == nil {
			list = append(list, ap)
		}
	}
	return list, nil
}

// killProcess forcefully terminates a process by its PID.
// It bypasses graceful shutdown by directly signaling the OS process.
func killProcess(pid int) {
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Kill()
	}
}

// deleteProcessFromDB removes the recorded process entry from the active_processes table
// in the SQLite database to keep the system state synchronized with the OS state.
func deleteProcessFromDB(dbPath string, pid int) {
	if dbConn, err := db.Open(dbPath); err == nil {
		_, _ = dbConn.Exec("DELETE FROM active_processes WHERE pid = ?;", pid)
		dbConn.Close()
	}
}

// isTaskCommand checks if a given command string is associated with the given taskID.
// It looks for common patterns like the task directory or command arguments.
func isTaskCommand(cmd, taskID string) bool {
	return strings.Contains(cmd, "task-"+taskID) || strings.Contains(cmd, "task "+taskID) || strings.Contains(cmd, "worktrees/task-"+taskID)
}

// killTaskProcesses aggregates and kills all active background processes
// that are associated with the specified taskID across the entire workspace.
func killTaskProcesses(dbPath, taskID string) {
	if list, err := getActiveProcesses(dbPath); err == nil {
		for _, ap := range list {
			if isTaskCommand(ap.Command, taskID) {
				killProcess(ap.Pid)
				deleteProcessFromDB(dbPath, ap.Pid)
			}
		}
	}
}

// getBranchToDelete parses the output of `git worktree list` to find
// the specific tracking branch associated with the transient task worktree.
func getBranchToDelete(dbPath, taskID string) string {
	var branchToDelete string
	if out, err := nomosexec.RunCommand(dbPath, "git", "worktree", "list"); err == nil {
		lines := strings.Split(out, "\n")
		for _, line := range lines {
			if strings.Contains(line, "task-"+taskID) {
				start := strings.Index(line, "[")
				end := strings.Index(line, "]")
				if start != -1 && end != -1 && end > start {
					branchToDelete = line[start+1 : end]
				}
			}
		}
	}
	return branchToDelete
}

// deleteBranches removes the specified task tracking branch from the repository.
// If no explicit branch is provided, it falls back to parsing all local branches
// and aggressively pruning any branches that match the task ID prefix.
func deleteBranches(dbPath, taskID, branchToDelete string) {
	if branchToDelete != "" {
		_, _ = nomosexec.RunCommand(dbPath, "git", "branch", "-D", branchToDelete)
	} else {
		fallbackBranch := fmt.Sprintf("task/%s-aider", taskID)
		_, _ = nomosexec.RunCommand(dbPath, "git", "branch", "-D", fallbackBranch)
		if out, err := nomosexec.RunCommand(dbPath, "git", "branch"); err == nil {
			for _, b := range strings.Split(out, "\n") {
				b = strings.TrimSpace(strings.TrimPrefix(b, "*"))
				if strings.HasPrefix(b, "task/"+taskID) {
					_, _ = nomosexec.RunCommand(dbPath, "git", "branch", "-D", b)
				}
			}
		}
	}
}

// cleanMainRepository forcefully resets the primary git repository
// by checking out the current state, cleaning all untracked files,
// and transitioning the system state back to IDLE.
func cleanMainRepository(repoRoot, dbPath, taskID string) {
	if bytes, err := os.ReadFile(config.StateTaskIdPath(repoRoot)); err == nil {
		if strings.TrimSpace(string(bytes)) == taskID {
			_, _ = nomosexec.RunCommand(dbPath, "git", "checkout", ".")
			_, _ = nomosexec.RunCommand(dbPath, "git", "clean", "-fd")
			_ = task.TransitionPhase(repoRoot, "IDLE")
			_ = os.WriteFile(config.StateTaskIdPath(repoRoot), []byte(""), 0644)
		}
	}
}

// ensureWritableAndRemove recursively changes permissions of a directory tree
// to ensure it is writable, then attempts to forcefully remove it from disk.
func ensureWritableAndRemove(targetPath string) error {
	_ = filepath.Walk(targetPath, func(pathStr string, fInfo os.FileInfo, fileErr error) error {
		if fileErr == nil {
			_ = os.Chmod(pathStr, 0777)
		}
		return nil
	})
	return os.RemoveAll(targetPath)
}

// PerformTaskReset executes the complete teardown sequence for a task:
// killing processes, removing worktrees, pruning branches, and cleaning the workspace.
func PerformTaskReset(repoRoot, dbPath, taskID, wtDir string) {
	killTaskProcesses(dbPath, taskID)
	branchToDelete := getBranchToDelete(dbPath, taskID)

	_, _ = nomosexec.RunCommand(dbPath, "git", "worktree", "prune")
	_, _ = nomosexec.RunCommand(dbPath, "git", "worktree", "remove", "-f", wtDir)
	_ = ensureWritableAndRemove(wtDir)

	deleteBranches(dbPath, taskID, branchToDelete)
	cleanMainRepository(repoRoot, dbPath, taskID)
}

// validateTransitionTask reads the phase state file to ensure that IDE-driven tasks
// are not improperly mutated by background or manual processes.
func validateTransitionTask(stateFile, taskID string) error {
	if data, err := os.ReadFile(stateFile); err == nil {
		var pstate struct {
			TaskId    string `json:"task_id"`
			AgentType string `json:"agent_type"`
			Agent     string `json:"agent"`
		}
		if json.Unmarshal(data, &pstate) == nil {
			if pstate.TaskId == taskID && (pstate.AgentType == "ide" || pstate.Agent == "antigravity") {
				return fmt.Errorf("This task is IDE-driven. Approvals and transitions must be processed directly inside the IDE chat session.")
			}
		}
	}
	return nil
}

// executeDoneTransition handles the final verification and closure of a task.
// It executes the DoD verification gate, updates the phase state to DONE,
// and closes the task record in the Kanban tracker database.
func executeDoneTransition(repoRoot, dbPath, taskID string) error {
	_ = task.TransitionPhase(repoRoot, "REVIEW")
	if _, err := nomosexec.RunCommand(dbPath, "bin/nomos", "verify"); err != nil {
		return fmt.Errorf("DoD Verification Failed: %v", err)
	}
	_ = task.TransitionPhase(repoRoot, "DONE")
	if cfg, err := task.LoadConfig(repoRoot); err == nil {
		if tracker, err := task.NewTracker(cfg); err == nil {
			_ = tracker.Close(context.Background(), taskID, "Done via Cockpit Kanban Board Release")
		}
	}
	return nil
}
