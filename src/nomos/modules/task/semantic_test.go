package task

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCosineSimilarity(t *testing.T) {
	v1 := []float32{1.0, 2.0, 3.0}
	v2 := []float32{1.0, 2.0, 3.0}
	sim := cosineSimilarity(v1, v2)
	require.InDelta(t, 1.0, sim, 0.0001)

	v3 := []float32{-1.0, -2.0, -3.0}
	sim2 := cosineSimilarity(v1, v3)
	require.InDelta(t, -1.0, sim2, 0.0001)

	v4 := []float32{0.0, 0.0, 0.0}
	sim3 := cosineSimilarity(v1, v4)
	require.Equal(t, 0.0, sim3)
}
