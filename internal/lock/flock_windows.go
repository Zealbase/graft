//go:build windows

package lock

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// tryFlock takes a non-blocking exclusive lock on the first byte of f using the
// Win32 LockFileEx API with LOCKFILE_FAIL_IMMEDIATELY, mapping the
// already-locked case to ErrLocked. This gives the same cross-process exclusion
// semantics as flock(2) on unix: two graft processes contending for the same
// workspace lock file serialize instead of racing on the global db.
//
// LockFileEx locks a byte range; locking [0,1) of the (empty) lock file is the
// conventional way to take a whole-file advisory lock on Windows. The lock is
// released automatically when the handle is closed, but we also unlock
// explicitly in unflock for symmetry and prompt release.
func tryFlock(f *os.File) error {
	ol := new(windows.Overlapped)
	err := windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1, 0, // lock the first byte
		ol,
	)
	if err == nil {
		return nil
	}
	// When the range is already locked by another handle, LockFileEx with
	// LOCKFILE_FAIL_IMMEDIATELY fails with ERROR_LOCK_VIOLATION (and, depending
	// on timing, ERROR_IO_PENDING). Map both to ErrLocked so the caller's
	// blocking poll loop behaves identically to the unix EWOULDBLOCK path.
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_IO_PENDING) {
		return ErrLocked
	}
	return err
}

// unflock releases the LockFileEx range lock held on f.
func unflock(f *os.File) error {
	ol := new(windows.Overlapped)
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, ol)
}
