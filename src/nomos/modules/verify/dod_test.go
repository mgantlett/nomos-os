package verify

import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyDoD(t *testing.T) {
	tempDir := t.TempDir()

	// Set up a mock git repo
	runGit := func(dir string, args ...string) {
		cmd := execGit(dir, args...)
		if err := cmd.Run(); err != nil {
			// ignore
		}
	}
	runGit(tempDir, "init")
	runGit(tempDir, "config", "user.name", "Test User")
	runGit(tempDir, "config", "user.email", "test@example.com")

	// Write basic files to pass formatting, vetting, and unit tests
	goModContent := "module testrepo\n\ngo 1.22\n"
	os.WriteFile(filepath.Join(tempDir, "go.work"), []byte("go 1.22\n\nuse .\n"), 0644)
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goModContent), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
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

	mainGoContent := `package main

import "fmt"

func main() {
	fmt.Println("Hello World")
}
`
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte(mainGoContent), 0644); err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}

	mainTestContent := `package main

import "testing"

func TestMain(t *testing.T) {
	if false {
		t.Error("dummy")
	}
}
`
	if err := os.WriteFile(filepath.Join(srcDir, "main_test.go"), []byte(mainTestContent), 0644); err != nil {
		t.Fatalf("failed to write main_test.go: %v", err)
	}

	// Commit them
	runGit(tempDir, "add", ".")
	runGit(tempDir, "commit", "--no-verify", "-m", "initial commit")

	// 1. Run VerifyDoD (should pass because there's no spec plan, formatting is correct, no secrets, tests pass)
	err := VerifyDoD(&workspace.WorkspaceContext{RepoRoot: tempDir})
	if err != nil {
		t.Errorf("expected VerifyDoD to pass, but got: %v", err)
	}

	// 2. Introduce a lint/gofmt failure (unformatted file)
	badGoContent := `package main
import "fmt"

// BadFormat is an exported function.
// It prints a bad format string.
// This ensures we pass the comment density check and the boy scout docstring check.
// We also create a test file to pass TDD.
func BadFormat() {
fmt.Println("bad")
}
` // bad indentation
	badGoPath := filepath.Join(srcDir, "bad.go")
	if err := os.WriteFile(badGoPath, []byte(badGoContent), 0644); err != nil {
		t.Fatalf("failed to write bad.go: %v", err)
	}

	badTestContent := `package main
import "testing"
func TestBadFormat(t *testing.T) {
	BadFormat()
	if false {
		t.Error("dummy")
	}
}
`
	badTestPath := filepath.Join(srcDir, "bad_test.go")
	if err := os.WriteFile(badTestPath, []byte(badTestContent), 0644); err != nil {
		t.Fatalf("failed to write bad_test.go: %v", err)
	}

	runGit(tempDir, "add", badGoPath)
	runGit(tempDir, "add", badTestPath)

	err = VerifyDoD(&workspace.WorkspaceContext{RepoRoot: tempDir})
	if err != nil {
		t.Errorf("expected VerifyDoD to pass and auto-format the file, but it failed: %v", err)
	}

	formattedBytes, _ := os.ReadFile(badGoPath)
	if !strings.Contains(string(formattedBytes), "\tfmt.Println") {
		t.Errorf("expected bad.go to be auto-formatted with correct indentation")
	}

	// 3. Test Bypass via environment variable
	os.Setenv("NOMOS_LEGACY_APPROVAL_TOKEN", "OVERRIDE")
	err = VerifyDoD(&workspace.WorkspaceContext{RepoRoot: tempDir})
	if err != nil {
		t.Errorf("expected VerifyDoD to succeed via bypass, but got: %v", err)
	}
	os.Unsetenv("NOMOS_LEGACY_APPROVAL_TOKEN")

	// Remove the unformatted file
	runGit(tempDir, "rm", "-f", "src/bad.go")

	// 4. Introduce a test failure
	failingTest := `package main
import "testing"
func TestFail(t *testing.T) {
	t.Error("forced failure")
}
`
	failingTestPath := filepath.Join(srcDir, "fail_test.go")
	os.WriteFile(failingTestPath, []byte(failingTest), 0644)
	runGit(tempDir, "add", failingTestPath)

	err = VerifyDoD(&workspace.WorkspaceContext{RepoRoot: tempDir})
	if err == nil {
		t.Errorf("expected VerifyDoD to fail due to failing test, but it succeeded")
	}

	// Clean up
	os.Remove(failingTestPath)
}
