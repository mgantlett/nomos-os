package cmd

import (
	"fmt"
	"os"

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"

	nomosexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/spf13/cobra"
)

var substrateCmd = &cobra.Command{
	Use:   "substrate [lock|unlock|validate] [path]",
	Short: "Manage workspace filesystem substrate locks and target path validations",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := os.Getwd()
		if err != nil {
			return err
		}

		action := args[0]
		switch action {
		case "lock":
			if err := nomosexec.LockSubstrate(root); err != nil {
				return err
			}
			synapse.Info("%s", fmt.Sprint("🔒 Workspace substrate locked (read-only)."))
		case "unlock":
			if err := nomosexec.UnlockSubstrate(root); err != nil {
				return err
			}
			synapse.Info("%s", fmt.Sprint("🔓 Workspace substrate unlocked (writable)."))
		case "validate":
			if len(args) < 2 {
				return fmt.Errorf("validate action requires target path argument: nomos substrate validate <target_path>")
			}
			targetPath := args[1]
			if err := nomosexec.ValidateSubstrateTargetPath(root, targetPath); err != nil {
				return err
			}
			synapse.Info("%s", fmt.Sprintf("✅ Substrate target path '%s' is valid.", targetPath))
		default:
			return fmt.Errorf("invalid action: %s (must be lock, unlock, or validate)", action)
		}
		return nil
	},
}

func init() {
	RootCmd.AddCommand(substrateCmd)
}
