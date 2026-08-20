package cmd

// Package cmd provides the root commands for the Nomos CLI.
// It includes utilities for path resolution and command handling.
import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"fmt"
	"os"
	"strings"
)

// findRepoRoot searches upwards from the start directory to find the git repository root.
// Results are memoized in repoRootCache to eliminate redundant disk I/O traversals.
func findRepoRoot(start string) string {
	ctx, err := workspace.NewContext(start)
	if err == nil {
		return ctx.RepoRoot
	}
	return start
}

// enforceRootZone strictly checks that the active directory is the global Hollow Shell.
// If it is a transient worktree, it panics with deterministic CLI directions.
func enforceRootZone(ctx *workspace.WorkspaceContext, cmdName string) error {
	wd, _ := os.Getwd()
	if strings.Contains(wd, ".explorer") {
		return fmt.Errorf("Execution out of bounds: 'nomos %s' cannot be executed inside the read-only .explorer worktree. Please run 'cd ../../' back to the root hollow shell before executing.", cmdName)
	}
	if !isOrchestratorRoot(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(wd); return c }()) {
		return fmt.Errorf("Execution out of bounds: 'nomos %s' must be executed from the Root Hollow Shell. Please cd back to the repository root.", cmdName)
	}
	return nil
}

// enforceWorktreeZone strictly checks that the active directory is an isolated transient worktree.
// If it is the global Hollow Shell (where src/ is hidden) or the read-only .explorer worktree, it panics with deterministic CLI directions.
func enforceWorktreeZone(ctx *workspace.WorkspaceContext, cmdName string) error {
	wd, _ := os.Getwd()
	if isOrchestratorRoot(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(wd); return c }()) {
		return fmt.Errorf("Execution out of bounds: 'nomos %s' must be executed inside an isolated transient worktree. Please run 'cd worktrees/<task>' before executing.", cmdName)
	}
	if strings.Contains(wd, ".explorer") {
		return fmt.Errorf("Execution out of bounds: 'nomos %s' cannot be executed inside the read-only .explorer worktree. Please run 'cd worktrees/<task>' before executing.", cmdName)
	}
	return nil
}
