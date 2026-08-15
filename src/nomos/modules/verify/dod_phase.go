package verify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
)

// runPhaseDisciplineCheck validates that no source code files are modified during PLAN or REVIEW phases,
// except during git pre-commit hook executions when the human PO has approved the active commit.
func runPhaseDisciplineCheck(r string) (StageResult, error) {
	res := StageResult{Name: "Phase Discipline Check", Passed: true}

	// Retrieve the active phase state file if it exists.
	phaseStatePath := config.PhaseStatePath(r)
	data, err := os.ReadFile(phaseStatePath)
	if err != nil {
		// Bypass phase validation if state file is not present.
		return res, nil
	}

	// Verify that the current phase state hash matches the signature persisted in SQLite database.
	if err := verifyPhaseStateHash(r, data); err != nil {
		res.Passed = false
		res.Error = err
		return res, nil
	}

	var state localPhaseState
	if err := json.Unmarshal(data, &state); err != nil {
		return res, nil
	}

	if isModificationPermitted(state) {
		res.Message = fmt.Sprintf("Workspace is in %s phase (commit_approved=%s); code modifications permitted.", state.CurrentPhase, state.CommitApproved)
		return res, nil
	}

	// Audit all modified and untracked repository files.
	forbidden, err := checkForbiddenCodeModifications(r)
	if err != nil {
		return res, nil
	}

	// Fail verification if code modifications were made during non-permitted phases.
	if len(forbidden) > 0 {
		res.Passed = false
		res.Error = fmt.Errorf("substrate lock violation: workspace is in '%s' phase, but code modifications were detected in:\n - %s\n💡 Guidance: Workspace substrate is locked (read-only). Run 'bin/nomos task transition EDIT' to unlock files.", state.CurrentPhase, strings.Join(forbidden, "\n - "))
	} else {

		res.Message = fmt.Sprintf("Workspace is in '%s' phase; substrate lock active (no code modifications detected).", state.CurrentPhase)
	}

	return res, nil
}

type localPhaseState struct {
	CurrentPhase   string `json:"current_phase"`
	CommitApproved string `json:"commit_approved"`
}

func isModificationPermitted(state localPhaseState) bool {
	if state.CurrentPhase == string(statepkg.PhaseEdit) {
		return true
	}
	return state.CurrentPhase == string(statepkg.PhaseReview) && state.CommitApproved == "true" && os.Getenv("NOMOS_IN_GIT_HOOK") == "1"
}

func verifyPhaseStateHash(r string, data []byte) error {
	currentHash := task.CalculatePhaseStateHash(data)
	persistedHash, err := task.GetPersistedPhaseStateHash(r)
	if err != nil {
		return fmt.Errorf("Phase State Tampering Check Failed: unable to query state signature: %w", err)
	}
	if persistedHash != currentHash {
		return fmt.Errorf("Phase State Tampering: current phase state file hash does not match signature in state database. Manual modifications are prohibited.")
	}
	return nil
}

func checkForbiddenCodeModifications(r string) ([]string, error) {
	modified, err := GetModifiedFiles(r)
	if err != nil {
		return nil, err
	}
	var forbidden []string
	for m := range modified {
		if !isPlanningFile(m) {
			forbidden = append(forbidden, m)
		}
	}
	return forbidden, nil
}

// isPlanningFile checks if a modified file is an agent specification, log, doc, or wiki/lessons log.
func isPlanningFile(m string) bool {
	m = filepath.ToSlash(m)
	return config.IsInternalAgentDir(m) ||
		config.IsInternalNomosDir(m) ||
		strings.HasSuffix(m, ".md") ||
		m == config.WalkthroughFileName ||
		m == "implementation_plan.md" ||
		m == "quality_debt.json"
}

// runDataIntegrityCheck executes the Data Integrity Gate.
// This gate ensures that no JSON state files in the workspace have been manually altered.
func runDataIntegrityCheck(r string) (StageResult, error) {
	res := StageResult{Name: "Data Integrity Gate", Passed: true}

	// Calculate the live cryptographic hash of all local state files.
	currentHash, err := task.CalculateWorkspaceStateHash(r)
	if err != nil {
		res.Passed = false
		res.Error = fmt.Errorf("failed to calculate workspace state hash: %w", err)
		return res, nil
	}

	// Retrieve the expected signature from the tmp lockfile.
	persistedHash, err := task.GetPersistedWorkspaceStateHash(r)
	if err != nil {
		res.Passed = false
		res.Error = fmt.Errorf("failed to retrieve workspace state signature: %w", err)
		return res, nil
	}

	// Fallback/Bootstrap logic for uninitialized projects
	if persistedHash == "" {
		if err := task.PersistWorkspaceStateHash(r, currentHash); err != nil {
			res.Passed = false
			res.Error = fmt.Errorf("failed to bootstrap workspace state signature: %w", err)
			return res, nil
		}
		res.Message = "Workspace state signature initialized."
		return res, nil
	}

	if persistedHash != currentHash {
		res.Passed = false
		res.Error = fmt.Errorf("Data Integrity Violation: The current hash of your JSON state files does not match the deterministic .workspace_state.hash signature.\nThis indicates an autonomous agent or script modified the <repoRoot>/.nomos/data/ JSON files directly instead of using the CLI mutations.\n💡 Guidance: Use 'bin/nomos task edit' or transition commands to modify state.")
		return res, nil
	}

	res.Message = "Workspace data signature is pristine."
	return res, nil
}
