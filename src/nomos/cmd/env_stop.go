// Package cmd defines the command-line interface.
package cmd

import (
	"fmt"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/env"
	"github.com/spf13/cobra"
)

/*
envStopCmd handles the graceful termination of a PM2 service.
This command delegates the heavy lifting to the `env` module,
which manages the underlying PM2 CLI interactions.
Stopping a service ensures that resources are freed up when
the agent or developer no longer requires a background daemon.
It requires exactly one argument: the service name.
*/
var envStopCmd = &cobra.Command{
	Use:   "stop [service|all]",
	Short: "Stop a background service",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, repoRoot, err := loadTrackerAndRoot()
		if err != nil {
			return err
		}

		if args[0] == "all" {
			fmt.Println("Stopping all services...")
			// PM2 natively handles "all" for stop.
			if err := env.Stop(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), "all"); err != nil {
				return err
			}
			fmt.Println("Successfully stopped all services")
			return nil
		}

		_, svc, err := loadEnvService(args[0])
		if err != nil {
			return err
		}

		fmt.Printf("Stopping %s...\n", svc.Name)
		if err := env.Stop(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), svc.Name); err != nil {
			return err
		}

		fmt.Printf("Successfully stopped %s\n", svc.Name)
		return nil
	},
}

// init registers the command to the env domain.
func init() {
	envCmd.AddCommand(envStopCmd)
}
