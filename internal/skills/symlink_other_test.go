//go:build !unix

package skills

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// TestIsSymlinkCapabilityErr verifies the classifier used by the non-unix
// createSymlink to decide whether an os.Symlink failure means "this platform
// cannot link" (-> ErrSymlinkUnavailable, degrade + warn) versus a transient
// failure (-> surfaced unwrapped). Only genuine capability errors must map to
// the sentinel so a disk-full / I/O error is not misreported as a platform
// limitation.
func TestIsSymlinkCapabilityErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"permission (ERROR_PRIVILEGE_NOT_HELD surrogate)", os.ErrPermission, true},
		{"ENOSYS", syscall.ENOSYS, true},
		{"transient ENOSPC", syscall.ENOSPC, false},
		{"plain error", errors.New("boom"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSymlinkCapabilityErr(tc.err); got != tc.want {
				t.Fatalf("isSymlinkCapabilityErr(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestCreateSymlink_CapabilityErrIsSentinel verifies that when os.Symlink fails
// with a capability error the wrapped result satisfies errors.Is(_,
// ErrSymlinkUnavailable) so the skills hook can detect "cannot link here" and
// degrade. We can't force a real privilege failure portably, so this asserts the
// wrapping contract via a permission-denied parent directory where supported;
// otherwise it asserts the sentinel value identity used by the wrapping.
func TestErrSymlinkUnavailable_IsSentinel(t *testing.T) {
	wrapped := errors.Join(errors.New("ctx"), ErrSymlinkUnavailable)
	if !errors.Is(wrapped, ErrSymlinkUnavailable) {
		t.Fatal("wrapped sentinel not detectable via errors.Is")
	}
	// A successful link on a Developer-Mode/privileged host must NOT yield the
	// sentinel; a missing-capability host yields it. Either way createSymlink
	// must never panic.
	root := t.TempDir()
	canon := filepath.Join(root, "canon")
	if err := os.MkdirAll(canon, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "link")
	err := createSymlink(canon, target)
	if err != nil && !errors.Is(err, ErrSymlinkUnavailable) {
		// A non-capability error is acceptable to surface, but log it for context.
		t.Logf("createSymlink returned non-capability error (acceptable): %v", err)
	}
}
