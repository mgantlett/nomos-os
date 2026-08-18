package exec

import (
	_ "modernc.org/sqlite"

	"github.com/mgantlett/nomos-commons/src/nomos/core/db"
)

// RegisterActiveProcess opens a database connection, registers the active PID,
// and immediately closes the connection.
func RegisterActiveProcess(dbPath string, pid int, command string) error {
	db, err := db.Open(dbPath)
	if err != nil {
		return err
	}

	// Enable Write-Ahead Logging to prevent database locking errors during concurrent
	// access from multiple processes or agent swarm operations.
	_, _ = db.Exec("PRAGMA journal_mode=WAL;")
	_, err = db.Exec("INSERT OR REPLACE INTO active_processes (pid, command) VALUES (?, ?);", pid, command)
	return err
}

// DeregisterActiveProcess opens a database connection, removes the active PID,
// and immediately closes the connection.
func DeregisterActiveProcess(dbPath string, pid int) error {
	db, err := db.Open(dbPath)
	if err != nil {
		return err
	}

	_, _ = db.Exec("PRAGMA journal_mode=WAL;")
	_, err = db.Exec("DELETE FROM active_processes WHERE pid = ?;", pid)
	return err
}

