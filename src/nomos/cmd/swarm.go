package cmd

import (
	"fmt"
	"github.com/mgantlett/nomos-os/src/nomos/modules/harness"

	"github.com/spf13/cobra"
)

var swarmCmd = &cobra.Command{
	Use:   "swarm",
	Short: "Level 2 Autonomous Pool execution (Sovereign Edition)",
	Long:  `Manage and orchestrate Swarm Tier 2 AI autonomous agents.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("Swarm autonomous delegation requires the Sovereign edition of Nomos")
	},
}

var swarmRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Autonomously pick top backlog tasks and execute via Swarm (Sovereign Edition)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("Swarm autonomous delegation requires the Sovereign edition of Nomos")
	},
}

var swarmDelegateCmd = &cobra.Command{
	Use:   "delegate [agent] <task_id>",
	Short: "Delegate task execution to a Swarm Tier 2 sub-agent (Sovereign Edition)",
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("Swarm autonomous delegation requires the Sovereign edition of Nomos")
	},
}

var swarmDriverCmd = &cobra.Command{
	Use:   "driver",
	Short: "Run the deterministic substrate harness loop",
	RunE: func(cmd *cobra.Command, args []string) error {
		d := harness.NewDriver("")
		return d.RunNomosLoop("Generate a simple hello world.")
	},
}

func init() {
	// Add flags so CLI usage still parses them gracefully if provided
	swarmRunCmd.Flags().IntP("limit", "l", 2, "Number of tasks to pick autonomously")
	swarmRunCmd.Flags().String("provider", "", "AI inference provider")
	swarmRunCmd.Flags().StringP("project", "p", "", "Project target scope")

	swarmDelegateCmd.Flags().String("provider", "", "AI inference provider")
	swarmDelegateCmd.Flags().String("phase", "PLAN", "Phase to execute (PLAN, EDIT)")

	swarmCmd.AddCommand(swarmRunCmd)
	swarmCmd.AddCommand(swarmDelegateCmd)
	swarmCmd.AddCommand(swarmDriverCmd)
	RootCmd.AddCommand(swarmCmd)
}
