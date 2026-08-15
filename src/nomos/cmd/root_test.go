package cmd

import (
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/mgantlett/nomos-commons/src/nomos/core/db"
)

func TestInitCacheDB(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "nomos-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "global_cache.db")

	// Call InitCacheDB (which should create the file and the schema)
	err = InitCacheDB(dbPath)
	if err != nil {
		t.Fatalf("InitCacheDB failed: %v", err)
	}

	// Verify database file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("db file was not created at %s", dbPath)
	}

	// Open db and verify tables exist
	db, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	// Check locks table
	row := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='locks';")
	var tableName string
	err = row.Scan(&tableName)
	if err != nil {
		t.Errorf("failed to find locks table: %v", err)
	}
	if tableName != "locks" {
		t.Errorf("expected locks table name, got %q", tableName)
	}

	// Check active_processes table
	row = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='active_processes';")
	err = row.Scan(&tableName)
	if err != nil {
		t.Errorf("failed to find active_processes table: %v", err)
	}
	if tableName != "active_processes" {
		t.Errorf("expected active_processes table name, got %q", tableName)
	}
}
