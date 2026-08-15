// Package cmd defines the command-line interface.
package cmd

import (
	"fmt"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/env"
	"github.com/spf13/cobra"
)

/*
envLogsCmd retrieves the standard output and error logs from a running PM2 service.
This command is critical for observability, allowing both developers and agents
to quickly diagnose issues in background daemons (such as Cockpit or Llama)
without needing to context-switch into raw PM2 commands.
It streams a fixed tail of the logs back to the CLI stdout.
*/
var envLogsCmd = &cobra.Command{
	Use:   "logs [service]",
	Short: "View logs for a background service",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use the centralized helper to fetch the workspace root and the resolved PM2 service configuration.
		repoRoot, svc, err := loadEnvService(args[0])
		if err != nil {
			// Abort if the service string isn't recognized.
			return err
		}

		// Stream the logs from the PM2 daemon to the local CLI stdout.
		out, err := env.Logs(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), svc.Name)
		if err != nil {
			// Fast fail on PM2 streaming error.
			return err
		}

		// Dump the accumulated stdout and stderr.
		fmt.Println(out)
		return nil
	},
}

// init registers the command to the env domain.
func init() {
	envCmd.AddCommand(envLogsCmd)
}
