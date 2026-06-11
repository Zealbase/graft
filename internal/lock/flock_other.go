//go:build !unix

package lock

import "os"

// tryFlock is a best-effort no-op on platforms without flock. The lock file is
// still created so behavior degrades gracefully (single-process correctness).
func tryFlock(f *os.File) error { return nil }

// unflock is a no-op counterpart to the fallback tryFlock.
func unflock(f *os.File) error { return nil }
