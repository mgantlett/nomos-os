package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"github.com/mgantlett/nomos-commons/src/nomos/core/telemetry"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
)

func init() {
	task.EscalationEvaluatorFunc = evaluateEscalation
}

func evaluateEscalation(ctx *workspace.WorkspaceContext, key string, failCount int, detail string) (bool, string, error) {
	repoRoot := ctx.RepoRoot
	escalate, reason := telemetry.GlobalSwarmAggregator.ShouldEscalate(key)
	if !escalate {
		return false, "", nil
	}

	cfg, err := config.LoadConfig(filepath.Join(config.GlobalDataDir(repoRoot), "config.yaml"))
	if err != nil {
		return false, "", err
	}

	if cfg.Provider == config.ProviderLocal {
		taskCfg, err := task.LoadConfig(ctx)
		if err == nil {
			tracker, err := task.NewTracker(taskCfg)
			if err == nil {
				tracker.Transition(context.Background(), key, task.StatusParked)
				synapse.Info("Local task %s exhausted cycles. Parked to prevent cloud escalation.", key)
			}
		}
		return false, "Parked local task instead of escalating", nil
	}

	synapse.Info("Escalating task %s to cloud due to: %s", key, reason)
	return true, reason, nil
}

// RemediateASTCycle hooks into verify phase errors and dispatches an autonomous
// swarm worker to remediate package dependency cycles. It uses deterministic hashing
// to prevent infinite remediation loops on the exact same cycle signature.
func RemediateASTCycle(root string, cycleSignature string) {
	// 1. Hash the cycle signature
	hash := sha256.Sum256([]byte(cycleSignature))
	cycleHash := hex.EncodeToString(hash[:])

	// 2. Check for infinite loop using escalations state directory
	escalationDir := filepath.Join(config.GlobalDataDir(root), "state", "escalations")
	_ = os.MkdirAll(escalationDir, 0755)
	flagFile := filepath.Join(escalationDir, cycleHash+".flag")

	if _, err := os.Stat(flagFile); err == nil {
		synapse.Info("AST cycle already remediated. Skipping dispatch to prevent infinite loop.\n")
		return
	}

	// 3. Set the loop protection flag
	_ = os.WriteFile(flagFile, []byte("remediated"), 0644)

	// 4. Create local Backlog task
	ctx := context.Background()
	title := "Bug: Fix AST Cycle"
	body := fmt.Sprintf("An AST cycle was detected during verification:\n\n%s\n\nPlease surgically fix the cyclic dependency.", cycleSignature)
	labels := []string{"type:bug", "priority:critical", "agent:swarm:antigravity"}

	// Initialize the local tracker using the active configuration.
	// We load the configuration from the active root to ensure correct binding.
	wCtx, _ := workspace.NewContext(root)
	cfg, err := task.LoadConfig(wCtx)
	if err != nil {
		synapse.Info("Failed to load task tracker config for auto-remediation: %v\n", err)
		return
	}

	// Instantiate the tracker backend.
	// This creates the SQLite interface and configures the tracking logic.
	tracker, err := task.NewTracker(cfg)
	if err != nil {
		synapse.Info("Failed to initialize task tracker for auto-remediation: %v\n", err)
		return
	}

	// Wrap the tracker with the Data Integrity state hashing middleware.
	// This ensures our autonomous actions cryptographically seal the workspace.
	tracker = task.WrapWithStateHash(tracker, wCtx)

	// Create the remediation task in the tracking system.
	// It is created in the Backlog status, unassigned, waiting for Swarm assignment.
	newKey, err := tracker.Create(ctx, title, body, labels, task.Unassigned, filepath.Base(root), task.TypeBug, false, task.StatusBacklog)
	if err != nil {
		synapse.Info("Failed to create remediation task: %v\n", err)
		return
	}

	synapse.Info("Created autonomous cycle remediation task: %s\n", newKey)

	// 5. Dispatch Area 1 Swarm worker
	// This asynchronously spins up an agent to heal the cyclic dependency tree.
	task.DispatchWorktreeSwarmWorker(ctx, root, newKey)
}
