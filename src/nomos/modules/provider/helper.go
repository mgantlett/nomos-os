// Package provider orchestrates the underlying LLM inference nodes.
// It manages the lifecycles, configuration overrides, and networking bounds
// for local agent models like Aider and local LLMs like Ollama or Llama.cpp.
package provider

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/telemetry"
	nomosexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/spf13/viper"
)

// ModelConfig represents configuration details for a single model,
// including its name, endpoint URL, inference settings, and host provider.
type ModelConfig struct {
	Model       string  `mapstructure:"model"`
	URL         string  `mapstructure:"url"`
	Temperature float64 `mapstructure:"temperature"`
	TopP        float64 `mapstructure:"top_p"`
	MaxTurns    int     `mapstructure:"max_turns"`
	MaxTokens   int     `mapstructure:"max_tokens"`
	Provider    string  `mapstructure:"provider"`
}

// ModelsYAML represents the schema of .nomos/models.yaml configuration file,
// holding default settings, phase-specific settings, and VM provider targets.
type ModelsYAML struct {
	Default   ModelConfig               `mapstructure:"default"`
	Phases    map[string]ModelConfig    `mapstructure:"phases"`
	Providers map[string]ProviderConfig `mapstructure:"providers"`
}

// parseEnvLine parses a single key=value line from a .env file into the target map.
func parseEnvLine(line string, res map[string]string) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return
	}
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return
	}
	k := strings.TrimSpace(parts[0])
	v := strings.TrimSpace(parts[1])
	if strings.HasPrefix(v, "\"") && strings.HasSuffix(v, "\"") {
		v = v[1 : len(v)-1]
	}
	res[k] = v
}

// ParseEnvFileToMap reads a simple .env file and extracts its key-value pairs into a map.
// It skips lines that are empty or begin with a '#' character, keeping only valid assignments.
// The resulting map is used to apply environmental overrides onto the agent and model configurations.
func ParseEnvFileToMap(path string) map[string]string {
	res := make(map[string]string)
	data, err := os.ReadFile(path)
	if err != nil {
		return res
	}
	for _, line := range strings.Split(string(data), "\n") {
		parseEnvLine(line, res)
	}
	return res
}

// SetupModelProviderTunnel starts the VM and SSH tunnel if the models.yaml config resolves to a provider
// for the given phase. It returns the resolved modelName, llmURL, a cleanup function to close the tunnel, and any error.
// loadModelsConfig reads and unmarshals the models.yaml config file from the project.
// If the configuration file is missing or unparseable, it returns an error.
func loadModelsConfig(ctx *workspace.WorkspaceContext) (*ModelsYAML, error) {
	repoRoot := ctx.RepoRoot
	v := viper.New()
	v.SetConfigFile(config.ModelsPath(repoRoot))
	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	var myaml ModelsYAML
	if err := v.Unmarshal(&myaml); err != nil {
		return nil, err
	}
	return &myaml, nil
}

// applyPhaseOverrides updates provider, model, and URL pointers from phase configuration settings.
func applyPhaseOverrides(phaseCfg ModelConfig, providerName *string, modelName *string, llmURL *string) {
	if phaseCfg.Provider != "" {
		*providerName = phaseCfg.Provider
	}
	if phaseCfg.Model != "" {
		*modelName = phaseCfg.Model
	}
	if phaseCfg.URL != "" {
		*llmURL = phaseCfg.URL
	}
}

// resolveFromModelsYAML overrides default model, provider, and URL from models.yaml configuration.
// It iterates default settings, checks for task-phase overrides, and fetches provider properties.
func resolveFromModelsYAML(myaml *ModelsYAML, phase string, modelName *string, llmURL *string) (string, ProviderConfig, bool) {
	providerName := myaml.Default.Provider
	if myaml.Default.Model != "" {
		*modelName = myaml.Default.Model
	}
	if myaml.Default.URL != "" {
		*llmURL = myaml.Default.URL
	}

	if phase != "" {
		if phaseCfg, ok := myaml.Phases[strings.ToLower(phase)]; ok {
			applyPhaseOverrides(phaseCfg, &providerName, modelName, llmURL)
		}
	}

	if providerName == "" {
		return providerName, ProviderConfig{}, false
	}

	cfg, ok := myaml.Providers[providerName]
	return providerName, cfg, ok
}

// resolveProviderAndModel resolves LLM provider configuration and model name.
// It extracts environmental values from config.env in the global data directory, falls back to models.yaml default
// configurations, applies task phase overrides, and handles openai model prefixes.
func resolveProviderAndModel(ctx *workspace.WorkspaceContext, phase string) (string, string, string, ProviderConfig, bool) {
	repoRoot := ctx.RepoRoot
	llmURL := "http://localhost:8082/v1"
	modelName := "model"

	envMap := ParseEnvFileToMap(filepath.Join(config.GlobalDataDir(repoRoot), "config.env"))
	if u := envMap["NOMOS_SWARM_LLM_URL"]; u != "" {
		llmURL = u
	}
	if m := envMap["NOMOS_SWARM_LLM_MODEL"]; m != "" {
		modelName = m
	}

	var providerName string
	var selectedProvider ProviderConfig
	var providerFound bool

	if myaml, err := loadModelsConfig(ctx); err == nil {
		providerName, selectedProvider, providerFound = resolveFromModelsYAML(myaml, phase, &modelName, &llmURL)
	}

	if strings.HasPrefix(modelName, "openai/") {
		modelName = strings.TrimPrefix(modelName, "openai/")
	}

	return modelName, llmURL, providerName, selectedProvider, providerFound
}

// waitForTunnelPort polls CheckLocalPort until the port becomes reachable or times out.
func waitForTunnelPort(port int) bool {
	for i := 0; i < 300; i++ {
		if i > 0 && i%20 == 0 {
			fmt.Printf("   ↳ Waiting for remote endpoint on tunnel port %d (attempt %d/300)...\n", port, i)
		}
		time.Sleep(500 * time.Millisecond)
		if CheckLocalPort(port) {
			return true
		}
	}
	return false
}

// initProxyTunnel handles the creation of the reverse proxy tunnel and verifies port connectivity.
func initProxyTunnel(dbPath string, selectedProvider ProviderConfig, ip string) (func(), error) {
	if CheckLocalPort(selectedProvider.LocalPort) {
		return func() {}, nil
	}

	fmt.Printf("🔗 Establishing SSH tunnel on localhost:%d -> localhost:%d...\n", selectedProvider.LocalPort, selectedProvider.RemotePort)
	proc, err := StartTunnel(dbPath, selectedProvider, ip)
	if err != nil {
		return nil, fmt.Errorf("failed to start tunnel: %w", err)
	}

	cleanup := func() {
		if proc != nil {
			_ = proc.Kill()
		}
	}

	if !waitForTunnelPort(selectedProvider.LocalPort) {
		cleanup()
		return nil, fmt.Errorf("timeout waiting for SSH tunnel port %d to become active (300 attempts)", selectedProvider.LocalPort)
	}

	fmt.Printf("   ↳ Tunnel successfully active on localhost:%d\n", selectedProvider.LocalPort)
	return cleanup, nil
}

// establishSshTunnel starts the necessary Provider VM and initializes a reverse proxy tunnel
// that binds a local port to the external IP endpoint. It ensures the local port is fully
// reachable before handing the cleanup responsibilities back to the caller goroutine.
func establishSshTunnel(dbPath string, providerName string, selectedProvider ProviderConfig) (string, func(), error) {
	fmt.Printf("🚀 Initializing provider VM %q...\n", providerName)
	ip, err := StartProvider(dbPath, selectedProvider)
	if err != nil {
		return "", nil, fmt.Errorf("failed to start provider %q: %w", providerName, err)
	}
	fmt.Printf("   ↳ VM is running at external NAT IP: %s\n", ip)

	cleanup, err := initProxyTunnel(dbPath, selectedProvider, ip)
	if err != nil {
		return "", nil, err
	}
	llmURL := fmt.Sprintf("http://localhost:%d/v1", selectedProvider.LocalPort)
	return llmURL, cleanup, nil
}

// SetupModelProviderTunnel starts the VM and SSH tunnel if the models.yaml config resolves to a provider
// for the given phase. It returns the resolved modelName, llmURL, a cleanup function to close the tunnel, and any error.
func SetupModelProviderTunnel(ctx *workspace.WorkspaceContext, dbPath string, phase string) (string, string, func(), error) {
	modelName, llmURL, providerName, selectedProvider, providerFound := resolveProviderAndModel(ctx, phase)

	var cleanup func() = func() {}
	if providerFound {
		var err error
		llmURL, cleanup, err = establishSshTunnel(dbPath, providerName, selectedProvider)
		if err != nil {
			return "", "", nil, err
		}
	}

	return modelName, llmURL, cleanup, nil
}

// ResolveInitialRoot parses parent workspace path from worktree directory structures.
func ResolveInitialRoot(path string) string {
	idx := strings.Index(path, "/.nomos/worktrees/task-")
	if idx != -1 {
		return path[:idx]
	}
	return path
}

// checkAndAddPlanFile checks if a path exists and appends it to the file slice if unseen.
func checkAndAddPlanFile(repoRoot string, w string, seen map[string]bool, files *[]string) {
	w = strings.Trim(w, "`'\",.*()[]{}")
	if !strings.HasPrefix(w, "src/") || seen[w] {
		return
	}
	fullPath := filepath.Join(repoRoot, w)
	if _, err := os.Stat(fullPath); err == nil {
		*files = append(*files, w)
		seen[w] = true
	}
}

// resolveAiderPlanFiles extracts target source code files mentioned in implementation plans.
func resolveAiderPlanFiles(ctx *workspace.WorkspaceContext, planPath string) []string {
	repoRoot := ctx.RepoRoot
	var files []string
	data, err := os.ReadFile(planPath)
	if err != nil {
		return files
	}

	words := strings.Fields(string(data))
	seen := make(map[string]bool)
	for _, w := range words {
		checkAndAddPlanFile(repoRoot, w, seen, &files)
	}
	return files
}

// resolveAiderArgs constructs the exact CLI arguments to pass to the Aider agent process.
// It checks if a spec plan exists, appends model and api-url parameters, and sets up
// auto-commit parameters for automated workspace tasks.
func resolveAiderArgs(ctx *workspace.WorkspaceContext, planPath, modelName, llmURL string) []string {
	repoRoot := ctx.RepoRoot
	chatHistory := filepath.Join(config.GlobalDataDir(repoRoot), "tmp", "aider.chat.history.md")
	args := []string{
		"--model", "openai/" + modelName,
		"--openai-api-base", llmURL,
		"--openai-api-key", "fake-key",
		"--message-file", planPath,
		"--chat-history-file", chatHistory,
		"--no-git",
		"--yes",
		"--exit",
		"--no-show-model-warnings",
		"--no-analytics",
		"--no-auto-commits",
		"--edit-format", "diff",
	}

	files := resolveAiderPlanFiles(ctx, planPath)
	if len(files) > 0 {
		args = append(args, files...)
	}
	return args
}

// resolveAiderExecCommand configures the shell command wrapper array to execute Aider.
// If a shell.nix file exists in the repository root, it wraps the command in nix-shell
// so that dependencies are correctly resolved; otherwise it invokes aider directly.
func resolveAiderExecCommand(ctx *workspace.WorkspaceContext, args []string) []string {
	repoRoot := ctx.RepoRoot
	var argv []string
	shellNixPath := filepath.Join(repoRoot, "shell.nix")
	hasShellNix := false
	if _, err := os.Stat(shellNixPath); err == nil {
		hasShellNix = true
	}

	if hasShellNix {
		runCmdStr := "aider"
		if len(args) > 0 {
			runCmdStr = nomosexec.ShellEscapeArgs("aider", args)
		}
		argv = []string{"/usr/bin/env", "nix-shell", "--keep", "OPENAI_API_KEY", "--keep", "OPENAI_API_BASE", "--run", runCmdStr}
	} else {
		argv = append([]string{"/usr/bin/env", "aider"}, args...)
	}
	return argv
}

func logAiderFailure(initialRoot, key string, state *os.ProcessState, waitErr error) {
	if waitErr == nil && state != nil {
		waitErr = fmt.Errorf("exit status %d", state.ExitCode())
	}
	detail := fmt.Sprintf("Aider task %s failed: %v", key, waitErr)
	_ = telemetry.LogTelemetryEvent(initialRoot, "aider_error", detail, key, "aider", nil)
}

func logAiderSuccess(ctx *workspace.WorkspaceContext, initialRoot, dbPath, key string) {
	detail := fmt.Sprintf("Aider task %s finished", key)
	_ = telemetry.LogTelemetryEvent(initialRoot, "aider_complete", detail, key, "aider", nil)
	_ = task.TransitionPhase(ctx, "REVIEW")
	verifyOut, verifyErr := nomosexec.RunCommand(dbPath, "bin/nomos", "verify")
	if verifyErr != nil {
		_ = telemetry.LogTelemetryEvent(initialRoot, "verify_fail", fmt.Sprintf("DoD failed: %v", verifyErr), key, "aider", map[string]interface{}{"output": verifyOut})
		return
	}
	_ = telemetry.LogTelemetryEvent(initialRoot, "verify_pass", "DoD passed successfully", key, "aider", nil)
}

func processStreamLogs(r *os.File, initialRoot, key string, logFile *os.File) {
	defer r.Close()
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if logFile != nil {
			_, _ = logFile.WriteString(line + "\n")
		}
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			_ = telemetry.LogTelemetryEvent(initialRoot, "info", trimmed, key, "aider", map[string]interface{}{"source": "aider"})
		}
	}
}

// pipeStreamLogs creates a pipe that streams stdout/stderr from background processes
// to both the file log writer and real-time telemetry JSONL events.
func pipeStreamLogs(initialRoot, key string, logFile *os.File) (*os.File, *os.File, func()) {
	r, w, err := os.Pipe()
	if err != nil {
		return logFile, logFile, func() {}
	}

	go processStreamLogs(r, initialRoot, key, logFile)
	return r, r, func() { w.Close() }
}

func executeAiderLifecycle(ctx *workspace.WorkspaceContext, initialRoot, dbPath, key string, proc *os.Process, cleanup func(), pid int, pipeCleanup func()) {
	if cleanup != nil {
		defer cleanup()
	}

	state, waitErr := proc.Wait()
	if pipeCleanup != nil {
		pipeCleanup()
	}
	_ = nomosexec.DeregisterActiveProcess(dbPath, pid)

	if waitErr != nil || (state != nil && !state.Success()) {
		logAiderFailure(initialRoot, key, state, waitErr)
	} else {
		logAiderSuccess(ctx, initialRoot, dbPath, key)
	}
}

// handleAiderLifecycle initiates an asynchronous goroutine to monitor the background Aider process.
// It waits for process termination, de-registers its PID from the active SQLite registry,
// logs telemetry events, transitions the task phase to REVIEW, and runs DoD verification.
func handleAiderLifecycle(ctx *workspace.WorkspaceContext, initialRoot, dbPath, key string, proc *os.Process, logFile *os.File, cleanup func(), pid int, pipeCleanup func()) {
	go func() {
		defer logFile.Close()
		executeAiderLifecycle(ctx, initialRoot, dbPath, key, proc, cleanup, pid, pipeCleanup)
	}()
}
