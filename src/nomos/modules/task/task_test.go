package task

import (
	"testing"
)

func TestNewTrackerError(t *testing.T) {
	cfg := &Config{TrackerType: "invalid"}
	_, err := NewTracker(cfg)
	if err == nil {
		t.Errorf("expected error for invalid tracker type, got nil")
	}
}
