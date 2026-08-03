package nquads_test

import (
	"errors"
	"iter"
	"slices"
	"strings"
	"testing"

	nquads11 "github.com/z5labs/rdf-go/rdf11/nquads"
	"github.com/z5labs/rdf-go/rdf12/nquads"
)

// collect drains a token sequence, returning the tokens yielded before any
// error and the error that stopped it.
func collect(seq iter.Seq2[nquads.Token, error]) ([]nquads.Token, error) {
	var tokens []nquads.Token
	for token, err := range seq {
		if err != nil {
			return tokens, err
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

// tok builds an expected token, keeping the tables readable.
func tok(line, column int, tt nquads.TokenType, value string) nquads.Token {
	return nquads.Token{
		Pos:   nquads.Pos{Line: line, Column: column},
		Type:  tt,
		Value: []byte(value),
	}
}

func equalTokens(got, want []nquads.Token) bool {
	return slices.EqualFunc(got, want, func(a, b nquads.Token) bool {
		return a.Pos == b.Pos && a.Type == b.Type && string(a.Value) == string(b.Value)
	})
}

// TestTokenizeTripleTerms covers the brackets RDF 1.2 adds around a triple
// standing as the object of another, and the lookahead that tells "<<(" from
// an IRIREF.
func TestTokenizeTripleTerms(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []nquads.Token
	}{
		{
			name: "a triple term as an object",
			src:  `<http://example.com/s> <http://example.com/p> <<( <http://example.com/a> <http://example.com/b> _:c )>> .`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenIRIRef, "http://example.com/s"),
				tok(1, 24, nquads.TokenIRIRef, "http://example.com/p"),
				tok(1, 47, nquads.TokenTripleTermOpen, "<<("),
				tok(1, 51, nquads.TokenIRIRef, "http://example.com/a"),
				tok(1, 74, nquads.TokenIRIRef, "http://example.com/b"),
				tok(1, 97, nquads.TokenBlankNodeLabel, "c"),
				tok(1, 101, nquads.TokenTripleTermClose, ")>>"),
				tok(1, 105, nquads.TokenDot, "."),
			},
		},
		{
			name: "the brackets alone",
			src:  `<<( )>>`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenTripleTermOpen, "<<("),
				tok(1, 5, nquads.TokenTripleTermClose, ")>>"),
			},
		},
		{
			name: "brackets need no white space around them",
			src:  `<<(_:a <http://example.com/p> "v")>>`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenTripleTermOpen, "<<("),
				tok(1, 4, nquads.TokenBlankNodeLabel, "a"),
				tok(1, 8, nquads.TokenIRIRef, "http://example.com/p"),
				tok(1, 31, nquads.TokenStringLiteral, "v"),
				tok(1, 34, nquads.TokenTripleTermClose, ")>>"),
			},
		},
		{
			name: "nested triple terms",
			src:  `<<( _:a <http://example.com/p> <<( _:b <http://example.com/q> "v" )>> )>>`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenTripleTermOpen, "<<("),
				tok(1, 5, nquads.TokenBlankNodeLabel, "a"),
				tok(1, 9, nquads.TokenIRIRef, "http://example.com/p"),
				tok(1, 32, nquads.TokenTripleTermOpen, "<<("),
				tok(1, 36, nquads.TokenBlankNodeLabel, "b"),
				tok(1, 40, nquads.TokenIRIRef, "http://example.com/q"),
				tok(1, 63, nquads.TokenStringLiteral, "v"),
				tok(1, 67, nquads.TokenTripleTermClose, ")>>"),
				tok(1, 71, nquads.TokenTripleTermClose, ")>>"),
			},
		},
		{
			// The lookahead that fails: the character after the '<' is put
			// back and read as the first of the IRI.
			name: "an iri beginning with a character the opener would have used",
			src:  `<(a> <<( <(b> )>>`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenIRIRef, "(a"),
				tok(1, 6, nquads.TokenTripleTermOpen, "<<("),
				tok(1, 10, nquads.TokenIRIRef, "(b"),
				tok(1, 15, nquads.TokenTripleTermClose, ")>>"),
			},
		},
		{
			name: "an empty iri after a failed lookahead",
			src:  `<> <<(`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenIRIRef, ""),
				tok(1, 4, nquads.TokenTripleTermOpen, "<<("),
			},
		},
		{
			// The lookahead crosses a line ending on the way back, and the
			// tokens after it have to be counted from the line the '<' was on.
			name: "an iri and a triple term on either side of a line ending",
			src:  "<a>\r\n<<( )>>",
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenIRIRef, "a"),
				tok(1, 4, nquads.TokenEOL, "\r\n"),
				tok(2, 1, nquads.TokenTripleTermOpen, "<<("),
				tok(2, 5, nquads.TokenTripleTermClose, ")>>"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collect(nquads.Tokenize(strings.NewReader(tc.src)))
			if err != nil {
				t.Fatalf("Tokenize() = %v, want nil", err)
			}
			if !equalTokens(got, tc.expected) {
				t.Errorf("Tokenize() = %v, want %v", got, tc.expected)
			}
		})
	}
}

// TestTokenizeVersionDirective covers the VERSION keyword. Its specifier is an
// ordinary string literal, so the directive is two tokens rather than one.
func TestTokenizeVersionDirective(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []nquads.Token
	}{
		{
			name: "a version directive",
			src:  `VERSION "1.2"`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenVersion, "VERSION"),
				tok(1, 9, nquads.TokenStringLiteral, "1.2"),
			},
		},
		{
			name: "a version directive above a triple",
			src:  "VERSION \"1.2\"\n<http://example.com/s> <http://example.com/p> _:o .",
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenVersion, "VERSION"),
				tok(1, 9, nquads.TokenStringLiteral, "1.2"),
				tok(1, 14, nquads.TokenEOL, "\n"),
				tok(2, 1, nquads.TokenIRIRef, "http://example.com/s"),
				tok(2, 24, nquads.TokenIRIRef, "http://example.com/p"),
				tok(2, 47, nquads.TokenBlankNodeLabel, "o"),
				tok(2, 51, nquads.TokenDot, "."),
			},
		},
		{
			name: "the specifier needs no white space before it",
			src:  `VERSION"1.2"`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenVersion, "VERSION"),
				tok(1, 8, nquads.TokenStringLiteral, "1.2"),
			},
		},
		{
			name: "the keyword alone",
			src:  `VERSION`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenVersion, "VERSION"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collect(nquads.Tokenize(strings.NewReader(tc.src)))
			if err != nil {
				t.Fatalf("Tokenize() = %v, want nil", err)
			}
			if !equalTokens(got, tc.expected) {
				t.Errorf("Tokenize() = %v, want %v", got, tc.expected)
			}
		})
	}
}

// TestTokenizeLangDir covers LANG_DIR, which is the RDF 1.1 LANGTAG plus an
// optional base direction after a second hyphen.
func TestTokenizeLangDir(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []nquads.Token
	}{
		{
			name: "a tag with no direction",
			src:  `"text"@en .`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenStringLiteral, "text"),
				tok(1, 7, nquads.TokenLangDir, "en"),
				tok(1, 11, nquads.TokenDot, "."),
			},
		},
		{
			name: "a left to right direction",
			src:  `"text"@en--ltr .`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenStringLiteral, "text"),
				tok(1, 7, nquads.TokenLangDir, "en--ltr"),
				tok(1, 16, nquads.TokenDot, "."),
			},
		},
		{
			name: "a right to left direction",
			src:  `"نص"@ar--rtl .`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenStringLiteral, "نص"),
				tok(1, 5, nquads.TokenLangDir, "ar--rtl"),
				tok(1, 14, nquads.TokenDot, "."),
			},
		},
		{
			name: "a direction after several subtags",
			src:  `"text"@en-GB--ltr .`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenStringLiteral, "text"),
				tok(1, 7, nquads.TokenLangDir, "en-GB--ltr"),
				tok(1, 19, nquads.TokenDot, "."),
			},
		},
		{
			name: "a direction after a numeric subtag",
			src:  `"text"@es-419--rtl .`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenStringLiteral, "text"),
				tok(1, 7, nquads.TokenLangDir, "es-419--rtl"),
				tok(1, 20, nquads.TokenDot, "."),
			},
		},
		{
			// The grammar admits any run of letters; that it has to read "ltr"
			// or "rtl" is stated where RDF terms are built, and is enforced
			// when the literal becomes one.
			name: "a direction the grammar admits and the prose does not",
			src:  `"text"@en--xyz .`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenStringLiteral, "text"),
				tok(1, 7, nquads.TokenLangDir, "en--xyz"),
				tok(1, 16, nquads.TokenDot, "."),
			},
		},
		{
			name: "a hyphen after a direction begins no subtag",
			src:  `"text"@en--ltr.`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenStringLiteral, "text"),
				tok(1, 7, nquads.TokenLangDir, "en--ltr"),
				tok(1, 15, nquads.TokenDot, "."),
			},
		},
		{
			name: "a direction at the end of the input",
			src:  `"text"@en--ltr`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenStringLiteral, "text"),
				tok(1, 7, nquads.TokenLangDir, "en--ltr"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collect(nquads.Tokenize(strings.NewReader(tc.src)))
			if err != nil {
				t.Fatalf("Tokenize() = %v, want nil", err)
			}
			if !equalTokens(got, tc.expected) {
				t.Errorf("Tokenize() = %v, want %v", got, tc.expected)
			}
		})
	}
}

// TestTokenizeRDF12Errors covers the ways the lexemes RDF 1.2 adds can be
// written wrongly.
func TestTokenizeRDF12Errors(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			// "<<" opens nothing unless a parenthesis follows, and an IRIREF
			// excludes a '<', so the second one is the character at fault —
			// which is where RDF 1.1 reports it too.
			name:        "an angle bracket doubled without a parenthesis",
			src:         `<<http://example.com/s> <http://example.com/p> _:o .`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 2}, R: '<'},
		},
		{
			name:        "the RDF 1.1 star spelling of a quoted triple",
			src:         `<<_:a <http://example.com/p> _:b>> <http://example.com/q> _:c .`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 2}, R: '<'},
		},
		{
			name:        "an opener cut short by the end of the input",
			src:         `<<`,
			expectedErr: nquads.UnexpectedEndOfInputError{Pos: nquads.Pos{Line: 1, Column: 3}},
		},
		{
			name:        "a closer missing its second angle bracket",
			src:         `<<( )> .`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 7}, R: ' '},
		},
		{
			name:        "a closing parenthesis followed by nothing",
			src:         `<<( )`,
			expectedErr: nquads.UnexpectedEndOfInputError{Pos: nquads.Pos{Line: 1, Column: 6}},
		},
		{
			name:        "a closer cut short after one angle bracket",
			src:         `<<( )>`,
			expectedErr: nquads.UnexpectedEndOfInputError{Pos: nquads.Pos{Line: 1, Column: 7}},
		},
		{
			name:        "the version keyword in lower case",
			src:         `version "1.2"`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 1}, R: 'v'},
		},
		{
			name:        "the version keyword misspelled",
			src:         `VERSTON "1.2"`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 5}, R: 'T'},
		},
		{
			name:        "the version keyword cut short",
			src:         `VERS`,
			expectedErr: nquads.UnexpectedEndOfInputError{Pos: nquads.Pos{Line: 1, Column: 5}},
		},
		{
			name:        "a direction with no letters",
			src:         `"text"@en-- .`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 12}, R: ' '},
		},
		{
			name:        "a direction written with digits",
			src:         `"text"@en--1 .`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 12}, R: '1'},
		},
		{
			name:        "a third hyphen",
			src:         `"text"@en---ltr .`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 12}, R: '-'},
		},
		{
			name:        "a direction left hanging by the end of the input",
			src:         `"text"@en--`,
			expectedErr: nquads.UnexpectedEndOfInputError{Pos: nquads.Pos{Line: 1, Column: 12}},
		},
		{
			name:        "a subtag after a direction",
			src:         `"text"@en--ltr-GB .`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 15}, R: '-'},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect(nquads.Tokenize(strings.NewReader(tc.src)))
			if err != tc.expectedErr {
				t.Errorf("Tokenize() error = %v, want %v", err, tc.expectedErr)
			}
		})
	}
}

// rdf11Sources are documents written in RDF 1.1 N-Quads, which RDF 1.2
// N-Quads must go on accepting unchanged.
var rdf11Sources = []string{
	"",
	" \t  ",
	`<http://example.com/s> <http://example.com/p> <http://example.com/o> .`,
	`<> .`,
	`_:b0 <http://example.com/p> "o" .`,
	`<http://example.com/s> <http://example.com/p> "text"@en-GB .`,
	`_:a <http://example.com/p> "1"^^<http://www.w3.org/2001/XMLSchema#integer> .`,
	"<http://example.com/a> <http://example.com/p> _:x .\n_:y <http://example.com/q> \"v\" .",
	". \n\n\n .",
	".\r\n.",
	"# a comment\n.",
	`_:a <http://example.com/p> _:b . # why`,
	"#",
	`"\t\b\n\r\f\"\'\\"`,
	`"\u00E9\U0001F600\u00e9\u00C9"`,
	`"say \"hi\"" .`,
	"<http://example.com/\\u0061\\U0000006C>",
	`"a\nb" .`,
	`"héllo wörld" <http://example.com/ünïcødé>`,
	`_:a.b`,
	`_:b0.`,
	`_:b0..`,
	`_:a..b.`,
	`_:0`,
	`_:a-1-b`,
	`_:café`,
	`_:λόγος`,
	`_:日本語`,
	`_:𐀀𐀁`,
	`_:a·b`,
	"_:áb",
	"_:a‿b",
	`@en`,
	`@zh-Hant-TW`,
	`@es-419`,
	`@en.`,
	`"text"@en`,
	`"text"^^<http://example.com/dt>`,
	"\n",
	"<http://example.com/s> <http://example.com/p> _:o .\n",
	`<http://example.com/s> <http://example.com/p> <http://example.com/o> <http://example.com/g> .`,
	`<http://example.com/s> <http://example.com/p> "v" _:g .`,
	"<http://example.com/a> <http://example.com/p> \"1\" <http://example.com/g> . # labelled\n",
}

// TestTokenizeAcceptsRDF11 is one half of the promise that RDF 1.2 is a
// superset: every document RDF 1.1 accepts is tokenized here without error.
func TestTokenizeAcceptsRDF11(t *testing.T) {
	for _, src := range rdf11Sources {
		t.Run(src, func(t *testing.T) {
			if _, err := collect(nquads.Tokenize(strings.NewReader(src))); err != nil {
				t.Errorf("Tokenize() = %v, want nil", err)
			}
		})
	}
}

// TestTokenizeAgreesWithRDF11 is the other half: accepting an RDF 1.1 document
// is not enough, the tokens have to be the same ones, at the same positions,
// carrying the same values. A rule that swallowed a character or moved a
// position would pass [TestTokenizeAcceptsRDF11] and be caught here.
//
// Only LANGTAG changed name, LANG_DIR being the same production with a
// direction appended, so the two token streams are compared through
// [rdf12TokenType].
func TestTokenizeAgreesWithRDF11(t *testing.T) {
	for _, src := range rdf11Sources {
		t.Run(src, func(t *testing.T) {
			var want []nquads.Token
			for token, err := range nquads11.Tokenize(strings.NewReader(src)) {
				if err != nil {
					t.Fatalf("rdf11 Tokenize() = %v, want nil", err)
				}
				want = append(want, nquads.Token{
					Pos:   nquads.Pos{Line: token.Pos.Line, Column: token.Pos.Column},
					Type:  rdf12TokenType(t, token.Type),
					Value: token.Value,
				})
			}

			got, err := collect(nquads.Tokenize(strings.NewReader(src)))
			if err != nil {
				t.Fatalf("Tokenize() = %v, want nil", err)
			}
			if !equalTokens(got, want) {
				t.Errorf("Tokenize() = %v, want %v", got, want)
			}
		})
	}
}

// rdf12TokenType is the RDF 1.2 counterpart of an RDF 1.1 token type.
func rdf12TokenType(t *testing.T, tt nquads11.TokenType) nquads.TokenType {
	t.Helper()

	switch tt {
	case nquads11.TokenIRIRef:
		return nquads.TokenIRIRef
	case nquads11.TokenBlankNodeLabel:
		return nquads.TokenBlankNodeLabel
	case nquads11.TokenStringLiteral:
		return nquads.TokenStringLiteral
	case nquads11.TokenLangTag:
		return nquads.TokenLangDir
	case nquads11.TokenDatatypeMarker:
		return nquads.TokenDatatypeMarker
	case nquads11.TokenDot:
		return nquads.TokenDot
	case nquads11.TokenComment:
		return nquads.TokenComment
	case nquads11.TokenEOL:
		return nquads.TokenEOL
	default:
		t.Fatalf("no RDF 1.2 counterpart for %v", tt)
		return 0
	}
}

func TestTokenize(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []nquads.Token
	}{
		{
			name:     "empty input",
			src:      "",
			expected: nil,
		},
		{
			name:     "only white space",
			src:      " \t  ",
			expected: nil,
		},
		{
			name: "a triple of iris",
			src:  `<http://example.com/s> <http://example.com/p> <http://example.com/o> .`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenIRIRef, "http://example.com/s"),
				tok(1, 24, nquads.TokenIRIRef, "http://example.com/p"),
				tok(1, 47, nquads.TokenIRIRef, "http://example.com/o"),
				tok(1, 70, nquads.TokenDot, "."),
			},
		},
		{
			// A graph label is an IRIREF like any other. The tokenizer has no
			// notion of the position a terminal stands in, which is why N-Quads
			// needs no lexeme N-Triples does not have.
			name: "a statement with an iri graph label",
			src:  `<http://example.com/s> <http://example.com/p> <http://example.com/o> <http://example.com/g> .`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenIRIRef, "http://example.com/s"),
				tok(1, 24, nquads.TokenIRIRef, "http://example.com/p"),
				tok(1, 47, nquads.TokenIRIRef, "http://example.com/o"),
				tok(1, 70, nquads.TokenIRIRef, "http://example.com/g"),
				tok(1, 93, nquads.TokenDot, "."),
			},
		},
		{
			name: "a triple term object followed by a blank node graph label",
			src:  `_:s <http://example.com/p> <<( _:a <http://example.com/q> "v" )>> _:g .`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenBlankNodeLabel, "s"),
				tok(1, 5, nquads.TokenIRIRef, "http://example.com/p"),
				tok(1, 28, nquads.TokenTripleTermOpen, "<<("),
				tok(1, 32, nquads.TokenBlankNodeLabel, "a"),
				tok(1, 36, nquads.TokenIRIRef, "http://example.com/q"),
				tok(1, 59, nquads.TokenStringLiteral, "v"),
				tok(1, 63, nquads.TokenTripleTermClose, ")>>"),
				tok(1, 67, nquads.TokenBlankNodeLabel, "g"),
				tok(1, 71, nquads.TokenDot, "."),
			},
		},
		{
			name: "an empty iri",
			src:  `<> .`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenIRIRef, ""),
				tok(1, 4, nquads.TokenDot, "."),
			},
		},
		{
			name: "a blank node subject",
			src:  `_:b0 <http://example.com/p> "o" .`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenBlankNodeLabel, "b0"),
				tok(1, 6, nquads.TokenIRIRef, "http://example.com/p"),
				tok(1, 29, nquads.TokenStringLiteral, "o"),
				tok(1, 33, nquads.TokenDot, "."),
			},
		},
		{
			name: "a typed literal",
			src:  `_:a <http://example.com/p> "1"^^<http://www.w3.org/2001/XMLSchema#integer> .`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenBlankNodeLabel, "a"),
				tok(1, 5, nquads.TokenIRIRef, "http://example.com/p"),
				tok(1, 28, nquads.TokenStringLiteral, "1"),
				tok(1, 31, nquads.TokenDatatypeMarker, "^^"),
				tok(1, 33, nquads.TokenIRIRef, "http://www.w3.org/2001/XMLSchema#integer"),
				tok(1, 76, nquads.TokenDot, "."),
			},
		},
		{
			name: "two lines",
			src:  "<http://example.com/a> <http://example.com/p> _:x .\n_:y <http://example.com/q> \"v\" .",
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenIRIRef, "http://example.com/a"),
				tok(1, 24, nquads.TokenIRIRef, "http://example.com/p"),
				tok(1, 47, nquads.TokenBlankNodeLabel, "x"),
				tok(1, 51, nquads.TokenDot, "."),
				tok(1, 52, nquads.TokenEOL, "\n"),
				tok(2, 1, nquads.TokenBlankNodeLabel, "y"),
				tok(2, 5, nquads.TokenIRIRef, "http://example.com/q"),
				tok(2, 28, nquads.TokenStringLiteral, "v"),
				tok(2, 32, nquads.TokenDot, "."),
			},
		},
		{
			name: "a run of line endings is one EOL",
			src:  ". \n\n\n .",
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenDot, "."),
				tok(1, 3, nquads.TokenEOL, "\n\n\n"),
				tok(4, 2, nquads.TokenDot, "."),
			},
		},
		{
			name: "a carriage return and line feed pair is one line ending",
			src:  ".\r\n.",
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenDot, "."),
				tok(1, 2, nquads.TokenEOL, "\r\n"),
				tok(2, 1, nquads.TokenDot, "."),
			},
		},
		{
			name: "a comment is a token",
			src:  "# a comment\n.",
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenComment, "# a comment"),
				tok(1, 12, nquads.TokenEOL, "\n"),
				tok(2, 1, nquads.TokenDot, "."),
			},
		},
		{
			name: "a comment mentioning a triple term",
			src:  "# <<( a )>>\n.",
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenComment, "# <<( a )>>"),
				tok(1, 12, nquads.TokenEOL, "\n"),
				tok(2, 1, nquads.TokenDot, "."),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collect(nquads.Tokenize(strings.NewReader(tc.src)))
			if err != nil {
				t.Fatalf("Tokenize() = %v, want nil", err)
			}
			if !equalTokens(got, tc.expected) {
				t.Errorf("Tokenize() = %v, want %v", got, tc.expected)
			}
		})
	}
}

// TestTokenizeEscapes covers the escape forms and their decoding: every token
// value below is shorter than the source it came from.
func TestTokenizeEscapes(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []nquads.Token
	}{
		{
			name: "every ECHAR",
			src:  `"\t\b\n\r\f\"\'\\"`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenStringLiteral, "\t\b\n\r\f\"'\\"),
			},
		},
		{
			name: "a four digit UCHAR",
			src:  "\"\\u00E9\"",
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenStringLiteral, "é"),
			},
		},
		{
			name: "an eight digit UCHAR",
			src:  `"\U0001F600"`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenStringLiteral, "\U0001F600"),
			},
		},
		{
			name: "a UCHAR inside an IRI",
			src:  "<http://example.com/\\u0061\\U0000006C>",
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenIRIRef, "http://example.com/al"),
			},
		},
		{
			name: "a decoded line feed does not advance the position",
			src:  `"a\nb" .`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenStringLiteral, "a\nb"),
				tok(1, 8, nquads.TokenDot, "."),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collect(nquads.Tokenize(strings.NewReader(tc.src)))
			if err != nil {
				t.Fatalf("Tokenize() = %v, want nil", err)
			}
			if !equalTokens(got, tc.expected) {
				t.Errorf("Tokenize() = %v, want %v", got, tc.expected)
			}
		})
	}
}

// TestTokenizeBlankNodeLabels covers the place N-Quads needs more than a
// single character of lookahead beyond the '<' of a triple term: a dot inside
// a label is only part of it when a label character follows.
func TestTokenizeBlankNodeLabels(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []nquads.Token
	}{
		{
			name: "a dot between label characters belongs to the label",
			src:  `_:a.b`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenBlankNodeLabel, "a.b"),
			},
		},
		{
			name: "a trailing dot ends the triple instead",
			src:  `_:b0.`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenBlankNodeLabel, "b0"),
				tok(1, 5, nquads.TokenDot, "."),
			},
		},
		{
			name: "several trailing dots are each their own token",
			src:  `_:b0..`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenBlankNodeLabel, "b0"),
				tok(1, 5, nquads.TokenDot, "."),
				tok(1, 6, nquads.TokenDot, "."),
			},
		},
		{
			name: "dots resumed by a label character are kept",
			src:  `_:a..b.`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenBlankNodeLabel, "a..b"),
				tok(1, 7, nquads.TokenDot, "."),
			},
		},
		{
			name: "a label ended by a triple term closer",
			src:  `<<( _:a <http://example.com/p> _:b )>>`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenTripleTermOpen, "<<("),
				tok(1, 5, nquads.TokenBlankNodeLabel, "a"),
				tok(1, 9, nquads.TokenIRIRef, "http://example.com/p"),
				tok(1, 32, nquads.TokenBlankNodeLabel, "b"),
				tok(1, 36, nquads.TokenTripleTermClose, ")>>"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collect(nquads.Tokenize(strings.NewReader(tc.src)))
			if err != nil {
				t.Fatalf("Tokenize() = %v, want nil", err)
			}
			if !equalTokens(got, tc.expected) {
				t.Errorf("Tokenize() = %v, want %v", got, tc.expected)
			}
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
			name:        "a character that begins no terminal",
			src:         `$ <http://example.com/p> _:o .`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 1}, R: '$'},
		},
		{
			name:        "an underscore not followed by a colon",
			src:         `_x`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 2}, R: 'x'},
		},
		{
			name:        "a colon may not appear in a label",
			src:         `_::a`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 3}, R: ':'},
		},
		{
			name:        "a single caret",
			src:         `"o"^x`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 5}, R: 'x'},
		},
		{
			name:        "a space inside an iri",
			src:         `<http://example.com/a b>`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 22}, R: ' '},
		},
		{
			name:        "an unterminated iri at end of input",
			src:         `<http://example.com/a`,
			expectedErr: nquads.UnterminatedIRIRefError{Pos: nquads.Pos{Line: 1, Column: 1}},
		},
		{
			name:        "an angle bracket at the very end of the input",
			src:         `<`,
			expectedErr: nquads.UnterminatedIRIRefError{Pos: nquads.Pos{Line: 1, Column: 1}},
		},
		{
			name:        "an iri broken by a line ending",
			src:         "<http://example.com/a\n>",
			expectedErr: nquads.UnterminatedIRIRefError{Pos: nquads.Pos{Line: 1, Column: 1}},
		},
		{
			name:        "an unterminated string at end of input",
			src:         `"hello`,
			expectedErr: nquads.UnterminatedStringError{Pos: nquads.Pos{Line: 1, Column: 1}},
		},
		{
			name:        "a string on the second line reports its own position",
			src:         "<http://example.com/a> .\n  \"oops",
			expectedErr: nquads.UnterminatedStringError{Pos: nquads.Pos{Line: 2, Column: 3}},
		},
		{
			name:        "an unknown escape",
			src:         `"a\qb"`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 4}, R: 'q'},
		},
		{
			name:        "a non hexadecimal digit in a UCHAR",
			src:         `"\u00g0"`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 6}, R: 'g'},
		},
		{
			name:        "a UCHAR naming a surrogate",
			src:         `"\uD800"`,
			expectedErr: nquads.InvalidCodePointError{Pos: nquads.Pos{Line: 1, Column: 2}, Code: 0xD800},
		},
		{
			name:        "a language tag with no letters",
			src:         `"o"@1`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 5}, R: '1'},
		},
		{
			name:        "a language subtag left empty by another character",
			src:         `"o"@en-. `,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 8}, R: '.'},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect(nquads.Tokenize(strings.NewReader(tc.src)))
			if err != tc.expectedErr {
				t.Errorf("Tokenize() error = %v, want %v", err, tc.expectedErr)
			}
		})
	}
}

// TestTokenizeTruncatedInput covers input that stops part way through a
// terminal. Ending between terminals is the ordinary end of a document and no
// error at all, so the two have to be told apart rather than both taken for
// the end of the input — otherwise a truncated file tokenizes cleanly and the
// damage surfaces somewhere else entirely.
func TestTokenizeTruncatedInput(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name:        "a blank node label with nothing after its colon",
			src:         `_:`,
			expectedErr: nquads.UnexpectedEndOfInputError{Pos: nquads.Pos{Line: 1, Column: 3}},
		},
		{
			name:        "a single caret at the end of the input",
			src:         `"o"^`,
			expectedErr: nquads.UnexpectedEndOfInputError{Pos: nquads.Pos{Line: 1, Column: 5}},
		},
		{
			name:        "an at sign with no tag after it",
			src:         `"o"@`,
			expectedErr: nquads.UnexpectedEndOfInputError{Pos: nquads.Pos{Line: 1, Column: 5}},
		},
		{
			name:        "a language tag left hanging on a hyphen",
			src:         `"o"@en-`,
			expectedErr: nquads.UnexpectedEndOfInputError{Pos: nquads.Pos{Line: 1, Column: 8}},
		},
		{
			name:        "a UCHAR short of its digits",
			src:         `"\u00`,
			expectedErr: nquads.UnexpectedEndOfInputError{Pos: nquads.Pos{Line: 1, Column: 6}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect(nquads.Tokenize(strings.NewReader(tc.src)))
			if err != tc.expectedErr {
				t.Errorf("Tokenize() error = %v, want %v", err, tc.expectedErr)
			}
		})
	}
}

// TestTokenizeEndsCleanlyBetweenTerminals is the other half of
// [TestTokenizeTruncatedInput]: input that stops where a terminal has just
// finished is a complete document, and must not be reported as cut off.
func TestTokenizeEndsCleanlyBetweenTerminals(t *testing.T) {
	sources := []string{
		"",
		" ",
		".",
		"_:b0",
		"<http://example.com/a>",
		`"text"`,
		`"text"@en`,
		`"text"@en--ltr`,
		`"text"^^<http://example.com/dt>`,
		"# comment",
		"\n",
		"VERSION",
		`VERSION "1.2"`,
		"<<(",
		")>>",
		`<<( _:a <http://example.com/p> "v" )>>`,
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			if _, err := collect(nquads.Tokenize(strings.NewReader(src))); err != nil {
				t.Errorf("Tokenize() = %v, want nil", err)
			}
		})
	}
}

// TestTokenizeStopsEarly checks that a caller who stops ranging stops the
// tokenizer with them, rather than the iterator running on to the end of the
// input.
func TestTokenizeStopsEarly(t *testing.T) {
	src := `<<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .`

	var seen int
	for range nquads.Tokenize(strings.NewReader(src)) {
		seen++
		break
	}
	if want := 1; seen != want {
		t.Errorf("yielded %d tokens after break, want %d", seen, want)
	}
}

// TestTokenizeYieldsNothingAfterAnError checks the contract the doc comment
// states: an error ends the sequence.
func TestTokenizeYieldsNothingAfterAnError(t *testing.T) {
	// A valid triple, then a character that begins no terminal, then more
	// input that would tokenize perfectly well on its own.
	src := "<http://example.com/s> .\n$ <http://example.com/p> .\n<http://example.com/x> .\n"

	var (
		tokens []nquads.Token
		errs   []error
	)
	for token, err := range nquads.Tokenize(strings.NewReader(src)) {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		tokens = append(tokens, token)
	}

	if len(errs) != 1 {
		t.Fatalf("yielded %d errors, want 1", len(errs))
	}
	want := []nquads.Token{
		tok(1, 1, nquads.TokenIRIRef, "http://example.com/s"),
		tok(1, 24, nquads.TokenDot, "."),
		tok(1, 25, nquads.TokenEOL, "\n"),
	}
	if !equalTokens(tokens, want) {
		t.Errorf("yielded %v, want %v", tokens, want)
	}
}

// TestErrorMessages pins the text of each error, which is what a user reading
// a failed parse actually sees.
func TestErrorMessages(t *testing.T) {
	pos := nquads.Pos{Line: 3, Column: 12}

	testCases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "unexpected character",
			err:  nquads.UnexpectedCharacterError{Pos: pos, R: '$'},
			want: `unexpected character '$' at line 3, column 12`,
		},
		{
			name: "unterminated string",
			err:  nquads.UnterminatedStringError{Pos: pos},
			want: "unterminated string literal at line 3, column 12",
		},
		{
			name: "unterminated iri",
			err:  nquads.UnterminatedIRIRefError{Pos: pos},
			want: "unterminated IRI at line 3, column 12",
		},
		{
			name: "unexpected end of input",
			err:  nquads.UnexpectedEndOfInputError{Pos: pos},
			want: "unexpected end of input at line 3, column 12",
		},
		{
			name: "invalid code point",
			err:  nquads.InvalidCodePointError{Pos: pos, Code: 0xD800},
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

// failingReader yields its data and then fails, standing in for a network
// connection or a file that goes wrong part way through.
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

// TestTokenizeReaderError checks that a failure of the underlying reader is
// reported as itself, rather than being mistaken for the end of the document.
// A truncated read looks exactly like a complete one otherwise.
func TestTokenizeReaderError(t *testing.T) {
	errBoom := errors.New("boom")

	testCases := []struct {
		name string
		src  string
	}{
		{
			name: "while skipping white space",
			src:  "   ",
		},
		{
			name: "part way through an iri",
			src:  `<http://example.com/a`,
		},
		{
			name: "part way through a string literal",
			src:  `"hello`,
		},
		{
			name: "part way through a comment",
			src:  "# a comment",
		},
		{
			name: "part way through a blank node label",
			src:  `_:abc`,
		},
		{
			name: "part way through a language tag",
			src:  `"o"@en`,
		},
		{
			name: "part way through a base direction",
			src:  `"o"@en--l`,
		},
		{
			name: "after the first angle bracket of a triple term",
			src:  `<`,
		},
		{
			name: "part way through a triple term opener",
			src:  `<<`,
		},
		{
			name: "part way through a triple term closer",
			src:  `)`,
		},
		{
			name: "part way through the version keyword",
			src:  `VER`,
		},
		{
			name: "part way through a line ending",
			src:  ".\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := &failingReader{data: []byte(tc.src), err: errBoom}

			_, err := collect(nquads.Tokenize(r))
			if !errors.Is(err, errBoom) {
				t.Errorf("Tokenize() error = %v, want %v", err, errBoom)
			}
		})
	}
}

// TestTokenizeUnicodeLabels covers the ranges PN_CHARS_BASE and PN_CHARS admit
// beyond ASCII. A blank node label is not limited to the Latin alphabet, and a
// tokenizer that assumed so would reject documents that are perfectly valid.
func TestTokenizeUnicodeLabels(t *testing.T) {
	testCases := []struct {
		name  string
		src   string
		label string
	}{
		{
			name:  "a latin letter with a diacritic",
			src:   `_:café`,
			label: "café",
		},
		{
			name:  "han",
			src:   `_:日本語`,
			label: "日本語",
		},
		{
			name:  "a character from beyond the basic multilingual plane",
			src:   `_:𐀀𐀁`,
			label: "𐀀𐀁",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collect(nquads.Tokenize(strings.NewReader(tc.src)))
			if err != nil {
				t.Fatalf("Tokenize() = %v, want nil", err)
			}

			want := []nquads.Token{tok(1, 1, nquads.TokenBlankNodeLabel, tc.label)}
			if !equalTokens(got, want) {
				t.Errorf("Tokenize() = %v, want %v", got, want)
			}
		})
	}
}

func TestTokenTypeString(t *testing.T) {
	types := map[nquads.TokenType]string{
		nquads.TokenIRIRef:          "IRIRef",
		nquads.TokenBlankNodeLabel:  "BlankNodeLabel",
		nquads.TokenStringLiteral:   "StringLiteral",
		nquads.TokenLangDir:         "LangDir",
		nquads.TokenDatatypeMarker:  "DatatypeMarker",
		nquads.TokenTripleTermOpen:  "TripleTermOpen",
		nquads.TokenTripleTermClose: "TripleTermClose",
		nquads.TokenVersion:         "Version",
		nquads.TokenDot:             "Dot",
		nquads.TokenComment:         "Comment",
		nquads.TokenEOL:             "EOL",
	}

	for tt, want := range types {
		t.Run(want, func(t *testing.T) {
			if got := tt.String(); got != want {
				t.Errorf("String() = %q, want %q", got, want)
			}
		})
	}

	t.Run("an unknown type panics rather than lying", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("String() returned normally, want a panic")
			}
		}()

		_ = nquads.TokenType(99).String()
	})
}

func TestTokenString(t *testing.T) {
	got := tok(1, 1, nquads.TokenTripleTermOpen, "<<(").String()
	if want := "TripleTermOpen(<<()"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestPosString(t *testing.T) {
	got := nquads.Pos{Line: 3, Column: 12}.String()
	if want := "3:12"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
