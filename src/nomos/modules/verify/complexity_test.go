package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyzeComplexityGo(t *testing.T) {
	tempDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tempDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	// Initialize Git repository
	if _, err := runGit(tempDir, "init"); err != nil {
		t.Fatalf("failed to init git: %v", err)
	}

	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}

	// Write complex Go file (complexity = 16)
	complexGo := `package src
func veryComplex() {
	a := 1
	if a == 1 { // +1 (2)
		if a == 2 { // +1 (3)
			for i := 0; i < 10; i++ { // +1 (4)
				switch a {
				case 1: // +1 (5)
					if a == 3 { // +1 (6)
					}
				case 2: // +1 (7)
					if a == 4 { // +1 (8)
					}
				case 3: // +1 (9)
					if a == 5 { // +1 (10)
					}
				case 4: // +1 (11)
					if a == 6 { // +1 (12)
					}
				case 5: // +1 (13)
					if a == 7 { // +1 (14)
					}
				case 6: // +1 (15)
					if a == 8 { // +1 (16)
					}
				}
			}
		}
	}
}
`
	err = os.WriteFile(filepath.Join(srcDir, "complex.go"), []byte(complexGo), 0644)
	if err != nil {
		t.Fatalf("failed to write Go file: %v", err)
	}

	// Stage files
	_, _ = runGit(tempDir, "add", ".")

	findings, err := AnalyzeComplexity(tempDir, true)
	if err != nil {
		t.Fatalf("AnalyzeComplexity failed: %v", err)
	}

	if len(findings) < 1 {
		t.Fatalf("expected at least 1 finding, got 0")
	}

	hasCyclomatic := false

	for _, f := range findings {
		if strings.Contains(f.Message, "cyclomatic complexity") {
			hasCyclomatic = true
		}
	}

	if !hasCyclomatic {
		t.Errorf("expected cyclomatic complexity finding, got none in: %+v", findings)
	}
}

func TestAnalyzeComplexityNesting(t *testing.T) {
	tempDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tempDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	// Initialize Git repository
	if _, err := runGit(tempDir, "init"); err != nil {
		t.Fatalf("failed to init git: %v", err)
	}

	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}

	err = os.WriteFile(filepath.Join(tempDir, "go.work"), []byte("go 1.22\n\nuse .\n"), 0644)
	err = os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module testrepo\n\ngo 1.22\n"), 0644)
	if err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	// Write nested TS file (2-space indent, max nesting level 5)
	nestedTS := `function test() {
  if (true) {
    if (true) {
      if (true) {
        if (true) {
          console.log("very nested");
        }
      }
    }
  }
}
`
	err = os.WriteFile(filepath.Join(srcDir, "nested.ts"), []byte(nestedTS), 0644)
	if err != nil {
		t.Fatalf("failed to write nested.ts: %v", err)
	}

	// Stage files
	_, _ = runGit(tempDir, "add", ".")

	findings, err := AnalyzeComplexity(tempDir, true)
	if err != nil {
		t.Fatalf("AnalyzeComplexity failed: %v", err)
	}

	var errorsCount, warningsCount int
	for _, f := range findings {
		if f.IsError {
			errorsCount++
		} else {
			warningsCount++
		}
	}

	if errorsCount != 1 {
		t.Errorf("expected 1 nesting level violation error, got %d", errorsCount)
	}
	if warningsCount != 2 {
		t.Errorf("expected 2 nesting level warnings, got %d", warningsCount)
	}
}

func TestScopeAwareComplexity(t *testing.T) {
	tempDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tempDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	if _, err := runGit(tempDir, "init"); err != nil {
		t.Fatalf("failed to init git: %v", err)
	}
	runGit(tempDir, "config", "user.name", "Test User")
	runGit(tempDir, "config", "user.email", "test@example.com")

	err = os.WriteFile(filepath.Join(tempDir, "go.work"), []byte("go 1.22\n\nuse .\n"), 0644)
	err = os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module testrepo\n\ngo 1.22\n"), 0644)
	if err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}

	srcDir := filepath.Join(tempDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}

	// Create a very complex file and commit it (simulating existing debt)
	complexGo := `package src
import "fmt"
func veryComplex() {
	a := 1
	if a == 1 {
		if a == 2 {
			for i := 0; i < 10; i++ {
				switch a {
				case 1:
					if a == 3 {}
				case 2:
					if a == 4 {}
				case 3:
					if a == 5 {}
				case 4:
					if a == 6 {}
				case 5:
					if a == 7 {}
				case 6:
					if a == 8 {}
				}
			}
		}
	}
}
`
	goFile := filepath.Join(srcDir, "complex.go")
	os.WriteFile(goFile, []byte(complexGo), 0644)
	runGit(tempDir, "add", ".")
	runGit(tempDir, "commit", "--no-verify", "-m", "initial commit with complex file")

	// Verify that if we modify the file syntactically, it doesn't fail DoD because it didn't worsen
	modifiedComplexGo := strings.ReplaceAll(complexGo, "\"fmt\"", "\"github.com/mgantlett/nomos-commons/src/nomos/synapse\"")
	os.WriteFile(goFile, []byte(modifiedComplexGo), 0644)
	runGit(tempDir, "add", goFile)

	findings, err := AnalyzeComplexity(tempDir, true)
	if err != nil {
		t.Fatalf("AnalyzeComplexity failed: %v", err)
	}

	// It should find the complexity violation
	if len(findings) == 0 {
		t.Fatalf("expected complexity findings, got none")
	}

	// But it shouldn't worsen
	for _, f := range findings {
		if f.IsError {
			worsened := DidComplexityWorsen(tempDir, "src/complex.go", f)
			if worsened {
				t.Errorf("expected DidComplexityWorsen to return false for syntactic edit, got true")
			}
		}
	}

	// Now introduce a NEW complex file (should worsen)
	os.WriteFile(filepath.Join(srcDir, "new_complex.go"), []byte(complexGo), 0644)
	runGit(tempDir, "add", "src/new_complex.go")

	findings2, _ := AnalyzeComplexity(tempDir, true)
	for _, f := range findings2 {
		if f.IsError && f.File == "src/new_complex.go" {
			worsened := DidComplexityWorsen(tempDir, "src/new_complex.go", f)
			if !worsened {
				t.Errorf("expected DidComplexityWorsen to return true for new file, got false")
			}
		}
	}
}
