package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/db"
	"github.com/mgantlett/nomos-commons/src/nomos/core/plugin"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	nomosexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/spf13/cobra"
)

var memorySaveCmd = &cobra.Command{
	Use:   "save [insight...]",
	Short: "Manually save a semantic insight into GitBrain",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		insight := strings.Join(args, " ")
		fmt.Printf("Saving memory to GitBrain: %s\n", insight)

		wd, _ := os.Getwd()
		repoRoot := nomosexec.FindRepoRoot(wd)
		if repoRoot == "" {
			repoRoot = wd
		}

		// 1. Try to use enterprise GitBrain for semantic search via Plugin architecture
		plugins, err := plugin.DiscoverPlugins(repoRoot)
		if err == nil {
			for _, p := range plugins {
				if filepath.Base(p) == "nomos-plugin-gitbrain" {
					_, err := plugin.CallPlugin(p, "save", map[string]string{
						"insight": insight,
					})
					if err != nil {
						return fmt.Errorf("failed to save memory via nomos-plugin-gitbrain: %w", err)
					}
					fmt.Println("✅ Memory saved successfully via Enterprise GitBrain!")
					return nil
				}
			}
		}

		// 2. Open Core Fallback: Save to local single-repo GitBrain SQLite database

		dbPath := workspace.MustNewContext(repoRoot).DbPath("gitbrain.db")
		if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
			return fmt.Errorf("failed to create memory db directory: %w", err)
		}

		if err := db.SaveLocalMemory(dbPath, insight); err != nil {
			return fmt.Errorf("failed to save local memory: %w", err)
		}

		synapse.Info("Saved memory to local GitBrain database (%s)\n", dbPath)
		fmt.Println("✅ Local memory saved successfully!")
		return nil
	},
}

func init() {
	memoryCmd.AddCommand(memorySaveCmd)
}
