// Package lock provides a workspace-exclusive advisory lock so concurrent graft
// processes operating on the same workspace serialize their mutating work (most
// importantly `graft sync`). It is a thin wrapper over an OS file lock taken on
// a caller-supplied lock file path: flock(2) on unix (covering linux + darwin)
// and LockFileEx on windows. On the rare platforms that have neither (js/wasm,
// plan9) it degrades to a best-effort no-op (single-process correctness only).
//
// The lock file lives OUTSIDE the repo: the gateway computes a global
// per-workspace path (~/.local/share/graft/locks/<ws-hash>.lock) so nothing
// runtime/local is written into the committed .graft/ tree.
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

// ErrLocked is returned by TryLock when the lock is already held by another
// process.
var ErrLocked = errors.New("lock: workspace is locked by another graft process")

// Handle is an acquired lock. Unlock releases it; it is safe to call Unlock more
// than once.
type Handle struct {
	f      *os.File
	closed bool
}

// open ensures the lock file's parent dir exists and opens (creating) it.
func open(lockPath string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("lock: mkdir: %w", err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("lock: open %s: %w", lockPath, err)
	}
	return f, nil
}

// TryLock attempts to acquire the lock at lockPath without blocking. It returns
// ErrLocked if another process holds it.
func TryLock(lockPath string) (*Handle, error) {
	f, err := open(lockPath)
	if err != nil {
		return nil, err
	}
	if err := tryFlock(f); err != nil {
		f.Close()
		return nil, err
	}
	return &Handle{f: f}, nil
}

// Lock acquires the lock at lockPath, blocking until it is available or ctx is
// done. It polls the non-blocking flock so cancellation is honored promptly.
func Lock(ctx context.Context, lockPath string) (*Handle, error) {
	f, err := open(lockPath)
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
