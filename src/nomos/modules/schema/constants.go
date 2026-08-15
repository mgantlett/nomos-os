// Package schema manages structured representations of the workspace state.
// This file centralizes global constants used across schema parsing and validation.
// Centralizing these constants prevents drift and reduces duplicated string literals
// throughout the codebase, promoting maintainability.
package schema

const (
	// DeepReviewChecklistItem is the strictly enforced checklist item required for AI agents to pass the DoR deep-review check.
	DeepReviewChecklistItem = "- [ ] Run a `/deep-review` on the active story before generating the implementation plan"
)
