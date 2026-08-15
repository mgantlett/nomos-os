package verify

import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// checkWorktreeStatus verifies that a given git worktree is clean and pushed.
// It runs git status --porcelain to check for uncommitted modifications,
// and uses git log @{u}..HEAD to ensure there are no unpushed commits.
// It returns an error if any checks fail, detailing the failure mode.
func checkWorktreeStatus(path string) error {
	// 1. Check if the worktree is clean
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = path
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GIT_") {
			env = append(env, e)
		}
	}
	statusCmd.Env = env
	out, err := statusCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git status failed in %s: %w, stderr: %s", path, err, string(out))
	}
	if len(strings.TrimSpace(string(out))) > 0 {
		return fmt.Errorf("upstream worktree has uncommitted changes: %s", path)
	}

	// 2. Check if the worktree is pushed
	// Check if HEAD is ahead of its upstream counterpart
	branchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branchCmd.Dir = path
	branchCmd.Env = env
	branchOut, err := branchCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get branch in %s: %w", path, err)
	}
	branch := strings.TrimSpace(string(branchOut))

	// Check if branch has an upstream tracking branch configured
	upstreamCmd := exec.Command("git", "rev-parse", "--abbrev-ref", branch+"@{u}")
	upstreamCmd.Dir = path
	upstreamCmd.Env = env
	if err := upstreamCmd.Run(); err != nil {
		return fmt.Errorf("upstream worktree branch '%s' has no remote tracking branch (not pushed): %s", branch, path)
	}

	// Check unpushed commits by comparing upstream to local HEAD
	logCmd := exec.Command("git", "log", "@{u}..HEAD", "--oneline")
	logCmd.Dir = path
	logCmd.Env = env
	logOut, err := logCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check upstream log in %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(logOut))) > 0 {
		return fmt.Errorf("upstream worktree has unpushed commits: %s", path)
	}

	return nil
}

// parseGoWork extracts all 'use' directives from a given go.work file content.
// It handles both inline `use ./path` and block `use ( ... )` formats.
// The returned slice contains the raw string paths defined in the file.
func parseGoWork(content []byte) []string {
	var uses []string
	lines := strings.Split(string(content), "\n")
	inUseBlock := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "use (") {
			inUseBlock = true
			continue
		}
		if inUseBlock {
			if line == ")" {
				inUseBlock = false
				continue
			}
			uses = append(uses, strings.Trim(line, `"`))
		} else if strings.HasPrefix(line, "use ") {
			path := strings.TrimSpace(strings.TrimPrefix(line, "use "))
			uses = append(uses, strings.Trim(path, `"`))
		}
	}
	return uses
}

// processWorktree resolves the absolute path and verifies the status of a worktree dependency.
// It skips non-git directories and non-paths like '.' to prevent infinite loops.
func processWorktree(usePath, root string, errCh chan<- error, wg *sync.WaitGroup) {
	if usePath == "." || usePath == "./" {
		return // Skip the current root
	}

	absUsePath := usePath
	if !filepath.IsAbs(usePath) {
		absUsePath = filepath.Clean(filepath.Join(root, usePath))
	}

	if _, err := os.Stat(filepath.Join(absUsePath, ".git")); os.IsNotExist(err) {
		return // Not a git repo or worktree, skip it
	}

	wg.Add(1)
	go func(path string) {
		defer wg.Done()
		if err := checkWorktreeStatus(path); err != nil {
			errCh <- err
		}
	}(absUsePath)
}

// runCrossRepoWorktreeGate parses go.work (if it exists) and ensures that all
// sibling git worktrees are fully committed and pushed to their remote origins.
// This prevents cross-repository topology errors during automated swarm agent deployment.
// It acts as a hard boundary before the active downstream repository is allowed to commit.
func runCrossRepoWorktreeGate(ctx *workspace.WorkspaceContext) (StageResult, error) {
	root := ctx.RepoRoot
	goWorkPath := filepath.Join(root, "go.work")

	if _, err := os.Stat(goWorkPath); os.IsNotExist(err) {
		return StageResult{Passed: true, Message: "No go.work found (Local Workspace inactive)."}, nil
	}

	content, err := os.ReadFile(goWorkPath)
	if err != nil {
		return StageResult{Passed: false, Message: "Failed to read go.work"}, err
	}

	uses := parseGoWork(content)

	var wg sync.WaitGroup
	errCh := make(chan error, len(uses))

	for _, usePath := range uses {
		processWorktree(usePath, root, errCh, &wg)
	}

	wg.Wait()
	close(errCh)

	var errs []string
	for err := range errCh {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return StageResult{
			Passed:  false,
			Message: "Cross-repo dependency validation failed. You must commit and push all linked upstream worktrees first.",
		}, fmt.Errorf("%s", strings.Join(errs, "\n"))
	}

	return StageResult{Passed: true, Message: "All dependent cross-repo worktrees are clean and pushed"}, nil
}
