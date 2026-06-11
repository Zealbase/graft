// Package omap provides a tiny insertion-ordered string-keyed map that
// marshals to YAML/JSON preserving key order. Provider packages use it to emit
// deterministic frontmatter / JSON with a stable, human-friendly field order
// (canonical fields first, then restored provider overrides).
package omap

import (
	"bytes"
	"encoding/json"

	"gopkg.in/yaml.v3"
)

// OMap is an ordered map of string keys to arbitrary values.
type OMap struct {
	keys []string
	vals map[string]any
}

// New returns an empty ordered map.
func New() *OMap {
	return &OMap{vals: map[string]any{}}
}

// Set inserts or updates a key, preserving first-insertion order.
func (m *OMap) Set(k string, v any) *OMap {
	if _, ok := m.vals[k]; !ok {
		m.keys = append(m.keys, k)
	}
	m.vals[k] = v
	return m
}

// Has reports whether the key is present.
func (m *OMap) Has(k string) bool { _, ok := m.vals[k]; return ok }

// Len returns the number of keys.
func (m *OMap) Len() int { return len(m.keys) }

// Keys returns the keys in insertion order.
func (m *OMap) Keys() []string { return m.keys }

// MarshalYAML emits a YAML mapping node with keys in insertion order.
func (m *OMap) MarshalYAML() (any, error) {
	n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, k := range m.keys {
		kn := &yaml.Node{}
		if err := kn.Encode(k); err != nil {
			return nil, err
		}
		vn := &yaml.Node{}
		if err := vn.Encode(m.vals[k]); err != nil {
			return nil, err
		}
		n.Content = append(n.Content, kn, vn)
	}
	return n, nil
}

// MarshalJSON emits a JSON object with keys in insertion order.
func (m *OMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		vb, err := json.Marshal(m.vals[k])
		if err != nil {
			return nil, err
		}
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
