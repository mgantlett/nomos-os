package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/gitbrain"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
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

		dbPath := config.ResolveGitBrainDbPath(repoRoot)

		// Save the insight natively into local Git Notes attached to HEAD
		err := gitbrain.WriteNote(dbPath, repoRoot, "HEAD", insight)
		if err != nil {
			return fmt.Errorf("failed to save memory to git notes: %w", err)
		}

		synapse.Info("Saved memory to local Git Notes natively\n")
		fmt.Println("✅ Local memory saved successfully!")
		return nil
	},
}

func init() {
	memoryCmd.AddCommand(memorySaveCmd)
}
