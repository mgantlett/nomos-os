package verify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckGoroutineLifecycle(t *testing.T) {
	tempDir := t.TempDir()

	// Initialize Git repository
	if _, err := runGit(tempDir, "init"); err != nil {
		t.Fatalf("failed to init git: %v", err)
	}
	// Configure dummy git user to permit commit/staging if needed
	_, _ = runGit(tempDir, "config", "user.name", "Test User")
	_, _ = runGit(tempDir, "config", "user.email", "test@example.com")

	cmdDir := filepath.Join(tempDir, "src", "nomos", "cmd")
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		t.Fatalf("failed to create cmd dir: %v", err)
	}

	// 1. Write clean code using sync.WaitGroup (should pass)
	goodFile := filepath.Join(cmdDir, "good.go")
	goodCode := `package cmd
import "sync"
func runGood() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
	}()
	wg.Wait()
}
`
	if err := os.WriteFile(goodFile, []byte(goodCode), 0644); err != nil {
		t.Fatalf("failed to write good.go: %v", err)
	}

	// 2. Write clean code using LifecycleRunner (should pass)
	runnerFile := filepath.Join(cmdDir, "runner.go")
	runnerCode := `package cmd
func runRunner() {
	// CallExpr does not produce GoStmt in AST
	someRunnerCall(func() {})
}
`
	if err := os.WriteFile(runnerFile, []byte(runnerCode), 0644); err != nil {
		t.Fatalf("failed to write runner.go: %v", err)
	}

	// Stage good files
	if _, err := runGit(tempDir, "add", "src/nomos/cmd/good.go", "src/nomos/cmd/runner.go"); err != nil {
		t.Fatalf("failed to stage good files: %v", err)
	}

	// Run audit - should pass
	if err := CheckGoroutineLifecycle(tempDir); err != nil {
		t.Errorf("expected no violations for good code, got: %v", err)
	}

	// 3. Write bad code with raw untracked goroutine (should fail)
	badFile := filepath.Join(cmdDir, "bad.go")
	badCode := `package cmd
func runBad() {
	go func() {
		println("untracked")
	}()
}
`
	if err := os.WriteFile(badFile, []byte(badCode), 0644); err != nil {
		t.Fatalf("failed to write bad.go: %v", err)
	}

	// Stage bad file
	if _, err := runGit(tempDir, "add", "src/nomos/cmd/bad.go"); err != nil {
		t.Fatalf("failed to stage bad file: %v", err)
	}

	// Run audit - should fail
	if err := CheckGoroutineLifecycle(tempDir); err == nil {
		t.Error("expected violation for raw untracked goroutine, but check passed")
	}
}
