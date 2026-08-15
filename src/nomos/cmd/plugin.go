package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"

	"github.com/mgantlett/nomos-commons/src/nomos/core/plugin"
	"github.com/spf13/cobra"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage and call Nomos plugins",
}

var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "Discovers and lists all executable Nomos plugins",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current working directory: %w", err)
		}
		plugins, err := plugin.DiscoverPlugins(cwd)
		if err != nil {
			return err
		}
		for _, p := range plugins {
			synapse.Info("%s", fmt.Sprint(p))
		}
		return nil
	},
}

var pluginCallCmd = &cobra.Command{
	Use:   "call [plugin-name] [method] [json-params]",
	Short: "Execute a target plugin and call the specified method with JSON parameters",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		pluginName := args[0]
		method := args[1]
		paramsRaw := args[2]

		// Resolve the plugin path
		var pluginPath string
		if strings.Contains(pluginName, string(os.PathSeparator)) {
			absPath, err := filepath.Abs(pluginName)
			if err != nil {
				return err
			}
			pluginPath = absPath
		} else {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current working directory: %w", err)
			}
			plugins, err := plugin.DiscoverPlugins(cwd)
			if err != nil {
				return err
			}
			for _, p := range plugins {
				if filepath.Base(p) == pluginName {
					pluginPath = p
					break
				}
			}
		}

		if pluginPath == "" {
			return fmt.Errorf("plugin %q not found", pluginName)
		}

		// Verify plugin path exists and is a file
		info, err := os.Stat(pluginPath)
		if err != nil {
			return fmt.Errorf("plugin path %q does not exist: %w", pluginPath, err)
		}
		if info.IsDir() {
			return fmt.Errorf("plugin path %q is a directory", pluginPath)
		}

		var params interface{}
		if err := json.Unmarshal([]byte(paramsRaw), &params); err != nil {
			return fmt.Errorf("invalid json-params: %w", err)
		}

		result, err := plugin.CallPlugin(pluginPath, method, params)
		if err != nil {
			return err
		}

		synapse.Info("%s", fmt.Sprint(string(result)))
		return nil
	},
}

func init() {
	pluginCmd.AddCommand(pluginListCmd)
	pluginCmd.AddCommand(pluginCallCmd)
}
