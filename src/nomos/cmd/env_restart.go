// Package cmd defines the command-line interface.
package cmd

import (
	"fmt"
	"github.com/mgantlett/nomos-os/src/nomos/modules/env"
	"github.com/spf13/cobra"
)

/*
envRestartCmd handles restarting a PM2 service.
This command delegates the heavy lifting to the `env` module,
which manages the underlying PM2 CLI interactions.
It requires exactly one argument: the service name or "all".
*/
var envRestartCmd = &cobra.Command{
	Use:   "restart [service|all]",
	Short: "Restart a background service",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		_, repoRoot, err := loadTrackerAndRoot()
		if err != nil {
			return err
		}

		if args[0] == "all" {
			fmt.Println("Restarting all services...")
			for _, s := range env.GetAllServices() {
				if svc, err := env.ResolveService(repoRoot, s); err == nil && svc.Port > 0 {
					fmt.Printf(" - %s: http://localhost:%d\n", svc.Name, svc.Port)
				}
			}
			if err := env.Restart(repoRoot, "all"); err != nil {
				return err
			}
			fmt.Println("Successfully restarted all services")
			return nil
		}

		_, svc, err := loadEnvService(args[0])
		if err != nil {
			return err
		}

		if svc.Port > 0 {
			fmt.Printf("Restarting %s on http://localhost:%d...\n", svc.Name, svc.Port)
		} else {
			fmt.Printf("Restarting %s...\n", svc.Name)
		}
		if err := env.Restart(repoRoot, svc.Name); err != nil {
			return err
		}

		fmt.Printf("Successfully restarted %s\n", svc.Name)
		return nil
	},
}

// init registers the command to the env domain.
func init() {
	envCmd.AddCommand(envRestartCmd)
}
