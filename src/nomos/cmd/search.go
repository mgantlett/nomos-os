package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/gitbrain"
	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
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

			dbPath := config.ResolveGitBrainDbPath(repoRoot)

			results := []map[string]interface{}{}

			// 1. Open Source Native GitBrain Semantic Search
			synapse.Info("Running native semantic search via GitBrain...\n")
			matches, err := gitbrain.SemanticSearch(dbPath, repoRoot, query, searchLimit)
			if err == nil && len(matches) > 0 {
				synapse.Info("Found %d semantic memories:\n", len(matches))
				for _, match := range matches {
					fmt.Printf("[Memory] (Score: %.2f) %s\n", match.Score, match.Content)
					results = append(results, map[string]interface{}{"match": match.Content, "type": "memory", "score": match.Score})
				}
			} else {
				synapse.Info("No semantic memories found. Falling back to codebase grep...\n")
			}

			// 2. Fallback to git grep
			// NOM-59: Use --cached to support searching through sparse checkouts
			grepCmd := exec.Command("git", "grep", "--cached", "-i", "-I", "--line-number", query)
			grepCmd.Dir = repoRoot
			out, _ := grepCmd.Output()

			lines := strings.Split(string(out), "\n")
			grepCount := 0
			for _, line := range lines {
				if line == "" {
					continue
				}
				if grepCount >= searchLimit && searchLimit > 0 {
					break
				}
				fmt.Printf("[Code] %s\n", line)
				results = append(results, map[string]interface{}{"match": line, "type": "code"})
				grepCount++
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
