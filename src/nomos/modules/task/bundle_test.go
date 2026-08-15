package task

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractBundledKeysFromDescription(t *testing.T) {
	desc := "This epic contains several tasks.\n\n**Bundled Tasks:** 123, 456, 789\nMore info here."
	keys := make(map[string]bool)
	extractBundledKeysFromDescription(desc, keys)
	require.True(t, keys["123"])
	require.True(t, keys["456"])
	require.True(t, keys["789"])
	require.False(t, keys["999"])
}

func TestGetBundledKeys(t *testing.T) {
	tasks := []Task{
		{Key: "E-1", Type: TypeBatch, Description: "**Bundled Tasks:** T-1, T-2"},
		{Key: "E-2", Type: TypeBatch, Description: "**Bundled Tasks:** T-3"},
		{Key: "T-4", Type: TypeTask, Description: "Not an epic"},
	}
	keys := getBundledKeys(tasks)
	require.True(t, keys["T-1"])
	require.True(t, keys["T-2"])
	require.True(t, keys["T-3"])
	require.False(t, keys["T-4"])
}
