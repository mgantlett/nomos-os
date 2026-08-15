// Package cmd defines the command-line interface.
package cmd

import (
	"fmt"
	"github.com/mgantlett/nomos-os/src/nomos/modules/env"
	"github.com/spf13/cobra"
)

// envCmd represents the parent 'nomos env' command domain.
// It acts as the root for all background service management commands.
var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage local environment and background daemon services",
	Long:  "env allows you to start, stop, list, and view logs of local background processes (Cockpit, Vitepress, Datasette, Llama) via PM2.",
}

// loadEnvService is a helper to deduplicate repo root and service resolution.
// It loads the global tracker, finds the repository root, and resolves the env service config.
func loadEnvService(service string) (string, *env.ServiceConfig, error) {
	_, repoRoot, err := loadTrackerAndRoot()
	if err != nil {
		return "", nil, fmt.Errorf("failed to load repo root: %w", err)
	}
	svc, err := env.ResolveService(repoRoot, service)
	if err != nil {
		return "", nil, fmt.Errorf("failed to resolve service %s: %w", service, err)
	}
	return repoRoot, svc, nil
}

// init registers the env domain command.
func init() {
	RootCmd.AddCommand(envCmd)
}
