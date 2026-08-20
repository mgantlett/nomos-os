package verify

import (
	"fmt"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

// runExplorerSandboxGuard ensures that nomos verify is never run from inside the .explorer read-only sparse worktree.
func runExplorerSandboxGuard(ctx *workspace.WorkspaceContext) (StageResult, error) {
	root := ctx.PrimaryWorktree
	res := StageResult{Name: "Explorer Sandbox Guard", Passed: true}

	if strings.Contains(root, ".explorer") {
		res.Passed = false
		res.Error = fmt.Errorf("FATAL: Unauthorized mutation context. You are attempting to run verification inside the read-only '.explorer' sparse-checkout root. You MUST use 'nomos task start' to scaffold a transient '-NOM-' cross-repo worktree before making code edits")
		return res, nil
	}
	
	// If the worktree does not contain "-NOM-", it's the bare root checkout.
	// We should also reject naked sparse root edits.
	if !strings.Contains(root, "-NOM-") {
		res.Passed = false
		res.Error = fmt.Errorf("FATAL: Unauthorized mutation context. You are attempting to run verification from the sparse repository root. You MUST use 'nomos task start' to scaffold a transient '-NOM-' cross-repo worktree before making code edits")
		return res, nil
	}

	res.Message = "Workspace is correctly isolated within a transient task worktree"
	return res, nil
}
