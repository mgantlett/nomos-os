package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mgantlett/nomos-commons/src/nomos/core/ast"
	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/gitops"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/mgantlett/nomos-os/src/nomos/modules/verify"
)

// ParseAstTool exposes the AST parsing functionality.
type ParseAstTool struct{}

func (t *ParseAstTool) Definition() McpTool {
	return McpTool{
		Name:        "parse_ast",
		Description: "Perform AST symbol extraction on Go/Python/JS/TS source file",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"filePath": map[string]interface{}{
					"type": "string",
				},
			},
			"required": []string{"filePath"},
		},
	}
}

func (t *ParseAstTool) Handle(ctx *workspace.WorkspaceContext, args json.RawMessage) (string, error) {
	var p struct {
		FilePath string `json:"filePath"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", err
	}
	absPath := filepath.Join(ctx.RepoRoot, p.FilePath)
	res, err := ast.ParseFile(absPath)
	if err != nil {
		return "", err
	}
	bytes, _ := json.MarshalIndent(res.Symbols, "", "  ")
	return string(bytes), nil
}

// VerifyDodTool exposes the Definition of Done verification.
type VerifyDodTool struct{}

func (t *VerifyDodTool) Definition() McpTool {
	return McpTool{
		Name:        "verify_dod",
		Description: "Execute the Go-native concurrent Definition of Done (DoD) verification checks",
		InputSchema: map[string]interface{}{
			"type": "object",
		},
	}
}

func (t *VerifyDodTool) Handle(ctx *workspace.WorkspaceContext, args json.RawMessage) (string, error) {
	err := verify.VerifyDoD(ctx)
	if err != nil {
		return "", fmt.Errorf("❌ Definition of Done verification failed: %v", err)
	}
	return "✅ Definition of Done verification succeeded!", nil
}

// TaskSyncTool exposes the Direct Sync capability securely.
type TaskSyncTool struct{}

func (t *TaskSyncTool) Definition() McpTool {
	return McpTool{
		Name:        "nomos_task_sync",
		Description: "Natively execute AI-AI DDP Direct Sync with strictly typed commit schemas",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"target_env": map[string]interface{}{
					"type":        "string",
					"description": "The branch to merge into (usually 'develop')",
				},
				"impact_list": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "string",
					},
					"description": "List of technical impacts this change has",
				},
				"resolution_details": map[string]interface{}{
					"type":        "string",
					"description": "Detailed description of the resolution",
				},
			},
			"required": []string{"target_env", "impact_list", "resolution_details"},
		},
	}
}

func (t *TaskSyncTool) Handle(ctx *workspace.WorkspaceContext, args json.RawMessage) (string, error) {
	var p struct {
		TargetEnv         string   `json:"target_env"`
		ImpactList        []string `json:"impact_list"`
		ResolutionDetails string   `json:"resolution_details"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", err
	}

	taskID := verify.GetActiveTaskId(ctx.RepoRoot)
	if taskID == "" {
		return "", fmt.Errorf("no active task found to sync")
	}

	wtPath := filepath.Join(ctx.WorktreesDir(), filepath.Base(ctx.RepoRoot)+"-"+taskID)
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		return "", fmt.Errorf("active task worktree not found at %s", wtPath)
	}

	// We create a temporary walkthrough file to serve as the commit message payload
	// The gitops.DirectMerge expects a file path.
	commitMsg := fmt.Sprintf("[Task %s] Direct Sync\n\n**Impact List:**\n", taskID)
	for _, imp := range p.ImpactList {
		commitMsg += fmt.Sprintf("- %s\n", imp)
	}
	commitMsg += fmt.Sprintf("\n**Resolution Details:**\n%s\n", p.ResolutionDetails)

	tmpFile := filepath.Join(ctx.TmpDir(), fmt.Sprintf("commit_payload_%s.md", taskID))
	_ = os.WriteFile(tmpFile, []byte(commitMsg), 0644)

	err := gitops.DirectMerge(wtPath, ctx, p.TargetEnv, tmpFile)
	if err != nil {
		return "", fmt.Errorf("direct sync failed: %w", err)
	}

	cfg, err := task.LoadConfig(ctx)
	if err == nil {
		tracker, err := task.NewTracker(cfg)
		if err == nil {
			bgCtx := context.Background()
			_ = tracker.Close(bgCtx, taskID, "Synced natively via nomos_task_sync MCP")
			
			if s, _ := task.GetPhaseState(ctx); s != nil && s.TaskId == taskID {
				_ = task.TransitionPhase(ctx, statepkg.PhaseIdle)
			}
		}
	}

	return fmt.Sprintf("✅ Active task %s successfully synced to %s", taskID, p.TargetEnv), nil
}
