// Package cmd provides the command-line interface for Nomos.
// The schema command is responsible for managing the local task schemas.
// It exposes functionalities for both validation and generation of markdown-based
// schema structures, ensuring tasks adhere strictly to the Definition of Done.
package cmd

import (
	"encoding/json"
	"fmt"

	nomosschema "github.com/mgantlett/nomos-os/src/nomos/modules/schema"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// schemaCmd represents the base command for schema operations.
var schemaCmd = &cobra.Command{
	Use:   "schema",
	Short: "Manage and inspect Nomos markdown schemas",
}

var schemaShowCmd = &cobra.Command{
	Use:   "show [type]",
	Short: "Print the default markdown template for a given schema type (task, plan, walkthrough, commit, triage)",
	Args:  cobra.MatchAll(cobra.MaximumNArgs(1), cobra.OnlyValidArgs),
	ValidArgs: []string{"task", "operations", "plan", "walkthrough", "commit", "triage"},
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			fmt.Println("Available schemas:")
			for _, arg := range cmd.ValidArgs {
				fmt.Printf("  - %s\n", arg)
			}
			return nil
		}

		schemaType := args[0]
		switch schemaType {
		case "task":
			schema := &nomosschema.TaskSchema{}
			fmt.Print(schema.GenerateMarkdown("code"))
		case "operations":
			schema := &nomosschema.TaskSchema{}
			fmt.Print(schema.GenerateMarkdown("operations"))
		case "plan":
			schema := &nomosschema.PlanSchema{}
			fmt.Print(schema.GenerateMarkdown())
		case "walkthrough":
			schema := &nomosschema.WalkthroughSchema{}
			fmt.Print(schema.GenerateMarkdown())
		case "commit":
			schema := &nomosschema.CommitSchema{}
			fmt.Print(schema.GenerateMarkdown())
		case "triage":
			schema := &nomosschema.IncidentTriageSchema{}
			fmt.Print(schema.GenerateMarkdown())
		default:
			return fmt.Errorf("unknown schema type: %s", schemaType)
		}
		return nil
	},
}

func buildCliSchema(cmd *cobra.Command) nomosschema.CliSchema {
	cliSchema := nomosschema.CliSchema{
		Name:        cmd.Name(),
		Aliases:     cmd.Aliases,
		Subcommands: make(map[string]nomosschema.CliSchema),
	}

	// Capture both local and inherited flags
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		cliSchema.Flags = append(cliSchema.Flags, f.Name)
		if f.Shorthand != "" {
			cliSchema.Flags = append(cliSchema.Flags, f.Shorthand)
		}
	})

	for _, child := range cmd.Commands() {
		if child.Name() == "help" {
			continue
		}
		cliSchema.Subcommands[child.Name()] = buildCliSchema(child)
	}

	return cliSchema
}

var schemaCliCmd = &cobra.Command{
	Use:   "cli",
	Short: "Print the CLI schema as JSON for the SSOT Drift Engine",
	RunE: func(cmd *cobra.Command, args []string) error {
		schema := buildCliSchema(RootCmd)
		b, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	},
}

func init() {
	schemaCmd.AddCommand(schemaShowCmd)
	schemaCmd.AddCommand(schemaCliCmd)
	RootCmd.AddCommand(schemaCmd)
}
