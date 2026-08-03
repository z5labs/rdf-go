package lex_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/z5labs/rdf-go/internal/lex"
)

// reader returns a [lex.NextFunc] over src, standing in for the tokenizer that
// supplies one in the packages that use this.
//
// Running out is reported as [io.ErrUnexpectedEOF] rather than [io.EOF] to
// stand for the error a tokenizer reports there, and to pin that this package
// hands it back untouched instead of deciding what the end of the input means.
func reader(src string) lex.NextFunc {
	r := strings.NewReader(src)
	return func() (rune, error) {
		c, _, err := r.ReadRune()
		if errors.Is(err, io.EOF) {
			return 0, io.ErrUnexpectedEOF
		}
		return c, err
	}
}

// TestEscape covers both escape forms a backslash may begin, and the three
// ways one can fail.
func TestEscape(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		echar bool
		want  rune
	}{
		{name: "a tab", src: `t`, echar: true, want: '\t'},
		{name: "a backspace", src: `b`, echar: true, want: '\b'},
		{name: "a line feed", src: `n`, echar: true, want: '\n'},
		{name: "a carriage return", src: `r`, echar: true, want: '\r'},
		{name: "a form feed", src: `f`, echar: true, want: '\f'},
		{name: "a double quote", src: `"`, echar: true, want: '"'},
		{name: "a single quote", src: `'`, echar: true, want: '\''},
		{name: "a backslash", src: `\`, echar: true, want: '\\'},
		{name: "a short UCHAR", src: `u0041`, echar: true, want: 'A'},
		{name: "a long UCHAR", src: `U0001F600`, echar: true, want: 0x1F600},
		{name: "a UCHAR in an IRIREF, where ECHAR is not allowed", src: `u00E9`, want: 'é'},
		{name: "a UCHAR written in upper case", src: `uABCD`, echar: true, want: 0xABCD},
		{name: "a UCHAR written in lower case", src: `uabcd`, echar: true, want: 0xABCD},
		{name: "the highest code point", src: `U0010FFFF`, echar: true, want: 0x10FFFF},
		{name: "the null character", src: `u0000`, echar: true, want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := lex.Escape(reader(test.src), test.echar)
			if err != nil {
				t.Fatalf("Escape() = %v, want nil", err)
			}
			if got != test.want {
				t.Errorf("Escape() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestEscapeUnexpectedCharacter covers the escapes that name nothing, which
// are reported as the character that made them so — without a position, which
// only the caller can supply.
func TestEscapeUnexpectedCharacter(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		echar bool
		want  rune
	}{
		{name: "an escape the grammar has no use for", src: `q`, echar: true, want: 'q'},
		{name: "an ECHAR inside an IRIREF, where only UCHAR is allowed", src: `n`, want: 'n'},
		{name: "a non-hexadecimal digit in a UCHAR", src: `u00G0`, echar: true, want: 'G'},
		{name: "a UCHAR ended early by a quote", src: `u00"`, echar: true, want: '"'},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := lex.Escape(reader(test.src), test.echar)

			var unexpected lex.UnexpectedCharacterError
			if !errors.As(err, &unexpected) {
				t.Fatalf("Escape() = %v, want %T", err, unexpected)
			}
			if unexpected.R != test.want {
				t.Errorf("R = %q, want %q", unexpected.R, test.want)
			}
		})
	}
}

// TestEscapeInvalidCodePoint covers the values eight hexadecimal digits can
// spell that Unicode has no character for.
func TestEscapeInvalidCodePoint(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want uint32
	}{
		{name: "the first surrogate", src: `uD800`, want: 0xD800},
		{name: "the last surrogate", src: `uDFFF`, want: 0xDFFF},
		{name: "the code point above the highest", src: `U00110000`, want: 0x110000},
		{name: "a value far above it", src: `UFFFFFFFF`, want: 0xFFFFFFFF},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := lex.Escape(reader(test.src), true)

			var invalid lex.InvalidCodePointError
			if !errors.As(err, &invalid) {
				t.Fatalf("Escape() = %v, want %T", err, invalid)
			}
			if invalid.Code != test.want {
				t.Errorf("Code = %#X, want %#X", invalid.Code, test.want)
			}
		})
	}
}

// TestEscapeReturnsReadErrors pins that an error from the [lex.NextFunc] comes
// back as it was given: the caller decides what running out of input means,
// and this package does not turn it into something of its own.
func TestEscapeReturnsReadErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{name: "nothing after the backslash", src: ``},
		{name: "a UCHAR with no digits", src: `u`},
		{name: "a UCHAR short of its digits", src: `u00`},
		{name: "a long UCHAR short of its digits", src: `U0001F6`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := lex.Escape(reader(test.src), true)
			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Errorf("Escape() = %v, want %v", err, io.ErrUnexpectedEOF)
			}
		})
	}
}

// TestPercentEscape covers the escape that is carried through rather than
// decoded, so what comes back is what was written.
func TestPercentEscape(t *testing.T) {
	tests := []struct {
		name       string
		src        string
		wantHi     rune
		wantLo     rune
		wantErrRne rune
		wantEOF    bool
	}{
		{name: "two digits", src: "20", wantHi: '2', wantLo: '0'},
		{name: "upper case letters", src: "C3rest", wantHi: 'C', wantLo: '3'},
		{name: "lower case letters", src: "af", wantHi: 'a', wantLo: 'f'},
		{name: "a first digit that is not hexadecimal", src: "g0", wantErrRne: 'g'},
		{name: "a second digit that is not hexadecimal", src: "0g", wantErrRne: 'g'},
		{name: "no digits at all", src: "", wantEOF: true},
		{name: "one digit and no more", src: "2", wantEOF: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			hi, lo, err := lex.PercentEscape(reader(test.src))

			switch {
			case test.wantEOF:
				if !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatalf("PercentEscape() = %v, want %v", err, io.ErrUnexpectedEOF)
				}
			case test.wantErrRne != 0:
				var unexpected lex.UnexpectedCharacterError
				if !errors.As(err, &unexpected) {
					t.Fatalf("PercentEscape() = %v, want %T", err, unexpected)
				}
				if unexpected.R != test.wantErrRne {
					t.Errorf("R = %q, want %q", unexpected.R, test.wantErrRne)
				}
			default:
				if err != nil {
					t.Fatalf("PercentEscape() = %v, want nil", err)
				}
				if hi != test.wantHi || lo != test.wantLo {
					t.Errorf("PercentEscape() = %q, %q, want %q, %q", hi, lo, test.wantHi, test.wantLo)
				}
			}
		})
	}
}

// TestErrorMessages pins what the two errors say. They are wrapped by the
// package that reports them, which adds a position and nothing else, so the
// wording here is what a reader of a parse error ends up seeing.
func TestErrorMessages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "an unexpected character",
			err:  lex.UnexpectedCharacterError{R: 'q'},
			want: `unexpected character 'q'`,
		},
		{
			name: "a code point above the highest",
			err:  lex.InvalidCodePointError{Code: 0x110000},
			want: "escape names no character, U+110000",
		},
		{
			name: "a surrogate",
			err:  lex.InvalidCodePointError{Code: 0xD800},
			want: "escape names no character, U+D800",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.err.Error(); got != test.want {
				t.Errorf("Error() = %q, want %q", got, test.want)
			}
		})
	}
}
