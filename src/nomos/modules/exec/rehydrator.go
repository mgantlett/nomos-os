package exec

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/assets"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
)

// RehydrateWorkspace reads the embedded AGENTS.md and workflows from the Go execution substrate
// and physically writes them to the repository's localized .agents/ directory.
// This guarantees perfect determinism across repositories while satisfying the AI IDE's
// native passive file discovery requirements.
// RehydrateWorkspace orchestrates the full rehydration process, extracting embedded files
// into the local workspace .agents folder.
func RehydrateWorkspace(ctx *workspace.WorkspaceContext, cliSchemaJSON string) error {
	embeddedFS := assets.GetTemplates()

	// 1. Rehydrate AGENTS.md Protocol
	if err := rehydrateProtocol(ctx, embeddedFS, cliSchemaJSON); err != nil {
		return err
	}

	// 2. Rehydrate Workflows
	if err := rehydrateWorkflows(ctx, embeddedFS); err != nil {
		return err
	}

	// 3. Ensure Gitignore Exclusions
	if err := ensureGitignoreExclusions(ctx); err != nil {
		return err
	}

	return nil
}

// ensureGitignoreExclusions guarantees that standard Nomos temporary folders
// and execution state folders are properly ignored in the repository's git status.
func ensureGitignoreExclusions(ctx *workspace.WorkspaceContext) error {
	repoRoot := ctx.RepoRoot
	gitignorePath := filepath.Join(repoRoot, ".gitignore")
	var content string
	if data, err := os.ReadFile(gitignorePath); err == nil {
		content = string(data)
	}

	exclusions := []string{
		"." + "nomos/*",
		"." + "nomos/state/*",
		"!" + "." + "nomos/state/quality_debt.json",
		"!" + "." + "nomos/resident_guidelines/",
		"bin/",
		"tmp/",
		".nomos_parent_task",
	}

	modified := false
	for _, excl := range exclusions {
		if !strings.Contains(content, excl) {
			if len(content) > 0 && !strings.HasSuffix(content, "\n") {
				content += "\n"
			}
			content += excl + "\n"
			modified = true
		}
	}

	if modified {
		if err := os.WriteFile(gitignorePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write gitignore: %w", err)
		}
	}
	return nil
}

// rehydrateProtocol extracts the global AGENTS.md file and writes it to the global configuration directory.
func rehydrateProtocol(ctx *workspace.WorkspaceContext, embeddedFS fs.FS, cliSchemaJSON string) error {
	protocolDir := workspace.GlobalAgentConfigDir()
	if err := os.MkdirAll(protocolDir, 0755); err != nil {
		return fmt.Errorf("failed to create global protocol directory: %w", err)
	}

	protocolContent, err := fs.ReadFile(embeddedFS, "protocol/AGENTS.md")
	if err != nil {
		return nil // Graceful skip if missing
	}

	protocolPath := filepath.Join(protocolDir, "AGENTS.md")

	markerStart := "<!-- NOMOS_OS_PROTOCOL_START -->\n"
	warning := "<!-- ⚠️ NOMOS CORE OS PROTOCOL BELOW. DO NOT EDIT THIS SECTION. ⚠️ -->\n\n"
	markerEnd := "\n<!-- NOMOS_OS_PROTOCOL_END -->\n"

	finalContent := markerStart + warning + string(protocolContent)

	if cliSchemaJSON != "" {
		finalContent += "\n\n## Core Capabilities & Toolbox\n```json\n" + cliSchemaJSON + "\n```\n"
	}
	finalContent += markerEnd

	_ = os.Remove(protocolPath) // Ensure we can overwrite
	if err := os.WriteFile(protocolPath, []byte(finalContent), 0644); err != nil {
		return fmt.Errorf("failed to write global AGENTS.md: %w", err)
	}
	synapse.Info("  ✅ Synced %s", protocolPath)
	return nil
}

// purgeGhostWorkflows deletes any local workflows that are no longer embedded.
func purgeGhostWorkflows(workflowsDir string, embeddedMap map[string]bool) {
	localEntries, errLocal := os.ReadDir(workflowsDir)
	if errLocal == nil {
		for _, localEntry := range localEntries {
			if !localEntry.IsDir() && strings.HasSuffix(localEntry.Name(), ".md") {
				if !embeddedMap[localEntry.Name()] {
					ghostPath := filepath.Join(workflowsDir, localEntry.Name())
					_ = os.Remove(ghostPath)
					synapse.Info("  🗑️ Purged ghost workflow %s", ghostPath)
				}
			}
		}
	}
}

// rehydrateWorkflows extracts all global workflows and writes them to the global .agents/workflows directory.
func rehydrateWorkflows(ctx *workspace.WorkspaceContext, embeddedFS fs.FS) error {
	workflowsDir := filepath.Join(workspace.GlobalAgentConfigDir(), "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		return fmt.Errorf("failed to create global workflows directory: %w", err)
	}

	// Read the workflows directory from embedded FS
	entries, err := fs.ReadDir(embeddedFS, "workflows")
	if err != nil {
		return nil // Graceful skip if missing
	}

	embeddedMap := make(map[string]bool)
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			embeddedMap[entry.Name()] = true
		}
	}

	// Purge existing local workflows that are no longer in the embedded filesystem
	purgeGhostWorkflows(workflowsDir, embeddedMap)

	for name := range embeddedMap {
		if err := writeEmbeddedWorkflow(embeddedFS, workflowsDir, name); err != nil {
			return err
		}
	}

	return nil
}

// writeEmbeddedWorkflow writes a single workflow to the local filesystem
// It ensures that the file is written with read-only permissions and prepends
// a strict warning comment advising the user not to edit the generated file.
// If an existing file is present, it removes it first to bypass read-only locks.
func writeEmbeddedWorkflow(embeddedFS fs.FS, workflowsDir, name string) error {
	content, err := fs.ReadFile(embeddedFS, filepath.Join("workflows", name))
	if err != nil {
		return err
	}
	warning := "<!-- ⚠️ DO NOT EDIT. GENERATED BY NOMOS REHYDRATOR. TO CHANGE RULES, MODIFY SUBSTRATE IN NOMOS-COMMONS ⚠️ -->\n\n"
	finalContent := warning + string(content)
	destPath := filepath.Join(workflowsDir, name)
	_ = os.Remove(destPath) // Ensure we can overwrite read-only files
	if err := os.WriteFile(destPath, []byte(finalContent), 0444); err != nil {
		return fmt.Errorf("failed to write workflow %s: %w", name, err)
	}
	synapse.Info("  ✅ Synced %s", destPath)
	return nil
}
