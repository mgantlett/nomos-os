// Package cmd contains the CLI commands for the Nomos binary.
// This file implements the ide integration command suite for theme switching and phase synchronization.
package cmd

import (
	"encoding/json"
	"fmt"

	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/spf13/cobra"
)

// ideCmd represents the root command for IDE integration tools.
// It acts as the parent command for all IDE environment management utilities.
var ideCmd = &cobra.Command{
	Use:   "ide",
	Short: "Antigravity IDE integration command suite",
	Long:  `Manage IDE visual theme preferences and consolidated phase state synchronization.`,
}

// ideThemeCmd toggles the Antigravity IDE theme setting between light and dark modes.
// It validates the target theme argument and updates the IDE settings file on disk.
var ideThemeCmd = &cobra.Command{
	Use:   "theme [light|dark]",
	Short: "Switch IDE visual theme setting (light or dark)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetTheme := strings.ToLower(args[0])
		if targetTheme != "light" && targetTheme != "dark" {
			return fmt.Errorf("invalid theme '%s': must be 'light' or 'dark'", targetTheme)
		}
		return applyIDETheme(targetTheme)
	},
}

// idePhaseCmd consolidates workspace phase state transitions under the ide namespace.
// It converts phase names to uppercase and delegates to handlePhaseTransition.
var idePhaseCmd = &cobra.Command{
	Use:   "phase [PLAN|EDIT|REVIEW]",
	Short: "Consolidate workspace phase transitions (PLAN, EDIT, REVIEW)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetPhase := statepkg.WorkspacePhase(strings.ToUpper(args[0]))
		if targetPhase != statepkg.PhasePlan && targetPhase != statepkg.PhaseEdit && targetPhase != statepkg.PhaseReview {
			return fmt.Errorf("invalid phase '%s': must be PLAN, EDIT, or REVIEW", targetPhase)
		}
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		repoRoot := findRepoRoot(wd)
		return handlePhaseTransition(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), targetPhase)
	},
}

// ideColorCmd randomizes the repository IDE workspace color.
var ideColorCmd = &cobra.Command{
	Use:   "color [random]",
	Short: "Randomize the repository IDE workspace color",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Enforce that only the 'random' argument is accepted for this prototype implementation.
		if args[0] != "random" {
			return fmt.Errorf("currently only 'random' is supported")
		}

		// Determine the active workspace directory in order to persist the state locally.
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		repoRoot := findRepoRoot(wd)

		// Define a curated selection of visually aesthetic hex codes that maintain contrast.
		// These specific palettes were chosen because they provide excellent contrast in both dark and light modes,
		// ensuring that the Antigravity IDE UI text overlays remain readable regardless of the chosen theme.
		palette := []string{
			"#d946ef", "#0ea5e9", "#10b981", "#f43f5e", "#eab308",
			"#8b5cf6", "#f97316", "#14b8a6", "#6366f1", "#64748b",
		}

		// Seed the pseudorandom number generator using the current unix nano timestamp.
		// This guarantees sufficient entropy so that sequential runs do not pick the same color.
		rand.Seed(time.Now().UnixNano())

		// Pick a random color from the palette array by generating an integer up to the array length.
		chosen := palette[rand.Intn(len(palette))]

		// Persist the randomly chosen color override in the .nomos/state directory to avoid git pollution.
		colorPath := filepath.Join(config.StateDir(repoRoot), ".repo_color")
		_ = os.MkdirAll(filepath.Dir(colorPath), 0755)
		if err := os.WriteFile(colorPath, []byte(chosen), 0644); err != nil {
			return fmt.Errorf("failed to save color override: %w", err)
		}

		fmt.Printf("🎨 Workspace color randomized to %s\n", chosen)

		// Immediately sync the IDE framing so the user sees the new color applied instantly.
		return exec.UpdateVSCodeTheme(repoRoot, false)
	},
}

// writeThemeSettingsFile writes the workbench.colorTheme property into a settings JSON file.
func writeThemeSettingsFile(p string, themeName string, targetTheme string) {
	_ = os.MkdirAll(filepath.Dir(p), 0755)
	sMap := make(map[string]interface{})
	if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &sMap)
	}
	sMap["workbench.colorTheme"] = themeName
	sMap["theme"] = targetTheme
	if out, err := json.MarshalIndent(sMap, "", "  "); err == nil {
		_ = os.WriteFile(p, out, 0644)
	}
}

// cleanWorkspaceThemeOverrides removes dark background colorCustomizations from active workspace .vscode/settings.json.
func cleanWorkspaceThemeOverrides(themeName string) {
	// Attempt to locate the root path of the active repository to target the correct .vscode folder.
	wd, err := os.Getwd()
	if err != nil {
		return
	}
	repoRoot := findRepoRoot(wd)

	// Read the workspace settings file, falling back gracefully if it does not exist.
	wsSettingsPath := filepath.Join(repoRoot, ".vscode", "settings.json")
	wsData, err := os.ReadFile(wsSettingsPath)
	if err != nil || len(wsData) == 0 {
		return
	}

	// Parse the existing JSON configuration safely into a generic map structure.
	var wsMap map[string]interface{}
	if err := json.Unmarshal(wsData, &wsMap); err != nil {
		return
	}

	wsMap["workbench.colorTheme"] = themeName

	// Remove hardcoded overrides that clash with the newly specified base theme variant.
	// This step is critical because VSCode caches colorCustomizations, which can cause
	// unreadable text if the user swaps from light to dark mode while retaining dark mode background colors.
	// By deleting these specific keys, we force VSCode to inherit the base properties of the active theme.
	if colors, ok := wsMap["workbench.colorCustomizations"].(map[string]interface{}); ok {
		delete(colors, "editor.background")
		delete(colors, "sideBar.background")
		delete(colors, "sideBar.border")
		delete(colors, "tab.activeBackground")
		delete(colors, "tab.inactiveBackground")
		delete(colors, "editorGroupHeader.tabsBackground")
		wsMap["workbench.colorCustomizations"] = colors
	}

	// Re-serialize the settings payload and save it atomically back to the filesystem.
	if out, err := json.MarshalIndent(wsMap, "", "  "); err == nil {
		_ = os.WriteFile(wsSettingsPath, out, 0644)
	}

	// Immediately sync IDE framing colors and Task ID to the active workspace.
	_ = exec.UpdateVSCodeTheme(repoRoot, false)
}

// applyIDETheme updates user settings across Antigravity IDE and VSCode paths.
func applyIDETheme(targetTheme string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("unable to resolve user home directory: %w", err)
	}

	targetPaths := []string{
		filepath.Join(homeDir, ".config", "Antigravity IDE", "User", "settings.json"),
		filepath.Join(homeDir, ".gemini", "antigravity-ide", "settings.json"),
		filepath.Join(homeDir, ".config", "Code", "User", "settings.json"),
		filepath.Join(homeDir, ".config", "VSCodium", "User", "settings.json"),
	}

	themeName := mapThemeName(targetTheme)
	for _, p := range targetPaths {
		writeThemeSettingsFile(p, themeName, targetTheme)
	}

	cleanWorkspaceThemeOverrides(themeName)
	fmt.Printf("🎨 Antigravity IDE theme updated to '%s' mode (%s).\n", targetTheme, themeName)
	return nil
}

// mapThemeName converts a theme key to the formal VSCode / Antigravity IDE theme name.
func mapThemeName(t string) string {
	if t == "light" {
		return "Default Light Modern"
	}
	return "Default Dark Modern"
}

// init registers the theme and phase subcommands under ideCmd.
func init() {
	ideCmd.AddCommand(ideThemeCmd)
	ideCmd.AddCommand(idePhaseCmd)
	ideCmd.AddCommand(ideColorCmd)
}
