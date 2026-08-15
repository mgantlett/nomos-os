package verify

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CheckHardcodedPaths scans modified source code files to ensure they don't contain
// hardcoded string literals for system paths like ".nomos/", ".agent/", or ".gemini/".
// These paths must be centralized in the config/paths.go package.
// This prevents bugs caused by assumptions about path structures that may change.
func CheckHardcodedPaths(root string, files []string) []ComplexityFinding {
	// findings stores all instances where a hardcoded path literal was found.
	// We return this slice so the calling verification stage can format the output.
	var findings []ComplexityFinding

	// We only want to flag these specific string literals in the code.
	// We use strings with a leading quote to avoid matching things like `nomos/tasks`.
	forbiddenSubstrings := []string{
		"\".nomos/",
		"\".agent/",
		"\".agents/",
		"\".gemini/",
	}

	for _, f := range files {
		fSlash := filepath.ToSlash(f)

		// Skip path centralizer itself and tests
		if strings.HasSuffix(fSlash, "paths.go") || strings.HasSuffix(fSlash, "_test.go") {
			continue
		}

		// Only check Go files for simplicity and to avoid false positives in markdown
		if !strings.HasSuffix(fSlash, ".go") {
			continue
		}

		absPath := filepath.Join(root, f)
		file, err := os.Open(absPath)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(file)
		lineNum := 1
		for scanner.Scan() {
			line := scanner.Text()
			for _, forbidden := range forbiddenSubstrings {
				if strings.Contains(line, forbidden) {
					// We use ComplexityFinding struct as a generic finding container
					findings = append(findings, ComplexityFinding{
						File:    f,
						Func:    fmt.Sprintf("hardcoded string: %s", forbidden),
						Line:    lineNum,
						Value:   0,
						IsError: true,
					})
				}
			}
			lineNum++
		}
		file.Close()
	}

	return findings
}
