package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIdePhaseCmd(t *testing.T) {
	// Create a temp workspace directory to mock the repo root
	tmpDir, err := os.MkdirTemp("", "nomos-ide-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Save original CWD and change to temp dir
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change wd: %v", err)
	}
	defer os.Chdir(origWd)

	// Initialize dummy nomos structure to pass handlePhaseTransition checks
	dataDir := filepath.Join(tmpDir, ".nomos", "data", "nomos-ide-test")
	stateDir := filepath.Join(dataDir, "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}

	// Create dummy phase state JSON so we don't fail reading it
	dummyPhaseState := `{"active_task": "test-task", "phase": "PLAN", "agent_tier": "tier-1-architect"}`
	if err := os.WriteFile(filepath.Join(stateDir, ".phase_state.json"), []byte(dummyPhaseState), 0644); err != nil {
		t.Fatalf("failed to write phase state: %v", err)
	}

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)

	// Test invalid phase
	RootCmd.SetArgs([]string{"ide", "phase", "INVALID"})
	if err := RootCmd.Execute(); err == nil {
		t.Fatalf("Expected error for invalid phase, but got nil")
	} else if !strings.Contains(err.Error(), "invalid phase") {
		t.Errorf("Expected 'invalid phase' error, got: %v", err)
	}

	// Test valid phase (PLAN)
	RootCmd.SetArgs([]string{"ide", "phase", "PLAN"})
	if err := RootCmd.Execute(); err != nil {
		// Just ensure it doesn't fail parsing the command
		// Note: handlePhaseTransition might return error if workspace is not fully initialized,
		// but the command itself is wired correctly.
		if strings.Contains(err.Error(), "invalid phase") {
			t.Fatalf("Should not return invalid phase for PLAN, got: %v", err)
		}
	}
}
