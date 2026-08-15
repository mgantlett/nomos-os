/*
Package cmd implements the core command line interface for the Nomos orchestrator.
This file contains the `phase` command, which allows the CLI to request phase transitions
via the Cockpit REST server, falling back to local transitions if the server is down.
*/
package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	statepkg "github.com/mgantlett/nomos-commons/src/nomos/core/state"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/task"
	"github.com/spf13/cobra"
)

// phaseCmd handles transition operations by issuing HTTP REST calls to the Cockpit server.
// It parses the desired phase target, verifies server availability, and falls back to local execution.
// By default, Nomos runs a local Go HTTP server (Cockpit) that synchronizes the frontend IDE UI
// with the underlying backend phase engine. This command bridges the CLI and the HTTP API.
var phaseCmd = &cobra.Command{
	Use:   "phase [phase-name]",
	Short: "Transition phase via the Cockpit RPC server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Convert input argument string to WorkspacePhase typed enum
		phase := statepkg.WorkspacePhase(strings.ToUpper(args[0]))
		if phase != statepkg.PhasePlan && phase != statepkg.PhaseEdit && phase != statepkg.PhaseReview && phase != statepkg.PhaseIdle {
			return fmt.Errorf("invalid phase: %s (must be PLAN, EDIT, REVIEW, or IDLE)", phase)
		}

		// Attempt HTTP POST RPC call to local Cockpit server endpoint.
		url := "http://localhost:8089/api/phase/transition"
		req := map[string]interface{}{
			"phase": string(phase),
		}
		// Marshal phase transition request object to JSON bytes
		bodyBytes, err := json.Marshal(req)
		if err != nil {
			return err
		}

		// Issue HTTP POST payload to local Cockpit server
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(bodyBytes))
		if err != nil {
			// Cockpit server is unreachable: fallback to local state transition engine
			fmt.Println("⚠️ Cockpit server unreachable. Falling back to direct local transition.")
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			repoRoot := findRepoRoot(wd)
			return task.TransitionPhase(func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }(), phase)
		}
		defer resp.Body.Close()

		// Handle HTTP non-200 OK error responses
		if resp.StatusCode != http.StatusOK {
			respBytes, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("phase transition rejected by server: %s", string(respBytes))
		}

		fmt.Printf("✅ Phase transitioned to %s via RPC successfully.\n", phase)
		return nil
	},
}

// Register phaseCmd subcommand with RootCmd entry point
func init() {
	RootCmd.AddCommand(phaseCmd)
}
