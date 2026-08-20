// Package schema manages structured representations of the workspace state.
// The TaskSchema dictates the markdown format for all backlog tasks.
// By centralizing this template, we eliminate fragile regex and manual string formatting,
// ensuring strict, deterministic serialization of tasks.
// Note: The TaskSchema separates the Definition of Done into discrete, testable components.
// The Acceptance Criteria must be mapped 1:1 with Walkthrough validation steps.
// The Rigor & Verification Boundary explicitly forbids skipping quality gates.
// This structure is parsed directly by the execution engine to generate test scaffolding.
// Any drift in this structure will break the `nomos verify` Walkthrough Parity checks.
// Agents are prohibited from modifying this template dynamically during runtime.
package schema

import (
	"fmt"
	"regexp"
	"strings"
)

// TaskSchema represents the structural schema of a Backlog Task
type TaskSchema struct {
	Description        string
	AcceptanceCriteria []string
	TechnicalNotes     []string
	TargetFiles        []string
	QualityDebt        []string
}

// Validate checks if the parsed task contains all mandatory information
func (s *TaskSchema) Validate(schemaType string) error {
	var missing []string
	if strings.TrimSpace(s.Description) == "" {
		missing = append(missing, "Execution Unit / Description")
	}
	if len(s.AcceptanceCriteria) == 0 {
		missing = append(missing, "Acceptance Criteria")
	}
	if len(s.TechnicalNotes) == 0 {
		missing = append(missing, "Technical Notes")
	}

	if schemaType == "code" || schemaType == "" {
		if len(s.TargetFiles) == 0 && len(s.QualityDebt) == 0 {
			missing = append(missing, "Rigor & Verification Boundary")
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("task creation rejected: markdown file is missing required populated sections:\n- %s\n\nEnsure your task file conforms to the Nomos schema.", strings.Join(missing, "\n- "))
	}
	return nil
}

// writeDescription appends the Description field to the provided string builder.
// If the Description is empty, it writes a placeholder to maintain the schema format.
func writeDescription(sb *strings.Builder, s *TaskSchema) {
	sb.WriteString("## 📝 Execution Unit / Description\n")
	if s.Description != "" {
		sb.WriteString(s.Description + "\n\n")
	} else {
		sb.WriteString("- \n\n")
	}
}

// writeAcceptanceCriteria iterates through the AcceptanceCriteria slice
// and appends each item as a markdown checklist task.
func writeAcceptanceCriteria(sb *strings.Builder, s *TaskSchema) {
	sb.WriteString("## ✅ Acceptance Criteria\n")
	if len(s.AcceptanceCriteria) > 0 {
		for _, ac := range s.AcceptanceCriteria {
			if !strings.HasPrefix(ac, "- [ ]") && !strings.HasPrefix(ac, "- [x]") {
				sb.WriteString(fmt.Sprintf("- [ ] %s\n", ac))
			} else {
				sb.WriteString(fmt.Sprintf("%s\n", ac))
			}
		}
	} else {
		sb.WriteString("- [ ] \n")
	}
	sb.WriteString("\n")
}

// writeTechnicalNotes appends technical implementation details or constraints.
// It ensures there is at least a placeholder if none are provided.
func writeTechnicalNotes(sb *strings.Builder, s *TaskSchema) {
	sb.WriteString("## 🛠️ Technical Notes\n")
	if len(s.TechnicalNotes) > 0 {
		for _, note := range s.TechnicalNotes {
			sb.WriteString(fmt.Sprintf("- %s\n", note))
		}
	} else {
		sb.WriteString("- *Note any architectural constraints, data migrations, or security boundary shifts here.*\n")
	}
	sb.WriteString("\n")
}

// writeRigorBoundary is uniquely used for code-centric tasks.
// It explicitly scopes the impacted files and defines quality debt overrides.
func writeRigorBoundary(sb *strings.Builder, s *TaskSchema) {
	sb.WriteString("## 🛡️ Rigor & Verification Boundary\n")
	sb.WriteString("- **Target Files:**\n")
	if len(s.TargetFiles) > 0 {
		for _, file := range s.TargetFiles {
			if strings.HasPrefix(file, "`") || strings.HasPrefix(file, "[") {
				sb.WriteString(fmt.Sprintf("  - %s\n", file))
			} else {
				sb.WriteString(fmt.Sprintf("  - `%s`\n", file))
			}
		}
	} else {
		sb.WriteString("  - `[NEW|MODIFY|DELETE] path/to/file`\n")
	}

	sb.WriteString("- **Quality Debt Exemptions:**\n")
	if len(s.QualityDebt) > 0 {
		for _, q := range s.QualityDebt {
			sb.WriteString(fmt.Sprintf("  - `%s`\n", q))
		}
	} else {
		sb.WriteString("  - `monolithic_file_limit: false`\n")
		sb.WriteString("  - `duplication_limit: false`\n")
	}
}

// GenerateMarkdown converts the struct into a unified Markdown string
func (s *TaskSchema) GenerateMarkdown(schemaType string) string {
	var sb strings.Builder
	writeDescription(&sb, s)
	writeAcceptanceCriteria(&sb, s)
	writeTechnicalNotes(&sb, s)

	if schemaType == "code" || schemaType == "" {
		writeRigorBoundary(&sb, s)
	}
	return sb.String()
}

// parseDescriptionLine extracts the execution unit description from the parsed markdown body.
func parseDescriptionLine(s *TaskSchema, line, trim string) {
	if trim != "-" {
		s.Description += line + "\n"
	}
}

// parseAcceptanceCriteriaLine extracts an individual markdown checkbox line into the slice.
func parseAcceptanceCriteriaLine(s *TaskSchema, trim string) {
	if strings.HasPrefix(trim, "-") {
		s.AcceptanceCriteria = append(s.AcceptanceCriteria, trim)
	}
}

// parseTechnicalNotesLine extracts technical boundary notes.
func parseTechnicalNotesLine(s *TaskSchema, trim string) {
	if strings.HasPrefix(trim, "-") {
		s.TechnicalNotes = append(s.TechnicalNotes, strings.TrimPrefix(trim, "- "))
	}
}

// parseRigorBoundaryLine maps bullet points to either the Quality Debt or Target Files lists.
func parseRigorBoundaryLine(s *TaskSchema, trim string) {
	trim = strings.TrimSpace(trim)
	if !strings.HasPrefix(trim, "-") {
		return
	}
	if strings.Contains(trim, "Quality Debt") || strings.Contains(trim, "Target Files") {
		return
	}

	// Strip the bullet point and leading/trailing whitespace
	val := strings.TrimSpace(strings.TrimPrefix(trim, "-"))

	// Strip optional backticks
	val = strings.Trim(strings.TrimSpace(val), "`")

	if val == "" || strings.Contains(val, "path/to/file") {
		return
	}

	if strings.Contains(val, ":") && strings.Contains(val, "limit") {
		s.QualityDebt = append(s.QualityDebt, val)
	} else {
		s.TargetFiles = append(s.TargetFiles, val)
	}
}

var headerRegex = regexp.MustCompile(`^#+\s+(.*)$`)

// ParseTaskSchema parses a raw markdown string back into the schema structure.
func ParseTaskSchema(markdown string, schemaType string) (*TaskSchema, error) {
	s := &TaskSchema{}
	lines := strings.Split(markdown, "\n")
	var currentSection string

	for _, line := range lines {
		trim := strings.TrimSpace(line)

		matches := headerRegex.FindStringSubmatch(trim)
		if len(matches) > 1 {
			currentSection = strings.ToLower(matches[1])
			continue
		}

		if trim == "" {
			continue
		}

		switch {
		case strings.Contains(currentSection, "execution unit"), strings.Contains(currentSection, "description"):
			parseDescriptionLine(s, line, trim)
		case strings.Contains(currentSection, "acceptance criteria"):
			parseAcceptanceCriteriaLine(s, trim)
		case strings.Contains(currentSection, "technical notes"):
			parseTechnicalNotesLine(s, trim)
		case strings.Contains(currentSection, "rigor & verification"), strings.Contains(currentSection, "rigor"), strings.Contains(currentSection, "verification"):
			parseRigorBoundaryLine(s, trim)
		}
	}
	s.Description = strings.TrimSpace(s.Description)

	if err := s.Validate(schemaType); err != nil {
		return nil, err
	}

	return s, nil
}
