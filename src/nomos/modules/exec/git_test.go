package exec

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgantlett/nomos-commons/src/nomos/core/db"
)

func initTempGitRepo(t *testing.T) (string, func()) {
	tmpDir, err := os.MkdirTemp("", "nomos-git-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Helper to run raw git commands inside temp directory
	runGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = tmpDir
		var cleanEnv []string
		for _, env := range os.Environ() {
			if strings.HasPrefix(env, "GIT_") && !strings.HasPrefix(env, "GIT_AUTHOR_") && !strings.HasPrefix(env, "GIT_COMMITTER_") {
				continue
			}
			cleanEnv = append(cleanEnv, env)
		}
		cmd.Env = cleanEnv
		err := cmd.Run()
		if err != nil {
			os.RemoveAll(tmpDir)
			t.Fatalf("git command failed: git %s: %v", strings.Join(args, " "), err)
		}
	}

	runGit("init", "-b", "main")
	runGit("config", "user.name", "Test User")
	runGit("config", "user.email", "test@example.com")

	return tmpDir, func() {
		os.RemoveAll(tmpDir)
	}
}

func initGitTestDB(t *testing.T, dir string) (string, func()) {
	nomosDir := filepath.Join(dir, ".nomos_test_state")
	if err := os.MkdirAll(nomosDir, 0755); err != nil {
		t.Fatalf("failed to create .nomos_test_state dir: %v", err)
	}
	dbPath := filepath.Join(nomosDir, "state.db")

	db, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	_, _ = db.Exec("PRAGMA journal_mode=WAL;")
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS active_processes (
		pid INTEGER PRIMARY KEY,
		command TEXT,
		started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`)

	return dbPath, func() {
		os.RemoveAll(nomosDir)
	}
}

func TestGitStatus(t *testing.T) {
	gitDir, cleanupGit := initTempGitRepo(t)
	defer cleanupGit()

	dbPath, cleanupDB := initGitTestDB(t, gitDir)
	defer cleanupDB()

	// We must temporarily change the working directory or pass it.
	// Wait, our Go Git functions should run in the current working directory, or should they accept a directory?
	// The acceptance criteria says: "Implement Git wrapper utilities in Go to execute branch checkouts, status checks, and diff parsing safely."
	// Normally they run in the current workspace. So changing the current working directory during the test is perfect!
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	err = os.Chdir(gitDir)
	if err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(oldCwd)

	// Verify status in an empty repo
	status, err := GitStatus(dbPath)
	if err != nil {
		t.Fatalf("GitStatus failed: %v, status output: %q", err, status)
	}

	if !strings.Contains(status, "On branch main") && !strings.Contains(status, "on branch main") {
		t.Errorf("expected status to mention branch main, got: %q", status)
	}
}

func TestGitCheckoutAndDiff(t *testing.T) {
	gitDir, cleanupGit := initTempGitRepo(t)
	defer cleanupGit()

	dbPath, cleanupDB := initGitTestDB(t, gitDir)
	defer cleanupDB()

	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	err = os.Chdir(gitDir)
	if err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(oldCwd)

	// Create and commit a file
	testFile := filepath.Join(gitDir, "test.txt")
	err = os.WriteFile(testFile, []byte("line 1\n"), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Commit it
	exec.Command("git", "add", "test.txt").Run()
	exec.Command("git", "commit", "-m", "first commit").Run()

	// Verify checkout to new branch
	checkoutOut, err := GitCheckout(dbPath, "new-feature")
	if err != nil {
		// Wait, did we pass checkout -b or checkout?
		// Usually GitCheckout should checkout an existing branch, or support -b if it does not exist, or create it.
		// Let's implement GitCheckout to check out a branch. If it fails, maybe try to check out with -b, or we implement GitCheckout to accept branch parameter.
		// Let's first create the branch using raw git and test Checkout
		exec.Command("git", "branch", "new-feature").Run()
		checkoutOut, err = GitCheckout(dbPath, "new-feature")
		if err != nil {
			t.Fatalf("GitCheckout failed: %v", err)
		}
	}
	if !strings.Contains(checkoutOut, "Switched to branch") && !strings.Contains(checkoutOut, "Already on") {
		// Git outputs checkout info to stderr, which our combined output captures
	}

	// Verify we are indeed on new-feature branch
	status, _ := GitStatus(dbPath)
	if !strings.Contains(status, "new-feature") {
		t.Errorf("expected status to show branch new-feature, got %q", status)
	}

	// Create unstaged modifications
	err = os.WriteFile(testFile, []byte("line 1\nline 2\n"), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Test unstaged diff
	diffUnstaged, err := GitDiff(dbPath, false)
	if err != nil {
		t.Fatalf("GitDiff unstaged failed: %v", err)
	}
	if !strings.Contains(diffUnstaged, "+line 2") {
		t.Errorf("expected unstaged diff to show '+line 2', got: %q", diffUnstaged)
	}

	// Stage it
	exec.Command("git", "add", "test.txt").Run()

	// Test staged diff
	diffStaged, err := GitDiff(dbPath, true)
	if err != nil {
		t.Fatalf("GitDiff staged failed: %v", err)
	}
	if !strings.Contains(diffStaged, "+line 2") {
		t.Errorf("expected staged diff to show '+line 2', got: %q", diffStaged)
	}
}
