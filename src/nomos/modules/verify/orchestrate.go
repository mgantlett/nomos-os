package verify

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"github.com/mgantlett/nomos-commons/src/nomos/core/telemetry"
	nomosexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
)

// nomosExecutableOverride is used during testing to bypass running the real nomos binary.
var nomosExecutableOverride string

// SwarmSubtask defines a single task to be executed by a swarm worker.
type SwarmSubtask struct {
	ID     string `json:"id"`
	Prompt string `json:"prompt"`
}

// SwarmPlan defines the multi-agent task execution plan.
type SwarmPlan struct {
	TargetRepo string         `json:"target_repo"`
	BaseBranch string         `json:"base_branch"`
	Subtasks   []SwarmSubtask `json:"subtasks"`
}

// OrchestrateSwarm executes a set of tasks in parallel using git worktrees.
func OrchestrateSwarm(planPath string) error {
	planData, err := os.ReadFile(planPath)
	if err != nil {
		return fmt.Errorf("failed to read plan file: %w", err)
	}

	var plan SwarmPlan
	if err := json.Unmarshal(planData, &plan); err != nil {
		return fmt.Errorf("failed to parse plan JSON: %w", err)
	}

	if plan.TargetRepo == "" {
		return fmt.Errorf("target_repo cannot be empty")
	}
	if plan.BaseBranch == "" {
		plan.BaseBranch = "develop"
	}

	// Verify target repo exists
	repoInfo, err := os.Stat(plan.TargetRepo)
	if err != nil || !repoInfo.IsDir() {
		return fmt.Errorf("target repository '%s' is not a valid directory", plan.TargetRepo)
	}

	synapse.Info("🚀 Starting Go-native Swarm Orchestrator\n")
	synapse.Info("   ↳ Target Repo: %s\n", plan.TargetRepo)
	synapse.Info("   ↳ Base Branch: %s\n", plan.BaseBranch)
	synapse.Info("   ↳ Subtasks:    %d\n\n", len(plan.Subtasks))

	// Pre-flight check: ensure base branch is active in main repo (stashing clean if needed)
	if err := prepareBaseBranch(plan.TargetRepo, plan.BaseBranch); err != nil {
		return fmt.Errorf("pre-flight base branch check failed: %w", err)
	}

	worktreeBase := filepath.Join(config.GlobalDataDir(plan.TargetRepo), "orchestrate", filepath.Base(plan.TargetRepo))
	if err := os.MkdirAll(worktreeBase, 0755); err != nil {
		return fmt.Errorf("failed to create temporary worktree directory: %w", err)
	}

	nomosBin := nomosExecutableOverride
	if nomosBin == "" {
		nomosBin = getNomosExecutable(plan.TargetRepo)
	}

	results := dispatchSwarmTasks(plan, worktreeBase, nomosBin)
	return mergeSwarmResults(plan, results)
}

type taskResult struct {
	task SwarmSubtask
	err  error
	path string
}

// scaffoldWorktree cleans up previous worktree references and creates a fresh git worktree.
func scaffoldWorktree(tID string, targetRepo string, wtDir string, branchName string, baseBranch string) error {
	_ = runGitCommand(targetRepo, "worktree", "remove", "-f", wtDir)
	_ = runGitCommand(targetRepo, "branch", "-D", branchName)
	return runGitCommand(targetRepo, "worktree", "add", "-f", wtDir, "-b", branchName, baseBranch)
}

// runSwarmCommand prepares the worker process log file, triggers the nomos swarm subcommand,
// registers the PID in the Cockpit telemetry cache, and waits for command execution to finish.
func runSwarmCommand(t SwarmSubtask, wtDir string, nomosBin string, dbPath string, logDir string) error {
	cmd := exec.Command(nomosBin, "swarm", t.Prompt)
	cmd.Dir = wtDir
	cmd.Env = cleanGitEnv()

	logFile, err := os.OpenFile(filepath.Join(logDir, "nomos.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		writer := telemetry.NewLogWriter("swarm", t.ID, "aider")
		cmd.Stdout = io.MultiWriter(logFile, writer)
		cmd.Stderr = io.MultiWriter(logFile, writer)
		defer logFile.Close()
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	pid := cmd.Process.Pid
	commandStr := fmt.Sprintf("aider --task-id %s (nomos swarm worktree run) specs/%s/plan", t.ID, t.ID)
	_ = nomosexec.RegisterActiveProcess(dbPath, pid, commandStr)

	runErr := cmd.Wait()
	_ = nomosexec.DeregisterActiveProcess(dbPath, pid)
	return runErr
}

// executeSingleSwarmSubtask configures git worktrees and dispatches task workers.
func executeSingleSwarmSubtask(t SwarmSubtask, plan SwarmPlan, worktreeBase, nomosBin string, results chan<- taskResult) {
	branchName := fmt.Sprintf("swarm-task-%s", t.ID)
	wtDir := filepath.Join(worktreeBase, t.ID)

	synapse.Info("🔨 [Orchestrator] Scaffolding Worktree: Task %s -> %s\n", t.ID, wtDir)
	if err := scaffoldWorktree(t.ID, plan.TargetRepo, wtDir, branchName, plan.BaseBranch); err != nil {
		results <- taskResult{t, fmt.Errorf("failed to create git worktree: %w", err), wtDir}
		return
	}

	synapse.Info("🚀 [Orchestrator] Dispatching Task %s in worktree...\n", t.ID)

	logDir := config.LogsDir(plan.TargetRepo)
	_ = os.MkdirAll(logDir, 0755)
	dbPath := config.ResolveCacheDbPath(plan.TargetRepo)

	runErr := runSwarmCommand(t, wtDir, nomosBin, dbPath, logDir)

	if runErr == nil {
		_ = runGitCommand(wtDir, "add", ".")
		_ = runGitCommand(wtDir, "commit", "--no-verify", "-m", fmt.Sprintf("Swarm task %s completed", t.ID))
	}
	results <- taskResult{t, runErr, wtDir}
}

// dispatchSwarmTasks schedules and executes the configured subtasks in parallel.
// It creates a goroutine for each subtask and returns a channel containing the results.
func dispatchSwarmTasks(plan SwarmPlan, worktreeBase, nomosBin string) <-chan taskResult {
	results := make(chan taskResult, len(plan.Subtasks))
	var wg sync.WaitGroup

	for _, task := range plan.Subtasks {
		wg.Add(1)
		go func(t SwarmSubtask) {
			defer wg.Done()
			executeSingleSwarmSubtask(t, plan, worktreeBase, nomosBin, results)
		}(task)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}

// mergeSwarmResults loops over the results channel to process all finished tasks.
// For successful tasks, it merges the temporary task branches back to the base branch.
func mergeSwarmResults(plan SwarmPlan, results <-chan taskResult) error {
	synapse.Info("\n🔄 [Orchestrator] Phase 2: Synchronizing and Merging Swarm...\n")
	hasErrors := false
	var completedTasks []string

	for res := range results {
		branchName := fmt.Sprintf("swarm-task-%s", res.task.ID)

		if res.err != nil {
			synapse.Info("❌ Task %s FAILED in worktree (check .agent/logs/swarm-task-%s.log): %v\n", res.task.ID, res.task.ID, res.err)
			hasErrors = true
		} else {
			synapse.Info("✅ Task %s completed successfully. Merging branch %s...\n", res.task.ID, branchName)

			// Merge branch back
			if err := runGitCommand(plan.TargetRepo, "merge", "--no-ff", branchName, "-m", fmt.Sprintf("Merge task %s into %s", res.task.ID, plan.BaseBranch)); err != nil {
				synapse.Info("🚨 Merge conflict on Task %s branch %s: %v\n", res.task.ID, branchName, err)
				_ = runGitCommand(plan.TargetRepo, "merge", "--abort")
				hasErrors = true
			} else {
				completedTasks = append(completedTasks, res.task.ID)
			}
		}

		// Prune worktree
		_ = runGitCommand(plan.TargetRepo, "worktree", "remove", "-f", res.path)
		_ = runGitCommand(plan.TargetRepo, "branch", "-D", branchName)
	}

	if len(completedTasks) > 0 {
		synapse.Info("\n🎉 Merged completed tasks: %s\n", strings.Join(completedTasks, ", "))
	}

	if hasErrors {
		return fmt.Errorf("some swarm tasks failed to complete or merge")
	}

	return nil
}

// prepareBaseBranch checks out the base branch in the primary workspace.
// If the branch is not current, it checks it out, creating a new branch from HEAD if necessary.
func prepareBaseBranch(repo, branch string) error {
	// Query current branch
	out, err := runGitCommandWithOutput(repo, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return err
	}
	curr := strings.TrimSpace(out)

	if curr != branch {
		// Switch to base branch
		if err := runGitCommand(repo, "checkout", branch); err != nil {
			// Try creating it if it doesn't exist
			if err := runGitCommand(repo, "checkout", "-b", branch); err != nil {
				return err
			}
		}
	}
	return nil
}

// getNomosExecutable determines the path of the nomos binary to run.
// It checks the local bin/nomos first, then the current process binary, and defaults to 'nomos'.
func getNomosExecutable(repo string) string {
	localBin := filepath.Join(repo, "bin", "nomos")
	if info, err := os.Stat(localBin); err == nil && !info.IsDir() {
		return localBin
	}
	if exe, err := os.Executable(); err == nil {
		return exe
	}
	return "nomos"
}

// runGitCommand helper executes a git command in the specified directory.
// It wraps runGit and returns only the execution error.
func runGitCommand(dir string, args ...string) error {
	_, err := runGit(dir, args...)
	return err
}

// runGitCommandWithOutput helper executes a git command in the specified directory.
// It wraps runGit and returns both the output string and the execution error.
func runGitCommandWithOutput(dir string, args ...string) (string, error) {
	return runGit(dir, args...)
}
