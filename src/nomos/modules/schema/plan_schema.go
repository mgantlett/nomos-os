// Package schema manages structured representations of the workspace state.
// The PlanSchema defines the template structure for AI implementation plans.
// It guarantees that all generated plans include a clear goal description,
// explicit identification of required user reviews, open questions, and a structured
// verification plan prior to execution phase transition.
// Note: The PlanSchema acts as a contract between the human Product Owner and the autonomous agent.
// By isolating planning from execution, it prevents unchecked AI hallucination during complex tasks.
// The goal description should be a high-level summary of the implementation strategy.
// Open questions must be resolved before any code is modified.
package schema

import "strings"

// PlanSchema represents the structural schema of an Implementation Plan
type PlanSchema struct {
	GoalDescription    string
	UserReviewRequired string
	OpenQuestions      string
	ProposedChanges    string
	VerificationPlan   string
}

func (s *PlanSchema) GenerateMarkdown() string {
	var sb strings.Builder

	sb.WriteString("# Implementation Plan\n\n")

	sb.WriteString("## 📝 Goal Description\n")
	if s.GoalDescription != "" {
		sb.WriteString(s.GoalDescription + "\n\n")
	} else {
		sb.WriteString("<Insert Goal Description Here>\n\n")
	}

	sb.WriteString("## ⚠️ User Review Required\n")
	if s.UserReviewRequired != "" {
		sb.WriteString(s.UserReviewRequired + "\n\n")
	} else {
		sb.WriteString("None.\n\n")
	}

	sb.WriteString("## ❓ Open Questions\n")
	if s.OpenQuestions != "" {
		sb.WriteString(s.OpenQuestions + "\n\n")
	} else {
		sb.WriteString("None.\n\n")
	}

	sb.WriteString("## 🛠️ Proposed Changes\n")
	if s.ProposedChanges != "" {
		sb.WriteString(s.ProposedChanges + "\n\n")
	} else {
		sb.WriteString("<List file-level changes here>\n\n")
	}

	sb.WriteString("## 🛡️ Verification Plan\n")
	if s.VerificationPlan != "" {
		sb.WriteString(s.VerificationPlan + "\n\n")
	} else {
		sb.WriteString("<List how you will verify changes here>\n")
	}

	return sb.String()
}
