package verify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAuditWorkflows(t *testing.T) {
	// Create a temporary workspace root
	tempRoot := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tempRoot, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	workflowsDir := filepath.Join(tempRoot, ".agent", "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatalf("failed to create workflows dir: %v", err)
	}

	validMd := "```bash\nbin/nomos task create \"Task\" --burden 2 --depth 2\n```"
	if err := os.WriteFile(filepath.Join(workflowsDir, "valid.md"), []byte(validMd), 0644); err != nil {
		t.Fatalf("failed to write valid.md: %v", err)
	}

	invalidFlagMd := "```bash\nbin/nomos task create \"Task\" --imaginary-flag 2\n```"
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
		discrepancies := checkNomosCommand(schema, realRoot, "test.md", 1, "bin/nomos task create \"Task\" --burden 2 --depth 2", "task create \"Task\" --burden 2 --depth 2")
		if len(discrepancies) > 0 {
			t.Errorf("expected 0 discrepancies for valid command, got %d: %v", len(discrepancies), discrepancies)
		}
	})

	t.Run("Invalid Flag", func(t *testing.T) {
		discrepancies := checkNomosCommand(schema, realRoot, "test.md", 2, "bin/nomos task create \"Task\" --imaginary-flag 2", "task create \"Task\" --imaginary-flag 2")
		if len(discrepancies) == 0 {
			t.Errorf("expected discrepancy for invalid flag, got 0")
		} else {
			if discrepancies[0].Message != "Flag '--imaginary-flag' is not supported by command 'bin/nomos task create \"Task\"'" && discrepancies[0].Message != "Flag '--imaginary-flag' is not supported by command 'bin/nomos task create Task'" {
				t.Errorf("unexpected message: %s", discrepancies[0].Message)
			}
		}
	})

	t.Run("Raw Shell Command", func(t *testing.T) {
		rawShellMd := "```bash\ngit commit -m 'bypass'\n```"
		if err := os.WriteFile(filepath.Join(workflowsDir, "raw_shell.md"), []byte(rawShellMd), 0644); err != nil {
			t.Fatalf("failed to write raw_shell.md: %v", err)
		}
		discrepancies, err := auditFile(schema, realRoot, filepath.Join(workflowsDir, "raw_shell.md"))
		if err != nil {
			t.Fatalf("auditFile failed: %v", err)
		}
		if len(discrepancies) == 0 {
			t.Errorf("expected discrepancy for raw shell command, got 0")
		} else if discrepancies[0].Message != "Workflow execution bypass: Only nomos commands are permitted inside execution blocks to enforce Cognitive Firewall determinism." {
			t.Errorf("unexpected message: %s", discrepancies[0].Message)
		}
	})
}
