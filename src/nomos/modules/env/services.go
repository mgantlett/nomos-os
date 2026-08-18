package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

// resolveCudaLibPath finds the paths to the NVIDIA CUDA libraries and host drivers.
// Migrated from provider package to env package.
// resolveCudaLibPath finds the paths to the NVIDIA CUDA libraries and host drivers.
// Migrated from provider package to env package.
func resolveCudaLibPath() string {
	// Initialize default OpenGL driver library path slice
	paths := []string{"/run/opengl-driver/lib"}
	// Glob search for all CUDA library Nix store paths
	matches, err := filepath.Glob("/nix/store/*cuda*/lib")
	if err == nil {
		for _, m := range matches {
			paths = append(paths, m)
		}
	}
	// Join search paths with standard OS colon delimiter
	return strings.Join(paths, ":")
}

// ServiceConfig defines command configuration and port mappings for workspace services.
type ServiceConfig struct {
	Name         string // Service identifier string used by CLI subcommands
	BuildCommand string // Build step executed prior to starting service instance
	Command      string // Foreground or background execution command string
	DevCommand   string // Command executed directly in foreground for hot-reloading
	LogFile      string // Absolute path to the unified output log file for this service
	Cwd          string // Working directory for the service. If empty, defaults to repoRoot.
	Port         int    // Network port the service listens on for HTTP/WebSocket traffic
}

// ResolveService returns the command configuration for a known service.
// It maps service names to their respective binary execution paths, default ports, and log file destinations.
func getLlamaBinPath() string {
	llamaBin := "/home/markg/llama.cpp/build/bin/llama-server"
	if _, err := os.Stat(llamaBin); err != nil {
		return "llama-server"
	}
	return llamaBin
}

func ResolveService(ctx *workspace.WorkspaceContext, service string) (*ServiceConfig, error) {
	repoRoot := ctx.RepoRoot
	// Construct absolute log file path destination inside active workspace logs directory
	logFile := filepath.Join(workspace.MustNewContext(repoRoot).LogsDir(), "nomos.jsonl")

	switch service {
	case "nomos":
		// The nomos engine itself (CLI). Not a daemon, but buildable.
		// Returns ServiceConfig targeting CLI build step
		return &ServiceConfig{
			Name:         "nomos",
			BuildCommand: "nix-shell --run \"go build -o bin/nomos ./src/nomos/main.go\"",
		}, nil

	case "llama-coder":
		// Execute local llama.cpp LLM server bound to CUDA library path on port 8082
		// Binds DeepSeek-R1-Distill-Qwen-14B model instance for code completion and instruction following with full GPU offload (-ngl 99)
		// Reasoning-tuned sampling: temp 0.6 / top-p 0.95 per DeepSeek R1 guidance, min-p 0.05 to cut low-probability
		// token tails, DRY 0.8 to break thinking loops, q8_0 KV cache to fit 32k context in VRAM, flash-attention for memory efficiency.
		cmdStr := fmt.Sprintf("env LD_LIBRARY_PATH=\"%s\" %s -ngl 99 -m /home/markg/models/DeepSeek-R1-Distill-Qwen-14B-Q4_K_M.gguf --alias deepseek-r1-distill-qwen-14b-q4_k_m.gguf --host 0.0.0.0 --port 8082 -c 32768 -b 8192 -ub 8192 --flash-attn on --cache-type-k q8_0 --cache-type-v q8_0 --temp 0.6 --top-p 0.95 --min-p 0.05 --dry-multiplier 0.8", resolveCudaLibPath(), getLlamaBinPath())
		return &ServiceConfig{
			Name:         "llama-coder",
			Command:      cmdStr,
			BuildCommand: "echo 'No build required; native CUDA GPU binary linked.'",
			LogFile:      logFile,
			Port:         8082,
		}, nil

	case "llama-embed":
		// Execute local text embedding model server on port 8081.
		// Runs fully on CPU (-ngl 0) to keep GPU VRAM available for the llama-coder reasoning model.
		cmdStr := fmt.Sprintf("env LD_LIBRARY_PATH=\"%s\" %s -ngl 0 --device none -m /home/markg/llama.cpp/models/nomic-embed-text-v1.5.Q4_K_M.gguf --host 0.0.0.0 --port 8081 --embedding -c 8192 -b 8192 -ub 8192", resolveCudaLibPath(), getLlamaBinPath())
		return &ServiceConfig{
			Name:         "llama-embed",
			Command:      cmdStr,
			BuildCommand: "echo 'No build required; native CUDA GPU binary linked.'",
			LogFile:      logFile,
			Port:         8081,
		}, nil

	case "datasette":
		// Example defaults for datasette, used to explore SQLite state databases.
		// Exposes the cache db on port 8001 by default.
		// Configures background command to serve the local SQLite cache database.
		cmdStr := fmt.Sprintf("datasette serve %s/.nomos/data/cache.db --host 0.0.0.0 --port 8001", repoRoot)
		return &ServiceConfig{
			Name:         "datasette",
			Command:      cmdStr,
			BuildCommand: "pip install datasette",
			LogFile:      logFile,
			Port:         8001,
		}, nil

	case "cockpit", "cockpit-dev":
		nomosBin, err := os.Executable()
		if err != nil {
			nomosBin = "nomos" // fallback
		}
		cmdStr := fmt.Sprintf("%s cockpit daemon", nomosBin)
		buildCmd := "echo 'Cockpit is built centrally with nomos'"

		var nomosOsRoot string
		if b, err := os.ReadFile(ctx.StateTaskIdPath()); err == nil {
			taskId := strings.TrimSpace(string(b))
			if taskId != "" {
				wtPath := filepath.Join(repoRoot, "worktrees", "nomos-os-"+taskId)
				if _, statErr := os.Stat(wtPath); statErr == nil {
					nomosOsRoot = wtPath
				}
			}
		}
		if nomosOsRoot == "" {
			nomosOsRoot = workspace.ResolveProjectRoot(repoRoot, "nomos-os")
		}

		// Return resolved ServiceConfig instance for environment substrate
		return &ServiceConfig{
			Name:         "cockpit",
			Command:      cmdStr,
			BuildCommand: buildCmd,
			DevCommand:   fmt.Sprintf(`npx -y concurrently -k "cd %s/src/nomos/modules/cockpit/ui && tsc -w" "%s"`, nomosOsRoot, cmdStr),
			LogFile:      logFile,
			Cwd:          repoRoot,
			Port:         8089,
		}, nil

	case "cockpit-sovereign", "cockpit-sovereign-dev":
		var sovereignRoot string
		
		// Attempt to resolve via active task ID first
		if b, err := os.ReadFile(ctx.StateTaskIdPath()); err == nil {
			taskId := strings.TrimSpace(string(b))
			if taskId != "" {
				wtPath := filepath.Join(repoRoot, "worktrees", "nomos-sovereign-"+taskId)
				if _, statErr := os.Stat(wtPath); statErr == nil {
					sovereignRoot = wtPath
				}
			}
		}

		if sovereignRoot == "" {
			sovereignRoot = workspace.ResolveProjectRoot(repoRoot, "nomos-sovereign")
		}
		if sovereignRoot == "" {
			return nil, fmt.Errorf("nomos-sovereign workspace not found")
		}
		cmdStr := "go run github.com/mgantlett/nomos-sovereign/src/nomos-cockpit/src/cmd/cockpitd --port 8090"
		buildCmd := fmt.Sprintf("go build -o %s/bin/cockpitd github.com/mgantlett/nomos-sovereign/src/nomos-cockpit/src/cmd/cockpitd", sovereignRoot)

		var cwdRoot string = ctx.PrimaryWorktree
		if b, err := os.ReadFile(ctx.StateTaskIdPath()); err == nil {
			taskId := strings.TrimSpace(string(b))
			if taskId != "" {
				// Reconstruct Cwd to orchestrator worktree which has go.work
				wtPath := filepath.Join(repoRoot, "worktrees", "nomos-commons-"+taskId)
				if _, statErr := os.Stat(wtPath); statErr == nil {
					cwdRoot = wtPath
				}
			}
		}

		var nomosOsRoot string
		if b, err := os.ReadFile(ctx.StateTaskIdPath()); err == nil {
			taskId := strings.TrimSpace(string(b))
			if taskId != "" {
				wtPath := filepath.Join(repoRoot, "worktrees", "nomos-os-"+taskId)
				if _, statErr := os.Stat(wtPath); statErr == nil {
					nomosOsRoot = wtPath
				}
			}
		}
		if nomosOsRoot == "" {
			nomosOsRoot = workspace.ResolveProjectRoot(repoRoot, "nomos-os")
		}

		return &ServiceConfig{
			Name:         "cockpit-sovereign",
			Command:      cmdStr,
			BuildCommand: buildCmd,
			DevCommand:   fmt.Sprintf(`npx -y concurrently -k "cd %s/src/nomos/modules/cockpit/ui && tsc -w" "%s"`, nomosOsRoot, cmdStr),
			LogFile:      logFile,
			Cwd:          cwdRoot,
			Port:         8090,
		}, nil

	case "vitepress":
		// Example defaults for the vitepress documentation engine.
		// Launches the local markdown server for viewing architecture docs.
		// Binds documentation site host and port configuration.
		docsDir := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(repoRoot))), "gantlett-systems", "gsi-management")
		cmdStr := "npm run docs:dev -- --host 0.0.0.0 --port 5174"
		return &ServiceConfig{
			Name:         "vitepress",
			Command:      cmdStr,
			BuildCommand: fmt.Sprintf("cd %s && npm install", docsDir),
			DevCommand:   cmdStr,
			LogFile:      logFile,
			Cwd:          docsDir,
			Port:         5174,
		}, nil

	default:
		return nil, fmt.Errorf("unknown environment service: %s", service)
	}
}

// GetAllServices returns a list of all known background daemon service names.
// Used by environment management subcommands when restarting or inspecting all active daemons.
func GetAllServices() []string {
	// Return list of canonical service identifiers configured in the environment substrate
	return []string{
		"llama-coder",
		"llama-embed",
		// "datasette", // Temporarily bypassed due to python nixpkgs failure
		"cockpit",
		"vitepress",
	}
}

// injectCudaEnv appends resolved CUDA library paths to an environment execution command string.
// Resolves existing LD_LIBRARY_PATH environment variables and prepends OpenGL and Nix CUDA library paths.
func injectCudaEnv(cmdStr string) string {
	// Initialize empty LD_LIBRARY_PATH string variable
	ldPathStr := ""
	// Check if valid CUDA library paths exist on host operating system
	if cudaPath := resolveCudaLibPath(); cudaPath != "" {
		ldPath := os.Getenv("LD_LIBRARY_PATH")
		// Merge existing environment library paths with resolved CUDA paths
		if ldPath != "" {
			ldPathStr = "LD_LIBRARY_PATH=" + cudaPath + ":" + ldPath
		} else {
			ldPathStr = "LD_LIBRARY_PATH=" + cudaPath
		}
	}
	// Return command string with explicit env prefix if CUDA library paths exist
	if ldPathStr != "" {
		return "env " + ldPathStr + " " + cmdStr
	}
	return cmdStr
}
