package trig_test

import (
	"iter"
	"slices"
	"strings"
	"testing"

	"github.com/z5labs/rdf-go/rdf12/trig"
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

// TestTokenizeBraces is the test this package exists for: a '{' opens a graph
// block, a "{|" opens an annotation block, and one character of lookahead is
// what tells them apart wherever they stand.
//
//	wrappedGraph    ::= '{' triplesBlock? '}'
//	annotationBlock ::= '{|' predicateObjectList '|}'
func TestTokenizeBraces(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []trig.Token
	}{
		{
			name: "a bare brace opens a graph block",
			src:  "{}",
			expected: []trig.Token{
				tok(1, 1, trig.TokenOpenBrace, "{"),
				tok(1, 2, trig.TokenCloseBrace, "}"),
			},
		},
		{
			name: "a brace and a bar open an annotation block",
			src:  "{||}",
			expected: []trig.Token{
				tok(1, 1, trig.TokenAnnotationOpen, "{|"),
				tok(1, 3, trig.TokenAnnotationClose, "|}"),
			},
		},
		{
			// The lookahead puts back what it did not use, so the term that
			// follows the brace begins where it would have had nothing looked.
			name: "a graph block whose first terminal follows the brace",
			src:  "{ex:s ex:p ex:o}",
			expected: []trig.Token{
				tok(1, 1, trig.TokenOpenBrace, "{"),
				tok(1, 2, trig.TokenPNameLN, "ex:s"),
				tok(1, 7, trig.TokenPNameLN, "ex:p"),
				tok(1, 12, trig.TokenPNameLN, "ex:o"),
				tok(1, 16, trig.TokenCloseBrace, "}"),
			},
		},
		{
			name: "an annotation immediately followed by a graph block",
			src:  "ex:s ex:p ex:o {| ex:q ex:v |} .\nex:g { }",
			expected: []trig.Token{
				tok(1, 1, trig.TokenPNameLN, "ex:s"),
				tok(1, 6, trig.TokenPNameLN, "ex:p"),
				tok(1, 11, trig.TokenPNameLN, "ex:o"),
				tok(1, 16, trig.TokenAnnotationOpen, "{|"),
				tok(1, 19, trig.TokenPNameLN, "ex:q"),
				tok(1, 24, trig.TokenPNameLN, "ex:v"),
				tok(1, 29, trig.TokenAnnotationClose, "|}"),
				tok(1, 32, trig.TokenDot, "."),
				tok(2, 1, trig.TokenPNameLN, "ex:g"),
				tok(2, 6, trig.TokenOpenBrace, "{"),
				tok(2, 8, trig.TokenCloseBrace, "}"),
			},
		},
		{
			name: "an annotation block inside a graph block",
			src:  "ex:g { ex:s ex:p ex:o {| ex:q ex:v |} }",
			expected: []trig.Token{
				tok(1, 1, trig.TokenPNameLN, "ex:g"),
				tok(1, 6, trig.TokenOpenBrace, "{"),
				tok(1, 8, trig.TokenPNameLN, "ex:s"),
				tok(1, 13, trig.TokenPNameLN, "ex:p"),
				tok(1, 18, trig.TokenPNameLN, "ex:o"),
				tok(1, 23, trig.TokenAnnotationOpen, "{|"),
				tok(1, 26, trig.TokenPNameLN, "ex:q"),
				tok(1, 31, trig.TokenPNameLN, "ex:v"),
				tok(1, 36, trig.TokenAnnotationClose, "|}"),
				tok(1, 39, trig.TokenCloseBrace, "}"),
			},
		},
		{
			// The bar of an annotation's opener is not the bar of its closer:
			// "{|}" is the opener and a stray brace, which the parser refuses.
			name: "a brace, a bar and a brace",
			src:  "{|}",
			expected: []trig.Token{
				tok(1, 1, trig.TokenAnnotationOpen, "{|"),
				tok(1, 3, trig.TokenCloseBrace, "}"),
			},
		},
		{
			name: "a graph block holding an annotated triple and closing at once",
			src:  "{ex:s ex:p ex:o{|ex:q ex:v|}}",
			expected: []trig.Token{
				tok(1, 1, trig.TokenOpenBrace, "{"),
				tok(1, 2, trig.TokenPNameLN, "ex:s"),
				tok(1, 7, trig.TokenPNameLN, "ex:p"),
				tok(1, 12, trig.TokenPNameLN, "ex:o"),
				tok(1, 16, trig.TokenAnnotationOpen, "{|"),
				tok(1, 18, trig.TokenPNameLN, "ex:q"),
				tok(1, 23, trig.TokenPNameLN, "ex:v"),
				tok(1, 27, trig.TokenAnnotationClose, "|}"),
				tok(1, 29, trig.TokenCloseBrace, "}"),
			},
		},
		{
			name: "a brace at the end of the input",
			src:  "{",
			expected: []trig.Token{
				tok(1, 1, trig.TokenOpenBrace, "{"),
			},
		},
		{
			name: "a brace and a comment",
			src:  "{ # inside\n}",
			expected: []trig.Token{
				tok(1, 1, trig.TokenOpenBrace, "{"),
				tok(1, 3, trig.TokenComment, "# inside"),
				tok(2, 1, trig.TokenCloseBrace, "}"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
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
		{
			name: "a blank node label ending at a brace",
			src:  "_:g{}",
			expected: []trig.Token{
				tok(1, 1, trig.TokenBlankNodeLabel, "g"),
				tok(1, 4, trig.TokenOpenBrace, "{"),
				tok(1, 5, trig.TokenCloseBrace, "}"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
	}
}

// TestTokenizeRDF12Terminals covers the lexemes RDF 1.2 adds, read where TriG
// puts them: inside a graph block, where nothing about them changes.
func TestTokenizeRDF12Terminals(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []trig.Token
	}{
		{
			name: "a triple term inside a block",
			src:  "{ ex:s ex:p <<( ex:a ex:b ex:c )>> }",
			expected: []trig.Token{
				tok(1, 1, trig.TokenOpenBrace, "{"),
				tok(1, 3, trig.TokenPNameLN, "ex:s"),
				tok(1, 8, trig.TokenPNameLN, "ex:p"),
				tok(1, 13, trig.TokenTripleTermOpen, "<<("),
				tok(1, 17, trig.TokenPNameLN, "ex:a"),
				tok(1, 22, trig.TokenPNameLN, "ex:b"),
				tok(1, 27, trig.TokenPNameLN, "ex:c"),
				tok(1, 32, trig.TokenTripleTermClose, ")>>"),
				tok(1, 36, trig.TokenCloseBrace, "}"),
			},
		},
		{
			name: "a reified triple with a reifier inside a block",
			src:  "{ << ex:a ex:b ex:c ~ ex:r >> ex:p ex:o }",
			expected: []trig.Token{
				tok(1, 1, trig.TokenOpenBrace, "{"),
				tok(1, 3, trig.TokenReifiedTripleOpen, "<<"),
				tok(1, 6, trig.TokenPNameLN, "ex:a"),
				tok(1, 11, trig.TokenPNameLN, "ex:b"),
				tok(1, 16, trig.TokenPNameLN, "ex:c"),
				tok(1, 21, trig.TokenReifier, "~"),
				tok(1, 23, trig.TokenPNameLN, "ex:r"),
				tok(1, 28, trig.TokenReifiedTripleClose, ">>"),
				tok(1, 31, trig.TokenPNameLN, "ex:p"),
				tok(1, 36, trig.TokenPNameLN, "ex:o"),
				tok(1, 41, trig.TokenCloseBrace, "}"),
			},
		},
		{
			name: "the version directive in both forms",
			src:  "@version \"1.2\" . VERSION '1.2'",
			expected: []trig.Token{
				tok(1, 1, trig.TokenVersion, "@version"),
				tok(1, 10, trig.TokenString, "1.2"),
				tok(1, 16, trig.TokenDot, "."),
				tok(1, 18, trig.TokenVersion, "VERSION"),
				tok(1, 26, trig.TokenString, "1.2"),
			},
		},
		{
			name: "a directional literal inside a block",
			src:  `{ ex:s ex:p "v"@en--ltr }`,
			expected: []trig.Token{
				tok(1, 1, trig.TokenOpenBrace, "{"),
				tok(1, 3, trig.TokenPNameLN, "ex:s"),
				tok(1, 8, trig.TokenPNameLN, "ex:p"),
				tok(1, 13, trig.TokenString, "v"),
				tok(1, 16, trig.TokenLangDir, "en--ltr"),
				tok(1, 25, trig.TokenCloseBrace, "}"),
			},
		},
		{
			name: "a language tag with no direction inside a block",
			src:  `{ ex:s ex:p "v"@en-GB }`,
			expected: []trig.Token{
				tok(1, 1, trig.TokenOpenBrace, "{"),
				tok(1, 3, trig.TokenPNameLN, "ex:s"),
				tok(1, 8, trig.TokenPNameLN, "ex:p"),
				tok(1, 13, trig.TokenString, "v"),
				tok(1, 16, trig.TokenLangDir, "en-GB"),
				tok(1, 23, trig.TokenCloseBrace, "}"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
	}
}

// TestTokenizeBracesInIRIs covers the one place a brace is not a terminal:
// inside an IRI, where the grammar excludes it outright.
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

// The rest of this file covers the terminals TriG takes from RDF 1.2 Turtle
// unchanged. They are exercised again here because the two tokenizers are still
// separate code — internal/lex holds the characters they read, not the rules
// that read them — and a fix to one that missed the other would show up nowhere
// else.

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
			name:     "a long double quoted string",
			src:      "\"\"\"a\nb\"\"\"",
			expected: []trig.Token{tok(1, 1, trig.TokenString, "a\nb")},
		},
		{
			name:     "every ECHAR",
			src:      `"\t\b\n\r\f\"\'\\"`,
			expected: []trig.Token{tok(1, 1, trig.TokenString, "\t\b\n\r\f\"'\\")},
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
			name:     "a blank node label with non ascii letters",
			src:      "_:日本語",
			expected: []trig.Token{tok(1, 1, trig.TokenBlankNodeLabel, "日本語")},
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
		name string
		src  string
		want error
	}{
		{
			name: "a character that begins no terminal",
			src:  "$",
			want: trig.UnexpectedCharacterError{Pos: trig.Pos{Line: 1, Column: 1}, R: '$'},
		},
		{
			// A bar begins nothing but the "|}" that closes an annotation, so
			// the brace after it is required rather than looked ahead at.
			name: "a bar with no brace after it",
			src:  "|x",
			want: trig.UnexpectedCharacterError{Pos: trig.Pos{Line: 1, Column: 2}, R: 'x'},
		},
		{
			name: "a name that is no keyword",
			src:  "notakeyword",
			want: trig.UnexpectedNameError{Pos: trig.Pos{Line: 1, Column: 1}, Name: "notakeyword"},
		},
		{
			name: "an unterminated string",
			src:  `"v`,
			want: trig.UnterminatedStringError{Pos: trig.Pos{Line: 1, Column: 1}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect(trig.Tokenize(strings.NewReader(tc.src)))
			if err != tc.want {
				t.Errorf("Tokenize() error = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestTokenTypeString covers the name every terminal prints as, which is what
// an UnexpectedTokenError names, and that no type is left without one.
func TestTokenTypeString(t *testing.T) {
	want := []string{
		"IRIRef", "PNameNS", "PNameLN", "BlankNodeLabel", "Anon", "String",
		"LangDir", "Integer", "Decimal", "Double", "A", "Boolean", "Prefix",
		"Base", "Version", "Graph", "OpenBrace", "CloseBrace", "DatatypeMarker",
		"TripleTermOpen", "TripleTermClose", "ReifiedTripleOpen",
		"ReifiedTripleClose", "Reifier", "AnnotationOpen", "AnnotationClose",
		"Dot", "Semicolon", "Comma", "OpenParen", "CloseParen", "OpenBracket",
		"CloseBracket", "Comment",
	}

	for i, name := range want {
		t.Run(name, func(t *testing.T) {
			if got := trig.TokenType(i).String(); got != name {
				t.Errorf("TokenType(%d).String() = %q, want %q", i, got, name)
			}
		})
	}

	// One past the last, which is a programming error rather than a name.
	t.Run("a type the grammar has none of", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("String() returned, want a panic")
			}
		}()
		_ = trig.TokenType(len(want)).String()
	})
}

// TestTokenizerErrorMessages covers what each tokenizer error says, since a
// position and a character are the whole of what a reader has to go on.
func TestTokenizerErrorMessages(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "unexpected character",
			err:  trig.UnexpectedCharacterError{Pos: trig.Pos{Line: 2, Column: 3}, R: '$'},
			want: "unexpected character '$' at line 2, column 3",
		},
		{
			name: "unterminated string",
			err:  trig.UnterminatedStringError{Pos: trig.Pos{Line: 2, Column: 3}},
			want: "unterminated string literal at line 2, column 3",
		},
		{
			name: "unterminated iri",
			err:  trig.UnterminatedIRIRefError{Pos: trig.Pos{Line: 2, Column: 3}},
			want: "unterminated IRI at line 2, column 3",
		},
		{
			name: "unexpected end of input",
			err:  trig.UnexpectedEndOfInputError{Pos: trig.Pos{Line: 2, Column: 3}},
			want: "unexpected end of input at line 2, column 3",
		},
		{
			name: "invalid code point",
			err:  trig.InvalidCodePointError{Pos: trig.Pos{Line: 2, Column: 3}, Code: 0xD800},
			want: "escape names no character, U+D800, at line 2, column 3",
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

// TestTokenizeRejectsAnEscapedIRICharacter covers what the W3C tests
// turtle-syntax-bad-uri-escape-01..03 and their TriG twins settle: a UCHAR
// spells a character rather than exempting one, so the characters IRIREF
// leaves out are left out however they are written.
//
//	IRIREF ::= '<' ([^#x00-#x20<>"{}|^`\] | UCHAR)* '>'
//
// The position is the backslash rather than the character it named, that being
// what has to be rewritten — a space in an IRI is written %20.
func TestTokenizeRejectsAnEscapedIRICharacter(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		r    rune
	}{
		{name: "a space", src: `<http://e/a\u0020b>`, r: ' '},
		{name: "a less-than sign", src: `<http://e/a\u003Cb>`, r: '<'},
		{name: "a greater-than sign", src: `<http://e/a\u003Eb>`, r: '>'},
		{name: "a backslash", src: `<http://e/a\u005Cb>`, r: '\\'},
		{name: "a vertical line", src: `<http://e/a\u007Cb>`, r: '|'},
		{name: "the null character", src: `<http://e/a\u0000b>`, r: 0},
		{name: "a tab written with the eight digit form", src: `<http://e/a\U00000009b>`, r: '\t'},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect(trig.Tokenize(strings.NewReader(tc.src)))

			// Column 12 is the backslash: the '<' is column 1 and
			// "http://e/a" is the ten characters after it.
			want := trig.EscapedIRICharacterError{
				Pos: trig.Pos{Line: 1, Column: 12},
				R:   tc.r,
			}
			if err != want {
				t.Errorf("Tokenize() error = %v, want %v", err, want)
			}
		})
	}
}

// TestTokenizeAcceptsAnEscapedCharacterAnIRIMayHold keeps the guard from
// growing into a ban on UCHARs: an escape naming a character the production
// admits is still decoded, and the token carries the character rather than the
// escape.
func TestTokenizeAcceptsAnEscapedCharacterAnIRIMayHold(t *testing.T) {
	tokens, err := collect(trig.Tokenize(strings.NewReader(`<http://e/a\u0062c>`)))
	if err != nil {
		t.Fatalf("Tokenize() error = %v, want nil", err)
	}
	if len(tokens) != 1 || string(tokens[0].Value) != "http://e/abc" {
		t.Errorf("Tokenize() = %v, want the iri http://e/abc", tokens)
	}
}
