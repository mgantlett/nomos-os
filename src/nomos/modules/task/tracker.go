package task

import (
	"errors"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

// Tracker interface removed per Pragmatic Procedural constraint

// NewTrackerOverride allows mocking the tracker instance in tests.
var NewTrackerOverride func(cfg *Config) (*LocalTracker, error)

// NewTracker creates a task tracker instance based on workspace config.
// It initializes the appropriate concrete implementation.
func NewTracker(cfg *Config) (*LocalTracker, error) {
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
