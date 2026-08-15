package cmd

import (
	"os"
	"os/exec"
	"strings"

	"syscall"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	nomosexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/spf13/cobra"
)

// shellCmd represents the 'shell' command inside the CLI router.
// It wraps the execution environment in nix-shell.
// By wrapping all executions within this shell, we guarantee deterministic dependency resolution
// (via Nix).
var shellCmd = &cobra.Command{
	Use:   "shell [command]",
	Short: "Start a nix-shell environment",
	Long:  "Wraps the execution environment in nix-shell to guarantee deterministic operations.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Identify the working directory to resolve the repository root.
		// This is critical because all substrate socket paths are relative to the workspace root.
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		repoRoot := findRepoRoot(wd)

		env := nomosexec.InjectSubstrateEnvironmentPrimary(os.Environ(), func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())

		var execArgs []string
		var binPath string

		// Attempt to resolve nix-shell. If it is available on the path, we wrap the entire
		// execution in the declarative deterministic environment defined by shell.nix.
		nixPath, err := exec.LookPath("nix-shell")
		if err == nil {
			binPath = nixPath
			if len(args) > 0 {
				// We must shell-escape the arguments or join them properly for --run
				commandStr := strings.Join(args, " ")
				execArgs = []string{"nix-shell", "--run", commandStr}
			} else {
				execArgs = []string{"nix-shell"}
			}
		} else {
			// Fallback if nix-shell is not found
			if len(args) > 0 {
				binPath, err = exec.LookPath(args[0])
				if err != nil {
					return err
				}
				execArgs = append([]string{binPath}, args[1:]...)
			} else {
				binPath = os.Getenv("SHELL")
				if binPath == "" {
					binPath = "/bin/sh"
				}
				execArgs = []string{binPath}
			}
		}

		// Use syscall.Exec to replace the current nomos process with the shell
		return syscall.Exec(binPath, execArgs, env)
	},
}
