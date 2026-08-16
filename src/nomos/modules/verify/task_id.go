package verify

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

// GetActiveTaskId attempts to detect the active Task ID from local files or git branch.
// This function operates as a cascading fallback mechanism. It sequentially checks:
// 1. The deterministic state file cache (.state_task_id) updated by lifecycle commands.
// 2. The temporary markdown file used by LLM agents for in-progress tasks.
// 3. The current git branch name, parsing conventional branching patterns.
// 4. Finally, the phase state json file acting as the single source of truth.
func GetActiveTaskId(root string) string {
	if id := getTaskIdFromParentTask(root); id != "" {
		return id
	}
	if id := getTaskIdFromState(root); id != "" {
		return id
	}
	if id := getTaskIdFromTaskMd(root); id != "" {
		return id
	}
	if id := getTaskIdFromGitBranch(root); id != "" {
		return id
	}
	return getTaskIdFromPhaseState(root)
}

// getTaskIdFromState reads the active task ID from the .agent/state/.state_task_id cache.
func getTaskIdFromState(root string) string {
	stateTaskIdPath := workspace.MustNewContext(root).NomosStatePath(".state_task_id")
	if content, err := os.ReadFile(stateTaskIdPath); err == nil {
		return strings.TrimSpace(string(content))
	}
	return ""
}

// getTaskIdFromParentTask reads the active task ID from the .nomos_parent_task file.
func getTaskIdFromParentTask(root string) string {
	parentTaskPath := filepath.Join(root, ".nomos_parent_task")
	if data, err := os.ReadFile(parentTaskPath); err == nil {
		return strings.TrimSpace(string(data))
	}
	return ""
}

// getTaskIdFromTaskMd parses the temporary task markdown file to extract the issue code.
func getTaskIdFromTaskMd(root string) string {
	taskMdPath := filepath.Join(workspace.MustNewContext(root).TmpDir(), "task.md")
	if content, err := os.ReadFile(taskMdPath); err == nil {
		rx := regexp.MustCompile(`([A-Z0-9]+-\d+)`)
		if match := rx.FindString(string(content)); match != "" {
			return strings.ToUpper(match)
		}
		rxNum := regexp.MustCompile(`(?:#)?(\d+)`)
		if match := rxNum.FindStringSubmatch(string(content)); len(match) > 1 {
			return match[1]
		}
	}
	return ""
}

// getTaskIdFromGitBranch parses the current active git branch name to detect the task ID.
func getTaskIdFromGitBranch(root string) string {
	if out, err := runGit(root, "branch", "--show-current"); err == nil {
		branch := strings.TrimSpace(out)
		rxBranch := regexp.MustCompile(`(?:task|bug|issue)/([a-zA-Z0-9]+-\d+|\d+)`)
		if match := rxBranch.FindStringSubmatch(branch); len(match) > 1 {
			return strings.ToUpper(match[1])
		}
		rxID := regexp.MustCompile(`(?:^|/)(\d+)(?:-|$)`)
		if match := rxID.FindStringSubmatch(branch); len(match) > 1 {
			return match[1]
		}
	}
	return ""
}

// getTaskIdFromPhaseState parses the internal phase state JSON document to extract
// the formally recorded task ID, acting as the absolute last resort fallback.
func getTaskIdFromPhaseState(root string) string {
	phaseStatePath := workspace.MustNewContext(root).NomosStatePath(".phase_state.json")
	if content, err := os.ReadFile(phaseStatePath); err == nil {
		rxState := regexp.MustCompile(`"task_id":\s*"([^"]+)"`)
		if match := rxState.FindStringSubmatch(string(content)); len(match) > 1 && match[1] != "" {
			return match[1]
		}
	}
	return ""
}
