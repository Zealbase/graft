// Package e2e drives the REAL compiled graft binary as a subprocess against
// throwaway workspaces. Every test gets its own t.TempDir() workspace with a
// real `git init`, runs graft commands, and verifies at three levels:
//
//   - file: provider files on disk are well-formed and re-parse losslessly;
//     .graft/agents/<name>/{agent.yaml,instructions.md,.meta.json} are correct.
//   - db:   <root>/.graft/graft.db is opened read-only and rows are asserted.
//   - raw:  stdout for `-o json` is parsed + field-checked (no ANSI) and exit
//     codes are asserted.
//
// The binary is built ONCE into a temp path (TestMain) and shared across tests.
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// graftBin is the path to the compiled binary, set by TestMain.
var graftBin string

// TestMain builds the graft binary once into a temp dir and runs the suite.
func TestMain(m *testing.M) {
	if _, err := exec.LookPath("git"); err != nil {
		fmt.Fprintln(os.Stderr, "e2e: git binary not on PATH; skipping suite")
		os.Exit(0)
	}
	tmp, err := os.MkdirTemp("", "graft-e2e-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: mkdtemp:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)

	graftBin = filepath.Join(tmp, "graft")
	// Build from the module root (two levels up from tests/e2e).
	build := exec.Command("go", "build", "-o", graftBin, "./cmd/graft")
	build.Dir = moduleRoot()
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "e2e: build graft:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// moduleRoot walks up from the test file's working dir until it finds go.mod.
func moduleRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
}

// runResult captures the outcome of one graft invocation.
type runResult struct {
	stdout   string
	stderr   string
	exitCode int
}

// graft runs the binary in dir with args. It isolates global config under an
// XDG_CONFIG_HOME inside dir so `config get/set` never touch the real user
// config. It returns the captured result (never fails the test itself; callers
// assert on exitCode).
func graft(t *testing.T, dir string, args ...string) runResult {
	t.Helper()
	cmd := exec.Command(graftBin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+filepath.Join(dir, "xdg-config"),
		"NO_COLOR=1",
		"GRAFT_THEME=",
	)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("graft %v: exec failed: %v", args, err)
		}
	}
	return runResult{stdout: out.String(), stderr: errb.String(), exitCode: code}
}

// mustGraft runs graft and fails the test if the exit code is non-zero.
func mustGraft(t *testing.T, dir string, args ...string) runResult {
	t.Helper()
	r := graft(t, dir, args...)
	if r.exitCode != 0 {
		t.Fatalf("graft %v exit=%d\nstdout:\n%s\nstderr:\n%s", args, r.exitCode, r.stdout, r.stderr)
	}
	return r
}

// decodeJSON parses r.stdout into v, failing on parse error. It also asserts the
// stdout carries no ANSI escape so piped consumers get clean JSON.
func decodeJSON(t *testing.T, r runResult, v any) {
	t.Helper()
	if strings.Contains(r.stdout, "\x1b[") {
		t.Fatalf("json stdout contains ANSI escape: %q", r.stdout)
	}
	if err := json.Unmarshal([]byte(r.stdout), v); err != nil {
		t.Fatalf("decode json: %v\nstdout:\n%s", err, r.stdout)
	}
}

// gitInit creates a real git repo in dir with a deterministic identity.
func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@t"},
		{"config", "user.name", "t"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// gitCommitAll stages and commits everything in dir.
func gitCommitAll(t *testing.T, dir, msg string) {
	t.Helper()
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", msg}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

// gitHead returns the short HEAD hash of dir's current branch.
func gitHead(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// writeFile writes data to a path relative to dir, creating parent dirs.
func writeFile(t *testing.T, dir, rel, data string) {
	t.Helper()
	abs := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readFile reads a path relative to dir.
func readFile(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// exists reports whether a path relative to dir exists.
func exists(dir, rel string) bool {
	_, err := os.Stat(filepath.Join(dir, rel))
	return err == nil
}
