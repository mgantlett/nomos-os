package exec

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"

	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

// Substrate represents the underlying system execution environment
// and context. It encompasses process spawning, environment variables,
// binary location, and workspace root discovery. It acts as the
// universal shim layer isolating business logic from direct OS
// system calls, providing hooks for telemetry, security checks,
// and Nix-shell wrapping.
//
// A critical feature of the substrate is its ability to locate
// global configurations and actively scan all local workspaces
// to compute aggregate states, like finding active tasks across
// multiple concurrent project directories.

func extractRevision(info *debug.BuildInfo) (string, bool) {
	var revision string
	var modified bool
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			revision = setting.Value
		}
		if setting.Key == "vcs.modified" && setting.Value == "true" {
			modified = true
		}
	}
	return revision, modified
}

// extractBuildInfoVersion attempts to parse Go's embedded vcs.revision metadata.
// It relies on Go 1.18+ debug module information injected during compilation.
func extractBuildInfoVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}

	revision, modified := extractRevision(info)
	if revision == "" {
		return ""
	}

	if len(revision) > 7 {
		revision = revision[:7]
	}
	if modified {
		revision += "-dirty"
	}
	return revision
}

// GetNomosVersion attempts to resolve the active version of the Nomos engine
// by checking embedded metadata first, and falling back to a runtime git describe.
// It formats the version as vX.Y.Z-dev.hash for untagged commits.
func GetNomosVersion() string {
	raw := ""
	if v := extractBuildInfoVersion(); v != "" {
		raw = v
	} else {
		// Fallback for edge cases where the binary was not built with module info
		// or was stripped of metadata.
		exe, err := os.Executable()
		if err == nil {
			dir := filepath.Dir(filepath.Dir(exe))
			cmd := exec.Command("git", "describe", "--tags", "--always")
			cmd.Dir = dir
			if out, err := cmd.Output(); err == nil {
				raw = string(out)
			}
		}
	}

	if raw == "" {
		return "dev"
	}

	raw = strings.TrimSpace(raw)
	// Match git describe output format: v1.1.0-319-gc1b1763 or with -dirty
	re := regexp.MustCompile(`^(v\d+\.\d+\.\d+)-(\d+)-g([a-f0-9]+)(-dirty)?$`)
	if match := re.FindStringSubmatch(raw); match != nil {
		return match[1] + "-dev." + match[3] + match[4]
	}

	// Match Go pseudo-version format: v1.1.1-0.20260806054701-b88fbfa5bf4a
	rePseudo := regexp.MustCompile(`^(v\d+\.\d+\.\d+)-0\.\d+-([a-f0-9]+)(?:\+dirty)?$`)
	if match := rePseudo.FindStringSubmatch(raw); match != nil {
		hash := match[2]
		if len(hash) > 7 {
			hash = hash[:7] // Truncate to short hash for consistency
		}
		if strings.HasSuffix(raw, "+dirty") {
			hash += "-dirty"
		}
		return match[1] + "-dev." + hash
	}

	// Fallback: If it's just a short hash because there are no tags
	reHash := regexp.MustCompile(`^[a-f0-9]{7,40}(-dirty)?$`)
	if reHash.MatchString(raw) {
		return "dev." + raw
	}

	return raw
}

// getRepoColor generates a deterministic hex color code from a repository root string.
// It prioritizes a manual override in .nomos/state/.repo_color, then a hardcoded visual palette,
// falling back to crc32 hashing.
func getRepoColor(root string) string {
	// First, check if the user has explicitly randomized the workspace color using `nomos ide color random`.
	// This state is stored locally within the repository's `.nomos/state/` folder to avoid committing personal preferences.
	colorPath := filepath.Join(workspace.MustNewContext(root).StateDir(), ".repo_color")
	if data, err := os.ReadFile(colorPath); err == nil && len(data) > 0 {
		return strings.TrimSpace(string(data))
	}

	// If no manual override is found, fall back to the deterministic mapping based on the repository directory name.
	// We extract just the base folder name (e.g., 'nomos-commons') to determine the color.
	baseName := filepath.Base(root)
	switch baseName {
	case "nomos-commons":
		return "#238636" // GitHub Green for the OS backbone
	case "nomos-cockpit":
		return "#8b5cf6" // Vibrant Violet for the control plane dashboard
	case "sophialabs":
		return "#3b82f6" // Brand Blue for the primary ecosystem root
	}

	// For all other dynamically generated or unmapped repositories, generate a deterministic fallback color
	// by hashing the base name string. This ensures the same repository always gets the same color.

	h := crc32.ChecksumIEEE([]byte(baseName))
	r := (h & 0xFF0000) >> 16
	g := (h & 0x00FF00) >> 8
	b := (h & 0x0000FF)
	r = (r % 60) + 20 // Dark muted colors for the activity bar
	g = (g % 60) + 20
	b = (b % 60) + 30
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// readVSCodeSettings loads existing VSCode settings from disk or returns an empty map.
func readVSCodeSettings(settingsPath string) map[string]interface{} {
	settings := make(map[string]interface{})
	if _, err := os.Stat(settingsPath); err == nil {
		if data, err := os.ReadFile(settingsPath); err == nil && len(data) > 0 {
			_ = json.Unmarshal(data, &settings)
		}
	}
	return settings
}

func findActivePhaseAcrossProjects() (string, string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", ""
	}
	projectsDir := filepath.Join(home, "Projects")

	var activePhase, activeTask string
	_ = filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		name := info.Name()
		if name == "node_modules" || name == ".git" || name == "archive" {
			return filepath.SkipDir
		}
		if name == ".nomos" {
			repoRoot := filepath.Dir(path)
			if p, t, found := checkActivePhaseInRepo(repoRoot); found {
				activePhase, activeTask = p, t
				return filepath.SkipAll
			}
			return filepath.SkipDir
		}
		return nil
	})
	return activePhase, activeTask
}

// checkActivePhaseInRepo evaluates a single repository for an active phase state.
func checkActivePhaseInRepo(repoRoot string) (string, string, bool) {
	phasePath := filepath.Join(repoRoot, ".nomos", "data", "state", ".phase_state.json")
	data, err := os.ReadFile(phasePath)
	if err != nil {
		return "", "", false
	}
	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		return "", "", false
	}
	p, _ := state["current_phase"].(string)
	t, _ := state["task_id"].(string)
	if t != "" && p != string(statepkg.PhaseIdle) {
		return p, t, true
	}
	return "", "", false
}

// resolveSubstratePhase evaluates phase names and active task ID based on local workspace state file and locked flag.
func resolveSubstratePhase(root string, locked bool) (string, string) {
	phaseName := string(statepkg.PhaseIdle)
	taskId := ""

	phasePath := workspace.MustNewContext(root).NomosStatePath(".phase_state.json")
	if data, err := os.ReadFile(phasePath); err == nil {
		var state map[string]interface{}
		if json.Unmarshal(data, &state) == nil {
			if p, ok := state["current_phase"].(string); ok {
				phaseName = p
			}
			if t, ok := state["task_id"].(string); ok {
				taskId = t
			}
		}
	}

	if !locked && phaseName != string(statepkg.PhaseIdle) {
		phaseName = string(statepkg.PhaseEdit)
	}

	return phaseName, taskId
}

// writeVSCodeTitleFile updates window.title setting in target .vscode/settings.json file.
// It loads existing settings, purges deprecated color customizations, and constructs the title bar payload.
func writeVSCodeTitleFile(targetDir string, baseName string, taskId string, emoji string, phaseName string, ver string) {
	// Construct settings path for target repository context
	settingsPath := filepath.Join(targetDir, ".vscode", "settings.json")
	settings := readVSCodeSettings(settingsPath)
	// Remove deprecated color customizers to maintain crisp visual aesthetics
	delete(settings, "workbench.colorCustomizations")
	// Format dynamic window title string containing active task key, phase emoji, and Nomos binary version
	if taskId != "" {
		settings["window.title"] = fmt.Sprintf("[%s | Task %s] %s PHASE: %s | Nomos %s | ${activeEditorShort}", baseName, taskId, emoji, phaseName, ver)
	} else {
		settings["window.title"] = fmt.Sprintf("%s - Antigravity IDE | %s PHASE: %s | Nomos %s | ${activeEditorShort}", baseName, emoji, phaseName, ver)
	}
	// Create parent .vscode directory if missing
	_ = os.MkdirAll(filepath.Dir(settingsPath), 0755)
	data, _ := json.MarshalIndent(settings, "", "  ")
	_ = os.WriteFile(settingsPath, data, 0644)
}

// UpdateVSCodeTheme modifies .vscode/settings.json to reflect the current phase color and title for the target workspace.
// It updates the target workspace title strictly based on its own local repository task state.
func UpdateVSCodeTheme(root string, locked bool) error {
	// Resolve active phase name and task ID for local repository
	phaseName, taskId := resolveSubstratePhase(root, locked)

	// Map lock status to visual emoji indicators
	emoji := "🟢"
	if locked {
		emoji = "🛑"
	}
	ver := GetNomosVersion()
	baseName := filepath.Base(root)

	// Write title strictly to target workspace directory
	writeVSCodeTitleFile(root, baseName, taskId, emoji, phaseName, ver)

	if strings.Contains(root, "worktrees") {
		projectName := filepath.Base(workspace.MustNewContext(root).DataDir())
		parentRoot := workspace.MustNewContext(root).RepoRoot
		if parentRoot != root {
			writeVSCodeTitleFile(parentRoot, projectName, taskId, emoji, phaseName, ver)
		}
	}

	return nil
}

// LockSubstrate updates the IDE title bar to indicate locked state.
func LockSubstrate(root string) error {
	return UpdateVSCodeTheme(root, true)
}

// UnlockSubstrate updates the IDE title bar and opens write permissions on build artifacts in EDIT phase.
func UnlockSubstrate(root string) error {
	distDir := filepath.Join(root, "src", "control-plane-ui", "dist")
	_ = filepath.Walk(distDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			_ = os.Chmod(path, 0644)
		}
		return nil
	})
	return UpdateVSCodeTheme(root, false)
}

// ValidateSubstrateTargetPath verifies that a target edit path is within the active workspace boundary or a task worktree.
// It returns a hard substrate permission error if an operation attempts to mutate files in an unauthorized sibling root clone.
func ValidateSubstrateTargetPath(root string, targetPath string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		absTarget = targetPath
	}

	// Target path is located inside the active workspace root directory
	relToRoot, err := filepath.Rel(absRoot, absTarget)
	if err == nil && !strings.HasPrefix(relToRoot, "..") {
		return nil
	}

	// Target path is located inside <repoRoot>/.nomos/data/ or a task-isolated worktree directory
	globalDataDir := workspace.MustNewContext(root).DataDir()
	relToGlobal, err := filepath.Rel(globalDataDir, absTarget)
	if err == nil && !strings.HasPrefix(relToGlobal, "..") {
		return nil
	}

	return fmt.Errorf("Substrate Edit Path Violation: target path '%s' resides outside workspace boundary '%s'. Direct edits across sibling root clones are banned. Execute '/nomos-workspace' to check out a task-isolated worktree.", targetPath, root)
}
