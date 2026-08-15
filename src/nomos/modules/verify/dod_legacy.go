package verify

import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"fmt"
	"strings"
)

func runLegacyCodeBlockerCheck(r string) (StageResult, error) {
	res := StageResult{Name: "Legacy Code Blocker", Passed: true}

	// Get all modified/staged files in the active workspace
	modifiedMap, err := GetModifiedFiles(r)
	if err != nil {
		res.Passed = false
		res.Error = fmt.Errorf("failed to get modified files: %w", err)
		return res, nil
	}

	var files []string
	for f := range modifiedMap {
		files = append(files, f)
	}

	violations, err := AuditImports(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(r); return c }(), files)
	if err != nil {
		res.Passed = false
		res.Error = fmt.Errorf("import audit failed: %w", err)
		return res, nil
	}

	if len(violations) > 0 {
		res.Passed = false
		res.Error = fmt.Errorf("banned import violations detected:\n - %s", strings.Join(violations, "\n - "))
	} else {
		res.Message = "No banned imports or phrases detected in modified files."
	}
	return res, nil
}
