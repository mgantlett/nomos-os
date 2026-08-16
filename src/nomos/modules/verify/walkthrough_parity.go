package verify

import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"

	"github.com/mgantlett/nomos-os/src/nomos/modules/schema"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
)

// VerifyWalkthroughParity verifies that the walkthrough document covers
// all the Acceptance Criteria defined in the active backlog task description.
func VerifyWalkthroughParity(root string) error {
	// Resolve active task identifier from phase state SSoT
	taskId := GetActiveTaskId(root)
	if taskId == "" {
		return nil
	}

	// Sync walkthrough content from task storage or brain artifacts
	walkthroughPath, errSync := syncWalkthroughFile(root, taskId)
	if errSync != nil {
		return errSync
	}

	// Fetch task details from issue tracker backend
	t, err := fetchTaskDetails(root, taskId)
	if err != nil {
		synapse.Info("   ⚠️  [Walkthrough Parity Warning] Failed to fetch task: %v\n", err)
		return nil
	}

	// We parse the task string description to extract the Acceptance Criteria block.
	s, err := schema.ParseTaskSchema(t.Description, "code")
	var criteria []string
	if err == nil {
		criteria = s.AcceptanceCriteria
	}

	// Add dynamic walkthrough parity check for extended criteria
	extended := extractExtendedCriteria(root, taskId)
	criteria = append(criteria, extended...)

	if len(criteria) == 0 {
		return nil
	}

	// Read walkthrough markdown bytes
	planBytes, err := os.ReadFile(walkthroughPath)
	if err != nil {
		return fmt.Errorf("failed to read walkthrough: %w", err)
	}

	// Evaluate coverage for each acceptance criterion
	var uncovered []string
	planText := string(planBytes)
	for _, cr := range criteria {
		if !isCriterionCovered(cr, planText) {
			uncovered = append(uncovered, cr)
		}
	}

	// Report error if uncovered criteria remain
	if len(uncovered) > 0 {
		return fmt.Errorf("Walkthrough Parity Failure: The following Acceptance Criteria from Task %s were not covered in the walkthrough:\n  - %s\n\nEnsure your walkthrough explicitly documents how these requirements were solved.", taskId, strings.Join(uncovered, "\n  - "))
	}

	return nil
}

// extractExtendedCriteria parses the implementation plan for extended acceptance criteria.
func extractExtendedCriteria(root string, taskId string) []string {
	var criteria []string
	planPath := filepath.Join(workspace.MustNewContext(root).DataPath("plans"), taskId+".md")
	planData, err := os.ReadFile(planPath)
	if err != nil {
		return criteria
	}

	lines := strings.Split(string(planData), "\n")
	inExtendedSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## Extended Acceptance Criteria") {
			inExtendedSection = true
			continue
		}
		if inExtendedSection && strings.HasPrefix(trimmed, "## ") {
			break
		}
		if inExtendedSection && strings.HasPrefix(trimmed, "- ") {
			// Strip checkbox if present
			item := strings.TrimPrefix(trimmed, "- ")
			item = strings.TrimPrefix(item, "[ ] ")
			item = strings.TrimPrefix(item, "[x] ")
			item = strings.TrimSpace(item)
			if item != "" {
				criteria = append(criteria, item)
			}
		}
	}
	return criteria
}

// syncFromBrainArtifact handles the logic for scanning user brain directories for walkthrough artifacts
// It identifies the most recently modified brain artifact across all conversation sessions.
// If an artifact is found, it copies it into the task walkthrough folder and the tmp location.
// This serves as the Priority 2 fallback when a task walkthrough is missing or newly created.
// syncFromBrainArtifact attempts to locate and copy a walkthrough artifact
// generated during the ide phase by the brain subsystem.
// This allows for AI agents to write artifacts directly to their workspace memory
// and have the verification gate automatically sync it into the final codebase
// for review. It uses early returns to limit complexity and cyclomatic depth.
func syncFromBrainArtifact(root string, taskId string, taskWalkthrough string, walkthroughPath string) (string, error) {
	// Attempt to resolve the user's home directory path to locate .gemini brain
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		// Skip brain search if home directory could not be resolved by OS
		return "", nil
	}

	// Glob search pattern across all conversation brain directories
	brainPattern := filepath.Join(homeDir, ".gemini", "antigravity-ide", "brain", "*", workspace.WalkthroughFileName)
	matches, _ := filepath.Glob(brainPattern)

	// Check if any matching brain artifacts were located
	if len(matches) == 0 {
		return "", nil
	}

	// Find the most recent valid artifact using a helper function
	latest := findLatestBrainArtifact(matches, taskId)

	// If no valid latest artifact was successfully identified, return early
	if latest == "" {
		return "", nil
	}

	// Read the bytes of the latest brain artifact from disk
	data, errBrain := os.ReadFile(latest)
	if errBrain != nil || len(data) == 0 {
		return "", nil
	}

	// Save walkthrough content to temporary verification buffer
	_ = os.MkdirAll(filepath.Dir(walkthroughPath), 0755)
	_ = os.WriteFile(walkthroughPath, data, 0644)

	// Persist walkthrough content to task SSoT directory
	_ = os.MkdirAll(workspace.MustNewContext(root).DataPath("walkthroughs"), 0755)
	_ = os.WriteFile(taskWalkthrough, data, 0644)

	// Return the successfully synced temporary walkthrough path
	return walkthroughPath, nil
}

// findLatestBrainArtifact scans matches to find the most recently modified artifact for the active task.
func findLatestBrainArtifact(matches []string, taskId string) string {
	latest := ""
	var latestTime int64 = 0

	for _, match := range matches {
		fi, err := os.Stat(match)
		if err != nil {
			continue
		}

		content, err := os.ReadFile(match)
		if err != nil || !strings.Contains(string(content), taskId) {
			continue
		}

		if fi.ModTime().UnixNano() > latestTime {
			latestTime = fi.ModTime().UnixNano()
			latest = match
		}
	}
	return latest
}

// syncWalkthroughFile checks task walkthrough paths and conversation brain artifacts, syncing to tmp buffer.
// syncWalkthroughFile locates the active task ID and attempts to synchronize
// the walkthrough documentation file into the workspace root.
// It searches for the artifact within the system brain or prompts the user
// if no artifact could be automatically matched with the current state.
func syncWalkthroughFile(root string, taskId string) (string, error) {
	// Priority 1: Check legacy or standard location for walkthrough in .nomos/walkthroughs/<taskId>.md
	localWalkthrough := filepath.Join(workspace.MustNewContext(root).DataDir(), "walkthroughs", taskId+".md")
	walkthroughPath := filepath.Join(workspace.MustNewContext(root).DataPath("walkthroughs"), taskId+".md")
	// Canonical task walkthrough file path in workspace SSoT storage
	taskWalkthrough := filepath.Join(workspace.MustNewContext(root).DataPath("walkthroughs"), fmt.Sprintf("%s.md", taskId))

	// Priority 1: Check active task walkthrough locally
	if data, errRead := os.ReadFile(localWalkthrough); errRead == nil && len(data) > 0 {
		_ = os.MkdirAll(filepath.Dir(walkthroughPath), 0755)
		// Write task walkthrough bytes to temporary verification buffer
		_ = os.WriteFile(walkthroughPath, data, 0644)
		return walkthroughPath, nil
	}

	// Priority 2: Check active task walkthrough globally
	if data, errRead := os.ReadFile(taskWalkthrough); errRead == nil && len(data) > 0 {
		_ = os.MkdirAll(filepath.Dir(walkthroughPath), 0755)
		// Write task walkthrough bytes to temporary verification buffer
		_ = os.WriteFile(walkthroughPath, data, 0644)
		return walkthroughPath, nil
	}

	// Priority 2: Discover latest conversation brain artifact walkthrough.md
	// This delegates the search to the syncFromBrainArtifact helper function
	if syncedPath, err := syncFromBrainArtifact(root, taskId, taskWalkthrough, walkthroughPath); err == nil && syncedPath != "" {
		// Return the successfully synced path from the brain artifact
		return syncedPath, nil
	}

	// Fail verification if no walkthrough artifact is present in workspace or brain
	if _, err := os.Stat(walkthroughPath); os.IsNotExist(err) {
		return "", fmt.Errorf("walkthrough file not found: %s. Run 'bin/nomos walkthrough' to generate one.", walkthroughPath)
	}
	return walkthroughPath, nil
}

// isCriterionCovered performs a token overlap check between criterion and text content.
func isCriterionCovered(criterion, text string) bool {
	// Strip markdown code backticks which interfere with token matching
	cleanCriterion := strings.ReplaceAll(criterion, "`", " ")
	cleanText := strings.ReplaceAll(text, "`", " ")

	// Split target criterion string into individual word tokens
	words := strings.Fields(strings.ToLower(cleanCriterion))
	// Convert full walkthrough text to lowercase for case-insensitive matching
	textLower := strings.ToLower(cleanText)

	// If criterion string contains no word tokens, mark as covered
	if len(words) == 0 {
		return true
	}

	// Counter for tracking matching significant terms in text
	matchCount := 0
	for _, w := range words {
		// Filter out short filler words under 4 characters
		if len(w) < 4 {
			continue
		}
		// Increment match counter if word is present in walkthrough text
		if strings.Contains(textLower, w) {
			matchCount++
		}
	}

	// If at least one significant word is found in the text, we consider it covered.
	return matchCount > 0
}

func fetchTaskDetails(root string, id string) (*task.Task, error) {
	primaryRoot := root
	if strings.Contains(root, "/worktrees/") {
		parts := strings.Split(root, "/worktrees/")
		parentRepo := parts[0]
		if _, statErr := os.Stat(workspace.MustNewContext(parentRepo).DbPath("graph.db")); statErr == nil {
			primaryRoot = parentRepo
		}
	}

	// Load configuration settings for workspace
	cfg, err := func() (*task.Config, error) { c, _ := workspace.NewContext(primaryRoot); return task.LoadConfig(c) }()
	if err != nil {
		// Return error if loading config failed
		return nil, err
	}
	// Create tracker instance using loaded config
	trk, err := task.NewTracker(cfg)
	if err != nil {
		// Return error if tracker initialization failed
		return nil, err
	}
	// Fetch task metadata by ID from active tracker
	return trk.View(context.Background(), id)
}
