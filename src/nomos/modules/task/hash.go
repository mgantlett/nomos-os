/*
Package task manages the core ticket tracking and state management logic.
The hash.go file implements the Data Integrity Gate cryptographic hashing.
It protects the local JSON files in <repoRoot>/.nomos/data/ by recursively collecting,
sorting, and hashing the JSON payloads to ensure that AI agents do not
manually bypass the deterministic task mutations.
*/
package task

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
)

// CalculateWorkspaceStateHash computes a deterministic cryptographic hash of all JSON state and task files.
// It iterates through the global tasks and state directories for the project.
// The algorithm works by recursively extracting and appending all JSON filenames and payloads
// associated with the active project namespace. The gathered string slice is strictly ordered
// alphanumerically via sort.Strings to ensure the SHA-256 buffer consumes the bytes deterministically,
// guaranteeing reproducible output hashes across any standard workspace configuration.
func CalculateWorkspaceStateHash(ctx *workspace.WorkspaceContext) (string, error) {
	repoRoot := ctx.RepoRoot
	dataDir := config.GlobalDataDir(repoRoot)
	stateDir := filepath.Join(dataDir, "state")

	var filesToHash []string

	// Delegate to helper to collect state files (.phase_state.json, quality_debt.json)
	stateFiles := collectStateFiles(stateDir)
	filesToHash = append(filesToHash, stateFiles...)

	// Sort to ensure deterministic hashing order for verification
	sort.Strings(filesToHash)

	// Hash all collected files
	h := sha256.New()
	for _, f := range filesToHash {
		data, err := os.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("failed to read file for hashing %s: %w", f, err)
		}
		// Write filename and content to hash context
		h.Write([]byte(filepath.Base(f)))
		h.Write(data)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// collectTaskFiles iterates through the global tasks directory and extracts tasks for this project.
// It parses the JSON to verify the "project" field matches the repoRoot base.
func collectTaskFiles(tasksDir, projectName string) []string {
	var files []string
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		return files
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			path := filepath.Join(tasksDir, e.Name())
			if isTaskForProject(path, projectName) {
				files = append(files, path)
			}
		}
	}
	return files
}

// isTaskForProject reads the file and unmarshals it to verify the project.
func isTaskForProject(path, projectName string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var partial struct {
		Project string `json:"project"`
	}
	if err := json.Unmarshal(data, &partial); err != nil {
		return false
	}
	return partial.Project == projectName
}

// collectStateFiles iterates through the project's state directory.
func collectStateFiles(stateDir string) []string {
	var files []string
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return files
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			files = append(files, filepath.Join(stateDir, e.Name()))
		}
	}
	return files
}

// PersistWorkspaceStateHash records the workspace signature hash to a flat file manifest.
func PersistWorkspaceStateHash(ctx *workspace.WorkspaceContext, hash string) error {
	repoRoot := ctx.RepoRoot
	hashPath := filepath.Join(config.TmpDir(repoRoot), ".workspace_state.hash")
	dir := filepath.Dir(hashPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create tmp directory: %w", err)
	}
	err := os.WriteFile(hashPath, []byte(hash), 0644)
	if err != nil {
		return fmt.Errorf("failed to write workspace state hash to file: %w", err)
	}
	return nil
}

// GetPersistedWorkspaceStateHash reads the registered workspace signature hash.
func GetPersistedWorkspaceStateHash(ctx *workspace.WorkspaceContext) (string, error) {
	repoRoot := ctx.RepoRoot
	hashPath := filepath.Join(config.TmpDir(repoRoot), ".workspace_state.hash")
	if _, err := os.Stat(hashPath); os.IsNotExist(err) {
		return "", nil
	}
	content, err := os.ReadFile(hashPath)
	if err != nil {
		return "", fmt.Errorf("failed to read workspace state hash: %w", err)
	}
	return string(content), nil
}

// UpdateWorkspaceStateHash is a convenience wrapper to recalculate and save the hash.
// This function should be called after any deterministic CLI mutation to the workspace.
func UpdateWorkspaceStateHash(ctx *workspace.WorkspaceContext) error {
	hash, err := CalculateWorkspaceStateHash(ctx)
	if err != nil {
		return err
	}
	return PersistWorkspaceStateHash(ctx, hash)
}

var asyncCommitsWg sync.WaitGroup

// WaitAsyncCommits blocks execution until all asynchronous workspace state hash updates complete.
// It is intended to be called with defer in main.go to ensure clean process termination.
func WaitAsyncCommits() {
	asyncCommitsWg.Wait()
}

// StateHashTracker wraps a Tracker to automatically update the Workspace State Hash upon mutations.
type StateHashTracker struct {
	Tracker
	ctx *workspace.WorkspaceContext
}

func WrapWithStateHash(t Tracker, ctx *workspace.WorkspaceContext) Tracker {
	return &StateHashTracker{Tracker: t, ctx: ctx}
}

func (w *StateHashTracker) updateHash() {
	asyncCommitsWg.Add(1)
	go func() {
		defer asyncCommitsWg.Done()
		_ = UpdateWorkspaceStateHash(w.ctx)
	}()
}

func (w *StateHashTracker) Start(ctx context.Context, key string, assignee string) error {
	err := w.Tracker.Start(ctx, key, assignee)
	if err == nil {
		w.updateHash()
	}
	return err
}

func (w *StateHashTracker) Close(ctx context.Context, key string, comment string) error {
	err := w.Tracker.Close(ctx, key, comment)
	if err == nil {
		w.updateHash()
	}
	return err
}

func (w *StateHashTracker) Cancel(ctx context.Context, key string, comment string) error {
	err := w.Tracker.Cancel(ctx, key, comment)
	if err == nil {
		w.updateHash()
	}
	return err
}

func (w *StateHashTracker) Comment(ctx context.Context, key string, comment string) error {
	err := w.Tracker.Comment(ctx, key, comment)
	if err == nil {
		w.updateHash()
	}
	return err
}

func (w *StateHashTracker) Transition(ctx context.Context, key string, status TaskStatus) error {
	err := w.Tracker.Transition(ctx, key, status)
	if err == nil {
		w.updateHash()
	}
	return err
}

func (w *StateHashTracker) Create(ctx context.Context, title string, body string, labels []string, parentKey string, project string, taskType TaskType, isSpike bool, initialStatus TaskStatus) (string, error) {
	key, err := w.Tracker.Create(ctx, title, body, labels, parentKey, project, taskType, isSpike, initialStatus)
	if err == nil {
		w.updateHash()
	}
	return key, err
}

func (w *StateHashTracker) Edit(ctx context.Context, key string, title *string, body *string, labels []string, contextBurden *int, logicDepth *int, blockedBy []string, sequence *int, project *string) error {
	err := w.Tracker.Edit(ctx, key, title, body, labels, contextBurden, logicDepth, blockedBy, sequence, project)
	if err == nil {
		w.updateHash()
	}
	return err
}
