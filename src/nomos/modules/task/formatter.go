package task

import (
	"fmt"
)

// FormatTaskSummary converts a Task into a human-readable string containing
// the task's ID, Status, Summary, and Assignee.
func FormatTaskSummary(t *Task) string {
	if t == nil {
		return "N/A"
	}

	id := t.Key
	if id == "" {
		id = "N/A"
	}

	summary := t.Title
	if summary == "" {
		summary = "No summary available"
	}

	assignee := t.Assignee
	if assignee == "" {
		assignee = "Unassigned"
	}

	return fmt.Sprintf("[%-7s] %s | %s\nAssignee: %s", id, t.Status, summary, assignee)
}
