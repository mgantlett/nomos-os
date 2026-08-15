package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHooksUseNixShell(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}

	rootCwd := wd
	for {
		if _, err := os.Stat(filepath.Join(rootCwd, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(rootCwd)
		if parent == rootCwd {
			t.Fatalf("could not find root directory containing go.mod")
		}
		rootCwd = parent
	}

	hooks := []string{"pre-commit", "pre-push"}
	for _, hookName := range hooks {
		hookPath := filepath.Join(rootCwd, "src", "nomos", "core", "assets", "templates", "hooks", "pre-commit")
		content, err := os.ReadFile(hookPath)
		if err != nil {
			t.Fatalf("failed to read hook %s at %s: %v", hookName, hookPath, err)
		}

		if !strings.Contains(string(content), "nix-shell --run") {
			t.Errorf("hook %s at %s does not contain 'nix-shell --run'", hookName, hookPath)
		}
	}
}
