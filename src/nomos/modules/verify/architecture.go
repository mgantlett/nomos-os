package verify

import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"fmt"
	"os"
	"os/exec"
	"strings"
)

// runArchitectureCheck enforces the Open Core boundaries by scanning the
// workspace for hardcoded references or imports pointing to enterprise modules,
// and ensures Cockpit UI master SSoT synchronization from Sovereign repo.
// It executes two separate verification gates: banned imports check and banned
// monolithic paths check. If any check fails, it blocks the commit to prevent
// leaks of proprietary enterprise code into the open-source repository.
func runArchitectureCheck(ctx *workspace.WorkspaceContext) (StageResult, error) {
	root := ctx.RepoRoot
	// 1. Audit Cockpit UI SSoT synchronization
	_, _ = SyncCockpitUI(root)

	// 2. Check for banned enterprise imports
	if errStr := checkBannedImports(root); errStr != "" {
		return StageResult{
			Name:    "Open Core Architecture Check",
			Passed:  false,
			Message: errStr,
		}, nil
	}

	// 3. Check for banned legacy monolithic paths
	if errStr := checkBannedPaths(root); errStr != "" {
		return StageResult{
			Name:    "Open Core Architecture Check",
			Passed:  false,
			Message: errStr,
		}, nil
	}

	return StageResult{
		Name:    "Open Core Architecture Check",
		Passed:  true,
		Message: "Open Core boundaries and SSoT UI synchronized.",
	}, nil
}

// checkBannedImports scans the workspace for prohibited enterprise Go module imports.
// It retrieves the Go module name of the current workspace and performs a recursive
// git grep to discover references to enterprise modules like nomos-gitbrain or
// nomos-swarm. If the current module is the Sovereign monorepo itself, the check is
// bypassed as those enterprise dependencies are native to its compilation context.
func checkBannedImports(root string) string {
	bannedImports := []string{
		"github.com/mgantlett/nomos-gitbrain",
		"github.com/mgantlett/nomos-swarm",
	}

	// Query current module context using list command.
	modCmd := exec.Command("go", "list", "-m")
	modCmd.Dir = root
	outBytes, _ := modCmd.Output()
	currentMod := strings.TrimSpace(string(outBytes))

	// Bypass banned imports validation if we are verifying the Sovereign repository itself.
	if strings.Contains(currentMod, "github.com/mgantlett/nomos-sovereign") {
		return ""
	}

	// Loop over each defined banned import and scan the workspace.
	for _, banned := range bannedImports {
		// If the current module matches the banned import, skip to avoid self-reference.
		if banned == currentMod {
			continue
		}
		// Run git grep to discover references to the banned module, ignoring verify module itself.
		grepCmd := exec.Command("git", "grep", "-I", "-n", banned, "--", ":!src/nomos/modules/verify/architecture.go")
		grepCmd.Dir = root
		out, err := grepCmd.Output()
		if err == nil && len(out) > 0 {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			return fmt.Sprintf("Found banned enterprise import '%s' in %d location(s).", banned, len(lines))
		}
	}
	return ""
}

// checkBannedPaths scans the workspace for prohibited monolithic module paths.
// It verifies that open-source code does not reference any files in monolithic
// enterprise paths such as src/nomos/modules/gitbrain or src/nomos/modules/swarm.
// This check is skipped when verifying the Sovereign monorepo because those paths
// are fully integrated and valid in the enterprise build context.
func checkBannedPaths(root string) string {
	// Query the current project Go module name from the root environment.
	modCmd := exec.Command("go", "list", "-m")
	modCmd.Dir = root
	outBytes, _ := modCmd.Output()
	currentMod := strings.TrimSpace(string(outBytes))

	// Skip monolithic path analysis if the module is the Sovereign Monorepo.
	if strings.Contains(currentMod, "github.com/mgantlett/nomos-sovereign") {
		return ""
	}

	bannedPaths := []string{
		"src/nomos/modules/gitbrain",
		"src/nomos/modules/swarm",
	}

	// Loop over all prohibited monolithic paths to scan for invalid local dependencies.
	for _, banned := range bannedPaths {
		// Execute git grep to check if code references monolithic enterprise folder structures.
		grepCmd := exec.Command("git", "grep", "-I", "-n", banned, "--", ":!src/nomos/modules/verify/architecture.go")
		grepCmd.Dir = root
		out, err := grepCmd.Output()
		if err == nil && len(out) > 0 {
			// Extract lines and format architecture boundary error message.
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			return fmt.Sprintf("Found banned monolithic path reference '%s' in %d location(s).", banned, len(lines))
		}
	}
	return ""
}

// SyncCockpitUI synchronizes the master Sovereign control-plane-ui index.html
// to Open Core Commons ui/index.html when private/nomos-sovereign is present.
func SyncCockpitUI(root string) (string, error) {
	// Paths to Sovereign Master index.html and Commons Open Core target index.html
	masterPath := "/home/markg/Projects/sophialabs/private/nomos-sovereign/src/nomos-cockpit/src/control-plane-ui/index.html"
	targetPath := "/home/markg/Projects/sophialabs/open/nomos-commons/src/nomos/modules/cockpit/ui/index.html"

	// If Sovereign repository master index.html does not exist, return gracefully
	masterBytes, err := os.ReadFile(masterPath)
	if err != nil {
		return "Open Core Cockpit UI self-contained (Sovereign master template not present).", nil
	}

	targetBytes, err := os.ReadFile(targetPath)
	if err == nil && string(masterBytes) == string(targetBytes) {
		return "Cockpit UI SSoT up to date.", nil
	}

	// Synchronize Master template bytes to target Open Core index.html
	if err := os.WriteFile(targetPath, masterBytes, 0644); err != nil {
		return "", fmt.Errorf("failed to sync Cockpit SSoT UI: %w", err)
	}

	return "Cockpit UI SSoT synchronized successfully from Sovereign master template.", nil
}
