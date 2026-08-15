package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
)

type mockTracker struct {
	task.Tracker
	created bool
	title   string
	body    string
	labels  []string
}

func (m *mockTracker) Cancel(ctx context.Context, key string, comment string) error { return nil }
func (m *mockTracker) Transition(ctx context.Context, key string, status task.TaskStatus) error {
	return nil
}
func (m *mockTracker) Create(ctx context.Context, title string, body string, labels []string, parentKey string, project string, taskType task.TaskType, isSpike bool, initialStatus task.TaskStatus) (string, error) {
	m.created = true
	m.title = title
	m.body = body
	m.labels = labels
	return "TEST-999", nil
}
func (m *mockTracker) Edit(ctx context.Context, key string, title *string, body *string, labels []string, contextBurden *int, logicDepth *int, blockedBy []string, sequence *int, project *string) error {
	return nil
}
func (m *mockTracker) List(ctx context.Context) ([]task.Task, error) {
	return nil, nil
}

func TestTaskCommandsRegistered(t *testing.T) {
	// Verify task subcommands are registered in Cobra
	foundReset := false
	foundPark := false
	for _, sub := range taskCmd.Commands() {
		if sub.Name() == "reset" {
			foundReset = true
		}
		if sub.Name() == "park" {
			foundPark = true
		}
	}

	if !foundReset {
		t.Error("expected reset subcommand to be registered under taskCmd")
	}
	if !foundPark {
		t.Error("expected park subcommand to be registered under taskCmd")
	}
}

func TestTaskCreatePriorityValidation(t *testing.T) {
	mock := &mockTracker{}
	task.NewTrackerOverride = func(cfg *task.Config) (task.Tracker, error) {
		return mock, nil
	}
	defer func() { task.NewTrackerOverride = nil }()

	cmd := taskCreateCmd

	tmpDir := t.TempDir()
	validBody := "## 📝 Execution Unit / Description\nDesc\n## ✅ Acceptance Criteria\n- [ ] crit\n## 🛠️ Technical Notes\n- note\n## 🛡️ Rigor & Verification Boundary\n- **Target Files:**\n  - `[MODIFY] f`\n"
	tmpFile1 := filepath.Join(tmpDir, "story1.md")
	err := os.WriteFile(tmpFile1, []byte(validBody), 0644)
	if err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	// Test Case 1: Missing priority label should fail
	cmd.SetArgs([]string{"Test Task 1"})
	cmd.Flags().Set("file", tmpFile1)
	cmd.Flags().Set("label", "layer:backend,cli:high,blast:low")
	err = cmd.RunE(cmd, []string{"Test Task 1"})
	if err == nil {
		t.Error("expected error due to missing priority label, but got nil")
	}

	// Test Case 2: Priority label present in --label should succeed
	mock.created = false
	cmd.Flags().Set("label", "priority:high,layer:backend,cli:high,blast:low")
	err = cmd.RunE(cmd, []string{"Test Task 1"})
	if err != nil {
		t.Errorf("expected success with priority label in flags, but got error: %v", err)
	}
	if !mock.created {
		t.Error("expected tracker.Create to be called")
	}

	// Test Case 3: Priority parsed from markdown body should succeed
	mock.created = false
	cmd.Flags().Set("label", "layer:backend,cli:high,blast:low")

	tmpFile := filepath.Join(tmpDir, "story.md")
	bodyContent := validBody + "- **Priority Tag:** priority:critical\n"
	err = os.WriteFile(tmpFile, []byte(bodyContent), 0644)
	if err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	cmd.Flags().Set("file", tmpFile)
	err = cmd.RunE(cmd, []string{"Test Task 2"})
	if err != nil {
		t.Errorf("expected success with priority in body, but got error: %v", err)
	}
	if !mock.created {
		t.Error("expected tracker.Create to be called")
	}

	foundPrio := false
	for _, l := range mock.labels {
		if l == "priority:critical" {
			foundPrio = true
		}
	}
	if !foundPrio {
		t.Error("expected priority:critical to be parsed from body and appended to labels")
	}
}

func TestFilterTasksByProject(t *testing.T) {
	tasks := []task.Task{
		{Key: "1", Project: "nomos-cockpit"},
		{Key: "2", Project: "sophialabs"},
		{Key: "3", Project: "nomos-cockpit"},
		{Key: "4", Project: ""},
	}

	repoRoot := "/home/user/Projects/nomos-cockpit"

	// Test filtering when not in local mode
	os.Setenv("NOMOS_TASKS_DIR", "/some/global/dir")
	defer os.Unsetenv("NOMOS_TASKS_DIR")

	filtered := FilterTasksByProject(tasks, func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())
	if len(filtered) != 2 {
		t.Errorf("Expected 2 tasks, got %d", len(filtered))
	}
	for _, tk := range filtered {
		if tk.Project != "nomos-cockpit" {
			t.Errorf("Expected project nomos-cockpit, got %s", tk.Project)
		}
	}

	// Test local mode filtering
	os.Unsetenv("NOMOS_TASKS_DIR")
	filteredLocal := FilterTasksByProject(tasks, func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())
	if len(filteredLocal) != 3 {
		t.Errorf("Expected 3 tasks in local mode (including empty project), got %d", len(filteredLocal))
	}
}
