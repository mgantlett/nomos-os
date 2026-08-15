package verify

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"github.com/mgantlett/nomos-os/src/nomos/modules/schema"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
)

// DebtGate defines the type for quality debt gates.
type DebtGate string

// StageAutoDebtTask registers an active technical debt bypass record automatically.
// Under the Nomos Unified Single-Standard DoD, all agents (Tier 1, Tier 2, Swarm, IDE)
// operate under identical machine-enforced gates. Non-worsening legacy debt is staged cleanly.
func StageAutoDebtTask(repoRoot string, file string, gate DebtGate, reason string) {
	if getActiveAgentTier(repoRoot) == state.Tier1 {
		return
	}
	manifestPath := filepath.Join(config.GlobalDataDir(repoRoot), "state", "quality_debt.json")
	_ = os.Chmod(manifestPath, 0644)
	var manifest QualityDebtManifest

	if data, err := os.ReadFile(manifestPath); err == nil {
		_ = json.Unmarshal(data, &manifest)
	}

	relFile := getRelativePath(repoRoot, file)

	// Check if already registered
	for _, item := range manifest.ActiveDebt {
		if getRelativePath(repoRoot, item.File) == relFile && item.Gate == string(gate) {
			return
		}
	}

	// Create refactor stories directory
	storiesDir := filepath.Join(config.TmpDir(repoRoot), "refactor_stories")
	_ = os.MkdirAll(storiesDir, 0755)

	storyHash := md5.Sum([]byte(relFile + "_" + string(gate)))
	storyID := hex.EncodeToString(storyHash[:])[:8]

	storyFile := filepath.Join(storiesDir, fmt.Sprintf("refactor_%s.md", storyID))

	s := &schema.TaskSchema{
		Description: fmt.Sprintf("Refactor Debt Story: Resolve %s in `%s`.", gate, relFile),
		AcceptanceCriteria: []string{
			fmt.Sprintf("- [ ] Resolve technical debt in `%s`.", relFile),
		},
		TechnicalNotes: []string{
			"- Custom acceptance criteria resolved technical debt.",
		},
	}
	storyContent := s.GenerateMarkdown("code")

	_ = os.WriteFile(storyFile, []byte(storyContent), 0644)

	// Deduplicate: If an active debt for this file and gate already exists, skip registering.
	for _, item := range manifest.ActiveDebt {
		if item.File == relFile && item.Gate == string(gate) {
			return
		}
	}

	// Append entry with AUTO task
	newItem := QualityDebtItem{
		File:       relFile,
		Gate:       string(gate),
		Reason:     reason,
		LinkedTask: "AUTO",
		CreatedAt:  time.Now().Format("2006-01-02T15:04:05Z"),
		ExpiresAt:  time.Now().AddDate(0, 1, 0).Format("2006-01-02T15:04:05Z"), // 1 month expiration
	}

	manifest.ActiveDebt = append(manifest.ActiveDebt, newItem)
	writeQualityDebtManifest(repoRoot, manifest.ActiveDebt)
}

// SyncQualityDebtManifest dynamically checks all active quality debts and prunes resolved ones.
// It also seamlessly promotes unresolved "AUTO" quality debt items into tracked backlog tasks.
// promoteAutoDebtTasks promotes AUTO bypasses into real backlog tasks.
func promoteAutoDebtTasks(repoRoot string, autoByFile map[string][]int, newActiveDebt []QualityDebtItem) bool {
	if len(autoByFile) == 0 {
		return false
	}
	modified := false
	var tracker task.Tracker
	if cfg, err := task.LoadConfig(repoRoot); err == nil {
		tracker, _ = task.NewTracker(cfg)
	}

	if tracker != nil {
		projectName := filepath.Base(filepath.Clean(repoRoot))
		for file, indices := range autoByFile {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			title := fmt.Sprintf("Resolve Quality Debt: %s", file)

			s := &schema.TaskSchema{
				Description:    fmt.Sprintf("As a developer, I want to resolve the following quality debt items in `%s`.", file),
				TechnicalNotes: []string{"- Automatically promoted from AUTO quality debt bypasses."},
				QualityDebt:    []string{"Ensure all corresponding DoD gates pass without bypasses."},
			}
			for _, idx := range indices {
				item := newActiveDebt[idx]
				s.AcceptanceCriteria = append(s.AcceptanceCriteria, fmt.Sprintf("- [ ] Resolve %s (Reason: %s)", item.Gate, item.Reason))
			}
			body := s.GenerateMarkdown("code")

			labels := []string{"priority:low", "type:debt", "cli:low"}
			newID, err := tracker.Create(ctx, title, body, labels, task.NoParentKey, projectName, task.TypeDebt, false, task.StatusTriage)
			cancel()

			if err == nil && newID != "" {
				modified = true
				synapse.Info("   \x1b[36m🚀 [Quality Debt Promoted] Converted AUTO bypasses in '%s' to Backlog Task %s\x1b[0m\n", file, newID)
				for _, idx := range indices {
					newActiveDebt[idx].LinkedTask = newID
				}
			}
		}
	}
	return modified
}

//
// QUALITY DEBT PROMOTION WORKFLOW:
// When an automated check (such as strict comment density, formatting, or dead
// code) detects a violation, the Cognitive Firewall might issue an "AUTO" bypass
// to allow the active feature commit to proceed (preventing developer stall).
// This module's job is to scan those AUTO bypasses and automatically "promote"
// them into explicit backlog tasks inside the .nomos/data/tasks folder, so they
// are tracked and eventually resolved during the next grooming session.
// -----------------------------------------------------------------------------

// SyncQualityDebtManifest dynamically checks all active quality debts and prunes resolved ones.
// It also seamlessly promotes unresolved "AUTO" quality debt items into tracked backlog tasks.
func SyncQualityDebtManifest(repoRoot string) {
	manifest, err := readQualityDebtManifest(repoRoot)
	if err != nil || len(manifest.ActiveDebt) == 0 {
		return
	}

	SyncQualityDebtStories(repoRoot)
	hasComplexityViolation := getComplexityViolationsMap(repoRoot)

	var newActiveDebt []QualityDebtItem
	modified := false
	autoByFile := make(map[string][]int)

	for _, item := range manifest.ActiveDebt {
		relFile := getRelativePath(repoRoot, item.File)
		absPath := filepath.Join(repoRoot, relFile)

		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			modified = true
			synapse.Info("   \x1b[32m✅ [Quality Debt Resolved] Automatically removed bypass for deleted file '%s' (gate: %s)\x1b[0m\n", relFile, item.Gate)
			continue
		}

		if isBypassResolved(repoRoot, absPath, relFile, item.Gate, hasComplexityViolation) {
			modified = true
			synapse.Info("   \x1b[32m✅ [Quality Debt Resolved] Automatically removed bypass for '%s' (gate: %s)\x1b[0m\n", relFile, item.Gate)
			continue
		}

		idx := len(newActiveDebt)
		newActiveDebt = append(newActiveDebt, item)
		if item.LinkedTask == "AUTO" {
			autoByFile[item.File] = append(autoByFile[item.File], idx)
		}
	}

	if promoteAutoDebtTasks(repoRoot, autoByFile, newActiveDebt) {
		modified = true
	}

	if modified {
		writeQualityDebtManifest(repoRoot, newActiveDebt)
	}
}

// PruneQualityDebtForTask removes any technical debt bypasses linked to the closed task.
// It iterates through the active debt items, filters out the entries where the linked
// task matches the target task ID, and persists the cleaned manifest back to the file system.
// This is a critical garbage collection step that executes automatically during the
// `nomos push` or `nomos task close` lifecycles to prevent ghost bypasses from lingering.
func PruneQualityDebtForTask(repoRoot string, taskId string) {
	// Read the current quality debt manifest from the repository agent directory.
	manifest, err := readQualityDebtManifest(repoRoot)
	if err != nil || len(manifest.ActiveDebt) == 0 {
		return
	}

	var newActiveDebt []QualityDebtItem
	modified := false

	// Filter out any bypasses that are linked to the closed task.
	for _, item := range manifest.ActiveDebt {
		if item.LinkedTask == taskId {
			modified = true
			synapse.Info("   \x1b[33m⚠️  [Quality Debt Pruned] Removed bypass for '%s' (gate: %s) because linked task %s is closed\x1b[0m\n", item.File, item.Gate, taskId)
			continue
		}
		newActiveDebt = append(newActiveDebt, item)
	}

	// Persist the updated manifest back to disk if any items were filtered.
	if modified {
		writeQualityDebtManifest(repoRoot, newActiveDebt)
	}
}

// SyncQualityDebtStories groups active quality debt items by their linked_task
// (if it is not "AUTO") and writes/updates a unified markdown story file
// under .agent/tmp/story_<taskId>.md listing all the active debt items.
func SyncQualityDebtStories(repoRoot string) {
	manifest, err := readQualityDebtManifest(repoRoot)
	if err != nil || len(manifest.ActiveDebt) == 0 {
		return
	}

	// Group debt items by linked task ID (excluding "AUTO" and empty)
	grouped := make(map[string][]QualityDebtItem)
	for _, item := range manifest.ActiveDebt {
		tID := strings.TrimSpace(item.LinkedTask)
		if tID != "" && strings.ToUpper(tID) != "AUTO" {
			grouped[tID] = append(grouped[tID], item)
		}
	}

	tmpDir := config.TmpDir(repoRoot)
	_ = os.MkdirAll(tmpDir, 0755)

	// Load active task tracker configuration to sync remote task bodies
	var tracker task.Tracker
	if cfg, err := task.LoadConfig(repoRoot); err == nil {
		tracker, _ = task.NewTracker(cfg)
	}

	for taskID, items := range grouped {
		content := generateTaskStoryMarkdown(repoRoot, taskID, items)

		if tracker != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

			// Preserve existing bundled tasks tracking if this task was an Epic
			appendBundledTasks(ctx, tracker, taskID, &content)

			_ = tracker.Edit(ctx, taskID, nil, &content, nil, nil, nil, nil, nil, nil)
			cancel()
		}

		storyPath := filepath.Join(tmpDir, fmt.Sprintf("story_%s.md", taskID))
		_ = os.WriteFile(storyPath, []byte(content), 0644)
	}
}

// generateTaskStoryMarkdown builds the structured markdown content for a backlog story.
func generateTaskStoryMarkdown(repoRoot string, taskID string, items []QualityDebtItem) string {
	s := &schema.TaskSchema{
		Description:    fmt.Sprintf("As a developer, I want to resolve the following quality debt items associated with Task %s to keep the codebase maintainable.", taskID),
		TechnicalNotes: []string{"- Automatically tracked and synchronized by Nomos Quality Debt Manager."},
		QualityDebt:    []string{"dry_candidate: true"},
	}

	seenFiles := make(map[string]bool)
	for _, item := range items {
		s.AcceptanceCriteria = append(s.AcceptanceCriteria, fmt.Sprintf("- [ ] Resolve %s in [%s](file://%s) (Reason: %s)", item.Gate, item.File, filepath.Join(repoRoot, item.File), item.Reason))

		if !seenFiles[item.File] {
			seenFiles[item.File] = true
			s.TargetFiles = append(s.TargetFiles, fmt.Sprintf("[MODIFY] %s", item.File))
		}
	}
	return s.GenerateMarkdown("code")
}

// isTaskTerminal verifies whether a specific task is permanently closed.
// It uses the SQLite task tracker to load the task and inspect its status field.
// "AUTO" bypass tasks are synthetically generated and never considered closed by this function.
func isTaskTerminal(repoRoot, tID string) bool {
	if tID == "" || tID == "AUTO" {
		return false
	}
	tracker := task.NewLocalTracker(repoRoot)
	tasks, err := tracker.List(context.Background())
	if err != nil {
		return false
	}
	for _, t := range tasks {
		if strings.ToUpper(t.Key) == strings.ToUpper(tID) {
			return t.IsClosed()
		}
	}
	return false
}

// appendBundledTasks dynamically injects the existing bundled task relationships
// into the quality debt generated story body.
// This preserves the hierarchical Epic -> Story relationship when auto-syncing
// markdown descriptions for heavily bundled quality debt tracking tasks.
func appendBundledTasks(ctx context.Context, tracker task.Tracker, taskID string, content *string) {
	if t, err := tracker.View(ctx, taskID); err == nil {
		if strings.Contains(t.Description, "**Bundled Tasks:**") {
			parts := strings.Split(t.Description, "**Bundled Tasks:**")
			if len(parts) > 1 {
				childrenStr := strings.TrimSpace(parts[1])
				childrenLine := strings.Split(childrenStr, "\n")[0]
				*content += "\n\n**Bundled Tasks:** " + strings.TrimSpace(childrenLine) + "\n"
			}
		}
	}
}
