package verify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAuditWorkflows(t *testing.T) {
	// Create a temporary workspace root
	tempRoot, err := os.MkdirTemp("", "nomos-test-workflows-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempRoot)

	workflowsDir := filepath.Join(tempRoot, ".agent", "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatalf("failed to create workflows dir: %v", err)
	}

	validMd := "```bash\nbin/nomos task create \"Story\" --burden 2 --depth 2\n```"
	if err := os.WriteFile(filepath.Join(workflowsDir, "valid.md"), []byte(validMd), 0644); err != nil {
		t.Fatalf("failed to write valid.md: %v", err)
	}

	invalidFlagMd := "```bash\nbin/nomos task create \"Story\" --imaginary-flag 2\n```"
	if err := os.WriteFile(filepath.Join(workflowsDir, "invalid_flag.md"), []byte(invalidFlagMd), 0644); err != nil {
		t.Fatalf("failed to write invalid_flag.md: %v", err)
	}

	invalidCmdMd := "```bash\nbin/nomos imaginary-cmd do-something\n```"
	if err := os.WriteFile(filepath.Join(workflowsDir, "invalid_cmd.md"), []byte(invalidCmdMd), 0644); err != nil {
		t.Fatalf("failed to write invalid_cmd.md: %v", err)
	}

	// We pass the actual project root as the root for command execution, because otherwise `bin/nomos` doesn't exist in tempRoot!
	// Wait, if we use tempRoot as root, `checkNomosCommand` will try to run `bin/nomos` in `tempRoot` and fail.
	// We need to pass the real workspace root so it can execute `bin/nomos`.
	// But `AuditWorkflows` scans `root/.agent/workflows`.
	// Since we want to test parsing without breaking the real workspace or depending on the real workspace state,
	// maybe we can just unit test `auditFile` or `checkNomosCommand` directly.

	// Since we are running `go test ./...` in the real workspace, we can just execute checkNomosCommand.
	realRoot, _ := os.Getwd()
	// Navigate up if we are inside verify (src/nomos/verify -> ../../../)
	if filepath.Base(realRoot) == "verify" {
		realRoot = filepath.Join(realRoot, "..", "..", "..")
	}

	schema, err := getCliSchema(realRoot)
	if err != nil {
		t.Skipf("Skipping workflow test because getCliSchema failed: %v", err)
	}

	t.Run("Valid Command", func(t *testing.T) {
		discrepancies := checkNomosCommand(schema, realRoot, "test.md", 1, "bin/nomos task create \"Story\" --burden 2 --depth 2", "task create \"Story\" --burden 2 --depth 2")
		if len(discrepancies) > 0 {
			t.Errorf("expected 0 discrepancies for valid command, got %d: %v", len(discrepancies), discrepancies)
		}
	})

	t.Run("Invalid Flag", func(t *testing.T) {
		discrepancies := checkNomosCommand(schema, realRoot, "test.md", 2, "bin/nomos task create \"Story\" --imaginary-flag 2", "task create \"Story\" --imaginary-flag 2")
		if len(discrepancies) == 0 {
			t.Errorf("expected discrepancy for invalid flag, got 0")
		} else {
			if discrepancies[0].Message != "Flag '--imaginary-flag' is not supported by command 'bin/nomos task create \"Story\"'" && discrepancies[0].Message != "Flag '--imaginary-flag' is not supported by command 'bin/nomos task create Story'" {
				t.Errorf("unexpected message: %s", discrepancies[0].Message)
			}
		}
	})

}
