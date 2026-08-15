package cmd

import (
	"os"

	"github.com/mgantlett/nomos-os/src/nomos/modules/integration"
	"github.com/spf13/cobra"
)

var (
	browserCmd = &cobra.Command{
		Use:   "browser",
		Short: "Interact with visual browser automation bridges via Conduit",
	}

	browserStatusCmd = &cobra.Command{
		Use:   "status",
		Short: "Check visual browser connection status",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			repoRoot := findRepoRoot(wd)
			return integration.RunConduit(repoRoot, []string{"status"})
		},
	}

	browserOpenCmd = &cobra.Command{
		Use:   "open [url]",
		Short: "Open target URL in browser page tab",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			url := args[0]
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			repoRoot := findRepoRoot(wd)
			return integration.RunConduit(repoRoot, []string{"tabs", "open", url})
		},
	}

	browserReadCmd = &cobra.Command{
		Use:   "read",
		Short: "Read raw text content from active browser page tab",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			repoRoot := findRepoRoot(wd)
			return integration.RunConduit(repoRoot, []string{"read", "page"})
		},
	}

	browserScreenshotCmd = &cobra.Command{
		Use:   "screenshot",
		Short: "Capture visual screenshot from active browser page tab",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			repoRoot := findRepoRoot(wd)
			return integration.RunConduit(repoRoot, []string{"screenshot"})
		},
	}

	browserTabsCmd = &cobra.Command{
		Use:   "tabs",
		Short: "List all active browser page tabs",
		RunE: func(cmd *cobra.Command, args []string) error {
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			repoRoot := findRepoRoot(wd)
			return integration.RunConduit(repoRoot, []string{"tabs", "list"})
		},
	}

	browserCloseCmd = &cobra.Command{
		Use:   "close [tab-id]",
		Short: "Close target browser page tab",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tabId := args[0]
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			repoRoot := findRepoRoot(wd)
			return integration.RunConduit(repoRoot, []string{"tabs", "close", tabId})
		},
	}

	browserExecCmd = &cobra.Command{
		Use:   "exec [tab-id] [js-code]",
		Short: "Execute custom javascript in target browser page tab",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			tabId := args[0]
			code := args[1]
			wd, err := os.Getwd()
			if err != nil {
				return err
			}
			repoRoot := findRepoRoot(wd)
			return integration.RunConduit(repoRoot, []string{"exec", tabId, code})
		},
	}
)

func init() {
	browserCmd.AddCommand(browserStatusCmd)
	browserCmd.AddCommand(browserOpenCmd)
	browserCmd.AddCommand(browserReadCmd)
	browserCmd.AddCommand(browserScreenshotCmd)
	browserCmd.AddCommand(browserTabsCmd)
	browserCmd.AddCommand(browserCloseCmd)
	browserCmd.AddCommand(browserExecCmd)
}
