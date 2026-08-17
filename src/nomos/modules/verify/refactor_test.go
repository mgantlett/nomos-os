package verify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

func TestNormalizeLine(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"  func foo() { // comment ", "funcfoo(){"},
		{"# python comment", ""},
		{"-- lua/haskell comment", ""},
		{"a = b + c", "a=b+c"},
	}

	for _, c := range cases {
		got := normalizeLine(c.input)
		if got != c.expected {
			t.Errorf("normalizeLine(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}

func TestRunRefactorChecks_LengthAndDuplication(t *testing.T) {
	tmpDir := t.TempDir()

	// Create dummy git repository environment inside temp dir
	gitInitCmd := execCommand(tmpDir, "git", "init")
	if err := gitInitCmd.Run(); err != nil {
		t.Fatalf("failed to init git: %v", err)
	}
	gitConfigCmd1 := execCommand(tmpDir, "git", "config", "user.name", "Test")
	_ = gitConfigCmd1.Run()
	gitConfigCmd2 := execCommand(tmpDir, "git", "config", "user.email", "test@example.com")
	_ = gitConfigCmd2.Run()

	// 1. Create a file exceeding 500 lines to test warnings (505 lines)
	var longLines []string
	for i := 0; i < 505; i++ {
		longLines = append(longLines, fmt.Sprintf("fmt.Println(\"line %d\")", i))
	}
	longFilePath := filepath.Join(tmpDir, "long_file.go")
	if err := os.WriteFile(longFilePath, []byte(strings.Join(longLines, "\n")), 0644); err != nil {
		t.Fatalf("failed to write long file: %v", err)
	}

	// 2. Create duplicate block files (15 duplicate lines)
	dupBlock := `
func executeStep(action string) {
	synapse.Info("Starting step: %s\n", action)
	time.Sleep(100 * time.Millisecond)
	synapse.Info("Completed step: %s\n", action)
	log.Printf("State updated for action %s", action)
	if action == "stop" {
		return
	}
	synapse.Info("%s", fmt.Sprint("Proceeding to next step"))
}
`
	file1Content := "package main\n" + dupBlock
	file2Content := "package main\n\n// Different comments\n" + dupBlock

	f1Path := filepath.Join(tmpDir, "file1.go")
	f2Path := filepath.Join(tmpDir, "file2.go")
	_ = os.WriteFile(f1Path, []byte(file1Content), 0644)
	_ = os.WriteFile(f2Path, []byte(file2Content), 0644)

	// Mock phase state to indicate low-tier agent so bypasses are created
	_ = os.MkdirAll(workspace.MustNewContext(tmpDir).StateDir(), 0755)
	phaseStatePath := workspace.MustNewContext(tmpDir).NomosStatePath(".phase_state.json")
	_ = os.WriteFile(phaseStatePath, []byte(`{"agent": "aider", "agent_tier": "low"}`), 0644)

	// Git add files to stage them
	gitAddCmd := execCommand(tmpDir, "git", "add", "long_file.go", "file1.go", "file2.go")
	if err := gitAddCmd.Run(); err != nil {
		t.Fatalf("failed to stage files: %v", err)
	}

	// Run refactor checks (should fail due to duplication)
	err := RunRefactorChecks(tmpDir, false)
	if err == nil {
		t.Errorf("expected RunRefactorChecks to fail due to duplication")
	}

	// Verify that AUTO task and refactor stories were staged
	manifestPath := filepath.Join(workspace.MustNewContext(tmpDir).DataDir(), "state", "quality_debt.json")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Errorf("expected quality_debt.json to be created")
	}

	data, err := os.ReadFile(manifestPath)
	if err == nil {
		var manifest QualityDebtManifest
		if err := json.Unmarshal(data, &manifest); err == nil {
			if len(manifest.ActiveDebt) == 0 {
				t.Errorf("expected active debt entries to be registered")
			}
			foundAuto := false
			for _, item := range manifest.ActiveDebt {
				if item.LinkedTask == "AUTO" {
					foundAuto = true
				}
			}
			if !foundAuto {
				t.Errorf("expected linked_task: AUTO to be created")
			}
		}
	}

	// Test bypass by setting valid bypass in quality_debt.json
	manifest := QualityDebtManifest{
		ActiveDebt: []QualityDebtItem{
			{
				File:       "file1.go",
				Gate:       "duplication_limit",
				Reason:     "Bypass test",
				LinkedTask: "123",
				CreatedAt:  time.Now().Format(time.RFC3339),
				ExpiresAt:  time.Now().AddDate(0, 1, 0).Format(time.RFC3339),
			},
			{
				File:       "file2.go",
				Gate:       "duplication_limit",
				Reason:     "Bypass test",
				LinkedTask: "123",
				CreatedAt:  time.Now().Format(time.RFC3339),
				ExpiresAt:  time.Now().AddDate(0, 1, 0).Format(time.RFC3339),
			},
		},
	}
	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	_ = os.WriteFile(manifestPath, manifestBytes, 0644)

	// Run refactor checks again (should pass now because of bypasses)
	err = RunRefactorChecks(tmpDir, false)
	if err != nil {
		t.Errorf("expected RunRefactorChecks to pass with bypasses, got error: %v", err)
	}
}

func TestCheckDuplicateStructs(t *testing.T) {
	tmpDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tmpDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	// 1. Initially no duplicates, should pass
	_, err = CheckDuplicateStructs(tmpDir)
	if err != nil {
		t.Errorf("expected CheckDuplicateStructs to pass on empty codebase, got error: %v", err)
	}

	// 2. Add single struct (Go), should pass
	goFile1 := filepath.Join(tmpDir, "state1.go")
	_ = os.WriteFile(goFile1, []byte("package main\ntype PhaseState struct {}"), 0644)
	_, err = CheckDuplicateStructs(tmpDir)
	if err != nil {
		t.Errorf("expected CheckDuplicateStructs to pass with single Go struct, got error: %v", err)
	}

	// 3. Add duplicate struct (Python), should fail
	pyFile1 := filepath.Join(tmpDir, "state2.py")
	_ = os.WriteFile(pyFile1, []byte("class PhaseState:\n    pass"), 0644)
	_, err = CheckDuplicateStructs(tmpDir)
	if err == nil {
		t.Errorf("expected CheckDuplicateStructs to fail with duplicate struct in Python")
	}

	// Remove Py file, should pass again
	_ = os.Remove(pyFile1)
	_, err = CheckDuplicateStructs(tmpDir)
	if err != nil {
		t.Errorf("expected CheckDuplicateStructs to pass after removing Python file, got error: %v", err)
	}

	// 4. Add duplicate struct (TS interface), should fail
	tsFile1 := filepath.Join(tmpDir, "state3.ts")
	_ = os.WriteFile(tsFile1, []byte("interface PhaseState {}"), 0644)
	_, err = CheckDuplicateStructs(tmpDir)
	if err == nil {
		t.Errorf("expected CheckDuplicateStructs to fail with duplicate interface in TypeScript")
	}
}
