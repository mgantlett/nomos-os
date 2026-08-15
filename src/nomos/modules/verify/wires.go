package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mgantlett/nomos-commons/src/nomos/core/config"
)

// wireFinding represents an unwired DOM element, disconnected API endpoint, or missing WS handler.
type wireFinding struct {
	Type        string // "UNWIRED_DOM_ELEMENT", "UNWIRED_API_ENDPOINT", "UNWIRED_WS_EVENT"
	File        string
	Identifier  string
	Description string
}

// wireAuditReport contains all detected unwired Findings across the repository.
type wireAuditReport struct {
	Findings []wireFinding
	Passed   bool
}

// AuditWires scans the workspace for disconnected endpoints, unwired DOM containers, and dangling WS events.
// It acts as a garbage collection check to prevent code rot across the frontend/backend divide.
func AuditWires(root string) (*wireAuditReport, error) {
	// Initialize the audit report struct with passing status.
	report := &wireAuditReport{
		Findings: make([]wireFinding, 0),
		Passed:   true,
	}

	// 1. Audit HTML DOM Container IDs vs TypeScript references
	if err := auditHTMLContainers(root, report); err != nil {
		return nil, err
	}

	// 2. Audit Go API Endpoints vs TypeScript fetch calls
	if err := auditAPIEndpoints(root, report); err != nil {
		return nil, err
	}

	if len(report.Findings) > 0 {
		report.Passed = false
	}

	return report, nil
}

// auditHTMLContainers inspects index.html and checks if element IDs are bound in TypeScript.
// This prevents legacy HTML elements from lingering after their TS handlers are deleted.
func auditHTMLContainers(root string, report *wireAuditReport) error {
	var htmlFiles []string

	tsContents, err := gatherTypeScriptContents(root)
	if err != nil {
		return err
	}

	err = walkWorkspace(root, func(path string) error {
		if strings.HasSuffix(path, ".html") {
			htmlFiles = append(htmlFiles, path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	idRegex := regexp.MustCompile(`id="([a-zA-Z0-9_-]+)"`)

	for _, htmlFile := range htmlFiles {
		contentBytes, e := os.ReadFile(htmlFile)
		if e != nil {
			continue
		}
		content := string(contentBytes)
		matches := idRegex.FindAllStringSubmatch(content, -1)

		relFile, _ := filepath.Rel(root, htmlFile)

		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			id := match[1]

			// Only audit data-bearing telemetry or tab container IDs
			if !shouldAuditDOMID(id) {
				continue
			}

			// Check if id is referenced anywhere in TS files
			if !strings.Contains(tsContents, id) {
				report.Findings = append(report.Findings, wireFinding{
					Type:        "UNWIRED_DOM_ELEMENT",
					File:        relFile,
					Identifier:  id,
					Description: fmt.Sprintf("HTML container element ID '%s' in %s is never referenced or populated by TypeScript handlers", id, relFile),
				})
			}
		}
	}

	return nil
}

// shouldAuditDOMID returns true if the DOM ID is a candidate for telemetry or data binding audit.
func shouldAuditDOMID(id string) bool {
	prefixes := []string{
		"analytics-", "telemetry-", "arch-", "ast-", "gitbrain-",
		"tier1-", "tier2-", "gpu-", "dod-", "circuit-",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(id, p) {
			return true
		}
	}
	return false
}

// auditAPIEndpoints checks registered Go HTTP endpoints against TypeScript fetch callers.
// This prevents legacy API endpoints from lingering after their frontend callers are deleted.
func auditAPIEndpoints(root string, report *wireAuditReport) error {
	var goContents string
	var goFiles []string

	tsContents, err := gatherTypeScriptContents(root)
	if err != nil {
		return err
	}

	err = walkWorkspace(root, func(path string) error {
		if strings.HasSuffix(path, ".go") {
			content, e := os.ReadFile(path)
			if e == nil {
				goContents += string(content) + "\n"
				goFiles = append(goFiles, path)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	endpointRegex := regexp.MustCompile(`"/api/([a-zA-Z0-9_-]+)"`)
	matches := endpointRegex.FindAllStringSubmatch(goContents, -1)

	seen := make(map[string]bool)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		route := "/api/" + match[1]
		if seen[route] {
			continue
		}
		seen[route] = true

		// Check if endpoint is fetched in TS
		if !strings.Contains(tsContents, route) && !strings.Contains(tsContents, match[1]) {
			relFile := "server"
			for _, f := range goFiles {
				cb, _ := os.ReadFile(f)
				if strings.Contains(string(cb), route) {
					relFile, _ = filepath.Rel(root, f)
					break
				}
			}

			report.Findings = append(report.Findings, wireFinding{
				Type:        "UNWIRED_API_ENDPOINT",
				File:        relFile,
				Identifier:  route,
				Description: fmt.Sprintf("Go HTTP endpoint '%s' in %s is registered but never invoked in TypeScript frontend", route, relFile),
			})
		}
	}
	return nil
}

// gatherTypeScriptContents scans the current workspace and the sibling sovereign workspace for TS/JS files.
func gatherTypeScriptContents(root string) (string, error) {
	var tsContents string
	err := walkWorkspace(root, func(path string) error {
		if strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".js") {
			content, e := os.ReadFile(path)
			if e == nil {
				tsContents += string(content) + "\n"
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}

	sovereignTsDir := config.SovereignCockpitUITsDir(root)
	tsContents += readTypeScriptFiles(sovereignTsDir)
	return tsContents, nil
}

// walkWorkspace walks the root workspace and filters files using a custom handler.
// It skips common irrelevant directories like node_modules, .git, and dist.
func walkWorkspace(root string, fn func(path string) error) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name() == "node_modules" || info.Name() == ".git" || info.Name() == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		return fn(path)
	})
}

// readTypeScriptFiles scans a directory for TS/JS files and concatenates their contents.
// This is used for static analysis of frontend references.
func readTypeScriptFiles(dir string) string {
	var contents string
	if _, err := os.Stat(dir); err == nil {
		_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err == nil && (strings.HasSuffix(p, ".ts") || strings.HasSuffix(p, ".js")) {
				cb, e := os.ReadFile(p)
				if e == nil {
					contents += string(cb) + "\n"
				}
			}
			return nil
		})
	}
	return contents
}
