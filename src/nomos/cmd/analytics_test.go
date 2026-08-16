package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

var nomosBinPath string

func TestMain(m *testing.M) {
	// Compile nomos binary
	tmpDir, err := os.MkdirTemp("", "nomos-build-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp build dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	binPath := filepath.Join(tmpDir, "nomos")
	// Build with CGO_ENABLED=0 since cgo compiler gcc isn't available
	cmd := exec.Command("go", "build", "-o", binPath, "../main.go")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build nomos: %v, output: %s\n", err, out)
		os.Exit(1)
	}

	nomosBinPath = binPath
	os.Exit(m.Run())
}

// Custom require helper to match implementation plan API without introducing github.com/stretchr/testify
type requireHelper struct {
	t *testing.T
}

func newRequire(t *testing.T) *requireHelper {
	return &requireHelper{t: t}
}

func (r *requireHelper) NoError(err error, msg ...interface{}) {
	r.t.Helper()
	if err != nil {
		if len(msg) > 0 {
			r.t.Fatalf("unexpected error: %v: %v", err, fmt.Sprint(msg...))
		} else {
			r.t.Fatalf("unexpected error: %v", err)
		}
	}
}

func (r *requireHelper) NotEmpty(val string, msg ...interface{}) {
	r.t.Helper()
	if val == "" {
		if len(msg) > 0 {
			r.t.Fatalf("expected non-empty string: %v", fmt.Sprint(msg...))
		} else {
			r.t.Fatalf("expected non-empty string")
		}
	}
}

func (r *requireHelper) Equal(expected, actual interface{}, msg ...interface{}) {
	r.t.Helper()
	if fmt.Sprintf("%v", expected) != fmt.Sprintf("%v", actual) {
		r.t.Fatalf("expected %v, got %v: %v", expected, actual, msg)
	}
}

func (r *requireHelper) True(cond bool, msg ...interface{}) {
	r.t.Helper()
	if !cond {
		r.t.Fatalf("expected condition to be true: %v", msg)
	}
}

func (r *requireHelper) Len(val interface{}, expectedLen int, msg ...interface{}) {
	r.t.Helper()
	var actualLen int
	switch v := val.(type) {
	case []map[string]json.RawMessage:
		actualLen = len(v)
	case []map[string]interface{}:
		actualLen = len(v)
	default:
		r.t.Fatalf("unsupported type for Len check")
	}
	if actualLen != expectedLen {
		r.t.Fatalf("expected length %d, got %d: %v", expectedLen, actualLen, msg)
	}
}

func (r *requireHelper) NotNil(val interface{}, msg ...interface{}) {
	r.t.Helper()
	if val == nil {
		r.t.Fatalf("expected non-nil value: %v", msg)
	}
}

func (r *requireHelper) Contains(str, substr string, msg ...interface{}) {
	r.t.Helper()
	if !strings.Contains(str, substr) {
		r.t.Fatalf("expected string %q to contain %q: %v", str, substr, msg)
	}
}

func runNomosCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var dir string
	actualArgs := args
	if len(args) > 0 {
		lastArg := args[len(args)-1]
		if info, err := os.Stat(lastArg); err == nil && info.IsDir() {
			dir = lastArg
			actualArgs = args[:len(args)-1]
		}
	}

	cmd := exec.Command(nomosBinPath, actualArgs...)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "NOMOS_TEST_MODE=1")
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Test 1: Verify Command Registered
func TestAnalyticsCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range RootCmd.Commands() {
		if cmd.Name() == "analytics" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'analytics' command to be registered under RootCmd")
	}
}

// Test 2: Verify JSON Flag Exists
func TestAnalyticsJSONFlagExists(t *testing.T) {
	flag, err := analyticsCmd.Flags().GetBool("json")
	if err != nil {
		t.Fatalf("expected '--json' flag to be registered on analytics command: %v", err)
	}
	if flag {
		t.Error("expected '--json' flag to default to false")
	}
}

// Test 3: Verify output is valid JSON
func TestAnalyticsJSONOutputIsValidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nomos-analytics-test-*")
	require := newRequire(t)
	require.NoError(err)
	defer os.RemoveAll(tmpDir)

	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v, output: %s", args, err, out)
		}
	}

	git("init")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test")
	git("commit", "--allow-empty", "-m", "feat: initial commit")

	out, err := runNomosCmd(t, "analytics", "--json", tmpDir)
	require.NoError(err)
	require.NotEmpty(out)

	var result map[string]json.RawMessage
	err = json.Unmarshal([]byte(out), &result)
	require.NoError(err)

	expectedKeys := []string{
		"period", "total_commits", "weekly_velocity",
		"commit_types", "telemetry", "module_ratings",
	}
	for _, key := range expectedKeys {
		_, ok := result[key]
		if !ok {
			t.Errorf("expected JSON to contain key %q", key)
		}
	}
}

// Test 4: Verify Weekly Velocity Shape
func TestAnalyticsJSONWeeklyVelocityShape(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nomos-analytics-test-*")
	require := newRequire(t)
	require.NoError(err)
	defer os.RemoveAll(tmpDir)

	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v, output: %s", args, err, out)
		}
	}

	git("init")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test")
	git("commit", "--allow-empty", "-m", "feat: initial")

	out, err := runNomosCmd(t, "analytics", "--json", tmpDir)
	require.NoError(err)

	var result map[string]json.RawMessage
	err = json.Unmarshal([]byte(out), &result)
	require.NoError(err)

	rawArr, ok := result["weekly_velocity"]
	require.True(ok, "weekly_velocity key missing")

	var arr []map[string]interface{}
	err = json.Unmarshal(rawArr, &arr)
	require.NoError(err)
	require.Len(arr, 4, "expected exactly 4 weekly entries")

	for i, entry := range arr {
		wkVal := entry["week"]
		cmVal := entry["commits"]

		var wk string
		if f, ok := wkVal.(float64); ok {
			wk = fmt.Sprintf("%.0f", f)
		} else if s, ok := wkVal.(string); ok {
			wk = s
		}

		var cm string
		if f, ok := cmVal.(float64); ok {
			cm = fmt.Sprintf("%.0f", f)
		} else if s, ok := cmVal.(string); ok {
			cm = s
		}

		require.Equal(fmt.Sprintf("%d", i+1), wk, fmt.Sprintf("week %d: expected ordinal label", i))
		require.NotEmpty(cm, fmt.Sprintf("week %d: commits must be non-empty", i))
	}
}

// Test 5: Verify Commit Types Shape
func TestAnalyticsJSONCommitTypesShape(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nomos-analytics-test-*")
	require := newRequire(t)
	require.NoError(err)
	defer os.RemoveAll(tmpDir)

	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v, output: %s", args, err, out)
		}
	}

	git("init")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test")
	git("commit", "--allow-empty", "-m", "feat: something")

	out, err := runNomosCmd(t, "analytics", "--json", tmpDir)
	require.NoError(err)

	var result map[string]json.RawMessage
	err = json.Unmarshal([]byte(out), &result)
	require.NoError(err)

	raw, ok := result["commit_types"]
	require.True(ok, "commit_types key missing")

	var types map[string]json.RawMessage
	err = json.Unmarshal(raw, &types)
	require.NoError(err)

	for _, expected := range []string{"feat", "fix", "docs", "refactor", "chore"} {
		_, present := types[expected]
		require.True(present, fmt.Sprintf("commit_types must contain key %q", expected))
	}
}

// Test 6: Verify Telemetry Defaults
func TestAnalyticsJSONTelemetryDefaults(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nomos-analytics-test-*")
	require := newRequire(t)
	require.NoError(err)
	defer os.RemoveAll(tmpDir)

	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v, output: %s", args, err, out)
		}
	}

	git("init")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test")
	git("commit", "--allow-empty", "-m", "chore: init")

	out, err := runNomosCmd(t, "analytics", "--json", tmpDir)
	require.NoError(err)

	var result map[string]json.RawMessage
	err = json.Unmarshal([]byte(out), &result)
	require.NoError(err)

	telemetryRaw, ok := result["telemetry"]
	require.True(ok, "telemetry key missing")

	var telemetry map[string]json.RawMessage
	err = json.Unmarshal(telemetryRaw, &telemetry)
	require.NoError(err)

	expectedTelemetry := map[string]string{
		"total_events": "0",
		"transitions":  "0",
		"failures":     "0",
		"lockouts":     "0",
		"bypasses":     "0",
		"bypass_ratio": "\"0.0%\"",
	}
	for key, expected := range expectedTelemetry {
		actual, ok := telemetry[key]
		require.True(ok, fmt.Sprintf("telemetry missing key %q", key))
		require.Equal(expected, string(actual), fmt.Sprintf("telemetry[%q] mismatch", key))
	}
}

// Test 7: Verify Module Ratings Empty
func TestAnalyticsJSONModuleRatingsEmpty(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nomos-analytics-test-*")
	require := newRequire(t)
	require.NoError(err)
	defer os.RemoveAll(tmpDir)

	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v, output: %s", args, err, out)
		}
	}

	git("init")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test")
	git("commit", "--allow-empty", "-m", "docs: readme")

	out, err := runNomosCmd(t, "analytics", "--json", tmpDir)
	require.NoError(err)

	var result map[string]json.RawMessage
	err = json.Unmarshal([]byte(out), &result)
	require.NoError(err)

	raw, ok := result["module_ratings"]
	require.True(ok, "module_ratings key missing")

	var ratings []map[string]json.RawMessage
	err = json.Unmarshal(raw, &ratings)
	require.NoError(err)
	require.NotNil(ratings, "module_ratings must be a non-nil JSON array")
	require.Len(ratings, 0, "module_ratings must be empty when no phase_state.json exists")
}

// Test 8: Verify Deterministic Output
func TestAnalyticsJSONDeterministicOutput(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nomos-analytics-test-*")
	require := newRequire(t)
	require.NoError(err)
	defer os.RemoveAll(tmpDir)

	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v, output: %s", args, err, out)
		}
	}

	git("init")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test")
	git("commit", "--allow-empty", "-m", "feat: stable")

	// Create a dummy phase_state.json with multiple out-of-order modules to verify determinism
	err = os.MkdirAll(filepath.Join(tmpDir, ".agent"), 0755)
	require.NoError(err)

	phaseStateContent := `{
		"module_metrics": {
			"auth": {
				"success_count": 10,
				"failed_runs": 2,
				"consecutive_fails": 0
			},
			"core": {
				"success_count": 5,
				"failed_runs": 0,
				"consecutive_fails": 0
			},
			"api": {
				"success_count": 20,
				"failed_runs": 1,
				"consecutive_fails": 0
			}
		}
	}`
	_ = os.MkdirAll(workspace.MustNewContext(tmpDir).StateDir(), 0755)
	err = os.WriteFile(workspace.MustNewContext(tmpDir).NomosStatePath(".phase_state.json"), []byte(phaseStateContent), 0644)
	require.NoError(err)

	out1, err := runNomosCmd(t, "analytics", "--json", tmpDir)
	require.NoError(err)
	out2, err := runNomosCmd(t, "analytics", "--json", tmpDir)
	require.NoError(err)

	require.Equal(out1, out2, "analytics --json must produce deterministic output")
}

// Test 9: Verify Text Output Unchanged
func TestAnalyticsJSONTextOutputUnchanged(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nomos-analytics-test-*")
	require := newRequire(t)
	require.NoError(err)
	defer os.RemoveAll(tmpDir)

	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v, output: %s", args, err, out)
		}
	}

	git("init")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test")
	git("commit", "--allow-empty", "-m", "fix: patch")

	// Create dummy telemetry and phase state so all sections get rendered
	err = os.MkdirAll(workspace.MustNewContext(tmpDir).NomosStatePath("logs"), 0755)
	if err != nil {
		t.Fatalf("Failed to create log dir: %v", err)
	}

	telemetryContent := `{"event_type": "phase_transition", "detail": "init"}
`

	err = os.WriteFile(filepath.Join(workspace.MustNewContext(tmpDir).NomosStatePath("logs"), "telemetry.jsonl"), []byte(telemetryContent), 0644)
	require.NoError(err)

	phaseStateContent := `{
		"module_metrics": {
			"auth": {
				"success_count": 10,
				"failed_runs": 2,
				"consecutive_fails": 0
			}
		}
	}`
	_ = os.MkdirAll(workspace.MustNewContext(tmpDir).StateDir(), 0755)
	err = os.WriteFile(workspace.MustNewContext(tmpDir).NomosStatePath(".phase_state.json"), []byte(phaseStateContent), 0644)
	require.NoError(err)

	out, err := runNomosCmd(t, "analytics", tmpDir)
	require.NoError(err)

	requiredHeaders := []string{
		"Weekly Velocity Trend",
		"Commit Type Distribution",
		"Telemetric Event Analytics",
		"Codebase Module Competence Ratings",
	}
	for _, h := range requiredHeaders {
		require.Contains(out, h, fmt.Sprintf("expected header %q in text output", h))
	}
}
