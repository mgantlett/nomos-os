package verify

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"
)

func execCommand(dir string, name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return cmd
}

func TestCheckQualityDebtBypass(t *testing.T) {
	tmpDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tmpDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	agentDir := filepath.Join(workspace.MustNewContext(tmpDir).DataDir(), "state")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("failed to create agent dir: %v", err)
	}

	// Mock phase state to indicate Tier2 agent so bypasses are processed
	_ = os.MkdirAll(workspace.MustNewContext(tmpDir).StateDir(), 0755)
	phaseStatePath := workspace.MustNewContext(tmpDir).NomosStatePath(".phase_state.json")
	_ = os.WriteFile(phaseStatePath, []byte(`{"agent": "aider", "agent_tier": "tier2"}`), 0644)

	manifestPath := filepath.Join(agentDir, "quality_debt.json")

	// 1. Check with no file (should return false)
	bypassed, _ := CheckQualityDebtBypass(tmpDir, "src/main.go", "go_format")
	if bypassed {
		t.Errorf("expected CheckQualityDebtBypass to return false when manifest doesn't exist")
	}

	// 2. Setup manifest with active and expired bypasses
	manifest := QualityDebtManifest{
		ActiveDebt: []QualityDebtItem{
			{
				File:       "src/main.go",
				Gate:       "go_format",
				Reason:     "legacy layout",
				LinkedTask: "123",
				CreatedAt:  time.Now().Format(time.RFC3339),
				ExpiresAt:  time.Now().Add(24 * time.Hour).Format(time.RFC3339), // active
			},
			{
				File:       "src/expired.go",
				Gate:       "go_vet",
				Reason:     "unreachable code",
				LinkedTask: "456",
				CreatedAt:  time.Now().Add(-48 * time.Hour).Format(time.RFC3339),
				ExpiresAt:  time.Now().Add(-24 * time.Hour).Format(time.RFC3339), // expired
			},
		},
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// 3. Test active bypass
	bypassed, taskID := CheckQualityDebtBypass(tmpDir, filepath.Join(tmpDir, "src/main.go"), "go_format")
	if !bypassed {
		t.Errorf("expected main.go to be bypassed")
	}
	if taskID != "123" {
		t.Errorf("expected linked task to be '123', got %q", taskID)
	}

	// 4. Test expired bypass (should return false)
	bypassed, _ = CheckQualityDebtBypass(tmpDir, filepath.Join(tmpDir, "src/expired.go"), "go_vet")
	if bypassed {
		t.Errorf("expected expired.go to not be bypassed")
	}

	// 5. Test missing bypass
	bypassed, _ = CheckQualityDebtBypass(tmpDir, filepath.Join(tmpDir, "src/missing.go"), "go_format")
	if bypassed {
		t.Errorf("expected missing.go to not be bypassed")
	}
}

func TestStageAutoDebtTask(t *testing.T) {
	tmpDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tmpDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	// Stub out git init so git add works inside the temp repo
	gitCmd := execCommand(tmpDir, "git", "init")
	_ = gitCmd.Run()

	// Mock phase state to indicate low-tier agent so bypasses are created
	_ = os.MkdirAll(workspace.MustNewContext(tmpDir).StateDir(), 0755)
	phaseStatePath := workspace.MustNewContext(tmpDir).NomosStatePath(".phase_state.json")
	_ = os.WriteFile(phaseStatePath, []byte(`{"agent": "aider", "agent_tier": "tier2"}`), 0644)

	StageAutoDebtTask(tmpDir, filepath.Join(tmpDir, "src/dirty.go"), "doc_drift", "drift detected")

	agentDir := filepath.Join(workspace.MustNewContext(tmpDir).DataDir(), "state")
	manifestPath := filepath.Join(agentDir, "quality_debt.json")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Fatalf("expected quality_debt.json to be created")
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read quality_debt.json: %v", err)
	}

	var manifest QualityDebtManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("failed to unmarshal quality_debt.json: %v", err)
	}

	if len(manifest.ActiveDebt) != 1 {
		t.Fatalf("expected 1 active debt entry, got %d", len(manifest.ActiveDebt))
	}

	item := manifest.ActiveDebt[0]
	if item.File != "src/dirty.go" {
		t.Errorf("expected file 'src/dirty.go', got %q", item.File)
	}
	if item.Gate != "doc_drift" {
		t.Errorf("expected gate 'doc_drift', got %q", item.Gate)
	}
	if item.LinkedTask != "AUTO" {
		t.Errorf("expected task 'AUTO', got %q", item.LinkedTask)
	}
}

func TestStageAutoDebtTask_AgentTierHigh(t *testing.T) {
	tmpDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tmpDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	agentDir := filepath.Join(workspace.MustNewContext(tmpDir).DataDir(), "state")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("failed to create agent dir: %v", err)
	}

	_ = os.MkdirAll(workspace.MustNewContext(tmpDir).StateDir(), 0755)
	phaseStatePath := workspace.MustNewContext(tmpDir).NomosStatePath(".phase_state.json")
	stateContent := `{"agent": "antigravity", "agent_tier": "tier1"}`
	if err := os.WriteFile(phaseStatePath, []byte(stateContent), 0644); err != nil {
		t.Fatalf("failed to write mock phase state: %v", err)
	}

	StageAutoDebtTask(tmpDir, filepath.Join(tmpDir, "src/dirty.go"), "doc_drift", "drift detected")

	manifestPath := filepath.Join(agentDir, "quality_debt.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Errorf("expected quality_debt.json to be created under Unified Single-Standard DoD")
	}
}

func TestStageAutoDebtTask_AgentTierLow(t *testing.T) {
	tmpDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tmpDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	agentDir := filepath.Join(workspace.MustNewContext(tmpDir).DataDir(), "state")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("failed to create agent dir: %v", err)
	}

	_ = os.MkdirAll(workspace.MustNewContext(tmpDir).StateDir(), 0755)
	phaseStatePath := workspace.MustNewContext(tmpDir).NomosStatePath(".phase_state.json")
	stateContent := `{"agent": "aider", "agent_tier": "tier2"}`
	if err := os.WriteFile(phaseStatePath, []byte(stateContent), 0644); err != nil {
		t.Fatalf("failed to write mock phase state: %v", err)
	}

	StageAutoDebtTask(tmpDir, filepath.Join(tmpDir, "src/dirty.go"), "doc_drift", "drift detected")

	manifestPath := filepath.Join(agentDir, "quality_debt.json")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Errorf("expected quality_debt.json to be created when AI agent tier is low, but it wasn't: %v", err)
	}
}

func TestSyncQualityDebtManifest(t *testing.T) {
	tmpDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tmpDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	// Stub git init
	_ = execCommand(tmpDir, "git", "init").Run()

	agentDir := filepath.Join(workspace.MustNewContext(tmpDir).DataDir(), "state")
	_ = os.MkdirAll(agentDir, 0755)

	// Create a clean Go file (formatted)
	cleanGoFile := filepath.Join(tmpDir, "clean.go")
	_ = os.WriteFile(cleanGoFile, []byte("package main\n\nfunc main() {}\n"), 0644)

	// Create an unformatted Go file
	unformattedGoFile := filepath.Join(tmpDir, "dirty.go")
	_ = os.WriteFile(unformattedGoFile, []byte("package main\n  func  main()  {}\n"), 0644)

	manifest := QualityDebtManifest{
		ActiveDebt: []QualityDebtItem{
			{
				File:       "clean.go",
				Gate:       "go_format",
				Reason:     "legacy layout",
				LinkedTask: "123",
				ExpiresAt:  time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
			{
				File:       "dirty.go",
				Gate:       "go_format",
				Reason:     "dirty indent",
				LinkedTask: "123",
				ExpiresAt:  time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
			{
				File:       "deleted.go",
				Gate:       "go_format",
				Reason:     "missing",
				LinkedTask: "123",
				ExpiresAt:  time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
		},
	}

	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	manifestPath := filepath.Join(agentDir, "quality_debt.json")
	_ = os.WriteFile(manifestPath, manifestBytes, 0644)

	SyncQualityDebtManifest(tmpDir)

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read manifest: %v", err)
	}

	var synced QualityDebtManifest
	_ = json.Unmarshal(data, &synced)

	// 'clean.go' (formatted) should be removed.
	// 'deleted.go' (deleted) should be removed.
	// 'dirty.go' (unformatted) should remain.
	if len(synced.ActiveDebt) != 1 {
		t.Fatalf("expected exactly 1 active debt item left, got %d", len(synced.ActiveDebt))
	}

	if synced.ActiveDebt[0].File != "dirty.go" {
		t.Errorf("expected dirty.go to remain, got %q", synced.ActiveDebt[0].File)
	}
}

func TestPruneQualityDebtForTask(t *testing.T) {
	tmpDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tmpDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	_ = execCommand(tmpDir, "git", "init").Run()

	agentDir := filepath.Join(workspace.MustNewContext(tmpDir).DataDir(), "state")
	_ = os.MkdirAll(agentDir, 0755)

	manifest := QualityDebtManifest{
		ActiveDebt: []QualityDebtItem{
			{
				File:       "file1.go",
				Gate:       "go_format",
				Reason:     "legacy layout",
				LinkedTask: "61",
				ExpiresAt:  time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
			{
				File:       "file2.go",
				Gate:       "go_format",
				Reason:     "dirty indent",
				LinkedTask: "999",
				ExpiresAt:  time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
		},
	}

	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	manifestPath := filepath.Join(agentDir, "quality_debt.json")
	_ = os.WriteFile(manifestPath, manifestBytes, 0644)

	PruneQualityDebtForTask(tmpDir, "61")

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read manifest: %v", err)
	}

	var synced QualityDebtManifest
	_ = json.Unmarshal(data, &synced)

	if len(synced.ActiveDebt) != 1 {
		t.Fatalf("expected exactly 1 active debt item left, got %d", len(synced.ActiveDebt))
	}

	if synced.ActiveDebt[0].LinkedTask != "999" {
		t.Errorf("expected task 999 to remain, got %q", synced.ActiveDebt[0].LinkedTask)
	}
}

func TestSyncQualityDebtManifest_AutoClear(t *testing.T) {
	tmpDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tmpDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	_ = execCommand(tmpDir, "git", "init").Run()

	agentDir := filepath.Join(workspace.MustNewContext(tmpDir).DataDir(), "state")
	_ = os.MkdirAll(agentDir, 0755)

	// 1. Create a clean Go file (formatted and has a matching test file)
	libFile := filepath.Join(tmpDir, "lib.go")
	_ = os.WriteFile(libFile, []byte("package main\n\nfunc MyFunc() {}\n"), 0644)

	testFile := filepath.Join(tmpDir, "lib_test.go")
	_ = os.WriteFile(testFile, []byte("package main\n\nimport \"testing\"\n\nfunc TestMyFunc(t *testing.T) {}\n"), 0644)

	manifest := QualityDebtManifest{
		ActiveDebt: []QualityDebtItem{
			{
				File:       "lib.go",
				Gate:       "tdd_coverage",
				Reason:     "missing test",
				LinkedTask: "AUTO",
				ExpiresAt:  time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
			{
				File:       "lib.go",
				Gate:       "doc_drift",
				Reason:     "doc drift",
				LinkedTask: "AUTO",
				ExpiresAt:  time.Now().Add(24 * time.Hour).Format(time.RFC3339),
			},
		},
	}

	manifestBytes, _ := json.MarshalIndent(manifest, "", "  ")
	manifestPath := filepath.Join(agentDir, "quality_debt.json")
	_ = os.WriteFile(manifestPath, manifestBytes, 0644)

	SyncQualityDebtManifest(tmpDir)

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read manifest: %v", err)
	}

	var synced QualityDebtManifest
	_ = json.Unmarshal(data, &synced)

	// Since 'lib_test.go' exists, 'tdd_coverage' should be resolved and pruned!
	// Since 'lib.go' is not in git staged files (it is untracked), 'doc_drift' is not staged and thus should be pruned!
	if len(synced.ActiveDebt) != 0 {
		t.Fatalf("expected all active debt items to be resolved and pruned, got %d items", len(synced.ActiveDebt))
	}
}
