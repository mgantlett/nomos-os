package task

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
)

type mockLifecycleTracker struct {
	Tracker
	comments []string
}

func (m *mockLifecycleTracker) Comment(ctx context.Context, key string, comment string) error {
	m.comments = append(m.comments, comment)
	return nil
}

func TestPostPhaseComment(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nomos-lifecycle-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set up config
	agentPluginsDir := filepath.Join(tempDir, ".nomos_test_state")
	if err := os.MkdirAll(agentPluginsDir, 0755); err != nil {
		t.Fatalf("failed to create agent plugins dir: %v", err)
	}
	configEnvContent := "export NOMOS_DEFAULT_TASK_TRACKER=local\n"
	if err := os.WriteFile(filepath.Join(agentPluginsDir, "config.env"), []byte(configEnvContent), 0644); err != nil {
		t.Fatalf("failed to write config.env: %v", err)
	}

	// Setup mock tracker override
	mock := &mockLifecycleTracker{}
	NewTrackerOverride = func(cfg *Config) (Tracker, error) {
		return mock, nil
	}
	defer func() { NewTrackerOverride = nil }()

	// Trigger PostPhaseComment
	PostPhaseComment(tempDir, "JAZZ-123", statepkg.PhasePlan)
	PostPhaseComment(tempDir, "JAZZ-123", statepkg.PhaseEdit)
	PostPhaseComment(tempDir, "JAZZ-123", statepkg.PhaseReview)

	if len(mock.comments) != 3 {
		t.Fatalf("expected 3 comments to be posted, got: %d", len(mock.comments))
	}

	if !strings.Contains(mock.comments[0], "PLAN") || !strings.Contains(mock.comments[0], "JAZZ-123") {
		t.Errorf("expected first comment to contain PLAN and task key, got: %s", mock.comments[0])
	}
	if !strings.Contains(mock.comments[1], "EDIT") {
		t.Errorf("expected second comment to contain EDIT, got: %s", mock.comments[1])
	}
	if !strings.Contains(mock.comments[2], "REVIEW") {
		t.Errorf("expected third comment to contain REVIEW, got: %s", mock.comments[2])
	}
}

func TestPostDoDFailure(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nomos-lifecycle-fail-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set up config env
	agentPluginsDir := filepath.Join(tempDir, ".nomos_test_state")
	if err := os.MkdirAll(agentPluginsDir, 0755); err != nil {
		t.Fatalf("failed to create agent plugins dir: %v", err)
	}
	configEnvContent := "export NOMOS_DEFAULT_TASK_TRACKER=local\n"
	if err := os.WriteFile(filepath.Join(agentPluginsDir, "config.env"), []byte(configEnvContent), 0644); err != nil {
		t.Fatalf("failed to write config.env: %v", err)
	}

	// Setup mock tracker override
	mock := &mockLifecycleTracker{}
	NewTrackerOverride = func(cfg *Config) (Tracker, error) {
		return mock, nil
	}
	defer func() { NewTrackerOverride = nil }()

	// Trigger PostDoDFailure
	PostDoDFailure(tempDir, "JAZZ-123", []string{"Go Tests: test failed", "TDD Coverage: missing hello_test.go"})

	if len(mock.comments) != 1 {
		t.Fatalf("expected 1 comment to be posted, got: %d", len(mock.comments))
	}

	expectedParts := []string{"JAZZ-123", "failed", "Go Tests: test failed", "TDD Coverage: missing hello_test.go"}
	for _, part := range expectedParts {
		if !strings.Contains(mock.comments[0], part) {
			t.Errorf("expected comment to contain %q, got: %s", part, mock.comments[0])
		}
	}
}
