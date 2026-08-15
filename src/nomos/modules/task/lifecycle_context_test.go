package task

import (
	"path/filepath"
	"testing"
)

// TestValidateWorkspaceTaskContext tests project workspace context validation assertions.
func TestValidateWorkspaceTaskContext(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Matching project context should pass validation
	matchingRoot := filepath.Join(tempDir, "nomos-commons")
	if err := ValidateWorkspaceTaskContext(matchingRoot, "COM-983", "nomos-commons"); err != nil {
		t.Fatalf("expected matching workspace context to pass, got: %v", err)
	}

	// 2. Case-insensitive matching project context should pass validation
	if err := ValidateWorkspaceTaskContext(matchingRoot, "COM-983", "NOMOS-COMMONS"); err != nil {
		t.Fatalf("expected case-insensitive matching workspace context to pass, got: %v", err)
	}

	// 3. Mismatched project context should fail validation with explicit error
	mismatchedRoot := filepath.Join(tempDir, "nomos-commons")
	if err := ValidateWorkspaceTaskContext(mismatchedRoot, "PMD-979", "papermind"); err == nil {
		t.Fatalf("expected mismatched workspace context to fail, but it passed")
	}
}
