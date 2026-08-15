// Package cmd defines the command-line interface.
package cmd

import (
	"fmt"

	"github.com/mgantlett/nomos-os/src/nomos/modules/env"
	"github.com/spf13/cobra"
)

/*
envStartCmd is responsible for starting a PM2 service.
This command relies heavily on the `env` module which abstracts the PM2 daemon manager.
By wrapping the PM2 CLI, Nomos ensures that background processes like Vitepress,
Datasette, or local inference models run consistently across different operating systems.
The command expects exactly one argument: the service name (e.g. 'llama-coder').
*/
var envStartCmd = &cobra.Command{
	Use:   "start [service|all]",
	Short: "Start a background service",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use the centralized helper to fetch the workspace root.
		_, repoRoot, err := loadTrackerAndRoot()
		if err != nil {
			return err
		}

		if args[0] == "all" {
			fmt.Println("Starting all services...")
			for _, s := range env.GetAllServices() {
				svc, err := env.ResolveService(repoRoot, s)
				if err != nil {
					continue
				}
				if svc.Port > 0 {
					fmt.Printf("Starting %s on http://localhost:%d...\n", svc.Name, svc.Port)
				} else {
					fmt.Printf("Starting %s...\n", svc.Name)
				}
				_ = env.Start(repoRoot, svc.Name, svc.LogFile, svc.Command, svc.Cwd)
			}
			fmt.Println("Successfully started all services")
			return nil
		}

		_, svc, err := loadEnvService(args[0])
		if err != nil {
			return err
		}

		if svc.Port > 0 {
			fmt.Printf("Starting %s on http://localhost:%d...\n", svc.Name, svc.Port)
		} else {
			fmt.Printf("Starting %s...\n", svc.Name)
		}
		if err := env.Start(repoRoot, svc.Name, svc.LogFile, svc.Command, svc.Cwd); err != nil {
			return err
		}

		fmt.Printf("Successfully started %s\n", svc.Name)
		return nil
	},
}

// init registers the command to the env domain.
func init() {
	envCmd.AddCommand(envStartCmd)
}
