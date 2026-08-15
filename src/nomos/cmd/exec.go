package cmd

import (
	"errors"
	"fmt"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"os"
	"os/exec"
	"strconv"

	nomosexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/spf13/cobra"
)

var forceFlag bool

var runCmd = &cobra.Command{
	Use:   "run [command] [args...]",
	Short: "Run a command with active process tracking in SQLite",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		out, err := nomosexec.RunCommand(cacheDbPath, "", args[0], args[1:]...)
		fmt.Print(out)
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				os.Exit(exitErr.ExitCode())
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

var lockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Manage task/workspace locks in SQLite",
}

var lockAcquireCmd = &cobra.Command{
	Use:   "acquire [key] [owner] [pid]",
	Short: "Acquire an exclusive lock",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		owner := args[1]
		pid, err := strconv.Atoi(args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid pid: %v\n", err)
			os.Exit(1)
		}

		ok, err := nomosexec.AcquireLock(cacheDbPath, key, owner, pid, forceFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "lock error: %v\n", err)
			os.Exit(1)
		}

		if ok {
			synapse.Info("%s", fmt.Sprint("locked"))
			os.Exit(0)
		} else {
			synapse.Info("%s", fmt.Sprint("collision"))
			os.Exit(1)
		}
	},
}

var lockReleaseCmd = &cobra.Command{
	Use:   "release [key] [owner]",
	Short: "Release a lock",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		owner := args[1]

		err := nomosexec.ReleaseLock(cacheDbPath, key, owner)
		if err != nil {
			fmt.Fprintf(os.Stderr, "release lock error: %v\n", err)
			os.Exit(1)
		}
		synapse.Info("%s", fmt.Sprint("released"))
	},
}

func init() {
	lockAcquireCmd.Flags().BoolVar(&forceFlag, "force", false, "force lock acquisition")
	lockCmd.AddCommand(lockAcquireCmd)
	lockCmd.AddCommand(lockReleaseCmd)
}
