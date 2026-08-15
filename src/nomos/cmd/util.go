package cmd

// Package cmd provides the root commands for the Nomos CLI.
// It includes utilities for path resolution and command handling.
import (
	"fmt"
	"os"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
)

// findRepoRoot searches upwards from the start directory to find the git repository root.
// Results are memoized in repoRootCache to eliminate redundant disk I/O traversals.
func findRepoRoot(start string) string {
	return config.FindRepoRoot(start)
}

// enforceRootZone strictly checks that the active directory is the global Hollow Shell.
// If it is a transient worktree, it panics with deterministic CLI directions.
func enforceRootZone(repoRoot, cmdName string) error {
	wd, _ := os.Getwd()
	if !isOrchestratorRoot(wd) {
		return fmt.Errorf("Execution out of bounds: 'nomos %s' must be executed from the Root Hollow Shell. Please cd back to the repository root.", cmdName)
	}
	return nil
}

// enforceWorktreeZone strictly checks that the active directory is an isolated transient worktree.
// If it is the global Hollow Shell (where src/ is hidden), it panics with deterministic CLI directions.
func enforceWorktreeZone(repoRoot, cmdName string) error {
	wd, _ := os.Getwd()
	if isOrchestratorRoot(wd) {
		return fmt.Errorf("Execution out of bounds: 'nomos %s' must be executed inside an isolated transient worktree. Please run 'cd worktrees/<task>' before executing.", cmdName)
	}
	return nil
}
