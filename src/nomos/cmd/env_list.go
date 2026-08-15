// Package cmd defines the command-line interface.
package cmd

import (
	"fmt"

	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-os/src/nomos/modules/env"
	"github.com/spf13/cobra"
)

// envListJsonFlag dictates if telemetry should be outputted as JSON.
var envListJsonFlag bool

// envListCmd represents the command to list PM2 services.
var envListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all background services and telemetry",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, repoRoot, err := loadTrackerAndRoot()
		if err != nil {
			return fmt.Errorf("failed to load repo root: %w", err)
		}

		out, err := env.List(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), envListJsonFlag)
		if err != nil {
			return err
		}

		// If the user hasn't requested JSON output, we print a formatted table.
		// Before printing the PM2 table, we also extract and print the configured
		// ports for each background service so the user knows where to connect.
		if !envListJsonFlag {
			serviceNames := env.GetAllServices()
			fmt.Println("Background Service Ports:")
			for _, name := range serviceNames {
				svc, err := env.ResolveService(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), name)
				if err != nil {
					continue
				}

				port := "N/A"

				// Extract port from command string if available
				// We search for standard flag signatures like "--port " or "-p "
				// and slice the resulting string to find the first argument token.
				// This allows the port list to dynamically reflect the configuration
				// defined in modules/env/services.go without hardcoding ports here.
				if idx := strings.Index(svc.Command, "--port "); idx != -1 {
					portFields := strings.Fields(svc.Command[idx+7:])
					if len(portFields) > 0 {
						port = portFields[0]
					}
				} else if idx := strings.Index(svc.Command, "-p "); idx != -1 {
					portFields := strings.Fields(svc.Command[idx+3:])
					if len(portFields) > 0 {
						port = strings.Trim(portFields[0], "\"'")
					}
				}

				if port != "N/A" {
					fmt.Printf("  %-15s : %s\n", svc.Name, port)
				}
			}
			fmt.Println()
		}

		fmt.Println(out)
		return nil
	},
}

// init registers the command and its JSON flag to the env domain.
func init() {
	envListCmd.Flags().BoolVar(&envListJsonFlag, "json", false, "Output telemetry in JSON format (via pm2 jlist)")
	envCmd.AddCommand(envListCmd)
}
