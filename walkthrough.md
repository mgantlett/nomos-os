# [Task NOM-134] Task Walkthrough

## Impact List:
- Removed `--no-verify` from `git push` in `PerformGitFlowMerge`.
- Enforced strict root git workspace check before spinning up new `nomos task start` worktrees, resolving a vulnerability where dirty roots bleed into the orchestrator.
- Removed internal `go.work` generation from `scaffoldTaskWorktree` to adhere to the root-based `gopls` architecture introduced in `NOM-132`.

## Architectural Context:
- The `--no-verify` flag was temporarily introduced to bypass a failure in `CrossRepoWorktreeGate`. Now that the gate successfully skips the primary active worktree, this bypass is safely removed.
- Cross-repo dependencies are now exclusively managed by a root `go.work` file. Therefore, all generated worktrees must remain completely free of `.nomos/tmp` and `go.work` internal artifacts to avoid confusing `gopls` or `go env GOWORK`. 

## Resolution Details:
- Purged internal `go.work` initialization code from `task_helpers.go`.
- Strictly enforced `!isGitTreeClean` without `--force` override.
- Removed `pushTargetCmd` bypass (`--no-verify`) from `merge.go`.

## Verification Instructions
- All DoD gates pass.
- `nomos task sync` successfully rebases without bypassing hooks.
