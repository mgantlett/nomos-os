package exec

import (
	"os/exec"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/mgantlett/nomos-commons/src/nomos/core/db"
)

func TestAcquireLockSuccess(t *testing.T) {
	dbPath, cleanup := initTestDB(t)
	defer cleanup()

	// Get our own PID
	myPid := 12345

	ok, err := AcquireLock(dbPath, "test-lock", "agent-1", myPid, false)
	if err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}
	if !ok {
		t.Errorf("expected to acquire lock successfully")
	}

	// Verify it's in the database
	db, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	var owner string
	var pid int
	err = db.QueryRow("SELECT owner, pid FROM locks WHERE lock_key = 'test-lock';").Scan(&owner, &pid)
	if err != nil {
		t.Fatalf("failed to query lock: %v", err)
	}
	if owner != "agent-1" || pid != myPid {
		t.Errorf("expected owner='agent-1', pid=%d, got owner=%q, pid=%d", myPid, owner, pid)
	}
}

func TestAcquireLockCollision(t *testing.T) {
	dbPath, cleanup := initTestDB(t)
	defer cleanup()

	// We spawn a dummy process (like sleep) that is active to test active PID collision
	cmd := exec.Command("sleep", "2")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start dummy process: %v", err)
	}
	defer cmd.Process.Kill()

	activePid := cmd.Process.Pid

	// Acquire lock with active process PID
	ok, err := AcquireLock(dbPath, "test-lock", "agent-1", activePid, false)
	if err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected to acquire first lock")
	}

	// Now try to acquire it from another agent/PID
	ok, err = AcquireLock(dbPath, "test-lock", "agent-2", 54321, false)
	if err != nil {
		t.Fatalf("second acquire check failed: %v", err)
	}
	if ok {
		t.Errorf("expected lock acquisition to fail due to active process collision")
	}
}

func TestAcquireLockDeadPIDRecovery(t *testing.T) {
	dbPath, cleanup := initTestDB(t)
	defer cleanup()

	// We spawn a process and let it exit immediately
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start command: %v", err)
	}
	_ = cmd.Wait()

	deadPid := cmd.Process.Pid

	// Artificially insert a lock with the dead PID
	db, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	_, err = db.Exec("INSERT INTO locks (lock_key, owner, pid) VALUES ('test-lock', 'agent-old', ?);", deadPid)
	if err != nil {
		t.Fatalf("failed to insert mock lock: %v", err)
	}

	// Try to acquire the lock. It should detect the dead PID and break the lock
	ok, err := AcquireLock(dbPath, "test-lock", "agent-new", 12345, false)
	if err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}
	if !ok {
		t.Errorf("expected to break lock and acquire it because previous PID is dead")
	}
}

func TestAcquireLockForce(t *testing.T) {
	dbPath, cleanup := initTestDB(t)
	defer cleanup()

	// Spawn dummy active process
	cmd := exec.Command("sleep", "2")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start dummy process: %v", err)
	}
	defer cmd.Process.Kill()

	activePid := cmd.Process.Pid

	ok, err := AcquireLock(dbPath, "test-lock", "agent-1", activePid, false)
	if err != nil || !ok {
		t.Fatalf("first acquire failed: %v, ok=%t", err, ok)
	}

	// Acquire with force = true
	ok, err = AcquireLock(dbPath, "test-lock", "agent-2", 54321, true)
	if err != nil {
		t.Fatalf("force acquire failed: %v", err)
	}
	if !ok {
		t.Errorf("expected to acquire lock via force overwrite")
	}
}

func TestReleaseLock(t *testing.T) {
	dbPath, cleanup := initTestDB(t)
	defer cleanup()

	ok, err := AcquireLock(dbPath, "test-lock", "agent-1", 12345, false)
	if err != nil || !ok {
		t.Fatalf("acquire failed")
	}

	// Try to release with wrong owner
	err = ReleaseLock(dbPath, "test-lock", "agent-2")
	if err != nil {
		t.Fatalf("ReleaseLock errored: %v", err)
	}

	// Verify lock still exists
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db")
	}
	var count int
	err = database.QueryRow("SELECT COUNT(*) FROM locks WHERE lock_key = 'test-lock';").Scan(&count)
	if err != nil || count != 1 {
		t.Errorf("expected lock to still exist, count=%d", count)
	}

	// Release with correct owner
	err = ReleaseLock(dbPath, "test-lock", "agent-1")
	if err != nil {
		t.Fatalf("ReleaseLock errored: %v", err)
	}

	// Verify lock is deleted
	database, err = db.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open db")
	}
	err = database.QueryRow("SELECT COUNT(*) FROM locks WHERE lock_key = 'test-lock';").Scan(&count)
	if err != nil || count != 0 {
		t.Errorf("expected lock to be deleted, count=%d", count)
	}
}
