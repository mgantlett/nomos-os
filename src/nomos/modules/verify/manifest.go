package verify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
)

// readQualityDebtManifest decodes the technical debt manifest JSON file
// located inside the .agent/ repository state directory. If the file
// does not exist, an empty manifest is returned.
// readQualityDebtManifest reads the manifest file, automatically deduplicating items
// and filtering out debt linked to closed tasks.
func readQualityDebtManifest(repoRoot string) (QualityDebtManifest, error) {
	var manifest QualityDebtManifest
	manifestPath := filepath.Join(workspace.MustNewContext(repoRoot).DataDir(), "state", "quality_debt.json")

	// Check if the manifest file exists before attempting to read it
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		return manifest, err
	}

	// Read raw bytes from the manifest file
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return manifest, err
	}

	// Deserialize json content into structural manifest data type
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}

	// Deduplicate active debt entries by (File, Gate) and filter out closed tasks.
	// This inline cleanup pass ensures that stale quality debt entries and duplicate
	// bypass declarations do not accumulate over multiple development iterations.
	seen := make(map[string]bool)
	var cleaned []QualityDebtItem
	modified := false

	for _, item := range manifest.ActiveDebt {
		rel := getRelativePath(repoRoot, item.File)
		key := rel + "::" + item.Gate
		tID := strings.TrimSpace(item.LinkedTask)

		// Check if the linked task file exists in .nomos/tasks and is currently closed.
		// If the associated backlog issue has been closed, the quality debt item is pruned.
		if isTaskTerminal(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), tID) {
			modified = true
			continue
		}

		// Enforce unique (File, Gate) keys in the active debt array.
		if !seen[key] {
			seen[key] = true
			if item.File != rel {
				item.File = rel
				modified = true
			}
			cleaned = append(cleaned, item)
		} else {
			// Duplicate entry detected; mark modified to rewrite clean manifest
			modified = true
		}
	}

	// Persist the sanitized and deduplicated quality debt manifest back to disk if changes occurred.
	if modified {
		manifest.ActiveDebt = cleaned
		manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
		if err == nil {
			_ = os.MkdirAll(filepath.Dir(manifestPath), 0755)
			_ = os.Chmod(manifestPath, 0644)
			_ = os.WriteFile(manifestPath, manifestBytes, 0644)
		}
	}

	return manifest, nil
}

// writeQualityDebtManifest commits the updated manifest array back to disk after deduplicating items.
func writeQualityDebtManifest(repoRoot string, activeDebt []QualityDebtItem) {
	manifestPath := filepath.Join(workspace.MustNewContext(repoRoot).DataDir(), "state", "quality_debt.json")
	_ = os.Chmod(manifestPath, 0644)

	// Deduplicate active debt entries by (File, Gate) to prevent file bloating
	seen := make(map[string]bool)
	var deduped []QualityDebtItem
	for _, item := range activeDebt {
		rel := getRelativePath(repoRoot, item.File)
		key := rel + "::" + item.Gate
		if !seen[key] {
			seen[key] = true
			item.File = rel
			deduped = append(deduped, item)
		}
	}

	manifest := QualityDebtManifest{ActiveDebt: deduped}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err == nil {
		_ = os.MkdirAll(filepath.Dir(manifestPath), 0755)
		_ = os.WriteFile(manifestPath, manifestBytes, 0644)
		// Update the deterministic workspace hash to prevent Data Integrity Gate failures
		_ = task.UpdateWorkspaceStateHash(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())

		// Dynamically sync and update unified backlog task files for all linked task IDs
		SyncQualityDebtStories(repoRoot)
	}
}
