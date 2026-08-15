package task

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewTracker(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nomos_tracker_test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	cfg := &Config{
		TrackerType: "local",
		RepoRoot:    tmpDir,
	}
	tracker, err := NewTracker(cfg)
	require.NoError(t, err)
	require.NotNil(t, tracker)

	cfg2 := &Config{
		TrackerType: "unknown",
	}
	_, err = NewTracker(cfg2)
	require.Error(t, err)
}
