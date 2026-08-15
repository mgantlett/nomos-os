package verify

// Config Drift verification gate ensures that the workspace
// environment variables used in the code match the available documented keys.
// If new os.Getenv() keys are introduced but not added to .env.example,
// this check will fail the Definition of Done gate.

import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// runConfigDriftCheck scans the codebase for environment variable lookups
// and ensures they are defined in .env.example templates.
func runConfigDriftCheck(ctx *workspace.WorkspaceContext) (StageResult, error) {
	root := ctx.RepoRoot
	envPath := filepath.Join(root, ".env.example")
	content, err := os.ReadFile(envPath)
	if err != nil {
		return StageResult{Passed: true, Message: "No .env.example found, skipping drift check."}, nil
	}

	// Extract allowed env keys from .env.example
	allowedKeys := make(map[string]bool)
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) >= 1 {
			allowedKeys[strings.TrimSpace(parts[0])] = true
		}
	}

	// Search codebase for env lookups across multiple languages
	grepPattern := `os\.Getenv\("[A-Z0-9_]+"\)|process\.env\.[A-Z0-9_]+|os\.getenv\(['"][A-Z0-9_]+['"]\)|os\.environ\.get\(['"][A-Z0-9_]+['"]\)|os\.environ\[['"][A-Z0-9_]+['"]\]`
	cmd := exec.Command("git", "grep", "-E", grepPattern)
	cmd.Dir = root
	out, _ := cmd.Output()

	var missingKeys []string
	var matches [][]string

	regexes := []*regexp.Regexp{
		regexp.MustCompile(`os\.Getenv\("([A-Z0-9_]+)"\)`),
		regexp.MustCompile(`process\.env\.([A-Z0-9_]+)`),
		regexp.MustCompile(`os\.getenv\(['"]([A-Z0-9_]+)['"]\)`),
		regexp.MustCompile(`os\.environ\.get\(['"]([A-Z0-9_]+)['"]\)`),
		regexp.MustCompile(`os\.environ\[['"]([A-Z0-9_]+)['"]\]`),
	}

	for _, re := range regexes {
		matches = append(matches, re.FindAllStringSubmatch(string(out), -1)...)
	}

	processed := make(map[string]bool)
	for _, m := range matches {
		if len(m) > 1 {
			key := m[1]
			if !allowedKeys[key] && !processed[key] {
				missingKeys = append(missingKeys, key)
				processed[key] = true
			}
		}
	}

	if len(missingKeys) > 0 {
		return StageResult{
			Passed:  false,
			Message: fmt.Sprintf("Config drift detected! The following env keys are used in code but missing from .env.example: %v", missingKeys),
		}, nil
	}

	return StageResult{Passed: true, Message: "No configuration drift detected."}, nil
}
