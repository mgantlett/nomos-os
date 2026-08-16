package exec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/telemetry"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

// RunCommand executes a command, registers its PID and command string in the
// active_processes table of the database at dbPath, and cleans it up upon exit.
// It returns the combined stdout and stderr output.
func RunCommand(dbPath string, cmdDir string, name string, args ...string) (string, error) {
	cmd, commandStr, err := createCommand(dbPath, cmdDir, name, args...)
	if err != nil {
		return "", err
	}

	var outBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	_ = telemetry.EmitEvent(cmdDir, "command", commandStr)
	startTime := time.Now()

	// 2. Start child process execution asynchronously.
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start command: %w", err)
	}

	pid := cmd.Process.Pid
	_ = RegisterActiveProcess(dbPath, pid, commandStr)

	// 3. Wait for process completion and record runtime duration.
	waitErr := cmd.Wait()
	_ = DeregisterActiveProcess(dbPath, pid)

	durationSec := time.Since(startTime).Seconds()
	_ = telemetry.EmitEvent(cmdDir, "command_exit", fmt.Sprintf("%.3fs", durationSec))

	output := outBuf.String()
	if waitErr != nil {
		return output, fmt.Errorf("command execution failed: %w", waitErr)
	}

	return output, nil
}

// StartCommand starts a command in the background, registers its PID in the
// active_processes table of the database at dbPath, and returns the *os.Process instance.
// This is used for long-running processes like daemons or web servers where the UI
// needs to maintain a reference to the process without blocking the main execution loop.
func StartCommand(dbPath string, cmdDir string, name string, args ...string) (*os.Process, error) {
	cmd, commandStr, err := createCommand(dbPath, cmdDir, name, args...)
	if err != nil {
		return nil, err
	}

	// Detach process group so it survives terminal closure and SIGHUP signals
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	// Redirect stdout and stderr directly to nomos.jsonl to capture native daemon telemetry
	logPath := filepath.Join(workspace.MustNewContext(cmdDir).LogsDir(), "nomos.jsonl")
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	_ = telemetry.EmitEvent(cmdDir, "command", commandStr)
	startTime := time.Now()

	// 2. Spawn background process.
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start background command: %w", err)
	}

	pid := cmd.Process.Pid
	_ = RegisterActiveProcess(dbPath, pid, commandStr)

	// 3. Monitor process exit asynchronously and trace duration metrics.
	go func() {
		_ = cmd.Wait()
		if logFile != nil {
			_ = logFile.Close()
		}
		_ = DeregisterActiveProcess(dbPath, pid)
		durationSec := time.Since(startTime).Seconds()
		_ = telemetry.EmitEvent(cmdDir, "command_exit", fmt.Sprintf("%.3fs", durationSec))
	}()

	return cmd.Process, nil
}

// RunCommandInteractive executes a command in the foreground, hooking up directly
// to os.Stdout, os.Stderr, and os.Stdin. This is used for dev commands like air or vite.
func RunCommandInteractive(dbPath string, cmdDir string, name string, args ...string) error {
	cmd, commandStr, err := createCommand(dbPath, cmdDir, name, args...)
	if err != nil {
		return err
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	_ = telemetry.EmitEvent(cmdDir, "command_interactive", commandStr)
	startTime := time.Now()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start interactive command: %w", err)
	}

	pid := cmd.Process.Pid
	_ = RegisterActiveProcess(dbPath, pid, commandStr)

	waitErr := cmd.Wait()
	_ = DeregisterActiveProcess(dbPath, pid)

	durationSec := time.Since(startTime).Seconds()
	_ = telemetry.EmitEvent(cmdDir, "command_exit", fmt.Sprintf("%.3fs", durationSec))

	if waitErr != nil {
		return fmt.Errorf("interactive command execution failed: %w", waitErr)
	}

	return nil
}
