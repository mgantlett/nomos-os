package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/spf13/cobra"
)

// Package cmd contains the CLI commands for the Nomos application.
// This file implements the 'explore' command, which leverages Datasette
// to visually inspect the internal state databases of the workspace.

var exploreCmd = &cobra.Command{
	Use:   "explore",
	Short: "Launch Datasette to explore Nomos state databases",
	RunE: func(cmd *cobra.Command, args []string) error {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		repoRoot := findRepoRoot(wd)
		if repoRoot == "" {
			return fmt.Errorf("must be run inside a git repository")
		}

		dbPath := workspace.MustNewContext(repoRoot).DataDir()

		fmt.Printf("Launching Datasette to explore %s...\n", dbPath)
		fmt.Println("Press Ctrl+C to exit.")

		datasetteCmd := exec.Command("datasette", dbPath)
		datasetteCmd.Stdout = os.Stdout
		datasetteCmd.Stderr = os.Stderr
		datasetteCmd.Stdin = os.Stdin

		if err := datasetteCmd.Run(); err != nil {
			return fmt.Errorf("failed to run datasette: %w", err)
		}

		return nil
	},
}
