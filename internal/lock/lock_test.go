package lock

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// lockPath returns a lock file path inside a fresh temp dir.
func lockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "ws.lock")
}

func TestTryLockExclusive(t *testing.T) {
	p := lockPath(t)

	h1, err := TryLock(p)
	if err != nil {
		t.Fatalf("first TryLock: %v", err)
	}

	// A second TryLock opens a distinct fd; flock contends across fds even in
	// the same process, so this must report ErrLocked.
	if _, err := TryLock(p); err != ErrLocked {
		t.Fatalf("second TryLock = %v, want ErrLocked", err)
	}

	// After releasing the first, a fresh lock succeeds.
	if err := h1.Unlock(); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	h2, err := TryLock(p)
	if err != nil {
		t.Fatalf("re-TryLock after unlock: %v", err)
	}
	if err := h2.Unlock(); err != nil {
		t.Fatalf("unlock h2: %v", err)
	}
}

func TestUnlockIdempotent(t *testing.T) {
	h, err := TryLock(lockPath(t))
	if err != nil {
		t.Fatalf("TryLock: %v", err)
	}
	if err := h.Unlock(); err != nil {
		t.Fatalf("first unlock: %v", err)
	}
	if err := h.Unlock(); err != nil {
		t.Fatalf("second unlock should be no-op: %v", err)
	}
}

func TestLockBlockingAcquiresFree(t *testing.T) {
	// Lock acquires a free lock immediately (ctx-cancel path is hard to force
	// single-process since flock does not contend on the same open fd).
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	h, err := Lock(ctx, lockPath(t))
	if err != nil {
		t.Fatalf("Lock on free path: %v", err)
	}
	if err := h.Unlock(); err != nil {
		t.Fatalf("unlock: %v", err)
	}
}

// TestLockCreatesParentDir verifies open() makes the lock file's parent dir
// (the global ~/.local/share/graft/locks dir is created on demand).
func TestLockCreatesParentDir(t *testing.T) {
	p := filepath.Join(t.TempDir(), "locks", "deep", "ws.lock")
	h, err := TryLock(p)
	if err != nil {
		t.Fatalf("TryLock with missing parent dirs: %v", err)
	}
	if err := h.Unlock(); err != nil {
		t.Fatalf("unlock: %v", err)
	}
}
