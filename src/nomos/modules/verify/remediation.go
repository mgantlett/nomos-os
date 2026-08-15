package verify

import (
	"crypto/rand"
	"fmt"
	"path/filepath"

	"github.com/mgantlett/nomos-commons/src/nomos/core/db"
	"github.com/mgantlett/nomos-commons/src/nomos/core/state"
)

// TriggerAutoRemediation injects a remediation task into the DAG when DoD verification fails.
// It creates a new task node and a 'blocks' edge to the target task.
// This autonomous orchestration guarantees that any failing logic is instantly transformed
// into a blocker task within the topological graph, forcing the Tier 1 dispatcher to provision
// a remediation cycle before the feature can be promoted.
func TriggerAutoRemediation(root string, targetTaskID string, failedGates []map[string]interface{}) error {
	if targetTaskID == "" {
		return fmt.Errorf("target task ID cannot be empty")
	}

	projectName := filepath.Base(root)
	dbPath := filepath.Join(root, ".nomos", "data", projectName, "state", "graph.db")

	database, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database for auto-remediation: %w", err)
	}
	defer db.CloseAll() // Close all connections opened by db.Open

	// Ensure the schema is ready
	err = state.InitializeDAGSchema(database)
	if err != nil {
		return fmt.Errorf("failed to initialize DAG schema: %w", err)
	}

	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Errorf("failed to generate UUID for remediation task: %w", err)
	}
	remedyID := fmt.Sprintf("remedy-%x", b)

	props := map[string]interface{}{
		"status":      "TODO",
		"title":       "Auto-Remediation: Fix DoD failures",
		"description": "This task was automatically provisioned by the autonomous dispatcher.",
		"failedGates": failedGates,
	}

	if err := state.AddNode(database, remedyID, "task", props); err != nil {
		return fmt.Errorf("failed to add remediation node: %w", err)
	}

	if err := state.AddEdge(database, remedyID, targetTaskID, "blocks"); err != nil {
		return fmt.Errorf("failed to link remediation edge: %w", err)
	}

	fmt.Printf("\n  \x1b[1;36m🔧 Triggered Auto-Remediation: DAG Task %s now blocks %s\x1b[0m\n", remedyID, targetTaskID)
	return nil
}
