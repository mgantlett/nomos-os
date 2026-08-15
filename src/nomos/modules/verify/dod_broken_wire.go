package verify

import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/telemetry"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
)

// runBrokenWireDetector sweeps for Zombie tasks and trips the Hallucination Circuit Breaker
func runBrokenWireDetector(root string) (StageResult, error) {
	cfg, err := func() (*task.Config, error) { c, _ := workspace.NewContext(root); return task.LoadConfig(c) }()
	if err != nil {
		return StageResult{Passed: false, Message: "failed to load tracker config"}, err
	}
	tracker, err := task.NewTracker(cfg)
	if err != nil {
		return StageResult{Passed: false, Message: "failed to init tracker"}, err
	}

	ctx := context.Background()
	tasks, err := tracker.List(ctx)
	if err != nil {
		return StageResult{Passed: false, Message: "failed to list tasks"}, err
	}

	zombieCount := sweepZombieTasks(root, ctx, tracker, tasks)

	wireReport, wireErr := AuditWires(root)
	if wireErr == nil && wireReport != nil && !wireReport.Passed {
		if hasWireSkip(root) {
			// Bypassed Broken Wire check via **Wire-Skip:** trailer.
			// This is typically used for cross-repo dependencies like nomos-sovereign.
		} else {
			return generateBrokenWireError(wireReport.Findings)
		}
	}

	activeTask := GetActiveTaskId(root)
	if activeTask != "" {
		circuitTripped := checkCircuitBreaker(root, activeTask)
		if circuitTripped {
			return StageResult{
				Passed:  false,
				Message: "CIRCUIT_BREAKER_TRIPPED: Active agent has hallucinated 5 consecutive DoD failures in the last 15 mins. Halting execution.",
			}, fmt.Errorf("hallucination circuit breaker tripped for task %s", activeTask)
		}
	}

	msg := "No broken wires detected"
	if zombieCount > 0 {
		msg = fmt.Sprintf("Recovered %d zombie tasks back to backlog", zombieCount)
	}

	return StageResult{Passed: true, Message: msg}, nil
}

// generateBrokenWireError formats the findings into a StageResult error.
// It includes a guidance warning for cross-repo dependencies.
func generateBrokenWireError(findings []wireFinding) (StageResult, error) {
	findingMsgs := make([]string, 0, len(findings))
	for _, f := range findings {
		findingMsgs = append(findingMsgs, fmt.Sprintf("[%s] %s: %s", f.Type, f.File, f.Description))
	}
	return StageResult{
		Passed:  false,
		Message: fmt.Sprintf("UNWIRED_CODE_DETECTED: Found %d unwired item(s):\n - %s\n\n    💡 Guidance: If this code is a cross-repo dependency (e.g., used by nomos-sovereign), DO NOT DELETE IT. Instead, use the '**Wire-Skip:** <Reason>' trailer in your commit message to bypass this check.", len(findings), strings.Join(findingMsgs, "\n - ")),
	}, fmt.Errorf("unwired code detected: %d finding(s)", len(findings))
}

// sweepZombieTasks iterates over tasks and transitions stale IN_PROGRESS tasks to BACKLOG.
func sweepZombieTasks(root string, ctx context.Context, tracker task.Tracker, tasks []task.Task) int {
	zombieCount := 0
	now := time.Now()
	for _, t := range tasks {
		if t.Status == task.StatusInProgress {
			// 12 hour threshold
			if now.Sub(t.UpdatedAt).Hours() > 12 {
				// Update status via Transition
				err := tracker.Transition(ctx, t.Key, task.StatusBacklog)
				if err == nil {
					// Add label via Edit
					newLabels := append(t.Labels, "sys_alert:broken_wire")
					_ = tracker.Edit(ctx, t.Key, nil, nil, newLabels, nil, nil, nil, nil, nil)

					zombieCount++
					_ = telemetry.EmitEventWithMetadata(root, telemetry.EventBrokenWireZombieReset, "Zombie task reset to backlog", map[string]interface{}{
						"task_id": t.Key,
					})
				}
			}
		}
	}
	return zombieCount
}

// checkCircuitBreaker scans the telemetry log to see if the active agent is in a hallucination loop.
func checkCircuitBreaker(root, activeTask string) bool {
	logPath := filepath.Join(config.LogsDir(root), "nomos.jsonl")
	f, err := os.Open(logPath)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var events []telemetry.LogEvent
	for scanner.Scan() {
		var event telemetry.LogEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err == nil {
			events = append(events, event)
		}
	}

	return evaluateFailures(events, activeTask, time.Now().Add(-15*time.Minute))
}

// evaluateFailures counts recent DoD failures from the event list.
func evaluateFailures(events []telemetry.LogEvent, activeTask string, cutoff time.Time) bool {
	recentFailures := 0

	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]

		evTaskId := extractTaskID(event)
		if evTaskId != activeTask {
			continue
		}

		t, err := time.Parse(time.RFC3339, event.Timestamp)
		if err != nil || t.Before(cutoff) {
			break
		}

		if event.Level == string(telemetry.EventVerifyGateFailure) {
			recentFailures++
			if recentFailures >= 5 {
				return true
			}
		} else if strings.HasPrefix(event.Msg, "nomos verify") && event.Level == string(telemetry.EventCliInvocation) {
			break
		}
	}
	return false
}

// extractTaskID gets the task ID from the event or its metadata.
func extractTaskID(event telemetry.LogEvent) string {
	if event.TaskID != "" {
		return event.TaskID
	}
	if event.Metadata != nil {
		if tid, ok := event.Metadata["task_id"].(string); ok {
			return tid
		}
	}
	return ""
}
