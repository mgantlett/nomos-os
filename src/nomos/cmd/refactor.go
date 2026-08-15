package cmd

import (
	"os"

	"github.com/mgantlett/nomos-os/src/nomos/modules/verify"
	"github.com/spf13/cobra"
)

var (
	refactorAll bool
	refactorCmd = &cobra.Command{
		Use:   "refactor",
		Short: "Run duplication and monolithic file length refactoring checks",
		Long:  `Run Go-native duplication sliding-window and monolithic line count boundary checks on code files.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			repoRoot := findRepoRoot(root)
			if err := enforceWorktreeZone(repoRoot, "refactor"); err != nil {
				return err
			}
			err = verify.RunRefactorChecks(root, refactorAll)
			if err != nil {
				// Return the error without showing usage since it's a validation failure.
				cmd.SilenceUsage = true
				return err
			}
			return nil
		},
	}
)

func init() {
	refactorCmd.Flags().BoolVarP(&refactorAll, "all", "a", false, "Scan all files instead of staged files")
}
