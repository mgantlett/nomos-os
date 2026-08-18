package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mgantlett/nomos-commons/src/nomos/core/plugin"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	nomosexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/spf13/cobra"
)

var memoryIndexCmd = &cobra.Command{
	Use:   "index",
	Short: "Trigger GitBrain to reindex git notes and codebase",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Triggering GitBrain reindexing...\n")

		wd, _ := os.Getwd()
		repoRoot := nomosexec.FindRepoRoot(wd)
		if repoRoot == "" {
			repoRoot = wd
		}

		plugins, err := plugin.DiscoverPlugins(repoRoot)
		if err == nil {
			for _, p := range plugins {
				if filepath.Base(p) == "nomos-plugin-gitbrain" {
					_, err := plugin.CallPlugin(p, "index", map[string]string{})
					if err != nil {
						return fmt.Errorf("failed to trigger indexing via nomos-plugin-gitbrain: %w", err)
					}
					synapse.Info("GitBrain index successfully rebuilt for %s\n", repoRoot)
					fmt.Println("✅ Reindexing complete!")
					return nil
				}
			}
		}

		return fmt.Errorf("nomos-plugin-gitbrain is required to execute indexing but was not found")
	},
}

func init() {
	memoryCmd.AddCommand(memoryIndexCmd)
}
