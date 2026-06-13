//go:build !unix && !windows

package lock

import "os"

// This fallback covers platforms that have neither flock(2) (unix) nor
// LockFileEx (windows) — in practice js/wasm and plan9, where graft does not
// run a concurrent multi-process workload. tryFlock is a best-effort no-op
// there; the lock file is still created so behavior degrades gracefully
// (single-process correctness). Real cross-process exclusion is provided on
// unix (flock_unix.go) and windows (flock_windows.go).
func tryFlock(f *os.File) error { return nil }

// unflock is a no-op counterpart to the fallback tryFlock.
func unflock(f *os.File) error { return nil }
