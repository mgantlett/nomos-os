/*
Package verify provides strict verification gates for the Nomos workspace.
This file implements the Definition of Ready (DoR) checks which assert
that the active workspace meets prerequisite conditions before execution logic begins.
*/
package verify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-os/src/nomos/modules/schema"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
)

// VerifyDoR strictly enforces the Definition of Ready constraints.
// It ensures that an active task ID is set, the implementation plan is approved,
// and the workspace phase is transitioned to EDIT or REVIEW mode.
func VerifyDoR(root string) error {
	phaseStatePath := config.PhaseStatePath(root)
	content, err := os.ReadFile(phaseStatePath)
	if err != nil {
		return fmt.Errorf("failed to read phase state: %w", err)
	}

	var state task.PhaseState
	if err := json.Unmarshal(content, &state); err != nil {
		return fmt.Errorf("failed to parse phase state: %w", err)
	}

	// 1. Enforce active task ID is present
	if state.TaskId == "" {
		return fmt.Errorf("definition of Ready (DoR) check failed: no active task ID set in workspace")
	}

	// 2. Enforce implementation spec plan is approved
	if state.PlanApproved != "true" {
		return fmt.Errorf("definition of Ready (DoR) check failed: implementation plan for task %s has not been approved by the PO", state.TaskId)
	}

	// 2.5. Enforce deep-review checklist step for AI agents
	if err := verifyAgentDeepReview(root, state); err != nil {
		return err
	}

	// 3. Enforce current phase is transitioned to EDIT or REVIEW
	if state.CurrentPhase != statepkg.PhaseEdit && state.CurrentPhase != statepkg.PhaseReview {
		return fmt.Errorf("definition of Ready (DoR) check failed: workspace is in %s phase, must be transitioned to EDIT or REVIEW to work on task %s", state.CurrentPhase, state.TaskId)
	}

	return nil
}

// verifyAgentDeepReview ensures that AI agents have completed their
// mandatory deep review workflow before advancing.
func verifyAgentDeepReview(root string, state task.PhaseState) error {
	if state.Agent != "antigravity" && state.Agent != "aider" && !strings.HasPrefix(state.Agent, "swarm:") {
		return nil
	}
	taskMdPath := filepath.Join(config.GlobalDataDir(root), "tmp", "task.md")
	content, err := os.ReadFile(taskMdPath)
	if err != nil {
		return nil
	}
	if strings.Contains(string(content), schema.DeepReviewChecklistItem) {
		return fmt.Errorf("definition of Ready (DoR) check failed: the deep-review checklist step has not been completed. Please run /deep-review on the active task and check it off")
	}
	return nil
}
