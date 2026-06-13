package catalog

import (
	"testing"
)

func TestLoad(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if c == nil {
		t.Fatal("Load() returned nil catalog")
	}
}

func TestVerify(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if err := c.Verify(); err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
}

func TestModelsForAPIProviders(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	apiProviders := []string{
		"claude-code", "codex", "grok-cli", "github-copilot",
		"gemini-cli", "cursor", "antigravity",
	}
	for _, p := range apiProviders {
		models, err := c.ModelsFor(p)
		if err != nil {
			t.Errorf("ModelsFor(%q) error: %v", p, err)
			continue
		}
		if len(models) == 0 {
			t.Errorf("ModelsFor(%q) returned empty list; expected non-empty", p)
		}
	}
}

func TestModelsForPassthroughProviders(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	passthroughProviders := []string{"roo-code", "goose", "opencode"}
	for _, p := range passthroughProviders {
		models, err := c.ModelsFor(p)
		if err != nil {
			t.Errorf("ModelsFor(%q) error: %v", p, err)
			continue
		}
		if len(models) != 0 {
			t.Errorf("ModelsFor(%q) returned %d models; expected empty list", p, len(models))
		}
	}
}

func TestSchema(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	for _, p := range Providers {
		schema, err := c.Schema(p)
		if err != nil {
			t.Errorf("Schema(%q) error: %v", p, err)
			continue
		}
		if len(schema) == 0 {
			t.Errorf("Schema(%q) returned empty bytes", p)
		}
	}
}

func TestCapabilities(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	for _, p := range Providers {
		_, err := c.CapabilitiesFor(p)
		if err != nil {
			t.Errorf("CapabilitiesFor(%q) error: %v", p, err)
		}
	}
}
