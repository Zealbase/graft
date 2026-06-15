package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shaik-Sirajuddin/graft/internal/cli/config"
	"github.com/Shaik-Sirajuddin/graft/internal/contract"
)

// writeSkillSrc creates a skill source dir with SKILL.md and returns its path.
func writeSkillSrc(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCLISkillInstallStatusList(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}

	src := writeSkillSrc(t, "commit")
	out, err := execCLI(t, root, nil, "skill", "install", src, "-o", "json")
	if err != nil {
		t.Fatalf("skill install: %v\n%s", err, out)
	}
	var states []contract.SkillStatus
	if err := json.Unmarshal([]byte(out), &states); err != nil {
		t.Fatalf("parse install states: %v\n%s", err, out)
	}
	// 4 supporting providers all linked (codex links via native discovery).
	if len(states) != 3 {
		t.Fatalf("install reported %d states, want 3: %+v", len(states), states)
	}
	for _, s := range states {
		if s.State != contract.SkillLinked && s.State != contract.SkillNativeLinked {
			t.Fatalf("provider %s state=%s want linked", s.Provider, s.State)
		}
	}

	// list shows the canonical skill.
	out, err = execCLI(t, root, nil, "skill", "list", "-o", "json")
	if err != nil {
		t.Fatalf("skill list: %v\n%s", err, out)
	}
	var skills []contract.Skill
	if err := json.Unmarshal([]byte(out), &skills); err != nil {
		t.Fatalf("parse skill list: %v\n%s", err, out)
	}
	if len(skills) != 1 || skills[0].Name != "commit" {
		t.Fatalf("skill list = %+v, want [commit]", skills)
	}

	// status: all linked.
	out, err = execCLI(t, root, nil, "skill", "status", "-o", "json")
	if err != nil {
		t.Fatalf("skill status: %v\n%s", err, out)
	}
	if err := json.Unmarshal([]byte(out), &states); err != nil {
		t.Fatalf("parse status: %v\n%s", err, out)
	}
	if len(states) != 3 {
		t.Fatalf("status %d states, want 3", len(states))
	}
}

func TestCLISkillSyncIdempotent(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	src := writeSkillSrc(t, "review")
	if _, err := execCLI(t, root, nil, "skill", "install", src); err != nil {
		t.Fatalf("install: %v", err)
	}
	// sync twice; both succeed, second is a no-op (idempotent).
	if out, err := execCLI(t, root, nil, "skill", "sync", "-o", "json"); err != nil {
		t.Fatalf("skill sync 1: %v\n%s", err, out)
	}
	out, err := execCLI(t, root, nil, "skill", "sync", "-o", "json")
	if err != nil {
		t.Fatalf("skill sync 2: %v\n%s", err, out)
	}
	var states []contract.SkillStatus
	if err := json.Unmarshal([]byte(out), &states); err != nil {
		t.Fatalf("parse sync states: %v\n%s", err, out)
	}
	for _, s := range states {
		if s.State != contract.SkillLinked && s.State != contract.SkillNativeLinked {
			t.Fatalf("sync left %s/%s in %s", s.Skill, s.Provider, s.State)
		}
	}
}

func TestCLISkillProviderScope(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	src := writeSkillSrc(t, "deploy")
	if _, err := execCLI(t, root, nil, "skill", "install", src, "--provider", "opencode"); err != nil {
		t.Fatalf("install scoped: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".opencode", "skills", "deploy")); err != nil {
		t.Fatalf("opencode link missing: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".claude", "skills", "deploy")); !os.IsNotExist(err) {
		t.Fatalf("claude-code should not be linked under provider scope: %v", err)
	}
}

func TestCLISkillStatusRawNoANSI(t *testing.T) {
	root := newWorkspace(t)
	if _, err := execCLI(t, root, nil, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := execCLI(t, root, nil, "skill", "status", "-o", "json")
	if err != nil {
		t.Fatalf("skill status: %v", err)
	}
	if strings.Contains(out, "\033[") {
		t.Fatalf("json output contains ANSI escapes:\n%q", out)
	}
}

func TestCLIConfigSkillKeys(t *testing.T) {
	dir := t.TempDir()
	resolver := &config.DefaultResolver{ConfigPath: filepath.Join(dir, "config.json")}

	// default: skills.enabled true.
	out, err := execNoGate(t, resolver, "config", "get", "-o", "json")
	if err != nil {
		t.Fatalf("config get: %v", err)
	}
	var cfg config.Config
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatalf("parse config: %v\n%s", err, out)
	}
	if !cfg.Skills.EnabledOrDefault() {
		t.Fatalf("skills.enabled default should be true: %+v", cfg.Skills)
	}

	// set skill keys.
	out, err = execNoGate(t, resolver, "config", "set", "-g",
		"--skills.enabled", "false", "--skills.autoInstall", "true",
		"--skills.providers", "claude-code,opencode", "-o", "json")
	if err != nil {
		t.Fatalf("config set: %v\n%s", err, out)
	}
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatalf("parse set: %v\n%s", err, out)
	}
	if cfg.Skills.EnabledOrDefault() {
		t.Fatalf("skills.enabled should be false after set")
	}
	if !cfg.Skills.AutoInstall || len(cfg.Skills.Providers) != 2 {
		t.Fatalf("skill keys not applied: %+v", cfg.Skills)
	}
}
