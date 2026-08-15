package verify

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// SymbolInfo represents a code-level declaration.
type SymbolInfo struct {
	Name      string
	Type      string // "function", "type", "variable"
	LineStart int
	LineEnd   int
}

// parseGoSymbols parses a Go source file and extracts all declarations.
func parseGoSymbols(filePath string) ([]SymbolInfo, error) {
	var symbols []SymbolInfo
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	ast.Inspect(node, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		switch decl := n.(type) {
		case *ast.FuncDecl:
			symbols = append(symbols, handleFuncDecl(decl, fset))
		case *ast.GenDecl:
			symbols = append(symbols, handleGenDecl(decl, fset)...)
		}
		return true
	})

	return symbols, nil
}

// handleFuncDecl processes function and method nodes to build SymbolInfo metadata.
func handleFuncDecl(decl *ast.FuncDecl, fset *token.FileSet) SymbolInfo {
	startPos := fset.Position(decl.Pos())
	endPos := fset.Position(decl.End())
	return SymbolInfo{
		Name:      decl.Name.Name,
		Type:      "function",
		LineStart: startPos.Line,
		LineEnd:   endPos.Line,
	}
}

// handleGenDecl processes general declaration nodes (type, var, const).
func handleGenDecl(decl *ast.GenDecl, fset *token.FileSet) []SymbolInfo {
	if decl.Tok == token.TYPE {
		return handleTypeDecl(decl, fset)
	}
	if decl.Tok == token.VAR || decl.Tok == token.CONST {
		return handleVarConstDecl(decl, fset)
	}
	return nil
}

// handleTypeDecl processes type declarations.
// It iterates through the specs in the general declaration, matching TypeSpec nodes,
// and extracts their names, type metadata, and line range positions in the source file.
func handleTypeDecl(decl *ast.GenDecl, fset *token.FileSet) []SymbolInfo {
	var symbols []SymbolInfo
	// Loop over specs inside type general declaration
	for _, spec := range decl.Specs {
		if typeSpec, ok := spec.(*ast.TypeSpec); ok {
			startPos := fset.Position(typeSpec.Pos())
			endPos := fset.Position(typeSpec.End())
			// Add parsed type symbol metadata
			symbols = append(symbols, SymbolInfo{
				Name:      typeSpec.Name.Name,
				Type:      "type",
				LineStart: startPos.Line,
				LineEnd:   endPos.Line,
			})
		}
	}
	return symbols
}

// handleVarConstDecl processes variables and constants declarations.
// It extracts name, type, and source line boundaries for declared values.
func handleVarConstDecl(decl *ast.GenDecl, fset *token.FileSet) []SymbolInfo {
	var symbols []SymbolInfo
	// Loop through all specs inside variable/constant general declaration
	for _, spec := range decl.Specs {
		if valueSpec, ok := spec.(*ast.ValueSpec); ok {
			// Iterate over individual variable names defined in a single spec line
			for _, name := range valueSpec.Names {
				startPos := fset.Position(name.Pos())
				endPos := fset.Position(name.End())
				// Append parsed variable symbol details
				symbols = append(symbols, SymbolInfo{
					Name:      name.Name,
					Type:      "variable",
					LineStart: startPos.Line,
					LineEnd:   endPos.Line,
				})
			}
		}
	}
	return symbols
}

// getGitAddedLines runs git diff -U0 on a file and returns a map of all added/modified line numbers.
// It queries git first using cached staging area modifications, then unstaged diff changes.
func getGitAddedLines(root, relPath string) (map[int]bool, error) {
	addedLines := make(map[int]bool)

	// Fetch changes currently staged in the git cache index
	out, err := runGit(root, "diff", "--cached", "-U0", "--", relPath)
	if err != nil {
		return nil, err
	}

	// Fallback to local unstaged edits if cache was empty
	if strings.TrimSpace(out) == "" {
		out, err = runGit(root, "diff", "-U0", "--", relPath)
		if err != nil {
			return nil, err
		}
	}

	// Regular expression matching unified diff hunk boundaries (+line,len)
	rx := regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)
	lines := strings.Split(out, "\n")
	// Loop through output lines to find chunk ranges
	for _, line := range lines {
		start, length, ok := parseDiffHunk(line, rx)
		if ok && length > 0 {
			// Track all lines modified within this unified diff hunk
			for i := start; i < start+length; i++ {
				addedLines[i] = true
			}
		}
	}

	return addedLines, nil
}

// parseDiffHunk parses unified diff header lines to get start and length.
// It inspects lines starting with @@ and extracts match indices from matching hunk regexes.
func parseDiffHunk(line string, rx *regexp.Regexp) (int, int, bool) {
	line = strings.TrimSpace(line)
	// Ignore normal diff addition/deletion code lines
	if !strings.HasPrefix(line, "@@") {
		return 0, 0, false
	}
	match := rx.FindStringSubmatch(line)
	if len(match) > 1 {
		start, _ := strconv.Atoi(match[1])
		length := 1
		// Parse optional length parameter if defined (e.g. +10,12)
		if len(match) > 2 && match[2] != "" {
			length, _ = strconv.Atoi(match[2])
		}
		return start, length, true
	}
	return 0, 0, false
}

// tokenizePlanText tokenizes implementation_plan.md, returning a map of relative file paths to their sets of tokens.
// It scans the plan lines, identifies modify/new file sections, and parses tokens under each target file.
func tokenizePlanText(planPath, root string) (map[string]map[string]bool, error) {
	fileTokens := make(map[string]map[string]bool)
	file, err := os.Open(planPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	// Regex boundaries to match strict files list (markdown format with file scheme)
	patternStrict := regexp.MustCompile(`(?i)^####\s+\[(?:NEW|MODIFY|DELETE)\]\s+\[[^\]]*\]\((?:file:\/\/)?([^\)]+)\)`)
	patternLoose := regexp.MustCompile(`(?i)^####\s+\[(?:NEW|MODIFY|DELETE)\]\s+([^\s]+)`)
	rxHeader := regexp.MustCompile(`^(?:##\s+|###\s+[^\[])`)

	var currentFile string
	scanner := bufio.NewScanner(file)

	// Scan document line by line
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Try extracting a file section block path
		filePath := extractPlannedFilePath(line, patternStrict, patternLoose, absRoot)
		if filePath != "" {
			currentFile = filePath
			fileTokens[currentFile] = make(map[string]bool)
			continue
		}

		// Parse token details if currently scanning within a specific file block
		if currentFile != "" {
			// Clear target context if encountering a new high-level markdown header
			if rxHeader.MatchString(line) {
				currentFile = ""
				continue
			}

			// Define callback to add tokens directly to file map
			addToken := func(t string) {
				registerPlanToken(fileTokens[currentFile], t)
			}
			extractLineTokens(line, addToken)
		}
	}

	return fileTokens, scanner.Err()
}

// extractPlannedFilePath extracts and normalizes file paths from the proposed changes header.
// It strips file/localhost schemes and converts relative path formats.
func extractPlannedFilePath(line string, rxStrict, rxLoose *regexp.Regexp, absRoot string) string {
	var filePath string
	if match := rxStrict.FindStringSubmatch(line); len(match) > 1 {
		filePath = match[1]
	} else if match := rxLoose.FindStringSubmatch(line); len(match) > 1 {
		filePath = match[1]
	}

	if filePath == "" {
		return ""
	}

	// Trim anchors and schemes from path string
	filePath = strings.Split(filePath, "#")[0]
	filePath = strings.TrimPrefix(filePath, "file://")
	filePath = strings.TrimPrefix(filePath, "localhost")

	rootBase := filepath.Base(absRoot)
	idx := strings.Index(filepath.ToSlash(filePath), "/"+rootBase+"/")
	if idx != -1 {
		filePath = filePath[idx+len(rootBase)+2:]
	}

	rel, err := filepath.Rel(absRoot, filepath.Join(absRoot, filePath))
	if err == nil {
		filePath = filepath.ToSlash(filepath.Clean(rel))
	}
	return strings.TrimPrefix(filePath, "./")
}

// registerPlanToken registers a token and splits it to register sub-parts.
// This splits camelCase, snake_case, and custom tokens to ensure matching words are registered.
func registerPlanToken(tokens map[string]bool, t string) {
	t = strings.TrimSpace(t)
	if t == "" {
		return
	}
	tokens[t] = true
	// Split token by non-alphanumeric separators to support split words matches
	parts := strings.FieldsFunc(t, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '$'
	})
	for _, p := range parts {
		if p != "" {
			tokens[p] = true
		}
	}
}

// extractLineTokens extracts backticks, quotes, and word tokens from a line.
// It uses regular expressions to find structured elements in the specification plan details.
func extractLineTokens(line string, addToken func(string)) {
	// Parse all backtick references `TransitionPhase`
	rxBackticks := regexp.MustCompile("`([^`]+)`")
	for _, match := range rxBackticks.FindAllStringSubmatch(line, -1) {
		addToken(match[1])
	}

	// Parse quote strings references "TransitionPhase"
	rxQuotes := regexp.MustCompile(`["']([^"']+)["']`)
	for _, match := range rxQuotes.FindAllStringSubmatch(line, -1) {
		addToken(match[1])
	}

	// Parse general alphanumeric words
	rxWords := regexp.MustCompile(`[a-zA-Z0-9_$]+`)
	for _, word := range rxWords.FindAllString(line, -1) {
		addToken(word)
	}
}
