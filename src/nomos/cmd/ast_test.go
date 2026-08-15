package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgantlett/nomos-commons/src/nomos/core/ast"
)

func TestAstAndGraphCmds(t *testing.T) {
	// Create a temp workspace directory
	tmpDir, err := os.MkdirTemp("", "nomos-cmd-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Save original CWD and change to temp dir
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get wd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change wd: %v", err)
	}
	defer os.Chdir(origWd)

	// Create a dummy Go file
	goContent := `package main
type Item struct {
	ID int
}
func ProcessItem() {}
`
	goFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(goFile, []byte(goContent), 0644); err != nil {
		t.Fatalf("failed to write main.go: %v", err)
	}

	// Create go.mod
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module testapp\n"), 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Test 1: 'nomos ast main.go'
	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)

	// We pass temporary config file to keep the run clean
	tempConfig := filepath.Join(tmpDir, "config.yaml")
	RootCmd.SetArgs([]string{"--config", tempConfig, "ast", "main.go"})

	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("RootCmd.Execute ast failed: %v, output: %s", err, buf.String())
	}

	var res ast.ParserResult
	if err := json.Unmarshal(buf.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse JSON output: %v, output: %s", err, buf.String())
	}

	if res.Language != "go" {
		t.Errorf("expected language 'go', got %q", res.Language)
	}
	if len(res.Symbols) != 2 {
		t.Errorf("expected 2 symbols, got %d", len(res.Symbols))
	}

	// Test 2: 'nomos graph show main.go'
	buf.Reset()
	RootCmd.SetArgs([]string{"--config", tempConfig, "graph", "show", "main.go"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("RootCmd.Execute graph show failed: %v, output: %s", err, buf.String())
	}

	if !strings.Contains(buf.String(), "└── main.go") {
		t.Errorf("expected output to contain 'main.go', got: %s", buf.String())
	}

	// Test 3: 'nomos graph cycles'
	buf.Reset()
	RootCmd.SetArgs([]string{"--config", tempConfig, "graph", "cycles"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("RootCmd.Execute graph cycles failed: %v, output: %s", err, buf.String())
	}

	if !strings.Contains(buf.String(), "Zero circular imports found") {
		t.Errorf("expected success message, got: %s", buf.String())
	}

	// Test 4: 'nomos graph blast-radius main.go'
	buf.Reset()
	RootCmd.SetArgs([]string{"--config", tempConfig, "graph", "blast-radius", "main.go"})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("RootCmd.Execute graph blast-radius failed: %v, output: %s", err, buf.String())
	}

	if !strings.Contains(buf.String(), "main.go") {
		t.Errorf("expected output to contain main.go, got: %s", buf.String())
	}

	// Test 5: 'nomos graph visual'
	buf.Reset()
	htmlPath := filepath.Join(tmpDir, "dependency_graph.html")
	RootCmd.SetArgs([]string{"--config", tempConfig, "graph", "visual", htmlPath})
	if err := RootCmd.Execute(); err != nil {
		t.Fatalf("RootCmd.Execute graph visual failed: %v, output: %s", err, buf.String())
	}

	if _, err := os.Stat(htmlPath); os.IsNotExist(err) {
		t.Fatalf("HTML visualization file was not generated")
	}
}
