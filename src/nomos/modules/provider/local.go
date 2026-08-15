package provider

import (
	"fmt"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/env"
)

// StartLocalDaemon starts the llama-server inference daemon using the unified env module.
func StartLocalDaemon(ctx *workspace.WorkspaceContext, daemon string) error {
	svc, err := env.ResolveService(ctx, daemon)
	if err != nil {
		return fmt.Errorf("local daemon start error: %w", err)
	}

	if err := env.Start(ctx, svc.Name, svc.LogFile, svc.Command, svc.Cwd); err != nil {
		return fmt.Errorf("env start failed: %w", err)
	}

	return nil
}

// StopLocalDaemon stops the given local daemon via the unified env module.
func StopLocalDaemon(ctx *workspace.WorkspaceContext, daemon string) error {
	// Let's resolve the service just to get its canonical PM2 name.
	svc, err := env.ResolveService(ctx, daemon)
	if err != nil {
		return fmt.Errorf("local daemon stop error: %w", err)
	}

	if err := env.Stop(ctx, svc.Name); err != nil {
		return fmt.Errorf("env stop failed: %w", err)
	}
	return nil
}
