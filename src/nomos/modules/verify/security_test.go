package verify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanSecurity(t *testing.T) {
	tempDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tempDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	// Create a subdirectory that should be scanned
	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}

	// Create node_modules directory which should be skipped
	nodeModulesDir := filepath.Join(tempDir, "node_modules")
	if err := os.MkdirAll(nodeModulesDir, 0755); err != nil {
		t.Fatalf("failed to create node_modules: %v", err)
	}

	// File with a secret in src (not ignored)
	secretFile := filepath.Join(srcDir, "auth.py")
	secretContent := `
# Some header info
aws_key = "AKIA1234567890ABCDEF" # AWS key
# xoxb-12345 is not valid length but slack token regex is xox[bpsa]-[a-zA-Z0-9-]+
slack_token = "xoxb-abc-123-xyz"
`
	if err := os.WriteFile(secretFile, []byte(secretContent), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// File in skipped directory
	skippedFile := filepath.Join(nodeModulesDir, "skipped.js")
	if err := os.WriteFile(skippedFile, []byte(`eval("1+1"); ghp_123456789012345678901234567890123456`), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// File with telemetry
	telemetryFile := filepath.Join(srcDir, "analytics.ts")
	telemetryContent := `
import { posthog } from 'posthog-js';
posthog.init('test_key');
`
	if err := os.WriteFile(telemetryFile, []byte(telemetryContent), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// File with eval and XSS sinks
	vulnFile := filepath.Join(srcDir, "index.js")
	vulnContent := `
function render(html) {
	eval("console.log(html)");
	document.getElementById("output").innerHTML = html;
}
`
	if err := os.WriteFile(vulnFile, []byte(vulnContent), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// File with hardcoded URL
	urlFile := filepath.Join(srcDir, "network.go")
	urlContent := `
package src
const api = "http://my-api-server.com/endpoint"
const home = "https://github.com/my-project" // github.com is ignored
`
	if err := os.WriteFile(urlFile, []byte(urlContent), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	findings, err := ScanSecurity(tempDir)
	if err != nil {
		t.Fatalf("ScanSecurity returned error: %v", err)
	}

	// We expect:
	// 1. AWS Key in auth.py (CRITICAL)
	// 2. Slack token in auth.py (CRITICAL)
	// 3. Posthog / Telemetry in analytics.ts (HIGH)
	// 4. eval in index.js (HIGH)
	// 5. innerHTML in index.js (HIGH)
	// 6. Hardcoded URL http://my-api-server.com/endpoint in network.go (MEDIUM)

	foundAWS := false
	foundSlack := false
	foundTelemetry := false
	foundEval := false
	foundXSS := false
	foundURL := false
	foundSkipped := false

	for _, f := range findings {
		if filepath.Base(f.File) == "skipped.js" {
			foundSkipped = true
		}
		if filepath.Base(f.File) == "auth.py" && f.Severity == "CRITICAL" {
			if f.Line == 3 {
				foundAWS = true
			}
			if f.Line == 5 {
				foundSlack = true
			}
		}
		if filepath.Base(f.File) == "analytics.ts" && f.Severity == "HIGH" && f.Line == 2 {
			foundTelemetry = true
		}
		if filepath.Base(f.File) == "index.js" && f.Severity == "HIGH" {
			if f.Line == 3 {
				foundEval = true
			}
			if f.Line == 4 {
				foundXSS = true
			}
		}
		if filepath.Base(f.File) == "network.go" && f.Severity == "MEDIUM" && f.Line == 3 {
			foundURL = true
		}
	}

	if foundSkipped {
		t.Errorf("expected node_modules/skipped.js to be skipped, but findings were reported")
	}
	if !foundAWS {
		t.Errorf("expected to find AWS Key on line 3 of auth.py")
	}
	if !foundSlack {
		t.Errorf("expected to find Slack token on line 5 of auth.py")
	}
	if !foundTelemetry {
		t.Errorf("expected to find Telemetry SDK in analytics.ts")
	}
	if !foundEval {
		t.Errorf("expected to find eval in index.js")
	}
	if !foundXSS {
		t.Errorf("expected to find innerHTML XSS sink in index.js")
	}
	if !foundURL {
		t.Errorf("expected to find hardcoded URL in network.go")
	}
}
