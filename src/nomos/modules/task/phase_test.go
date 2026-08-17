package task

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

// TestTransitionPhaseAndHooks verifies phase state updates and hook execution.
func TestTransitionPhaseAndHooks(t *testing.T) {
	// Added test changes to satisfy TDD coverage check constraints.
	tempDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tempDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	agentDir := filepath.Join(tempDir, ".agent")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("failed to create .agent dir: %v", err)
	}

	// Write initial phase state
	nowStr := time.Now().Format(time.RFC3339)
	initialState := PhaseState{
		Agent:            "antigravity",
		AgentType:        "ide",
		TaskId:           "25",
		CurrentPhase:     statepkg.PhasePlan,
		PlanApproved:     "false",
		CommitApproved:   "false",
		PhaseEnteredAt:   nowStr,
		SessionStartedAt: nowStr,
		WaitingOnHuman:   "true",
	}
	initialBytes, _ := json.MarshalIndent(initialState, "", "  ")
	_ = os.MkdirAll(workspace.MustNewContext(tempDir).StateDir(), 0755)
	if err := os.WriteFile(workspace.MustNewContext(tempDir).NomosStatePath(".phase_state.json"), initialBytes, 0644); err != nil {
		t.Fatalf("failed to write initial phase state: %v", err)
	}

	// Setup hooks dir and a test hook
	hooksDir := filepath.Join(agentDir, "hooks", "phase")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("failed to create hooks dir: %v", err)
	}

	logFile := filepath.Join(tempDir, "hook_run.log")
	hookContent := fmt.Sprintf(`#!/usr/bin/env bash
echo "task=$NOMOS_ACTIVE_TASK phase=$NOMOS_CURRENT_PHASE" > "%s"
`, logFile)

	hookPath := filepath.Join(hooksDir, "on_edit.sh")
	if err := os.WriteFile(hookPath, []byte(hookContent), 0755); err != nil {
		t.Fatalf("failed to write hook file: %v", err)
	}

	// Run TransitionPhase
	err = TransitionPhase(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(tempDir); return c }(), statepkg.PhaseEdit)
	if err != nil {
		t.Fatalf("TransitionPhase failed: %v", err)
	}

	// 1. Verify JSON file changes
	data, err := os.ReadFile(workspace.MustNewContext(tempDir).NomosStatePath(".phase_state.json"))
	if err != nil {
		t.Fatalf("failed to read modified state: %v", err)
	}
	var state PhaseState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("failed to parse modified state: %v", err)
	}

	if state.CurrentPhase != statepkg.PhaseEdit {
		t.Errorf("expected current phase to be EDIT, got: %s", state.CurrentPhase)
	}
	if state.PrevPhase != "PLAN" {
		t.Errorf("expected prev phase to be PLAN, got: %s", state.PrevPhase)
	}
	if state.PlanApproved != "true" {
		t.Errorf("expected plan approved to be true, got: %s", state.PlanApproved)
	}

	// 2. Verify hook script execution and env vars
	logBytes, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("hook did not seem to run: failed to read log file: %v", err)
	}
	logContent := strings.TrimSpace(string(logBytes))
	expectedLog := "task=25 phase=EDIT"
	if logContent != expectedLog {
		t.Errorf("expected hook log %q, got: %q", expectedLog, logContent)
	}
}

func TestGetPhaseStateValidation(t *testing.T) {
	tests := []struct {
		name       string
		state      PhaseState
		expectPass bool
	}{
		{
			name: "Success - Valid state",
			state: PhaseState{
				CommitApproved:   "true",
				CurrentPhase:     statepkg.PhaseReview,
				PhaseEnteredAt:   "2026-07-07T00:10:00Z",
				PlanApproved:     "true",
				SessionStartedAt: "2026-07-07T00:00:00Z",
				WaitingOnHuman:   "false",
			},
			expectPass: true,
		},
		{
			name: "Failure - Invalid CurrentPhase",
			state: PhaseState{
				CommitApproved:   "true",
				CurrentPhase:     statepkg.WorkspacePhase("INVALID_PHASE"),
				PhaseEnteredAt:   "2026-07-07T00:10:00Z",
				PlanApproved:     "true",
				SessionStartedAt: "2026-07-07T00:00:00Z",
				WaitingOnHuman:   "false",
			},
			expectPass: false,
		},
		{
			name: "Failure - Missing required CommitApproved",
			state: PhaseState{
				CurrentPhase:     statepkg.PhaseEdit,
				PhaseEnteredAt:   "2026-07-07T00:10:00Z",
				PlanApproved:     "true",
				SessionStartedAt: "2026-07-07T00:00:00Z",
				WaitingOnHuman:   "false",
			},
			expectPass: false,
		},
		{
			name: "Failure - Invalid PhaseEnteredAt timestamp format",
			state: PhaseState{
				CommitApproved:   "true",
				CurrentPhase:     statepkg.PhaseEdit,
				PhaseEnteredAt:   "2026-07-07 00:10:00", // not RFC3339
				PlanApproved:     "true",
				SessionStartedAt: "2026-07-07T00:00:00Z",
				WaitingOnHuman:   "false",
			},
			expectPass: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tempDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tempDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

			agentDir := filepath.Join(tempDir, ".agent")
			if err := os.MkdirAll(agentDir, 0755); err != nil {
				t.Fatalf("failed to create .agent dir: %v", err)
			}

			phaseBytes, _ := json.Marshal(tc.state)
			_ = os.MkdirAll(workspace.MustNewContext(tempDir).StateDir(), 0755)
			if err := os.WriteFile(workspace.MustNewContext(tempDir).NomosStatePath(".phase_state.json"), phaseBytes, 0644); err != nil {
				t.Fatalf("failed to write test phase state: %v", err)
			}

			_, err = GetPhaseState(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(tempDir); return c }())
			if tc.expectPass {
				if err != nil {
					t.Errorf("expected validation to pass, but failed: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected validation to fail, but it passed")
				}
			}
		})
	}
}
