package verify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/stretchr/testify/require"
)

func TestVerifyWalkthroughParityExtended(t *testing.T) {
	// Create mock workspace
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".nomos", "data"), 0755)

	// Mock task data in backend
	lt := task.NewLocalTracker(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(root); return c }())
	taskId, err := lt.Create(context.Background(), "Mock Task", "## ✅ Acceptance Criteria\n- [ ] Base requirement 1\n", []string{}, "", "", "Task", false, task.StatusBacklog)
	require.NoError(t, err)

	// Mock task phase state
	stateDir := filepath.Join(workspace.MustNewContext(root).StateDir())
	require.NoError(t, os.MkdirAll(stateDir, 0755))
	stateData := map[string]interface{}{
		"task_id": taskId,
	}
	stateBytes, _ := json.Marshal(stateData)
	require.NoError(t, os.WriteFile(filepath.Join(stateDir, ".phase_state.json"), stateBytes, 0644))

	// Mock plan
	plansDir := workspace.MustNewContext(root).DataPath("plans")
	require.NoError(t, os.MkdirAll(plansDir, 0755))
	planContent := `
# Implementation Plan

## Extended Acceptance Criteria
- [ ] Extended objective 2
- Extended feature 3
`
	require.NoError(t, os.WriteFile(filepath.Join(plansDir, taskId+".md"), []byte(planContent), 0644))

	// Test 1: Walkthrough missing extended criteria
	walkthroughsDir := workspace.MustNewContext(root).DataPath("walkthroughs")
	require.NoError(t, os.MkdirAll(walkthroughsDir, 0755))

	wtContent1 := `
# Walkthrough
I did base requirement 1, but nothing else.
`
	require.NoError(t, os.WriteFile(filepath.Join(walkthroughsDir, taskId+".md"), []byte(wtContent1), 0644))

	err = VerifyWalkthroughParity(root)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Extended objective 2")
	require.Contains(t, err.Error(), "Extended feature 3")

	// Test 2: Walkthrough covering all criteria
	wtContent2 := `
# Walkthrough
I did base requirement 1.
I also implemented extended objective 2 and extended feature 3!
`
	require.NoError(t, os.WriteFile(filepath.Join(walkthroughsDir, taskId+".md"), []byte(wtContent2), 0644))

	err = VerifyWalkthroughParity(root)
	require.NoError(t, err)
}
