// Package task manages the active state, phase discipline, and prompt engineering.
// holyghost_prompt encapsulates the Tier 2 execution context parameters.
// This handles the serialization of environmental capabilities, schema constraints,
// and current task objectives into a format consumable by subordinate AI agents.
// It enforces that agents operate strictly within their permitted boundaries.
package task

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/ast"
	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/plugin"
	"github.com/mgantlett/nomos-commons/src/nomos/core/state"
)

// CodeSnippet represents a code fragment retrieved from semantic or topological search.
type CodeSnippet struct {
	Content string `json:"content"`
	File    string `json:"file"`
}

// fetchTaskKeywords views task tracker and falls back to task.md to obtain clean keywords.
// It extracts descriptive key terms from task titles and bodies for vector semantic searches.
func fetchTaskKeywords(ctx context.Context, repoRoot string, tracker Tracker, taskKey string) string {
	// Attempt to view task record from local tracker store
	t, err := tracker.View(ctx, taskKey)
	if err != nil {
		// Log warning and attempt local task.md fallback reading
		fmt.Printf("⚠️  Holy Ghost: Unable to fetch task from tracker: %v. Checking local fallback...\n", err)
		taskMdPath := filepath.Join(config.TmpDir(repoRoot), "task.md")
		if content, err2 := os.ReadFile(taskMdPath); err2 == nil {
			return cleanKeywords(string(content))
		}
		return ""
	}
	// Concatenate task title and description string for keyword extraction
	return cleanKeywords(t.Title + " " + t.Description)
}

func cleanKeywords(raw string) string {
	// Remove non-alphanumeric characters, lowercase it, and keep it under 200 characters.
	// This sanitization is critical to prevent vector databases and semantic
	// search engines from choking on markdown syntax, JSON snippets, or
	// URL-encoded strings that are frequently pasted into task descriptions.
	// We aggressively strip punctuation to ensure only semantic tokens remain.
	reg := regexp.MustCompile(`[^a-zA-Z0-9\s]+`)
	cleaned := reg.ReplaceAllString(raw, " ")
	words := strings.Fields(cleaned)
	if len(words) == 0 {
		return ""
	}
	combined := strings.Join(words, " ")
	if len(combined) > 200 {
		combined = combined[:200]
	}
	return strings.TrimSpace(combined)
}

// writeSemanticMemoryAndCodebase runs semantic query searches on memory and codebase indexes.
// It invokes the GitBrain dense vector search engine to pull relevant past experience and code fragments.
func writeSemanticMemoryAndCodebase(f *strings.Builder, keywords string, repoRoot string) {
	fmt.Fprintln(f, "## Semantic Memory Insights")

	// Trigger the GitBrain semantic search engine.
	// GitBrain is an enterprise sub-module that leverages dense vector embeddings
	// to scan the repository's semantic memory and codebase for historical context.
	// We pass the cleaned keywords extracted from the task description into the
	// GitBrain CLI tool, which returns a structured JSON payload of relevant snippets.
	// If the module is missing or the search fails, we gracefully degrade.
	plugins, err := plugin.DiscoverPlugins(repoRoot)
	var gitbrainPlugin string
	if err == nil {
		for _, p := range plugins {
			if filepath.Base(p) == "nomos-plugin-gitbrain" {
				gitbrainPlugin = p
				break
			}
		}
	}

	if gitbrainPlugin == "" {
		fmt.Fprintln(f, "_Semantic matches require the GitBrain enterprise module._")
		fmt.Fprintln(f, "")
		fmt.Fprintln(f, "## Relevant Codebase Snippets")
		fmt.Fprintln(f, "_Semantic matches require the GitBrain enterprise module._")
		return
	}

	out, err := plugin.CallPlugin(gitbrainPlugin, "search", map[string]string{
		"keywords": keywords,
		"repoRoot": repoRoot,
	})

	if err != nil {
		fmt.Fprintln(f, "_Semantic matches require the GitBrain enterprise module._")
		fmt.Fprintln(f, "")
		fmt.Fprintln(f, "## Relevant Codebase Snippets")
		fmt.Fprintln(f, "_Semantic matches require the GitBrain enterprise module._")
		return
	}

	var results struct {
		Notes []struct {
			Content string `json:"content"`
		} `json:"notes"`
		Code []CodeSnippet `json:"code"`
	}

	// Parse JSON vector search result payload from GitBrain stdout
	if err := json.Unmarshal(out, &results); err != nil {
		fmt.Fprintln(f, "_Error parsing GitBrain response._")
		return
	}

	// Format matched memory notes into Markdown bullet list items
	for _, n := range results.Notes {
		fmt.Fprintf(f, "- %s\n", strings.ReplaceAll(n.Content, "\n", " "))
	}
	if len(results.Notes) == 0 {
		fmt.Fprintln(f, "_No relevant memory insights found._")
	}

	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "## Relevant Codebase Snippets")

	// We pass the GitBrain code matches as a slice of CodeSnippet structs to the new fetchCodebaseSnippets helper.
	// This reduces cyclomatic complexity while enabling the topological graph search.
	var convertedCode []CodeSnippet
	for _, c := range results.Code {
		convertedCode = append(convertedCode, CodeSnippet{Content: c.Content, File: c.File})
	}
	fetchCodebaseSnippets(f, convertedCode, repoRoot)
}

// fetchCodebaseSnippets retrieves contextual codebase snippets either from the deterministic AST
// topological graph or falls back to GitBrain fuzzy matching if the AST is unavailable.
// It uses recursive CTEs bounded to depth 3 to avoid exceeding context windows.
func fetchCodebaseSnippets(f *strings.Builder, resultsCode []CodeSnippet, repoRoot string) {
	// Extract seed file paths from vector search matches
	var seedFiles []string
	for _, c := range resultsCode {
		seedFiles = append(seedFiles, c.File)
	}

	if len(seedFiles) == 0 {
		fmt.Fprintln(f, "_No relevant codebase snippets found._")
		return
	}

	// Build and persist DAG to the global AST SQLite DB.
	g, err := ast.BuildDependencyGraph(repoRoot)
	if err == nil {
		_ = ast.SaveGraphToDB(repoRoot, g)
		topoFiles, _ := ast.QueryTopologicalContext(repoRoot, seedFiles)

		// De-duplicate seed and topologically discovered file paths
		fileSet := make(map[string]bool)
		for _, f := range seedFiles {
			fileSet[f] = true
		}
		for _, f := range topoFiles {
			fileSet[f] = true
		}

		// Read and format source file contents into Markdown code blocks
		for file := range fileSet {
			content, err := os.ReadFile(filepath.Join(repoRoot, file))
			if err == nil {
				fmt.Fprintf(f, "**%s**\n```\n%s\n```\n", file, string(content))
			}
		}
	} else {
		// Fallback to GitBrain purely if AST graph fails
		for _, c := range resultsCode {
			fmt.Fprintf(f, "**%s**\n```\n%s\n```\n", c.File, c.Content)
		}
	}
}

// writePhaseAndModelGuidelines appends active sprint phase rules, model rules, and AGENT.md guidelines.
// It loads phase state JSON configuration and injects localized phase protocol constraints.
func writePhaseAndModelGuidelines(f *strings.Builder, repoRoot string) {
	// Resolve phase state JSON filepath
	// The phase state document dictates the cognitive constraints
	// of the active agent, determining what it is allowed to modify
	// (e.g., PLAN phase blocks source code editing, EDIT phase enforces TDD).
	phaseStatePath := config.PhaseStatePath(repoRoot)
	var phase string
	var model string
	if data, err := os.ReadFile(phaseStatePath); err == nil {
		var state struct {
			Agent        string `json:"agent"`
			CurrentPhase string `json:"current_phase"`
		}
		// Unmarshal phase state JSON object
		if err := json.Unmarshal(data, &state); err == nil {
			phase = state.CurrentPhase
			model = state.Agent
		}
	}

	// Write phase guidelines section to prompt file
	writePhaseGuidelines(f, repoRoot, state.WorkspacePhase(phase))
	// Write model guidelines section to prompt file
	writeModelGuidelines(f, repoRoot, model)
	// Write workspace protocol guidelines section to prompt file
	writeWorkspaceProtocol(f, repoRoot)
}

// writePhaseGuidelines writes active sprint phase rules based on phase name.
// It directly injects substrate-embedded default PLAN, EDIT, and REVIEW prompts.
func writePhaseGuidelines(f *strings.Builder, repoRoot string, phase state.WorkspacePhase) {
	// Guard against empty phase string inputs
	if phase == "" {
		return
	}
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "## Active Sprint Phase Guidelines")

	switch phase {
	case state.PhasePlan:
		// Inject PLAN phase restriction guidelines
		fmt.Fprintln(f, "> [!IMPORTANT]\n> **Phase: PLAN**\n> Focus exclusively on drafting the implementation_plan.md. Do not write, modify, or delete any source code files until the Product Owner has transitioned the workspace to the EDIT phase.")
	case state.PhaseEdit:
		// Inject EDIT phase TDD guidelines
		fmt.Fprintln(f, "> [!NOTE]\n> **Phase: EDIT**\n> Implement the PO signed-off implementation plan. Enforce the test-driven development (TDD) loop: write failing tests first, then implementation code.")
	case state.PhaseReview:
		// Inject REVIEW phase sign-off guidelines
		fmt.Fprintf(f, "> [!IMPORTANT]\n> **Phase: REVIEW**\n> Prepare execution %s. Verify walkthrough report is checked into repository specifications folder (%s).\n", config.WalkthroughFileName, config.WalkthroughFinalPath(repoRoot, "<taskId>"))
	}
}

// writeModelGuidelines writes model rules or default tip to the context file.
// It relies exclusively on substrate-embedded conditionals for specific AI models.
func writeModelGuidelines(f *strings.Builder, repoRoot string, model string) {
	// Guard against empty model string inputs
	if model == "" {
		return
	}

	if strings.Contains(strings.ToLower(model), "gemini") || strings.Contains(strings.ToLower(model), "antigravity") {
		fmt.Fprintln(f, "")
		fmt.Fprintln(f, "## Model Execution Protocol")
		// Embedded default guidelines for Gemini and Antigravity models
		fmt.Fprintln(f, "> [!TIP]\n> **Model Specific Guidelines (Gemini/Antigravity)**\n> Keep responses extremely concise. Output code-first modifications. Limit unrequested text explanations to at most three lines.")
	}
}

// writeWorkspaceProtocol appends unified workspace AGENTS.md rules.
// It reads both the global OS protocol and workspace-level AGENTS.md configuration documents and embeds them into the prompt.
func writeWorkspaceProtocol(f *strings.Builder, repoRoot string) {
	// 1. Read Global OS Protocol
	globalAgentMdPath := filepath.Join(config.GlobalAgentConfigDir(), "AGENTS.md")
	if _, err := os.Stat(globalAgentMdPath); err == nil {
		protocolData, err := os.ReadFile(globalAgentMdPath)
		if err == nil {
			fmt.Fprintln(f, "## Global Nomos OS Protocol")
			fmt.Fprintln(f, string(protocolData))
			fmt.Fprintln(f, "")
		}
	}

	// 2. Read Local Workspace Rules
	localAgentMdPath := config.WorkspaceAgentConfigPath(repoRoot)
	if _, err := os.Stat(localAgentMdPath); err == nil {
		protocolData, err := os.ReadFile(localAgentMdPath)
		if err == nil {
			fmt.Fprintln(f, "## Workspace Project Rules")
			fmt.Fprintln(f, string(protocolData))
			fmt.Fprintln(f, "")
		} else {
			fmt.Printf("⚠️  Holy Ghost: Failed to read local AGENTS.md from %s: %v\n", localAgentMdPath, err)
		}
	}
}

// gatherActiveContext builds the gravity anchor string from the task's implementation plan.
// It parses file:// links embedded in implementation_plan.md and reads target source contents.
func gatherActiveContext(repoRoot string, taskKey string) string {
	planPath := filepath.Join(config.PlansDir(repoRoot), taskKey, "implementation_plan.md")
	planData, err := os.ReadFile(planPath)
	if err != nil {
		return ""
	}

	var activeContext strings.Builder
	activeContext.WriteString(string(planData))
	activeContext.WriteString("\n")

	// Parse file:// URI patterns to pull referenced source files into active context
	re := regexp.MustCompile(`file://([^\)]+)`)
	matches := re.FindAllStringSubmatch(string(planData), -1)
	for _, match := range matches {
		if data, err := os.ReadFile(match[1]); err == nil {
			activeContext.WriteString(string(data))
			activeContext.WriteString("\n")
		}
	}

	return activeContext.String()
}

// writeResidentGuidelines reads and appends Markdown files from both global (<repoRoot>/.nomos/data/resident_guidelines)
// and project-level (<repoRoot>/.agents/resident_guidelines) directories.
func writeResidentGuidelines(f *strings.Builder, repoRoot string, taskKey string) {
	dirs := []string{
		filepath.Join(config.GlobalDataDir(repoRoot), "resident_guidelines"),
		filepath.Join(repoRoot, ".agents", "resident_guidelines"),
	}

	hasGuidelines := false
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}

			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				continue
			}

			if !hasGuidelines {
				fmt.Fprintln(f, "\n## 🪐 Nomos Gravity: Architectural Goals")
				hasGuidelines = true
			}
			fmt.Fprintf(f, "### %s\n%s\n\n", entry.Name(), string(data))
		}
	}
}

// writeBatchBoard appends the current Active Context Batch (DAG) context.
// It queries the task tracker to identify unblocked tasks in the active execution batch.
func writeBatchBoard(f *strings.Builder, repoRoot string, tracker Tracker) {
	ctx := context.Background()
	tasks, err := tracker.List(ctx)
	if err != nil {
		return
	}

	// Filter active context batch tasks using dependency graph status
	activeBatch := GetActiveContextBatch(tasks)
	if len(activeBatch) == 0 {
		return
	}

	fmt.Fprintln(f, "## Active Context Batch (Topologically Unblocked)")
	for _, t := range activeBatch {
		fmt.Fprintf(f, "- [%s] %s (Labels: %v)\n", t.Key, t.Title, t.Labels)
	}
	fmt.Fprintln(f, "")
}

// writeTaskPlanAndWorkflows appends the active task's implementation plan and phase-based workflows.
// It inspects current sprint phase state and injects relevant compacted playbook instructions.
func writeTaskPlanAndWorkflows(f *strings.Builder, repoRoot string, taskKey string) {
	// 1. Read and append active task implementation plan
	planPath := filepath.Join(config.PlansDir(repoRoot), taskKey, "implementation_plan.md")
	if data, err := os.ReadFile(planPath); err == nil {
		fmt.Fprintln(f, "")
		fmt.Fprintln(f, "## Active Implementation Plan (spec/implementation_plan.md)")
		fmt.Fprintln(f, string(data))
	}

	// 2. Read phase name
	phaseStatePath := config.PhaseStatePath(repoRoot)
	var phase state.WorkspacePhase
	if data, err := os.ReadFile(phaseStatePath); err == nil {
		var stateData struct {
			CurrentPhase state.WorkspacePhase `json:"current_phase"`
		}
		if err := json.Unmarshal(data, &stateData); err == nil {
			phase = stateData.CurrentPhase
		}
	}

	// 3. Append compacted workflows based on current sprint phase
	fmt.Fprintln(f, "")
	fmt.Fprintln(f, "## Compacted Workflow Guidelines")
	fmt.Fprintln(f, "The following are playbooks relevant to your current phase. Trigger them when needed:")

	workflowsDir := config.WorkflowsDir(repoRoot)

	// Append playbooks mapped to the active workspace phase
	switch phase {
	case state.PhasePlan:
		appendWorkflowToFile(f, workflowsDir, "nomos-start")
		appendWorkflowToFile(f, workflowsDir, "nomos-verify")
	case state.PhaseEdit:
		appendWorkflowToFile(f, workflowsDir, "nomos-verify")
		appendWorkflowToFile(f, workflowsDir, "nomos-triage")
		appendWorkflowToFile(f, workflowsDir, "nomos-refactor")
	case state.PhaseReview:
		appendWorkflowToFile(f, workflowsDir, "nomos-close")
		appendWorkflowToFile(f, workflowsDir, "nomos-push")
	default:
		appendWorkflowToFile(f, workflowsDir, "nomos-verify")
	}
}

// appendWorkflowToFile appends the first 25 lines of a playbook file to the target context file f.
// It strips YAML frontmatter and truncates long playbooks to optimize LLM prompt token context efficiency.
func appendWorkflowToFile(f *strings.Builder, workflowsDir string, name string) {
	path := filepath.Join(workflowsDir, name+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	fmt.Fprintf(f, "\n### /%s Playbook\n", name)
	lines := strings.Split(string(data), "\n")
	count := 0
	for _, line := range lines {
		// Skip frontmatter separator lines
		if strings.HasPrefix(line, "---") {
			continue
		}
		fmt.Fprintln(f, line)
		count++
		// Truncate playbook content beyond 25 lines
		if count > 25 {
			fmt.Fprintln(f, "... [truncated for context efficiency]")
			break
		}
	}
}
