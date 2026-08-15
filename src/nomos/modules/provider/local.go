package provider

import (
	"fmt"

	"github.com/mgantlett/nomos-os/src/nomos/modules/env"
)

// StartLocalDaemon starts the llama-server inference daemon using the unified env module.
func StartLocalDaemon(repoRoot, daemon string) error {
	svc, err := env.ResolveService(repoRoot, daemon)
	if err != nil {
		return fmt.Errorf("local daemon start error: %w", err)
	}

	if err := env.Start(repoRoot, svc.Name, svc.LogFile, svc.Command, svc.Cwd); err != nil {
		return fmt.Errorf("env start failed: %w", err)
	}

	return nil
}

// StopLocalDaemon stops the given local daemon via the unified env module.
func StopLocalDaemon(daemon string) error {
	// Let's resolve the service just to get its canonical PM2 name.
	svc, err := env.ResolveService("", daemon)
	if err != nil {
		return fmt.Errorf("local daemon stop error: %w", err)
	}

	if err := env.Stop("", svc.Name); err != nil {
		return fmt.Errorf("env stop failed: %w", err)
	}
	return nil
}
