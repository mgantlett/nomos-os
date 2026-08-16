package verify

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

func TestParseSpecFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nomos_spec_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Dynamically build the plan content with paths matching tempDir
	planContent := fmt.Sprintf(`
# Some header info

## Proposed Changes

### Component 1

#### [NEW] [foo.go](file://%s/src/nomos/verify/foo.go)
- Some info

#### [MODIFY] [bar_test.go](file://%s/src/nomos/verify/bar_test.go)
- Some description
`, filepath.ToSlash(tempDir), filepath.ToSlash(tempDir))

	planFile := filepath.Join(tempDir, "implementation_plan.md")
	if err := os.WriteFile(planFile, []byte(planContent), 0644); err != nil {
		t.Fatalf("failed to write plan file: %v", err)
	}

	planned, err := ParseSpecFiles(planFile, tempDir)
	if err != nil {
		t.Fatalf("ParseSpecFiles returned error: %v", err)
	}

	if len(planned) != 2 {
		t.Errorf("expected 2 planned files, got %d. planned maps: %+v", len(planned), planned)
	}

	expectedFoo := "src/nomos/verify/foo.go"
	expectedBar := "src/nomos/verify/bar_test.go"

	if !planned[expectedFoo] {
		t.Errorf("expected planned files to contain %s", expectedFoo)
	}
	if !planned[expectedBar] {
		t.Errorf("expected planned files to contain %s", expectedBar)
	}
}

func TestGetActiveTaskId(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nomos_task_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tmpDir := workspace.MustNewContext(tempDir).TmpDir()
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatalf("failed to create task.md dir: %v", err)
	}
	taskMdPath := filepath.Join(tmpDir, "task.md")
	if err := os.WriteFile(taskMdPath, []byte("Task: GEN-1234\n"), 0644); err != nil {
		t.Fatalf("failed to write task.md: %v", err)
	}

	id := GetActiveTaskId(tempDir)
	if id != "GEN-1234" {
		t.Errorf("expected task ID GEN-1234, got %q", id)
	}

	os.Remove(taskMdPath)

	_ = os.MkdirAll(workspace.MustNewContext(tempDir).StateDir(), 0755)
	stateContent := `{"task_id": "347"}`
	if err := os.WriteFile(workspace.MustNewContext(tempDir).NomosStatePath(".phase_state.json"), []byte(stateContent), 0644); err != nil {
		t.Fatalf("failed to write phase state: %v", err)
	}

	id2 := GetActiveTaskId(tempDir)
	if id2 != "347" {
		t.Errorf("expected task ID 347, got %q", id2)
	}
}

func TestCheckSpecParity(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nomos_parity_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	specsDir := filepath.Join(workspace.MustNewContext(tempDir).DataPath("plans"), "347")
	if err := os.MkdirAll(specsDir, 0755); err != nil {
		t.Fatalf("failed to create specs dir: %v", err)
	}

	planContent := fmt.Sprintf(`
## Proposed Changes
#### [NEW] [foo.go](file://%s/src/nomos/verify/foo.go)
#### [NEW] [bar.go](file://%s/src/nomos/verify/bar.go)
`, filepath.ToSlash(tempDir), filepath.ToSlash(tempDir))

	planPath := filepath.Join(specsDir, "implementation_plan.md")
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("failed to write plan: %v", err)
	}

	runGit := func(dir string, args ...string) {
		cmd := execGit(dir, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v, output: %s", args, err, string(out))
		}
	}

	runGit(tempDir, "init")
	runGit(tempDir, "config", "user.name", "Test User")
	runGit(tempDir, "config", "user.email", "test@example.com")

	baseFile := filepath.Join(tempDir, "base.txt")
	os.WriteFile(baseFile, []byte("hello"), 0644)
	runGit(tempDir, "add", "base.txt")
	runGit(tempDir, "commit", "-m", "initial commit")
	runGit(tempDir, "branch", "develop")
	runGit(tempDir, "checkout", "-b", "task/347-task-nomos-verify")

	fooPath := filepath.Join(tempDir, "src", "nomos", "verify")
	os.MkdirAll(fooPath, 0755)
	os.WriteFile(filepath.Join(fooPath, "foo.go"), []byte("package verify\n"), 0644)
	os.WriteFile(filepath.Join(fooPath, "extra.go"), []byte("package verify\n"), 0644)

	drift, parity, err := CheckSpecParity(tempDir, "347")
	if err != nil {
		t.Fatalf("CheckSpecParity failed: %v", err)
	}

	if drift < 66.0 || drift > 67.0 {
		t.Errorf("expected drift score around 66.7%%, got %.2f%%", drift)
	}
	if parity < 33.0 || parity > 34.0 {
		t.Errorf("expected parity score around 33.3%%, got %.2f%%", parity)
	}

	reportPath := filepath.Join(specsDir, "parity_report.md")
	reportContent, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("failed to read report: %v", err)
	}

	if !strings.Contains(string(reportContent), "Self-Drift Score") {
		t.Errorf("expected report to contain Self-Drift Score")
	}
}

func TestParseSpecFilesProposedChanges(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nomos_parse_spec_test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	planContent := fmt.Sprintf(`
# Some Plan

## User Review Required

## Proposed Changes

### Component 1
#### [NEW] [foo.go](file://%s/src/nomos/verify/foo.go)
- [MODIFY] [bar_test.go](file://%s/src/nomos/verify/bar_test.go)

### Component 2
- [NEW] src/nomos/verify/hello.go
* [MODIFY] src/nomos/verify/world.go
#### [DELETE] src/nomos/verify/delete_me.go

## Verification Plan
- [NEW] test.go
`, filepath.ToSlash(tempDir), filepath.ToSlash(tempDir))

	planPath := filepath.Join(tempDir, "implementation_plan.md")
	if err := os.WriteFile(planPath, []byte(planContent), 0644); err != nil {
		t.Fatalf("failed to write plan: %v", err)
	}

	planned, err := ParseSpecFiles(planPath, tempDir)
	if err != nil {
		t.Fatalf("ParseSpecFiles failed: %v", err)
	}

	expected := []string{
		"src/nomos/verify/foo.go",
		"src/nomos/verify/bar_test.go",
		"src/nomos/verify/hello.go",
		"src/nomos/verify/world.go",
		"src/nomos/verify/delete_me.go",
	}

	for _, exp := range expected {
		if !planned[exp] {
			t.Errorf("expected planned files to contain %q, but it did not. Got planned: %v", exp, planned)
		}
	}

	if planned["test.go"] {
		t.Errorf("did not expect test.go to be parsed since it is under Verification Plan")
	}
}

func execGit(dir string, args ...string) *exec.Cmd {
	// If performing a commit in test setup, append --no-verify to bypass active githooks
	if len(args) > 0 && args[0] == "commit" {
		args = append(args, "--no-verify")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = cleanGitEnv()
	return cmd
}

func TestASTSymbolParityVerification(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nomos_ast_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 1. Create a dummy implementation plan with tokenized details
	planContent := `
## Proposed Changes
#### [MODIFY] [foo.go](file:///src/nomos/verify/foo.go)
- We will implement 'TransitionPhase' and a new struct type 'CustomState'.
`
	specsDir := filepath.Join(tempDir, ".agent", "specs", "129")
	os.MkdirAll(specsDir, 0755)
	planPath := filepath.Join(specsDir, "implementation_plan.md")
	os.WriteFile(planPath, []byte(planContent), 0644)

	// 2. Test Plan tokenization
	tokens, err := tokenizePlanText(planPath, tempDir)
	if err != nil {
		t.Fatalf("tokenizePlanText failed: %v", err)
	}
	fooTokens := tokens["src/nomos/verify/foo.go"]
	if fooTokens == nil {
		t.Fatalf("expected tokens for foo.go, got nil")
	}
	if !fooTokens["TransitionPhase"] {
		t.Errorf("expected token TransitionPhase in plan tokens")
	}
	if !fooTokens["CustomState"] {
		t.Errorf("expected token CustomState in plan tokens")
	}

	// 3. Test Go AST parser
	goCode := `package verify
type CustomState struct {
	Val int
}
func TransitionPhase() {
}
func undocumentedFunc() {
}
`
	srcDir := filepath.Join(tempDir, "src", "nomos", "verify")
	os.MkdirAll(srcDir, 0755)
	goFilePath := filepath.Join(srcDir, "foo.go")
	os.WriteFile(goFilePath, []byte(goCode), 0644)

	symbols, err := parseGoSymbols(goFilePath)
	if err != nil {
		t.Fatalf("parseGoSymbols failed: %v", err)
	}

	foundStruct := false
	foundFunc := false
	for _, sym := range symbols {
		if sym.Name == "CustomState" && sym.Type == "type" {
			foundStruct = true
		}
		if sym.Name == "TransitionPhase" && sym.Type == "function" {
			foundFunc = true
		}
	}
	if !foundStruct {
		t.Errorf("expected to find CustomState struct symbol")
	}
	if !foundFunc {
		t.Errorf("expected to find TransitionPhase function symbol")
	}
}

func TestIsAgentStateFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{".nomos/tmp/somefile.json", true},
		{".nomos/specs/113/implementation_plan.md", true},
		{".agent/workflows/handshake.md", false},
		{".agent/workflows/close.md", false},
		{"src/nomos/cmd/handshake.go", false},
		{"README.md", true},
		{".nomos/state/quality_debt.json", false},
	}

	for _, tc := range tests {
		result := isAgentStateFile(tc.path)
		if result != tc.expected {
			t.Errorf("isAgentStateFile(%q) = %t; want %t", tc.path, result, tc.expected)
		}
	}
}
