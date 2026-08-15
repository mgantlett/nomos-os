package task

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCleanKeywords(t *testing.T) {
	raw := "foo\nbar  baz\tqux"
	cleaned := cleanKeywords(raw)
	// Should remove newlines and extra spaces
	require.Contains(t, cleaned, "foo")
	require.Contains(t, cleaned, "bar")
	require.NotContains(t, cleaned, "\n")
}
