package schema

import (
	"strings"
	"testing"
)

func TestTaskSchema(t *testing.T) {
	schema := &TaskSchema{
		Description:        "Implement new schema",
		AcceptanceCriteria: []string{"- [ ] Do X", "Do Y"},
		TechnicalNotes:     []string{"Use native structs"},
		TargetFiles:        []string{"[MODIFY] src/nomos/task/schema.go"},
		QualityDebt:        []string{"monolithic_file_limit: false"},
	}

	markdown := schema.GenerateMarkdown("code")
	if !strings.Contains(markdown, "Implement new schema") {
		t.Errorf("Expected description in markdown")
	}

	parsed, err := ParseTaskSchema(markdown, "code")
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	if parsed.Description != "Implement new schema" {
		t.Errorf("Expected 'Implement new schema', got %q", parsed.Description)
	}

	if len(parsed.TargetFiles) != 1 || parsed.TargetFiles[0] != "[MODIFY] src/nomos/task/schema.go" {
		t.Errorf("Expected target files to match: %v", parsed.TargetFiles)
	}
}
