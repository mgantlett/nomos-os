package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"github.com/spf13/cobra"
)

// maintenanceCmd represents the "maintenance" command which triggers garbage collection
// routines on the repository. It cleans up stale branches, orphaned worktrees, and
// stale configuration state across both the current repository and any linked cross-repo siblings.
var maintenanceCmd = &cobra.Command{
	Use:   "maintenance",
	Short: "Run routine workspace maintenance tasks and garbage collection",
	RunE: func(cmd *cobra.Command, args []string) error {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		repoRoot := findRepoRoot(wd)

		if err := enforceRootZone(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), "maintenance"); err != nil {
			return err
		}

		synapse.Info("🧹 Running Nomos Workspace Maintenance...")

		// 1. Prune orphaned worktrees
		synapse.Info("  => Pruning orphaned worktrees...")
		wtCmd := exec.Command("git", "worktree", "prune")
		wtCmd.Dir = repoRoot
		wtCmd.Run()

		// 2. Fetch and prune remote references
		synapse.Info("  => Pruning remote tracking references...")
		fetchCmd := exec.Command("git", "fetch", "--prune")
		fetchCmd.Dir = repoRoot
		fetchCmd.Run()

		// 3. Delete stale merged feature branches
		synapse.Info("  => Deleting stale merged local branches...")

		var closedTasks map[string]bool
		taskCmd := exec.Command("bin/nomos", "task", "list", "--show-closed", "--json")
		taskCmd.Dir = repoRoot
		if out, err := taskCmd.Output(); err == nil {
			closedTasks = make(map[string]bool)
			var tasks []struct {
				ID     string `json:"key"`
				Status string `json:"status"`
			}
			if err := json.Unmarshal(out, &tasks); err == nil {
				for _, t := range tasks {
					if t.Status == "CLOSED" || t.Status == "DONE" {
						closedTasks[t.ID] = true
					}
				}
			} else {
				synapse.Info("      ⚠️ Failed to unmarshal tasks: %v", err)
			}
		} else {
			synapse.Info("      ⚠️ Failed to get closed tasks: %v", err)
		}

		var rootMergedBranches []string
		branchCmd := exec.Command("git", "branch", "--merged")
		branchCmd.Dir = repoRoot
		if out, err := branchCmd.Output(); err == nil {
			branches := strings.Split(string(out), "\n")
			for _, b := range branches {
				b = strings.TrimSpace(b)
				if b == "" || strings.HasPrefix(b, "*") {
					continue
				}
				if b == "master" || b == "main" || b == "develop" {
					continue
				}
				rootMergedBranches = append(rootMergedBranches, b)
			}

			// Force prune any worktrees holding these merged branches BEFORE we try to delete them
			aggressivelyPruneWorktrees(repoRoot, rootMergedBranches, rootMergedBranches, closedTasks)

			for _, b := range rootMergedBranches {
				synapse.Info("      🗑️  Deleting merged branch: %s", b)
				delCmd := exec.Command("git", "branch", "-d", b)
				delCmd.Dir = repoRoot
				delCmd.Run()
			}
		}

		// 4. Discover sibling repos and prune them too
		synapse.Info("  => Scanning for sibling cross-repo worktrees...")
		wtDir := filepath.Join(repoRoot, "worktrees")
		if entries, err := os.ReadDir(wtDir); err == nil {
			processedSiblings := make(map[string]bool)
			for _, entry := range entries {
				if entry.IsDir() {
					gitFile := filepath.Join(wtDir, entry.Name(), ".git")
					if data, err := os.ReadFile(gitFile); err == nil {
						parts := strings.Split(string(data), ":")
						if len(parts) >= 2 {
							gitdir := strings.TrimSpace(parts[1])
							siblingRoot := filepath.Dir(filepath.Dir(filepath.Dir(gitdir)))
							if siblingRoot != repoRoot && !processedSiblings[siblingRoot] {
								processedSiblings[siblingRoot] = true
								synapse.Info("  => Running maintenance on sibling repo: %s...", filepath.Base(siblingRoot))

								sPrune := exec.Command("git", "worktree", "prune")
								sPrune.Dir = siblingRoot
								sPrune.Run()

								sFetch := exec.Command("git", "fetch", "--prune")
								sFetch.Dir = siblingRoot
								sFetch.Run()

								sBranchCmd := exec.Command("git", "branch", "--merged")
								sBranchCmd.Dir = siblingRoot
								if out, err := sBranchCmd.Output(); err == nil {
									var mergedBranches []string
									branches := strings.Split(string(out), "\n")
									for _, b := range branches {
										b = strings.TrimSpace(b)
										if b == "" || strings.HasPrefix(b, "*") || b == "master" || b == "main" || b == "develop" {
											continue
										}
										mergedBranches = append(mergedBranches, b)
									}

									// Force prune any worktrees holding these merged branches BEFORE we try to delete them
									aggressivelyPruneWorktrees(siblingRoot, mergedBranches, rootMergedBranches, closedTasks)

									for _, b := range mergedBranches {
										synapse.Info("      🗑️  Deleting merged sibling branch: %s", b)
										sDelCmd := exec.Command("git", "branch", "-d", b)
										sDelCmd.Dir = siblingRoot
										sDelCmd.Run()
									}

									// Also delete any sibling branch if the ROOT repo merged it, because cross-repo tasks share a lifecycle.
									for _, b := range rootMergedBranches {
										// Check if branch still exists locally
										chkCmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+b)
										chkCmd.Dir = siblingRoot
										if err := chkCmd.Run(); err == nil {
											synapse.Info("      🗑️  Deleting orphaned sibling branch: %s", b)
											sDelCmd := exec.Command("git", "branch", "-D", b)
											sDelCmd.Dir = siblingRoot
											sDelCmd.Run()
										}
									}
								}
							}
						}
					}
				}
			}
		}

		// 5. Sweep for leftover untracked physical directories for closed tasks
		if entries, err := os.ReadDir(wtDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					wtPath := filepath.Join(wtDir, entry.Name())
					if parentTaskBytes, err := os.ReadFile(filepath.Join(wtPath, ".nomos_parent_task")); err == nil {
						taskID := strings.TrimSpace(string(parentTaskBytes))
						if closedTasks[taskID] {
							synapse.Info("      🔥 Sweeping leftover directory for closed task %s: %s", taskID, entry.Name())
							os.RemoveAll(wtPath)
						}
					}
				}
			}
		}

		synapse.Info("✅ Maintenance complete!")
		return nil
	},
}

// aggressivelyPruneWorktrees finds all worktrees checked out to a branch that is fully merged,
// or whose parent task is closed, and forcibly removes them, freeing up the branch to be deleted cleanly.
// It uses git worktree list to find linked worktrees and matches them against the provided
// lists of merged branches and closed tasks. If a match is found, it forcefully tears down the
// worktree and deletes any lingering untracked files left behind on the filesystem.
func aggressivelyPruneWorktrees(repoRoot string, localMergedBranches []string, rootMergedBranches []string, closedTasks map[string]bool) {
	mergedSet := make(map[string]bool)
	for _, b := range localMergedBranches {
		mergedSet[b] = true
	}
	for _, b := range rootMergedBranches {
		mergedSet[b] = true
	}

	wtListCmd := exec.Command("git", "worktree", "list", "--porcelain")
	wtListCmd.Dir = repoRoot
	if out, err := wtListCmd.Output(); err == nil {
		var currentWT string
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "worktree ") {
				currentWT = strings.TrimPrefix(line, "worktree ")
			} else if strings.HasPrefix(line, "branch ") {
				branchRef := strings.TrimPrefix(line, "branch ")
				branchName := strings.TrimPrefix(branchRef, "refs/heads/")

				shouldPrune := false
				if mergedSet[branchName] && currentWT != repoRoot {
					shouldPrune = true
				} else if currentWT != repoRoot {
					// Check if branch still exists
					chkCmd := exec.Command("git", "show-ref", "--verify", "--quiet", branchRef)
					chkCmd.Dir = repoRoot
					if err := chkCmd.Run(); err != nil {
						// Branch doesn't exist anymore, it's an orphaned worktree!
						shouldPrune = true
					}
				}

				// Check if the parent task is closed
				if !shouldPrune && currentWT != repoRoot {
					if parentTaskBytes, err := os.ReadFile(filepath.Join(currentWT, ".nomos_parent_task")); err == nil {
						taskID := strings.TrimSpace(string(parentTaskBytes))
						if closedTasks[taskID] {
							shouldPrune = true
							synapse.Info("      🔥 Task %s is closed. Pruning its worktree...", taskID)
						}
					}
				}

				if shouldPrune {
					synapse.Info("      🔥 Forcibly tearing down orphaned worktree: %s", filepath.Base(currentWT))
					rmCmd := exec.Command("git", "worktree", "remove", "--force", currentWT)
					rmCmd.Dir = repoRoot
					rmCmd.Run()

					// Remove any remaining untracked files like .nomos_parent_task
					os.RemoveAll(currentWT)
				}
			}
		}
	}
}

func init() {
	RootCmd.AddCommand(maintenanceCmd)
}
