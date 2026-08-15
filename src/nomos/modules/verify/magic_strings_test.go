package verify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckMagicStrings(t *testing.T) {
	tmpDir := t.TempDir()

	validCode := `package main
import "github.com/mgantlett/nomos-os/src/nomos/modules/task"
func testValid() {
	tracker.Create(task.TypeBug)
	tracker.Edit(task.TypeTask)
}`

	invalidCode := `package main
func testInvalid() {
	tracker.Create("Bug")
	task.Edit("Task")
}`

	validFile := filepath.Join(tmpDir, "valid.go")
	invalidFile := filepath.Join(tmpDir, "invalid.go")
	os.WriteFile(validFile, []byte(validCode), 0644)
	os.WriteFile(invalidFile, []byte(invalidCode), 0644)

	findings := CheckMagicStrings(tmpDir, []string{"valid.go"})
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for valid code, got %d", len(findings))
	}

	findingsInvalid := CheckMagicStrings(tmpDir, []string{"invalid.go"})
	if len(findingsInvalid) != 2 {
		t.Errorf("expected 2 findings for invalid code, got %d", len(findingsInvalid))
	}
}
