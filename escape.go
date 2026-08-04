package rdf

import "strings"

// escapeString writes s to b escaped for the N-Triples STRING_LITERAL_QUOTE
// production.
//
// Canonical N-Triples escapes exactly four characters inside a quoted literal
// — '"', '\', line feed and carriage return — because the production admits
// every other character directly:
//
//	STRING_LITERAL_QUOTE ::= '"' ([^#x22#x5C#xA#xD] | ECHAR | UCHAR)* '"'
//
// A tab, for instance, is written literally rather than as \t: both parse, but
// only one is canonical.
//
// Iterating bytes rather than runes is safe because every escaped character is
// ASCII, and no byte of a multi-byte UTF-8 sequence is ever below 0x80.
func escapeString(b *strings.Builder, s string) {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteByte(c)
		}
	}
}
