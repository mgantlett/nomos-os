package harness

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/mgantlett/nomos-os/src/nomos/modules/gitops"
)

// Driver controls the Deterministic Substrate Harness loop.
// It manages the execution of OpenCode and the dynamic feedback loop.
type Driver struct {
	WorktreeDir string
}

// NewDriver instantiates a new Driver for the Deterministic Harness loop.
func NewDriver(worktreeDir string) *Driver {
	return &Driver{WorktreeDir: worktreeDir}
}

// ExecuteOpenCode runs the opencode agent with a specific message.
func (d *Driver) ExecuteOpenCode(ctx context.Context, message string) error {
	cmd := exec.CommandContext(ctx, "nix-shell", "-p", "opencode", "--run", fmt.Sprintf("opencode --message %q", message))
	if d.WorktreeDir != "" {
		cmd.Dir = d.WorktreeDir
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunNomosLoop runs opencode and verifies it, dynamically continuing as long as errors change.
// It feeds compiler and test errors back to the agent until convergence or a complete stall.
func (d *Driver) RunNomosLoop(initialPrompt string) error {
	fmt.Println("🚀 Starting Dynamic Nomos Harness Loop with OpenCode")
	ctx := context.Background()

	if err := d.ExecuteOpenCode(ctx, initialPrompt); err != nil {
		fmt.Printf("Initial OpenCode run failed (this is expected if it left broken code): %v\n", err)
	}

	consecutiveIdenticalErrors := 0
	lastErrorSig := ""

	for {
		// Run sandbox verification. The sandbox isolates the execution
		// to prevent the LLM from running harmful commands on the host system.
		verifyErr := d.RunSandboxedCrucible()
		if verifyErr == nil {
			fmt.Println("✅ Convergence Achieved! Code passed all deterministic gates.")

			// Natively invoke AI-AI DDP Direct Merge
			fmt.Println("🚀 Triggering autonomous GitOps sync...")
			repoRoot := d.WorktreeDir // Normally we would traverse up to find the root, but passing it down is fine for now
			if err := gitops.DirectMerge(d.WorktreeDir, repoRoot, "develop", ""); err != nil {
				return fmt.Errorf("autonomous merge failed: %w", err)
			}
			return nil
		}

		errMsg := verifyErr.Error()

		// Truncate error message if too long to avoid token bloat
		if len(errMsg) > 2000 {
			errMsg = errMsg[:2000] + "\n... (truncated)"
		}

		// Check for stall
		if errMsg == lastErrorSig {
			consecutiveIdenticalErrors++
		} else {
			consecutiveIdenticalErrors = 0
		}
		lastErrorSig = errMsg

		// If the error persists after 3 attempts, abort to prevent infinite loops
		if consecutiveIdenticalErrors >= 3 {
			fmt.Println("<ESCALATION_REQUIRED>")
			return fmt.Errorf("AGENT STALLED: OpenCode hit the exact same deterministic error 3 times in a row. Aborting.\n%s", errMsg)
		}

		fmt.Printf("⚠️ Crucible Failed. Feeding errors back to OpenCode (Stall counter: %d/3)...\n", consecutiveIdenticalErrors)
		// Format a structured Nomos prompt payload containing the raw diagnostic output
		// and feed it directly back into OpenCode's message argument so the LLM
		// can correct its previous mistakes.
		feedback := fmt.Sprintf("Your previous patch failed deterministic verification. Fix these errors:\n\n%s", errMsg)

		if err := d.ExecuteOpenCode(ctx, feedback); err != nil {
			fmt.Printf("OpenCode feedback run failed: %v\n", err)
		}
	}
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
