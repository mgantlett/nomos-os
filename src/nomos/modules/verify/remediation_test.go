package verify

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mgantlett/nomos-commons/src/nomos/core/db"
	"github.com/mgantlett/nomos-commons/src/nomos/core/state"
)

func TestTriggerAutoRemediation(t *testing.T) {
	// Setup temporary workspace to mock the DB path
	tempDir := t.TempDir()
	projectName := filepath.Base(tempDir)
	stateDir := filepath.Join(tempDir, ".nomos", "data", projectName, "state")
	err := os.MkdirAll(stateDir, 0755)
	if err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}

	dbPath := filepath.Join(stateDir, "graph.db")

	failedGates := []map[string]interface{}{
		{"gate_name": "Go Format & Vet", "error": "vet failed"},
	}

	// Trigger remediation on a dummy target task
	err = TriggerAutoRemediation(tempDir, "447", failedGates)
	if err != nil {
		t.Fatalf("TriggerAutoRemediation failed: %v", err)
	}

	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.CloseAll()

	dispatcher := state.NewDAGDispatcher(database, tempDir)

	// Since 447 hasn't been added as a task node, let's add it to test the blocked logic
	err = state.AddNode(database, "447", "task", map[string]interface{}{"status": "IN_PROGRESS"})
	if err != nil {
		t.Fatalf("failed to add target node: %v", err)
	}

	unblocked, err := dispatcher.GetUnblockedTasks()
	if err != nil {
		t.Fatalf("GetUnblockedTasks failed: %v", err)
	}

	// The remedy task should be unblocked (because it has no incoming edges).
	// But 447 should NOT be unblocked, because it's blocked by remedy.
	foundRemedy := false
	found447 := false
	for _, n := range unblocked {
		if n.ID == "447" {
			found447 = true
		}
		if n.ID != "447" {
			foundRemedy = true
			if n.Properties["title"] != "Auto-Remediation: Fix DoD failures" {
				t.Errorf("expected auto-remediation title, got %v", n.Properties["title"])
			}
		}
	}

	if found447 {
		t.Errorf("expected 447 to be blocked, but it was returned as unblocked")
	}
	if !foundRemedy {
		t.Errorf("expected a remedy task to be unblocked")
	}
}
