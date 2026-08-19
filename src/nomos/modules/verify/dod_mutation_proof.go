package verify

import (
	"fmt"
	"strings"

	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
)

// runMutationProofGate ensures that if the workspace is in the EDIT phase,
// there must be a dirty working directory (code mutations exist).
// This explicitly blocks stochastic LLM agent hallucinations where they
// call run_verify without calling code modification tools.
func runMutationProofGate(ctx *workspace.WorkspaceContext) (StageResult, error) {
	root := ctx.PrimaryWorktree
	res := StageResult{Name: "Mutation Proof Gate", Passed: true}

	state, err := task.GetPhaseState(ctx)
	if err != nil {
		// If there is no active task state, skip the mutation proof gate
		res.Message = "Skipped (no active task state)"
		return res, nil
	}

	if state.CurrentPhase != statepkg.PhaseEdit {
		res.Message = fmt.Sprintf("Skipped (Phase is %s)", state.CurrentPhase)
		return res, nil
	}

	out, err := runGit(root, "status", "--porcelain")
	if err != nil {
		res.Passed = false
		res.Error = fmt.Errorf("failed to run git status: %w", err)
		return res, nil
	}

	var actualMutations int
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		
		// Filter out automatically generated swarm/escrow and state files
		if strings.Contains(line, "go.work") || 
		   strings.Contains(line, ".nomos-swarm-escrow.json") || 
		   strings.Contains(line, ".nomos/") ||
		   strings.Contains(line, "go.work.sum") {
			continue
		}
		
		actualMutations++
	}

	if actualMutations == 0 {
		res.Passed = false
		res.Error = fmt.Errorf("mutation proof failed: workspace is in EDIT phase but git working tree has no source code mutations. You must make code changes before verification")
		return res, nil
	}

	res.Message = "Workspace contains active code mutations"
	return res, nil
}
