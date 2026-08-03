package trig_test

import (
	"iter"
	"slices"
	"strings"
	"testing"

	"github.com/z5labs/rdf-go/rdf11/trig"
)

func collect(seq iter.Seq2[trig.Token, error]) ([]trig.Token, error) {
	var tokens []trig.Token
	for token, err := range seq {
		if err != nil {
			return tokens, err
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

func tok(line, column int, tt trig.TokenType, value string) trig.Token {
	return trig.Token{
		Pos:   trig.Pos{Line: line, Column: column},
		Type:  tt,
		Value: []byte(value),
	}
}

func equalTokens(got, want []trig.Token) bool {
	return slices.EqualFunc(got, want, func(a, b trig.Token) bool {
		return a.Pos == b.Pos && a.Type == b.Type && string(a.Value) == string(b.Value)
	})
}

func run(t *testing.T, src string, want []trig.Token) {
	t.Helper()

	got, err := collect(trig.Tokenize(strings.NewReader(src)))
	if err != nil {
		t.Fatalf("Tokenize() = %v, want nil", err)
	}
	if !equalTokens(got, want) {
		t.Errorf("Tokenize() = %v, want %v", got, want)
	}
}

// TestTokenizeGraphTerminals covers the three terminals TriG adds to Turtle,
// which are the whole of the difference between the two tokenizers.
func TestTokenizeGraphTerminals(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []trig.Token
	}{
		{
			name: "an empty block",
			src:  "{}",
			expected: []trig.Token{
				tok(1, 1, trig.TokenOpenBrace, "{"),
				tok(1, 2, trig.TokenCloseBrace, "}"),
			},
		},
		{
			name: "a labelled block",
			src:  "<http://e/g> { }",
			expected: []trig.Token{
				tok(1, 1, trig.TokenIRIRef, "http://e/g"),
				tok(1, 14, trig.TokenOpenBrace, "{"),
				tok(1, 16, trig.TokenCloseBrace, "}"),
			},
		},
		{
			// The grammar writes the keyword in double quotes, which makes it
			// case-insensitive, and the value keeps the case it was written in.
			name:     "the keyword in upper case",
			src:      "GRAPH",
			expected: []trig.Token{tok(1, 1, trig.TokenGraph, "GRAPH")},
		},
		{
			name:     "the keyword in lower case",
			src:      "graph",
			expected: []trig.Token{tok(1, 1, trig.TokenGraph, "graph")},
		},
		{
			name:     "the keyword in mixed case",
			src:      "GrApH",
			expected: []trig.Token{tok(1, 1, trig.TokenGraph, "GrApH")},
		},
		{
			// A name followed by a colon never reaches the keyword rule, so the
			// keyword does not steal a prefix that happens to be spelled alike.
			name:     "a prefixed name on the graph prefix",
			src:      "graph:name",
			expected: []trig.Token{tok(1, 1, trig.TokenPNameLN, "graph:name")},
		},
		{
			name:     "the graph prefix with no local name",
			src:      "graph:",
			expected: []trig.Token{tok(1, 1, trig.TokenPNameNS, "graph")},
		},
		{
			name: "the keyword followed by the dot that ends a statement",
			src:  "graph.",
			expected: []trig.Token{
				tok(1, 1, trig.TokenGraph, "graph"),
				tok(1, 6, trig.TokenDot, "."),
			},
		},
		{
			name: "a whole block",
			src:  "GRAPH ex:g { ex:s ex:p ex:o . }",
			expected: []trig.Token{
				tok(1, 1, trig.TokenGraph, "GRAPH"),
				tok(1, 7, trig.TokenPNameLN, "ex:g"),
				tok(1, 12, trig.TokenOpenBrace, "{"),
				tok(1, 14, trig.TokenPNameLN, "ex:s"),
				tok(1, 19, trig.TokenPNameLN, "ex:p"),
				tok(1, 24, trig.TokenPNameLN, "ex:o"),
				tok(1, 29, trig.TokenDot, "."),
				tok(1, 31, trig.TokenCloseBrace, "}"),
			},
		},
		{
			name: "braces need no white space around them",
			src:  "ex:g{ex:s ex:p ex:o}",
			expected: []trig.Token{
				tok(1, 1, trig.TokenPNameLN, "ex:g"),
				tok(1, 5, trig.TokenOpenBrace, "{"),
				tok(1, 6, trig.TokenPNameLN, "ex:s"),
				tok(1, 11, trig.TokenPNameLN, "ex:p"),
				tok(1, 16, trig.TokenPNameLN, "ex:o"),
				tok(1, 20, trig.TokenCloseBrace, "}"),
			},
		},
		{
			name: "a block spread over lines",
			src:  "ex:g {\n\tex:s ex:p ex:o .\n}",
			expected: []trig.Token{
				tok(1, 1, trig.TokenPNameLN, "ex:g"),
				tok(1, 6, trig.TokenOpenBrace, "{"),
				tok(2, 2, trig.TokenPNameLN, "ex:s"),
				tok(2, 7, trig.TokenPNameLN, "ex:p"),
				tok(2, 12, trig.TokenPNameLN, "ex:o"),
				tok(2, 17, trig.TokenDot, "."),
				tok(3, 1, trig.TokenCloseBrace, "}"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
	}
}

// TestTokenizeBracesAreNotIRICharacters covers the one place a brace is not a
// terminal: inside an IRI, where the grammar excludes it outright.
//
//	IRIREF ::= '<' ([^#x00-#x20<>"{}|^`\] | UCHAR)* '>'
func TestTokenizeBracesInIRIs(t *testing.T) {
	for _, src := range []string{"<http://e/{>", "<http://e/}>"} {
		t.Run(src, func(t *testing.T) {
			_, err := collect(trig.Tokenize(strings.NewReader(src)))
			want := trig.UnexpectedCharacterError{Pos: trig.Pos{Line: 1, Column: 11}, R: rune(src[10])}
			if err != want {
				t.Errorf("Tokenize() error = %v, want %v", err, want)
			}
		})
	}
}

// The rest of this file covers the terminals TriG takes from Turtle unchanged.
// They are exercised again here because the two tokenizers are still separate
// code — internal/lex holds the characters they read, not the rules that read
// them — and a fix to one that missed the other would show up nowhere else.

func TestTokenizeTurtleTerminals(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []trig.Token
	}{
		{
			name:     "empty input",
			src:      "",
			expected: nil,
		},
		{
			name:     "an iri",
			src:      "<http://e/s>",
			expected: []trig.Token{tok(1, 1, trig.TokenIRIRef, "http://e/s")},
		},
		{
			name:     "a UCHAR inside an iri",
			src:      "<http://e/\\u0061>",
			expected: []trig.Token{tok(1, 1, trig.TokenIRIRef, "http://e/a")},
		},
		{
			name:     "a blank node label",
			src:      "_:b0",
			expected: []trig.Token{tok(1, 1, trig.TokenBlankNodeLabel, "b0")},
		},
		{
			name:     "an anonymous blank node",
			src:      "[  ]",
			expected: []trig.Token{tok(1, 1, trig.TokenAnon, "[]")},
		},
		{
			name: "a blank node property list",
			src:  "[ a ]",
			expected: []trig.Token{
				tok(1, 1, trig.TokenOpenBracket, "["),
				tok(1, 3, trig.TokenA, "a"),
				tok(1, 5, trig.TokenCloseBracket, "]"),
			},
		},
		{
			name: "a collection",
			src:  "( )",
			expected: []trig.Token{
				tok(1, 1, trig.TokenOpenParen, "("),
				tok(1, 3, trig.TokenCloseParen, ")"),
			},
		},
		{
			name:     "a double quoted string",
			src:      `"text"`,
			expected: []trig.Token{tok(1, 1, trig.TokenString, "text")},
		},
		{
			name:     "a single quoted string",
			src:      `'text'`,
			expected: []trig.Token{tok(1, 1, trig.TokenString, "text")},
		},
		{
			name:     "a long double quoted string",
			src:      "\"\"\"a\nb\"\"\"",
			expected: []trig.Token{tok(1, 1, trig.TokenString, "a\nb")},
		},
		{
			name:     "a long single quoted string",
			src:      "'''a''b'''",
			expected: []trig.Token{tok(1, 1, trig.TokenString, "a''b")},
		},
		{
			name:     "every ECHAR",
			src:      `"\t\b\n\r\f\"\'\\"`,
			expected: []trig.Token{tok(1, 1, trig.TokenString, "\t\b\n\r\f\"'\\")},
		},
		{
			name:     "an eight digit UCHAR",
			src:      `"\U0001F600"`,
			expected: []trig.Token{tok(1, 1, trig.TokenString, "\U0001F600")},
		},
		{
			name: "a language tag",
			src:  `"v"@en-GB`,
			expected: []trig.Token{
				tok(1, 1, trig.TokenString, "v"),
				tok(1, 4, trig.TokenLangTag, "en-GB"),
			},
		},
		{
			name: "a datatype",
			src:  `"v"^^<http://e/dt>`,
			expected: []trig.Token{
				tok(1, 1, trig.TokenString, "v"),
				tok(1, 4, trig.TokenDatatypeMarker, "^^"),
				tok(1, 6, trig.TokenIRIRef, "http://e/dt"),
			},
		},
		{
			name: "the numeric literals",
			src:  "42 -0.5 .5 1e6 1.0E-6 +7",
			expected: []trig.Token{
				tok(1, 1, trig.TokenInteger, "42"),
				tok(1, 4, trig.TokenDecimal, "-0.5"),
				tok(1, 9, trig.TokenDecimal, ".5"),
				tok(1, 12, trig.TokenDouble, "1e6"),
				tok(1, 16, trig.TokenDouble, "1.0E-6"),
				tok(1, 23, trig.TokenInteger, "+7"),
			},
		},
		{
			name: "an integer and the dot that ends a statement",
			src:  "1.",
			expected: []trig.Token{
				tok(1, 1, trig.TokenInteger, "1"),
				tok(1, 2, trig.TokenDot, "."),
			},
		},
		{
			// A number holds one decimal point, so the second ends this one
			// and begins the next, exactly as it does in ".5.5".
			name: "a second decimal point begins a second number",
			src:  "1.2.3",
			expected: []trig.Token{
				tok(1, 1, trig.TokenDecimal, "1.2"),
				tok(1, 4, trig.TokenDecimal, ".3"),
			},
		},
		{
			name: "a second decimal point where the first began the number",
			src:  ".5.5",
			expected: []trig.Token{
				tok(1, 1, trig.TokenDecimal, ".5"),
				tok(1, 3, trig.TokenDecimal, ".5"),
			},
		},
		{
			name: "a decimal and the dot that ends a statement",
			src:  "1.2.",
			expected: []trig.Token{
				tok(1, 1, trig.TokenDecimal, "1.2"),
				tok(1, 4, trig.TokenDot, "."),
			},
		},
		{
			name: "the booleans are case sensitive",
			src:  "true false",
			expected: []trig.Token{
				tok(1, 1, trig.TokenBoolean, "true"),
				tok(1, 6, trig.TokenBoolean, "false"),
			},
		},
		{
			name: "the directives in both forms",
			src:  "@prefix @base PREFIX base",
			expected: []trig.Token{
				tok(1, 1, trig.TokenPrefix, "@prefix"),
				tok(1, 9, trig.TokenBase, "@base"),
				tok(1, 15, trig.TokenPrefix, "PREFIX"),
				tok(1, 22, trig.TokenBase, "base"),
			},
		},
		{
			name: "the punctuation",
			src:  "; ,",
			expected: []trig.Token{
				tok(1, 1, trig.TokenSemicolon, ";"),
				tok(1, 3, trig.TokenComma, ","),
			},
		},
		{
			name:     "a comment",
			src:      "# a comment",
			expected: []trig.Token{tok(1, 1, trig.TokenComment, "# a comment")},
		},
		{
			name:     "a local name with escapes",
			src:      `ex:a\-b%20c`,
			expected: []trig.Token{tok(1, 1, trig.TokenPNameLN, "ex:a-b%20c")},
		},
		{
			name: "a prefixed name ending at the statement's dot",
			src:  "ex:a.",
			expected: []trig.Token{
				tok(1, 1, trig.TokenPNameLN, "ex:a"),
				tok(1, 5, trig.TokenDot, "."),
			},
		},
		{
			name:     "a blank node label with non ascii letters",
			src:      "_:日本語",
			expected: []trig.Token{tok(1, 1, trig.TokenBlankNodeLabel, "日本語")},
		},
		{
			name:     "a blank node label holding a dot",
			src:      "_:a.b",
			expected: []trig.Token{tok(1, 1, trig.TokenBlankNodeLabel, "a.b")},
		},
		{
			name: "a blank node label ending at the statement's dot",
			src:  "_:b0.",
			expected: []trig.Token{
				tok(1, 1, trig.TokenBlankNodeLabel, "b0"),
				tok(1, 5, trig.TokenDot, "."),
			},
		},
		{
			// A colon ends a blank node label, which may not hold one, and
			// begins a prefixed name on the empty prefix.
			name: "a colon after a blank node label begins a prefixed name",
			src:  "_:a:b",
			expected: []trig.Token{
				tok(1, 1, trig.TokenBlankNodeLabel, "a"),
				tok(1, 4, trig.TokenPNameLN, ":b"),
			},
		},
		{
			name: "a blank node label ending at a brace",
			src:  "_:g{}",
			expected: []trig.Token{
				tok(1, 1, trig.TokenBlankNodeLabel, "g"),
				tok(1, 4, trig.TokenOpenBrace, "{"),
				tok(1, 5, trig.TokenCloseBrace, "}"),
			},
		},
		{
			name:     "an empty short string written as two quotes",
			src:      `""`,
			expected: []trig.Token{tok(1, 1, trig.TokenString, "")},
		},
		{
			name:     "a long string holding two quotes in a row",
			src:      `"""a""b"""`,
			expected: []trig.Token{tok(1, 1, trig.TokenString, `a""b`)},
		},
		{
			name:     "an escape inside a long string",
			src:      `"""a\nb"""`,
			expected: []trig.Token{tok(1, 1, trig.TokenString, "a\nb")},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
	}
}

func TestTokenizeErrors(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name:        "a character no terminal begins with",
			src:         "$",
			expectedErr: trig.UnexpectedCharacterError{Pos: trig.Pos{Line: 1, Column: 1}, R: '$'},
		},
		{
			name:        "a bare name that is no keyword",
			src:         "nope",
			expectedErr: trig.UnexpectedNameError{Pos: trig.Pos{Line: 1, Column: 1}, Name: "nope"},
		},
		{
			name:        "an unterminated iri",
			src:         "<http://e/a",
			expectedErr: trig.UnterminatedIRIRefError{Pos: trig.Pos{Line: 1, Column: 1}},
		},
		{
			name:        "an unterminated string",
			src:         `"text`,
			expectedErr: trig.UnterminatedStringError{Pos: trig.Pos{Line: 1, Column: 1}},
		},
		{
			name:        "a short string may not span lines",
			src:         "\"a\nb\"",
			expectedErr: trig.UnterminatedStringError{Pos: trig.Pos{Line: 1, Column: 1}},
		},
		{
			name:        "an unknown escape in a string",
			src:         `"a\qb"`,
			expectedErr: trig.UnexpectedCharacterError{Pos: trig.Pos{Line: 1, Column: 4}, R: 'q'},
		},
		{
			name:        "a UCHAR naming a surrogate",
			src:         `"\uD800"`,
			expectedErr: trig.InvalidCodePointError{Pos: trig.Pos{Line: 1, Column: 2}, Code: 0xD800},
		},
		{
			name:        "a UCHAR above the highest code point",
			src:         `"\U00110000"`,
			expectedErr: trig.InvalidCodePointError{Pos: trig.Pos{Line: 1, Column: 2}, Code: 0x110000},
		},
		{
			name:        "a blank node label cut off after its colon",
			src:         "_:",
			expectedErr: trig.UnexpectedEndOfInputError{Pos: trig.Pos{Line: 1, Column: 3}},
		},
		{
			name:        "an underscore not followed by a colon",
			src:         "_x",
			expectedErr: trig.UnexpectedCharacterError{Pos: trig.Pos{Line: 1, Column: 2}, R: 'x'},
		},
		{
			name:        "a single caret",
			src:         `"o"^x`,
			expectedErr: trig.UnexpectedCharacterError{Pos: trig.Pos{Line: 1, Column: 5}, R: 'x'},
		},
		{
			name:        "an exponent with no digits",
			src:         "1e",
			expectedErr: trig.UnexpectedEndOfInputError{Pos: trig.Pos{Line: 1, Column: 3}},
		},
		{
			// Every numeric production requires a digit, so a sign and a
			// decimal point on their own are not a number, however the rest of
			// the input goes on.
			name:        "a minus sign and a point at the end of the input",
			src:         "-.",
			expectedErr: trig.UnexpectedEndOfInputError{Pos: trig.Pos{Line: 1, Column: 3}},
		},
		{
			name:        "a plus sign and a point at the end of the input",
			src:         "+.",
			expectedErr: trig.UnexpectedEndOfInputError{Pos: trig.Pos{Line: 1, Column: 3}},
		},
		{
			name:        "a sign and a point followed by something that is not a digit",
			src:         "-.x",
			expectedErr: trig.UnexpectedCharacterError{Pos: trig.Pos{Line: 1, Column: 3}, R: 'x'},
		},
		{
			name:        "a sign and a point followed by an exponent",
			src:         "-.e6",
			expectedErr: trig.UnexpectedCharacterError{Pos: trig.Pos{Line: 1, Column: 3}, R: 'e'},
		},
		{
			name:        "a prefix ending in a dot",
			src:         "pre.:x",
			expectedErr: trig.UnexpectedCharacterError{Pos: trig.Pos{Line: 1, Column: 4}, R: '.'},
		},
		{
			// RDF 1.2 arrives here: the second '<' is not an IRI character.
			name:        "a triple term bracket",
			src:         "<<( <http://e/s> <http://e/p> <http://e/o> )>>",
			expectedErr: trig.UnexpectedCharacterError{Pos: trig.Pos{Line: 1, Column: 2}, R: '<'},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect(trig.Tokenize(strings.NewReader(tc.src)))
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
		"", " ", "\n", ".", "{", "}", "{}", "GRAPH", "graph", "ex:", "ex:name",
		"_:b0", "<http://e/a>", `"text"`, `'text'`, `"""text"""`, `"text"@en`,
		"# comment", "42", "3.14", "1e6", "a", "PREFIX", "@prefix", "[]", "[",
		"(", ")",
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			if _, err := collect(trig.Tokenize(strings.NewReader(src))); err != nil {
				t.Errorf("Tokenize() = %v, want nil", err)
			}
		})
	}
}

func TestTokenizeStopsEarly(t *testing.T) {
	var seen int
	for range trig.Tokenize(strings.NewReader("<http://e/g> { <http://e/s> <http://e/p> <http://e/o> }")) {
		seen++
		break
	}
	if want := 1; seen != want {
		t.Errorf("yielded %d tokens after break, want %d", seen, want)
	}
}

func TestTokenAndPosString(t *testing.T) {
	if got, want := (trig.Pos{Line: 3, Column: 12}).String(), "3:12"; got != want {
		t.Errorf("Pos.String() = %q, want %q", got, want)
	}
	if got, want := tok(1, 1, trig.TokenPNameLN, "ex:a").String(), "PNameLN(ex:a)"; got != want {
		t.Errorf("Token.String() = %q, want %q", got, want)
	}

	types := map[trig.TokenType]string{
		trig.TokenIRIRef: "IRIRef", trig.TokenPNameNS: "PNameNS",
		trig.TokenPNameLN: "PNameLN", trig.TokenBlankNodeLabel: "BlankNodeLabel",
		trig.TokenAnon: "Anon", trig.TokenString: "String",
		trig.TokenLangTag: "LangTag", trig.TokenInteger: "Integer",
		trig.TokenDecimal: "Decimal", trig.TokenDouble: "Double",
		trig.TokenA: "A", trig.TokenBoolean: "Boolean",
		trig.TokenPrefix: "Prefix", trig.TokenBase: "Base",
		trig.TokenGraph: "Graph", trig.TokenOpenBrace: "OpenBrace",
		trig.TokenCloseBrace:     "CloseBrace",
		trig.TokenDatatypeMarker: "DatatypeMarker", trig.TokenDot: "Dot",
		trig.TokenSemicolon: "Semicolon", trig.TokenComma: "Comma",
		trig.TokenOpenParen: "OpenParen", trig.TokenCloseParen: "CloseParen",
		trig.TokenOpenBracket: "OpenBracket", trig.TokenCloseBracket: "CloseBracket",
		trig.TokenComment: "Comment",
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
		_ = trig.TokenType(99).String()
	})
}

func TestErrorMessages(t *testing.T) {
	pos := trig.Pos{Line: 3, Column: 12}

	testCases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "unexpected character",
			err:  trig.UnexpectedCharacterError{Pos: pos, R: '$'},
			want: `unexpected character '$' at line 3, column 12`,
		},
		{
			name: "unexpected name",
			err:  trig.UnexpectedNameError{Pos: pos, Name: "nope"},
			want: `unexpected name "nope" at line 3, column 12`,
		},
		{
			name: "unterminated string",
			err:  trig.UnterminatedStringError{Pos: pos},
			want: "unterminated string literal at line 3, column 12",
		},
		{
			name: "unterminated iri",
			err:  trig.UnterminatedIRIRefError{Pos: pos},
			want: "unterminated IRI at line 3, column 12",
		},
		{
			name: "unexpected end of input",
			err:  trig.UnexpectedEndOfInputError{Pos: pos},
			want: "unexpected end of input at line 3, column 12",
		},
		{
			name: "invalid code point",
			err:  trig.InvalidCodePointError{Pos: pos, Code: 0xD800},
			want: "escape names no character, U+D800, at line 3, column 12",
		},
		{
			name: "unexpected token",
			err: trig.UnexpectedTokenError{
				Expected: []trig.TokenType{trig.TokenDot, trig.TokenCloseBrace},
				Actual:   tok(3, 12, trig.TokenOpenBrace, "{"),
			},
			want: "unexpected token at line 3, column 12: OpenBrace({), expected one of: Dot, CloseBrace",
		},
		{
			name: "unexpected end of tokens",
			err: trig.UnexpectedEndOfTokensError{
				Expected: []trig.TokenType{trig.TokenCloseBrace},
				Pos:      pos,
			},
			want: "unexpected end of tokens at line 3, column 12, expected one of: CloseBrace",
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
