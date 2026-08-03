package turtle_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/z5labs/rdf-go/rdf11/turtle"
)

// The rules covered here came from the N-Triples tokenizer along with the rest
// of the file they sit in. They are exercised again because the copies are
// still separate code — internal/lex holds the characters they read, not the
// rules that read them — and a fix to one that missed the other would show up
// nowhere else.

func TestTokenizeSharedTerminals(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []turtle.Token
	}{
		{
			name:     "an empty iri",
			src:      `<>`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenIRIRef, "")},
		},
		{
			name:     "a UCHAR inside an iri",
			src:      "<http://e/\\u0061>",
			expected: []turtle.Token{tok(1, 1, turtle.TokenIRIRef, "http://e/a")},
		},
		{
			name:     "an eight digit UCHAR",
			src:      `"\U0001F600"`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenString, "\U0001F600")},
		},
		{
			name:     "every ECHAR",
			src:      `"\t\b\n\r\f\"\'\\"`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenString, "\t\b\n\r\f\"'\\")},
		},
		{
			name: "a blank node label holding a dot",
			src:  `_:a.b`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenBlankNodeLabel, "a.b"),
			},
		},
		{
			name: "a blank node label ending at the statement's dot",
			src:  `_:b0.`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenBlankNodeLabel, "b0"),
				tok(1, 5, turtle.TokenDot, "."),
			},
		},
		{
			name:     "a blank node label with non ascii letters",
			src:      `_:日本語`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenBlankNodeLabel, "日本語")},
		},
		{
			name:     "a comment with no text",
			src:      "#",
			expected: []turtle.Token{tok(1, 1, turtle.TokenComment, "#")},
		},
		{
			// A colon ends a blank node label, which may not hold one, and
			// begins a prefixed name on the empty prefix. N-Triples has no
			// prefixed names and so reports the colon as an error instead.
			name: "a colon after a blank node label begins a prefixed name",
			src:  `_:a:b`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenBlankNodeLabel, "a"),
				tok(1, 4, turtle.TokenPNameLN, ":b"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
	}
}

func TestTokenizeSharedErrors(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name:        "a space inside an iri",
			src:         `<http://e/a b>`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 12}, R: ' '},
		},
		{
			name:        "an ECHAR is not allowed inside an iri",
			src:         `<http://e/\n>`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 12}, R: 'n'},
		},
		{
			name:        "an unterminated iri at the end of the input",
			src:         `<http://e/a`,
			expectedErr: turtle.UnterminatedIRIRefError{Pos: turtle.Pos{Line: 1, Column: 1}},
		},
		{
			name:        "an unknown escape in a string",
			src:         `"a\qb"`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 4}, R: 'q'},
		},
		{
			name:        "a non hexadecimal digit in a UCHAR",
			src:         `"\u00g0"`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 6}, R: 'g'},
		},
		{
			name:        "a UCHAR naming a surrogate",
			src:         `"\uD800"`,
			expectedErr: turtle.InvalidCodePointError{Pos: turtle.Pos{Line: 1, Column: 2}, Code: 0xD800},
		},
		{
			name:        "a UCHAR above the highest code point",
			src:         `"\U00110000"`,
			expectedErr: turtle.InvalidCodePointError{Pos: turtle.Pos{Line: 1, Column: 2}, Code: 0x110000},
		},
		{
			name:        "an underscore not followed by a colon",
			src:         `_x`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 2}, R: 'x'},
		},
		{
			name:        "a single caret",
			src:         `"o"^x`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 5}, R: 'x'},
		},
		{
			name:        "a blank node label cut off after its colon",
			src:         `_:`,
			expectedErr: turtle.UnexpectedEndOfInputError{Pos: turtle.Pos{Line: 1, Column: 3}},
		},
		{
			name:        "a caret at the end of the input",
			src:         `"o"^`,
			expectedErr: turtle.UnexpectedEndOfInputError{Pos: turtle.Pos{Line: 1, Column: 5}},
		},
		{
			name:        "a local name escape cut off",
			src:         `ex:a\`,
			expectedErr: turtle.UnexpectedEndOfInputError{Pos: turtle.Pos{Line: 1, Column: 6}},
		},
		{
			name:        "a percent escape cut off",
			src:         `ex:a%2`,
			expectedErr: turtle.UnexpectedEndOfInputError{Pos: turtle.Pos{Line: 1, Column: 7}},
		},
		{
			name:        "an exponent with no digits",
			src:         `1e`,
			expectedErr: turtle.UnexpectedEndOfInputError{Pos: turtle.Pos{Line: 1, Column: 3}},
		},
		{
			name:        "an exponent followed by something that is not a digit",
			src:         `1e! `,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 3}, R: '!'},
		},
		{
			// Every numeric production requires a digit, so a sign and a
			// decimal point on their own are not a number, however the rest of
			// the input goes on.
			name:        "a minus sign and a point at the end of the input",
			src:         `-.`,
			expectedErr: turtle.UnexpectedEndOfInputError{Pos: turtle.Pos{Line: 1, Column: 3}},
		},
		{
			name:        "a plus sign and a point at the end of the input",
			src:         `+.`,
			expectedErr: turtle.UnexpectedEndOfInputError{Pos: turtle.Pos{Line: 1, Column: 3}},
		},
		{
			name:        "a sign and a point followed by something that is not a digit",
			src:         `-.x`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 3}, R: 'x'},
		},
		{
			name:        "a sign and a point followed by an exponent",
			src:         `-.e6`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 3}, R: 'e'},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect(turtle.Tokenize(strings.NewReader(tc.src)))
			if err != tc.expectedErr {
				t.Errorf("Tokenize() error = %v, want %v", err, tc.expectedErr)
			}
		})
	}
}

// TestTokenizeEndsCleanly covers input that stops where a terminal has just
// finished, which is a complete document rather than a truncated one.
func TestTokenizeEndsCleanly(t *testing.T) {
	sources := []string{
		"", " ", "\n", ".", "ex:", "ex:name", "_:b0", "<http://e/a>",
		`"text"`, `'text'`, `"""text"""`, `"text"@en`, "# comment",
		"42", "3.14", "1e6", "a", "PREFIX", "@prefix", "[]", "[", "(", ")",
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			if _, err := collect(turtle.Tokenize(strings.NewReader(src))); err != nil {
				t.Errorf("Tokenize() = %v, want nil", err)
			}
		})
	}
}

func TestErrorMessages(t *testing.T) {
	pos := turtle.Pos{Line: 3, Column: 12}

	testCases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "unexpected character",
			err:  turtle.UnexpectedCharacterError{Pos: pos, R: '$'},
			want: `unexpected character '$' at line 3, column 12`,
		},
		{
			name: "unexpected name",
			err:  turtle.UnexpectedNameError{Pos: pos, Name: "nope"},
			want: `unexpected name "nope" at line 3, column 12`,
		},
		{
			name: "unterminated string",
			err:  turtle.UnterminatedStringError{Pos: pos},
			want: "unterminated string literal at line 3, column 12",
		},
		{
			name: "unterminated iri",
			err:  turtle.UnterminatedIRIRefError{Pos: pos},
			want: "unterminated IRI at line 3, column 12",
		},
		{
			name: "unexpected end of input",
			err:  turtle.UnexpectedEndOfInputError{Pos: pos},
			want: "unexpected end of input at line 3, column 12",
		},
		{
			name: "invalid code point",
			err:  turtle.InvalidCodePointError{Pos: pos, Code: 0xD800},
			want: "escape names no character, U+D800, at line 3, column 12",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTokenAndPosString(t *testing.T) {
	if got, want := (turtle.Pos{Line: 3, Column: 12}).String(), "3:12"; got != want {
		t.Errorf("Pos.String() = %q, want %q", got, want)
	}
	if got, want := tok(1, 1, turtle.TokenPNameLN, "ex:a").String(), "PNameLN(ex:a)"; got != want {
		t.Errorf("Token.String() = %q, want %q", got, want)
	}

	types := map[turtle.TokenType]string{
		turtle.TokenIRIRef: "IRIRef", turtle.TokenPNameNS: "PNameNS",
		turtle.TokenPNameLN: "PNameLN", turtle.TokenBlankNodeLabel: "BlankNodeLabel",
		turtle.TokenAnon: "Anon", turtle.TokenString: "String",
		turtle.TokenLangTag: "LangTag", turtle.TokenInteger: "Integer",
		turtle.TokenDecimal: "Decimal", turtle.TokenDouble: "Double",
		turtle.TokenA: "A", turtle.TokenBoolean: "Boolean",
		turtle.TokenPrefix: "Prefix", turtle.TokenBase: "Base",
		turtle.TokenDatatypeMarker: "DatatypeMarker", turtle.TokenDot: "Dot",
		turtle.TokenSemicolon: "Semicolon", turtle.TokenComma: "Comma",
		turtle.TokenOpenParen: "OpenParen", turtle.TokenCloseParen: "CloseParen",
		turtle.TokenOpenBracket: "OpenBracket", turtle.TokenCloseBracket: "CloseBracket",
		turtle.TokenComment: "Comment",
	}
	for tt, want := range types {
		if got := tt.String(); got != want {
			t.Errorf("TokenType.String() = %q, want %q", got, want)
		}
	}

	t.Run("an unknown type panics rather than lying", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("String() returned normally, want a panic")
			}
		}()
		_ = turtle.TokenType(99).String()
	})
}

// failingReader yields its data and then fails.
type failingReader struct {
	data []byte
	err  error
	read int
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.read >= len(r.data) {
		return 0, r.err
	}
	n := copy(p, r.data[r.read:])
	r.read += n
	return n, nil
}

func TestTokenizeReaderError(t *testing.T) {
	errBoom := errors.New("boom")

	sources := []string{
		"   ", `<http://e/a`, `"hello`, `"""hello`, "# a comment",
		`_:abc`, `"o"@en`, `ex:name`, `42`, `1e`, `[`, `"o"^`,
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			_, err := collect(turtle.Tokenize(&failingReader{data: []byte(src), err: errBoom}))
			if !errors.Is(err, errBoom) {
				t.Errorf("Tokenize() error = %v, want %v", err, errBoom)
			}
		})
	}
}

func TestTokenizeStopsEarly(t *testing.T) {
	var seen int
	for range turtle.Tokenize(strings.NewReader(`<http://e/s> <http://e/p> <http://e/o> .`)) {
		seen++
		break
	}
	if want := 1; seen != want {
		t.Errorf("yielded %d tokens after break, want %d", seen, want)
	}
}
