package harness

import (
	"fmt"
	"strings"

	"github.com/mgantlett/nomos-os/src/nomos/modules/exec"
)

// Driver controls the Deterministic Substrate Harness loop.
// It manages the execution of NCode and the dynamic feedback loop.
type Driver struct {
	WorktreeDir string
}

// NewDriver instantiates a new Driver for the Deterministic Harness loop.
func NewDriver(worktreeDir string) *Driver {
	return &Driver{WorktreeDir: worktreeDir}
}


// RunSandboxedCrucible runs go vet, go build, and go test in the workspace.
func (d *Driver) RunSandboxedCrucible() error {
	dir := d.WorktreeDir
	if dir == "" {
		dir = "."
	}

	// 1. go vet
	// Enforce strict static analysis to catch dead code and formatting errors.
	cmdVet := exec.Command("nix-shell", "-p", "go", "--run", "go vet ./...")
	cmdVet.Dir = dir
	outVet, errVet := cmdVet.CombinedOutput()
	if errVet != nil {
		return fmt.Errorf("GO_VET_FAILURE:\n%s", strings.TrimSpace(string(outVet)))
	}

	// 2. go build
	// Ensure the generated code compiles successfully into a valid binary.
	cmdBuild := exec.Command("nix-shell", "-p", "go", "--run", "go build ./...")
	cmdBuild.Dir = dir
	outBuild, errBuild := cmdBuild.CombinedOutput()
	if errBuild != nil {
		return fmt.Errorf("COMPILER_ERROR:\n%s", strings.TrimSpace(string(outBuild)))
	}

	// 3. go test
	// Execute all test cases to verify the logical correctness of the agent's work.
	cmdTest := exec.Command("nix-shell", "-p", "go", "--run", "go test ./...")
	cmdTest.Dir = dir
	outTest, errTest := cmdTest.CombinedOutput()
	if errTest != nil {
		return fmt.Errorf("TEST_FAILURE:\n%s", strings.TrimSpace(string(outTest)))
	}

	return nil
}
