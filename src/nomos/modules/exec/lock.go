package exec

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"syscall"

	_ "modernc.org/sqlite"

	"github.com/mgantlett/nomos-commons/src/nomos/core/db"
)

// IsProcessAlive checks if a process with a given PID is still running.
// It sends signal 0 to the process to query its status from the OS kernel.
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix/Linux, FindProcess always returns a Process struct without verification.
	// We must send signal 0 to check if the process is actually alive.
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	// If the error is ESRCH (No such process), the process is dead.
	if errors.Is(err, syscall.ESRCH) {
		return false
	}
	// If the error is EPERM (Operation not permitted), the process exists but is owned by another user.
	if errors.Is(err, syscall.EPERM) {
		return true
	}
	return false
}

func tryAcquireLockTx(tx *sql.Tx, lockKey string, owner string, pid int, force bool) (bool, error) {
	var existingOwner string
	var existingPid int
	row := tx.QueryRow("SELECT owner, pid FROM locks WHERE lock_key = ?;", lockKey)
	err := row.Scan(&existingOwner, &existingPid)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_, err = tx.Exec("INSERT INTO locks (lock_key, owner, pid) VALUES (?, ?, ?);", lockKey, owner, pid)
			if err != nil {
				return false, fmt.Errorf("failed to insert lock: %w", err)
			}
			return true, nil
		}
		return false, fmt.Errorf("failed to query locks table: %w", err)
	}

	if force || !IsProcessAlive(existingPid) {
		_, err = tx.Exec("INSERT OR REPLACE INTO locks (lock_key, owner, pid, acquired_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP);", lockKey, owner, pid)
		if err != nil {
			return false, fmt.Errorf("failed to replace lock: %w", err)
		}
		return true, nil
	}

	return false, nil
}

// AcquireLock attempts to acquire an exclusive named lock inside .nomos/cache.db.
// If the lock is held:
// - If force is true, it overrides the lock.
// - If the holding process is dead (PID is not active), it overrides the lock (deadlock recovery).
// - Otherwise, it returns false (lock collision).
func AcquireLock(dbPath string, lockKey string, owner string, pid int, force bool) (bool, error) {
	db, err := db.Open(dbPath)
	if err != nil {
		return false, fmt.Errorf("failed to open database: %w", err)
	}

	_, _ = db.Exec("PRAGMA journal_mode=WAL;")

	tx, err := db.Begin()
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	success, err := tryAcquireLockTx(tx, lockKey, owner, pid, force)
	if err != nil {
		return false, err
	}

	if success {
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("failed to commit transaction: %w", err)
		}
	}

	return success, nil
}

// ReleaseLock removes the named lock from .nomos/cache.db if it is owned by the request owner.
func ReleaseLock(dbPath string, lockKey string, owner string) error {
	db, err := db.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	_, _ = db.Exec("PRAGMA journal_mode=WAL;")

	_, err = db.Exec("DELETE FROM locks WHERE lock_key = ? AND owner = ?;", lockKey, owner)
	if err != nil {
		return fmt.Errorf("failed to delete lock: %w", err)
	}

	return nil
}
