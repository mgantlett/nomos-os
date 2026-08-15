package cmd

import (
	"github.com/spf13/cobra"
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Interact with the GitBrain semantic memory indexes",
	Long:  `The memory command group allows saving and querying semantic memory insights from the active Workspace using the GitBrain enterprise module.`,
}

func init() {
	RootCmd.AddCommand(memoryCmd)
}
