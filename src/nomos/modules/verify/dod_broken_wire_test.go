package verify

import (
	"testing"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/telemetry"
	"github.com/stretchr/testify/require"
)

func TestExtractTaskID(t *testing.T) {
	ev1 := telemetry.LogEvent{TaskID: "123"}
	require.Equal(t, "123", extractTaskID(ev1))

	ev2 := telemetry.LogEvent{
		Metadata: map[string]interface{}{
			"task_id": "456",
		},
	}
	require.Equal(t, "456", extractTaskID(ev2))

	ev3 := telemetry.LogEvent{}
	require.Equal(t, "", extractTaskID(ev3))
}

func TestEvaluateFailures(t *testing.T) {
	cutoff := time.Now().Add(-15 * time.Minute)
	events := []telemetry.LogEvent{
		{Timestamp: time.Now().Format(time.RFC3339), TaskID: "123", Level: string(telemetry.EventVerifyGateFailure)},
		{Timestamp: time.Now().Format(time.RFC3339), TaskID: "123", Level: string(telemetry.EventVerifyGateFailure)},
		{Timestamp: time.Now().Format(time.RFC3339), TaskID: "123", Level: string(telemetry.EventVerifyGateFailure)},
		{Timestamp: time.Now().Format(time.RFC3339), TaskID: "123", Level: string(telemetry.EventVerifyGateFailure)},
		{Timestamp: time.Now().Format(time.RFC3339), TaskID: "123", Level: string(telemetry.EventVerifyGateFailure)},
	}

	// 5 failures should trip circuit
	require.True(t, evaluateFailures(events, "123", cutoff))

	// fewer than 5 should not trip
	require.False(t, evaluateFailures(events[:4], "123", cutoff))
}
