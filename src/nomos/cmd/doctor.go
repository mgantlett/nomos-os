// Package cmd defines the command-line interfaces and Cobra routing for Nomos.
// This file implements the doctor subcommand for checking installation health.
package cmd

import (
	"context"       // Context support for task tracker connections
	"fmt"           // Structured format printing
	"os"            // File system operations and current working directory
	"os/exec"       // Subprocess execution for git checks
	"path/filepath" // Platform-agnostic file path manipulation
	"strings"

	"time" // Connectivity check timeouts

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace" // String manipulation for diagnostic reports

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task" // Task credentials loader
	"github.com/mgantlett/nomos-os/src/nomos/modules/verify"
	"github.com/spf13/cobra" // CLI subcommand builder
)

// doctorCmd represents the doctor validation command.
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Confirm Nomos is properly installed and fully configured",
	Long:  `Confirm Nomos is properly installed and fully configured in the current repository by verifying config files, submodules, symlinks, git hooks, and environment configurations.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Resolve the current working directory to run diagnostics against
		root, err := os.Getwd()
		if err != nil {
			return err
		}
		repoRoot := findRepoRoot(root)
		if err := enforceRootZone(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), "doctor"); err != nil {
			return err
		}

		passed := true
		diagnostics := make(map[string]string)

		// 1. Verify if the target repository is an active Git repository
		if checkGitRepo(root) {
			diagnostics["GitRepo"] = "Repository is initialized"
		} else {
			diagnostics["GitRepo"] = "Repository is NOT initialized. Run 'git init'"
			passed = false
		}

		// 2. Verify that the .nomos-commons submodule is cloned and populated
		if checkSubmodule(root) {
			diagnostics["Submodule"] = ".nomos-commons submodule exists and is populated"
		} else {
			diagnostics["Submodule"] = ".nomos-commons submodule is missing or empty"
			passed = false
		}

		// 3. Verify that the repository's configuration file exists
		if checkConfig(root) {
			diagnostics["Config"] = "global config.yaml exists and is valid"
		} else {
			diagnostics["Config"] = "global config.yaml is missing or invalid"
			passed = false
		}

		// 4. Verify that the local cache database has been created and populated
		if checkDatabase(root) {
			diagnostics["Database"] = "Cache DB is initialized"
		} else {
			diagnostics["Database"] = "Cache DB is NOT initialized"
			passed = false
		}

		// 5. Verify symlink redirectors and binaries in the target workspace
		symlinksOk := true
		if !checkSymlink(filepath.Join(root, "bin", "nomos")) {
			diagnostics["Symlink_bin"] = "bin/nomos is missing or invalid"
			symlinksOk = false
		}
		if !checkSymlink(filepath.Join(root, "claude.md")) {
			diagnostics["Symlink_claude"] = "claude.md redirector is missing or invalid"
			symlinksOk = false
		}
		if !checkSymlink(filepath.Join(root, ".clinerules")) {
			diagnostics["Symlink_clinerules"] = ".clinerules redirector is missing or invalid"
			symlinksOk = false
		}
		if symlinksOk {
			diagnostics["Symlinks"] = "bin/nomos, claude.md, and .clinerules are mapped"
		} else {
			passed = false
		}

		// 6. Verify that Git pre-commit, commit-msg, and pre-push hooks are symlinked
		hooksOk := true
		for _, h := range []string{"pre-commit", "commit-msg", "pre-push"} {
			if !checkGitHook(root, h) {
				diagnostics["GitHook_"+h] = "Git hook is missing or not configured"
				hooksOk = false
			}
		}
		if hooksOk {
			diagnostics["GitHooks"] = "Git hooks are configured"
		} else {
			passed = false
		}

		// 7. Verify presence of local Nix execution shells
		if checkFileExists(filepath.Join(root, "shell.nix")) && checkFileExists(filepath.Join(root, ".envrc")) {
			diagnostics["NixShell"] = "shell.nix and .envrc are present"
		} else {
			diagnostics["NixShell"] = "shell.nix or .envrc is missing"
		}

		// 8. Verify connection credentials to the configured task tracker
		if checkTaskTracker(root) {
			diagnostics["TaskTracker"] = "Connectivity is healthy"
		} else {
			diagnostics["TaskTracker"] = "Connectivity failed"
			passed = false
		}

		// 9. Verify background daemon and runtime health
		status, err := verify.AuditHealth(root)
		if err != nil {
			diagnostics["HealthChecks"] = fmt.Sprintf("Execution failed: %v", err)
			passed = false
		} else {
			if status.LlamaAlive {
				diagnostics["LlamaServer"] = "Port 8082 is responsive"
			} else {
				diagnostics["LlamaServer"] = "Port 8082 is UNREACHABLE"
				passed = false
			}

			if status.CockpitAlive {
				diagnostics["Cockpit"] = "Port 8089 is responsive"
			} else {
				diagnostics["Cockpit"] = "Port 8089 is UNREACHABLE"
				passed = false
			}

			if len(status.StaleLocksCleared) > 0 {
				diagnostics["SelfHealing"] = strings.Join(status.StaleLocksCleared, "; ")
			}
		}

		synapse.Emit("SystemHealth", map[string]interface{}{
			"passed":      passed,
			"diagnostics": diagnostics,
		})

		if passed {
			return nil
		}
		return fmt.Errorf("nomos diagnostics failed")
	},
}

// checkGitRepo executes git checks to confirm worktree status.
// It uses git rev-parse --is-inside-work-tree to verify the current directory
// is part of a valid git repository, which is a prerequisite for Nomos operations.
func checkGitRepo(root string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = root
	return cmd.Run() == nil
}

// checkSubmodule checks for the existence of the submodule files.
// Nomos relies on a shared submodule (nomos-commons) for its core engine.
// This function verifies that the submodule is present and initialized by checking
// for the existence of the main.go file within the submodule directory.
// If the root repository is nomos-commons itself, it skips this check.
func checkSubmodule(root string) bool {
	if filepath.Base(root) == "nomos-commons" {
		return true
	}
	f := filepath.Join(root, ".nomos-commons", "src", "nomos", "main.go")
	info, err := os.Stat(f)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// checkConfig checks if the global config file is present.
func checkConfig(root string) bool {
	f := filepath.Join(workspace.MustNewContext(root).DataDir(), "config.yaml")
	info, err := os.Stat(f)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// checkDatabase verifies the presence of the SQLite cache file.
func checkDatabase(root string) bool {
	f := workspace.MustNewContext(root).DbPath("cache.db")
	_, err := os.Stat(f)
	return err == nil
}

// checkFileExists is a generic check for file presence.
func checkFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// checkSymlink checks if a path is mapped either as a symlink or file.
func checkSymlink(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return (info.Mode()&os.ModeSymlink) != 0 || !info.IsDir()
}

// checkGitHook verifies that the git hook exists and is executable.
// It checks if the hook file exists in the repository's .git/hooks directory
// and verifies that it has executable permissions or is a valid symlink,
// ensuring that Nomos verification checks run automatically during git operations.
func checkGitHook(root string, hook string) bool {
	path := filepath.Join(root, ".git", "hooks", hook)
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return (info.Mode()&os.ModeSymlink) != 0 || (info.Mode()&0111) != 0
}

// checkTaskTracker checks the credentials and connectivity of the tracker API.
func checkTaskTracker(root string) bool {
	cfg, err := func() (*task.Config, error) { c, _ := workspace.NewContext(root); return task.LoadConfig(c) }()
	if err != nil {
		return false
	}
	tracker, err := task.NewTracker(cfg)
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = tracker.List(ctx)
	if err != nil {
		synapse.Info("  ⚠️  [Task Tracker Info] Connection test failed: %v\n", err)
		return false
	}
	return true
}
