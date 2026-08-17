package verify

import (
	"github.com/mgantlett/nomos-commons/src/nomos/core/workspace"

	"os"
	"path/filepath"
	"testing"
)

func TestDoDStagesGuidance(t *testing.T) {
	for _, stage := range DoDStages {
		if stage.Guidance == "" {
			t.Errorf("DoD stage %q is missing actionable guidance description", stage.Name)
		}
	}
}

func TestVerifyDoDTDDCheck(t *testing.T) {
	tempDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tempDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	runGit := func(dir string, args ...string) {
		cmd := execGit(dir, args...)
		_ = cmd.Run()
	}
	runGit(tempDir, "init")
	runGit(tempDir, "config", "user.name", "Test User")
	runGit(tempDir, "config", "user.email", "test@example.com")

	// 1. Stage a logic file only (should fail)
	logicPath := filepath.Join(tempDir, "hello.go")
	os.WriteFile(logicPath, []byte("package main\n"), 0644)
	runGit(tempDir, "add", "hello.go")

	err = checkTDD(tempDir)
	if err == nil {
		t.Errorf("expected TDD check to fail for logic file without test, but it succeeded")
	}

	// 2. Stage its accompanying test file (should pass)
	testPath := filepath.Join(tempDir, "hello_test.go")
	os.WriteFile(testPath, []byte("package main\nimport \"testing\"\nfunc TestHello(t *testing.T) { if false { t.Error(\"dummy\") } }\n"), 0644)
	runGit(tempDir, "add", "hello_test.go")

	err = checkTDD(tempDir)
	if err != nil {
		t.Errorf("expected TDD check to succeed when test file is paired, got: %v", err)
	}
}

func TestVerifyDoDBoyScoutCheck(t *testing.T) {
	tempDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tempDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	runGit := func(dir string, args ...string) {
		cmd := execGit(dir, args...)
		_ = cmd.Run()
	}
	runGit(tempDir, "init")
	runGit(tempDir, "config", "user.name", "Test User")
	runGit(tempDir, "config", "user.email", "test@example.com")

	// 1. Stage an undocumented public function (should fail)
	badGoContent := `package main
func BadUndocumentedFunction() {}
`
	goPath := filepath.Join(tempDir, "main.go")
	os.WriteFile(goPath, []byte(badGoContent), 0644)
	runGit(tempDir, "add", "main.go")

	err = checkBoyScout(tempDir)
	if err == nil {
		t.Errorf("expected Boy Scout check to fail for undocumented public function, but it succeeded")
	}

	// 2. Stage a documented public function (should pass)
	goodGoContent := `package main
// GoodFunction is documented.
func GoodFunction() {}
`
	os.WriteFile(goPath, []byte(goodGoContent), 0644)
	runGit(tempDir, "add", "main.go")

	err = checkBoyScout(tempDir)
	if err != nil {
		t.Errorf("expected Boy Scout check to succeed for documented function, got: %v", err)
	}

	// 3. Stage an undocumented private function (should pass)
	privateGoContent := `package main
func privateFunction() {}
`
	os.WriteFile(goPath, []byte(privateGoContent), 0644)
	runGit(tempDir, "add", "main.go")

	err = checkBoyScout(tempDir)
	if err != nil {
		t.Errorf("expected Boy Scout check to succeed for private function, got: %v", err)
	}
}

func TestVerifyDoDDocDriftCheck(t *testing.T) {
	tempDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tempDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	runGit := func(dir string, args ...string) {
		cmd := execGit(dir, args...)
		_ = cmd.Run()
	}
	runGit(tempDir, "init")
	runGit(tempDir, "config", "user.name", "Test User")
	runGit(tempDir, "config", "user.email", "test@example.com")

	// 1. Stage a public boundary file only (should fail)
	cmdDir := filepath.Join(tempDir, "src", "nomos", "cmd")
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		t.Fatalf("failed to create cmd dir: %v", err)
	}
	logicPath := filepath.Join(cmdDir, "hello.go")
	os.WriteFile(logicPath, []byte("package cmd\n"), 0644)
	runGit(tempDir, "add", "src/nomos/cmd/hello.go")

	err = checkDocDrift(tempDir)
	if err == nil {
		t.Errorf("expected Doc Drift check to fail when public API file is staged without docs, but it succeeded")
	}

	// 2. Stage a doc file (should pass)
	docsDir := filepath.Join(tempDir, "docs")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		t.Fatalf("failed to create docs dir: %v", err)
	}
	docPath := filepath.Join(docsDir, "hello.md")
	os.WriteFile(docPath, []byte("# Hello\n"), 0644)
	runGit(tempDir, "add", "docs/hello.md")

	err = checkDocDrift(tempDir)
	if err != nil {
		t.Errorf("expected Doc Drift check to succeed when docs are staged, got: %v", err)
	}
}

func TestVerifyDoDGeneratedCodeBlocker(t *testing.T) {
	tempDir := t.TempDir()
	var err error
	_ = err
	if err := os.MkdirAll(filepath.Join(tempDir, ".nomos"), 0755); err != nil {
		t.Fatalf("failed to create .nomos dir: %v", err)
	}

	runGit := func(dir string, args ...string) {
		cmd := execGit(dir, args...)
		_ = cmd.Run()
	}
	runGit(tempDir, "init")
	runGit(tempDir, "config", "user.name", "Test User")
	runGit(tempDir, "config", "user.email", "test@example.com")

	// 1. Stage a generated file without skip (should fail)
	genPath := filepath.Join(tempDir, "gen.go")
	os.WriteFile(genPath, []byte("// Code generated by something. DO NOT EDIT.\npackage main\n"), 0644)
	runGit(tempDir, "add", "gen.go")

	_, err = runGeneratedCodeBlockerCheck(&workspace.WorkspaceContext{RepoRoot: tempDir})
	if err == nil {
		t.Errorf("expected Generated Code Blocker to fail for generated file, but it succeeded")
	}

	// 2. Add skip message (should pass)
	os.MkdirAll(filepath.Join(tempDir, ".git"), 0755)
	commitMsgPath := filepath.Join(tempDir, ".git", "COMMIT_EDITMSG")
	os.WriteFile(commitMsgPath, []byte("Some change\n\nGen-Skip: Intended edit\n"), 0644)

	_, err = runGeneratedCodeBlockerCheck(&workspace.WorkspaceContext{RepoRoot: tempDir})
	if err != nil {
		t.Errorf("expected Generated Code Blocker to succeed with Gen-Skip, got: %v", err)
	}
}
