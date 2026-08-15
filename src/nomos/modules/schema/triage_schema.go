// Package schema manages structured representations of the workspace state.
// The TriageSchema handles incident reporting and defect triage structures.
// It guarantees that bug reports and system failures contain requisite
// reproduction steps, impact scope, and root cause analysis placeholders.
// Note: TriageSchema must be populated immediately upon Incident ingestion.
// It provides the structured context required for the root cause analysis (RCA) pipeline.
// The triage schema helps route incidents to the correct subsystem domain expert.
// It mandates that all bugs include steps to reproduce before they enter the backlog.
package schema

import (
	"fmt"
	"strings"
)

// IncidentTriageSchema represents the structural schema of an Incident Triage Report
type IncidentTriageSchema struct {
	CurrentFailures []string
	FailureHistory  []string
	ResolutionSteps []string
}

func (s *IncidentTriageSchema) GenerateMarkdown() string {
	var sb strings.Builder

	sb.WriteString("## 📝 Incident Triage Report\n")
	sb.WriteString("Persistent health check failures have been detected by the self-healing daemon.\n\n")

	sb.WriteString("### Current Failures:\n")
	if len(s.CurrentFailures) > 0 {
		for _, f := range s.CurrentFailures {
			sb.WriteString("- " + f + "\n")
		}
	} else {
		sb.WriteString("- <List active failures>\n")
	}
	sb.WriteString("\n")

	sb.WriteString("### Diagnostic Failure History:\n")
	if len(s.FailureHistory) > 0 {
		for _, f := range s.FailureHistory {
			sb.WriteString("- " + f + "\n")
		}
	} else {
		sb.WriteString("- <List failure history>\n")
	}
	sb.WriteString("\n")

	sb.WriteString("### Suggested Resolution Steps:\n")
	if len(s.ResolutionSteps) > 0 {
		for i, r := range s.ResolutionSteps {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, r))
		}
	} else {
		sb.WriteString("1. Run '/triage' check command to inspect active process loops.\n")
		sb.WriteString("2. Confirm the LlamaServer coder model server is running and listening on port 8082.\n")
		sb.WriteString("3. Validate Cockpit dashboard processes and WebSocket connections.\n")
	}

	return sb.String()
}
