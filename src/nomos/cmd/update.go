// Package cmd defines the command-line interfaces and Cobra routing for Nomos.
// This file implements the update subcommand to update the global Nomos binary.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/plugin"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/spf13/cobra"
)

type toolConfig struct {
	RepoURL     string
	RepoDirName string
	BuildSubDir string
	BinaryName  string
	Branch      string
}

var ecosystemTools = map[string]toolConfig{
	"nomos": {
		RepoURL:     "git@github.com:mgantlett/nomos-commons.git",
		RepoDirName: "nomos-commons",
		BuildSubDir: "src/nomos",
		BinaryName:  "nomos",
		Branch:      "main",
	},
	"nomos-sovereign": {
		RepoURL:     "git@github.com:mgantlett/nomos-sovereign.git",
		RepoDirName: "nomos-sovereign",
		BuildSubDir: ".",
		BinaryName:  "nomos-sovereign",
		Branch:      "master",
	},
}

var updateAll bool
var globalInstall bool

func init() {
	// Add CLI flags to control update targets and scoping
	updateCmd.Flags().BoolVar(&updateAll, "all", false, "Update core Nomos and the Sovereign submodule")
	updateCmd.Flags().BoolVarP(&globalInstall, "install", "i", false, "Compile and install global binaries from pristine remote source")
}

// updateTool handles the compilation and installation of a specific ecosystem tool.
// It clones the source repository into the ~/.nomos/src directory if it doesn't exist,
// or pulls the latest changes if it does. It then invokes the appropriate build system
// (Nix if available, fallback to bash) to compile the tool and drops the final binary
// into the active workspace's bin/ path, maintaining isolation.
func updateTool(repoRoot string, name string, config toolConfig) error {
	synapse.Info("🔄 Updating %s...", name)

	srcDir := filepath.Join(repoRoot, ".nomos", "data", "src")
	repoPath := filepath.Join(srcDir, config.RepoDirName)

	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return fmt.Errorf("failed to create src directory: %w", err)
	}

	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		synapse.Info("⬇️  Cloning pristine source for %s...", name)
		cloneCmd := exec.Command("git", "clone", config.RepoURL)
		cloneCmd.Dir = srcDir
		cloneCmd.Stdout = os.Stdout
		cloneCmd.Stderr = os.Stderr
		if err := cloneCmd.Run(); err != nil {
			return fmt.Errorf("failed to clone %s: %w", config.RepoDirName, err)
		}
	} else {
		synapse.Info("⬇️  Pulling latest stable changes for %s...", name)
		checkoutCmd := exec.Command("git", "checkout", config.Branch)
		checkoutCmd.Dir = repoPath
		_ = checkoutCmd.Run() // Best effort

		pullCmd := exec.Command("git", "pull", "origin", config.Branch)
		pullCmd.Dir = repoPath
		pullCmd.Stdout = os.Stdout
		pullCmd.Stderr = os.Stderr
		if err := pullCmd.Run(); err != nil {
			return fmt.Errorf("failed to pull %s: %w", config.RepoDirName, err)
		}
	}

	synapse.Info("⚙️  Compiling %s...", name)
	repoBin := filepath.Join(repoRoot, "bin", config.BinaryName)
	buildStr := fmt.Sprintf("cd %s && go build -o %s .", config.BuildSubDir, repoBin)

	var buildCmd *exec.Cmd
	if _, err := os.Stat(filepath.Join(repoPath, "shell.nix")); err == nil {
		buildCmd = exec.Command("nix-shell", "--command", buildStr)
	} else {
		buildCmd = exec.Command("bash", "-c", buildStr)
	}

	buildCmd.Dir = repoPath
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to compile %s: %w", name, err)
	}

	return nil
}

// updateCmd updates the global Nomos engine binary and performs workspace hygiene.
var updateCmd = &cobra.Command{
	Use:   "update [tools...]",
	Short: "Update the global Nomos engine binary and workspace links",
	RunE: func(cmd *cobra.Command, args []string) error {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
		repoRoot := findRepoRoot(wd)
		if repoRoot == "" {
			return fmt.Errorf("must run update from within a Nomos workspace")
		}

		var targets []string
		if updateAll {
			targets = []string{"nomos", "nomos-sovereign"}
		} else if len(args) > 0 {
			targets = args
		} else {
			targets = []string{"nomos"}
		}

		if globalInstall {
			for _, target := range targets {
				target = strings.ToLower(target)
				config, exists := ecosystemTools[target]
				if !exists {
					synapse.Info("⚠️  Unknown tool '%s'. Available: nomos, gitbrain, swarm, cockpit", target)
					continue
				}

				if err := updateTool(repoRoot, target, config); err != nil {
					return err
				}
			}
		} else {
			synapse.Info("⚠️  Skipping binary compilation (--install flag not active)")
		}

		if err := enforceRootZone(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), "update"); err != nil {
			return err
		}

		// Synchronize local workspace protocols, hooks, and schemas
		if err := runInitSync(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }()); err != nil {
			synapse.Info("Warning: failed to synchronize workspace: %v\n", err)
		}

		// Scaffold NixOS plugin configuration structures downstream
		if err := plugin.ScaffoldNixosPlugin(repoRoot); err != nil {
			synapse.Info("Warning: failed to scaffold NixOS plugin: %v\n", err)
		}

		// Run automated workspace hygiene cleanups
		_ = RunHygieneCleanups(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())

		synapse.Info("✅ Global update complete for %v!", targets)
		return nil
	},
}
