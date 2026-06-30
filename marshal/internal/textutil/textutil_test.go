// SPDX-License-Identifier: AGPL-3.0-or-later
package textutil

import (
	"strings"
	"testing"
)

// Test_Sanitise_StripsUnsafeCharacters verifies that SanitiseForTerminal
// removes all control characters, Unicode bidi formatting, line/paragraph
// separators, zero-width characters, and the Arabic Letter Mark from a
// string, while preserving normal printable characters.
func Test_Sanitise_StripsUnsafeCharacters(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "plain name unchanged", input: "Alice", expected: "Alice"},
		{name: "NUL stripped", input: "Alice\x00Bob", expected: "AliceBob"},
		{name: "all control chars stripped", input: "\x01\x1f", expected: ""},
		{name: "DEL stripped", input: "\x7f", expected: ""},
		{name: "DEL in middle stripped", input: "Alice\x7fBob", expected: "AliceBob"},
		{name: "space preserved and tab stripped", input: " spaces and\ttabs", expected: " spaces andtabs"},
		{name: "newline stripped", input: "Alice\nBob", expected: "AliceBob"},
		{name: "empty string", input: "", expected: ""},
		{name: "non-ASCII printable preserved", input: "café", expected: "café"},
		// C1 control characters (U+0080–U+009F) — added to guard against 8-bit CSI injection
		{name: "C1 boundary-start stripped", input: "\u0080", expected: ""},
		{name: "C1 8-bit CSI stripped", input: "\u009b", expected: ""},
		{name: "C1 boundary-end stripped", input: "\u009f", expected: ""},
		{name: "C1 embedded in string stripped", input: "sha256:\u009b1A", expected: "sha256:1A"},
		{name: "U+00A0 no-break space preserved (just above C1)", input: "\u00a0", expected: "\u00a0"},
		// Unicode bidi / line-separator characters — Trojan-source visual-spoof guard
		{name: "U+202E RLO stripped (right-to-left override)", input: "/mnt/\u202egnp.txt", expected: "/mnt/gnp.txt"},
		{name: "U+200E LRM stripped (left-to-right mark)", input: "path\u200e/sub", expected: "path/sub"},
		{name: "U+200F RLM stripped (right-to-left mark)", input: "path\u200f/sub", expected: "path/sub"},
		{name: "U+202A bidi embedding start stripped", input: "\u202apath", expected: "path"},
		{name: "U+202B bidi embedding start stripped (RLE)", input: "\u202bpath", expected: "path"},
		{name: "U+202C PDF stripped (pop directional formatting)", input: "path\u202c", expected: "path"},
		{name: "U+202D LRO stripped (left-to-right override)", input: "\u202dpath", expected: "path"},
		{name: "U+2066 LRI stripped (left-to-right isolate)", input: "\u2066path\u2069", expected: "path"},
		{name: "U+2067 RLI stripped (right-to-left isolate)", input: "\u2067path\u2069", expected: "path"},
		{name: "U+2068 FSI stripped (first strong isolate)", input: "\u2068path\u2069", expected: "path"},
		{name: "U+2069 PDI stripped (pop directional isolate)", input: "path\u2069", expected: "path"},
		{name: "U+2028 line separator stripped", input: "line\u2028break", expected: "linebreak"},
		{name: "U+2029 paragraph separator stripped", input: "para\u2029break", expected: "parabreak"},
		{name: "bidi chars mixed with plain text stripped, plain preserved", input: "real\u202e/etc/passwd", expected: "real/etc/passwd"},
		// Zero-width formatting characters — visual-distortion guard for mount paths
		{name: "zero-width chars stripped", input: "/mnt/\u200bfoo\u200cbar\u200dbaz\ufeffe", expected: "/mnt/foobarbaze"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When SanitiseForTerminal is called with the test input
			got := SanitiseForTerminal(tt.input)

			// Then the output must match the expected sanitised result
			if got != tt.expected {
				t.Errorf("SanitiseForTerminal(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// Test_Sanitise_RemovesArabicLetterMark verifies that U+061C (ARABIC LETTER
// MARK), a Unicode format character that some terminals honour for display
// reordering, is removed from a path string before terminal output.
func Test_Sanitise_RemovesArabicLetterMark(t *testing.T) {
	// Given a path containing the Arabic Letter Mark (U+061C)
	input := "/home/user/\u061Cpath"

	// When SanitiseForTerminal is called
	got := SanitiseForTerminal(input)

	// Then the output must not contain U+061C
	if strings.ContainsRune(got, '\u061C') {
		t.Errorf("SanitiseForTerminal(%q) = %q; output still contains U+061C (Arabic Letter Mark)", input, got)
	}
}
