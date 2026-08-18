// Package schema manages structured representations of the workspace state.
// The WalkthroughSchema ensures that end-of-task reviews match acceptance criteria.
// It mandates that agents provide a structured explanation of completed work,
// including what was tested and how UI/state changes were validated.
package schema

import "strings"

// WalkthroughSchema represents the structural schema of a Task Walkthrough
type WalkthroughSchema struct {
	ChangesMade          string
	ArchitecturalContext string
	ImpactList           string
	ResolutionDetails    string
	ValidationResults    string
}

func (s *WalkthroughSchema) GenerateMarkdown() string {
	var sb strings.Builder

	sb.WriteString("# Task Walkthrough\n\n")

	sb.WriteString("## 📝 Changes Made\n")
	if s.ChangesMade != "" {
		sb.WriteString(s.ChangesMade + "\n\n")
	} else {
		sb.WriteString("<Summarize what was changed>\n\n")
	}

	sb.WriteString("## **Architectural Context:**\n")
	if s.ArchitecturalContext != "" {
		sb.WriteString("- " + s.ArchitecturalContext + "\n\n")
	} else {
		sb.WriteString("- <Describe architectural reasoning here>\n\n")
	}

	sb.WriteString("## **Impact List:**\n")
	if s.ImpactList != "" {
		sb.WriteString("- " + s.ImpactList + "\n\n")
	} else {
		sb.WriteString("- <List files or components changed>\n\n")
	}

	sb.WriteString("## **Resolution Details:**\n")
	if s.ResolutionDetails != "" {
		sb.WriteString("- " + s.ResolutionDetails + "\n\n")
	} else {
		sb.WriteString("- <Describe exactly how the issue was resolved>\n\n")
	}

	sb.WriteString("## **Validation Results:**\n")
	if s.ValidationResults != "" {
		sb.WriteString("- " + s.ValidationResults + "\n")
	} else {
		sb.WriteString("- <List tests run and validations performed>\n")
	}

	return sb.String()
}
