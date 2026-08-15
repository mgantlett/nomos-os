package cmd

import (
	"fmt"
	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"os"
	"os/exec"

	"github.com/mgantlett/nomos-os/src/nomos/modules/provider"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// providerCmd acts as the root command for all model provider CLI subcommands.
var providerCmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage remote model providers (GCP Spot VMs and SSH tunnels)",
	Long:  `Allows booting, stopping, querying status, and opening SSH port-forwarding tunnels to GPU Spot VMs.`,
}

// providerStatusCmd queries and prints the instance and SSH tunnel state.
var providerStatusCmd = &cobra.Command{
	Use:   "status [provider-name]",
	Short: "Get the operational status of a provider instance and port tunnel",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get provider configuration name from arguments
		name := args[0]

		// Load the parsed configuration for this provider name
		cfg, err := resolveProviderConfig(name)
		if err != nil {
			return err
		}

		// Fetch VM instance status from GCP
		status, err := provider.StatusProvider(cacheDbPath, cfg)
		if err != nil {
			return err
		}

		// Display GCP Compute Engine VM instance status
		synapse.Info("Instance:   %s (%s)\n", cfg.Instance, status)

		// Check if local SSH forwarding port is listening
		tunnelActive := provider.CheckLocalPort(cfg.LocalPort)
		if tunnelActive {
			synapse.Info("SSH Tunnel: Active (listening on localhost:%d)\n", cfg.LocalPort)
		} else {
			synapse.Info("SSH Tunnel: Inactive\n")
		}
		return nil
	},
}

// providerStartCmd transitions a provider VM into a RUNNING state.
var providerStartCmd = &cobra.Command{
	Use:   "start [provider-name]",
	Short: "Boot the provider instance and retrieve its NAT IP",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get provider configuration name from arguments
		name := args[0]

		// Load the parsed configuration for this provider name
		cfg, err := resolveProviderConfig(name)
		if err != nil {
			return err
		}

		// Trigger the VM startup procedure
		synapse.Info("🚀 Starting provider instance %s...\n", cfg.Instance)
		ip, err := provider.StartProvider(cacheDbPath, cfg)
		if err != nil {
			return err
		}

		// Print the resulting public NAT IP address
		synapse.Info("✅ Instance is running at external NAT IP: %s\n", ip)
		return nil
	},
}

// providerStopCmd halts execution of the provider VM instance.
var providerStopCmd = &cobra.Command{
	Use:   "stop [provider-name]",
	Short: "Shut down the provider compute engine instance",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get provider configuration name from arguments
		name := args[0]

		// Load the parsed configuration for this provider name
		cfg, err := resolveProviderConfig(name)
		if err != nil {
			return err
		}

		// Trigger the VM shutdown procedure to minimize GCP billing
		synapse.Info("💤 Stopping provider instance %s...\n", cfg.Instance)
		if err := provider.StopProvider(cacheDbPath, cfg); err != nil {
			return err
		}

		// Print successful confirmation
		synapse.Info("✅ Instance stop command executed successfully.\n")
		return nil
	},
}

// providerTunnelCmd launches the blocking foreground SSH port-forwarding tunnel.
var providerTunnelCmd = &cobra.Command{
	Use:   "tunnel [provider-name]",
	Short: "Establish an SSH port tunnel to the remote provider instance",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get provider configuration name from arguments
		name := args[0]

		// Load the parsed configuration for this provider name
		cfg, err := resolveProviderConfig(name)
		if err != nil {
			return err
		}

		// Fetch the VM instance's current external IP address
		ip, err := provider.GetIP(cacheDbPath, cfg)
		if err != nil {
			return fmt.Errorf("could not get instance IP (is it running?): %w", err)
		}

		// Display status message detailing connection destination and port mappings
		synapse.Info("🔗 Opening SSH tunnel to %s@%s mapping localhost:%d -> localhost:%d...\n", cfg.SSHUser, ip, cfg.LocalPort, cfg.RemotePort)

		// Execute the tunnel creation process using active process tracker
		proc, err := provider.StartTunnel(cacheDbPath, cfg, ip)
		if err != nil {
			return err
		}

		// Block and wait for background SSH tunnel process to exit
		synapse.Info("✅ Tunnel process started in background (PID: %d). Waiting for exit...\n", proc.Pid)
		state, err := proc.Wait()
		if err != nil {
			return fmt.Errorf("tunnel wait failed: %w", err)
		}

		// Report the final termination state of the tunnel
		synapse.Info("Tunnel exited: %s\n", state.String())
		return nil
	},
}

// providerLocalCmd handles local inference daemon management.
var providerLocalCmd = &cobra.Command{
	Use:   "local",
	Short: "Manage local inference model daemons",
	Long:  `Allows booting, stopping, and querying status of local inference daemons (e.g. llama-server).`,
}

var providerLocalStartCmd = &cobra.Command{
	Use:   "start [daemon-name]",
	Short: "Start a local model daemon (e.g. coder, embed)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		daemon := args[0]
		wd, _ := os.Getwd()
		repoRoot := findRepoRoot(wd)
		synapse.Info("🚀 Starting local daemon: %s...\n", daemon)
		if err := provider.StartLocalDaemon(repoRoot, daemon); err != nil {
			return err
		}
		synapse.Info("%s", fmt.Sprint("✅ Local daemon started successfully."))
		return nil
	},
}

var providerLocalStopCmd = &cobra.Command{
	Use:   "stop [daemon-name]",
	Short: "Stop a local model daemon",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		daemon := args[0]
		synapse.Info("💤 Stopping local daemon: %s...\n", daemon)
		if err := provider.StopLocalDaemon(daemon); err != nil {
			return err
		}
		synapse.Info("%s", fmt.Sprint("✅ Local daemon stopped successfully."))
		return nil
	},
}

var providerLocalStatusCmd = &cobra.Command{
	Use:   "status [daemon-name]",
	Short: "Get the operational status of a local daemon",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		daemon := args[0]
		synapse.Info("🔍 Checking status for local daemon: %s...\n", daemon)
		execCmd := exec.Command("npx", "--prefer-offline", "pm2", "describe", "llama-server-"+daemon)
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr
		return execCmd.Run()
	},
}

// resolveProviderConfig loads the provider configuration by resolving repository root and provider name.
func resolveProviderConfig(name string) (provider.ProviderConfig, error) {
	// Query current working directory
	wd, err := os.Getwd()
	if err != nil {
		return provider.ProviderConfig{}, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Locate repository root relative to working directory
	repoRoot := findRepoRoot(wd)

	// Load provider details from models.yaml
	return LoadProviderConfig(repoRoot, name)
}

// LoadProviderConfig parses models.yaml configuration and retrieves a provider by name.
func LoadProviderConfig(repoRoot string, providerName string) (provider.ProviderConfig, error) {
	// Initialize custom viper configuration parser
	v := viper.New()
	v.SetConfigFile(config.ModelsPath(repoRoot))
	if err := v.ReadInConfig(); err != nil {
		return provider.ProviderConfig{}, fmt.Errorf("failed to read models.yaml config: %w", err)
	}

	// Unmarshal complete models.yaml configuration
	var myaml provider.ModelsYAML
	if err := v.Unmarshal(&myaml); err != nil {
		return provider.ProviderConfig{}, fmt.Errorf("failed to parse models.yaml config: %w", err)
	}

	// Fetch requested provider configuration from parsed map
	cfg, ok := myaml.Providers[providerName]
	if !ok {
		return provider.ProviderConfig{}, fmt.Errorf("provider %q not found in models.yaml", providerName)
	}

	return cfg, nil
}

// register subcommands in cobra
func init() {
	providerCmd.AddCommand(providerStartCmd)
	providerCmd.AddCommand(providerStopCmd)
	providerCmd.AddCommand(providerStatusCmd)
	providerCmd.AddCommand(providerTunnelCmd)

	providerLocalCmd.AddCommand(providerLocalStartCmd)
	providerLocalCmd.AddCommand(providerLocalStopCmd)
	providerLocalCmd.AddCommand(providerLocalStatusCmd)
	providerCmd.AddCommand(providerLocalCmd)
}
