package task

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/stretchr/testify/require"
)

func TestLocalTracker(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nomos_local_tracker_test")
	require.NoError(t, err)
	os.MkdirAll(filepath.Join(tmpDir, ".nomos", "data", "db"), 0755)
	defer os.RemoveAll(tmpDir)

	tracker := NewLocalTracker(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(tmpDir); return c }())
	require.NotNil(t, tracker)

	ctx := context.Background()
	tasks, err := tracker.List(ctx)
	require.NoError(t, err)
	require.Empty(t, tasks)

	// Create a task
	key, err := tracker.Create(context.Background(), "Test Task", "Desc", []string{}, "", "TEST", TypeTask, false, StatusBacklog)
	require.NoError(t, err)
	require.NotEmpty(t, key)

	tasks, err = tracker.List(ctx)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, "Test Task", tasks[0].Title)
}
