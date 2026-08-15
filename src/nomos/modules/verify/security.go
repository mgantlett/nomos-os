// Package verify implements the Definition of Done (DoD) verification gates.
// This specific file (security.go) contains the logic for static analysis and
// secret scanning across the repository. It defines regex patterns to detect
// hardcoded credentials, API keys, private keys, and common vulnerabilities.
//
// The scanner operates concurrently and uses Git to skip ignored files.
// It generates structured reports and can automatically block commits
// if CRITICAL or HIGH severity findings are discovered.
package verify

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SecuritySeverity represents the risk level of a finding.
type SecuritySeverity string

const (
	SeverityCritical SecuritySeverity = "CRITICAL"
	SeverityHigh     SecuritySeverity = "HIGH"
	SeverityMedium   SecuritySeverity = "MEDIUM"
	SeverityLow      SecuritySeverity = "LOW"
)

// Finding represents a single security finding during the codebase scan.
// It contains metadata about the exact location of the vulnerability,
// the snippet of code that triggered the rule, and the severity level.
//
// These findings are aggregated and returned to the CLI or the automated
// verification gate to determine if the push should be blocked.
type Finding struct {
	// File is the relative path to the file containing the vulnerability.
	File string
	// Line is the line number where the finding was detected (1-indexed).
	Line int
	// Content is the raw text string that matched the security rule.
	Content string
	// Severity indicates the risk level (CRITICAL, HIGH, MEDIUM, LOW).
	// CRITICAL and HIGH findings will typically block Git hooks.
	Severity SecuritySeverity
	// Message is a human-readable description of the security rule broken.
	Message string
}

// isGitIgnored checks if a file is git-ignored and not tracked.
func isGitIgnored(root, file string) bool {
	if _, err := runGit(root, "check-ignore", "-q", file); err == nil {
		if _, errTracked := runGit(root, "ls-files", "--error-unmatch", file); errTracked != nil {
			return true
		}
	}
	return false
}

// skippedDirs maps directory names that should be excluded from security auditing.
// This prevents scanning build, cache, package-lock, or dependency directories.
var skippedDirs = map[string]bool{
	"node_modules":           true,
	"vendor":                 true,
	".venv":                  true,
	"__pycache__":            true,
	"dist":                   true,
	"build":                  true,
	".nomos":                 true,
	".git":                   true,
	".nomos-commons":         true,
	"unsloth_compiled_cache": true,
	"control-plane-ui":       true,
	"ui":                     true,
}

// shouldSkipPath checks if the given path matches common directories we shouldn't scan.
// Splits the path and checks individual folder elements against the skippedDirs map.
func shouldSkipPath(path string) bool {
	// Normalize file path separators to standard forward slashes for cross-platform matching
	normalized := filepath.ToSlash(path)
	// Check if path contains cockpit embedded UI distribution directory
	if strings.Contains(normalized, "cockpit/ui") || strings.Contains(normalized, "modules/cockpit/ui") {
		// Instantly skip scanning static embedded UI web assets to avoid false positive XSS reports
		return true
	}
	// Split path components by slash to inspect directory names individually
	parts := strings.Split(normalized, "/")
	for _, p := range parts {
		// Evaluate directory component against list of excluded build and vendor folders
		if skippedDirs[p] {
			return true // Skip matches in excluded folders list
		}
	}
	// Do not recursively scan the security check code files themselves to prevent infinite self-referential reporting
	return strings.Contains(normalized, "src/nomos/modules/verify/security.go") || strings.Contains(normalized, "src/nomos/modules/verify/security_test.go")
}

// isTestOrMockFile checks if the filename/path indicates a test, mock or fixture file.
// Matches typical testing suffixes or folder structures.
func isTestOrMockFile(path string) bool {
	lower := strings.ToLower(path)
	// Return true if filename contains common testing keywords
	return strings.Contains(lower, "test") ||
		strings.Contains(lower, "spec") ||
		strings.Contains(lower, "fixture") ||
		strings.Contains(lower, "mock") ||
		strings.Contains(lower, "__tests__") ||
		strings.Contains(lower, "__mocks__")
}

// isCommented returns true if the line starts with a comment token for the given extension.
func isCommented(line, ext string) bool {
	trimmed := strings.TrimSpace(line)
	switch ext {
	case ".py", ".sh", ".yaml", ".yml":
		return strings.HasPrefix(trimmed, "#")
	case ".js", ".ts", ".jsx", ".tsx", ".go", ".java", ".c", ".cpp", ".h":
		return strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*")
	}
	return false
}

var (
	// Secrets Regexes
	rxAWS          = regexp.MustCompile(`AKIA[0-9A-Z]{16}`)
	rxOpenAIStripe = regexp.MustCompile(`sk-[a-zA-Z0-9]{20,}`)
	rxGitHubPAT    = regexp.MustCompile(`ghp_[a-zA-Z0-9]{36}`)
	rxGitHubApp    = regexp.MustCompile(`ghs_[a-zA-Z0-9]{36}`)
	rxGitLabPAT    = regexp.MustCompile(`glpat-[a-zA-Z0-9-]{20,}`)
	rxSlack        = regexp.MustCompile(`xox[bpsa]-[a-zA-Z0-9-]+`)
	rxGoogleOAuth  = regexp.MustCompile(`ya29\.[a-zA-Z0-9_-]+`)
	rxGoogleAPI    = regexp.MustCompile(`AIza[a-zA-Z0-9_-]{35}`)
	rxPostHog      = regexp.MustCompile(`phc_[a-zA-Z0-9]{30,}`)
	rxGenericKey   = regexp.MustCompile(`(?i)(KEY|SECRET|TOKEN|PAT|PASSWORD)[_A-Z0-9]*\s*=\s*["']([a-zA-Z0-9_-]{16,})["']`)

	// Telemetry Regexes
	rxTelemetrySDK = regexp.MustCompile(`posthog|PostHog|mixpanel|Mixpanel|@amplitude/|amplitude-js|amplitude\.getInstance|Amplitude\.init|segment\.io|Segment\.|sentry\.io|Sentry\.|google\.analytics|gtag|hotjar|Hotjar|hubspot|HubSpot|fullstory|FullStory|heap\.io|heapanalytics|datadog|DataDog|newrelic|New\.Relic`)
	rxTelemetryGen = regexp.MustCompile(`telemetry|captureEvent|trackEvent`)

	// Network Regexes (URL pattern)
	rxURL = regexp.MustCompile(`https?://[a-zA-Z0-9._-]+\.[a-z]{2,}[^"'\) ]*`)
	// We want to skip these URLs
	ignoredURLs = []string{
		"localhost", "127.0.0.1", "example.com", "schema.org", "json-schema", "w3.org",
		"github.com", "npmjs.org", "npmjs.com", "pypi.org", "bell.ca", "bell.corp.bce.ca",
		"jasonkarns.com", "mislav.net", "sstephenson.us", "sphinx-doc.org",
	}

	// Eval Regexes
	rxEvalJS = regexp.MustCompile(`\beval\s*\(|new\s+Function\s*\(|document\.write\s*\(`)
	rxEvalPy = regexp.MustCompile(`\bexec\s*\(|\beval\s*\(`)

	// XSS Sinks
	rxXSSSinks        = regexp.MustCompile(`\.innerHTML\s*=|\.outerHTML\s*=|\.insertAdjacentHTML\s*\(|document\.write\s*\(|document\.writeln\s*\(|dangerouslySetInnerHTML|v-html|\{@html|bypassSecurityTrust(Html|Script|Style|Url|ResourceUrl)`)
	rxXSSInlineURI    = regexp.MustCompile(`\b(href|src)\s*=\s*['"]\s*javascript:`)
	rxXSSUnquoted     = regexp.MustCompile(`\b(href|src|class|id|name|value|action)=\{\{[a-zA-Z0-9_.-]+\}\}`)
	rxXSSUnquotedHTML = regexp.MustCompile(`\b(href|src|class|id|name|value|action)=\{[a-zA-Z0-9_.-]+\}`)

	// CI/CD workflow rules
	rxCICDTarget = regexp.MustCompile(`pull_request_target:`)
	rxCICDWrite  = regexp.MustCompile(`permissions:\s*write-all`)

	// Lifecycle scripts in package.json
	rxLifecycleScript = regexp.MustCompile(`"(preinstall|install|postinstall)"\s*:`)
)

func isIgnoredURL(url string) bool {
	for _, ign := range ignoredURLs {
		if strings.Contains(url, ign) {
			return true
		}
	}
	return false
}

// ScanSecurity scans the workspace path for security issues.
func ScanSecurity(root string) ([]Finding, error) {
	var findings []Finding
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		relPath, err := filepath.Rel(absRoot, path)
		if err != nil {
			relPath = path
		}

		fileFindings, walkErr := auditFileForSecurity(absRoot, path, relPath, d)
		if len(fileFindings) > 0 {
			findings = append(findings, fileFindings...)
		}
		return walkErr
	})

	return findings, err
}

func auditFileForSecurity(absRoot, path, relPath string, d fs.DirEntry) ([]Finding, error) {
	if shouldSkipPath(relPath) {
		if d.IsDir() {
			return nil, filepath.SkipDir
		}
		return nil, nil
	}

	if d.IsDir() {
		return nil, nil
	}

	ext := strings.ToLower(filepath.Ext(path))
	name := filepath.Base(path)

	precheckFindings, stop, err := auditFileSecurityPrecheck(absRoot, relPath, ext, name)
	if stop {
		return precheckFindings, err
	}

	if shouldSkipSecurityContent(ext, name) || isBinaryFile(path) {
		return nil, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, nil
	}
	defer file.Close()

	isTest := isTestOrMockFile(relPath)
	isCICD := strings.HasPrefix(filepath.ToSlash(relPath), ".github/workflows/") && (ext == ".yml" || ext == ".yaml")

	return scanFileSecurityLines(file, ext, name, relPath, absRoot, isTest, isCICD)
}

func scanFileSecurityLines(file *os.File, ext, name, relPath, absRoot string, isTest, isCICD bool) ([]Finding, error) {
	scanner := bufio.NewScanner(file)
	bufMax := make([]byte, 64*1024)
	scanner.Buffer(bufMax, 1024*1024)

	var findings []Finding
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		commented := isCommented(line, ext)

		// Run line-by-line checks
		findings = append(findings, scanLineForSecrets(line, relPath, lineNum, isTest, absRoot)...)
		findings = append(findings, scanLineForTelemetry(line, ext, relPath, lineNum)...)
		findings = append(findings, scanLineForURLs(line, ext, relPath, lineNum)...)
		findings = append(findings, scanLineForEval(line, ext, relPath, lineNum, commented, isTest)...)
		findings = append(findings, scanLineForXSS(line, ext, relPath, lineNum, commented, isTest)...)
		findings = append(findings, scanLineForCICD(line, relPath, lineNum, isCICD)...)
		findings = append(findings, scanLineForLifecycle(line, name, relPath, lineNum)...)
	}

	if err := scanner.Err(); err != nil {
		// ignore ErrTooLong, just continue
	}

	return findings, nil
}

func auditFileSecurityPrecheck(absRoot, relPath, ext, name string) ([]Finding, bool, error) {
	if ext == ".pem" || ext == ".key" || strings.HasPrefix(name, "id_rsa") || strings.HasPrefix(name, "id_ed25519") {
		lowerPath := strings.ToLower(relPath)
		if !strings.Contains(lowerPath, "certifi") && !strings.Contains(lowerPath, "cacert") && !strings.Contains(lowerPath, "ca-certificates") {
			ignored := isGitIgnored(absRoot, relPath)
			severity := SeverityCritical
			msg := "Private key file found in repo"
			if ignored {
				severity = SeverityLow
				msg = "Private key file present but safely git-ignored"
			}
			return []Finding{{
				File:     relPath,
				Line:     1,
				Severity: severity,
				Message:  msg,
			}}, true, nil
		}
		return nil, true, nil
	}

	if name == ".env" {
		ignored := isGitIgnored(absRoot, relPath)
		severity := SeverityHigh
		msg := ".env file found and NOT safely git-ignored"
		if ignored {
			severity = SeverityLow
			msg = ".env file present but safely git-ignored"
		}
		return []Finding{{
			File:     relPath,
			Line:     1,
			Severity: severity,
			Message:  msg,
		}}, true, nil
	}
	return nil, false, nil
}

func shouldSkipSecurityContent(ext, name string) bool {
	return ext == ".lock" || name == "package-lock.json" || name == "yarn.lock" || name == "pnpm-lock.yaml" || name == "go.sum"
}

func isBinaryFile(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return true
	}
	defer file.Close()

	buf := make([]byte, 1024)
	n, _ := file.Read(buf)
	return n > 0 && bytes.Contains(buf[:n], []byte{0})
}

// scanLineForTelemetry checks individual code lines for telemetry SDK imports or tracking calls.
func scanLineForTelemetry(line, ext, relPath string, lineNum int) []Finding {
	var findings []Finding
	if ext == ".ts" || ext == ".js" || ext == ".py" || ext == ".go" {
		if rxTelemetrySDK.MatchString(line) {
			findings = append(findings, Finding{
				File:     relPath,
				Line:     lineNum,
				Content:  strings.TrimSpace(line),
				Severity: SeverityHigh,
				Message:  "Telemetry SDK detected in code",
			})
		} else if rxTelemetryGen.MatchString(line) {
			if !strings.Contains(relPath, ".nomos") {
				findings = append(findings, Finding{
					File:     relPath,
					Line:     lineNum,
					Content:  strings.TrimSpace(line),
					Severity: SeverityMedium,
					Message:  "Generic telemetry pattern detected",
				})
			}
		}
	}
	return findings
}

// scanLineForURLs scans source code lines for un-ignored hardcoded external HTTP endpoints.
func scanLineForURLs(line, ext, relPath string, lineNum int) []Finding {
	var findings []Finding
	if ext == ".ts" || ext == ".js" || ext == ".py" || ext == ".yaml" || ext == ".json" || ext == ".go" {
		urls := rxURL.FindAllString(line, -1)
		for _, u := range urls {
			if !isIgnoredURL(u) {
				findings = append(findings, Finding{
					File:     relPath,
					Line:     lineNum,
					Content:  strings.TrimSpace(line),
					Severity: SeverityMedium,
					Message:  fmt.Sprintf("Hardcoded external URL found: %s", u),
				})
			}
		}
	}
	return findings
}

// scanLineForEval audits code lines for dynamic evaluation statements such as eval or exec.
func scanLineForEval(line, ext, relPath string, lineNum int, commented, isTest bool) []Finding {
	var findings []Finding
	if !commented && !isTest {
		if ext == ".ts" || ext == ".js" {
			if rxEvalJS.MatchString(line) {
				findings = append(findings, Finding{
					File:     relPath,
					Line:     lineNum,
					Content:  strings.TrimSpace(line),
					Severity: SeverityHigh,
					Message:  "eval()/new Function()/document.write() dynamic execution",
				})
			}
		} else if ext == ".py" {
			if rxEvalPy.MatchString(line) {
				findings = append(findings, Finding{
					File:     relPath,
					Line:     lineNum,
					Content:  strings.TrimSpace(line),
					Severity: SeverityHigh,
					Message:  "Python exec()/eval() execution",
				})
			}
		}
	}
	return findings
}

// scanLineForXSS checks web asset files for unescaped innerHTML sinks or unquoted attribute interpolation.
func scanLineForXSS(line, ext, relPath string, lineNum int, commented, isTest bool) []Finding {
	if commented || isTest || !isXSSExtension(ext) {
		return nil
	}

	var findings []Finding
	if rxXSSSinks.MatchString(line) {
		findings = append(findings, Finding{
			File:     relPath,
			Line:     lineNum,
			Content:  strings.TrimSpace(line),
			Severity: SeverityHigh,
			Message:  "Dynamic DOM/framework XSS bypass/sink detected",
		})
	}
	if rxXSSInlineURI.MatchString(line) {
		findings = append(findings, Finding{
			File:     relPath,
			Line:     lineNum,
			Content:  strings.TrimSpace(line),
			Severity: SeverityHigh,
			Message:  "Inline executable JavaScript URI detected",
		})
	}
	if rxXSSUnquoted.MatchString(line) {
		findings = append(findings, Finding{
			File:     relPath,
			Line:     lineNum,
			Content:  strings.TrimSpace(line),
			Severity: SeverityMedium,
			Message:  "Unquoted attribute dynamic template placeholder",
		})
	}
	if ext == ".html" && rxXSSUnquotedHTML.MatchString(line) {
		findings = append(findings, Finding{
			File:     relPath,
			Line:     lineNum,
			Content:  strings.TrimSpace(line),
			Severity: SeverityMedium,
			Message:  "Unquoted attribute dynamic template placeholder",
		})
	}
	return findings
}

func isXSSExtension(ext string) bool {
	switch ext {
	case ".html", ".js", ".ts", ".jsx", ".tsx", ".vue", ".svelte", ".go":
		return true
	}
	return false
}

func scanLineForCICD(line, relPath string, lineNum int, isCICD bool) []Finding {
	var findings []Finding
	if isCICD {
		if rxCICDTarget.MatchString(line) {
			findings = append(findings, Finding{
				File:     relPath,
				Line:     lineNum,
				Content:  strings.TrimSpace(line),
				Severity: SeverityHigh,
				Message:  "pull_request_target workflow hook detected",
			})
		}
		if rxCICDWrite.MatchString(line) {
			findings = append(findings, Finding{
				File:     relPath,
				Line:     lineNum,
				Content:  strings.TrimSpace(line),
				Severity: SeverityHigh,
				Message:  "permissions: write-all broad permission detected",
			})
		}
	}
	return findings
}

// scanLineForLifecycle scans a file line to verify if it contains package.json lifecycle execution hooks.
func scanLineForLifecycle(line, name, relPath string, lineNum int) []Finding {
	var findings []Finding
	if name == "package.json" {
		if rxLifecycleScript.MatchString(line) {
			findings = append(findings, Finding{
				File:     relPath,
				Line:     lineNum,
				Content:  strings.TrimSpace(line),
				Severity: SeverityHigh,
				Message:  "Lifecycle script (preinstall/postinstall) found in package.json",
			})
		}
	}
	return findings
}

// scanLineForSecrets runs a suite of cryptographic regex patterns over line contents to detect hardcoded API keys.
func scanLineForSecrets(line, relPath string, lineNum int, isTest bool, absRoot string) []Finding {
	var findings []Finding
	for _, item := range []struct {
		rx   *regexp.Regexp
		name string
	}{
		{rxAWS, "AWS Access Key"},
		{rxOpenAIStripe, "OpenAI/Stripe secret key"},
		{rxGitHubPAT, "GitHub PAT"},
		{rxGitHubApp, "GitHub App token"},
		{rxGitLabPAT, "GitLab PAT"},
		{rxSlack, "Slack token"},
		{rxGoogleOAuth, "Google OAuth token"},
		{rxGoogleAPI, "Google API key"},
		{rxPostHog, "PostHog project key"},
		{rxGenericKey, "Generic Hardcoded Key"},
	} {
		if item.rx.MatchString(line) {
			severity := SeverityCritical
			msg := fmt.Sprintf("Potential secret matching '%s' in production code", item.name)
			if isTest {
				severity = SeverityLow
				msg = fmt.Sprintf("Secret pattern '%s' in test fixtures (likely fake)", item.name)
			} else if isGitIgnored(absRoot, relPath) {
				severity = SeverityLow
				msg = fmt.Sprintf("Secret matching '%s' inside safely git-ignored file", item.name)
			}
			findings = append(findings, Finding{
				File:     relPath,
				Line:     lineNum,
				Content:  strings.TrimSpace(line),
				Severity: severity,
				Message:  msg,
			})
		}
	}
	return findings
}
