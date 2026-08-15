package schema

// CliSchema maps directly to the JSON structure outputted by `nomos schema cli`.
// It provides a nested tree of commands, flags, and aliases for deterministic validation.
type CliSchema struct {
	Name        string               `json:"name"`
	Aliases     []string             `json:"aliases,omitempty"`
	Flags       []string             `json:"flags,omitempty"`
	Subcommands map[string]CliSchema `json:"subcommands,omitempty"`
}
