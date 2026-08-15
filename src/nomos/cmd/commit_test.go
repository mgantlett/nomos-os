package cmd

import (
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"strings"
	"testing"
)

func TestParseCommitMessage(t *testing.T) {
	validMsg := `feat(auth): do something

**Impact List:**
  - file1.go
**Resolution Details:**
  - fix
`
	// format with backticks
	validMsg = strings.ReplaceAll(validMsg, "%s", "```")

	title, err := parseCommitMessage(validMsg)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if title != "feat(auth): do something" {
		t.Fatalf("expected title 'feat(auth): do something', got '%s'", title)
	}

	invalidMsg := `feat(auth): do something

no text headings here
`
	_, err = parseCommitMessage(invalidMsg)
	if err == nil || !strings.Contains(err.Error(), "**Impact List:**") {
		t.Fatalf("expected missing text headings error, got %v", err)
	}

}

func TestValidateCommitMessage(t *testing.T) {
	validMsg := `feat(auth): do something

**Impact List:**
  - file1.go
`
	validMsg = strings.ReplaceAll(validMsg, "%s", "```")

	state := &task.PhaseState{TaskId: "123"}
	res, err := validateCommitMessage(".", validMsg, state)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !strings.HasPrefix(res, "[Task 123] feat(auth): do something") {
		t.Fatalf("expected prepended title, got '%s'", res)
	}
}
