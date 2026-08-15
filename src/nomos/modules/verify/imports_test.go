package verify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAuditImports(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "nomos_import_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a dummy .agent/rules/banned_imports.json configuration
	agentDir := filepath.Join(tempDir, ".agent", "rules")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("failed to create .agent/rules dir: %v", err)
	}

	configJSON := `{
		"banned_imports": ["github.com/mgantlett/ado-core"],
		"banned_phrases": ["os/exec"]
	}`
	if err := os.WriteFile(filepath.Join(agentDir, "banned_imports.json"), []byte(configJSON), 0644); err != nil {

		t.Fatalf("failed to write config file: %v", err)
	}

	// 1. Violating Go file
	goFile1 := `package main
import "github.com/mgantlett/ado-core"
`
	if err := os.WriteFile(filepath.Join(tempDir, "file1.go"), []byte(goFile1), 0644); err != nil {
		t.Fatalf("failed to write file1.go: %v", err)
	}

	// 2. Violating Go file (os/exec) inside non-exempt file
	goFile2 := `package main
import "os/exec"
`
	if err := os.WriteFile(filepath.Join(tempDir, "file2.go"), []byte(goFile2), 0644); err != nil {
		t.Fatalf("failed to write file2.go: %v", err)
	}

	// 3. Exempt Go file importing os/exec
	exemptDir := filepath.Join(tempDir, "src", "nomos", "exec")
	if err := os.MkdirAll(exemptDir, 0755); err != nil {
		t.Fatalf("failed to create exempt dir: %v", err)
	}
	goFile3 := `package exec
import "os/exec"
`
	if err := os.WriteFile(filepath.Join(exemptDir, "runner.go"), []byte(goFile3), 0644); err != nil {
		t.Fatalf("failed to write runner.go: %v", err)
	}

	// 4. Violating Nix file
	nixFile := `{ pkgs ? import <nixpkgs> {} }:
{
  shell = import "os/exec";
  core = builtins.import "github.com/mgantlett/ado-core";
}
`
	if err := os.WriteFile(filepath.Join(tempDir, "shell.nix"), []byte(nixFile), 0644); err != nil {
		t.Fatalf("failed to write shell.nix: %v", err)
	}

	// 5. Violating Shell file
	shFile := `#!/bin/bash
source "os/exec"
. "github.com/mgantlett/ado-core"
`
	if err := os.WriteFile(filepath.Join(tempDir, "script.sh"), []byte(shFile), 0644); err != nil {
		t.Fatalf("failed to write script.sh: %v", err)
	}

	files := []string{
		"file1.go",
		"file2.go",
		"src/nomos/modules/exec/runner.go",
		"shell.nix",
		"script.sh",
	}

	violations, err := AuditImports(tempDir, files)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Total expected violations:
	// - file1.go: banned import "github.com/mgantlett/ado-core" (1)
	// - file2.go: banned phrase "os/exec" (2)
	// - shell.nix: builtins.import "github.com/mgantlett/ado-core" (3) and import "os/exec" (4)
	// - script.sh: source "os/exec" (5) and . "github.com/mgantlett/ado-core" (6)
	if len(violations) != 6 {
		t.Errorf("expected 6 violations, got %d: %v", len(violations), violations)
	}
}
