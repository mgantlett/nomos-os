package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mgantlett/nomos-os/src/nomos/cmd"
	nomosexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
)

// main is the primary entrypoint for the nomos CLI.
// It performs early boot-time self-re-execution to inject the Nix environment
// if it detects that it is running outside of the intended nix-shell.
func main() {
	// Flush all asynchronous state operations before process exit
	defer task.WaitAsyncCommits()
	// Check if the current process is already wrapped in a nix-shell.
	// This environment variable is automatically set by the shell.nix file
	// or by direnv when the environment is loaded.
	if os.Getenv("IN_NIX_SHELL") == "" {
		cwd, err := os.Getwd()
		if err == nil {
			// Traverse directory tree upwards to locate the workspace root.
			// The repository root is required to find the top-level shell.nix configuration.
			repoRoot := nomosexec.FindRepoRoot(cwd)
			shellNix := filepath.Join(repoRoot, "shell.nix")

			// If a shell.nix configuration exists in the workspace root,
			// proceed with the self-re-execution bootstrapper.
			if _, err := os.Stat(shellNix); err == nil {
				// Use the substrate wrapper to safely locate the nix-shell binary.
				nixBin, err := nomosexec.LookPath("nix-shell")
				if err == nil {
					// Escape os.Args safely to pass them cleanly through bash.
					// Since nix-shell --run uses bash -c internally, we must quote the arguments
					// to prevent command injection or parsing errors.
					var parts []string
					for _, arg := range os.Args {
						parts = append(parts, "'"+strings.ReplaceAll(arg, "'", "'\\''")+"'")
					}
					runCmdStr := strings.Join(parts, " ")

					// Define the target execution array invoking nix-shell.
					argv := []string{"nix-shell", shellNix, "--run", runCmdStr}

					// Replaces the current process image completely via syscall.Exec.
					// This is crucial to preserve pseudo-terminals (PTY) and standard I/O streams
					// without needing to proxy them manually via os/exec pipes.
					_ = syscall.Exec(nixBin, argv, os.Environ())
				}
			}
		}
	}

	// Proceed with the standard Cobra command root execution.
	cmd.Execute()
}
