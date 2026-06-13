package fmark

import (
	"testing"
)

// TestDecodeMapNumericNormalization verifies that yaml.v3's int coercion of
// whole-number floats (e.g. "temperature: 1.0" → int(1)) is corrected to
// float64 by DecodeMap, so that a round-trip through MarshalYAML + DecodeMap
// does not produce spurious SourceHash drift.
func TestDecodeMapNumericNormalization(t *testing.T) {
	input := []byte("temperature: 1.0\ntop_p: 0.9\nmax_tokens: 4096\n")

	m, err := DecodeMap(input)
	if err != nil {
		t.Fatalf("DecodeMap error: %v", err)
	}

	// temperature: 1.0 — yaml.v3 decodes as int(1); normalizeInts must lift to float64.
	temp, ok := m["temperature"]
	if !ok {
		t.Fatal("key 'temperature' missing")
	}
	if _, isFloat := temp.(float64); !isFloat {
		t.Errorf("temperature: want float64, got %T (%v)", temp, temp)
	}

	// top_p: 0.9 — has a fractional part; yaml.v3 decodes as float64 already.
	topP, ok := m["top_p"]
	if !ok {
		t.Fatal("key 'top_p' missing")
	}
	if _, isFloat := topP.(float64); !isFloat {
		t.Errorf("top_p: want float64, got %T (%v)", topP, topP)
	}

	// max_tokens: 4096 — a plain integer key; promoted to float64.
	mt, ok := m["max_tokens"]
	if !ok {
		t.Fatal("key 'max_tokens' missing")
	}
	if _, isFloat := mt.(float64); !isFloat {
		t.Errorf("max_tokens: want float64, got %T (%v)", mt, mt)
	}
}

// TestDecodeMapRoundTrip verifies that a document containing "temperature: 1.0"
// round-trips through DecodeMap → MarshalYAML → DecodeMap without type drift
// (i.e. the second DecodeMap still yields a float64, not an int).
func TestDecodeMapRoundTrip(t *testing.T) {
	original := []byte("temperature: 1.0\n")

	m1, err := DecodeMap(original)
	if err != nil {
		t.Fatalf("first DecodeMap: %v", err)
	}

	remarshalled, err := MarshalYAML(m1)
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}

	m2, err := DecodeMap(remarshalled)
	if err != nil {
		t.Fatalf("second DecodeMap: %v", err)
	}

	v1 := m1["temperature"]
	v2 := m2["temperature"]

	if _, ok := v1.(float64); !ok {
		t.Errorf("round-trip pass 1: temperature is %T, want float64", v1)
	}
	if _, ok := v2.(float64); !ok {
		t.Errorf("round-trip pass 2: temperature is %T, want float64", v2)
	}

	// Values must be numerically equal across the round-trip.
	f1, _ := v1.(float64)
	f2, _ := v2.(float64)
	if f1 != f2 {
		t.Errorf("round-trip value drift: %v → %v", f1, f2)
	}
}

// TestSplitJoinRoundTrip verifies that Split + Join reconstructs the original
// document exactly (no BOM stripping, no byte corruption).
func TestSplitJoinRoundTrip(t *testing.T) {
	doc := []byte("---\nmodel: gpt-4o\ntemperature: 0.7\n---\n# Hello\n\nBody text.\n")
	fm, body := Split(doc)
	got := Join(fm, body)
	if string(got) != string(doc) {
		t.Errorf("Split+Join mismatch:\nwant: %q\n got: %q", doc, got)
	}
}
