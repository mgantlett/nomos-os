package cmd

import (
	"github.com/spf13/cobra"
)

// swarmCmd is the parent command for Swarm Tier 2 orchestration
var swarmCmd = &cobra.Command{
	Use:   "swarm",
	Short: "Orchestrate autonomous Tier 2 Swarm workers",
	Long:  `The deterministic OS layer for Swarm Tier 2 scheduling. Acts as a procedural interface to delegate and monitor proprietary Swarm workers.`,
}

// init registers the swarm command tree
func init() {
	RootCmd.AddCommand(swarmCmd)
}
