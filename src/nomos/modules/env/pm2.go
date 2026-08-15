package env

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	nexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
)

// ensurePM2Sync reconciles the in-memory PM2 daemon version with the local CLI binary.
// This prevents version mismatch warnings and daemon freezes across environment runs.
func ensurePM2Sync(repoRoot string) {
	dbPath := config.ResolveCacheDbPath(repoRoot)
	_, _ = nexec.RunCommand(dbPath, repoRoot, "npx", "--prefer-offline", "pm2", "update")
}

// writeScript materializes the command string into a global per-service shell script.
// The shell script exists to avoid PM2 quoting and parsing bugs when launching binaries.
// Services are resolved from services.go (the single source of truth) and regenerated
// on both Start and Restart so the two never drift out of sync. The script lives in the
// repo-independent EnvScriptsDir: PM2 stores the exec path at registration time and
// re-executes that stored path on restart, so a cwd-derived location would silently
// orphan regenerated configs.
func writeScript(ctx *workspace.WorkspaceContext, name, cmdStr string) (string, error) {
	repoRoot := ctx.RepoRoot
	stateDir := config.EnvScriptsDir(repoRoot)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create state directory %s: %w", stateDir, err)
	}
	scriptPath := filepath.Join(stateDir, name+".sh")

	// Write the command to a shell script to avoid PM2 quoting and parsing bugs.
	scriptContent := fmt.Sprintf("#!/bin/bash\nexec %s\n", cmdStr)
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		return "", fmt.Errorf("failed to write pm2 startup script: %w", err)
	}
	return scriptPath, nil
}

// Start launches a background daemon via PM2.
// It creates necessary state and logging directories, generates an isolated shell script,
// and invokes the underlying runner to track process execution.
func Start(ctx *workspace.WorkspaceContext, name, logFile, cmdStr, cwd string) error {
	repoRoot := ctx.RepoRoot
	ensurePM2Sync(repoRoot)

	// Resolve logging directory path
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		return fmt.Errorf("failed to create log directory %s: %w", filepath.Dir(logFile), err)
	}

	dbPath := config.ResolveCacheDbPath(repoRoot)
	if err := reconcileRegistration(dbPath, ctx, name, cmdStr, logFile, cwd); err != nil {
		return err
	}

	// ReconcileRegistration only registers the process if missing/stale.
	// If it was already correctly registered but in a 'stopped' state,
	// we must explicitly start it now.
	out, err := nexec.RunCommand(dbPath, repoRoot, "npx", "--prefer-offline", "pm2", "start", name)
	if err != nil {
		return fmt.Errorf("pm2 start %s failed: %w, output: %s", name, err, out)
	}
	return nil
}

// Stop terminates a background daemon via PM2.
func Stop(ctx *workspace.WorkspaceContext, name string) error {
	repoRoot := ctx.RepoRoot
	dbPath := config.ResolveCacheDbPath(repoRoot)
	out, err := nexec.RunCommand(dbPath, repoRoot, "npx", "--prefer-offline", "pm2", "stop", name)
	if err != nil {
		return fmt.Errorf("pm2 stop failed: %w, output: %s", err, out)
	}
	return nil
}

// Restart regenerates the startup script from the resolved service config (services.go
// is the single source of truth) and restarts the daemon via PM2. This ensures config
// changes take effect on restart without manual edits to the state scripts.
//
// PM2 stores the exec script path at registration time and re-executes that stored
// path on `pm2 restart`. If the stored path differs from the canonical global script
// (e.g. the process was registered from a different cwd-derived repoRoot), we must
// re-register via `pm2 delete` + `pm2 start` so the regenerated script is actually used.
func Restart(ctx *workspace.WorkspaceContext, name string) error {
	repoRoot := ctx.RepoRoot
	dbPath := config.ResolveCacheDbPath(repoRoot)

	if name == "all" {
		for _, s := range GetAllServices() {
			if err := ensureRegisteredScriptPath(dbPath, ctx, s); err != nil {
				return err
			}
		}
		out, err := nexec.RunCommand(dbPath, repoRoot, "npx", "--prefer-offline", "pm2", "restart", name)
		if err != nil {
			return fmt.Errorf("pm2 restart failed: %w, output: %s", err, out)
		}
		return nil
	}

	// Ensure PM2 stores the canonical global script path for this service.
	if err := ensureRegisteredScriptPath(dbPath, ctx, name); err != nil {
		return err
	}

	svc, err := ResolveService(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to resolve service %s: %w", name, err)
	}
	if _, err := writeScript(ctx, svc.Name, svc.Command); err != nil {
		return err
	}

	out, err := nexec.RunCommand(dbPath, repoRoot, "npx", "--prefer-offline", "pm2", "restart", name)
	if err != nil {
		return fmt.Errorf("pm2 restart failed: %w, output: %s", err, out)
	}
	return nil
}

// ensureRegisteredScriptPath reconciles the PM2 registration for a service so it runs
// the canonical global script (EnvScriptsDir). PM2 stores the exec path at start time,
// so if the registered path differs from the canonical one we re-register via
// `pm2 delete` + `pm2 start`, preserving the original log file and cwd.
func ensureRegisteredScriptPath(dbPath string, ctx *workspace.WorkspaceContext, name string) error {
	svc, err := ResolveService(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to resolve service %s: %w", name, err)
	}
	return reconcileRegistration(dbPath, ctx, svc.Name, svc.Command, svc.LogFile, svc.Cwd)
}

// reconcileRegistration ensures PM2 has the named daemon registered against the
// canonical global script path, regenerating the script from cmdStr. PM2 stores the
// exec path at registration time, so if the registered path differs from the canonical
// one (e.g. registered from a different cwd-derived repoRoot) we re-register via
// `pm2 delete` + `pm2 start`, preserving the original log file and cwd.
func reconcileRegistration(dbPath string, ctx *workspace.WorkspaceContext, name, cmdStr, logFile, cwd string) error {
	repoRoot := ctx.RepoRoot
	canonicalPath := filepath.Join(config.EnvScriptsDir(repoRoot), name+".sh")

	registeredPath, registeredLogFile, registeredCwd, err := resolveRegisteredPM2Process(ctx, name)
	if err != nil {
		return err
	}

	scriptPath, err := writeScript(ctx, name, cmdStr)
	if err != nil {
		return err
	}

	if registeredPath != "" && registeredPath == canonicalPath {
		// Already registered against the canonical script; regenerate is done.
		return nil
	}

	// Either the process isn't registered yet or it points at a stale, cwd-derived
	// script path. Re-register so PM2 execs the canonical global script on restart.
	if registeredPath != "" {
		if _, err := nexec.RunCommand(dbPath, repoRoot, "npx", "--prefer-offline", "pm2", "delete", name); err != nil {
			return fmt.Errorf("pm2 delete failed: %w", err)
		}
	}

	// Preserve the log file and cwd that PM2 already has registered for the process.
	if registeredLogFile != "" {
		logFile = registeredLogFile
	}
	if registeredCwd != "" {
		cwd = registeredCwd
	}

	args := []string{
		"-u", "name",
		"npx", "--prefer-offline", "pm2", "start", scriptPath,
		"--name", name,
		"--log", logFile,
		"--merge-logs",
	}
	if cwd != "" {
		args = append(args, "--cwd", cwd)
	}
	if _, err := nexec.RunCommand(dbPath, repoRoot, "env", args...); err != nil {
		return fmt.Errorf("pm2 start failed: %w", err)
	}
	return nil
}

// resolveRegisteredPM2Process returns the stored exec path, merged log path, and cwd
// that PM2 has registered for the named daemon, or empty strings if not registered.
func resolveRegisteredPM2Process(ctx *workspace.WorkspaceContext, name string) (string, string, string, error) {
	repoRoot := ctx.RepoRoot
	dbPath := config.ResolveCacheDbPath(repoRoot)
	out, err := nexec.RunCommand(dbPath, repoRoot, "npx", "--prefer-offline", "pm2", "jlist")
	if err != nil {
		return "", "", "", fmt.Errorf("pm2 jlist failed: %w", err)
	}

	var procs []struct {
		Name   string `json:"name"`
		PM2Env struct {
			PMExecPath string `json:"pm_exec_path"`
			PMLogPath  string `json:"pm_log_path"`
			PMCwd      string `json:"pm_cwd"`
		} `json:"pm2_env"`
	}
	if err := json.Unmarshal([]byte(out), &procs); err != nil {
		return "", "", "", fmt.Errorf("failed to parse pm2 jlist: %w", err)
	}

	for _, p := range procs {
		if p.Name == name {
			return p.PM2Env.PMExecPath, p.PM2Env.PMLogPath, p.PM2Env.PMCwd, nil
		}
	}
	return "", "", "", nil
}

// List returns the telemetry of all PM2 daemons. If asJSON is true, returns raw `pm2 jlist` output.
func List(ctx *workspace.WorkspaceContext, asJSON bool) (string, error) {
	repoRoot := ctx.RepoRoot
	dbPath := config.ResolveCacheDbPath(repoRoot)
	cmd := "list"
	if asJSON {
		cmd = "jlist"
	}
	out, err := nexec.RunCommand(dbPath, repoRoot, "npx", "--prefer-offline", "pm2", cmd)
	if err != nil {
		return "", fmt.Errorf("pm2 %s failed: %w", cmd, err)
	}
	return strings.TrimSpace(out), nil
}

// Logs streams the logs of a background daemon via PM2.
// It returns the output string containing the recent logs.
func Logs(ctx *workspace.WorkspaceContext, name string) (string, error) {
	repoRoot := ctx.RepoRoot
	dbPath := config.ResolveCacheDbPath(repoRoot)
	// We run `pm2 logs --nostream` to just fetch and return, rather than hang.
	// If the user expects streaming in the CLI, we could exec differently, but this is fine for now.
	out, err := nexec.RunCommand(dbPath, repoRoot, "npx", "--prefer-offline", "pm2", "logs", name, "--nostream")
	if err != nil {
		return "", fmt.Errorf("pm2 logs failed: %w, output: %s", err, out)
	}
	return out, nil
}
