// Package cmd defines the command line interface subcommands for Nomos.
// This file implements the handshake subcommand to synchronize and rehydrate developer context fast.
package cmd

import (
	"context"
	"os"

	"path/filepath"
	"sync"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"github.com/mgantlett/nomos-commons/src/nomos/core/telemetry"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/spf13/cobra"
)

type MemoryInsight struct {
	CommitHash string `json:"commit_hash"`
	Insight    string `json:"insight"`
}

type HandshakePayload struct {
	Timestamp string `json:"timestamp"`
	// Branch represents the active Git branch of the workspace.
	Branch string `json:"branch"`
	// DirtyFiles contains a list of unstaged or modified files in the working directory.
	DirtyFiles []string `json:"dirty_files"`
	// Claims holds any active workspace locks or claims by an autonomous agent.
	Claims []string `json:"claims"`
	// OpenTasks provides a snapshot of the current active sprint/backlog tasks.
	OpenTasks []task.Task `json:"open_tasks"`
	// Memories holds the semantic memories retrieved from GitBrain for the active context.
	Memories       []MemoryInsight `json:"memories"`
	ActiveTaskKey  string          `json:"active_task_key"`
	ActiveTaskName string          `json:"active_task_name"`
	// SuggestedActions acts as a dynamic onboarding guide for new users or Swarm agents.
	// It provides a deterministic list of core CLI commands or workflows that the engine supports.
	// Swarm agents parse this array during the /Handshake bootstrap sequence to build their context.
	SuggestedActions []string `json:"suggested_actions"`
	Errors           []string `json:"errors"`
}

// handshakeCmd defines the Cobra CLI subcommand structure for the handshake workflow.
// handshakeCmd represents the nomos workspace bootstrap process.
// It initializes git configuration, verifies dependencies, and sets up local data structures.
// Additionally, it syncs ecosystem reference clones to ensure cross-repo AI contexts remain active.
var handshakeCmd = &cobra.Command{
	Use:   "handshake",
	Short: "Initialize workspace state, synchronize OS protocols, and download ecosystem references",
	Long: `handshake is the primary bootstrap command. 
It writes baseline state, ensures the Git hook verification boundary is in place, and synchronizes the global .agents protocol into the local workspace.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Resolve the current working directory to find the root of the active repository.
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		repoRoot := findRepoRoot(wd)

		if err := enforceRootZone(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), "handshake"); err != nil {
			return err
		}

		// Clean up any legacy .agent directories from older versions of the Nomos engine.
		migrateLegacyDirectories(repoRoot)

		// Initialize the telemetry and context payload.
		payload := createHandshakePayload(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())

		// Rotate session logs to ensure we don't build up massive JSONL logs indefinitely.
		telemetry.RotateSessionLogs(filepath.Join(config.LogsDir(repoRoot), "nomos.jsonl"), 20)

		var branch string
		var dirtyFiles []string
		var claims []string

		// WaitGroup is used to run all the heavy I/O tasks concurrently during handshake
		// to minimize startup latency, giving the agent immediate responsiveness.
		// We add 3 because we are running 3 distinct parallel discovery tasks below.
		var wg sync.WaitGroup
		wg.Add(3)

		// 2. Discover the current active branch for workspace state binding.
		go func() {
			defer wg.Done()
			branch = getBranch(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())
		}()

		go func() {
			defer wg.Done()
			dirtyFiles = getDirtyFiles(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())
		}()

		go func() {
			defer wg.Done()
			claims = getClaims(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())
		}()

		ctx := context.Background()
		populateTrackerTasks(ctx, &payload, func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())

		query := "General Workspace Context"
		if payload.ActiveTaskName != "" {
			query = payload.ActiveTaskName
		}
		memories, errs := getMemories(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), query)

		wg.Wait()

		payload.Branch = branch
		payload.DirtyFiles = dirtyFiles
		payload.Claims = claims
		payload.Memories = memories
		payload.Errors = append(payload.Errors, errs...)

		synapse.Emit("Handshake", payload)
		return nil
	},
}
