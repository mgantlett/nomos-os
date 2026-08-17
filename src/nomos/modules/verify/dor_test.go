package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

func TestVerifyDoR(t *testing.T) {
	tests := []struct {
		name          string
		phaseState    string
		expectError   bool
		errorContains string
	}{
		{
			name: "Success - EDIT phase and approved",
			phaseState: `{
				"task_id": "23",
				"plan_approved": "true",
				"current_phase": "EDIT"
			}`,
			expectError: false,
		},
		{
			name: "Success - REVIEW phase and approved",
			phaseState: `{
				"task_id": "23",
				"plan_approved": "true",
				"current_phase": "REVIEW"
			}`,
			expectError: false,
		},
		{
			name: "Failure - missing task_id",
			phaseState: `{
				"task_id": "",
				"plan_approved": "true",
				"current_phase": "EDIT"
			}`,
			expectError:   true,
			errorContains: "no active task ID set",
		},
		{
			name: "Failure - plan not approved",
			phaseState: `{
				"task_id": "23",
				"plan_approved": "false",
				"current_phase": "EDIT"
			}`,
			expectError:   true,
			errorContains: "has not been approved",
		},
		{
			name: "Failure - bad phase PLAN",
			phaseState: `{
				"task_id": "23",
				"plan_approved": "true",
				"current_phase": "PLAN"
			}`,
			expectError:   true,
			errorContains: "must be transitioned to EDIT or REVIEW",
		},
		{
			name: "Failure - bad phase IDLE",
			phaseState: `{
				"task_id": "23",
				"plan_approved": "true",
				"current_phase": "IDLE"
			}`,
			expectError:   true,
			errorContains: "must be transitioned to EDIT or REVIEW",
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

			phaseStatePath := workspace.MustNewContext(tempDir).NomosStatePath(".phase_state.json")
			if err := os.MkdirAll(filepath.Dir(phaseStatePath), 0755); err != nil {
				t.Fatalf("failed to create state dir: %v", err)
			}

			if err := os.WriteFile(phaseStatePath, []byte(tc.phaseState), 0644); err != nil {
				t.Fatalf("failed to write phase state: %v", err)
			}

			err = VerifyDoR(tempDir)
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tc.errorContains)
				} else if !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("expected error containing %q, got: %v", tc.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			}
		})
	}
}
