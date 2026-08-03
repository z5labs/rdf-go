package lex

import "strings"

// IsPNCharsBase reports whether r is a PN_CHARS_BASE.
//
//	PN_CHARS_BASE ::= [A-Z] | [a-z] | [#x00C0-#x00D6] | [#x00D8-#x00F6]
//	                | [#x00F8-#x02FF] | [#x0370-#x037D] | [#x037F-#x1FFF]
//	                | [#x200C-#x200D] | [#x2070-#x218F] | [#x2C00-#x2FEF]
//	                | [#x3001-#xD7FF] | [#xF900-#xFDCF] | [#xFDF0-#xFFFD]
//	                | [#x10000-#xEFFFF]
func IsPNCharsBase(r rune) bool {
	switch {
	case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z':
		return true
	case r >= 0x00C0 && r <= 0x00D6,
		r >= 0x00D8 && r <= 0x00F6,
		r >= 0x00F8 && r <= 0x02FF,
		r >= 0x0370 && r <= 0x037D,
		r >= 0x037F && r <= 0x1FFF,
		r >= 0x200C && r <= 0x200D,
		r >= 0x2070 && r <= 0x218F,
		r >= 0x2C00 && r <= 0x2FEF,
		r >= 0x3001 && r <= 0xD7FF,
		r >= 0xF900 && r <= 0xFDCF,
		r >= 0xFDF0 && r <= 0xFFFD,
		r >= 0x10000 && r <= 0xEFFFF:
		return true
	default:
		return false
	}
}

// IsPNCharsU reports whether r is a PN_CHARS_U.
//
//	PN_CHARS_U ::= PN_CHARS_BASE | '_'
//
// A colon is not one. Some renderings of the grammar list it here, but the W3C
// suite settles it: nt-syntax-bad-bnode-01 (_::a) and nt-syntax-bad-bnode-02
// (_:abc:def) are both negative syntax tests, so a label carrying a colon is
// not a label.
func IsPNCharsU(r rune) bool {
	return IsPNCharsBase(r) || r == '_'
}

// IsPNChars reports whether r is a PN_CHARS.
//
//	PN_CHARS ::= PN_CHARS_U | '-' | [0-9] | #x00B7 | [#x0300-#x036F]
//	           | [#x203F-#x2040]
func IsPNChars(r rune) bool {
	switch {
	case IsPNCharsU(r), r == '-', r >= '0' && r <= '9', r == 0x00B7:
		return true
	case r >= 0x0300 && r <= 0x036F, r >= 0x203F && r <= 0x2040:
		return true
	default:
		return false
	}
}

// IsPNLocalEsc reports whether r is one of the characters a backslash may
// escape in a local name.
//
//	PN_LOCAL_ESC ::= '\' ('_' | '~' | '.' | '-' | '!' | '$' | '&' | "'" | '(' |
//	                      ')' | '*' | '+' | ',' | ';' | '=' | '/' | '?' | '#' |
//	                      '@' | '%')
func IsPNLocalEsc(r rune) bool {
	return strings.ContainsRune(`_~.-!$&'()*+,;=/?#@%`, r)
}

// IsDigit reports whether r is a decimal digit.
//
//	[0-9]
func IsDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

// IsHex reports whether r is a HEX digit.
//
//	HEX ::= [0-9] | [A-F] | [a-f]
func IsHex(r rune) bool {
	_, ok := hexValue(r)
	return ok
}

// hexValue returns the value of a HEX digit.
//
//	HEX ::= [0-9] | [A-F] | [a-f]
func hexValue(r rune) (int, bool) {
	switch {
	case r >= '0' && r <= '9':
		return int(r - '0'), true
	case r >= 'a' && r <= 'f':
		return int(r-'a') + 10, true
	case r >= 'A' && r <= 'F':
		return int(r-'A') + 10, true
	default:
		return 0, false
	}
}
