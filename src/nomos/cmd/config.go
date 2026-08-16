package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/spf13/cobra"
)

// configCmd represents the config command which allows for managing Nomos configuration.
// It serves as a parent command for specific configuration management subcommands.
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage nomos configuration",
}

// configGetCmd represents the get subcommand which retrieves specific configuration values.
// Currently it supports retrieving the agent_dir configuration.
var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		path := cfgFile
		if path == "" {
			path = filepath.Join(workspace.MustNewContext(exec.FindRepoRoot(cwd)).DataDir(), "config.yaml")
		}

		cfg, err := config.LoadConfig(path)
		if err != nil {
			cfg = &config.Config{}
		}

		key := args[0]
		switch key {
		case "agent_dir":
			fmt.Println(cfg.ResolveAgentDir(exec.FindRepoRoot(cwd)))
		default:
			return fmt.Errorf("unknown configuration key: %s", key)
		}
		return nil
	},
}

// init registers the config command and its subcommands to the root command structure.
// This function is called automatically when the package is initialized.
func init() {
	configCmd.AddCommand(configGetCmd)
	RootCmd.AddCommand(configCmd)
}
