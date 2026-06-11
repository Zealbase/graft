package grokcli

import "testing"

func TestSkillsUnsupported(t *testing.T) {
	sp := SkillProvider()
	if sp.SkillsSupported() {
		t.Fatal("grokcli should not support skills")
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
