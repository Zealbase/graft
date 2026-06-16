package roocode

import "testing"

func TestSkillsSupported(t *testing.T) {
	sp := SkillProvider()
	if !sp.SkillsSupported() {
		t.Fatal("roocode should support skills")
	}
	if sp.Name() != name {
		t.Errorf("Name()=%q want %q", sp.Name(), name)
	}
	if d := sp.SkillDir("/ws"); d != "" {
		t.Errorf("SkillDir=%q want empty", d)
	}
	refs, err := sp.DetectSkills("/ws")
	if err != nil {
		t.Fatalf("DetectSkills err: %v", err)
	}
	if refs != nil {
		t.Errorf("expected nil refs, got %+v", refs)
	}
}

func TestSkillsNativeDiscovery(t *testing.T) {
	sp := SkillProvider()
	type nativeDiscoverer interface {
		NativeCanonicalDiscovery() bool
	}
	nd, ok := sp.(nativeDiscoverer)
	if !ok {
		t.Fatal("roo-code SkillProvider does not implement NativeCanonicalDiscovery()")
	}
	if !nd.NativeCanonicalDiscovery() {
		t.Fatal("roo-code NativeCanonicalDiscovery() == false, want true")
	}
}

func TestSkillsHomeSkillDirs(t *testing.T) {
	sp := SkillProvider()
	home := "/h"
	dirs := sp.HomeSkillDirs(home)
	if len(dirs) != 2 {
		t.Fatalf("HomeSkillDirs = %v, want 2 entries", dirs)
	}
	if dirs[0] != home+"/.roo/skills" {
		t.Errorf("HomeSkillDirs[0] = %q, want ~/.roo/skills", dirs[0])
	}
	if dirs[1] != home+"/.agents/skills" {
		t.Errorf("HomeSkillDirs[1] = %q, want ~/.agents/skills", dirs[1])
	}
	if sp.HomeSkillDirs("") != nil {
		t.Errorf("HomeSkillDirs(\"\") not nil")
	}
}
