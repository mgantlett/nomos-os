// Package verify implements quality assurance checks for the Nomos workspace.
// This file analyzes monolithic file size constraints (warn 1500, block 2000 lines)
// and evaluates sliding window duplicate code blocks to prevent codebase bloatedness.
// Duplicate code is one of the primary drivers of technical debt and regression bugs,
// because fixing a bug in one place does not fix it in the duplicated block.
// Nomos combats this by running an exact-match sliding window over all source files.
// When developers copy-paste blocks larger than the window size, the build is failed
// to force them into using DRY (Don't Repeat Yourself) principles and refactoring.
// Similarly, the monolithic file size check discourages engineers from dumping
// all of their features into massive "god objects" or single files, pushing them
// towards modular architecture and smaller, testable units of code.
// The tools provided here run as part of the Definition of Done (DoD) lifecycle.
// By breaking down monolithic files into smaller modular units of code,
// the maintainability of the project is improved and tests are easier to write.
// Duplicate blocks across files must either be consolidated into generic helper
// functions, or bypassed using quality debt entries.
// The code deduplicator handles multiple languages, skipping minified files or assets.
// It tracks file lines in memory and hashes them for quick constant-time comparisons.
// It skips test files to allow for copy-pasted assertions where DRY is less critical.
// We also track file modifications and bypass them if they exceed threshold sizes.
//
// Thresholds are configured globally per workspace and must be rigorously followed.
// Failure to meet the density checks will prevent code from merging via verify gates.
package verify

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/synapse"
)

var (
	codeExtensions = map[string]bool{
		".go":   true,
		".py":   true,
		".ts":   true,
		".tsx":  true,
		".js":   true,
		".jsx":  true,
		".rs":   true,
		".nix":  true,
		".sh":   true,
		".html": true,
		".css":  true,
		".scss": true,
		".json": true,
		".yaml": true,
		".yml":  true,
	}

	duplicationExtensions = map[string]bool{
		".go":  true,
		".py":  true,
		".ts":  true,
		".tsx": true,
		".js":  true,
		".jsx": true,
		".rs":  true,
		".sh":  true,
	}

	contractExcludePattern = regexp.MustCompile(`(?i)(_test\.go|\.test\.|\.spec\.|\/tests?\/|\/testing\/|^test_|plugins-available\/|scripts\/|mock\/|mocks\/|\/dist\/|dist\/|\/node_modules\/|node_modules\/|\/\.agent\/|\.agent\/)`)
)

// normalizeLine strips a source line of whitespace, comments, and structural noise
// so that structurally equivalent lines produce identical normalized outputs for hashing.
func normalizeLine(line string) string {
	cleaned := strings.TrimSpace(line)
	if idx := strings.Index(cleaned, "//"); idx != -1 {
		cleaned = cleaned[:idx]
	}
	if idx := strings.Index(cleaned, "#"); idx != -1 {
		cleaned = cleaned[:idx]
	}
	if idx := strings.Index(cleaned, "--"); idx != -1 {
		cleaned = cleaned[:idx]
	}
	cleaned = strings.TrimSpace(cleaned)

	// Ignore package declarations and import blocks
	if strings.HasPrefix(cleaned, "package ") || strings.HasPrefix(cleaned, "import ") || cleaned == "import" || cleaned == "package" {
		return ""
	}
	// Ignore quoted strings (Go/JSON imports and single value configuration lines)
	if (strings.HasPrefix(cleaned, "\"") && strings.HasSuffix(cleaned, "\"")) || (strings.HasPrefix(cleaned, "`") && strings.HasSuffix(cleaned, "`")) {
		return ""
	}

	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "\t", "")
	cleaned = strings.ReplaceAll(cleaned, "\r", "")
	return cleaned
}

type hashResult struct {
	HashStr string
	EndLine int
}

// hashBlockAt computes an MD5 hash of the next `windowSize` non-empty normalized lines
// starting from `startIdx`. Returns nil if the start index is an empty/ignored line
// (preventing self-matching false positives) or if insufficient lines remain.
func hashBlockAt(lines []string, normalizedLines []string, startIdx int, windowSize int) *hashResult {
	// Skip empty starting lines to prevent identical windows from different offsets
	if startIdx < len(normalizedLines) && len(normalizedLines[startIdx]) == 0 {
		return nil
	}
	var blockLines []string
	cursor := startIdx
	for len(blockLines) < windowSize && cursor < len(lines) {
		norm := normalizedLines[cursor]
		if len(norm) > 0 {
			blockLines = append(blockLines, norm)
		}
		cursor++
	}

	if len(blockLines) == windowSize {
		blockContent := strings.Join(blockLines, "\n")
		hasher := md5.New()
		hasher.Write([]byte(blockContent))
		return &hashResult{
			HashStr: hex.EncodeToString(hasher.Sum(nil)),
			EndLine: cursor,
		}
	}
	return nil
}

type duplicateOccurrence struct {
	File         string
	LineStart    int
	LineEnd      int
	OriginalText string
}

// getRelativePath converts an absolute file path to a relative path from the project root.
func getRelativePath(root string, path string) string {
	absRoot, err1 := filepath.Abs(root)
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	absPath, err2 := filepath.Abs(path)
	if err1 == nil && err2 == nil {
		if rel, err := filepath.Rel(absRoot, absPath); err == nil {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}

// getProjectFiles retrieves all tracked files from git; falls back to a manual
// directory walk if git is unavailable (e.g., in non-git workspaces).
func getProjectFiles(root string) ([]string, error) {
	out, err := runGit(root, "ls-files")
	if err != nil {
		// Fallback to manual directory scan
		var files []string
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && shouldSkipDirectory(d.Name()) {
				return filepath.SkipDir
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err == nil {
				files = append(files, rel)
			}
			return nil
		})
		return files, err
	}

	lines := strings.Split(out, "\n")
	var files []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			files = append(files, l)
		}
	}
	return files, nil
}

// filterFiles checks a list of files and extracts only the relevant source files.
// It matches valid programming extensions and filters out unit tests and mocks.
func filterFiles(files []string) []string {
	var filtered []string

	// Evaluate extension and exclude patterns for each file
	for _, f := range files {
		ext := filepath.Ext(f)

		// If file has a supported code extension and is not an excluded artifact type
		if codeExtensions[ext] && !contractExcludePattern.MatchString(f) {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

// filterDuplicationFiles filters a list of files to only include files with extensions
// defined in duplicationExtensions, while excluding test and agent artifacts.
func filterDuplicationFiles(files []string) []string {
	var filtered []string
	for _, f := range files {
		ext := filepath.Ext(f)
		// Only check duplication for source code extensions, bypassing templates/JSON/Nix
		if duplicationExtensions[ext] && !contractExcludePattern.MatchString(f) {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

// RunRefactorChecks runs monolithic file length and structural code duplication audits.
// It verifies that no single file exceeds code length guidelines and scans codebases
// for repeated or duplicate patterns using a sliding window algorithm.
func RunRefactorChecks(root string, all bool) error {
	synapse.Info("%s", fmt.Sprint("\n\x1b[1m\x1b[36m▶ 🔍 Nomos Code Duplication & File Length Auditor\x1b[0m"))

	// Get a list of files that need to be audited (staged changes or all codebase files)
	filesToAudit, err := getFilesToAudit(root, all)
	if err != nil {
		return err
	}

	// Skip checks if there are no modified files to evaluate
	if len(filesToAudit) == 0 {
		synapse.Info("%s", fmt.Sprint("   \x1b[32m↳ No staged logic files to audit. Skipping.\x1b[0m"))
		return nil
	}

	hasErrors := false

	// 1. FILE LENGTH AUDIT
	// Perform length audit verifying file sizing targets
	synapse.Info("%s", fmt.Sprint("\n\x1b[1m1. Evaluating Monolithic File Length Constraints...\x1b[0m"))
	if auditFilesLength(root, filesToAudit) {
		hasErrors = true
	}

	// 2. STRUCTURAL CODE DUPLICATION AUDIT
	// Initialize sliding window block size (default: 10 lines)
	synapse.Info("%s", fmt.Sprint("\n\x1b[1m2. Evaluating Structural Code Duplication (Sliding Window: 10 lines)...\x1b[0m"))

	windowSize := 10
	allFiles, err := getProjectFiles(root)
	if err != nil {
		return err
	}
	allFiltered := filterDuplicationFiles(allFiles)

	// Pre-build index of md5 hash blocks across the entire project
	hashMap, err := buildProjectDuplicationMap(root, allFiltered, windowSize)
	if err != nil {
		return err
	}

	// Audit each file against the global duplicate index
	for _, file := range filesToAudit {
		ext := filepath.Ext(file)
		if !duplicationExtensions[ext] {
			continue
		}
		dupCount, dupDensity, duplicateMatchesList, err := auditFileDuplication(root, file, windowSize, hashMap)
		if err != nil {
			continue
		}
		if printDuplicationReports(root, file, dupCount, dupDensity, duplicateMatchesList) {
			hasErrors = true
		}
	}

	// Fail verification if any limit checks are broken
	if hasErrors {
		return fmt.Errorf("specification check failed: monolithic file bounds or duplicate code limits violated")
	}

	synapse.Info("%s", fmt.Sprint("   \x1b[32m✅ Specification Check Passed. 100% compliant duplication and size boundaries!\x1b[0m"))
	return nil
}

// getFilesToAudit resolves the subset of files to check (either staged files or all internal files).
func getFilesToAudit(root string, all bool) ([]string, error) {
	// If checking all files, retrieve everything in workspace
	if all {
		allFiles, err := getProjectFiles(root)
		if err != nil {
			return nil, err
		}
		return filterFiles(allFiles), nil
	}
	// Fetch currently staged/changed files in Git index
	staged, err := getStagedFiles(root)
	if err != nil {
		return nil, err
	}
	return filterFiles(staged), nil
}

// auditFilesLength iterates through files to verify that none exceed maximum monolithic line boundaries.
func auditFilesLength(root string, filesToAudit []string) bool {
	hasErrors := false
	// Scan line counts of each file individually
	for _, file := range filesToAudit {
		absPath := filepath.Join(root, file)
		contentBytes, err := os.ReadFile(absPath)
		if err != nil {
			continue // Skip unreadable files
		}
		lines := strings.Split(string(contentBytes), "\n")
		lineCount := len(lines)

		// Enforce strict limit checks
		if lineCount > 2000 {
			// Check if file is registered in quality debt list
			bypassed, linkedTask := CheckQualityDebtBypass(root, file, "monolithic_file_limit")
			if bypassed {
				synapse.Info("   \x1b[32m⏭️  [Quality Debt] Bypassed 'monolithic_file_limit' for '%s' (Linked to active task #%s)\x1b[0m\n", file, linkedTask)
			} else {
				hasErrors = true
				synapse.Info("   \x1b[31m❌ Monolithic File Length Violation: Staged file '%s' exceeds 2000 lines (%d lines).\x1b[0m\n", file, lineCount)
				StageAutoDebtTask(root, file, "monolithic_file_limit", fmt.Sprintf("File '%s' exceeds 2000 lines (%d lines)", file, lineCount))
			}
		} else if lineCount > 1500 {
			// Trigger a warning suggestion if line count exceeds warn threshold (1500)
			synapse.Info("   \x1b[33m⚠️  Monolithic File Length Warning: Staged file '%s' exceeds 1500 lines (%d lines). Consider modularizing.\x1b[0m\n", file, lineCount)
		} else {
			// Pass if line count is comfortably below limits
			synapse.Info("   \x1b[32m↳ %s: %d lines (Passed)\x1b[0m\n", file, lineCount)
		}
	}
	return hasErrors
}

// buildProjectDuplicationMap builds an MD5 block map for all filtered files in the project.
// It maps the hash of each sliding window block to its file occurrences.
func buildProjectDuplicationMap(root string, allFiltered []string, windowSize int) (map[string][]duplicateOccurrence, error) {
	// Initialize duplication index registry map
	hashMap := make(map[string][]duplicateOccurrence)

	// Read and hash each code file in the database
	for _, file := range allFiltered {
		absPath := filepath.Join(root, file)
		contentBytes, err := os.ReadFile(absPath)
		if err != nil {
			continue // Skip files that are not accessible
		}

		// Parse code lines and strip whitespace, ignoring block comments
		strippedContent := stripBlockComments(contentBytes)
		lines := strings.Split(strippedContent, "\n")
		normalizedLines := make([]string, len(lines))
		for idx, line := range lines {
			normalizedLines[idx] = normalizeLine(line)
		}

		// Index all sliding window occurrences for this file
		addFileDuplicationOccurrences(file, lines, normalizedLines, windowSize, hashMap)
	}
	return hashMap, nil
}

// addFileDuplicationOccurrences indexes all sliding window code blocks from a single file
// into the project-wide hash map. Empty/ignored starting lines are skipped to prevent
// identical hash windows that begin at different offsets over blank/comment regions.
func addFileDuplicationOccurrences(file string, lines, normalizedLines []string, windowSize int, hashMap map[string][]duplicateOccurrence) {
	// Slide window across each index of the file lines
	for i := 0; i <= len(lines)-windowSize; i++ {
		if len(normalizedLines[i]) == 0 {
			continue
		}
		var origLines []string
		cursor := i
		var blockLines []string
		// Collect normalized lines within the window size boundaries
		for len(blockLines) < windowSize && cursor < len(lines) {
			norm := normalizedLines[cursor]
			// Only count non-empty code lines in the window block
			if len(norm) > 0 {
				blockLines = append(blockLines, norm)
				origLines = append(origLines, lines[cursor])
			}
			cursor++
		}

		if len(blockLines) == windowSize {
			blockContent := strings.Join(blockLines, "\n")
			hasher := md5.New()
			hasher.Write([]byte(blockContent))
			hashStr := hex.EncodeToString(hasher.Sum(nil))

			occurrence := duplicateOccurrence{
				File:         file,
				LineStart:    i + 1,
				LineEnd:      cursor,
				OriginalText: strings.Join(origLines, "\n"),
			}
			hashMap[hashStr] = append(hashMap[hashStr], occurrence)
		}
	}
}

type matchDetail struct {
	startLine      int
	endLine        int
	matchFile      string
	matchStartLine int
	matchEndLine   int
	blockText      string
}

// auditFileDuplication scans a single file against the project-wide duplication hash map
// and returns the total count of duplicated lines, the density percentage, and match details.
func auditFileDuplication(root string, file string, windowSize int, hashMap map[string][]duplicateOccurrence) (int, float64, []matchDetail, error) {
	absPath := filepath.Join(root, file)
	contentBytes, err := os.ReadFile(absPath)
	if err != nil {
		return 0, 0, nil, err
	}
	strippedContent := stripBlockComments(contentBytes)
	lines := strings.Split(strippedContent, "\n")
	totalLines := len(lines)
	normalizedLines := make([]string, len(lines))
	for idx, line := range lines {
		normalizedLines[idx] = normalizeLine(line)
	}

	duplicatedLineNumbers := make(map[int]bool)
	var duplicateMatchesList []matchDetail

	for i := 0; i <= len(lines)-windowSize; i++ {
		res := hashBlockAt(lines, normalizedLines, i, windowSize)
		if res == nil {
			continue
		}

		validDuplicates := findValidDuplicates(res.HashStr, file, i, windowSize, hashMap)
		if len(validDuplicates) > 0 {
			recordDuplicates(i, res, validDuplicates, duplicatedLineNumbers, &duplicateMatchesList)
		}
	}

	dupCount := len(duplicatedLineNumbers)
	var dupDensity float64
	if totalLines > 0 {
		dupDensity = (float64(dupCount) / float64(totalLines)) * 100
	}

	return dupCount, dupDensity, duplicateMatchesList, nil
}

// findValidDuplicates filters hash matches to exclude self-overlapping windows
// within the same file. Only cross-file or sufficiently distant same-file matches qualify.
func findValidDuplicates(hashStr string, file string, i, windowSize int, hashMap map[string][]duplicateOccurrence) []duplicateOccurrence {
	matches := hashMap[hashStr]
	var validDuplicates []duplicateOccurrence
	for _, m := range matches {
		if m.File != file || m.LineStart-1-i >= windowSize || i+1-m.LineStart >= windowSize {
			validDuplicates = append(validDuplicates, m)
		}
	}
	return validDuplicates
}

// recordDuplicates adds valid duplicate matches into the tracking structures.
func recordDuplicates(i int, res *hashResult, validDuplicates []duplicateOccurrence, duplicatedLineNumbers map[int]bool, duplicateMatchesList *[]matchDetail) {
	for l := i + 1; l <= res.EndLine; l++ {
		duplicatedLineNumbers[l] = true
	}
	primaryMatch := validDuplicates[0]
	*duplicateMatchesList = append(*duplicateMatchesList, matchDetail{
		startLine:      i + 1,
		endLine:        res.EndLine,
		matchFile:      primaryMatch.File,
		matchStartLine: primaryMatch.LineStart,
		matchEndLine:   primaryMatch.LineEnd,
		blockText:      primaryMatch.OriginalText,
	})
}

// printDuplicationBreakdown iterates through the match details and prints unique blocks.
func printDuplicationBreakdown(duplicateMatchesList []matchDetail) {
	printedRanges := make(map[string]bool)
	for _, match := range duplicateMatchesList {
		if !isRangeAlreadyPrinted(match.startLine, match.endLine, printedRanges) {
			rangeKey := fmt.Sprintf("%d-%d", match.startLine, match.endLine)
			printedRanges[rangeKey] = true
			printDuplicateMatchBlock(match)
		}
	}
}

// printDuplicationReports formats and prints duplication analysis results for a file,
// escalating from info to warning to violation based on density thresholds.
func printDuplicationReports(root string, file string, dupCount int, dupDensity float64, duplicateMatchesList []matchDetail) bool {
	if dupDensity > 5.0 {
		bypassed, linkedTask := CheckQualityDebtBypass(root, file, "duplication_limit")
		if bypassed {
			synapse.Info("   \x1b[32m⏭️  [Quality Debt] Bypassed 'duplication_limit' for '%s' (Linked to active task #%s)\x1b[0m\n", file, linkedTask)
		} else {
			synapse.Info("   \x1b[31m❌ Code Duplication Violation: Staged file '%s' has %d duplicated lines (%.2f%% duplication density). Max allowed is 5%%.\x1b[0m\n", file, dupCount, dupDensity)
			StageAutoDebtTask(root, file, "duplication_limit", fmt.Sprintf("File '%s' has %d duplicated lines (%.2f%% duplication density)", file, dupCount, dupDensity))

			synapse.Info("%s", fmt.Sprint("      \x1b[33m📋 Duplicated Blocks Breakdown:\x1b[0m"))
			printDuplicationBreakdown(duplicateMatchesList)
			return true
		}
	} else if dupDensity > 0 {
		synapse.Info("   \x1b[33m⚠️  Code Duplication Warning: Staged file '%s' contains %d duplicated lines (%.2f%% duplication density). Keep it clean!\x1b[0m\n", file, dupCount, dupDensity)
	} else {
		synapse.Info("   \x1b[32m↳ %s: 0%% duplication density (Passed)\x1b[0m\n", file)
	}
	return false
}

// stripBlockComments removes block comments from source code by replacing them with spaces
// and maintaining newlines to preserve line numbering for error reporting.
func stripBlockComments(content []byte) string {
	str := string(content)
	var sb strings.Builder
	inCStyle := false
	inHTML := false
	inPython := false

	for i := 0; i < len(str); i++ {
		if !inCStyle && !inHTML && !inPython {
			if strings.HasPrefix(str[i:], "/*") {
				inCStyle = true
				sb.WriteString("  ")
				i++
				continue
			}
			if strings.HasPrefix(str[i:], "<!--") {
				inHTML = true
				sb.WriteString("    ")
				i += 3
				continue
			}
			if strings.HasPrefix(str[i:], "\"\"\"") {
				inPython = true
				sb.WriteString("   ")
				i += 2
				continue
			}
			sb.WriteByte(str[i])
		} else {
			if str[i] == '\n' {
				sb.WriteByte('\n')
			} else {
				sb.WriteByte(' ') // Replace comment characters with spaces
			}

			if inCStyle && strings.HasPrefix(str[i:], "*/") {
				inCStyle = false
				sb.WriteString(" ")
				i++
			} else if inHTML && strings.HasPrefix(str[i:], "-->") {
				inHTML = false
				sb.WriteString("  ")
				i += 2
			} else if inPython && strings.HasPrefix(str[i:], "\"\"\"") {
				inPython = false
				sb.WriteString("  ")
				i += 2
			}
		}
	}
	return sb.String()
}

// isRangeAlreadyPrinted checks if a line range is a subset of any previously printed range
// to prevent duplicate match output for overlapping sliding window blocks.
func isRangeAlreadyPrinted(startLine, endLine int, printedRanges map[string]bool) bool {
	for printed := range printedRanges {
		var pStart, pEnd int
		fmt.Sscanf(printed, "%d-%d", &pStart, &pEnd)
		if startLine >= pStart && endLine <= pEnd {
			return true
		}
	}
	return false
}

// printDuplicateMatchBlock renders a single duplication match with its source/target
// line ranges and a preview of the first 3 duplicated lines.
func printDuplicateMatchBlock(match matchDetail) {
	synapse.Info("        \x1b[36m• Matches lines %d-%d\x1b[0m with \x1b[90m%s:%d-%d\x1b[0m\n", match.startLine, match.endLine, match.matchFile, match.matchStartLine, match.matchEndLine)
	synapse.Info("%s", fmt.Sprint("        \x1b[90m--------------------------------------------\x1b[0m"))
	textLines := strings.Split(match.blockText, "\n")
	limit := 3
	if len(textLines) < limit {
		limit = len(textLines)
	}
	for j := 0; j < limit; j++ {
		synapse.Info("               %s\n", textLines[j])
	}
	if len(textLines) > 3 {
		synapse.Info("%s", fmt.Sprint("               ..."))
	}
	synapse.Info("%s", fmt.Sprint("        \x1b[90m--------------------------------------------\x1b[0m"))
}

// duplicationDensityStr formats a density float for display, returning "0" for
// zero values and a two-decimal-place string otherwise.
func duplicationDensityStr(d float64) string {
	if d == 0 {
		return "0"
	}
	return fmt.Sprintf("%.2f", d)
}

// CheckDuplicateStructs verifies that core domain structs (specifically PhaseState)
// are defined exactly once in the codebase, preventing type-safety and serialization drift.
func CheckDuplicateStructs(root string) (int, error) {
	var occurrences []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, isMatch, err := checkDuplicateStructFile(root, path, info)
		if err != nil {
			return err
		}
		if isMatch {
			occurrences = append(occurrences, relPath)
		}
		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("failed to walk codebase: %w", err)
	}

	if len(occurrences) > 1 {
		return len(occurrences), fmt.Errorf("duplicate definition of 'PhaseState' struct/class/interface detected in: %s. Please define it in a single package/module and import it", strings.Join(occurrences, ", "))
	}

	return len(occurrences), nil
}

func checkDuplicateStructFile(root string, path string, info os.FileInfo) (string, bool, error) {
	if info.IsDir() {
		if shouldSkipDirectory(info.Name()) {
			return "", false, filepath.SkipDir
		}
		return "", false, nil
	}

	ext := filepath.Ext(path)
	if !isTargetExtension(ext) || isTestFile(path) {
		return "", false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, nil
	}

	if matchDuplicateStruct(ext, string(data)) {
		relPath, _ := filepath.Rel(root, path)
		return filepath.ToSlash(relPath), true, nil
	}
	return "", false, nil
}

// shouldSkipDirectory checks if a directory should be excluded from AST duplicate struct audits.
func shouldSkipDirectory(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules" || name == "dist" || name == "testrepo" || name == "control-plane-ui"
}

// isTargetExtension checks if a file extension is supported for duplicate struct detection.
func isTargetExtension(ext string) bool {
	return ext == ".go" || ext == ".ts" || ext == ".tsx" || ext == ".py"
}

func matchDuplicateStruct(ext string, content string) bool {
	switch ext {
	case ".go":
		return strings.Contains(content, "type "+"PhaseState"+" struct")
	case ".ts", ".tsx":
		return strings.Contains(content, "interface "+"PhaseState") || strings.Contains(content, "class "+"PhaseState")
	case ".py":
		return strings.Contains(content, "class "+"PhaseState")
	}
	return false
}
