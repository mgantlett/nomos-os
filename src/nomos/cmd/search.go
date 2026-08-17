package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/plugin"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	nomosexec "github.com/mgantlett/nomos-os/src/nomos/modules/exec"
	"github.com/spf13/cobra"
)

var (
	searchLimit int

	searchCmd = &cobra.Command{
		Use:   "search [query]",
		Short: "Search codebase",
		Long:  `Perform a text search across the codebase. (Semantic search requires the GitBrain enterprise module)`,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			query := args[0]

			wd, _ := os.Getwd()
			repoRoot := nomosexec.FindRepoRoot(wd)

			// Try to use enterprise GitBrain for semantic search via Plugin architecture
			plugins, err := plugin.DiscoverPlugins(repoRoot)
			if err == nil {
				for _, p := range plugins {
					if filepath.Base(p) == "nomos-plugin-gitbrain" {
						synapse.Info("Running semantic search via GitBrain...\n")
						out, err := plugin.CallPlugin(p, "search", map[string]string{
							"query": query,
						})
						if err == nil {
							os.Stdout.Write(out)
							return
						}
						break
					}
				}
			}

			// Fallback to git grep
			grepCmd := exec.Command("git", "grep", "-i", "-I", "--line-number", query)
			grepCmd.Dir = repoRoot
			out, _ := grepCmd.Output()

			lines := strings.Split(string(out), "\n")
			if len(lines) > searchLimit && searchLimit > 0 {
				lines = lines[:searchLimit]
			}

			results := []map[string]interface{}{}
			for _, line := range lines {
				if line == "" {
					continue
				}
				results = append(results, map[string]interface{}{"match": line})
			}

			synapse.Emit("SearchResults", map[string]interface{}{
				"scope":   "code",
				"query":   query,
				"results": results,
			})
		},
	}
)

func init() {
	searchCmd.Flags().IntVar(&searchLimit, "limit", 10, "maximum number of search results to return")
}
