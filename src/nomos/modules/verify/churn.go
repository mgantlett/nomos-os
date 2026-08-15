package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
)

// churnResult holds the complexity and frequency data for a single processed file.
// It is used to pass results thread-safely from the worker pool to the main aggregate loop.
type churnResult struct {
	file  string // The relative path to the staged file
	score int    // The final computed risk score (churn * maxCC)
	churn int    // The raw git commit frequency count
	maxCC int    // The raw maximum cyclomatic nesting depth
}

// processChurnWorkerPool executes the parallel churn analysis across all staged files.
// It uses a sync.WaitGroup to bound the number of concurrent git processes to NumCPU.
// This prevents OS-level file descriptor exhaustion or CPU context switching overhead.
func processChurnWorkerPool(root string, files []string) []churnResult {
	var mu sync.Mutex
	jobs := make(chan string, len(files))
	resultsChan := make(chan churnResult, len(files))

	// Determine optimal worker pool size. Bound it to 8 max to avoid fork bombing.
	numWorkers := runtime.NumCPU()
	if numWorkers > 8 {
		numWorkers = 8
	}
	var wg sync.WaitGroup

	// Spin up the worker pool goroutines.
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range jobs {
				resultsChan <- processSingleFileChurn(root, file, &mu)
			}
		}()
	}

	// Feed all staged files into the jobs channel to be picked up by the workers.
	for _, file := range files {
		jobs <- file
	}
	close(jobs)

	// Wait asynchronously for all workers to finish processing the jobs.
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Drain the results channel into a slice to return.
	var finalResults []churnResult
	for r := range resultsChan {
		finalResults = append(finalResults, r)
	}
	return finalResults
}

// processSingleFileChurn handles the audit logic for exactly one file.
// Extracted to reduce cyclomatic complexity of the worker pool loop.
func processSingleFileChurn(root string, file string, mu *sync.Mutex) churnResult {
	ext := filepath.Ext(file)

	// Exclude unsupported extensions from the churn/complexity analysis.
	if !isNestingCheckedExtension(ext) {
		return churnResult{file: file, score: -1}
	}

	// Exclude deleted files
	if _, err := os.Stat(filepath.Join(root, file)); os.IsNotExist(err) {
		return churnResult{file: file, score: -1}
	}

	// Exclude UI code from backend-centric complexity analysis.
	if strings.Contains(file, "/control-plane-ui/") || strings.HasPrefix(file, "src/control-plane-ui/") {
		return churnResult{file: file, score: -1}
	}

	// Check if the current file has a registered legacy code quality debt bypass.
	if bypassed, linkedTask := CheckQualityDebtBypass(root, file, "churn_complexity"); bypassed {
		mu.Lock()
		synapse.Info("   \x1b[32m⏭️  [Quality Debt] Bypassed 'churn_complexity' hotspot for '%s' (Linked to active task #%s)\x1b[0m\n", file, linkedTask)
		mu.Unlock()
		return churnResult{file: file, score: -1}
	}

	// Execute git log process to fetch the churn.
	churn, err := getFileChurnCount(root, file)
	if err != nil || churn == 0 {
		return churnResult{file: file, score: -1}
	}

	// Calculate the max nesting complexity of the file.
	maxCC := getFileMaxNestingComplexity(root, file)

	// Send the final compiled metrics into the results channel.
	return churnResult{file: file, score: churn * maxCC, churn: churn, maxCC: maxCC}
}

// runChurnComplexityAudit implements the main audit gate logic for the Churn-vs-Complexity gate.
// It detects high-risk hotspots in the codebase by correlating commit frequency (churn)
// with the maximum cyclomatic complexity of functions inside modified files.
// Files that change frequently but are simple are usually just configuration or data files.
// Files that are highly complex but rarely change are usually stable core libraries.
// However, files that are BOTH highly complex AND change frequently are major sources of bugs.
// This check flags files crossing the hotspot threshold (Churn * Complexity > Limit).
func runChurnComplexityAudit(root string) (StageResult, error) {
	res := StageResult{Name: "Churn-vs-Complexity Audit", Passed: true}

	// Retrieve list of currently staged files in Git
	files, err := getStagedFiles(root)
	if err != nil {
		return res, nil
	}

	var violations []string
	var stats []string
	var maxScore int

	// Process each modified file to determine risk score using a bounded worker pool
	results := processChurnWorkerPool(root, files)

	// Aggregate all the scores from the parallel worker processes.
	for _, r := range results {
		// Ignore files that were filtered out during the worker phase.
		if r.score == -1 {
			continue
		}

		// Update the highest hotspot score detected in this commit batch.
		if r.score > maxScore {
			maxScore = r.score
		}

		stats = append(stats, fmt.Sprintf("   ↳ %s: Churn = %d, Max Nesting = %d, Hotspot Score = %d", r.file, r.churn, r.maxCC, r.score))

		// 500 is the maximum allowed Hotspot Score limit. Reject if it exceeds.
		if r.score > 500 {
			violations = append(violations, fmt.Sprintf("file '%s' hotspot score %d exceeds limit of 500 (Churn: %d, Max Nesting: %d)", r.file, r.score, r.churn, r.maxCC))
		}
	}

	res.Metrics = map[string]interface{}{
		"max_hotspot_score": maxScore,
	}

	// Flag failures if any high-risk files are detected
	if len(violations) > 0 {
		res.Passed = false
		res.Error = fmt.Errorf("high risk complexity hotspots detected:\n - %s", strings.Join(violations, "\n - "))
	} else {
		res.Message = "All modified files pass hotspot complexity constraints (Score <= 500).\n" + strings.Join(stats, "\n")
	}

	return res, nil
}

// getFileChurnCount runs git log to determine the total commit frequency of a file.
func getFileChurnCount(root, file string) (int, error) {
	// Query git history for list of commits modifying this file
	out, err := runGit(root, "log", "--follow", "--format=oneline", "--since=6.months.ago", "--", file)
	if err != nil {
		return 0, err
	}
	if out == "" {
		return 0, nil
	}
	lines := strings.Split(out, "\n")
	return len(lines), nil
}

// getFileMaxNestingComplexity returns the maximum nesting level of a file.
func getFileMaxNestingComplexity(root, file string) int {
	absPath := filepath.Join(root, file)
	findings, err := analyzeNesting(absPath, file)
	if err != nil || len(findings) == 0 {
		return 1
	}

	maxNesting := 1
	for _, f := range findings {
		if f.Value > maxNesting {
			maxNesting = f.Value
		}
	}
	return maxNesting
}
