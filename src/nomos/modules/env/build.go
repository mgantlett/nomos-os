// Package env abstracts system-level daemons and background processes.
package env

import (
	"fmt"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
	nexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
)

// Build triggers the build sequence for a resolved service configuration.
// It executes the associated BuildCommand synchronously in the workspace root.
func Build(ctx *workspace.WorkspaceContext, svc *ServiceConfig) error {
	repoRoot := ctx.RepoRoot
	if svc.BuildCommand == "" {
		return fmt.Errorf("service '%s' does not have a build command configured", svc.Name)
	}

	dbPath := config.ResolveCacheDbPath(repoRoot)

	// Execute the build command synchronously using standard execution logic.
	// We map stdout/stderr directly so the user sees compiler output in real time.
	out, err := nexec.RunCommand(dbPath, repoRoot, "bash", "-c", svc.BuildCommand)
	if out != "" {
		fmt.Println(out)
	}
	if err != nil {
		return fmt.Errorf("build failed for %s: %w", svc.Name, err)
	}

	return nil
}
