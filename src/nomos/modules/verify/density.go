package verify

import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
)

// runCommentDensityCheck performs a check on staged source files to verify comment-to-code density.
// It ensures that all files maintain a minimum of 10% comment lines relative to code lines.
// Files with high logic density but no explanation are notoriously hard to maintain.
// This check enforces that engineers leave breadcrumbs and reasoning for complex implementations.
func runCommentDensityCheck(ctx *workspace.WorkspaceContext) (StageResult, error) {
	root := ctx.RepoRoot
	res := StageResult{Name: "Comment Density Check", Passed: true}

	// Retrieve list of currently staged files
	files, err := getStagedFiles(root)
	if err != nil {
		return res, nil
	}

	var violations []string
	minDensity := 100.0
	globalSpamTracker := make(map[string]int)

	// Check each staged file
	for _, file := range files {
		violation, isViolated, density := checkFileDensity(root, file, globalSpamTracker)
		if isViolated {
			violations = append(violations, violation)
		}
		if density < minDensity {
			minDensity = density
		}
	}

	res.Metrics = map[string]interface{}{
		"min_density_percentage": minDensity,
	}

	// Verify if any files failed the threshold
	if len(violations) > 0 {
		res.Passed = false
		res.Error = fmt.Errorf("comment density constraints violated:\n - %s", strings.Join(violations, "\n - "))
	} else {
		res.Message = "All staged source files pass comment density constraints (>= 10%)."
	}

	return res, nil
}

// isIgnoredUIPath determines if a file path belongs to ignored UI build artifacts or embedded assets.
func isIgnoredUIPath(fSlash string) bool {
	ignoredPrefixes := []string{"dist/", "node_modules/", "src/control-plane-ui/"}
	for _, p := range ignoredPrefixes {
		if strings.HasPrefix(fSlash, p) {
			return true
		}
	}
	ignoredSubstrings := []string{"/dist/", "/node_modules/", "/control-plane-ui/", "/modules/cockpit/ui/"}
	for _, s := range ignoredSubstrings {
		if strings.Contains(fSlash, s) {
			return true
		}
	}
	return false
}

// isExcludedPath checks if a given file path should be ignored during comment density checks.
// We exclude non-logic files, test files, third-party libraries, and generated UI bundles
// to prevent the density scanner from artificially deflating the overall workspace score.
func isExcludedPath(file string) bool {
	ext := filepath.Ext(file)
	validExts := map[string]bool{
		".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
		".py": true, ".rs": true, ".html": true, ".css": true, ".c": true,
		".cpp": true, ".h": true, ".hpp": true, ".java": true, ".sh": true,
		".bash": true, ".nix": true, ".rb": true, ".php": true,
	}
	if !validExts[ext] {
		return true
	}
	if strings.HasSuffix(file, "_test.go") || strings.Contains(file, ".test.") {
		return true
	}
	return isIgnoredUIPath(filepath.ToSlash(file))
}

// checkFileDensity evaluates the comment density for a single file.
// Returns violation message, boolean indicator if it is below the threshold, and the density percentage.
// evaluateDensityThreshold checks if file density meets the 10% threshold or worsens.
func evaluateDensityThreshold(root, file string, density float64, comments, code int) (string, bool) {
	if density >= 10.0 {
		return "", false
	}
	if DidDensityWorsen(root, file, density) {
		msg := fmt.Sprintf("file '%s' has comment density of %.1f%% (comments: %d, code: %d) below minimum of 10%%", file, density, comments, code)
		StageAutoDebtTask(root, file, "comment_density_limit", msg)
		return msg, true
	}
	msg := fmt.Sprintf("file '%s' has comment density of %.1f%% below minimum of 10%% (Bypassed: Structural edit did not worsen density)", file, density)
	synapse.Info("   \x1b[33m⚠️  [Density Warning] %s\x1b[0m\n", msg)
	return "", false
}

// checkFileDensity evaluates the comment density for a single file.
// Returns violation message, boolean indicator if it is below the threshold, and the density percentage.
func checkFileDensity(root, file string, globalSpamTracker map[string]int) (string, bool, float64) {
	if isExcludedPath(file) {
		return "", false, 100.0
	}

	// Check if this file has a registered quality debt bypass for comment density
	bypassed, linkedTask := CheckQualityDebtBypass(root, file, "comment_density_limit")
	if bypassed {
		synapse.Info("   \x1b[32m⏭️  [Quality Debt] Bypassed 'comment_density_limit' for '%s' (Linked to active task #%s)\x1b[0m\n", file, linkedTask)
		return "", false, 100.0
	}

	absPath := filepath.Join(root, file)
	comments, code, err := calculateCommentDensity(absPath, globalSpamTracker)
	if err != nil {
		return fmt.Sprintf("failed to parse file %s: %v", file, err), true, 0.0
	}

	density := (float64(comments) / float64(code)) * 100.0
	msg, failed := evaluateDensityThreshold(root, file, density, comments, code)
	return msg, failed, density
}

// DidDensityWorsen verifies if the current comment density is worse than it was previously in HEAD.
// It acts as a "Boy Scout" fallback: if a developer touches a heavily-undocumented legacy file,
// they are not blocked by the density check unless they actively worsened the ratio.
func DidDensityWorsen(root, file string, currentDensity float64) bool {
	headContent, err := runGit(root, "show", "HEAD:"+file)
	if err != nil {
		return true // New file or error getting HEAD, assume it worsened
	}
	comments, code, err := calculateCommentDensityString(headContent, make(map[string]int))
	if err != nil || code == 0 {
		return true
	}
	headDensity := (float64(comments) / float64(code)) * 100.0
	// If current density is >= head density, it didn't worsen (or it's the same)
	return currentDensity < headDensity
}

// calculateCommentDensity reads a file and parses its comment density metrics.
func calculateCommentDensity(filePath string, globalSpamTracker map[string]int) (int, int, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return 0, 0, err
	}
	return calculateCommentDensityString(string(content), globalSpamTracker)
}

// calculateCommentDensityString counts comment lines and code lines from a string content.
func calculateCommentDensityString(content string, globalSpamTracker map[string]int) (int, int, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	commentLines := 0
	codeLines := 0
	inBlockComment := false
	var recentComments []string
	var pendingComments int
	var commentBuffer string

	// Scan through lines sequentially
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue // Skip blank lines
		}

		isComment, isCode := parseLineComments(line, &inBlockComment)
		if isComment {
			// Strip comment markers to prepare the string for entropy and distance analysis
			// This allows us to compare the core semantic content of comments regardless of their syntax.
			cleanComment := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(line, "//"), "/*"), "<!--"), "#"))
			if len(cleanComment) > 0 {
				recentComments = processComment(cleanComment, recentComments, &pendingComments, &commentBuffer, globalSpamTracker)
			}
		} else if isCode {
			codeLines++

			// AST-Bound Docstring Heuristic:
			// We evaluate any pending block of comments that immediately precedes this line of code.
			// By checking if the code is a recognized declaration (like a func or struct),
			// we can reward proper architectural docstrings while penalizing scattered spam comments.
			if pendingComments > 0 {
				commentLines += applyDocstringHeuristics(line, pendingComments)
				pendingComments = 0
				commentBuffer = ""
			}
		}
	}

	return commentLines, codeLines, nil
}

// processComment checks a new comment against recently seen comments to detect copy-pasted spam.
// It uses a Levenshtein distance threshold to prevent developers from artificially inflating
// their comment density scores by copy-pasting the same block of text repeatedly.
// If the comment is valid, it adds it to the active tracking buffers and increments the pending count.
func processComment(cleanComment string, recentComments []string, pendingComments *int, commentBuffer *string, globalSpamTracker map[string]int) []string {
	isSpam := false

	// Detect cross-file boilerplate spam using a global tracker
	if len(cleanComment) > 15 {
		globalSpamTracker[cleanComment]++
		if globalSpamTracker[cleanComment] > 3 {
			isSpam = true
		}
	}

	// Compare against the sliding window of recent comments
	// A Levenshtein distance < 5 indicates that the strings are functionally identical
	if !isSpam {
		for _, recent := range recentComments {
			if levenshteinDistance(cleanComment, recent) < 5 {
				isSpam = true
				break
			}
		}
	}

	// If the comment is unique enough, process it into the current block state
	if !isSpam {
		*pendingComments++
		*commentBuffer += cleanComment + " "
		recentComments = append(recentComments, cleanComment)

		// Maintain the sliding window size of 5 recent comments
		if len(recentComments) > 5 {
			recentComments = recentComments[1:]
		}
	}
	return recentComments
}

// applyDocstringHeuristics assesses the value of a comment block based on what it precedes.
// Comments that act as docstrings for top-level declarations are counted fully,
// whereas floating or inline comments receive a 50% penalty to discourage inline clutter.
func applyDocstringHeuristics(line string, pendingComments int) int {
	if isDeclaration(line) {
		// Comments are bound to a structural declaration (high value)
		return pendingComments
	}
	// Comments are not bound to a declaration, apply 50% density penalty
	return pendingComments / 2
}

// isDeclaration applies a regex/string heuristic to detect if a code line is a declaration.
func isDeclaration(line string) bool {
	prefixes := []string{"func ", "type ", "class ", "interface ", "export ", "def ", "const ", "let ", "var ", "struct ", "enum "}
	for _, p := range prefixes {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

// minCost returns the minimum of three cost values.
func minCost(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= c {
		return b
	}
	return c
}

// levenshteinDistance calculates the Levenshtein distance between two strings.
// This algorithm computes the minimum number of single-character edits (insertions, deletions, or substitutions)
// required to change one word into the other. We use it to detect spammy copy-paste comments.
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}
	d := make([][]int, len(s1)+1)
	for i := range d {
		d[i] = make([]int, len(s2)+1)
	}
	for i := 0; i <= len(s1); i++ {
		d[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		d[0][j] = j
	}
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}
			d[i][j] = minCost(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
		}
	}
	return d[len(s1)][len(s2)]
}

// isBlockCommentEnd checks if a line closes a block comment.
func isBlockCommentEnd(line string) bool {
	return strings.Contains(line, "*/") || strings.Contains(line, "-->") || strings.Contains(line, "\"\"\"")
}

// isBlockCommentStart checks if a line opens a block comment.
func isBlockCommentStart(line string) bool {
	return strings.HasPrefix(line, "/*") || strings.HasPrefix(line, "<!--") || strings.HasPrefix(line, "\"\"\"")
}

// isSingleLineComment checks if a line starts with a single line comment prefix.
func isSingleLineComment(line string) bool {
	return strings.HasPrefix(line, "//") || strings.HasPrefix(line, "#")
}

// parseLineComments checks if a trimmed line is a comment or a code line, managing the block comment state.
func parseLineComments(line string, inBlockComment *bool) (bool, bool) {
	if *inBlockComment {
		if isBlockCommentEnd(line) {
			*inBlockComment = false
		}
		return true, false
	}

	if isBlockCommentStart(line) {
		if !isBlockCommentEnd(line) && strings.Count(line, "\"\"\"") != 2 {
			*inBlockComment = true
		}
		return true, false
	}

	if isSingleLineComment(line) {
		return true, false
	}

	return false, true
}
