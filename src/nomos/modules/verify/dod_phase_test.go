package verify

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
)

func TestRunPhaseDisciplineCheck(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nomos_phase_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set up a mock git repo
	runGitCmd := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		_ = cmd.Run()
	}
	runGitCmd(tempDir, "init")
	runGitCmd(tempDir, "config", "user.name", "Test User")
	runGitCmd(tempDir, "config", "user.email", "test@example.com")

	// Ensure there is at least one commit so git diff HEAD works
	initialFile := filepath.Join(tempDir, "initial.go")
	if err := os.WriteFile(initialFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to write initial file: %v", err)
	}
	runGitCmd(tempDir, "add", ".")
	runGitCmd(tempDir, "commit", "-m", "initial")

	agentDir := filepath.Join(tempDir, ".agent")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("failed to create .agent dir: %v", err)
	}

	writePhaseState := func(phase, approved string) {
		state := map[string]string{
			"current_phase":   phase,
			"commit_approved": approved,
		}
		data, err := json.Marshal(state)
		if err != nil {
			t.Fatalf("failed to marshal state: %v", err)
		}
		_ = os.MkdirAll(config.StateDir(tempDir), 0755)
		statePath := config.PhaseStatePath(tempDir)
		if err := os.WriteFile(statePath, data, 0644); err != nil {
			t.Fatalf("failed to write phase state file: %v", err)
		}
		// Persist signature to mock sqlite db in tempDir
		hash := task.CalculatePhaseStateHash(data)
		if err := task.PersistPhaseStateHash(tempDir, hash); err != nil {
			t.Fatalf("failed to persist phase state signature: %v", err)
		}
	}

	// Helper to introduce a code modification
	modifyCodeFile := func() {
		codeFile := filepath.Join(tempDir, "code.go")
		if err := os.WriteFile(codeFile, []byte("package main\n\nfunc main() {}"), 0644); err != nil {
			t.Fatalf("failed to write code file: %v", err)
		}
	}

	// Helper to clean up code modifications
	cleanCodeFile := func() {
		codeFile := filepath.Join(tempDir, "code.go")
		_ = os.Remove(codeFile)
	}

	// 1. EDIT phase (should pass even with modified code files)
	writePhaseState("EDIT", "false")
	modifyCodeFile()
	res, err := runPhaseDisciplineCheck(tempDir)
	if err != nil {
		t.Errorf("expected no error in EDIT phase, got: %v", err)
	}
	if !res.Passed {
		t.Errorf("expected EDIT phase discipline to pass, but failed: %v", res.Error)
	}
	cleanCodeFile()

	// 2. PLAN phase with no modified files (should pass)
	writePhaseState("PLAN", "false")
	res, err = runPhaseDisciplineCheck(tempDir)
	if err != nil {
		t.Errorf("expected no error in PLAN phase, got: %v", err)
	}
	if !res.Passed {
		t.Errorf("expected PLAN phase discipline to pass, but failed: %v", res.Error)
	}

	// 3. PLAN phase with modified code files (should fail)
	modifyCodeFile()
	res, err = runPhaseDisciplineCheck(tempDir)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if res.Passed {
		t.Errorf("expected PLAN phase discipline to fail with modified files, but passed")
	}
	cleanCodeFile()

	// 4. REVIEW phase with no modified files (should pass)
	writePhaseState("REVIEW", "false")
	res, err = runPhaseDisciplineCheck(tempDir)
	if err != nil {
		t.Errorf("expected no error in REVIEW phase, got: %v", err)
	}
	if !res.Passed {
		t.Errorf("expected REVIEW phase discipline to pass with no modified files, but failed: %v", res.Error)
	}

	// 5. REVIEW phase with modified code files, commit not approved (should fail)
	modifyCodeFile()
	res, err = runPhaseDisciplineCheck(tempDir)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if res.Passed {
		t.Errorf("expected REVIEW phase discipline to fail with unapproved commit, but passed")
	}

	// 6. REVIEW phase with modified code files, commit approved, NOT in git hook (should fail)
	writePhaseState("REVIEW", "true")
	os.Unsetenv("NOMOS_IN_GIT_HOOK")
	res, err = runPhaseDisciplineCheck(tempDir)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if res.Passed {
		t.Errorf("expected REVIEW phase discipline to fail when not in git hook, but passed")
	}

	// 7. REVIEW phase with modified code files, commit approved, in git hook (should pass)
	os.Setenv("NOMOS_IN_GIT_HOOK", "1")
	defer os.Unsetenv("NOMOS_IN_GIT_HOOK")
	res, err = runPhaseDisciplineCheck(tempDir)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if !res.Passed {
		t.Errorf("expected REVIEW phase discipline to pass in git hook, but failed: %v", res.Error)
	}
	cleanCodeFile()

	// 8. Mismatched database hash (tampering simulation - should fail)
	writePhaseState("EDIT", "false")
	// Manually overwrite phase state json without updating DB signature
	tamperedData := []byte(`{"current_phase": "PLAN", "commit_approved": "false", "tampered": "true"}`)
	_ = os.MkdirAll(config.StateDir(tempDir), 0755)
	statePath := config.PhaseStatePath(tempDir)
	if err := os.WriteFile(statePath, tamperedData, 0644); err != nil {
		t.Fatalf("failed to write tampered phase state file: %v", err)
	}
	res, err = runPhaseDisciplineCheck(tempDir)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
	if res.Passed {
		t.Errorf("expected phase discipline to fail with tampered/mismatched hash, but passed")
	}
	if res.Error == nil || !strings.Contains(res.Error.Error(), "Phase State Tampering") {
		t.Errorf("expected tampering error message, got: %v", res.Error)
	}
}

func TestIsMetadataOnly(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nomos_metadata_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	runGitCmd := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		_ = cmd.Run()
	}
	runGitCmd(tempDir, "init")

	// 1. No modified files (should return true)
	metaOnly, err := IsMetadataOnly(tempDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !metaOnly {
		t.Errorf("expected empty modifications to be classified as metadata only")
	}

	// 2. Only planning/metadata files modified (should return true)
	planningFile := filepath.Join(tempDir, "walkthrough.md")
	if err := os.WriteFile(planningFile, []byte("# Walkthrough"), 0644); err != nil {
		t.Fatalf("failed to write planning file: %v", err)
	}
	metaOnly, err = IsMetadataOnly(tempDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !metaOnly {
		t.Errorf("expected walkthrough.md modification to be classified as metadata only")
	}

	// 3. Source code file modified (should return false)
	codeFile := filepath.Join(tempDir, "code.go")
	if err := os.WriteFile(codeFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to write code file: %v", err)
	}
	metaOnly, err = IsMetadataOnly(tempDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metaOnly {
		t.Errorf("expected code.go modification NOT to be classified as metadata only")
	}
}

func TestCheckPOCommitApprovalIdle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nomos_idle_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	runGitCmd := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		_ = cmd.Run()
	}
	runGitCmd(tempDir, "init")

	// Setup initial commit so git diff HEAD works
	initialFile := filepath.Join(tempDir, "initial.go")
	_ = os.WriteFile(initialFile, []byte("package main"), 0644)
	runGitCmd(tempDir, "add", ".")
	runGitCmd(tempDir, "commit", "-m", "initial")

	agentDir := filepath.Join(tempDir, ".agent")
	_ = os.MkdirAll(agentDir, 0755)

	// Write IDLE phase state
	state := map[string]string{
		"current_phase": "IDLE",
	}
	data, _ := json.Marshal(state)
	_ = os.MkdirAll(config.StateDir(tempDir), 0755)
	_ = os.WriteFile(config.PhaseStatePath(tempDir), data, 0644)

	// Create planning metadata file
	planningFile := filepath.Join(tempDir, "walkthrough.md")
	_ = os.WriteFile(planningFile, []byte("# Walkthrough"), 0644)

	// In IDLE phase with only metadata changes, CheckPOCommitApproval should pass (return nil)
	err = CheckPOCommitApproval(tempDir)
	if err != nil {
		t.Errorf("expected CheckPOCommitApproval to pass for IDLE + metadata changes, got: %v", err)
	}
}
