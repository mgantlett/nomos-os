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
	root := ctx.RepoRoot
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

	if strings.TrimSpace(out) == "" {
		res.Passed = false
		res.Error = fmt.Errorf("mutation proof failed: workspace is in EDIT phase but git working tree is completely clean. You must make code changes before verification")
		return res, nil
	}

	res.Message = "Workspace contains active code mutations"
	return res, nil
}
