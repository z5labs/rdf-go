package rdf

import "strings"

const hexDigits = "0123456789ABCDEF"

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

// escapeIRI writes s to b escaped for the N-Triples IRIREF production.
//
// The production excludes the ASCII control range, the space, and a handful of
// delimiters, none of which have an ECHAR form inside an IRI, so each is
// written as a UCHAR:
//
//	IRIREF ::= '<' ([^#x00-#x20<>"{}|^`\] | UCHAR)* '>'
//
// As in [escapeString], byte iteration is safe because every excluded
// character is ASCII.
func escapeIRI(b *strings.Builder, s string) {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c <= 0x20 || strings.IndexByte(`<>"{}|^`+"`"+`\`, c) >= 0 {
			b.WriteString(`\u00`)
			b.WriteByte(hexDigits[c>>4])
			b.WriteByte(hexDigits[c&0x0F])
			continue
		}
		b.WriteByte(c)
	}
}
