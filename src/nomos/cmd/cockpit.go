/*
Package cmd provides CLI subcommands for the Nomos execution engine.
The cockpit.go file implements the 'nomos cockpit' command which launches
the embedded Open Core web dashboard server.
*/
package cmd

import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"os"
	"os/exec"
	"path/filepath"

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"github.com/mgantlett/nomos-os/src/nomos/modules/cockpit"
	nomosexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/spf13/cobra"
)

// cockpitPort holds the target HTTP port number for the embedded web dashboard listener.
var cockpitPort int

// cockpitSovereignFlag launches Sovereign Enterprise Edition server.
var cockpitSovereignFlag bool

// cockpitDevFlag enables hot-reloading development mode for Cockpit UI engineers.
var cockpitDevFlag bool

// cockpitCmd defines the Cobra CLI subcommand structure for launching Cockpit.
var cockpitCmd = &cobra.Command{
	Use:   "cockpit",
	Short: "Launch the local Nomos Cockpit web dashboard",
	Long:  `Launches the Nomos Cockpit web dashboard server (Kanban Board, DoD Doctor, Live Log Viewer). Pass --sovereign (-s) for Sovereign Enterprise Edition, and --dev (-d) for hot-reloading development mode.`,
	RunE:  runCockpitCmd,
}

func runCockpitCmd(cmd *cobra.Command, args []string) error {
	wd, _ := os.Getwd()
	repoRoot := nomosexec.FindRepoRoot(wd)

	var serviceName string
	if cockpitSovereignFlag {
		if cockpitDevFlag {
			serviceName = "cockpit-sovereign-dev"
		} else {
			serviceName = "cockpit-sovereign"
		}
	} else {
		if cockpitDevFlag {
			serviceName = "cockpit-dev"
		} else {
			serviceName = "cockpit"
		}
	}

	if cockpitDevFlag {
		synapse.Info("🚀 Delegating to daemon supervisor: nomos dev %s...", serviceName)
		return executeNomosDev(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), serviceName)
	}
	synapse.Info("🚀 Delegating to daemon supervisor: nomos env start %s...", serviceName)
	return executeNomosEnvStart(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), serviceName)
}

func executeNomosDev(ctx *workspace.WorkspaceContext, service string) error {
	repoRoot := ctx.RepoRoot
	nomosBin := "bin/nomos"
	if _, err := os.Stat(filepath.Join(repoRoot, "bin/nomos")); err != nil {
		if exe, errExe := os.Executable(); errExe == nil {
			nomosBin = exe
		} else {
			nomosBin = "nomos"
		}
	}
	// Note: We use executeNomosDev to bypass pm2 so that hot-reload logs directly to stdout
	execCmd := exec.Command(nomosBin, "dev", service)
	execCmd.Dir = ctx.PrimaryWorktree
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	return execCmd.Run()
}

func executeNomosEnvStart(ctx *workspace.WorkspaceContext, service string) error {
	repoRoot := ctx.RepoRoot
	nomosBin := "bin/nomos"
	if _, err := os.Stat(filepath.Join(repoRoot, "bin/nomos")); err != nil {
		if exe, errExe := os.Executable(); errExe == nil {
			nomosBin = exe
		} else {
			nomosBin = "nomos"
		}
	}
	execCmd := exec.Command(nomosBin, "env", "start", service)
	execCmd.Dir = ctx.PrimaryWorktree
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	return execCmd.Run()
}

var cockpitDaemonCmd = &cobra.Command{
	Use:    "daemon",
	Hidden: true,
	Short:  "Runs the actual cockpit server synchronously (for PM2)",
	RunE: func(cmd *cobra.Command, args []string) error {
		wd, _ := os.Getwd()
		repoRoot := nomosexec.FindRepoRoot(wd)
		ctx, _ := workspace.NewContext(repoRoot)
		server := cockpit.NewServer(ctx, cockpitPort, nil)
		return server.Start()
	},
}

// init registers the cockpit subcommand and flag definitions under RootCmd.
func init() {
	// Register port flag definition with default 8089 listener
	cockpitCmd.Flags().IntVarP(&cockpitPort, "port", "p", 8089, "Port number to listen on for Cockpit web UI")
	// Register sovereign flag definition for launching Sovereign Enterprise Edition
	cockpitCmd.Flags().BoolVarP(&cockpitSovereignFlag, "sovereign", "s", false, "Launch Sovereign Enterprise Edition Cockpit server")
	// Register dev flag definition for launching hot-reloading development mode
	cockpitCmd.Flags().BoolVarP(&cockpitDevFlag, "dev", "d", false, "Launch in hot-reloading development mode (air + tsc -w)")
	// Add cockpit command to root Cobra command hierarchy
	RootCmd.AddCommand(cockpitCmd)
	cockpitCmd.AddCommand(cockpitDaemonCmd)
}
