package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-os/src/nomos/modules/env"
	"github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/spf13/cobra"
)

/*
devCmd is responsible for starting a foreground development service.
Unlike `nomos env start`, which daemonizes processes into the background using PM2,
`nomos dev` runs the service interactively in the foreground, hooking up directly
to standard input and output. It uses native tools like `air` for Go hot-reloading
or native HMR (e.g., Vite) based on the DevCommand configured for the service.
*/
var devCmd = &cobra.Command{
	Use:   "dev [service]",
	Short: "Start a service in foreground development mode",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Fetch workspace context
		_, repoRoot, err := loadTrackerAndRoot()
		if err != nil {
			return err
		}

		serviceName := args[0]
		if serviceName == "cockpit" {
			return fmt.Errorf("deprecated command: 'nomos dev cockpit' has been deprecated in favor of 'nomos cockpit --dev'. Please run 'nomos cockpit --dev' (or 'nomos cockpit -d') to launch hot-reloading development mode.")
		}

		svc, err := env.ResolveService(repoRoot, serviceName)
		if err != nil {
			return fmt.Errorf("failed to resolve service: %w", err)
		}

		if svc.DevCommand == "" {
			return fmt.Errorf("service '%s' does not support dev mode (DevCommand is not configured)", serviceName)
		}

		fmt.Printf("Starting %s in development mode...\n", svc.Name)

		// Execute DevCommand via nix-shell to correctly evaluate quotes, shell features, and Nix dependencies
		binName := "nix-shell"
		binArgs := []string{"--run", svc.DevCommand}

		// Determine current working directory and temporarily change to it if needed
		// RunCommandInteractive resolves the target command but doesn't explicitly
		// switch the caller's working directory. For tools like air or npm, we must
		// be in the correct directory.
		origDir, _ := os.Getwd()

		cwd := svc.Cwd
		if cwd == "" {
			cwd = repoRoot
		}

		// If we are already inside a directory that matches the service name (e.g. a worktree)
		// we should respect the current working directory instead of forcing the primary clone.
		if strings.Contains(filepath.Base(origDir), serviceName) {
			cwd = origDir
		}

		if err := os.Chdir(cwd); err != nil {
			return fmt.Errorf("failed to change to service directory: %w", err)
		}
		defer os.Chdir(origDir)

		dbPath := config.ResolveCacheDbPath(config.GlobalDataDir(repoRoot))

		// Execute interactive foreground command
		if err := exec.RunCommandInteractive(dbPath, cwd, binName, binArgs...); err != nil {
			return fmt.Errorf("dev execution stopped: %w", err)
		}

		return nil
	},
}
