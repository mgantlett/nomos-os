package task

import (
	"strings"
	"testing"
)

func TestFormatTaskSummary_AllFields(t *testing.T) {
	task := &Task{
		Key:      "PROJ-123",
		Status:   "In Progress",
		Title:    "Add login",
		Assignee: "alice",
	}

	got := FormatTaskSummary(task)
	expected := "PROJ-123"
	status := "In Progress"
	summary := "Add login"
	assignee := "alice"

	if !strings.Contains(got, expected) {
		t.Errorf("expected output to contain ID %q, got: %s", expected, got)
	}
	if !strings.Contains(got, status) {
		t.Errorf("expected output to contain Status %q, got: %s", status, got)
	}
	if !strings.Contains(got, summary) {
		t.Errorf("expected output to contain Summary %q, got: %s", summary, got)
	}
	if !strings.Contains(got, assignee) {
		t.Errorf("expected output to contain Assignee %q, got: %s", assignee, got)
	}
}

func TestFormatTaskSummary_EmptyKey(t *testing.T) {
	task := &Task{
		Key:      "",
		Status:   "Done",
		Title:    "Fixed bug",
		Assignee: "bob",
	}

	got := FormatTaskSummary(task)

	if !strings.Contains(got, "N/A") {
		t.Errorf("expected output to contain fallback ID 'N/A', got: %s", got)
	}
}

func TestFormatTaskSummary_EmptySummary(t *testing.T) {
	task := &Task{
		Key:      "PROJ-456",
		Status:   "Todo",
		Title:    "",
		Assignee: "carol",
	}

	got := FormatTaskSummary(task)

	if !strings.Contains(got, "No summary available") {
		t.Errorf("expected output to contain fallback summary 'No summary available', got: %s", got)
	}
}

func TestFormatTaskSummary_EmptyAssignee(t *testing.T) {
	task := &Task{
		Key:      "PROJ-789",
		Status:   "In Review",
		Title:    "Refactor API",
		Assignee: "",
	}

	got := FormatTaskSummary(task)

	if !strings.Contains(got, "Unassigned") {
		t.Errorf("expected output to contain fallback assignee 'Unassigned', got: %s", got)
	}
}

func TestFormatTaskSummary_NilTask(t *testing.T) {
	got := FormatTaskSummary(nil)

	if got != "N/A" {
		t.Errorf("expected 'N/A' for nil task, got: %s", got)
	}
}
