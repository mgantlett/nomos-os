package task

import (
	"context"
	"errors"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

// Tracker defines operations for interacting with ticketing engines.
type Tracker interface {
	List(ctx context.Context) ([]Task, error)
	ListAll(ctx context.Context) ([]Task, error)
	View(ctx context.Context, key string) (*Task, error)
	Start(ctx context.Context, key string, assignee string) error
	Close(ctx context.Context, key string, comment string) error
	Cancel(ctx context.Context, key string, comment string) error
	ResetBackend(ctx context.Context, key string) error
	Comment(ctx context.Context, key string, comment string) error
	Transition(ctx context.Context, key string, status TaskStatus) error
	Create(ctx context.Context, title string, body string, labels []string, parentKey string, project string, taskType TaskType, isSpike bool, initialStatus TaskStatus) (string, error)
	Edit(ctx context.Context, key string, title *string, body *string, labels []string, contextBurden *int, logicDepth *int, blockedBy []string, sequence *int, project *string) error
}

// NewTrackerOverride allows mocking the tracker instance in tests.
var NewTrackerOverride func(cfg *Config) (Tracker, error)

// NewTracker creates a task tracker instance based on workspace config.
// It matches the tracker type to initialize the
// appropriate concrete implementation satisfying the task.Tracker interface.
func NewTracker(cfg *Config) (Tracker, error) {
	if NewTrackerOverride != nil {
		return NewTrackerOverride(cfg)
	}
	switch cfg.TrackerType {
	case "local":
		ctx, _ := workspace.NewContext(cfg.RepoRoot)
		return NewLocalTracker(ctx), nil
	default:
		return nil, errors.New("unsupported task tracker type: " + cfg.TrackerType)
	}
}
