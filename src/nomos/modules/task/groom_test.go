package task

import (
	"context"
	"testing"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

func TestExtractSemanticTokens(t *testing.T) {
	task1 := Task{
		Key:         "100",
		Title:       "Fix bug in src/nomos/cmd/swarm.go",
		Description: "The swarm orchestrator crashes.",
		Labels:      []string{"layer:cmd", "priority:high"},
	}

	tokens := extractSemanticTokens(task1)
	if !tokens["src/nomos/cmd/swarm.go"] {
		t.Errorf("Expected token src/nomos/cmd/swarm.go to be found")
	}
	if !tokens["layer:cmd"] {
		t.Errorf("Expected label token layer:cmd to be found")
	}

	task2 := Task{
		Key:         "101",
		Title:       "Another bug",
		Description: "It is in src/nomos/cmd/swarm.go",
		Labels:      []string{"layer:cmd", "priority:low"},
	}

	tokens2 := extractSemanticTokens(task2)
	if !haveOverlap(tokens, tokens2) {
		t.Errorf("Expected tasks to overlap on layer:cmd and swarm.go")
	}
}

type MockTrackerForGroom struct {
	Tracker
	tasks []Task
	edits []string
}

func (m *MockTrackerForGroom) List(ctx context.Context) ([]Task, error) {
	return m.tasks, nil
}

func (m *MockTrackerForGroom) Edit(ctx context.Context, key string, title *string, body *string, labels []string, contextBurden *int, logicDepth *int, blockedBy []string, sequence *int, project *string) error {
	m.edits = append(m.edits, key)
	return nil
}

func TestGroom_MonolithicDetection(t *testing.T) {
	mockTracker := &MockTrackerForGroom{
		tasks: []Task{
			{Key: "100", Status: StatusBacklog, Type: TypeBug, Title: "Bug 1", ContextBurden: 3, LogicDepth: 3},                                  // Monolithic (6 > 5)
			{Key: "101", Status: StatusBacklog, Type: TypeBug, Title: "Bug 2", ContextBurden: 2, LogicDepth: 2},                                  // Fine (4 <= 5)
			{Key: "102", Status: StatusBacklog, Type: TypeBug, Title: "Bug 3", ContextBurden: 4, LogicDepth: 2, Labels: []string{"needs-split"}}, // Monolithic but already tagged
		},
	}
	ctx := context.Background()
	_ = GroomBacklog(ctx, func() *workspace.WorkspaceContext { c, _ := workspace.NewContext("."); return c }(), mockTracker, 13, "", true)

	if len(mockTracker.edits) != 1 || mockTracker.edits[0] != "100" {
		t.Errorf("Expected only Task 100 to be edited for needs-split, got: %v", mockTracker.edits)
	}
}

func TestAutoBundleTasks_NoPanics(t *testing.T) {
	mockTracker := &MockTrackerForGroom{
		tasks: []Task{
			{Key: "100", Status: StatusBacklog, Type: TypeBug, Title: "Bug 1 src/test.go", ContextBurden: 1, LogicDepth: 1},
			{Key: "101", Status: StatusBacklog, Type: TypeBug, Title: "Bug 2 src/test.go", ContextBurden: 1, LogicDepth: 1},
			{Key: "102", Status: StatusBacklog, Type: TaskType("feature"), Title: "Feature src/test.go", ContextBurden: 2, LogicDepth: 2}, // different type!
		},
	}

	ctx := context.Background()
	// Just verify it doesn't crash since executeBundle will fail to run bin/nomos locally in unit tests usually,
	// but actually os/exec might just fail gracefully with "failed to bundle".
	_ = AutoBundleTasks(ctx, mockTracker, 13.0)
}

func TestGroom_detectCycles(t *testing.T) {
	tasks := []Task{
		{Key: "TASK-1", BlockedBy: []string{"TASK-2"}},
		{Key: "TASK-2", BlockedBy: []string{"TASK-3"}},
		{Key: "TASK-3", BlockedBy: []string{"TASK-1"}}, // Cycle
		{Key: "TASK-4", BlockedBy: []string{"TASK-5"}},
		{Key: "TASK-5", BlockedBy: []string{}},
	}

	cycles := detectCycles(tasks)
	if len(cycles) == 0 {
		t.Errorf("Expected to detect a cycle, but got none")
	}

	foundCycle := false
	for _, c := range cycles {
		if c[len(c)-1] == "TASK-1" || c[len(c)-1] == "TASK-2" || c[len(c)-1] == "TASK-3" {
			foundCycle = true
		}
	}

	if !foundCycle {
		t.Errorf("Expected cycle containing TASK-1, TASK-2, or TASK-3")
	}
}

func TestGroom_detectDuplicates(t *testing.T) {
	tasks := []Task{
		{
			Key:         "TASK-1",
			Status:      StatusBacklog,
			Title:       "Implement User Login",
			Description: "We need a user login page that connects to the authentication backend to allow users to access the platform.",
		},
		{
			Key:         "TASK-2",
			Status:      StatusBacklog,
			Title:       "User Login Page",
			Description: "Create a user login page that connects to the authentication backend. This allows users to access the platform securely.",
		},
		{
			Key:         "TASK-3",
			Status:      StatusDone,
			Title:       "Refactor Database Layer",
			Description: "Optimize the database schema and query performance for the analytics dashboard.",
		},
	}

	duplicates := detectDuplicates(tasks)
	if len(duplicates) == 0 {
		t.Fatalf("Expected to detect duplicates, but found none")
	}

	found := false
	for _, dup := range duplicates {
		if (dup[0] == "TASK-1" && dup[1] == "TASK-2") || (dup[0] == "TASK-2" && dup[1] == "TASK-1") {
			found = true
		}
	}

	if !found {
		t.Errorf("Expected to find TASK-1 and TASK-2 as duplicates")
	}
}
