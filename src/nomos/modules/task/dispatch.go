// Package task provides models and functions for manipulating Agile tasks.
// dispatch.go contains the Autonomous DAG Dispatcher logic that automatically
// triggers Swarm workers when dependent tasks become unblocked.
package task

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
)

const maxSwarmWorkers = 5

func getActiveSwarmCount() int {
	out, err := exec.Command("pgrep", "-c", "-f", "nomos-swarm").Output()
	if err != nil {
		return 0
	}
	count, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return count
}

// DispatchWorktreeSwarmWorker spawns an asynchronous nomos-swarm process for the given task.
// It executes detached so it does not block the main process or hold active SQLite transactions.
func DispatchWorktreeSwarmWorker(ctx context.Context, repoRoot string, taskKey string) {
	if getActiveSwarmCount() >= maxSwarmWorkers {
		synapse.Info("Max concurrent active worker limits reached (%d). Skipping dispatch for task %s.\n", maxSwarmWorkers, taskKey)
		return
	}
	// The binary we want to execute
	swarmBin := filepath.Join(repoRoot, "bin", "nomos-swarm")

	// Create the command. We don't attach stdout/stderr to the parent
	// to ensure it is fully detached.
	cmd := exec.CommandContext(ctx, swarmBin, taskKey)
	cmd.Dir = repoRoot

	// Run it asynchronously
	if err := cmd.Start(); err != nil {
		synapse.Info("Failed to dispatch swarm worker for task %s: %v\n", taskKey, err)
		return
	}

	synapse.Info("Dispatched autonomous Swarm worker for unblocked task %s (PID: %d)\n", taskKey, cmd.Process.Pid)

	// We intentionally do not call cmd.Wait() here. The process will run independently.
	// For production readiness in a daemon this would be reaped or managed, but as a CLI
	// spawning a background process, this satisfies the decoupling requirement.
	go func() {
		// Wait in a separate goroutine just to reap the child process and avoid zombies
		_ = cmd.Wait()
		synapse.Info("Swarm worker for task %s completed.\n", taskKey)
	}()
}
