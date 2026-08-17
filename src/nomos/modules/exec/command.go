package exec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FindRepoRoot traverses directory nodes upwards to discover the repository root.
// It checks for standard sentinel markers (.nomos, .agent, or .git) and falls back
// to the initial starting path if none are discovered.
func FindRepoRoot(start string) string {
	curr := start
	for {
		if _, err := os.Stat(filepath.Join(curr, ".agents")); err == nil {
			return curr
		}
		if _, err := os.Stat(filepath.Join(curr, ".git")); err == nil {
			return curr
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return ""
}

func isRepoRoot(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, ".agents")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return true
	}
	return false
}

// checkShellNix checks whether the workspace has a shell.nix configuration
// file to wrap background command execution environments.
func checkShellNix(cmdDir string) bool {
	if cmdDir != "" {
		shellNixPath := filepath.Join(cmdDir, "shell.nix")
		if _, err := os.Stat(shellNixPath); err == nil {
			return true
		}
	}
	return false
}

func isUnauthorizedArg(arg string, forbiddenPatterns []string) bool {
	lowerArg := strings.ToLower(arg)
	for _, pat := range forbiddenPatterns {
		if strings.Contains(lowerArg, pat) {
			return true
		}
	}
	return false
}

// isUnauthorizedCommand implements the Cognitive Firewall's lowest-level safeguard by
// proactively blocking explicitly forbidden mutating shell commands (e.g. chmod, chown)
// to prevent catastrophic filesystem ownership alterations by the autonomous agent.
func isUnauthorizedCommand(name string, args []string) bool {
	lowerName := strings.ToLower(name)
	if lowerName == "chmod" || lowerName == "chown" {
		return true
	}
	forbiddenPatterns := []string{
		"chmod", "chown",
		"chmod ", "chown ",
		"\tchmod", "\tchown",
		";chmod", ";chown",
		"&chmod", "&chown",
		"|chmod", "|chown",
	}
	for _, arg := range args {
		if isUnauthorizedArg(arg, forbiddenPatterns) {
			return true
		}
	}
	return false
}

// shouldWrapInNixShell determines if the requested command needs to be executed within
// an ephemeral Nix environment based on whether the command is locally available or
// declared in the repository's shell.nix file dependencies.
func shouldWrapInNixShell(cmdDir, name string) bool {
	if name == "nix-shell" {
		return false
	}
	// Check if already in system path
	_, err := exec.LookPath(name)
	if err == nil {
		return false
	}
	// Missing from path. Check if we have shell.nix
	shellNixPath := filepath.Join(cmdDir, "shell.nix")
	data, err := os.ReadFile(shellNixPath)
	if err != nil {
		return false
	}
	content := string(data)

	// Map common command names to Nix packages
	mappings := map[string]string{
		"gcloud": "google-cloud",
		"aider":  "aider",
		"npm":    "nodejs",
		"npx":    "nodejs",
		"node":   "nodejs",
		"tsc":    "typescript",
	}

	if pkg, ok := mappings[name]; ok {
		if strings.Contains(content, pkg) {
			return true
		}
	}
	return strings.Contains(content, name)
}

// createCommand resolves command path wrapping requirements and builds the Cmd instance.
func createCommand(dbPath string, cmdDir string, name string, args ...string) (*exec.Cmd, string, error) {
	if isUnauthorizedCommand(name, args) {
		return nil, "", fmt.Errorf("Security Violation: manual command executions containing 'chmod' or 'chown' are strictly prohibited by Nomos security policy")
	}

	cmd := buildExecCmd(cmdDir, name, args...)

	if cmdDir != "" {
		cmd.Dir = cmdDir
		cmd.Env = cleanGitEnv()
	}

	commandStr := name + " " + strings.Join(args, " ")
	return cmd, commandStr, nil
}

// buildExecCmd constructs the exec.Cmd instance, wrapping in nix-shell if required.
func buildExecCmd(cmdDir, name string, args ...string) *exec.Cmd {
	if os.Getenv("IN_NIX_SHELL") != "" || !shouldWrapInNixShell(cmdDir, name) {
		return exec.Command(name, args...)
	}
	runCmdStr := name
	if len(args) > 0 {
		runCmdStr = ShellEscapeArgs(name, args)
	}
	return exec.Command("nix-shell", "--run", runCmdStr)
}

// cleanGitEnv strips inherited git repository variables to avoid cross-repo pollution.
func cleanGitEnv() []string {
	var clean []string
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "GIT_DIR=") || strings.HasPrefix(env, "GIT_WORK_TREE=") || strings.HasPrefix(env, "GIT_INDEX_FILE=") || strings.HasPrefix(env, "GIT_PREFIX=") || strings.HasPrefix(env, "GIT_COMMON_DIR=") {
			continue
		}
		clean = append(clean, env)
	}
	return clean
}

// LookPath wraps os/exec.LookPath for external packages that are banned from importing os/exec.
func LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

// Command wraps os/exec.Command
func Command(name string, arg ...string) *exec.Cmd {
	if isUnauthorizedCommand(name, arg) {
		panic(fmt.Sprintf("Security Violation: manual command executions containing 'chmod' or 'chown' are strictly prohibited by Nomos security policy"))
	}
	return buildExecCmd("", name, arg...)
}

// CommandContext wraps os/exec.CommandContext
func CommandContext(ctx context.Context, name string, arg ...string) *exec.Cmd {
	if isUnauthorizedCommand(name, arg) {
		panic(fmt.Sprintf("Security Violation: manual command executions containing 'chmod' or 'chown' are strictly prohibited by Nomos security policy"))
	}
	if os.Getenv("IN_NIX_SHELL") != "" || !shouldWrapInNixShell("", name) {
		return exec.CommandContext(ctx, name, arg...)
	}
	runCmdStr := name
	if len(arg) > 0 {
		runCmdStr = ShellEscapeArgs(name, arg)
	}
	return exec.CommandContext(ctx, "nix-shell", "--run", runCmdStr)
}

// Cmd is an alias for os/exec.Cmd
type Cmd = exec.Cmd
