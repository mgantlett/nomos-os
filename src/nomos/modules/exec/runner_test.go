// Package exec tests CLI subcommand runner functions.
package exec

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/mgantlett/nomos-commons/src/nomos/core/db"
)

func initTestDB(t *testing.T) (string, func()) {
	tmpDir, err := os.MkdirTemp("", "nomos-exec-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(tmpDir, "global_cache.db")

	db, err := db.Open(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to open database: %v", err)
	}

	_, _ = db.Exec("PRAGMA journal_mode=WAL;")

	// Create tables needed for testing
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS locks (
		lock_key TEXT PRIMARY KEY,
		owner TEXT,
		pid INTEGER,
		acquired_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create locks table: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS active_processes (
		pid INTEGER PRIMARY KEY,
		command TEXT,
		started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create active_processes table: %v", err)
	}

	return dbPath, func() {
		os.RemoveAll(tmpDir)
	}
}

func TestRunCommandSuccess(t *testing.T) {
	dbPath, cleanup := initTestDB(t)
	defer cleanup()

	out, err := RunCommand(dbPath, "", "echo", "hello-world")
	if err != nil {
		t.Fatalf("RunCommand failed: %v", err)
	}

	trimmed := strings.TrimSpace(out)
	if trimmed != "hello-world" {
		t.Errorf("expected 'hello-world', got %q", trimmed)
	}
}

func TestRunCommandErrorExit(t *testing.T) {
	dbPath, cleanup := initTestDB(t)
	defer cleanup()

	_, err := RunCommand(dbPath, "", "false")
	if err == nil {
		t.Fatalf("expected command execution to return error, got nil")
	}
}

func TestRunCommandPIDRegistration(t *testing.T) {
	dbPath, cleanup := initTestDB(t)
	defer cleanup()

	var wg sync.WaitGroup
	wg.Add(1)

	var runErr error
	go func() {
		defer wg.Done()
		_, runErr = RunCommand(dbPath, "", "sleep", "1")
	}()

	// Let the command start
	time.Sleep(200 * time.Millisecond)

	// Check sqlite DB for active process registration
	db, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	var pid int
	var command string
	row := db.QueryRow("SELECT pid, command FROM active_processes LIMIT 1;")
	err = row.Scan(&pid, &command)
	if err != nil {
		t.Errorf("failed to find active process in database during execution: %v", err)
	}

	if pid <= 0 {
		t.Errorf("expected valid pid in active_processes, got %d", pid)
	}
	if !strings.Contains(command, "sleep") {
		t.Errorf("expected command to contain 'sleep', got %q", command)
	}

	wg.Wait()
	if runErr != nil {
		t.Errorf("RunCommand in goroutine failed: %v", runErr)
	}

	// Verify PID is removed after command completion
	var count int
	row = db.QueryRow("SELECT COUNT(*) FROM active_processes WHERE pid = ?;", pid)
	err = row.Scan(&count)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected active process count to be 0 after completion, got %d", count)
	}
}

func TestUnauthorizedCommandGuard(t *testing.T) {
	dbPath, cleanup := initTestDB(t)
	defer cleanup()

	// 1. Direct chmod command execution must be blocked
	_, err := RunCommand(dbPath, "", "chmod", "755", "test.sh")
	if err == nil || !strings.Contains(err.Error(), "Security Violation") {
		t.Errorf("expected RunCommand direct chmod to be blocked as security violation, got: %v", err)
	}

	// 2. Direct chown command execution must be blocked
	_, err = RunCommand(dbPath, "", "chown", "root:root", "test.sh")
	if err == nil || !strings.Contains(err.Error(), "Security Violation") {
		t.Errorf("expected RunCommand direct chown to be blocked as security violation, got: %v", err)
	}

	// 3. Embedded chmod inside shell args must be blocked
	_, err = RunCommand(dbPath, "", "bash", "-c", "chmod +x test.sh")
	if err == nil || !strings.Contains(err.Error(), "Security Violation") {
		t.Errorf("expected RunCommand embedded chmod to be blocked as security violation, got: %v", err)
	}

	// 4. StartCommand with chown must be blocked
	_, err = StartCommand(dbPath, "", "chown", "mark:mark", "file.txt")
	if err == nil || !strings.Contains(err.Error(), "Security Violation") {
		t.Errorf("expected StartCommand direct chown to be blocked as security violation, got: %v", err)
	}
}
