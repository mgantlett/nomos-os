package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

// ResolveConduitCLI searches standard paths for the conduit CLI.
func ResolveConduitCLI(ctx *workspace.WorkspaceContext) (string, []string, error) {
	repoRoot := ctx.RepoRoot
	// Look in local node_modules
	localPath := filepath.Join(repoRoot, "node_modules", ".bin", "conduit-cli")
	if fi, err := os.Stat(localPath); err == nil && !fi.IsDir() {
		return localPath, nil, nil
	}

	// Look in host/cli.js in local repo (if applicable)
	localJS := filepath.Join(repoRoot, "host", "cli.js")
	if fi, err := os.Stat(localJS); err == nil && !fi.IsDir() {
		return "node", []string{localJS}, nil
	}

	// Check system PATH
	if path, err := exec.LookPath("conduit-cli"); err == nil {
		return path, nil, nil
	}

	// Check user home Projects
	home, err := os.UserHomeDir()
	if err == nil {
		p1 := filepath.Join(home, "Projects", "Conduit", "host", "cli.js")
		if fi, err := os.Stat(p1); err == nil && !fi.IsDir() {
			return "node", []string{p1}, nil
		}
		p2 := filepath.Join(home, "Conduit", "host", "cli.js")
		if fi, err := os.Stat(p2); err == nil && !fi.IsDir() {
			return "node", []string{p2}, nil
		}
	}

	return "", nil, fmt.Errorf("conduit-cli not found in workspace, PATH, or home projects")
}

// RunConduit executes the resolved conduit-cli with the given arguments and environment.
func RunConduit(ctx *workspace.WorkspaceContext, args []string) error {
	repoRoot := ctx.RepoRoot
	bin, baseArgs, err := ResolveConduitCLI(ctx)
	if err != nil {
		return err
	}

	// Load API key if present
	apiKeyPath := filepath.Join(repoRoot, "data", "api_key.txt")
	if data, err := os.ReadFile(apiKeyPath); err == nil {
		os.Setenv("CONDUIT_API_KEY", string(data))
	}

	finalArgs := append(baseArgs, args...)
	cmd := exec.Command(bin, finalArgs...)
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
