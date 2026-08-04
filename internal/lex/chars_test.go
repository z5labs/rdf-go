package lex_test

import (
	"testing"

	"github.com/z5labs/rdf-go/internal/lex"
)

// TestIsPNCharsBase walks the edge of every range the production lists, since
// a character class written as a table of ranges goes wrong at its ends and
// nowhere else.
func TestIsPNCharsBase(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{name: "an upper case letter", r: 'A', want: true},
		{name: "the last upper case letter", r: 'Z', want: true},
		{name: "a lower case letter", r: 'a', want: true},
		{name: "the last lower case letter", r: 'z', want: true},
		{name: "a digit", r: '0', want: false},
		{name: "an underscore", r: '_', want: false},
		{name: "a colon", r: ':', want: false},
		{name: "a hyphen", r: '-', want: false},
		{name: "the character before the first Latin range", r: 0x00BF, want: false},
		{name: "the first character of the Latin range", r: 0x00C0, want: true},
		{name: "the last character of the Latin range", r: 0x00D6, want: true},
		{name: "the multiplication sign the range excludes", r: 0x00D7, want: false},
		{name: "the first character of the range after it", r: 0x00D8, want: true},
		{name: "the division sign the range excludes", r: 0x00F7, want: false},
		{name: "a zero width non-joiner", r: 0x200C, want: true},
		{name: "a zero width joiner", r: 0x200D, want: true},
		{name: "the character after the joiners", r: 0x200E, want: false},
		{name: "the last character below the surrogates", r: 0xD7FF, want: true},
		{name: "the first surrogate", r: 0xD800, want: false},
		{name: "the last character of the plane", r: 0xFFFD, want: true},
		{name: "the noncharacter after it", r: 0xFFFE, want: false},
		{name: "the first supplementary character", r: 0x10000, want: true},
		{name: "the last supplementary character", r: 0xEFFFF, want: true},
		{name: "the character after the supplementary range", r: 0xF0000, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := lex.IsPNCharsBase(test.r); got != test.want {
				t.Errorf("IsPNCharsBase(%q) = %t, want %t", test.r, got, test.want)
			}
		})
	}
}

// TestIsPNCharsU covers what PN_CHARS_U adds to PN_CHARS_BASE, and the one
// character readings of the grammar disagree about.
func TestIsPNCharsU(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{name: "a letter, which PN_CHARS_BASE already admits", r: 'a', want: true},
		{name: "the underscore it adds", r: '_', want: true},
		{name: "a colon, which it does not add", r: ':', want: false},
		{name: "a digit", r: '0', want: false},
		{name: "a hyphen", r: '-', want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := lex.IsPNCharsU(test.r); got != test.want {
				t.Errorf("IsPNCharsU(%q) = %t, want %t", test.r, got, test.want)
			}
		})
	}
}

// TestIsPNChars covers what PN_CHARS adds to PN_CHARS_U: the characters a name
// may carry after its first.
func TestIsPNChars(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{name: "a letter", r: 'a', want: true},
		{name: "an underscore", r: '_', want: true},
		{name: "the hyphen it adds", r: '-', want: true},
		{name: "the first digit it adds", r: '0', want: true},
		{name: "the last digit it adds", r: '9', want: true},
		{name: "the middle dot it adds", r: 0x00B7, want: true},
		{name: "the first combining mark", r: 0x0300, want: true},
		{name: "the last combining mark", r: 0x036F, want: true},
		{name: "the character after the combining marks", r: 0x0370, want: true},
		{name: "the first undertie", r: 0x203F, want: true},
		{name: "the last undertie", r: 0x2040, want: true},
		{name: "the character after the underties", r: 0x2041, want: false},
		{name: "a dot, which only a rule may allow", r: '.', want: false},
		{name: "a colon", r: ':', want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := lex.IsPNChars(test.r); got != test.want {
				t.Errorf("IsPNChars(%q) = %t, want %t", test.r, got, test.want)
			}
		})
	}
}

// TestIsPNLocalEsc pins the whole of PN_LOCAL_ESC, which is short enough to
// state exactly, and two characters that look as though they belong to it.
func TestIsPNLocalEsc(t *testing.T) {
	const escapable = `_~.-!$&'()*+,;=/?#@%`

	for _, r := range escapable {
		t.Run("the escapable "+string(r), func(t *testing.T) {
			if !lex.IsPNLocalEsc(r) {
				t.Errorf("IsPNLocalEsc(%q) = false, want true", r)
			}
		})
	}

	tests := []struct {
		name string
		r    rune
	}{
		{name: "a letter", r: 'a'},
		{name: "a backslash", r: '\\'},
		{name: "a colon", r: ':'},
		{name: "a quote", r: '"'},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if lex.IsPNLocalEsc(test.r) {
				t.Errorf("IsPNLocalEsc(%q) = true, want false", test.r)
			}
		})
	}
}

// TestIsDigit and TestIsHex cover the two digit classes, which differ in the
// six letters HEX adds and in nothing else.
func TestIsDigit(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{name: "the first digit", r: '0', want: true},
		{name: "the last digit", r: '9', want: true},
		{name: "the character before the first", r: '/', want: false},
		{name: "the character after the last", r: ':', want: false},
		{name: "a hexadecimal letter, which is not a digit here", r: 'a', want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := lex.IsDigit(test.r); got != test.want {
				t.Errorf("IsDigit(%q) = %t, want %t", test.r, got, test.want)
			}
		})
	}
}

func TestIsHex(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{name: "a decimal digit", r: '7', want: true},
		{name: "the first lower case letter", r: 'a', want: true},
		{name: "the last lower case letter", r: 'f', want: true},
		{name: "the letter after it", r: 'g', want: false},
		{name: "the first upper case letter", r: 'A', want: true},
		{name: "the last upper case letter", r: 'F', want: true},
		{name: "the letter after it", r: 'G', want: false},
		{name: "a space", r: ' ', want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := lex.IsHex(test.r); got != test.want {
				t.Errorf("IsHex(%q) = %t, want %t", test.r, got, test.want)
			}
		})
	}
}

// TestIsIRIChar walks the excluded set and the characters either side of the
// one range the production writes, an IRIREF being defined by what it leaves
// out rather than by what it admits.
func TestIsIRIChar(t *testing.T) {
	tests := []struct {
		name string
		r    rune
		want bool
	}{
		{name: "a letter", r: 'a', want: true},
		{name: "a digit", r: '0', want: true},
		{name: "a colon", r: ':', want: true},
		{name: "a solidus", r: '/', want: true},
		{name: "a percent sign, which is how a space is written", r: '%', want: true},
		{name: "a non-ascii character", r: 'ü', want: true},
		{name: "the null character", r: 0x00, want: false},
		{name: "a tab", r: '\t', want: false},
		{name: "a line feed", r: '\n', want: false},
		{name: "a carriage return", r: '\r', want: false},
		{name: "the last character of the excluded range", r: 0x20, want: false},
		{name: "the first character after it", r: 0x21, want: true},
		{name: "a less-than sign", r: '<', want: false},
		{name: "a greater-than sign", r: '>', want: false},
		{name: "a quotation mark", r: '"', want: false},
		{name: "a left brace", r: '{', want: false},
		{name: "a right brace", r: '}', want: false},
		{name: "a vertical line", r: '|', want: false},
		{name: "a circumflex accent", r: '^', want: false},
		{name: "a grave accent", r: '`', want: false},
		{name: "a reverse solidus", r: '\\', want: false},
		{name: "the delete character, which the production admits", r: 0x7F, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := lex.IsIRIChar(test.r); got != test.want {
				t.Errorf("IsIRIChar(%q) = %t, want %t", test.r, got, test.want)
			}
		})
	}
}
