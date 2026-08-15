/*
Package cmd provides CLI commands for the Nomos agentic framework.
This file implements the `task transition` command, which allows manual state
overrides in the absence of the Cockpit UI.
*/
package cmd

import (
	"fmt"
	"os"
	"strings"

	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/spf13/cobra"
)

// taskTransitionCmd updates the active phase state file to restrict or permit modifications.
// It directly mutates the `.phase_state.json` file on disk, bypassing the Cockpit HTTP API.
// This is typically used by internal hooks, the CLI fallback, or developers managing state manually.
var taskTransitionCmd = &cobra.Command{
	Use:   "transition [phase]",
	Short: "Transition the active workspace phase (PLAN, EDIT, REVIEW)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Convert input string argument to WorkspacePhase typed enum
		phase := statepkg.WorkspacePhase(strings.ToUpper(args[0]))
		if phase != statepkg.PhasePlan && phase != statepkg.PhaseEdit && phase != statepkg.PhaseReview {
			return fmt.Errorf("invalid phase: %s. Must be PLAN, EDIT, or REVIEW", phase)
		}

		// Discover active working directory and resolve repository root
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		repoRoot := findRepoRoot(wd)

		// Enforce Tier 2 agent restrictions: sub-agents cannot manually transition phase state
		if pState, err := task.GetPhaseState(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }()); err == nil && pState.AgentTier == statepkg.Tier2 {
			return fmt.Errorf("Tier 2 atomic rigidity violation: agents are explicitly forbidden from manually transitioning phase")
		}

		// Execute phase state transition and trigger phase transition hooks
		return task.TransitionPhase(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), phase)
	},
}

// Register taskTransitionCmd subcommand with taskCmd registry
func init() {
	taskCmd.AddCommand(taskTransitionCmd)
}
