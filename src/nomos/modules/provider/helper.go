// Package provider orchestrates the underlying LLM inference nodes.
// It manages the lifecycles, configuration overrides, and networking bounds
// for local agent models like Aider and local LLMs like Ollama or Llama.cpp.
package provider

import (
	"fmt"
	"strings"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

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


// SetupModelProviderTunnel starts the VM and SSH tunnel if the models.yaml config resolves to a provider
// for the given phase. It returns the resolved modelName, llmURL, a cleanup function to close the tunnel, and any error.
// loadModelsConfig reads and unmarshals the models.yaml config file from the project.
// If the configuration file is missing or unparseable, it returns an error.
func loadModelsConfig(ctx *workspace.WorkspaceContext) (*ModelsYAML, error) {
	repoRoot := ctx.RepoRoot
	v := viper.New()
	v.SetConfigFile(workspace.MustNewContext(repoRoot).DataPath("models.yaml"))
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
func resolveProviderAndModel(ctx *workspace.WorkspaceContext, phase string) (string, string, string, string, ProviderConfig, bool) {
	repoRoot := ctx.RepoRoot
	llmURL := "http://localhost:8082/v1"
	modelName := "model"
	apiKey := ""

	projCfg, err := config.LoadProjectSettings(repoRoot)
	if err == nil && projCfg.ActiveSwarmProvider != "" {
		if cp, ok := projCfg.CloudProviders[projCfg.ActiveSwarmProvider]; ok {
			llmURL = cp.URL
			modelName = cp.DefaultModel
			apiKey = cp.APIKey
		}
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

	return modelName, llmURL, apiKey, providerName, selectedProvider, providerFound
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
// for the given phase. It returns the resolved modelName, llmURL, apiKey, a cleanup function to close the tunnel, and any error.
func SetupModelProviderTunnel(ctx *workspace.WorkspaceContext, dbPath string, phase string) (string, string, string, func(), error) {
	modelName, llmURL, apiKey, providerName, selectedProvider, providerFound := resolveProviderAndModel(ctx, phase)

	var cleanup func() = func() {}
	if providerFound {
		var err error
		llmURL, cleanup, err = establishSshTunnel(dbPath, providerName, selectedProvider)
		if err != nil {
			return "", "", "", nil, err
		}
	}

	return modelName, llmURL, apiKey, cleanup, nil
}

// ResolveInitialRoot parses parent workspace path from worktree directory structures.
func ResolveInitialRoot(path string) string {
	idx := strings.Index(path, "/.nomos/worktrees/task-")
	if idx != -1 {
		return path[:idx]
	}
	return path
}


