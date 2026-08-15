package verify

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrchestrateSwarm(t *testing.T) {
	// Create mock git target repo
	tmpDir, err := os.MkdirTemp("", "orchestrate_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	execCmd := func(dir string, name string, args ...string) error {
		if name == "git" {
			if len(args) > 0 && args[0] == "commit" {
				args = append(args, "--no-verify")
			}
		}
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		if name == "git" {
			cmd.Env = cleanGitEnv()
		}
		return cmd.Run()
	}

	if err := execCmd(tmpDir, "git", "init"); err != nil {
		t.Fatalf("git init failed: %v", err)
	}
	_ = execCmd(tmpDir, "git", "config", "user.name", "Test")
	_ = execCmd(tmpDir, "git", "config", "user.email", "test@example.com")
	_ = execCmd(tmpDir, "git", "config", "commit.gpgsign", "false")

	// Create initial file & commit to create base branch
	initialFile := filepath.Join(tmpDir, "readme.md")
	if err := os.WriteFile(initialFile, []byte("# Initial commit"), 0644); err != nil {
		t.Fatalf("failed to write initial file: %v", err)
	}
	if err := execCmd(tmpDir, "git", "add", "readme.md"); err != nil {
		t.Fatalf("git add failed: %v", err)
	}
	if err := execCmd(tmpDir, "git", "commit", "-m", "Initial commit"); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}
	// Create develop branch
	if err := execCmd(tmpDir, "git", "checkout", "-b", "develop"); err != nil {
		t.Fatalf("git checkout develop failed: %v", err)
	}

	// Create a mock executable script
	mockExe := filepath.Join(tmpDir, "mock_nomos")
	scriptContent := `#!/bin/sh
# Mock nomos executable
TASK_ID=$(basename "$(pwd)")
echo "Mock run for task $TASK_ID" > "task_output_$TASK_ID.txt"
git add "task_output_$TASK_ID.txt"
git commit -m "Mock task commit $TASK_ID"
exit 0
`
	if err := os.WriteFile(mockExe, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to write mock executable: %v", err)
	}
	nomosExecutableOverride = mockExe
	defer func() { nomosExecutableOverride = "" }()

	// Create a plan file
	plan := SwarmPlan{
		TargetRepo: tmpDir,
		BaseBranch: "develop",
		Subtasks: []SwarmSubtask{
			{ID: "test1", Prompt: "Task 1"},
			{ID: "test2", Prompt: "Task 2"},
		},
	}
	planBytes, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("failed to marshal plan: %v", err)
	}
	planFile := filepath.Join(tmpDir, "plan.json")
	if err := os.WriteFile(planFile, planBytes, 0644); err != nil {
		t.Fatalf("failed to write plan file: %v", err)
	}

	// Run OrchestrateSwarm
	if err := OrchestrateSwarm(planFile); err != nil {
		t.Fatalf("OrchestrateSwarm failed: %v", err)
	}

	// Verify that branches were merged
	out, err := runGitCommandWithOutput(tmpDir, "log", "--oneline")
	if err != nil {
		t.Fatalf("failed to get git log: %v", err)
	}
	logStr := string(out)
	if !strings.Contains(logStr, "Merge task test1") {
		t.Errorf("expected git log to contain merge task test1, got: %s", logStr)
	}
	if !strings.Contains(logStr, "Merge task test2") {
		t.Errorf("expected git log to contain merge task test2, got: %s", logStr)
	}
}
