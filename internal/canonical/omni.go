package canonical

import (
	"fmt"
	"strings"
)

// Omni sentinel markers. The prepended omni block is wrapped in these stable
// HTML comments so that on re-init / re-refresh the existing block is recognized
// and REPLACED in place rather than duplicated. The block is ordinary Markdown
// to every downstream consumer (core/sync fans it out to providers verbatim).
//
// Layout (the whole block is the FIRST block of the body):
//
//	<!-- graft:omni <ref> -->
//	<sysInstr>
//	<!-- /graft:omni -->
//	<original body…>
const (
	omniOpenPrefix = "<!-- graft:omni "
	omniOpenSuffix = " -->"
	omniClose      = "<!-- /graft:omni -->"
)

// ContainsOmniMarker reports whether s contains a line that would collide with
// graft's omni sentinel markers: a line equal to the close marker, or a line
// beginning with the open-marker prefix. Such a line inside resolved
// sys-instructions would make the prepended block self-corrupting (the next
// stripLeadingOmniBlock would match the embedded close marker and truncate
// mid-content), so the gateway uses this to refuse applying such input.
//
// Scanning is line-oriented and tolerant of CRLF: each line has any trailing CR
// stripped before comparison, mirroring stripLeadingOmniBlock.
func ContainsOmniMarker(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == omniClose || strings.HasPrefix(line, omniOpenPrefix) {
			return true
		}
	}
	return false
}

// PrependOmniBlock returns body with a graft-managed omni block as its first
// block. If a graft-managed omni block already leads the body it is REPLACED in
// place (never duplicated, never nested). Calling it twice with the same args is
// identical to calling it once (idempotent).
//
// PRECONDITION: sysInstr must NOT contain a line that collides with the omni
// sentinel markers (see ContainsOmniMarker) — such a line would make the
// resulting block self-corrupting on the next strip/refresh. This function does
// NOT validate or sanitize sysInstr; the gateway enforces the precondition at
// its resolver boundary (gateway.applyOmni) before any Body write.
//
// Rules:
//   - ref == "" is a no-op: body is returned unchanged.
//   - Only graft's OWN leading sentinel block is ever recognized/replaced. A
//     literal "<!-- graft:omni … -->" that a user authored elsewhere in the body
//     (or not as the exact leading block) is preserved verbatim — it is never
//     treated as the managed block, stripped, or duplicated.
//   - Body bytes outside the managed block are preserved faithfully (multiline,
//     CRLF/LF, leading/trailing newlines).
func PrependOmniBlock(body, ref, sysInstr string) string {
	if ref == "" {
		return body
	}

	// Strip an existing leading graft block, if any, so we replace in place.
	stripped, _ := stripLeadingOmniBlock(body)

	block := omniOpenPrefix + ref + omniOpenSuffix + "\n" +
		sysInstr + "\n" +
		omniClose

	if stripped == "" {
		return block
	}
	// One blank line separates the managed block from the original body, mirroring
	// normal Markdown block separation. The original body bytes are untouched.
	return block + "\n" + stripped
}

// ReplaceOmniBlock replaces (or inserts) the leading graft-managed omni block.
// It has identical semantics to PrependOmniBlock — it exists as a named entry
// point for the refresh path where "replace" reads more clearly than "prepend".
func ReplaceOmniBlock(body, ref, sysInstr string) string {
	return PrependOmniBlock(body, ref, sysInstr)
}

// HasOmniBlock reports whether body begins with a graft-managed omni block.
func HasOmniBlock(body string) bool {
	_, found := stripLeadingOmniBlock(body)
	return found
}

// stripLeadingOmniBlock removes a graft-managed omni block from the very start
// of body and returns the remainder plus whether a block was found and stripped.
// When no managed leading block is present, body is returned unchanged with
// found=false.
//
// Recognition is anchored to the LEADING position and the exact sentinel form:
// the body must begin with the open marker prefix on its first line, and a close
// marker line must follow. This is what protects user-authored literal sentinels
// elsewhere in the body — only a block that is genuinely first and well-formed is
// stripped.
//
// This relies on the precondition (see PrependOmniBlock / ContainsOmniMarker)
// that graft never writes sysInstr containing a sentinel-colliding line into the
// managed block: the FIRST close-marker line is treated as the block's end, so a
// stray close marker inside the sysInstr would truncate the block mid-content.
func stripLeadingOmniBlock(body string) (string, bool) {
	// The open marker must be the first thing in the body (no leading whitespace
	// is tolerated — graft always writes it flush at offset 0).
	if !strings.HasPrefix(body, omniOpenPrefix) {
		return body, false
	}

	// The first line must be a complete, well-formed open marker:
	// "<!-- graft:omni <ref> -->" with nothing trailing on the line.
	firstLineEnd := strings.IndexByte(body, '\n')
	var firstLine, rest string
	if firstLineEnd < 0 {
		firstLine, rest = body, ""
	} else {
		firstLine, rest = body[:firstLineEnd], body[firstLineEnd+1:]
	}
	// Tolerate a trailing CR (CRLF body) on the marker line.
	openLine := strings.TrimSuffix(firstLine, "\r")
	if !strings.HasSuffix(openLine, omniOpenSuffix) {
		return body, false
	}

	// Find the matching close marker. It must appear as its own line within the
	// remainder. We scan line-by-line so a close marker embedded mid-line in user
	// content is not matched.
	idx := 0
	for idx < len(rest) {
		nl := strings.IndexByte(rest[idx:], '\n')
		var line string
		var lineEnd int
		if nl < 0 {
			line = rest[idx:]
			lineEnd = len(rest)
		} else {
			line = rest[idx : idx+nl]
			lineEnd = idx + nl + 1
		}
		if strings.TrimSuffix(line, "\r") == omniClose {
			// Everything after the close marker line is the original body.
			return rest[lineEnd:], true
		}
		idx = lineEnd
	}

	// Open marker present but no close marker found: this is NOT a well-formed
	// graft block (e.g. a user wrote a half-open sentinel). Leave body untouched.
	return body, false
}

// DefaultOmniResolver is the honest stub shipped in v0..0.7. It reports that no
// ref is supported and refuses to resolve, so omni refs are recorded in
// .meta.json but never applied until a real resolver capability ships. It does
// NOT fabricate any omni execution.
type DefaultOmniResolver struct{}

// Supported always reports false: the default resolver cannot resolve any ref.
func (DefaultOmniResolver) Supported(string) bool { return false }

// Resolve always returns an error: the default resolver performs no resolution.
func (DefaultOmniResolver) Resolve(ref string) (string, error) {
	return "", fmt.Errorf("canonical: omni agent resolution not yet supported (ref %q)", ref)
}
