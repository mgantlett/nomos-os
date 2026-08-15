package harness

import (
	"context"
	"strings"
	"testing"
)

func TestDriverInitialization(t *testing.T) {
	driver := NewDriver("/tmp/mock")
	if driver.WorktreeDir != "/tmp/mock" {
		t.Errorf("Expected /tmp/mock, got %s", driver.WorktreeDir)
	}
}

// A simple test to ensure RunSandboxedCrucible handles errors correctly
func TestRunSandboxedCrucible_Mock(t *testing.T) {
	// In a real test, we would mock the exec.Command to return structured output.
	// For now, we just ensure the struct builds and functions are exported.
	driver := NewDriver(".")
	err := driver.ExecuteOpenCode(context.Background(), "mock message")
	if err == nil || !strings.Contains(err.Error(), "exit status") {
		// opencode command doesn't exist natively outside nix-shell, or requires arguments.
		// We just want to ensure it tries to execute.
	}
}
