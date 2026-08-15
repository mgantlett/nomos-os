/*
Package cmd provides the command-line interface for the Nomos application.
It uses the Cobra library to define commands, subcommands, and flags.
This file specifically handles the 'audit' subcommand group, which contains
commands for verifying various properties of the codebase such as complexity,
imports, and workflow determinism.
*/
package cmd

import (
	"fmt"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-os/src/nomos/modules/verify"
	"github.com/spf13/cobra"
)

var (
	complexityAll bool
	importsAll    bool

	auditCmd = &cobra.Command{
		Use:   "audit",
		Short: "Perform developer audits on the current workspace",
	}

	complexityCmd = &cobra.Command{
		Use:   "complexity",
		Short: "Audit code complexity and control-flow nesting levels",
		Run: func(cmd *cobra.Command, args []string) {
			root, err := os.Getwd()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to get current working directory: %v\n", err)
				os.Exit(1)
			}

			findings, err := verify.AnalyzeComplexity(root, !complexityAll)
			if err != nil {
				fmt.Fprintf(os.Stderr, "complexity audit failed: %v\n", err)
				os.Exit(1)
			}

			errorsCount := 0
			warningsCount := 0

			synapse.Info("%s", fmt.Sprint("\n▶ 🔍 Nomos Code Complexity & Indentation Nesting Auditor"))
			synapse.Info("%s", fmt.Sprint("────────────────────────────────────────────────────────────"))

			for _, f := range findings {
				status := "⚠️  WARNING"
				if f.IsError {
					status = "❌ VIOLATION"
					errorsCount++
				} else {
					warningsCount++
				}
				if f.Func != "" {
					synapse.Info("  %s: %s (function '%s', line %d) - %s\n", status, f.File, f.Func, f.Line, f.Message)
				} else {
					synapse.Info("  %s: %s (line %d) - %s\n", status, f.File, f.Line, f.Message)
				}
			}

			synapse.Info("%s", fmt.Sprint("────────────────────────────────────────────────────────────"))
			synapse.Info("Audit summary: %d violation(s), %d warning(s)\n", errorsCount, warningsCount)

			if errorsCount > 0 {
				synapse.Info("%s", fmt.Sprint("\n❌ Complexity check failed. Please refactor the code blocks above."))
				os.Exit(1)
			}

			synapse.Info("%s", fmt.Sprint("\n✅ Complexity check passed successfully!"))
		},
	}

	importsCmd = &cobra.Command{
		Use:   "imports",
		Short: "Audit code imports against banned packages and rules",
		Run: func(cmd *cobra.Command, args []string) {
			root, err := os.Getwd()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to get current working directory: %v\n", err)
				os.Exit(1)
			}

			var files []string
			if importsAll {
				err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if !info.IsDir() && strings.HasSuffix(info.Name(), ".go") {
						rel, err := filepath.Rel(root, path)
						if err == nil {
							files = append(files, rel)
						}
					}
					return nil
				})
				if err != nil {
					fmt.Fprintf(os.Stderr, "failed to list files: %v\n", err)
					os.Exit(1)
				}
			} else {
				modifiedMap, err := verify.GetModifiedFiles(root)
				if err != nil {
					fmt.Fprintf(os.Stderr, "failed to get modified files: %v\n", err)
					os.Exit(1)
				}
				for f := range modifiedMap {
					files = append(files, f)
				}
			}

			violations, err := verify.AuditImports(root, files)
			if err != nil {
				fmt.Fprintf(os.Stderr, "import audit failed: %v\n", err)
				os.Exit(1)
			}

			synapse.Info("%s", fmt.Sprint("\n▶ 🔍 Nomos Import & Legacy Code Blocker Auditor"))
			synapse.Info("%s", fmt.Sprint("────────────────────────────────────────────────────────────"))
			for _, v := range violations {
				synapse.Info("  ❌ VIOLATION: %s\n", v)
			}
			synapse.Info("%s", fmt.Sprint("────────────────────────────────────────────────────────────"))
			synapse.Info("Audit summary: %d violation(s)\n", len(violations))

			if len(violations) > 0 {
				synapse.Info("%s", fmt.Sprint("\n❌ Import check failed. Forbidden packages detected."))
				os.Exit(1)
			}

			synapse.Info("%s", fmt.Sprint("\n✅ Import check passed successfully!"))
		},
	}
	// workflowsCmd represents the 'audit workflows' subcommand
	workflowsCmd = &cobra.Command{
		Use:   "workflows",
		Short: "Audit markdown workflows for parity with deterministic CLI definitions",
		Run: func(cmd *cobra.Command, args []string) {
			// Get the current working directory to establish the root context for the parity checks
			root, err := os.Getwd()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to get current working directory: %v\n", err)
				os.Exit(1)
			}

			// Invoke the core logic from verify package to find non-deterministic commands
			discrepancies, err := verify.AuditWorkflows(root)
			if err != nil {
				fmt.Fprintf(os.Stderr, "workflow audit failed: %v\n", err)
				os.Exit(1)
			}

			// Render the CLI dashboard for violations
			synapse.Info("%s", fmt.Sprint("\n▶ 🔍 Nomos Workflow Determinism Auditor"))
			synapse.Info("%s", fmt.Sprint("────────────────────────────────────────────────────────────"))
			for _, d := range discrepancies {
				// Print detailed error locations (file, line number, message) for each invalid command/flag
				synapse.Info("  ❌ VIOLATION: %s (line %d) - %s\n", d.File, d.Line, d.Message)
				synapse.Info("      ↳ Command: %s\n", d.Command)
			}
			synapse.Info("%s", fmt.Sprint("────────────────────────────────────────────────────────────"))
			synapse.Info("Audit summary: %d violation(s)\n", len(discrepancies))

			// Hard fail the command if there are discrepancies to block pipelines/DoD
			if len(discrepancies) > 0 {
				synapse.Info("%s", fmt.Sprint("\n❌ Workflow audit failed. Non-deterministic instructions detected."))
				os.Exit(1)
			}

			synapse.Info("%s", fmt.Sprint("\n✅ Workflow parity check passed successfully!"))
		},
	}
	// wiresCmd represents the 'audit wires' subcommand
	wiresCmd = &cobra.Command{
		Use:   "wires",
		Short: "Audit workspace for unwired API endpoints and dangling HTML DOM elements",
		Run: func(cmd *cobra.Command, args []string) {
			root, err := os.Getwd()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to get current working directory: %v\n", err)
				os.Exit(1)
			}

			report, err := verify.AuditWires(root)
			if err != nil {
				fmt.Fprintf(os.Stderr, "wire audit failed: %v\n", err)
				os.Exit(1)
			}

			synapse.Info("%s", fmt.Sprint("\n▶ 📡 Nomos Unwired Code & Disconnected Wire Auditor"))
			synapse.Info("%s", fmt.Sprint("────────────────────────────────────────────────────────────"))

			for _, f := range report.Findings {
				synapse.Info("  ❌ [%s] %s: %s\n", f.Type, f.File, f.Description)
			}
			synapse.Info("%s", fmt.Sprint("────────────────────────────────────────────────────────────"))
			synapse.Info("Audit summary: %d unwired item(s) found\n", len(report.Findings))

			if !report.Passed {
				synapse.Info("%s", fmt.Sprint("\n❌ Wire audit failed. Unwired API endpoints or DOM containers detected."))
				os.Exit(1)
			}

			synapse.Info("%s", fmt.Sprint("\n✅ All API endpoints and DOM containers are fully wired!"))
		},
	}

	deadCodeAll bool

	deadCodeCmd = &cobra.Command{
		Use:     "dead-code",
		Aliases: []string{"deadcode"},
		Short:   "Audit codebase for unused exported symbols, dead branches, and defensive AI bloat (Alex & Cipher Persona Engine)",
		Run: func(cmd *cobra.Command, args []string) {
			root, err := os.Getwd()
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to get current working directory: %v\n", err)
				os.Exit(1)
			}

			var findings []verify.DeadCodeFinding
			findings, err = verify.AuditDeadCode(root, deadCodeAll)
			if err != nil {
				fmt.Fprintf(os.Stderr, "dead code audit failed: %v\n", err)
				os.Exit(1)
			}

			synapse.Info("%s", fmt.Sprint("\n▶ ✂️ Nomos Dead Code & Defensive AI Bloat Auditor (Alex & Cipher Persona Engine)"))
			synapse.Info("%s", fmt.Sprint("────────────────────────────────────────────────────────────"))

			violationsCount := 0
			warningsCount := 0

			for _, f := range findings {
				status := "⚠️  UNUSED SYMBOL"
				if f.IsViolation {
					status = "❌ UNREACHABLE BRANCH"
					violationsCount++
				} else {
					warningsCount++
				}
				if f.Line > 0 {
					synapse.Info("  %s: %s (line %d) - %s\n", status, f.File, f.Line, f.Message)
				} else {
					synapse.Info("  %s: %s - %s\n", status, f.File, f.Message)
				}
			}

			synapse.Info("%s", fmt.Sprint("────────────────────────────────────────────────────────────"))
			synapse.Info("Audit summary: %d unreachable branch(es), %d unused symbol(s)\n", violationsCount, warningsCount)

			if violationsCount > 0 {
				synapse.Info("%s", fmt.Sprint("\n❌ Dead code check failed. Purge unreachable code or unused symbols."))
				os.Exit(1)
			}

			synapse.Info("%s", fmt.Sprint("\n✅ Dead code check passed! Codebase is tight with zero defensive bloat."))
		},
	}
)

func init() {
	// Initialize local flags for audit subcommands
	complexityCmd.Flags().BoolVar(&complexityAll, "all", false, "Scan all files in the project instead of only staged files")
	importsCmd.Flags().BoolVar(&importsAll, "all", false, "Scan all files in the project instead of only staged files")
	deadCodeCmd.Flags().BoolVar(&deadCodeAll, "all", false, "Scan all files in the project instead of only staged files")

	// Register commands into the audit root command
	auditCmd.AddCommand(complexityCmd)
	auditCmd.AddCommand(importsCmd)
	auditCmd.AddCommand(workflowsCmd)
	auditCmd.AddCommand(wiresCmd)
	auditCmd.AddCommand(deadCodeCmd)
}
