//go:build unix

package lock

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// tryFlock takes a non-blocking exclusive flock on f, mapping the would-block
// case to ErrLocked.
func tryFlock(f *os.File) error {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return ErrLocked
	}
	return err
}

// unflock releases the flock held on f.
func unflock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}
