//go:build !windows

package e2e

// TestSkillCopy_AbsoluteSymlink_Relinked (unix build tag): install a skill in
// rootA so the supporting providers symlink to rootA's canonical skill dir, then
// copy rootA→rootB (copyTree preserves symlinks VERBATIM).
//
// The implementation creates ABSOLUTE symlinks (createSymlink links the absolute
// <root>/.agents/skills/<name>; see internal/skills/symlink.go and
// symlink_other.go). Copying the tree to a NEW root therefore reproduces the link
// pointing back into rootA — a WRONG link for rootB (rootA still exists during
// the test, so it is a wrong-link, not a dangling one). This test asserts that
// hazard and its recovery POSITIVELY:
//
//   - the copied provider links in rootB still point into rootA (verbatim copy);
//   - `graft skill sync` (no --override needed: override only governs a REAL
//     dir/file blocking the path; a WRONG symlink is auto-relinked by the Link
//     state machine) re-links every provider to rootB's OWN canonical
//     .agents/skills/<name>, reporting linked and not erroring;
//   - the re-link is IDEMPOTENT — a second `skill sync` reports linked again, no
//     conflict, and leaves the now-correct link untouched (no re-link).
//
// Out of scope (documented): a RELATIVE-symlink platform would copy a link that
// resolves, relative to its own location, to rootB's canonical dir — the copy
// would be self-healing and need no override. The unix/other builds here use the
// absolute strategy, so the wrong-link + override path is what is exercised.

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// linkInode returns the inode number of the symlink itself (Lstat, not the
// target). A re-create (os.Remove + os.Symlink) allocates a NEW inode, so an
// unchanged inode across a second sync PROVES no re-link happened — robust on
// filesystems whose mtime resolution is 1s (where comparing the link's mtime is
// a tautology). Unix-only; this file already carries //go:build !windows.
func linkInode(t *testing.T, path string) uint64 {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("lstat %s: unexpected Sys() type %T (want *syscall.Stat_t)", path, fi.Sys())
	}
	return uint64(st.Ino)
}

func TestSkillCopy_AbsoluteSymlink_Relinked(t *testing.T) {
	// --- Set up rootA with a synced skill ---
	rootA := newGitWorkspace(t)
	// Disable the skill hook so we control when skill sync runs.
	mustGraft(t, rootA, "config", "set", "--skills.enabled", "false")
	mustGraft(t, rootA, "init")
	writeCanonicalSkill(t, rootA, "my-skill", "Skill body")

	// Explicitly sync skills in rootA to create the provider symlinks.
	mustGraft(t, rootA, "config", "set", "--skills.enabled", "true")
	var statusA []skillStatusJSON
	decodeJSON(t, mustGraft(t, rootA, "skill", "sync", "-o", "json"), &statusA)
	for prov := range supportingSkillDirs {
		if s, ok := stateOf(statusA, prov, "my-skill"); !ok || s != "linked" {
			t.Fatalf("rootA provider %s skill state=%q (ok=%v), want linked", prov, s, ok)
		}
	}

	// Precondition for this test: rootA's provider links are ABSOLUTE and point
	// into rootA. A relative link would mean the implementation changed strategy —
	// fail loudly (the wrong-link copy hazard would no longer apply, and this test
	// should be replaced by the relative-survives-copy assertion).
	for _, dir := range supportingSkillDirs {
		link := filepath.Join(rootA, dir, "my-skill")
		target, err := os.Readlink(link)
		if err != nil {
			t.Fatalf("readlink rootA %s: %v", link, err)
		}
		if !filepath.IsAbs(target) || !strings.HasPrefix(target, rootA) {
			t.Fatalf("rootA provider link %s target=%q; this test asserts the ABSOLUTE-symlink "+
				"strategy (target under rootA). If the strategy changed to relative, the copied "+
				"link would self-heal and this wrong-link test must be replaced.", link, target)
		}
	}

	// --- Copy rootA → rootB (symlinks copied VERBATIM) ---
	rootB := t.TempDir()
	copyTree(t, rootA, rootB)
	gitInit(t, rootB)
	gitCommitAll(t, rootB, "copy from rootA")

	// POSITIVE ASSERTION 1: the copied absolute links in rootB still point into
	// rootA (verbatim copy reproduced the wrong target).
	for _, dir := range supportingSkillDirs {
		link := filepath.Join(rootB, dir, "my-skill")
		target, err := os.Readlink(link)
		if err != nil {
			t.Fatalf("readlink rootB %s: %v", link, err)
		}
		if !strings.HasPrefix(target, rootA) {
			t.Fatalf("copied link %s target=%q, want it to still point into rootA (%s) — copyTree "+
				"did not preserve the absolute symlink verbatim", link, target, rootA)
		}
	}

	// POSITIVE ASSERTION 2: `skill sync` (no override) auto-relinks the wrong
	// absolute links to rootB's OWN canonical dir and reports linked. The Link
	// state machine treats a WRONG/dangling symlink as recoverable and re-links it
	// without --override (override only governs a REAL dir/file at the path).
	wantTargetB := canonicalSkillDir(rootB, "my-skill")
	var states1 []skillStatusJSON
	decodeJSON(t, mustGraft(t, rootB, "skill", "sync", "-o", "json"), &states1)
	for prov, dir := range supportingSkillDirs {
		link := filepath.Join(rootB, dir, "my-skill")
		assertLinkedTo(t, link, wantTargetB)
		if s, ok := stateOf(states1, prov, "my-skill"); !ok || s != "linked" {
			t.Fatalf("rootB sync: provider %s state=%q (ok=%v), want linked (wrong link auto-relinked)", prov, s, ok)
		}
	}

	// Capture each (now-correct) link's INODE to prove idempotency. Inode is
	// robust where mtime is not: a re-create changes the inode, so an unchanged
	// inode proves the link was left untouched (no os.Remove+os.Symlink).
	inodeBefore := map[string]uint64{}
	for prov, dir := range supportingSkillDirs {
		inodeBefore[prov] = linkInode(t, filepath.Join(rootB, dir, "my-skill"))
	}

	// POSITIVE ASSERTION 3: idempotency — a second `skill sync` over the
	// now-correct links reports linked again, no conflict, and does NOT re-create
	// the link.
	var states2 []skillStatusJSON
	decodeJSON(t, mustGraft(t, rootB, "skill", "sync", "-o", "json"), &states2)
	for prov, dir := range supportingSkillDirs {
		if s, ok := stateOf(states2, prov, "my-skill"); !ok || s != "linked" {
			t.Fatalf("rootB idempotent sync: provider %s state=%q (ok=%v), want linked", prov, s, ok)
		}
		link := filepath.Join(rootB, dir, "my-skill")
		assertLinkedTo(t, link, wantTargetB)
		if got := linkInode(t, link); got != inodeBefore[prov] {
			t.Fatalf("rootB idempotent sync re-created provider %s link (inode %d -> %d); "+
				"want untouched (no re-link needed)", prov, inodeBefore[prov], got)
		}
	}
}
