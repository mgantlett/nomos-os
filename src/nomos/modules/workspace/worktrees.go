package workspace

import (
	"os"
	"path/filepath"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

// GetCrossRepoWorktrees scans the transient worktrees directory and returns
// a list of absolute paths to active task worktrees (identified by .nomos_parent_task).
// It iterates over the target user workspace configuration folder and skips any directories
// that do not contain the explicit tracker marker file to prevent accidental teardowns.
func GetCrossRepoWorktrees(ctx *workspace.WorkspaceContext) ([]string, error) {
	repoRoot := ctx.RepoRoot
	wtDir := workspace.MustNewContext(repoRoot).WorktreesDir()
	entries, err := os.ReadDir(wtDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var worktrees []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		absPath := filepath.Join(wtDir, entry.Name())
		// Active task worktrees contain a .nomos_parent_task tracker file
		if _, err := os.Stat(filepath.Join(absPath, ".nomos_parent_task")); err == nil {
			worktrees = append(worktrees, absPath)
		}
	}

	return worktrees, nil
}
