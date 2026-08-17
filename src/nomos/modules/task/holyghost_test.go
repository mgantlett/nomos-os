package task

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

type mockTracker struct {
	Tracker
	task *Task
	err  error
}

func (m *mockTracker) View(ctx context.Context, key string) (*Task, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.task, nil
}

func (m *mockTracker) List(ctx context.Context) ([]Task, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.task != nil {
		return []Task{*m.task}, nil
	}
	return []Task{}, nil
}

func TestGenerateHolyGhostContext(t *testing.T) {
	tempDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tempDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	agentDir := workspace.MustNewContext(tempDir).TmpDir()
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("failed to create .agent/tmp: %v", err)
	}
	if err := os.MkdirAll(workspace.MustNewContext(tempDir).TmpDir(), 0755); err != nil {
		t.Fatalf("failed to create tmp dir: %v", err)
	}

	mockTask := &Task{
		Key:         "JAZZ-123",
		Title:       "Implement the feature",
		Description: "This feature must be done using standard Go structures.",
	}
	tracker := &mockTracker{task: mockTask}

	// Run GenerateHolyGhostContext
	err = GenerateHolyGhostContext(context.Background(), func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(tempDir); return c }(), tracker, "JAZZ-123")
	if err != nil {
		t.Fatalf("GenerateHolyGhostContext failed: %v", err)
	}

	// Verify file is created and populated
	promptPath := filepath.Join(agentDir, ".context-prompt.md")
	contentBytes, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("failed to read context prompt: %v", err)
	}

	content := string(contentBytes)
	if !strings.Contains(content, "# Holy Ghost Architectural Context (Task JAZZ-123)") {
		t.Errorf("expected context prompt to contain task key JAZZ-123, got: %s", content)
	}
	if !strings.Contains(content, "## Semantic Memory Insights") {
		t.Errorf("expected context prompt to contain Semantic Memory Insights header")
	}
	if !strings.Contains(content, "## Relevant Codebase Snippets") {
		t.Errorf("expected context prompt to contain Relevant Codebase Snippets header")
	}
}

func TestGenerateHolyGhostContextTemplates(t *testing.T) {
	tempDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tempDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	if err := os.MkdirAll(workspace.MustNewContext(tempDir).TmpDir(), 0755); err != nil {
		t.Fatalf("failed to create agent tmp dir: %v", err)
	}

	if err := os.MkdirAll(workspace.MustNewContext(tempDir).TmpDir(), 0755); err != nil {
		t.Fatalf("failed to create tmp dir: %v", err)
	}

	// 2. Write phase state
	state := `{
		"agent": "gemini",
		"current_phase": "PLAN",
		"task_id": "JAZZ-123"
	}`
	_ = os.MkdirAll(workspace.MustNewContext(tempDir).StateDir(), 0755)
	if err := os.WriteFile(workspace.MustNewContext(tempDir).NomosStatePath(".phase_state.json"), []byte(state), 0644); err != nil {
		t.Fatalf("failed to write phase state: %v", err)
	}

	mockTask := &Task{
		Key:         "JAZZ-123",
		Title:       "Implement the feature",
		Description: "This feature must be done using standard Go structures.",
	}
	tracker := &mockTracker{task: mockTask}

	// 3. Run context generator
	err = GenerateHolyGhostContext(context.Background(), func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(tempDir); return c }(), tracker, "JAZZ-123")
	if err != nil {
		t.Fatalf("GenerateHolyGhostContext failed: %v", err)
	}

	promptPath := filepath.Join(workspace.MustNewContext(tempDir).TmpDir(), ".context-prompt.md")
	contentBytes, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("failed to read context prompt: %v", err)
	}

	content := string(contentBytes)
	if !strings.Contains(content, "Focus exclusively on drafting the implementation_plan.md.") {
		t.Errorf("expected context to contain embedded PLAN rules, got: %s", content)
	}
	if !strings.Contains(content, "Keep responses extremely concise.") {
		t.Errorf("expected context to contain embedded Gemini rules, got: %s", content)
	}
}

func TestGenerateHolyGhostContextCompact(t *testing.T) {
	tempDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tempDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	agentDir := filepath.Join(tempDir, ".agent")
	if err := os.MkdirAll(workspace.MustNewContext(tempDir).TmpDir(), 0755); err != nil {
		t.Fatalf("failed to create agent tmp dir: %v", err)
	}

	// 1. Write phase state with compact_context = true
	state := `{
		"agent": "gemini",
		"current_phase": "PLAN",
		"task_id": "125",
		"compact_context": true
	}`
	_ = os.MkdirAll(workspace.MustNewContext(tempDir).StateDir(), 0755)
	if err := os.WriteFile(workspace.MustNewContext(tempDir).NomosStatePath(".phase_state.json"), []byte(state), 0644); err != nil {
		t.Fatalf("failed to write phase state: %v", err)
	}
	if err := os.MkdirAll(workspace.MustNewContext(tempDir).TmpDir(), 0755); err != nil {
		t.Fatalf("failed to create tmp dir: %v", err)
	}

	// 2. Write an implementation plan listing planned files
	specsDir := filepath.Join(agentDir, "specs", "125")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatalf("failed to create specs dir: %v", err)
	}
	planContent := `
## Proposed Changes

### AST Package
- [MODIFY] [parser.go](file:///src/nomos/ast/parser.go)
`
	if err := os.WriteFile(filepath.Join(specsDir, "implementation_plan.md"), []byte(planContent), 0644); err != nil {
		t.Fatalf("failed to write implementation plan: %v", err)
	}

	// 3. Create a dependency package directory and a Go file to extract signatures from
	// The planned file is in ast/ package, which imports nothing local in our test,
	// but let's fake a planned file in "src/nomos/task/holyghost.go" which imports "src/nomos/ast"
	planContent2 := fmt.Sprintf(`
## Proposed Changes

### Task Package
- [MODIFY] [holyghost.go](file://%s)
`, filepath.ToSlash(filepath.Join(tempDir, "src", "nomos", "modules", "task", "holyghost.go")))
	if err := os.WriteFile(filepath.Join(specsDir, "implementation_plan.md"), []byte(planContent2), 0644); err != nil {
		t.Fatalf("failed to write implementation plan: %v", err)
	}

	taskPkgDir := filepath.Join(tempDir, "src", "nomos", "modules", "task")
	_ = os.MkdirAll(taskPkgDir, 0755)
	activeGoFile := filepath.Join(taskPkgDir, "holyghost.go")
	activeGoContent := `package task
import "github.com/mgantlett/nomos-commons/src/nomos/core/ast"
`
	_ = os.WriteFile(activeGoFile, []byte(activeGoContent), 0644)

	astPkgDir := filepath.Join(tempDir, "src", "nomos", "core", "ast")
	_ = os.MkdirAll(astPkgDir, 0755)
	depGoFile := filepath.Join(astPkgDir, "parser.go")
	depGoContent := `package ast

type Option struct {
    Name string
}

func PerformOption(o Option) error {
    return nil
}
`
	_ = os.WriteFile(depGoFile, []byte(depGoContent), 0644)

	mockTask := &Task{
		Key:         "125",
		Title:       "Context compaction",
		Description: "Compact the RAG context",
	}
	tracker := &mockTracker{task: mockTask}

	// Run GenerateHolyGhostContext
	err = GenerateHolyGhostContext(context.Background(), func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(tempDir); return c }(), tracker, "125")
	if err != nil {
		t.Fatalf("GenerateHolyGhostContext failed: %v", err)
	}

	promptPath := filepath.Join(workspace.MustNewContext(tempDir).TmpDir(), ".context-prompt.md")
	contentBytes, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("failed to read context prompt: %v", err)
	}

	content := string(contentBytes)
	if !strings.Contains(content, "## Dependency Package Signatures") {
		t.Errorf("expected context to contain Dependency Package Signatures section, got: %s", content)
	}
	if !strings.Contains(content, "func PerformOption(o Option) error") {
		t.Errorf("expected signature to be preserved, got: %s", content)
	}
	if strings.Contains(content, "return nil") {
		t.Errorf("expected body to be stripped from signature, got: %s", content)
	}
}
