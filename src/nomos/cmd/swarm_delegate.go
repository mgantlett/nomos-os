package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/spf13/cobra"
)

var delegatePhase string

var delegateCmd = &cobra.Command{
	Use:   "delegate [ncode] [task-key]",
	Short: "Mode 1: Delegate bounded tasks to local sub-agents in active workspace",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		agentName := args[0]
		taskKey := args[1]

		if agentName != "ncode" {
			return fmt.Errorf("unsupported agent: %s. Expected 'ncode'", agentName)
		}

		// Ensure ncode is installed (Sovereign feature enforcement)
		ncodePath, err := exec.LookPath("ncode")
		if err != nil {
			fmt.Println("⚠️  Sovereign Feature Warning: Proprietary 'ncode' binary is not installed or not in PATH.")
			fmt.Println("The Swarm orchestration layer is native to Nomos OS, but the AI execution layer requires the Sovereign Edition.")
			return fmt.Errorf("ncode binary not found")
		}

		tracker, repoRoot, err := loadTrackerAndRoot()
		if err != nil {
			return err
		}

		ctx := context.Background()
		tObj, errView := tracker.View(ctx, taskKey)
		if errView != nil {
			return fmt.Errorf("failed to load task %s: %w", taskKey, errView)
		}

		workspaceCtx := workspace.MustNewContext(repoRoot)

		fmt.Printf("🚀 Delegating task %s to Native NCode Swarm Agent...\n", taskKey)
		
		// Mimic nomos task start
		if err := tracker.Transition(ctx, taskKey, task.StatusInProgress); err != nil {
			return fmt.Errorf("failed to transition task to IN_PROGRESS: %w", err)
		}

		// Spawn transient, isolated worktree per delegated task
		if err := scaffoldTaskWorktree(workspaceCtx, taskKey); err != nil {
			return fmt.Errorf("failed to scaffold worktree: %w", err)
		}

		repoName := filepath.Base(repoRoot)
		worktreeDir := filepath.Join(workspaceCtx.WorktreesDir(), fmt.Sprintf("%s-%s", repoName, taskKey))

		// Restrict Tier 2 Swarm Worker from accessing root AGENTS.md to prevent hallucination
		sparseCmd := exec.Command("git", "sparse-checkout", "set", "--no-cone", "/*", "!/.agents/")
		sparseCmd.Dir = worktreeDir
		sparseCmd.Run()

		if delegatePhase != "" {
			var instructions string
			switch strings.ToUpper(delegatePhase) {
			case "PLAN":
				instructions = "Your explicit constraint is to operate in the PLAN phase.\nYou must NOT modify any source code.\nYou must only read the codebase and generate an `implementation_plan.md` artifact.\n"
			case "EDIT":
				instructions = "Your explicit constraint is to operate in the EDIT phase.\nYou must implement the code modifications detailed in the implementation plan and ensure they pass the `nomos verify` gate.\n"
			case "REVIEW":
				instructions = "Your explicit constraint is to operate in the REVIEW phase.\nYou must not modify logic. You must only verify tests, write the `walkthrough.md`, and prepare the task for sync.\n"
			default:
				instructions = "You are operating as a Swarm worker. Follow standard execution constraints."
			}
			
			promptPath := filepath.Join(worktreeDir, ".nomos", "data", "tmp", ".context-prompt.md")
			os.MkdirAll(filepath.Dir(promptPath), 0755)
			os.WriteFile(promptPath, []byte(instructions), 0644)
			fmt.Printf("✅ Injected holy-ghost constraints for %s phase into swarm worktree.\n", strings.ToUpper(delegatePhase))
		}

		// Generate .nomos-swarm-escrow.json containing the task details
		escrowPath := filepath.Join(worktreeDir, ".nomos-swarm-escrow.json")
		escrowData, err := json.MarshalIndent(tObj, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal escrow JSON: %w", err)
		}
		if err := os.WriteFile(escrowPath, escrowData, 0644); err != nil {
			return fmt.Errorf("failed to write escrow file: %w", err)
		}

		// Strict iteration/retry boundary
		maxIterations := 5
		for i := 1; i <= maxIterations; i++ {
			fmt.Printf("🔄 [Iteration %d/%d] Starting ncode execution...\n", i, maxIterations)

			ncodeCmd := exec.Command(ncodePath)
			ncodeCmd.Dir = worktreeDir
			ncodeCmd.Stdout = os.Stdout
			ncodeCmd.Stderr = os.Stderr
			if err := ncodeCmd.Run(); err != nil {
				fmt.Printf("⚠️  ncode exited with error on iteration %d: %v\n", i, err)
			}

			fmt.Println("🔍 Running quality gates (nomos verify)...")
			verifyCmd := exec.Command("nomos", "verify")
			verifyCmd.Dir = worktreeDir
			verifyCmd.Stdout = os.Stdout
			verifyCmd.Stderr = os.Stderr
			errVerify := verifyCmd.Run()

			if errVerify == nil {
				fmt.Printf("✅ Swarm worker successfully passed Definition of Done on iteration %d!\n", i)
				
				// Mark task as complete natively
				if err := tracker.Transition(ctx, taskKey, task.StatusDone); err != nil {
					return fmt.Errorf("failed to transition task to DONE: %w", err)
				}
				fmt.Printf("✅ Task %s transitioned to DONE state.\n", taskKey)
				return nil
			}

			fmt.Printf("❌ Verification failed on iteration %d. Passing context back to swarm...\n", i)
		}

		return fmt.Errorf("swarm worker failed to pass quality gates after %d iterations", maxIterations)
	},
}

func init() {
	delegateCmd.Flags().StringVar(&delegatePhase, "phase", "", "The specific DDP phase to constrain the delegated Swarm worker to (e.g. PLAN, EDIT, REVIEW)")
	swarmCmd.AddCommand(delegateCmd)
}
