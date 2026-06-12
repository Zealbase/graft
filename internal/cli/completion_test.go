package cli

import (
	"path/filepath"
	"testing"
)

func TestResolveCompletionTargetBash(t *testing.T) {
	home := "/home/u"
	xdg := "/home/u/.local/share"
	target, err := resolveCompletionTarget("bash", home, xdg)
	if err != nil {
		t.Fatalf("bash: %v", err)
	}
	want := filepath.Join(xdg, "bash-completion", "completions", "graft")
	if target.AutoPath != want {
		t.Fatalf("bash AutoPath = %q, want %q", target.AutoPath, want)
	}
	if target.RCFile != filepath.Join(home, ".bashrc") {
		t.Fatalf("bash RCFile = %q", target.RCFile)
	}
}

func TestResolveCompletionTargetBashDefaultXDG(t *testing.T) {
	// Empty xdgData falls back to ~/.local/share.
	target, err := resolveCompletionTarget("bash", "/home/u", "")
	if err != nil {
		t.Fatalf("bash: %v", err)
	}
	want := filepath.Join("/home/u", ".local", "share", "bash-completion", "completions", "graft")
	if target.AutoPath != want {
		t.Fatalf("bash default-xdg AutoPath = %q, want %q", target.AutoPath, want)
	}
}

func TestResolveCompletionTargetZsh(t *testing.T) {
	target, err := resolveCompletionTarget("zsh", "/home/u", "")
	if err != nil {
		t.Fatalf("zsh: %v", err)
	}
	want := filepath.Join("/home/u", ".zsh", "completions", "_graft")
	if target.AutoPath != want {
		t.Fatalf("zsh AutoPath = %q, want %q", target.AutoPath, want)
	}
}

func TestResolveCompletionTargetFish(t *testing.T) {
	target, err := resolveCompletionTarget("fish", "/home/u", "")
	if err != nil {
		t.Fatalf("fish: %v", err)
	}
	want := filepath.Join("/home/u", ".config", "fish", "completions", "graft.fish")
	if target.AutoPath != want {
		t.Fatalf("fish AutoPath = %q, want %q", target.AutoPath, want)
	}
}

func TestResolveCompletionTargetUnknownShell(t *testing.T) {
	if _, err := resolveCompletionTarget("powershell", "/home/u", ""); err == nil {
		t.Fatalf("expected error for unsupported shell")
	}
}

func TestCompletionInstallWritesAutoPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))

	c := EntrypointWithVersion(nil, nil, "test")
	// Drive the install subcommand directly via the root.
	root := c.Root()
	root.SetArgs([]string{"completion", "install", "fish"})
	if err := root.Execute(); err != nil {
		t.Fatalf("completion install fish: %v", err)
	}
	out := filepath.Join(home, ".config", "fish", "completions", "graft.fish")
	if !fileExists(out) {
		t.Fatalf("fish completion not written to %s", out)
	}
}

func TestAppendSourceLineIdempotent(t *testing.T) {
	rc := filepath.Join(t.TempDir(), ".bashrc")
	line := "source <(graft completion bash)"
	if err := appendSourceLine(rc, line); err != nil {
		t.Fatalf("append 1: %v", err)
	}
	if err := appendSourceLine(rc, line); err != nil {
		t.Fatalf("append 2: %v", err)
	}
	data, err := readFileString(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if n := countOccurrences(data, line); n != 1 {
		t.Fatalf("source line appended %d times, want 1 (idempotent)", n)
	}
}
