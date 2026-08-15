package exec

import (
	"path/filepath"
	"testing"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
)

// TestValidateSubstrateTargetPath tests target edit path validation for local, worktree, and cross-root paths.
func TestValidateSubstrateTargetPath(t *testing.T) {
	root := t.TempDir()

	// 1. Local workspace path should pass validation
	localFile := filepath.Join(root, "src", "main.go")
	if err := ValidateSubstrateTargetPath(root, localFile); err != nil {
		t.Fatalf("expected local target path to be valid, got: %v", err)
	}

	// 2. Global data / worktree path should pass validation
	worktreeFile := filepath.Join(config.GlobalDataDir(root), "worktrees", "sibling-task", "main.go")
	if err := ValidateSubstrateTargetPath(root, worktreeFile); err != nil {
		t.Fatalf("expected worktree path to be valid, got: %v", err)
	}

	// 3. Unauthorized sibling root path should fail validation
	siblingRoot := filepath.Join(filepath.Dir(root), "unauthorized-sibling-repo", "src", "main.go")
	if err := ValidateSubstrateTargetPath(root, siblingRoot); err == nil {
		t.Fatalf("expected cross-root sibling path to fail validation, but it passed")
	}
}
