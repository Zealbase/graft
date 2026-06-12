package gateway

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

	"github.com/adrg/xdg"
)

// Global XDG locations. The sqlite db and per-workspace lock live OUTSIDE the
// repo so an in-repo .graft/ holds only the portable store (agents/ + .meta.json).
//
//	db:    ~/.local/share/graft/graft.db
//	locks: ~/.local/share/graft/locks/<ws-hash>.lock

// globalDBPath returns the global sqlite path, creating its parent dir. It uses
// xdg.DataFile which resolves $XDG_DATA_HOME (default ~/.local/share) and makes
// the parent directory.
func globalDBPath() (string, error) {
	return xdg.DataFile(filepath.Join("graft", "graft.db"))
}

// globalLockPath returns the per-workspace lock file path under the global locks
// dir, keyed by a hash of the workspace identity (root+remote+branch). The
// caller (lock.open) creates the parent dir on demand.
func globalLockPath(root, remote, branch string) (string, error) {
	hash := wsHash(root, remote, branch)
	// xdg.DataFile makes the parent dir and returns the full path.
	return xdg.DataFile(filepath.Join("graft", "locks", hash+".lock"))
}

// wsHash is the stable identity hash for a workspace (root+remote+branch),
// matching the store's workspace identity tuple. 16 hex chars is ample for a
// per-user lock filename.
func wsHash(root, remote, branch string) string {
	sum := sha256.Sum256([]byte(root + "\x00" + remote + "\x00" + branch))
	return hex.EncodeToString(sum[:8])
}
