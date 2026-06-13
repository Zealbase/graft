// Package fmark is a shared helper for providers whose on-disk format is a
// Markdown document with a leading YAML frontmatter block delimited by "---"
// fences (claude-code, cursor, github-copilot, gemini-cli, opencode).
//
// It only splits/joins the frontmatter bytes from the Markdown body. Each
// provider decodes the returned frontmatter bytes into its OWN typed struct
// (for the format contract) and, for losslessness, also into a generic map to
// capture any keys the struct does not model. fmark stays format-agnostic about
// the field set.
package fmark

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// bom is the UTF-8 byte order mark, sometimes prefixed to text files.
const bom = "\uFEFF"

// Split separates a Markdown-with-frontmatter document into the raw YAML
// frontmatter bytes and the Markdown body. A file with no leading "---" fence
// is treated as all body with empty frontmatter.
func Split(raw []byte) (frontmatter []byte, body string) {
	s := strings.TrimPrefix(string(raw), bom)
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return nil, string(raw)
	}
	nl := strings.IndexByte(s, '\n')
	rest := s[nl+1:]
	end := findFence(rest)
	if end < 0 {
		return nil, string(raw)
	}
	fmText := rest[:end]
	bodyText := stripFenceLine(rest[end:])
	return []byte(fmText), bodyText
}

// findFence returns the byte offset of a line equal to "---" (ignoring a
// trailing \r), or -1.
func findFence(s string) int {
	off := 0
	for {
		line, next := readLine(s[off:])
		if strings.TrimRight(line, "\r") == "---" {
			return off
		}
		if next < 0 {
			return -1
		}
		off += next
	}
}

func readLine(s string) (string, int) {
	i := strings.IndexByte(s, '\n')
	if i < 0 {
		return s, -1
	}
	return s[:i], i + 1
}

func stripFenceLine(s string) string {
	i := strings.IndexByte(s, '\n')
	if i < 0 {
		return ""
	}
	return s[i+1:]
}

// DecodeMap unmarshals frontmatter bytes into a generic map (for capturing
// keys not modeled by a provider's typed struct). Empty frontmatter yields an
// empty map.
//
// Numeric normalization: yaml.v3 decodes whole-number floats (e.g.
// "temperature: 1.0") as int (1) because YAML treats them as integers when
// there is no decimal portion in the parsed value.  When the map is later
// re-marshalled the int 1 renders as "1" instead of "1.0", producing a
// spurious SourceHash drift on the next load.  To prevent this, all int
// values in the top-level map are promoted to float64 after unmarshalling —
// provider numeric frontmatter fields (temperature, top_p, etc.) are always
// semantically numeric, never int-with-distinct-semantics.
func DecodeMap(frontmatter []byte) (map[string]any, error) {
	m := map[string]any{}
	if len(bytes.TrimSpace(frontmatter)) == 0 {
		return m, nil
	}
	if err := yaml.Unmarshal(frontmatter, &m); err != nil {
		return nil, fmt.Errorf("fmark: parse frontmatter: %w", err)
	}
	if m == nil {
		m = map[string]any{}
	}
	normalizeInts(m)
	return m, nil
}

// normalizeInts walks a map[string]any and promotes any int or int64 value to
// float64.  This prevents yaml.v3's whole-number float coercion (e.g.
// temperature: 1.0 → int(1)) from causing spurious SourceHash drift when the
// map is later re-marshalled.
func normalizeInts(m map[string]any) {
	for k, v := range m {
		switch val := v.(type) {
		case int:
			m[k] = float64(val)
		case int64:
			m[k] = float64(val)
		}
	}
}

// Decode unmarshals frontmatter bytes into the provided typed struct pointer.
func Decode(frontmatter []byte, v any) error {
	if len(bytes.TrimSpace(frontmatter)) == 0 {
		return nil
	}
	if err := yaml.Unmarshal(frontmatter, v); err != nil {
		return fmt.Errorf("fmark: decode frontmatter: %w", err)
	}
	return nil
}

// Join renders frontmatter bytes plus a body into a complete document.
func Join(frontmatter []byte, body string) []byte {
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(frontmatter)
	if !bytes.HasSuffix(frontmatter, []byte("\n")) {
		buf.WriteByte('\n')
	}
	buf.WriteString("---\n")
	if body != "" {
		buf.WriteString(body)
	}
	return buf.Bytes()
}

// MarshalYAML marshals v deterministically with 2-space indent.
func MarshalYAML(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("fmark: encode frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("fmark: encode frontmatter: %w", err)
	}
	return buf.Bytes(), nil
}
