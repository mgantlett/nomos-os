package task

import (
	"encoding/json"
	"time"
)

// TaskStatus represents the lifecycle state of a task.
type TaskStatus string

const (
	StatusTriage     TaskStatus = "TRIAGE"
	StatusBacklog    TaskStatus = "BACKLOG"
	StatusDone       TaskStatus = "DONE"
	StatusCancelled  TaskStatus = "CANCELLED"
	StatusInProgress TaskStatus = "IN_PROGRESS"
	StatusParked     TaskStatus = "PARKED"
)

// TaskType represents the category of the task.
type TaskType string

const (
	TypeBatch TaskType = "Batch"
	TypeTask  TaskType = "Task"
	TypeBug   TaskType = "Bug"
	TypeDebt  TaskType = "Debt" // Quality debt and refactoring tasks
)

const (
	NoParentKey = ""
	Unassigned  = ""
)

// TaskComment represents a timestamped comment with author attribution.
type TaskComment struct {
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
	Body      string    `json:"body"`
}

// TaskTransition represents a state change event for velocity tracking.
type TaskTransition struct {
	Timestamp time.Time  `json:"timestamp"`
	OldStatus TaskStatus `json:"old_status"`
	NewStatus TaskStatus `json:"new_status"`
	Author    string     `json:"author"`
}

// Task represents a backlog item or sprint task.
type Task struct {
	// Identity
	Key       string     `json:"key"`
	Project   string     `json:"project,omitempty"`    // Owning project (e.g. nomos-commons)
	ParentKey string     `json:"parent_key,omitempty"` // Hierarchical parent link (e.g. Epic or Story key)
	Type      TaskType   `json:"type,omitempty"`       // Task type (Batch, Task, Bug, Debt)
	Title     string     `json:"title"`                // Renamed from Summary
	Status    TaskStatus `json:"status"`               // TRIAGE, BACKLOG, PLAN, EDIT, REVIEW, DONE, CANCELLED
	Assignee  string     `json:"assignee,omitempty"`

	// Orchestration & AI Context
	Sequence      int      `json:"sequence"` // Explicit execution sequence (replaces seq:XX label)
	ContextBurden int      `json:"context_burden"`
	LogicDepth    int      `json:"logic_depth"`
	Labels        []string `json:"labels"` // Proper JSON array of strings

	// Dependencies (DAG)
	BlockedBy []string `json:"blocked_by"` // Array of Task Keys that must be DONE first

	// Temporal Metadata
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	ClosedAt        *time.Time `json:"closed_at,omitempty"`
	AgentCycles     int        `json:"agent_cycles,omitempty"`     // Replaces execution_velocity
	ReworkFrequency int        `json:"rework_frequency,omitempty"` // Tracking churn/reopens

	// Content
	Description string           `json:"description"`
	Comments    []TaskComment    `json:"comments,omitempty"`
	ActivityLog []TaskTransition `json:"activity_log"`

	// Scoping & Flags
	IsSpike bool `json:"is_spike"` // Denotes PoC/spike status
}

// IsClosed returns true if the task is in a terminal state (DONE or CANCELLED).
func (t *Task) IsClosed() bool {
	return t.Status == StatusDone || t.Status == StatusCancelled
}

// unmarshalLegacyComments converts a legacy JSON array of string comments
// into the modern TaskComment struct format to ensure backward compatibility
// across system updates without data loss.
func unmarshalLegacyComments(data []byte) ([]TaskComment, error) {
	var legacyComments []string
	if err := json.Unmarshal(data, &legacyComments); err != nil {
		return nil, err
	}
	var comments []TaskComment
	for _, str := range legacyComments {
		comments = append(comments, TaskComment{
			Author:    "legacy",
			CreatedAt: time.Time{},
			Body:      str,
		})
	}
	return comments, nil
}

// unmarshalCommentsField attempts to decode the comments field dynamically.
// It first attempts the modern format ([]TaskComment), and if it fails,
// it falls back to the legacy string array format via unmarshalLegacyComments.
func unmarshalCommentsField(data []byte) ([]TaskComment, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var comments []TaskComment
	if err := json.Unmarshal(data, &comments); err == nil {
		return comments, nil
	}
	return unmarshalLegacyComments(data)
}

// UnmarshalJSON provides a graceful migration path for legacy local tasks
// which previously stored Comments as []string instead of []TaskComment.
func (t *Task) UnmarshalJSON(data []byte) error {
	// Alias prevents infinite recursion when unmarshalling.
	type TaskAlias Task
	// We need to parse into a temporary struct that uses json.RawMessage for Comments
	// so we can inspect its type before decoding it.
	aux := &struct {
		Comments json.RawMessage `json:"comments,omitempty"`
		*TaskAlias
	}{
		TaskAlias: (*TaskAlias)(t),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	comments, err := unmarshalCommentsField(aux.Comments)
	if err != nil {
		return err
	}
	if comments != nil {
		t.Comments = comments
	}

	return nil
}
