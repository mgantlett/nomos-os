// Package exec manages substrate environment injection, process isolation, and security firewall guards.
package exec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
)

// (Socket resolution functions removed)

// resolvePhaseToken reads the active phase token from .phase_state.json.
func resolvePhaseToken(targetDir string) string {
	phasePath := config.PhaseStatePath(targetDir)
	if data, err := os.ReadFile(phasePath); err == nil {
		var state map[string]interface{}
		if json.Unmarshal(data, &state) == nil {
			if t, ok := state["phase_token"].(string); ok {
				return t
			}
		}
	}
	return ""
}

// InjectSubstrateEnvironment inspects targetDir and injects the cryptographic
// phase token configuration into baseEnv.
func InjectSubstrateEnvironment(baseEnv []string, targetDir string) []string {
	cleanedTarget := filepath.Clean(targetDir)
	phaseToken := resolvePhaseToken(cleanedTarget)

	envMap := make(map[string]string)
	for _, kv := range baseEnv {
		splitIdx := strings.IndexByte(kv, '=')
		if splitIdx > 0 {
			envMap[kv[:splitIdx]] = kv[splitIdx+1:]
		}
	}

	envMap["NOMOS_PHASE_TOKEN"] = phaseToken
	envMap["NOMOS_WORKSPACE_ROOT"] = cleanedTarget
	envMap["GIT_TERMINAL_PROMPT"] = "0"
	envMap["GIT_EDITOR"] = "true"

	res := make([]string, 0, len(envMap))
	for k, v := range envMap {
		res = append(res, k+"="+v)
	}
	return res
}

// InjectSubstrateEnvironmentPrimary wraps baseEnv with runtime configuration
// specifically for Mode 1 (Foreground primary workspace delegation).
func InjectSubstrateEnvironmentPrimary(baseEnv []string, ctx *workspace.WorkspaceContext) []string {
	repoRoot := ctx.RepoRoot
	return InjectSubstrateEnvironment(baseEnv, repoRoot)
}

// InjectSubstrateEnvironmentWorktree wraps baseEnv with runtime configuration
// specifically for Mode 2 (Isolated worktree swarm worker execution).
func InjectSubstrateEnvironmentWorktree(baseEnv []string, wtDir string) []string {
	return InjectSubstrateEnvironment(baseEnv, wtDir)
}
