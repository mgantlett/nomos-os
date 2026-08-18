package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-commons/src/nomos/core/telemetry"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/mgantlett/nomos-os/src/nomos/modules/verify"
	"github.com/spf13/cobra"
)

var forceStartFlag bool

// taskStartCmd transitions a task's tracking status, generates localized context prompts,
// writes the baseline tracking files, and registers the active task ID in the workspace.
// It enforces clean git working tree checks and performs JIT intelligence routing.
// taskStartCmd represents the start command.
// It is responsible for initiating a new task session, scaffolding the required
// Git worktrees, linking Go workspaces (if cross-repo orchestration is requested),
// and transitioning the system state into the PLAN phase.
var taskStartCmd = &cobra.Command{
	Use:   "start [task-keys...] [assignee-name]",
	Short: "Start one or more specific tasks in the workspace",
	Args:  cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		assignee := "antigravity"



		// Load the configured task tracker and locate the repository root.
		tracker, repoRoot, err := loadTrackerAndRoot()
		if err != nil {
			return err
		}

		ctx := context.Background()
		// Initialize empty slice to hold the target task keys to start
		var keys []string
		if len(args) > 0 {
			// Extract assignee if the last argument is not a valid task key.
			// This allows users to run commands like 'nomos task start 123 markg'
			// where 'markg' is the assignee, not a task ID.
			// Inspect the final argument to distinguish target assignees from task IDs
			lastArg := args[len(args)-1]
			// Regex pattern matching standard Nomos task key format (e.g., COM-123 or PMD-979)
			rxTaskKey := regexp.MustCompile(`(?i)^[A-Z0-9]+-\d+$`)
			if !rxTaskKey.MatchString(lastArg) {
				// Query the local task tracker to check if the argument matches a registered task ID
				if _, errView := tracker.View(ctx, lastArg); errView != nil {
					// The last argument is not a valid task key in the backend,
					// so we safely assume it's the target assignee for the task execution.
					assignee = lastArg
					// Remove the assignee from the keys array so we only iterate over task IDs.
					keys = args[:len(args)-1]
				} else {
					// All arguments represent valid task keys in the current workspace tracker
					keys = args
				}
			} else {
				// Target argument explicitly matches task key format, set keys slice
				keys = args
			}
		} else {
			// No keys provided; automatically determine the next priority task
			tasks, err := tracker.List(ctx)
			if err != nil {
				// Failed to list backlog tasks from the repository
				return fmt.Errorf("failed to list backlog tasks: %w", err)
			}
			// Filter out tasks that do not belong to the current project context
			tasks = FilterTasksByProject(tasks, func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())

			// Select the most critical task from the remaining list
			autoKey, err := selectNextPriorityTask(tasks)
			if err != nil {
				// Abort if the automated selection process fails
				return err
			}
			fmt.Printf("🎯 Auto-selected highest priority task: %s\n", autoKey)
			keys = []string{autoKey}
		}

		// Perform Auto-Context Switching if the primary task belongs to a sibling project
		if len(keys) > 0 {
			firstTaskKey := keys[0]
			if tObj, errView := tracker.View(ctx, firstTaskKey); errView == nil {
				currentProjectBase := filepath.Base(filepath.Clean(repoRoot))
				if !strings.EqualFold(currentProjectBase, tObj.Project) {
					fmt.Printf("🔄 Auto-switching context to target project root: %s...\n", tObj.Project)
					if resolvedRoot := workspace.ResolveProjectRoot(repoRoot, tObj.Project); resolvedRoot != "" {
						if errChdir := os.Chdir(resolvedRoot); errChdir == nil {
							repoRoot = resolvedRoot
						} else {
							fmt.Printf("⚠️  Warning: Failed to switch directory to %s: %v\n", resolvedRoot, errChdir)
						}
					}
				}
			}
		}

		if err := enforceRootZone(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), "task start"); err != nil {
			return err
		}

		if pState, err := task.GetPhaseState(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }()); err == nil {
			// Tier 2 agents (e.g. Swarm agents) run as stateless execution daemons.
			// They are forbidden from starting tasks because they don't have interactive IDE states.
			if pState.AgentTier == statepkg.Tier2 {
				return fmt.Errorf("Tier 2 atomic rigidity violation: agents are explicitly forbidden from starting new tasks")
			}

			// If TasksCompletedInSession > 0, it means the Orchestrator AI has already
			// completed a full task lifecycle within this active session.
			// To maintain a clean context and avoid hallucinations, we warn the user.
			if pState.TasksCompletedInSession > 0 {
				fmt.Println("\n⚠️  [SESSION WARNING] This workspace has already completed a task during the active session.")
				fmt.Println("To prevent context pollution and state leaks, it is highly recommended to start a clean session and run '/Nomos Handshake' before proceeding.")
				fmt.Println("Proceeding anyway, but be warned.")
			}
		}

		if !isGitTreeClean(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }()) && !forceStartFlag {
			return fmt.Errorf("workspace contains uncommitted changes. Please stash or commit your dirty files before starting a new task, or pass --force (-f) to bind task to current uncommitted work.")
		}

		// Rotate telemetry logs for the new task session
		telemetry.RotateSessionLogs(filepath.Join(workspace.MustNewContext(repoRoot).LogsDir(), "nomos.jsonl"), 20)

		for _, key := range keys {
			// Ensure the target task can be successfully viewed and parsed from the local JSON backend store.
			// This performs a direct read from the task repository database.
			tObj, errView := tracker.View(ctx, key)
			if errView != nil {
				// Search globally across all registered project stores to check for project context mismatch
				if allTasks, errAll := tracker.ListAll(ctx); errAll == nil {
					for _, t := range allTasks {
						if strings.EqualFold(t.Key, key) {
							if errCtx := task.ValidateWorkspaceTaskContext(repoRoot, key, t.Project); errCtx != nil {
								return errCtx
							}
						}
					}
				}
				// Return an error if the specific task key cannot be resolved from the backend
				return fmt.Errorf("failed to load task %s for routing: %w", key, errView)
			}
			if errCtx := task.ValidateWorkspaceTaskContext(repoRoot, key, tObj.Project); errCtx != nil {
				return errCtx
			}
			// Perform final checks to ensure agent configuration is applied correctly
			_, _, errStart := task.StartTask(ctx, func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), tracker, key, assignee, "", injectIntelligenceProfileExemptions)
			if errStart != nil {
				return errStart
			}

			if stashID, found := DetectStashForTask(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), key); found {
				fmt.Printf("⚠️  Stashed WIP detected for Task %s (%s). Run 'git stash pop %s' to resume work.\n", key, stashID, stashID)
			}

			// Audit overall system health before spinning up the interactive development environment
			if hStatus, err := verify.AuditHealth(repoRoot); err == nil {
				if len(hStatus.Failures) > 0 {
					// Display warnings without blocking execution, providing context to the engineer
					fmt.Printf("⚠️  [Boot Diagnostics] Health check warning: %s\n", hStatus.Failures[0])
				} else {
					fmt.Println("🏥 [Boot Diagnostics] System is fully healthy.")
				}
			}
			// Announce transition to PLAN phase for standard local development
			fmt.Printf("✅ Task %s started successfully and workspace transitioned to PLAN phase.\n", key)

			if isOrchestratorRoot(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }()) {
				_ = scaffoldTaskWorktree(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), key)
				
				// Explicitly link the 3 core repositories instead of scanning everything blindly
				allCoreRepos := []string{
					filepath.Join(filepath.Dir(filepath.Dir(repoRoot)), "open", "nomos-os"),
					filepath.Join(filepath.Dir(filepath.Dir(repoRoot)), "open", "nomos-commons"),
					filepath.Join(filepath.Dir(filepath.Dir(repoRoot)), "private", "nomos-sovereign"),
				}

				var discoveredRepos []string
				cleanRepoRoot := filepath.Clean(repoRoot)
				for _, r := range allCoreRepos {
					if filepath.Clean(r) != cleanRepoRoot {
						discoveredRepos = append(discoveredRepos, r)
					}
				}

				if len(discoveredRepos) > 0 {
					fmt.Printf("🔄 Auto-discovered %d sibling repositories for cross-repo workspace orchestration.\n", len(discoveredRepos))
					scaffoldCrossRepoWorktrees(repoRoot, key, discoveredRepos)
				}
			}
		}
		return nil
	},
}

// init registers the taskStartCmd with the parent taskCmd.
// It also defines the necessary CLI flags for starting a task.
func init() {
	taskStartCmd.Flags().BoolVarP(&forceStartFlag, "force", "f", false, "Force starting task even if working directory has uncommitted changes")
	taskCmd.AddCommand(taskStartCmd)
}
