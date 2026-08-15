// Package cmd defines the command-line interface.
package cmd

import (
	"fmt"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-os/src/nomos/modules/env"
	"github.com/spf13/cobra"
)

/*
envBuildCmd triggers the build sequence for a supported target.
By moving builds into the env module, users do not need to memorize
different compiler commands for different services (go build, npm run, etc).
*/
var envBuildCmd = &cobra.Command{
	Use:   "build [service]",
	Short: "Build a service or binary",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Fetch the workspace root and the resolved service configuration.
		repoRoot, svc, err := loadEnvService(args[0])
		if err != nil {
			return err
		}

		fmt.Printf("Building %s...\n", svc.Name)

		// Run the synchronous build command.
		if err := env.Build(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), svc); err != nil {
			return err
		}

		fmt.Printf("Successfully built %s\n", svc.Name)
		return nil
	},
}

// init registers the command to the env domain.
func init() {
	envCmd.AddCommand(envBuildCmd)
}
