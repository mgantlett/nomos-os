package cmd

import (
	"os"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
	"github.com/mgantlett/nomos-os/src/nomos/modules/mcp"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start a Go-native Model Context Protocol (MCP) server over stdin/stdout",
	RunE: func(cmd *cobra.Command, args []string) error {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		repoRoot := findRepoRoot(wd)

		// Disable printing log messages to stdout to prevent corrupting JSON-RPC stream
		os.Stdout = os.NewFile(1, "/dev/stdout")

		ctx := func() *workspace.WorkspaceContext { c, _ := workspace.NewContext(repoRoot); return c }()
		server := mcp.NewServer(ctx, os.Stdin, os.Stdout)

		// Register standard tools
		server.RegisterTool(&mcp.ParseAstTool{})
		server.RegisterTool(&mcp.VerifyDodTool{})
		server.RegisterTool(&mcp.TaskSyncTool{})

		return server.Run()
	},
}

func init() {
	RootCmd.AddCommand(mcpCmd)
}
