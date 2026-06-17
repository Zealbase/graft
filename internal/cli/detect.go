package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/Shaik-Sirajuddin/graft/internal/cli/config"
)

// providerDetect describes how to heuristically tell whether a provider's tool
// is present on this machine: a config dir under $HOME and/or a binary on PATH.
// A provider with no signals is shown unchecked by default (unknown != absent).
type providerDetect struct {
	// homeDirs are dirs (relative to $HOME) whose existence implies the tool.
	homeDirs []string
	// binaries are executables looked up on PATH.
	binaries []string
}

// providerDetectors maps each supported provider id to its detection heuristic.
// Kept CLI-local (no internal/providers import) per the gateway-only rule.
var providerDetectors = map[string]providerDetect{
	"claude-code":    {homeDirs: []string{".claude"}, binaries: []string{"claude"}},
	"cline":          {homeDirs: []string{".cline/agents"}, binaries: []string{"cline"}},
	"codex":          {homeDirs: []string{".codex"}, binaries: []string{"codex"}},
	"continue":       {homeDirs: []string{".continue/agents"}, binaries: []string{"continue"}},
	"gemini-cli":     {homeDirs: []string{".gemini"}, binaries: []string{"gemini"}},
	"cursor":         {homeDirs: []string{".cursor"}, binaries: []string{"cursor"}},
	"github-copilot": {homeDirs: []string{".config/github-copilot", ".copilot"}, binaries: []string{"copilot"}},
	"kilo-code":      {homeDirs: []string{".kilo/agents", ".kilocodemodes"}, binaries: []string{"kilo"}},
	"opencode":       {homeDirs: []string{".opencode", ".config/opencode"}, binaries: []string{"opencode"}},
	"roo-code":       {homeDirs: []string{".roo"}, binaries: []string{"roo"}},
	"goose":          {homeDirs: []string{".config/goose"}, binaries: []string{"goose"}},
	"grok-cli":       {homeDirs: []string{".grok"}, binaries: []string{"grok"}},
	"antigravity":    {homeDirs: []string{".gemini/antigravity-cli", ".antigravity"}, binaries: []string{"antigravity"}},
}

// providerInstalled reports whether a provider looks present on this machine.
// home is the base dir for the home-dir checks (os.UserHomeDir in production;
// injectable for tests). Either signal (a config dir OR a binary) suffices.
func providerInstalled(id, home string) bool {
	d, ok := providerDetectors[id]
	if !ok {
		return false
	}
	for _, rel := range d.homeDirs {
		if home != "" {
			if fi, err := os.Stat(filepath.Join(home, rel)); err == nil && fi.IsDir() {
				return true
			}
		}
	}
	for _, bin := range d.binaries {
		if _, err := exec.LookPath(bin); err == nil {
			return true
		}
	}
	return false
}

// detectInstalledProviders returns the sorted subset of supported providers that
// look installed under the given home dir.
func detectInstalledProviders(home string) []string {
	var out []string
	for _, id := range config.SupportedProviders() {
		if providerInstalled(id, home) {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// userHome returns the user's home dir for detection, or "" if it cannot be
// resolved (detection then relies on PATH only).
func userHome() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}
