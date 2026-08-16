package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

func TestVerifyDoDCommitApproval(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nomos_dod_po_test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	agentDir := filepath.Join(tempDir, ".agent")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("failed to create .agent dir: %v", err)
	}
	agentRulesDir := filepath.Join(tempDir, ".agent", "rules")
	if err := os.MkdirAll(agentRulesDir, 0755); err != nil {
		t.Fatalf("failed to create agent rules dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentRulesDir, "AGENT.md"), []byte("# AGENT\n"), 0644); err != nil {
		t.Fatalf("failed to write AGENT.md: %v", err)
	}

	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}
	mockNomosPath := filepath.Join(binDir, "nomos")
	mockScript := `#!/bin/sh
if [ "$1" = "schema" ] && [ "$2" = "cli" ]; then
	echo '{"name": "nomos", "subcommands": {}}'
	exit 0
fi
exit 1
`
	if err := os.WriteFile(mockNomosPath, []byte(mockScript), 0755); err != nil {
		t.Fatalf("failed to write mock nomos: %v", err)
	}

	_ = os.MkdirAll(workspace.MustNewContext(tempDir).StateDir(), 0755)
	phaseStatePath := workspace.MustNewContext(tempDir).NomosStatePath(".phase_state.json")

	runGit := func(dir string, args ...string) {
		cmd := execGit(dir, args...)
		_ = cmd.Run()
	}
	runGit(tempDir, "init")

	tests := []struct {
		name        string
		phaseState  string
		expectError bool
		errContains string
	}{
		{
			name: "Success - REVIEW phase and commit_approved true",
			phaseState: `{
				"agent": "antigravity",
				"current_phase": "REVIEW",
				"commit_approved": "true",
				"task_id": "123"
			}`,
			expectError: false,
		},
		{
			name: "Success - Human check (empty agent)",
			phaseState: `{
				"agent": "",
				"current_phase": "EDIT",
				"commit_approved": "false"
			}`,
			expectError: false,
		},
		{
			name: "Success - Human check (os-automaton)",
			phaseState: `{
				"agent": "os-automaton",
				"current_phase": "EDIT",
				"commit_approved": "false"
			}`,
			expectError: false,
		},
		{
			name: "Success - agent active and phase EDIT (relies on PhaseToken)",
			phaseState: `{
				"agent": "antigravity",
				"current_phase": "EDIT",
				"commit_approved": "false"
			}`,
			expectError: false,
		},
		{
			name: "Failure - agent active, phase REVIEW, commit_approved false",
			phaseState: `{
				"agent": "antigravity",
				"current_phase": "REVIEW",
				"commit_approved": "false"
			}`,
			expectError: true,
			errContains: "walkthrough and diff have not been approved",
		},
		{
			name: "Failure - agent active, phase REVIEW, commit_approved missing",
			phaseState: `{
				"agent": "antigravity",
				"current_phase": "REVIEW"
			}`,
			expectError: true,
			errContains: "walkthrough and diff have not been approved",
		},
	}

	os.Setenv("NOMOS_IN_GIT_HOOK", "1")
	defer os.Unsetenv("NOMOS_IN_GIT_HOOK")

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_ = os.RemoveAll(filepath.Join(agentDir, "specs"))
			if tc.name == "Success - REVIEW phase and commit_approved true" {
				walkthroughsDir := workspace.MustNewContext(tempDir).DataPath("walkthroughs")
				os.MkdirAll(walkthroughsDir, 0755)
				walkthroughPath := filepath.Join(walkthroughsDir, "123.md")
				_ = os.WriteFile(walkthroughPath, []byte("# Walkthrough\n"), 0644)

				pastTime := time.Now().Add(-1 * time.Hour)
				_ = os.Chtimes(walkthroughPath, pastTime, pastTime)
			}

			if err := os.WriteFile(phaseStatePath, []byte(tc.phaseState), 0644); err != nil {
				t.Fatalf("failed to write phase state: %v", err)
			}

			if tc.name == "Success - REVIEW phase and commit_approved true" {
				pastTime := time.Now().Add(-1 * time.Hour)
				_ = os.Chtimes(phaseStatePath, pastTime, pastTime)
			}

			err := CheckPOCommitApproval(tempDir)
			assertErrorContains(t, err, tc.expectError, tc.errContains)
		})
	}
}

func assertErrorContains(t *testing.T, err error, expectError bool, errContains string) {
	if expectError {
		if err == nil {
			t.Errorf("expected error containing %q, got nil", errContains)
		} else if !strings.Contains(err.Error(), errContains) {
			t.Errorf("expected error containing %q, got: %v", errContains, err)
		}
	} else {
		if err != nil {
			t.Errorf("expected no error, got: %v", err)
		}
	}
}

func TestVerifyDoDWalkthroughGate(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nomos-dod-walkthrough-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	agentDir := filepath.Join(tempDir, ".agent")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("failed to create .agent dir: %v", err)
	}

	// 1. Write phase state in REVIEW phase but commit_approved = true
	state := `{
		"agent": "antigravity",
		"current_phase": "REVIEW",
		"commit_approved": "true",
		"task_id": "22"
	}`
	_ = os.MkdirAll(workspace.MustNewContext(tempDir).StateDir(), 0755)
	phaseStatePath := workspace.MustNewContext(tempDir).NomosStatePath(".phase_state.json")
	if err := os.WriteFile(phaseStatePath, []byte(state), 0644); err != nil {
		t.Fatalf("failed to write phase state: %v", err)
	}

	os.Setenv("NOMOS_IN_GIT_HOOK", "1")
	defer os.Unsetenv("NOMOS_IN_GIT_HOOK")

	// 2. CheckPOCommitApproval should fail because walkthrough.md is missing
	err = CheckPOCommitApproval(tempDir)
	if err == nil {
		t.Errorf("expected PO commit check to fail for missing walkthrough in REVIEW phase, but it succeeded")
	}

	// 3. Create specs folder and walkthrough.md
	walkthroughsDir := workspace.MustNewContext(tempDir).DataPath("walkthroughs")
	if err := os.MkdirAll(walkthroughsDir, 0755); err != nil {
		t.Fatalf("failed to create walkthroughs dir: %v", err)
	}
	walkthroughPath := filepath.Join(walkthroughsDir, "22.md")
	if err := os.WriteFile(walkthroughPath, []byte("# Walkthrough\n"), 0644); err != nil {
		t.Fatalf("failed to write walkthrough: %v", err)
	}

	// 3a. CheckPOCommitApproval should succeed now without cognitive firewall delay
	err = CheckPOCommitApproval(tempDir)
	if err != nil {
		t.Errorf("expected PO commit check to succeed, got: %v", err)
	}

	// 4. CheckPOCommitApproval should now pass
	err = CheckPOCommitApproval(tempDir)
	if err != nil {
		t.Errorf("expected PO commit check to pass when walkthrough is backdated, got error: %v", err)
	}
}
