// Package schema manages structured representations of the workspace state.
// The WalkthroughSchema ensures that end-of-task reviews match acceptance criteria.
// It mandates that agents provide a structured explanation of completed work,
// including what was tested and how UI/state changes were validated.
package schema

import "strings"

// WalkthroughSchema represents the structural schema of a Task Walkthrough
type WalkthroughSchema struct {
	ChangesMade       string
	WhatWasTested     string
	ValidationResults string
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

	sb.WriteString("## 🛡️ What was Tested\n")
	if s.WhatWasTested != "" {
		sb.WriteString(s.WhatWasTested + "\n\n")
	} else {
		sb.WriteString("<List the tests run>\n\n")
	}

	sb.WriteString("## ✅ Validation Results\n")
	if s.ValidationResults != "" {
		sb.WriteString(s.ValidationResults + "\n\n")
	} else {
		sb.WriteString("<Describe the outcome of verification>\n")
	}

	return sb.String()
}
