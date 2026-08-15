// Package schema manages structured representations of the workspace state.
// The CommitSchema struct encapsulates the exact format required for all Git commits
// executed via the nomos engine. It enforces the inclusion of structured reasoning,
// architectural context, and resolution details to guarantee high-quality project history.
// Note: The CommitSchema is evaluated prior to any git push.
// It ensures that every single modification can be traced back to a verified task.
package schema

import (
	"strings"
)

// CommitSchema represents the structural schema of a Nomos Commit Message
type CommitSchema struct {
	Subject              string
	ArchitecturalContext []string
	ImpactList           []string
	ResolutionDetails    []string
}

// GenerateMarkdown converts the struct into a unified Markdown string
func (s *CommitSchema) GenerateMarkdown() string {
	var sb strings.Builder

	if s.Subject != "" {
		sb.WriteString(s.Subject + "\n\n")
	} else {
		sb.WriteString("[Task <ID>] <summary>\n\n")
	}

	sb.WriteString("**Architectural Context:**\n")
	if len(s.ArchitecturalContext) > 0 {
		for _, ctx := range s.ArchitecturalContext {
			sb.WriteString("- " + ctx + "\n")
		}
	} else {
		sb.WriteString("- <Describe architectural reasoning here>\n")
	}
	sb.WriteString("\n")

	sb.WriteString("**Impact List:**\n")
	if len(s.ImpactList) > 0 {
		for _, imp := range s.ImpactList {
			sb.WriteString("- " + imp + "\n")
		}
	} else {
		sb.WriteString("- <List files or components changed>\n")
	}
	sb.WriteString("\n")

	sb.WriteString("**Resolution Details:**\n")
	if len(s.ResolutionDetails) > 0 {
		for _, res := range s.ResolutionDetails {
			sb.WriteString("- " + res + "\n")
		}
	} else {
		sb.WriteString("- <Describe exactly how the issue was resolved>\n")
	}
	sb.WriteString("\n")

	return sb.String()
}
