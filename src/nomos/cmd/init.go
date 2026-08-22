package cmd

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/assets"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	nomosexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/spf13/cobra"
)

// initCmd represents the init command which initializes a new workspace
// or updates an existing workspace's configuration and git hooks.
// It uses embedded files from the assets package to ensure the user always gets
// the latest canonical versions of the hooks without needing external template directories.
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize Nomos dependencies and configurations in the current workspace",
}

// initSyncCmd defines the Cobra subcommand responsible for rehydrating embedded
// system templates, workflow definitions, and AGENTS.md instructions directly
// into the current project workspace directory structure.
var initSyncCmd = &cobra.Command{
	Use:        "sync",
	Short:      "Synchronize embedded workflows and protocols into the local workspace",
	Deprecated: "use 'nomos update -w' instead",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Discover working directory and resolve top-level git repository root
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		repoRoot := findRepoRoot(wd)

		if err := enforceRootZone(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), "init"); err != nil {
			return err
		}

		return runInitSync(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())
	},
}

// runInitSync executes the synchronization of embedded workflows and protocols
// into the specified repository root. It is used by both the init sync command
// and during the handshake boot sequence.
func runInitSync(ctx *workspace.WorkspaceContext) error {
	repoRoot := ctx.RepoRoot
	// Rehydrate the workspace protocol and workflows from the Go substrate
	synapse.Info("Syncing embedded workflows and protocols into %s...", repoRoot)

	// Serialize root CLI command schema into JSON for AGENTS.md auto-generation.
	// Note: We don't need to rebuild the full CLI schema just to sync workflows,
	// but since RehydrateWorkspace expects it for AGENTS.md, we generate it.
	cliSchemaBytes, _ := json.Marshal(buildCliSchema(RootCmd))
	cliSchemaJSON := string(cliSchemaBytes)

	// Write embedded workflow files and project rules into the target repository
	if err := nomosexec.RehydrateWorkspace(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), cliSchemaJSON); err != nil {
		return fmt.Errorf("failed to rehydrate workspace: %w", err)
	}

	// Also install Git hooks during sync to maintain phase discipline
	if err := installHooks(repoRoot); err != nil {
		return fmt.Errorf("failed to install git hooks: %w", err)
	}

	// Scaffold Nix workspace resolution configurations for downstream repositories
	scaffoldNixEnvironment(repoRoot)

	// Scaffold virgin repository isolation configuration
	scaffoldVirginRepo(repoRoot)

	// Ensure the base clone is a hollow shell so stale files don't break verification gates
	autoConfigureHollowShell(repoRoot)

	// Scaffold the .explorer deterministic read-only worktree
	autoConfigureExplorerWorktree(repoRoot)

	synapse.Info(" ✅ Synchronization complete")
	synapse.Info("")
	autoConfigureIDEs()

	return nil
}

// installHooks encapsulates the logic to install the embedded Nomos Git hooks.
// It retrieves hook templates embedded directly within the Go binary assets and
// copies them to the target .git/hooks directory with executable permissions (0755).
func installHooks(root string) error {
	// Verify that the repository target directory contains a valid .git/hooks path
	gitHooksDir := filepath.Join(root, ".git", "hooks")
	if _, err := os.Stat(gitHooksDir); os.IsNotExist(err) {
		return fmt.Errorf(".git/hooks directory not found in repository root")
	}

	synapse.Info("%s", fmt.Sprint("Installing embedded Git hooks into ", gitHooksDir))
	embeddedFS := assets.GetTemplates()

	// Read hooks from embedded filesystem assets package
	hookFiles := []string{
		"templates/hooks/pre-commit",
		"templates/hooks/pre-push",
		"templates/hooks/commit-msg",
		"templates/hooks/post-merge",
		"templates/hooks/post-commit",
		"templates/hooks/phase/on_phase_change.sh",
	}

	// Iterate through embedded hook file definitions and deploy executable binaries
	for _, hookPath := range hookFiles {
		content, err := fs.ReadFile(embeddedFS, hookPath)
		if err != nil {
			return fmt.Errorf("failed to read embedded hook %s: %w", hookPath, err)
		}

		// Calculate target destination path on local disk
		fileName := filepath.Base(hookPath)
		var destPath string
		if fileName == "on_phase_change.sh" {
			// Sub-directory path for phase state change trigger hooks
			phaseDir := filepath.Join(gitHooksDir, "phase")
			os.MkdirAll(phaseDir, 0755)
			destPath = filepath.Join(phaseDir, fileName)
		} else {
			destPath = filepath.Join(gitHooksDir, fileName)
		}

		// Remove existing file/symlink to prevent errors if it's a broken symlink
		os.Remove(destPath)

		// Write executable hook script with read-write-execute permissions for user
		err = os.WriteFile(destPath, content, 0755)
		if err != nil {
			return fmt.Errorf("failed to write hook to %s: %w", destPath, err)
		}
		synapse.Info(" ✅ Installed %s\n", destPath)
	}

	return nil
}

// initHooksCmd provides backward compatibility for the deprecated `nomos init hooks` command.
var initHooksCmd = &cobra.Command{
	Use:        "hooks",
	Short:      "Install the embedded Nomos Git hooks into .git/hooks",
	Deprecated: "use 'nomos init sync' instead",
	RunE: func(cmd *cobra.Command, args []string) error {
		var root string
		// Query git executable directly to identify repository root path
		out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
		if err == nil {
			root = strings.TrimSpace(string(out))
		} else {
			root = "."
		}
		return installHooks(root)
	},
}

// init registers subcommands onto the parent Cobra `initCmd` during package initialization.
func init() {
	initCmd.AddCommand(initHooksCmd)
	initCmd.AddCommand(initSyncCmd)
}

// autoConfigureIDEs detects known AI coding agents and automatically symlinks the global customizations.
func autoConfigureIDEs() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}

	// Detect Antigravity IDE
	geminiConfigDir := filepath.Join(homeDir, ".gemini", "config")
	if _, err := os.Stat(geminiConfigDir); err == nil {
		synapse.Info(" 🤖 Antigravity IDE detected. Auto-configuring global customizations...")

		globalConfigDir := workspace.GlobalAgentConfigDir()

		// Symlink AGENTS.md
		agentsSrc := filepath.Join(globalConfigDir, "AGENTS.md")
		agentsDest := filepath.Join(homeDir, ".gemini", "GEMINI.md")
		os.RemoveAll(agentsDest)
		if err := os.Symlink(agentsSrc, agentsDest); err == nil {
			synapse.Info("    🔗 Symlinked GEMINI.md -> %s", agentsDest)
		} else {
			synapse.Info("    ⚠️  Failed to symlink GEMINI.md: %v", err)
		}

		// Symlink workflows
		workflowsSrc := filepath.Join(globalConfigDir, "workflows")
		workflowsDest := filepath.Join(geminiConfigDir, "global_workflows")
		os.RemoveAll(workflowsDest)
		if err := os.Symlink(workflowsSrc, workflowsDest); err == nil {
			synapse.Info("    🔗 Symlinked global_workflows/ -> %s", workflowsDest)
		} else {
			synapse.Info("    ⚠️  Failed to symlink global_workflows/: %v", err)
		}
	} else {
		// Fallback for unknown IDEs
		synapse.Info(" 💡 IDE SETUP REQUIRED for Tier 1 UX (Slash Commands & Global Rules)")
		synapse.Info("    Ensure your AI coding agent has mapped its global customizations root to:")
		synapse.Info("    %s", workspace.GlobalAgentConfigDir())
	}
	synapse.Info("")
}

// scaffoldNixEnvironment ensures that the downstream repository has a valid
// shell.nix and .envrc configured to native resolve the nomos-os binary
// instead of relying on a global zsh/bash alias hack.
func scaffoldNixEnvironment(repoRoot string) {
	shellNixPath := filepath.Join(repoRoot, "shell.nix")
	envrcPath := filepath.Join(repoRoot, ".envrc")

	nomosBinPath := workspace.MustNewContext(repoRoot).NomosOSBinPath()

	// 1. Scaffold or Parse shell.nix
	if _, err := os.Stat(shellNixPath); os.IsNotExist(err) {
		synapse.Info(" 📦 Generating default shell.nix for workspace isolation...")
		defaultShellNix := `{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  buildInputs = with pkgs; [
    go
    git
    jq
    sqlite
    curl
    nodejs
    pm2
  ];

  shellHook = ''
    # Add nomos-os and nomos-sovereign to path
    export PATH="` + nomosBinPath + `:/home/markg/Projects/sophialabs/private/nomos-sovereign/bin:$PATH"
  '';
}
`
		os.WriteFile(shellNixPath, []byte(defaultShellNix), 0644)
	}

	// 2. Scaffold or Update .envrc
	envrcContent := []byte{}
	if _, err := os.Stat(envrcPath); err == nil {
		envrcContent, _ = os.ReadFile(envrcPath)
	}

	envrcStr := string(envrcContent)
	needsNix := !strings.Contains(envrcStr, "use nix")
	needsPath := !strings.Contains(envrcStr, nomosBinPath)

	if needsNix || needsPath {
		synapse.Info(" 🔗 Updating .envrc with Nix / Nomos paths...")

		f, err := os.OpenFile(envrcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			defer f.Close()
			if needsNix && !strings.Contains(envrcStr, "use flake") {
				f.WriteString("\nuse nix\n")
			}
			if needsPath {
				f.WriteString(fmt.Sprintf("\nPATH_add %s\n", nomosBinPath))
			}
		}
	}
}

// autoConfigureHollowShell converts the root repository into a sparse checkout hollow shell
// to support the DDP Swarm worktree architecture while preserving root worktree behavior.
func autoConfigureHollowShell(repoRoot string) {
	// 1. Ensure the repo is NOT core.bare=true (Hollow Shells shouldn't be bare)
	cmd := exec.Command("git", "config", "--get", "core.bare")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	isBare := err == nil && strings.TrimSpace(string(out)) == "true"

	if isBare {
		synapse.Info(" 🐚 Restoring root repository from core.bare=true to normal (core.bare=false) for hollow shell...")
		setBareCmd := exec.Command("git", "config", "core.bare", "false")
		setBareCmd.Dir = repoRoot
		if err := setBareCmd.Run(); err != nil {
			synapse.Info("    ⚠️  Failed to set core.bare=false: %v", err)
		}
	}

	// 2. Enable extensions.worktreeConfig so that worktrees can disable sparse-checkout independently
	synapse.Info(" ⚙️  Enabling extensions.worktreeConfig...")
	wtConfigCmd := exec.Command("git", "config", "extensions.worktreeConfig", "true")
	wtConfigCmd.Dir = repoRoot
	if err := wtConfigCmd.Run(); err != nil {
		synapse.Info("    ⚠️  Failed to enable extensions.worktreeConfig: %v", err)
	}

	// 3. Configure Sparse Checkout Patterns
	synapse.Info(" 🐚 Configuring Sparse-Checkout Hollow Shell...")
	sparseFile := filepath.Join(repoRoot, ".git", "info", "sparse-checkout")

	settings, err := config.LoadProjectSettings(repoRoot)
	sparseLines := []string{"/*"}
	if err == nil && len(settings.SparseExclude) > 0 {
		for _, exclude := range settings.SparseExclude {
			sparseLines = append(sparseLines, fmt.Sprintf("!/%s/", strings.Trim(exclude, "/")))
		}
	} else {
		// Fallback
		sparseLines = append(sparseLines, "!/src/", "!/docs/")
	}
	sparseLines = append(sparseLines, "")
	sparseContent := []byte(strings.Join(sparseLines, "\n"))

	os.MkdirAll(filepath.Dir(sparseFile), 0755)
	if err := os.WriteFile(sparseFile, sparseContent, 0644); err != nil {
		synapse.Info("    ⚠️  Failed to write sparse-checkout patterns: %v", err)
	}

	// 4. Initialize and Reapply Sparse Checkout
	initCmd := exec.Command("git", "sparse-checkout", "init", "--no-cone")
	initCmd.Dir = repoRoot
	initCmd.Run()

	reapplyCmd := exec.Command("git", "sparse-checkout", "reapply")
	reapplyCmd.Dir = repoRoot
	if err := reapplyCmd.Run(); err != nil {
		synapse.Info("    ⚠️  Failed to reapply sparse-checkout: %v", err)
	} else {
		synapse.Info("    ✅ Successfully configured Hollow Shell using generic sparse exclusions.")
	}
}

// autoConfigureExplorerWorktree scaffolds a deterministic .explorer read-only worktree
// configured with reverse sparse checkout to ONLY show the files hidden in the root workspace.
func autoConfigureExplorerWorktree(repoRoot string) {
	explorerDir := filepath.Join(repoRoot, "worktrees", ".explorer")
	synapse.Info(" 🔭 Scaffolding Deterministic Explorer Worktree...")

	// 1. Create the explorer-sync branch if it doesn't exist
	// Using --no-track explicitly prevents VSCode from displaying 'Sync Changes' prompts
	cmdBranch := exec.Command("git", "-C", repoRoot, "branch", "--no-track", "explorer-sync", "develop")
	if err := cmdBranch.Run(); err != nil {
		// If the branch already exists, explicitly disconnect it from any remote tracking
		// to ensure the VSCode Git Graph stops showing "Outgoing Changes".
		cmdUnset := exec.Command("git", "-C", repoRoot, "branch", "--unset-upstream", "explorer-sync")
		_ = cmdUnset.Run()
	}

	// 2. Create the worktree if it doesn't exist
	if _, err := os.Stat(explorerDir); os.IsNotExist(err) {
		cmdWt := exec.Command("git", "-C", repoRoot, "worktree", "add", explorerDir, "explorer-sync")
		if errWt := cmdWt.Run(); errWt != nil {
			synapse.Info("    ⚠️  Failed to create .explorer worktree: %v", errWt)
			return
		}
	} else {
		// Fast-forward existing worktree
		cmdPull := exec.Command("git", "-C", explorerDir, "pull", "origin", "develop")
		if err := cmdPull.Run(); err != nil {
			// Fallback to merge local develop if origin doesn't exist
			cmdMerge := exec.Command("git", "-C", explorerDir, "merge", "develop")
			_ = cmdMerge.Run()
		}
	}

	// 3. Isolate worktree config
	cmdCfg := exec.Command("git", "-C", explorerDir, "config", "--worktree", "core.sparseCheckout", "true")
	_ = cmdCfg.Run()

	// 4. Configure reverse sparse checkout patterns based on config.yaml
	settings, err := config.LoadProjectSettings(repoRoot)
	sparseLines := []string{"/*", "!/*/"}
	if err == nil && len(settings.SparseExclude) > 0 {
		for _, exclude := range settings.SparseExclude {
			sparseLines = append(sparseLines, fmt.Sprintf("/%s/", strings.Trim(exclude, "/")))
		}
	} else {
		// Fallback
		sparseLines = append(sparseLines, "/src/", "/docs/")
	}

	initCmd := exec.Command("git", "-C", explorerDir, "sparse-checkout", "init", "--no-cone")
	_ = initCmd.Run()

	setCmd := exec.Command("git", "-C", explorerDir, "sparse-checkout", "set", "--no-cone", "--stdin")
	setCmd.Stdin = strings.NewReader(strings.Join(sparseLines, "\n"))
	if err := setCmd.Run(); err != nil {
		synapse.Info("    ⚠️  Failed to configure explorer sparse checkout: %v", err)
	} else {
		synapse.Info("    ✅ Successfully configured .explorer worktree with reverse sparse checkout.")
	}
}

// scaffoldVirginRepo ensures that a new repository is configured with
// project-level isolated SQLite databases and proper gitignore rules.
func scaffoldVirginRepo(repoRoot string) {
	synapse.Info(" 🌱 Scaffolding virgin repository isolation...")

	// 1. Scaffold .gitignore
	gitIgnorePath := filepath.Join(repoRoot, ".gitignore")
	if _, err := os.Stat(gitIgnorePath); os.IsNotExist(err) {
		synapse.Info("    📝 Generating default .gitignore...")
		defaultGitIgnore := `# Nomos Exclusions
.nomos/*
!.nomos/data/config.yaml
.nomos_backup/*
bin/

# Agent Exclusions
.agent/tmp/
.agent/state/
.agent/locks/
.agent/logs/
.agent/.*
.agent/*.db
.agent/*.pid
.gitbrain*.db
.phase_state.json

# Standard Exclusions
tmp/
/worktrees/
.envrc
.direnv/
`
		os.WriteFile(gitIgnorePath, []byte(defaultGitIgnore), 0644)
	}

	// 2. Ensure .nomos/data/db exists
	dbDir := filepath.Join(repoRoot, ".nomos", "data", "db")
	os.MkdirAll(dbDir, 0755)

	// 3. Scaffold config.yaml
	configPath := filepath.Join(repoRoot, ".nomos", "data", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		synapse.Info("    ⚙️  Generating localized config.yaml for database isolation...")
		projectName := filepath.Base(filepath.Clean(repoRoot))
		
		defaultConfig := fmt.Sprintf(`env: development
db_path: cache.db
agent_dir: .agents
sparse_exclude:
  - "src"
  - "docs"
task_tracker_db_path: "%s/.nomos/data/db/graph.db"
gitbrain_db_path: "%s/.nomos/data/db/gitbrain.db"
default_project: "%s"
embedding_url: "http://localhost:8081/v1/embeddings"
`, repoRoot, repoRoot, projectName)
		os.WriteFile(configPath, []byte(defaultConfig), 0644)
	}
}
