// Package verify contains the Definition of Done quality gates and verification engines.
// This specific file contains security-related checks such as verifying locked permissions
// and scanning for embedded secrets before committing code.
// The DoD gates ensure all Go code is properly formatted and safe to be executed.
package verify

import (
	"fmt"
	"strings"
)

func runSecurityAuditCheck(r string) (StageResult, error) {
	res := StageResult{Name: "Security Audit", Passed: true}
	findings, err := ScanSecurity(r)
	if err != nil {
		res.Passed = false
		res.Error = fmt.Errorf("security scan failed: %w", err)
		return res, nil
	}

	criticalOrHigh := 0
	var summary []string
	for _, f := range findings {
		if f.Severity == SeverityCritical || f.Severity == SeverityHigh {
			criticalOrHigh++
			summary = append(summary, fmt.Sprintf("- [%s] %s:%d: %s", f.Severity, f.File, f.Line, f.Message))
		}
	}

	if criticalOrHigh > 0 {
		res.Passed = false
		res.Error = fmt.Errorf("security audit failed with %d critical/high finding(s):\n%s", criticalOrHigh, strings.Join(summary, "\n"))
	} else {
		res.Message = fmt.Sprintf("Passed with %d low/medium finding(s)", len(findings))
	}

	res.Metrics = map[string]interface{}{
		"finding_count":               len(findings),
		"finding_count_critical_high": criticalOrHigh,
	}

	return res, nil
}
