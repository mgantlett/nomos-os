package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/ast"
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/spf13/cobra"
)

func getWorkspaceGraph(cmd *cobra.Command) (*ast.DependencyGraph, string) {
	workspaceRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error getting current working directory: %v\n", err)
		os.Exit(1)
	}
	g, err := ast.BuildDependencyGraph(workspaceRoot)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Error building graph: %v\n", err)
		os.Exit(1)
	}
	return g, workspaceRoot
}

func requireArgs(cmd *cobra.Command, args []string, msg string) {
	if len(args) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), msg)
		os.Exit(1)
	}
}

var astCmd = &cobra.Command{
	Use:   "ast [file]",
	Short: "Parse supported source files and extract symbols as JSON",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		res, err := ast.ParseFile(args[0])
		if err != nil {
			errJSON, _ := json.Marshal(map[string]string{"error": err.Error()})
			cmd.Println(string(errJSON))
			os.Exit(1)
		}
		resJSON, _ := json.MarshalIndent(res, "", "  ")
		cmd.Println(string(resJSON))
	},
}

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Perform dependency graph analysis",
}

var graphShowCmd = &cobra.Command{
	Use:   "show [file]",
	Short: "Renders ASCII dependency tree starting at <file>",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		g, _ := getWorkspaceGraph(cmd)
		if _, ok := g.Graph[args[0]]; !ok {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error: Target module '%s' not found in internal project files.\n", args[0])
			os.Exit(1)
		}
		lines := ast.RenderAsciiTree(args[0], g)
		for _, line := range lines {
			cmd.Println(line)
		}
	},
}

var graphCyclesCmd = &cobra.Command{
	Use:   "cycles",
	Short: "Scans project modules and lists all circular dependencies",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		g, _ := getWorkspaceGraph(cmd)
		cycles := ast.DetectCycles(g)
		if len(cycles) == 0 {
			cmd.Println("🎉 Zero circular imports found! Code is perfectly acyclic.")
		} else {
			cmd.Printf("⚠️  Detected %d Circular Import Loop(s):\n", len(cycles))
			for idx, cycle := range cycles {
				cmd.Printf("  [%d] %s\n", idx+1, strings.Join(cycle, " -> "))
			}
			os.Exit(1)
		}
	},
}

var graphVisualCmd = &cobra.Command{
	Use:   "visual [output_html]",
	Short: "Generates interactive HTML D3.js visualization",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		g, workspaceRoot := getWorkspaceGraph(cmd)
		outPath := filepath.Join(workspace.MustNewContext(workspaceRoot).TmpDir(), "dependency_graph.html")
		if len(args) > 0 {
			outPath = args[0]
		}
		cycles := ast.DetectCycles(g)
		err := ast.GenerateInteractiveHtml(g, cycles, outPath)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error generating visual graph: %v\n", err)
			os.Exit(1)
		}
		cmd.Printf("✅ AST Dependency Graph visualization generated: %s\n", outPath)
	},
}

var graphBlastRadiusCmd = &cobra.Command{
	Use:   "blast-radius [modified_files...]",
	Short: "Traverses backwards to print all affected files",
	Run: func(cmd *cobra.Command, args []string) {
		requireArgs(cmd, args, "Error: Please provide at least one modified file path.")
		g, workspaceRoot := getWorkspaceGraph(cmd)

		var relFiles []string
		for _, f := range args {
			relF := f
			if filepath.IsAbs(f) {
				if r, err := filepath.Rel(workspaceRoot, f); err == nil {
					relF = r
				}
			}
			relFiles = append(relFiles, relF)
		}

		affected := ast.BlastRadius(relFiles, g)
		for _, f := range affected {
			cmd.Println(f)
		}
	},
}

var graphFindTestsCmd = &cobra.Command{
	Use:   "find-tests [modified_files...]",
	Short: "Find BATS tests mapping to the affected modules",
	Run: func(cmd *cobra.Command, args []string) {
		requireArgs(cmd, args, "Error: Please provide at least one modified file path.")
		g, workspaceRoot := getWorkspaceGraph(cmd)
		tests, err := ast.FindTests(args, workspaceRoot, g)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Error finding tests: %v\n", err)
			os.Exit(1)
		}
		if len(tests) > 0 {
			cmd.Println(strings.Join(tests, " "))
		}
	},
}

func init() {
	graphCmd.AddCommand(graphShowCmd)
	graphCmd.AddCommand(graphCyclesCmd)
	graphCmd.AddCommand(graphVisualCmd)
	graphCmd.AddCommand(graphBlastRadiusCmd)
	graphCmd.AddCommand(graphFindTestsCmd)
}
