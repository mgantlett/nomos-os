package verify

import (
	"fmt"
	"testing"
)

func TestDensityOutput(t *testing.T) {
	files := []string{
		"walkthrough_parity.go",
		"dod.go",
		"dod_helpers.go",
	}
	for _, f := range files {
		comments, code, err := calculateCommentDensity(f, make(map[string]int))
		if err != nil {
			fmt.Printf("%s: ERROR: %v\n", f, err)
		} else {
			density := (float64(comments) / float64(code)) * 100.0
			fmt.Printf("%s: %.2f%% (comments: %d, code: %d)\n", f, density, comments, code)
		}
	}
}
