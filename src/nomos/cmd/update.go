// Package cmd defines the command-line interfaces and Cobra routing for Nomos.
// This file implements the update subcommand to update the global Nomos binary.
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	plugin "github.com/mgantlett/nomos-commons/src/nomos/core/plugin"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
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
	"gitbrain": {
		RepoURL:     "git@github.com:mgantlett/nomos-gitbrain.git",
		RepoDirName: "nomos-gitbrain",
		BuildSubDir: "src/cmd/nomos-gitbrain",
		BinaryName:  "nomos-gitbrain",
		Branch:      "master",
	},
	"swarm": {
		RepoURL:     "git@github.com:mgantlett/nomos-swarm.git",
		RepoDirName: "nomos-swarm",
		BuildSubDir: "src/cmd/nomos-swarm",
		BinaryName:  "nomos-swarm",
		Branch:      "master",
	},
	"cockpit": {
		RepoURL:     "git@github.com:mgantlett/nomos-cockpit.git",
		RepoDirName: "nomos-cockpit",
		BuildSubDir: "src/cmd/cockpitd",
		BinaryName:  "nomos-cockpit",
		Branch:      "master",
	},
}

var updateAll bool
var workspaceOnly bool

func init() {
	// Add CLI flags to control update targets and scoping
	updateCmd.Flags().BoolVar(&updateAll, "all", false, "Update all ecosystem tools (nomos, gitbrain, swarm, cockpit)")
	updateCmd.Flags().BoolVarP(&workspaceOnly, "workspace", "w", false, "Only synchronize local workspace protocols and hygiene (skip global binary compilation)")
}

// updateTool handles the compilation and installation of a specific ecosystem tool.
// It clones the source repository into the ~/.nomos/src directory if it doesn't exist,
// or pulls the latest changes if it does. It then invokes the appropriate build system
// (Nix if available, fallback to bash) to compile the tool and drops the final binary
// into the user's ~/.local/bin path, making it globally available.
func updateTool(home string, name string, config toolConfig) error {
	synapse.Info("🔄 Updating %s...", name)

	srcDir := filepath.Join(home, ".nomos", "src")
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
	homeBin := filepath.Join(home, ".local", "bin", config.BinaryName)
	buildStr := fmt.Sprintf("cd %s && go build -o %s .", config.BuildSubDir, homeBin)

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
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}

		var targets []string
		if updateAll {
			targets = []string{"nomos", "gitbrain", "swarm", "cockpit"}
		} else if len(args) > 0 {
			targets = args
		} else {
			targets = []string{"nomos"}
		}

		if !workspaceOnly {
			for _, target := range targets {
				target = strings.ToLower(target)
				config, exists := ecosystemTools[target]
				if !exists {
					synapse.Info("⚠️  Unknown tool '%s'. Available: nomos, gitbrain, swarm, cockpit", target)
					continue
				}

				if err := updateTool(home, target, config); err != nil {
					return err
				}
			}
		} else {
			synapse.Info("⚠️  Skipping global binary compilation (--workspace flag active)")
		}

		// Fetch current working directory for local workspace updates
		if wd, err := os.Getwd(); err == nil {
			repoRoot := findRepoRoot(wd)

			if err := enforceRootZone(repoRoot, "update"); err != nil {
				return err
			}

			if err := enforceRootZone(repoRoot, "update"); err != nil {
				return err
			}

			// Synchronize local workspace protocols, hooks, and schemas
			if err := runInitSync(repoRoot); err != nil {
				synapse.Info("Warning: failed to synchronize workspace: %v\n", err)
			}

			// Scaffold NixOS plugin configuration structures downstream
			if err := plugin.ScaffoldNixosPlugin(repoRoot); err != nil {
				synapse.Info("Warning: failed to scaffold NixOS plugin: %v\n", err)
			}

			// Run automated workspace hygiene cleanups
			_ = RunHygieneCleanups(repoRoot)
		}

		synapse.Info("✅ Global update complete for %v!", targets)
		return nil
	},
}
