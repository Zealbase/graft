// Package lock provides a workspace-exclusive advisory lock so concurrent graft
// processes operating on the same workspace serialize their mutating work (most
// importantly `graft sync`). It is a thin wrapper over an OS file lock (flock on
// unix) taken on <root>/.graft/lock.
//
// The lock is advisory and process-scoped: it coordinates separate graft
// processes, not goroutines within one process. Two variants are offered:
//
//   - TryLock — non-blocking; returns ErrLocked immediately if another process
//     holds the lock.
//   - Lock    — blocking; waits until the lock is acquired (honoring ctx).
//
// Release the lock by calling Unlock (or Handle.Close). The lock file itself is
// left on disk; only the kernel-held flock is released.
package lock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ErrLocked is returned by TryLock when the workspace lock is already held by
// another process.
var ErrLocked = errors.New("lock: workspace is locked by another graft process")

// LockFile is the path, relative to the workspace root, of the advisory lock.
func LockFile(root string) string {
	return filepath.Join(root, ".graft", "lock")
}

// Handle is an acquired workspace lock. Unlock releases it; it is safe to call
// Unlock more than once.
type Handle struct {
	f      *os.File
	closed bool
}

// open ensures .graft/ exists and opens (creating) the lock file for locking.
func open(root string) (*os.File, error) {
	path := LockFile(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("lock: mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("lock: open %s: %w", path, err)
	}
	return f, nil
}

// TryLock attempts to acquire the workspace lock without blocking. It returns
// ErrLocked if another process holds it.
func TryLock(root string) (*Handle, error) {
	f, err := open(root)
	if err != nil {
		return nil, err
	}
	if err := tryFlock(f); err != nil {
		f.Close()
		return nil, err
	}
	return &Handle{f: f}, nil
}

// Lock acquires the workspace lock, blocking until it is available or ctx is
// done. It polls the non-blocking flock so cancellation is honored promptly.
func Lock(ctx context.Context, root string) (*Handle, error) {
	f, err := open(root)
	if err != nil {
		return nil, err
	}
	for {
		err := tryFlock(f)
		if err == nil {
			return &Handle{f: f}, nil
		}
		if !errors.Is(err, ErrLocked) {
			f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

// Unlock releases the lock and closes the underlying file. Idempotent.
func (h *Handle) Unlock() error {
	if h == nil || h.closed {
		return nil
	}
	h.closed = true
	if h.f == nil {
		return nil
	}
	uerr := unflock(h.f)
	cerr := h.f.Close()
	if uerr != nil {
		return uerr
	}
	return cerr
}

// Close is an alias for Unlock so a Handle can be used with defer x.Close().
func (h *Handle) Close() error { return h.Unlock() }
