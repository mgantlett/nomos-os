// Package cmd provides the CLI commands for the Nomos orchestrator.
// This package is responsible for bridging the user intent from the command line
// into the core logic of the Nomos framework.
package cmd

import (
	"context"
	"fmt"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-commons/src/nomos/core/telemetry"
	"github.com/mgantlett/nomos-os/src/nomos/modules/gitops"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/mgantlett/nomos-os/src/nomos/modules/verify"
	"github.com/mgantlett/nomos-os/src/nomos/modules/workspace"
	"github.com/spf13/cobra"
)

// pushCmd represents the push command which handles GitOps deployments.
// It acts as the final gatekeeper in the workflow, enforcing that all DoD checks pass
// and that the working directory is completely clean before allowing the developer
// or AI agent to merge and push branches to the remote repository.
//
// The Push Flow entails several strict steps:
//  1. Pre-Flight Working Tree Check: Ensures that `git status` is clean, aborting if
//     there are uncommitted changes, unless those changes are purely Nomos JSON state files.
//  2. Definition of Done (DoD) Verification: Invokes the verification suite to run static
//     analysis, cyclomatic complexity bounds, comment density checks, test coverage assertions,
//     and code duplication (DRY) checks. If any gate fails, push is hard-aborted.
//  3. Binary Compilation: Attempts to cut a release binary via standard Go build tooling,
//     asserting that the code compiles successfully.
//  4. State/Task Binding: Extracts any "Closes XXXX" strings from the Git log to automatically
//     transition corresponding active tasks in the Nomos tracker to the DONE phase upon successful push.
//  5. GitFlow Automation: If on a feature branch, it systematically checks out the target environment branch,
//     pulls the latest upstream refs, performs a no-edit merge, and ultimately pushes to the origin remote.
var pushCmd = &cobra.Command{
	Use:   "push [develop|master]",
	Short: "Push active code changes and run verification gates",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetEnv := "develop"
		if len(args) > 0 {
			targetEnv = args[0]
		}

		if targetEnv != "develop" && targetEnv != "master" {
			return fmt.Errorf("invalid target environment: %s (must be develop or master)", targetEnv)
		}

		wd, err := os.Getwd()
		if err != nil {
			return err
		}

		repoRoot := findRepoRoot(wd)

		if err := enforceWorktreeZone(repoRoot, "push"); err != nil {
			return err
		}

		// -----------------------------------------------------------------------------
		// ANTI-ROGUE AGENT GATE
		// Nomos operates entirely via autonomous AI agents and strictly enforces the
		// Deterministic Delivery Protocol (DDP). Because LLMs are stochastic and can
		// sometimes hallucinate commands or bypass intended workflows, we must enforce
		// these boundaries at the Go binary level.
		//
		// If an AI agent attempts to bypass the `nomos task merge` release mechanism
		// by manually invoking `nomos push` while it is bound to an active sprint
		// task (i.e. in the EDIT phase), this logic will intercept the command and
		// throw a hard runtime panic. This mathematically guarantees that all releases
		// are atomically orchestrated through `task_merge.go`, which handles cross-repo
		// synchronization, DoD gating, and ticket closure safely.
		// -----------------------------------------------------------------------------
		taskID := verify.GetActiveTaskId(repoRoot)
		if taskID != "" && taskID != "UNKNOWN" {
			return fmt.Errorf("DDP VIOLATION: Manual 'nomos push' is strictly forbidden during an active task cycle. You must execute the atomic release using 'nomos task merge -F walkthrough.md'")
		}

		synapse.Info("🚀 Starting push flow to target environment: %s\n", targetEnv)

		// Process cross-repo transient worktrees before root workspace push
		if err := processCrossRepoWorktrees(repoRoot, targetEnv); err != nil {
			return fmt.Errorf("cross-repo worktree push failed: %w", err)
		}

		// 0. Pre-Flight Working Tree Check: ensure working tree is 100% clean before executing push
		gitStatusCmd := exec.Command("git", "status", "--porcelain")
		gitStatusCmd.Dir = repoRoot
		if statusOut, err := gitStatusCmd.CombinedOutput(); err == nil {
			lines := strings.Split(strings.TrimSpace(string(statusOut)), "\n")
			dirty := false
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				// Ignore modifications to task JSON files
				if config.IsNomosTaskFile(line) {
					continue
				}
				dirty = true
				break
			}
			if dirty {
				return fmt.Errorf("push aborted: working tree contains uncommitted changes. Please run 'bin/nomos commit' to pass quality gates and commit your changes before pushing\n\nUncommitted changes:\n%s", string(statusOut))
			}
		}

		// 1. Run Definition of Done Gates
		synapse.Info("%s", fmt.Sprint("🛡️  Running Definition of Done (DoD) verification gates..."))
		os.Setenv("NOMOS_IN_GIT_HOOK", "1")
		if err := verify.VerifyDoD(repoRoot); err != nil {
			os.Unsetenv("NOMOS_IN_GIT_HOOK")
			return fmt.Errorf("definition of done checks failed, push aborted: %w", err)
		}
		os.Unsetenv("NOMOS_IN_GIT_HOOK")
		synapse.Info("%s", fmt.Sprint("✅ DoD verification gates passed."))

		// 1.5. Cut a release build by executing Go compiler targeting output binary.
		// It checks if workspace is running inside a subfolder or root worktree.
		buildDir := repoRoot
		outPath := "bin/nomos"
		if stat, err := os.Stat(filepath.Join(repoRoot, ".nomos-commons")); err == nil && stat.IsDir() {
			buildDir = filepath.Join(repoRoot, ".nomos-commons")
			outPath = filepath.Join("..", "bin", "nomos")
		}

		var mainPath string
		if _, err := os.Stat(filepath.Join(buildDir, "src", "nomos", "main.go")); err == nil {
			mainPath = filepath.Join("src", "nomos", "main.go")
		} else if _, err := os.Stat(filepath.Join(buildDir, "src", "cmd")); err == nil {
			entries, _ := os.ReadDir(filepath.Join(buildDir, "src", "cmd"))
			for _, e := range entries {
				if e.IsDir() {
					cand := filepath.Join("src", "cmd", e.Name(), "main.go")
					if _, err := os.Stat(filepath.Join(buildDir, cand)); err == nil {
						mainPath = cand
						break
					}
				}
			}
		} else if _, err := os.Stat(filepath.Join(buildDir, "src", "main.go")); err == nil {
			mainPath = filepath.Join("src", "main.go")
		} else if _, err := os.Stat(filepath.Join(buildDir, "main.go")); err == nil {
			mainPath = "main.go"
		}

		if mainPath != "" {
			projectName := filepath.Base(filepath.Clean(repoRoot))
			if outPath == "bin/nomos" && projectName != "nomos-commons" && projectName != "nomos" {
				outPath = filepath.Join("bin", projectName)
			}
			synapse.Info("🔨 Compiling binary release (%s)...\n", mainPath)
			buildCmd := exec.Command("go", "build", "-o", outPath, mainPath)
			buildCmd.Dir = buildDir
			if buildOut, err := buildCmd.CombinedOutput(); err != nil {
				return fmt.Errorf("failed to build binary: %s\n%w", string(buildOut), err)
			}
			synapse.Info("%s", fmt.Sprint("✅ Binary compiled successfully."))
		} else {
			synapse.Info("%s", fmt.Sprint("⏭️ No Go main entry point found, skipping binary compilation."))
		}

		// Helper to run git command
		runGit := func(args ...string) (string, error) {
			gitCmd := exec.Command("git", args...)
			gitCmd.Dir = repoRoot
			out, err := gitCmd.CombinedOutput()
			return strings.TrimSpace(string(out)), err
		}

		// Get current branch
		currBranch, err := runGit("rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return fmt.Errorf("failed to get current branch: %w", err)
		}

		originalBranch := currBranch
		synapse.Info("Current branch is: %s\n", currBranch)

		// 2. Extract Task IDs from unpushed commits for auto-closure
		var autoCloseKeys []string
		unpushedLog, err := runGit("log", fmt.Sprintf("origin/%s..HEAD", targetEnv), "--format=%B")
		if err == nil && unpushedLog != "" {
			re := regexp.MustCompile(`(?i)(Resolves|Closes|Fixes)[[:space:]]*([A-Z]+-[0-9]+|[0-9]+)`)
			matches := re.FindAllStringSubmatch(unpushedLog, -1)
			seen := make(map[string]bool)
			for _, match := range matches {
				if len(match) > 2 {
					key := strings.ToUpper(match[2])
					if !seen[key] {
						seen[key] = true
						autoCloseKeys = append(autoCloseKeys, key)
					}
				}
			}
		}

		// Add active task ID from state
		activeTaskId := verify.GetActiveTaskId(repoRoot)
		if activeTaskId != "" {
			found := false
			for _, k := range autoCloseKeys {
				if k == activeTaskId {
					found = true
					break
				}
			}
			if !found {
				autoCloseKeys = append(autoCloseKeys, activeTaskId)
			}
		}

		// 3. GitFlow Merge & Push
		if targetEnv == "develop" && currBranch != "develop" && currBranch != "master" {
			synapse.Info("🔀 GitFlow: Merging feature branch '%s' into 'develop'...\n", currBranch)

			// Temporarily unlock workspace to allow git checkout
			_ = task.TransitionPhase(repoRoot, statepkg.PhaseEdit)

			// Checkout develop
			if _, err := runGit("checkout", "develop"); err != nil {
				// Try creating it if it doesn't exist
				if _, err2 := runGit("checkout", "-b", "develop"); err2 != nil {
					return fmt.Errorf("failed to checkout develop branch: %w", err2)
				}
			}

			// Pull develop
			_, _ = runGit("pull", "origin", "develop")

			// Merge feature branch
			if _, err := runGit("merge", currBranch, "--no-edit"); err != nil {
				return fmt.Errorf("merge conflict detected, please resolve manually on develop: %w", err)
			}

			currBranch = "develop"

			// Auto-Prune local feature branch
			synapse.Info("🧹 Auto-Pruning: Deleting merged local feature branch '%s'...\n", originalBranch)
			if _, err := runGit("branch", "-d", originalBranch); err != nil {
				synapse.Info("Warning: failed to delete branch %s: %v\n", originalBranch, err)
			}
		}

		if targetEnv == "master" && currBranch == "develop" {
			synapse.Info("%s", fmt.Sprint("🔀 GitFlow: Merging 'develop' into 'master'..."))

			// Checkout master
			if _, err := runGit("checkout", "master"); err != nil {
				if _, err2 := runGit("checkout", "-b", "master"); err2 != nil {
					return fmt.Errorf("failed to checkout master branch: %w", err2)
				}
			}

			// Pull master
			_, _ = runGit("pull", "origin", "master")

			// Merge develop
			if _, err := runGit("merge", "develop", "--no-edit"); err != nil {
				return fmt.Errorf("merge conflict detected, please resolve manually on master: %w", err)
			}

			currBranch = "master"
		}

		// Query git show stats for the most recent HEAD commit.
		shortStat, _ := runGit("show", "--shortstat", "--oneline", "HEAD")
		statStr := "No diff stats available"
		lines := strings.Split(shortStat, "\n")
		// Extract only the final summary line containing insertion and deletion counts.
		if len(lines) > 1 {
			statStr = strings.TrimSpace(lines[len(lines)-1])
		}

		// Retrieve the full commit SHA hash for remote linkage.
		commitSha, _ := runGit("rev-parse", "HEAD")
		// Query origin remote tracking endpoint.
		remoteUrl, _ := runGit("config", "--get", "remote.origin.url")
		githubUrl := ""
		if remoteUrl != "" {
			// Convert SSH git endpoint URLs to standard web URLs.
			cleaned := remoteUrl
			cleaned = strings.TrimPrefix(cleaned, "git@github.com:")
			cleaned = strings.TrimPrefix(cleaned, "https://github.com/")
			cleaned = strings.TrimSuffix(cleaned, ".git")
			if commitSha != "" {
				githubUrl = fmt.Sprintf("https://github.com/%s/commit/%s", cleaned, commitSha)
			}
		}

		// 4. Auto-Close Tasks: Process pending sprint tasks and update closed state telemetry
		if len(autoCloseKeys) > 0 {
			// Load task tracker configuration from active workspace root
			cfg, err := task.LoadConfig(repoRoot)
			if err == nil {
				// Instantiate task tracker instance for active workspace
				tracker, err := task.NewTracker(cfg)
				if err == nil {
					ctx := context.Background()

					// Collect child task keys bundled under parent epics or stories
					var bundledChildren []string
					for _, key := range autoCloseKeys {
						// Retrieve individual task view from tracker store
						t, err := tracker.View(ctx, key)
						if err == nil && strings.Contains(t.Description, "**Bundled Tasks:**") {
							// Parse child task references from description metadata section
							parts := strings.Split(t.Description, "**Bundled Tasks:**")
							if len(parts) > 1 {
								childrenStr := strings.TrimSpace(parts[1])
								childrenLine := strings.Split(childrenStr, "\n")[0]
								children := strings.Split(childrenLine, ",")
								for _, child := range children {
									child = strings.TrimSpace(child)
									if child != "" {
										bundledChildren = append(bundledChildren, child)
									}
								}
							}
						}
					}
					// Append discovered child tasks to auto-closure queue
					autoCloseKeys = append(autoCloseKeys, bundledChildren...)

					// De-duplicate task closure keys to avoid redundant backend operations
					var splitKeys []string
					for _, k := range autoCloseKeys {
						parts := strings.Split(k, ",")
						for _, p := range parts {
							p = strings.TrimSpace(p)
							if p != "" {
								found := false
								for _, sk := range splitKeys {
									if sk == p {
										found = true
										break
									}
								}
								if !found {
									splitKeys = append(splitKeys, p)
								}
							}
						}
					}
					autoCloseKeys = splitKeys

					synapse.Info("%s", fmt.Sprint("✅ Executing post-push task auto-closures..."))
					for _, key := range autoCloseKeys {
						synapse.Info("   ↳ Auto-closing %s...\n", key)

						// Query git details
						sha, _ := runGit("rev-parse", "HEAD")
						commitMsg, _ := runGit("log", "-1", "--pretty=%B")
						branch, _ := runGit("rev-parse", "--abbrev-ref", "HEAD")

						// Try parsing walkthrough summary
						walkthroughContent := ""
						walkthroughPath := filepath.Join(config.GlobalDataDir(repoRoot), "walkthroughs", key+".md")
						if data, err := os.ReadFile(walkthroughPath); err == nil {
							walkthroughContent = string(data)
						} else {
							// fallback check
							files, _ := os.ReadDir(filepath.Join(config.GlobalDataDir(repoRoot), "walkthroughs"))
							for _, f := range files {
								if f.IsDir() && strings.Contains(f.Name(), key) {
									if data, err := os.ReadFile(filepath.Join(config.GlobalDataDir(repoRoot), "walkthroughs", f.Name()+".md")); err == nil {
										walkthroughContent = string(data)
										break
									}
								}
							}
						}

						// Construct rich markdown comment containing Git stats and GitHub commit URL.
						var commentBuilder strings.Builder
						commentBuilder.WriteString("🎉 **Task Completed & Pushed successfully!**\n\n")
						commentBuilder.WriteString("### 🚀 Deployment & GitOps Telemetry\n")
						commentBuilder.WriteString(fmt.Sprintf("- **Branch:** `%s`\n", branch))
						if githubUrl != "" && sha != "" {
							shortSha := sha
							if len(shortSha) > 8 {
								shortSha = shortSha[:8]
							}
							commentBuilder.WriteString(fmt.Sprintf("- **Commit:** [%s](%s)\n", shortSha, githubUrl))
						} else {
							commentBuilder.WriteString(fmt.Sprintf("- **Commit SHA:** `%s`\n", sha))
						}
						// Append lines added and removed counts to close ticket description.
						commentBuilder.WriteString(fmt.Sprintf("- **Lines Changed:** %s\n", statStr))
						commentBuilder.WriteString("\n### 📝 Git Commit Details\n")
						commentBuilder.WriteString("```\n")
						commentBuilder.WriteString(commitMsg)
						commentBuilder.WriteString("\n```\n")

						if walkthroughContent != "" {
							commentBuilder.WriteString("\n### 📖 Walkthrough / Specs Checklist\n")
							commentBuilder.WriteString(walkthroughContent)
						}

						_ = tracker.Close(ctx, key, commentBuilder.String())
						verify.PruneQualityDebtForTask(repoRoot, key)
					}
				}
			}
		}

		// Push to remote
		synapse.Info("🔄 GitOps Sync: Pushing '%s' to remote origin...\n", currBranch)
		if targetEnv == "master" {
			if _, err := runGit("push", "origin", "master", "--tags", "--no-verify"); err != nil {
				return fmt.Errorf("failed to push master to origin: %w", err)
			}
		} else {
			if _, err := runGit("push", "origin", "develop", "--no-verify"); err != nil {
				return fmt.Errorf("failed to push develop to origin: %w", err)
			}

			// Auto-sync main downstream channel
			synapse.Info("%s", fmt.Sprint("🚀 Auto-Synchronizing downstream release channel (main)..."))
			if _, err := runGit("push", "origin", "develop:main", "--tags", "--no-verify"); err != nil {
				synapse.Info("⚠️  Warning: failed to auto-sync main branch: %v\n", err)
			}
		}

		// Multi-Repo Root Workspace Sync: push sibling root repositories (e.g. nomos-sovereign)
		syncMultiRepoRootPushes(repoRoot, targetEnv)

		// Compile human-readable deploy stats message.
		deployMsg := fmt.Sprintf("Successfully pushed branch '%s' to remote origin (target environment: %s).", currBranch, targetEnv)
		if commitSha != "" {
			shortSha := commitSha
			if len(shortSha) > 8 {
				shortSha = shortSha[:8]
			}
			if githubUrl != "" {
				deployMsg = fmt.Sprintf("Successfully pushed branch '%s' to remote origin (target environment: %s).\n\n**Commit Details:**\n- **Stats**: %s\n- **GitHub Link**: [%s](%s)", currBranch, targetEnv, statStr, shortSha, githubUrl)
			} else {
				deployMsg = fmt.Sprintf("Successfully pushed branch '%s' to remote origin (target environment: %s).\n\n**Commit Details:**\n- **Stats**: %s\n- **SHA**: %s", currBranch, targetEnv, statStr, shortSha)
			}
		}

		_ = telemetry.EmitEvent(repoRoot, "gitops_deploy", deployMsg)

		// Auto-return to develop if on master
		if currBranch == "master" {
			synapse.Info("%s", fmt.Sprint("🔀 GitFlow: Returning to 'develop' working branch..."))
			_, _ = runGit("checkout", "develop")
		}

		// Trigger Vault and Remarkable Sync
		synapse.Info("%s", fmt.Sprint("📚 Triggering Vault Synchronization..."))
		syncCmd := exec.Command("bin/nomos", "vault", "sync")
		syncCmd.Dir = repoRoot
		syncCmd.Stdout = os.Stdout
		syncCmd.Stderr = os.Stderr
		if err := syncCmd.Run(); err != nil {
			synapse.Info("⚠️  Warning: failed to sync vault: %v\n", err)
		}

		// 5. Clean up active workspace state and transitions
		_ = os.Remove(config.StateTaskIdPath(repoRoot))
		_ = os.Remove(filepath.Join(repoRoot, ".agent", ".state_dod_failure_count"))
		_ = os.Remove(filepath.Join(repoRoot, ".agent", ".state_commit_approved"))
		_ = os.Remove(filepath.Join(repoRoot, ".agent", ".state_plan_approved"))

		// Transition phase to IDLE
		if err := task.TransitionPhase(repoRoot, statepkg.PhaseIdle); err != nil {
			return err
		}

		synapse.Info("%s", fmt.Sprint("🎉 Nomos push and GitOps synchronization completed successfully!"))
		return nil
	},
}

// syncMultiRepoRootPushes automatically pushes sibling open root repositories (e.g. nomos-sovereign)
// when Nomos Push is executed, maintaining 100% cross-repo synchronization.
func syncMultiRepoRootPushes(repoRoot string, targetEnv string) {
	parentDir := filepath.Dir(filepath.Dir(repoRoot))
	sovereignPath := filepath.Join(parentDir, "private", "nomos-sovereign")
	if stat, err := os.Stat(sovereignPath); err == nil && stat.IsDir() {
		statusCmd := exec.Command("git", "status", "-sb")
		statusCmd.Dir = sovereignPath
		if out, err := statusCmd.Output(); err == nil {
			statusStr := string(out)
			if strings.Contains(statusStr, "ahead") || strings.Contains(statusStr, "develop") {
				synapse.Info("🔄 Multi-Repo Root Sync: Pushing sibling root repository %s...\n", sovereignPath)
				pushCmd := exec.Command("git", "push", "origin", targetEnv, "--no-verify")
				pushCmd.Dir = sovereignPath
				if pushOut, err := pushCmd.CombinedOutput(); err != nil {
					synapse.Info("⚠️  Warning: failed to push sibling repo %s: %v (%s)\n", sovereignPath, err, string(pushOut))
				} else {
					synapse.Info("  ✅ Pushed %s\n", sovereignPath)
				}
			}
		}
	}
}

// syncSingleWorktree handles the processing, committing, and pushing of a single transient worktree.
// It performs Definition of Done verification, mechanically commits dirty changes using the parent Task ID,
// and merges into the target deployment branch to synchronize cross-repository codebase state.
// This function ensures the transient downstream repository remains in sync with the orchestrating root task.
func syncSingleWorktree(wt, repoRoot, targetEnv string) error {
	synapse.Info("🔄 Cross-Repo Sync: Processing transient worktree %s...\n", wt)

	parentTaskPath := filepath.Join(wt, ".nomos_parent_task")
	parentTaskID := ""
	if data, err := os.ReadFile(parentTaskPath); err == nil {
		parentTaskID = strings.TrimSpace(string(data))
	}

	if parentTaskID == "" {
		return fmt.Errorf("missing .nomos_parent_task in worktree %s", wt)
	}

	if err := commitWorktreeChanges(wt, parentTaskID); err != nil {
		return err
	}

	branch, err := gitops.PushWorktreeBranch(wt)
	if err != nil {
		return err
	}

	if branch != targetEnv && targetEnv == "develop" {
		if err := gitops.PerformGitFlowMerge(wt, branch, targetEnv); err != nil {
			return err
		}
	}

	synapse.Info("🧹 Tearing down transient worktree %s...\n", wt)

	wtRemoveCmd := exec.Command("git", "worktree", "remove", "-f", wt)
	wtRemoveCmd.Dir = repoRoot
	wtRemoveCmd.Run()

	synapse.Info("🧹 Pruning merged feature branch '%s'...\n", branch)
	branchDelCmd := exec.Command("git", "branch", "-D", branch)
	branchDelCmd.Dir = repoRoot
	branchDelCmd.Run()

	return nil
}

// commitWorktreeChanges checks if the worktree is dirty.
// If dirty, it runs DoD verification and creates an atomic Git commit.
// This enforces quality control gates within the downstream worktree context.
// It explicitly scopes the commit message to include the active task ID
// and specifies that the changes were automatically synced from the root workspace.
// This guarantees traceability and atomic delivery across multiple repositories.
func commitWorktreeChanges(wt, parentTaskID string) error {
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = wt
	out, err := statusCmd.CombinedOutput()
	if err != nil {
		return err
	}

	if len(strings.TrimSpace(string(out))) > 0 {
		synapse.Info("🛡️  Running Definition of Done (DoD) verification in worktree %s...\n", wt)
		if err := gitops.RunDoD(wt); err != nil {
			return fmt.Errorf("DoD failed in worktree %s: %w", wt, err)
		}

		addCmd := exec.Command("git", "add", ".")
		addCmd.Dir = wt
		if addOut, err := addCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to stage worktree changes: %v (%s)", err, string(addOut))
		}

		// Natively execute git commit with structured reasoning
		commitMsg := fmt.Sprintf("[Task %s] feat(worktree): Auto-sync from root workspace\n\n**Impact List:**\n- Auto-synced changes in worktree\n\n**Resolution Details:**\n- Mechanically committed cross-repo changes before root push", parentTaskID)
		commitCmd := exec.Command("git", "commit", "-m", commitMsg)
		commitCmd.Dir = wt
		if commitOut, err := commitCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git commit failed in worktree %s: %v (%s)", wt, err, string(commitOut))
		}
	}
	return nil
}

// processCrossRepoWorktrees natively processes transient cross-repo worktrees.
// It iterates over active worktrees, runs DoD, commits automatically, pushes, merges, and cleans up.
// When an agent modifies codebase across multiple repositories, Nomos spins up linked Git worktrees.
// This function orchestrates the lifecycle completion of these transient worktrees by looping through
// them, performing necessary commits on dirty state, executing the remote push protocol, and finally
// tearing them down (pruning branches) to restore a pristine environment.
func processCrossRepoWorktrees(repoRoot, targetEnv string) error {
	wts, err := workspace.GetCrossRepoWorktrees(repoRoot)
	if err != nil {
		return err
	}
	if len(wts) == 0 {
		return nil
	}

	for _, wt := range wts {
		if err := syncSingleWorktree(wt, repoRoot, targetEnv); err != nil {
			return err
		}
	}
	return nil
}
