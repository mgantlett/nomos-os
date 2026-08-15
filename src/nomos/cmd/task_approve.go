/*
Package cmd provides the CLI interface to interact with the Nomos engine.
This file implements the `task approve` command which is a vital part of the Agentic Approval
flow. It securely signs and advances the phase state upon Product Owner review.
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
	nomosexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/spf13/cobra"
)

// taskApproveCmd approves the plan or walkthrough for the active task and re-signs phase state.
// If the agent is in PLAN phase, it marks the plan as approved and ready for EDIT.
// If the agent is in EDIT phase, it enforces walkthrough creation and moves to REVIEW.
// If the agent is in REVIEW phase, it locks the substrate and marks the final commit as approved.
var taskApproveCmd = &cobra.Command{
	Use:   "approve",
	Short: "Approve the active task plan or walkthrough",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Retrieve repository working directory root
		repoRoot, err := os.Getwd()
		if err != nil {
			return err
		}

		// Read and unmarshal local phase state structure
		state, err := task.GetPhaseState(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())
		if err != nil {
			return err
		}

		phaseStatePath := config.PhaseStatePath(repoRoot)

		// Handle phase transition logic according to current workspace state machine
		if state.CurrentPhase == statepkg.PhasePlan {
			// In PLAN phase: approve implementation plan to unlock EDIT phase
			state.PlanApproved = "true"
		} else if state.CurrentPhase == statepkg.PhaseEdit {
			// In EDIT phase: enforce walkthrough artifact existence before entering REVIEW
			walkthroughPath := filepath.Join(config.WalkthroughsDir(repoRoot), state.TaskId+".md")
			if _, err := os.Stat(walkthroughPath); os.IsNotExist(err) {
				return fmt.Errorf("task approval rejected: global walkthroughs/%s.md is missing. Generate walkthrough artifact before transitioning to REVIEW", state.TaskId)
			}
			state.CurrentPhase = statepkg.PhaseReview
		} else if state.CurrentPhase == statepkg.PhaseReview {
			// In REVIEW phase: approve commit, release human gate, and seal substrate lock
			state.CommitApproved = "true"
			state.WaitingOnHuman = "false"
			_ = nomosexec.LockSubstrate(repoRoot)
		}

		// Marshal updated state into formatted JSON byte stream
		pbytes, err := json.MarshalIndent(state, "", "  ")
		if err != nil {
			return err
		}

		// Temporarily unlock phase state permissions for writing
		_ = os.Chmod(phaseStatePath, 0600)
		if err := os.WriteFile(phaseStatePath, pbytes, 0440); err != nil {
			return err
		}
		// Re-seal phase state file permissions to 0440 read-only
		_ = os.Chmod(phaseStatePath, 0440)

		// Calculate and persist SHA-256 state signature for Data Integrity Gate
		hash := task.CalculatePhaseStateHash(pbytes)
		_ = task.PersistPhaseStateHash(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), hash)
		_ = task.UpdateWorkspaceStateHash(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())

		fmt.Printf("Successfully approved task %s (PlanApproved: %s, CommitApproved: %s) and signed state\n", state.TaskId, state.PlanApproved, state.CommitApproved)
		return nil
	},
}

// Register taskApproveCmd subcommand with parent taskCmd registry
func init() {
	taskCmd.AddCommand(taskApproveCmd)
}
