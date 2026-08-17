// Package cmd implements the command line interface and command routing for Nomos.
// It defines all subcommands, flags, and their execution logic.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/telemetry"
	"github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"

	"github.com/mgantlett/nomos-commons/src/nomos/core/db"
)

var (
	// cfgFile stores the path to the optional custom configuration file.
	cfgFile string
	// cacheDbPath stores the path to the workspace's local sqlite cache database.
	cacheDbPath string

	// RootCmd represents the base command when called without any subcommands.
	// It acts as the primary entrypoint for the CLI router.
	RootCmd = &cobra.Command{
		Use:     "nomos",
		Short:   "Nomos is a zero-dependency static engine and local AI orchestrator",
		Long:    `Nomos is a high-performance developer workspace engine designed to orchestrate agent tasks, perform AST auditing, and maintain local definition of done gates.`,
		Version: exec.GetNomosVersion(),
	}
)

func init() {
	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (defaults to global data dir config.yaml)")

	RootCmd.PersistentPreRunE = persistentPreRun
	RootCmd.PersistentPostRun = persistentPostRun
}

// persistentPostRun is executed after the command finishes.
// It ensures that any open database connections in the workspace are cleanly closed.
func persistentPostRun(cmd *cobra.Command, args []string) {
	// Close all pooled sqlite connections.
	db.CloseAll()
}

// persistentPreRun configures the CLI state before executing any command.
// This resolves the config path, loads viper configurations, and boots SQL caches.
func persistentPreRun(cmd *cobra.Command, args []string) error {
	path := cfgFile
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
		repoRoot := exec.FindRepoRoot(cwd)
		path = filepath.Join(workspace.MustNewContext(repoRoot).DataDir(), "config.yaml")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		defaultConfig := []byte(`
env: development
db_path: cache.db
agent_dir: .agents
`)
		if err := os.WriteFile(path, defaultConfig, 0644); err != nil {
			return fmt.Errorf("failed to write default config: %w", err)
		}
	}

	_, err := config.LoadConfig(path)
	if err != nil {
		return err
	}

	cacheDbPath = workspace.MustNewContext(filepath.Dir(path)).DbPath("cache.db")
	if err := InitCacheDB(cacheDbPath); err != nil {
		return err
	}

	return nil

}

func init() {
	// Register subcommands
	RootCmd.AddCommand(initCmd)
	RootCmd.AddCommand(doctorCmd)
	RootCmd.AddCommand(devCmd)
	RootCmd.AddCommand(handshakeCmd)
	RootCmd.AddCommand(verifyCmd)
	RootCmd.AddCommand(runCmd)
	RootCmd.AddCommand(lockCmd)
	RootCmd.AddCommand(pluginCmd)
	RootCmd.AddCommand(providerCmd)
	RootCmd.AddCommand(astCmd)
	RootCmd.AddCommand(graphCmd)
	RootCmd.AddCommand(searchCmd)

	RootCmd.AddCommand(updateCmd)
	RootCmd.AddCommand(browserCmd)
	RootCmd.AddCommand(refactorCmd)
	RootCmd.AddCommand(auditCmd)
	RootCmd.AddCommand(shellCmd)
	RootCmd.AddCommand(ideCmd)
	RootCmd.AddCommand(exploreCmd)
}

// Execute runs the Cobra CLI command routing.
// It acts as the main entrypoint from the main package and blocks
// until the command completes. It initializes telemetry tracking
// and captures the start time for latency measurements.
// If an error is returned by the underlying cobra command,
// Execute prints the error to Stderr and exits with code 1.
func Execute() {
	start := time.Now()
	err := RootCmd.Execute()

	trackCLIInvocation(start, err)

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// InitCacheDB opens the database and initializes tables if they do not exist.
// This function acts as the central state management initializer for the CLI,
// configuring caching, active workspace locking, and running process registration.
func InitCacheDB(dbPath string) error {
	// Resolve base directory of the SQLite cache database
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory %s: %w", dir, err)
	}

	// Open SQL database connection to cache file
	db, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open sqlite database at %s: %w", dbPath, err)
	}

	// Enable Write-Ahead Logging (WAL) mode for performance.
	// This reduces lock contention during parallel agent actions.
	_, _ = db.Exec("PRAGMA journal_mode=WAL;")

	// Create locks table for directory coordination.
	// This prevents multiple agent loops from editing files concurrently.
	locksTableSchema := `
	CREATE TABLE IF NOT EXISTS locks (
		lock_key TEXT PRIMARY KEY,
		owner TEXT,
		pid INTEGER,
		acquired_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(locksTableSchema); err != nil {
		return fmt.Errorf("failed to create locks table: %w", err)
	}

	// Create active_processes table to track spawned subprocesses.
	// This enables cleanup (killing) of child processes if the parent crashes.
	activeProcessesSchema := `
	CREATE TABLE IF NOT EXISTS active_processes (
		pid INTEGER PRIMARY KEY,
		command TEXT,
		started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(activeProcessesSchema); err != nil {
		return fmt.Errorf("failed to create active_processes table: %w", err)
	}

	return nil
}

// getCallerType determines if the CLI was invoked by an agent or human.
func getCallerType(ctx *workspace.WorkspaceContext) string {
	repoRoot := ctx.RepoRoot
	if state, stateErr := task.GetPhaseState(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }()); stateErr == nil {
		if state.Agent != "" && state.Agent != "null" && state.Agent != "os-automaton" {
			return "agent"
		}
	}
	return "human"
}

// trackCLIInvocation logs the outcome and execution latency of the nomos command to telemetry.jsonl.
func trackCLIInvocation(start time.Time, err error) {
	if os.Getenv("NOMOS_TEST_MODE") == "1" || strings.HasSuffix(os.Args[0], ".test") {
		return
	}

	wd, getWdErr := os.Getwd()
	if getWdErr != nil {
		return
	}
	repoRoot := findRepoRoot(wd)
	callerType := getCallerType(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }())

	cmdStr := ""
	argsStr := ""
	if len(os.Args) > 0 {
		cmdStr = filepath.Base(os.Args[0])
	}
	if len(os.Args) > 1 {
		argsStr = strings.Join(os.Args[1:], " ")
	}

	durationMs := time.Since(start).Milliseconds()
	exitCode := 0
	if err != nil {
		exitCode = 1
	}

	payload := map[string]interface{}{
		"command":     cmdStr,
		"args":        argsStr,
		"duration_ms": durationMs,
		"exit_code":   exitCode,
		"caller_type": callerType,
	}

	fullCmd := cmdStr
	if argsStr != "" {
		fullCmd = cmdStr + " " + argsStr
	}

	_ = telemetry.EmitEventWithMetadata(repoRoot, "CLI_INVOCATION", fullCmd, payload)
}
