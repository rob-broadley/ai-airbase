// SPDX-License-Identifier: AGPL-3.0-or-later
package textutil

import "strings"

// SanitiseForTerminal strips C0 control characters (< 0x20), DEL (0x7f), C1
// control characters (0x80–0x9F, including the 8-bit CSI U+009B), Unicode
// bidirectional formatting characters (U+200E–U+200F, U+202A–U+202E,
// U+2066–U+2069), line/paragraph separators (U+2028–U+2029), zero-width
// characters (U+200B Zero Width Space, U+200C Zero Width Non-Joiner,
// U+200D Zero Width Joiner, U+FEFF BOM/Zero Width No-Break Space), and the
// Arabic Letter Mark (U+061C) from a string before writing it to terminal
// output, guarding against escape sequence injection and Trojan-source
// visual-spoof attacks from untrusted sources such as OCI image labels.
func SanitiseForTerminal(v string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) ||
			(r >= 0x200e && r <= 0x200f) || // LRM, RLM
			(r >= 0x202a && r <= 0x202e) || // bidi embedding/override
			(r >= 0x2066 && r <= 0x2069) || // bidi isolates
			r == 0x2028 || r == 0x2029 || // line/paragraph separator
			r == 0x200b || r == 0x200c || r == 0x200d || r == 0xfeff || // zero-width chars
			r == 0x061c { // Arabic Letter Mark
			return -1 // drop control and bidi formatting characters
		}
		return r
	}, v)
}
