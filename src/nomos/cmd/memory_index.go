package cmd

import (
	"fmt"
	"os"

	"github.com/mgantlett/nomos-commons/src/nomos/core/gitbrain"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	nomosexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/spf13/cobra"
)

var memoryIndexCmd = &cobra.Command{
	Use:   "index",
	Short: "Trigger GitBrain to reindex git notes and migrate local SQLite memories",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Triggering GitBrain reindexing...\n")

		wd, _ := os.Getwd()
		repoRoot := nomosexec.FindRepoRoot(wd)
		if repoRoot == "" {
			repoRoot = wd
		}

		ctx := workspace.MustNewContext(repoRoot)
		dbPath := ctx.DbPath("gitbrain.db")

		// 1. One-Time aggressive migration from SQLite fallback to Git Notes
		err := gitbrain.MigrateMemoriesToGitNotes(dbPath, repoRoot)
		if err != nil {
			return fmt.Errorf("failed to migrate memories to git notes: %w", err)
		}

		// 2. Fetch and Index all Git Notes
		err = gitbrain.IndexNotes(repoRoot, dbPath)
		if err != nil {
			return fmt.Errorf("failed to index git notes: %w", err)
		}

		// 3. Traverse and Index Codebase
		err = gitbrain.IndexCodebase(repoRoot, dbPath)
		if err != nil {
			return fmt.Errorf("failed to index codebase: %w", err)
		}

		synapse.Info("GitBrain index successfully rebuilt for %s\n", repoRoot)
		fmt.Println("✅ Reindexing complete!")

		return nil
	},
}

func init() {
	memoryCmd.AddCommand(memoryIndexCmd)
}
