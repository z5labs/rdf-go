package ntriples_test

import (
	"errors"
	"iter"
	"slices"
	"strings"
	"testing"

	"github.com/z5labs/rdf-go/rdf11/ntriples"
)

// collect drains a token sequence, returning the tokens yielded before any
// error and the error that stopped it.
func collect(seq iter.Seq2[ntriples.Token, error]) ([]ntriples.Token, error) {
	var tokens []ntriples.Token
	for token, err := range seq {
		if err != nil {
			return tokens, err
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

// tok builds an expected token, keeping the tables readable.
func tok(line, column int, tt ntriples.TokenType, value string) ntriples.Token {
	return ntriples.Token{
		Pos:   ntriples.Pos{Line: line, Column: column},
		Type:  tt,
		Value: []byte(value),
	}
}

func equalTokens(got, want []ntriples.Token) bool {
	return slices.EqualFunc(got, want, func(a, b ntriples.Token) bool {
		return a.Pos == b.Pos && a.Type == b.Type && string(a.Value) == string(b.Value)
	})
}

func TestTokenize(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []ntriples.Token
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
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenIRIRef, "http://example.com/s"),
				tok(1, 24, ntriples.TokenIRIRef, "http://example.com/p"),
				tok(1, 47, ntriples.TokenIRIRef, "http://example.com/o"),
				tok(1, 70, ntriples.TokenDot, "."),
			},
		},
		{
			name: "an empty iri",
			src:  `<> .`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenIRIRef, ""),
				tok(1, 4, ntriples.TokenDot, "."),
			},
		},
		{
			name: "a blank node subject",
			src:  `_:b0 <http://example.com/p> "o" .`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenBlankNodeLabel, "b0"),
				tok(1, 6, ntriples.TokenIRIRef, "http://example.com/p"),
				tok(1, 29, ntriples.TokenStringLiteral, "o"),
				tok(1, 33, ntriples.TokenDot, "."),
			},
		},
		{
			name: "a language tagged literal",
			src:  `<http://example.com/s> <http://example.com/p> "text"@en-GB .`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenIRIRef, "http://example.com/s"),
				tok(1, 24, ntriples.TokenIRIRef, "http://example.com/p"),
				tok(1, 47, ntriples.TokenStringLiteral, "text"),
				tok(1, 53, ntriples.TokenLangTag, "en-GB"),
				tok(1, 60, ntriples.TokenDot, "."),
			},
		},
		{
			name: "a typed literal",
			src:  `_:a <http://example.com/p> "1"^^<http://www.w3.org/2001/XMLSchema#integer> .`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenBlankNodeLabel, "a"),
				tok(1, 5, ntriples.TokenIRIRef, "http://example.com/p"),
				tok(1, 28, ntriples.TokenStringLiteral, "1"),
				tok(1, 31, ntriples.TokenDatatypeMarker, "^^"),
				tok(1, 33, ntriples.TokenIRIRef, "http://www.w3.org/2001/XMLSchema#integer"),
				tok(1, 76, ntriples.TokenDot, "."),
			},
		},
		{
			name: "two lines",
			src:  "<http://example.com/a> <http://example.com/p> _:x .\n_:y <http://example.com/q> \"v\" .",
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenIRIRef, "http://example.com/a"),
				tok(1, 24, ntriples.TokenIRIRef, "http://example.com/p"),
				tok(1, 47, ntriples.TokenBlankNodeLabel, "x"),
				tok(1, 51, ntriples.TokenDot, "."),
				tok(1, 52, ntriples.TokenEOL, "\n"),
				tok(2, 1, ntriples.TokenBlankNodeLabel, "y"),
				tok(2, 5, ntriples.TokenIRIRef, "http://example.com/q"),
				tok(2, 28, ntriples.TokenStringLiteral, "v"),
				tok(2, 32, ntriples.TokenDot, "."),
			},
		},
		{
			name: "a run of line endings is one EOL",
			src:  ". \n\n\n .",
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenDot, "."),
				tok(1, 3, ntriples.TokenEOL, "\n\n\n"),
				tok(4, 2, ntriples.TokenDot, "."),
			},
		},
		{
			name: "a carriage return and line feed pair is one line ending",
			src:  ".\r\n.",
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenDot, "."),
				tok(1, 2, ntriples.TokenEOL, "\r\n"),
				tok(2, 1, ntriples.TokenDot, "."),
			},
		},
		{
			name: "a comment is a token",
			src:  "# a comment\n.",
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenComment, "# a comment"),
				tok(1, 12, ntriples.TokenEOL, "\n"),
				tok(2, 1, ntriples.TokenDot, "."),
			},
		},
		{
			name: "a comment at the end of a triple",
			src:  `_:a <http://example.com/p> _:b . # why`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenBlankNodeLabel, "a"),
				tok(1, 5, ntriples.TokenIRIRef, "http://example.com/p"),
				tok(1, 28, ntriples.TokenBlankNodeLabel, "b"),
				tok(1, 32, ntriples.TokenDot, "."),
				tok(1, 34, ntriples.TokenComment, "# why"),
			},
		},
		{
			name: "a comment with no text",
			src:  "#",
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenComment, "#"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collect(ntriples.Tokenize(strings.NewReader(tc.src)))
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
		expected []ntriples.Token
	}{
		{
			name: "every ECHAR",
			src:  `"\t\b\n\r\f\"\'\\"`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenStringLiteral, "\t\b\n\r\f\"'\\"),
			},
		},
		{
			name: "a four digit UCHAR",
			src:  "\"\\u00E9\"",
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenStringLiteral, "é"),
			},
		},
		{
			name: "an eight digit UCHAR",
			src:  `"\U0001F600"`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenStringLiteral, "\U0001F600"),
			},
		},
		{
			name: "hexadecimal digits in either case",
			src:  "\"\\u00e9\\u00C9\"",
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenStringLiteral, "éÉ"),
			},
		},
		{
			name: "an escaped quote does not end the literal",
			src:  `"say \"hi\"" .`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenStringLiteral, `say "hi"`),
				tok(1, 14, ntriples.TokenDot, "."),
			},
		},
		{
			name: "a UCHAR inside an IRI",
			src:  "<http://example.com/\\u0061\\U0000006C>",
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenIRIRef, "http://example.com/al"),
			},
		},
		{
			name: "a decoded line feed does not advance the position",
			src:  `"a\nb" .`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenStringLiteral, "a\nb"),
				tok(1, 8, ntriples.TokenDot, "."),
			},
		},
		{
			name: "non ascii text passes through",
			src:  `"héllo wörld" <http://example.com/ünïcødé>`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenStringLiteral, "héllo wörld"),
				tok(1, 15, ntriples.TokenIRIRef, "http://example.com/ünïcødé"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collect(ntriples.Tokenize(strings.NewReader(tc.src)))
			if err != nil {
				t.Fatalf("Tokenize() = %v, want nil", err)
			}
			if !equalTokens(got, tc.expected) {
				t.Errorf("Tokenize() = %v, want %v", got, tc.expected)
			}
		})
	}
}

// TestTokenizeBlankNodeLabels covers the one place N-Triples needs more than a
// single character of lookahead: a dot inside a label is only part of it when
// a label character follows.
func TestTokenizeBlankNodeLabels(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []ntriples.Token
	}{
		{
			name: "a dot between label characters belongs to the label",
			src:  `_:a.b`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenBlankNodeLabel, "a.b"),
			},
		},
		{
			name: "a trailing dot ends the triple instead",
			src:  `_:b0.`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenBlankNodeLabel, "b0"),
				tok(1, 5, ntriples.TokenDot, "."),
			},
		},
		{
			name: "a trailing dot before white space",
			src:  `_:b0. `,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenBlankNodeLabel, "b0"),
				tok(1, 5, ntriples.TokenDot, "."),
			},
		},
		{
			name: "several trailing dots are each their own token",
			src:  `_:b0..`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenBlankNodeLabel, "b0"),
				tok(1, 5, ntriples.TokenDot, "."),
				tok(1, 6, ntriples.TokenDot, "."),
			},
		},
		{
			name: "dots resumed by a label character are kept",
			src:  `_:a..b.`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenBlankNodeLabel, "a..b"),
				tok(1, 7, ntriples.TokenDot, "."),
			},
		},
		{
			name: "a label may begin with a digit",
			src:  `_:0`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenBlankNodeLabel, "0"),
			},
		},
		{
			name: "a label may contain hyphens and digits",
			src:  `_:a-1-b`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenBlankNodeLabel, "a-1-b"),
			},
		},
		{
			name: "a label of one character",
			src:  `_:x .`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenBlankNodeLabel, "x"),
				tok(1, 5, ntriples.TokenDot, "."),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collect(ntriples.Tokenize(strings.NewReader(tc.src)))
			if err != nil {
				t.Fatalf("Tokenize() = %v, want nil", err)
			}
			if !equalTokens(got, tc.expected) {
				t.Errorf("Tokenize() = %v, want %v", got, tc.expected)
			}
		})
	}
}

func TestTokenizeLangTags(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []ntriples.Token
	}{
		{
			name: "a primary subtag alone",
			src:  `@en`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenLangTag, "en"),
			},
		},
		{
			name: "several subtags",
			src:  `@zh-Hant-TW`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenLangTag, "zh-Hant-TW"),
			},
		},
		{
			name: "digits are allowed after the first subtag",
			src:  `@es-419`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenLangTag, "es-419"),
			},
		},
		{
			name: "a tag ends at the dot",
			src:  `@en.`,
			expected: []ntriples.Token{
				tok(1, 1, ntriples.TokenLangTag, "en"),
				tok(1, 4, ntriples.TokenDot, "."),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collect(ntriples.Tokenize(strings.NewReader(tc.src)))
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
			expectedErr: ntriples.UnexpectedCharacterError{Pos: ntriples.Pos{Line: 1, Column: 1}, R: '$'},
		},
		{
			name:        "an underscore not followed by a colon",
			src:         `_x`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: ntriples.Pos{Line: 1, Column: 2}, R: 'x'},
		},
		{
			name:        "a blank node label that begins with a hyphen",
			src:         `_:-a`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: ntriples.Pos{Line: 1, Column: 3}, R: '-'},
		},
		{
			name:        "a colon may not appear in a label",
			src:         `_::a`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: ntriples.Pos{Line: 1, Column: 3}, R: ':'},
		},
		{
			name:        "nor part way through one",
			src:         `_:abc:def`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: ntriples.Pos{Line: 1, Column: 6}, R: ':'},
		},
		{
			name:        "a single caret",
			src:         `"o"^x`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: ntriples.Pos{Line: 1, Column: 5}, R: 'x'},
		},
		{
			name:        "a space inside an iri",
			src:         `<http://example.com/a b>`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: ntriples.Pos{Line: 1, Column: 22}, R: ' '},
		},
		{
			name:        "a delimiter the IRIREF production excludes",
			src:         `<http://example.com/{}>`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: ntriples.Pos{Line: 1, Column: 21}, R: '{'},
		},
		{
			name:        "an unterminated iri at end of input",
			src:         `<http://example.com/a`,
			expectedErr: ntriples.UnterminatedIRIRefError{Pos: ntriples.Pos{Line: 1, Column: 1}},
		},
		{
			name:        "an iri broken by a line ending",
			src:         "<http://example.com/a\n>",
			expectedErr: ntriples.UnterminatedIRIRefError{Pos: ntriples.Pos{Line: 1, Column: 1}},
		},
		{
			name:        "an unterminated string at end of input",
			src:         `"hello`,
			expectedErr: ntriples.UnterminatedStringError{Pos: ntriples.Pos{Line: 1, Column: 1}},
		},
		{
			name:        "a string broken by a line ending",
			src:         "\"hello\nworld\"",
			expectedErr: ntriples.UnterminatedStringError{Pos: ntriples.Pos{Line: 1, Column: 1}},
		},
		{
			name:        "a string on the second line reports its own position",
			src:         "<http://example.com/a> .\n  \"oops",
			expectedErr: ntriples.UnterminatedStringError{Pos: ntriples.Pos{Line: 2, Column: 3}},
		},
		{
			name:        "an unknown escape",
			src:         `"a\qb"`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: ntriples.Pos{Line: 1, Column: 4}, R: 'q'},
		},
		{
			name:        "an ECHAR is not allowed inside an iri",
			src:         `<http://example.com/\n>`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: ntriples.Pos{Line: 1, Column: 22}, R: 'n'},
		},
		{
			name:        "a non hexadecimal digit in a UCHAR",
			src:         `"\u00g0"`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: ntriples.Pos{Line: 1, Column: 6}, R: 'g'},
		},
		{
			name:        "a UCHAR naming a surrogate",
			src:         `"\uD800"`,
			expectedErr: ntriples.InvalidCodePointError{Pos: ntriples.Pos{Line: 1, Column: 2}, Code: 0xD800},
		},
		{
			name:        "a UCHAR above the highest code point",
			src:         `"\U00110000"`,
			expectedErr: ntriples.InvalidCodePointError{Pos: ntriples.Pos{Line: 1, Column: 2}, Code: 0x110000},
		},
		{
			name:        "a language tag with no letters",
			src:         `"o"@1`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: ntriples.Pos{Line: 1, Column: 5}, R: '1'},
		},
		{
			name:        "a language subtag left empty by another character",
			src:         `"o"@en-. `,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: ntriples.Pos{Line: 1, Column: 8}, R: '.'},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect(ntriples.Tokenize(strings.NewReader(tc.src)))
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
			expectedErr: ntriples.UnexpectedEndOfInputError{Pos: ntriples.Pos{Line: 1, Column: 3}},
		},
		{
			name:        "an underscore with nothing after it",
			src:         `_`,
			expectedErr: ntriples.UnexpectedEndOfInputError{Pos: ntriples.Pos{Line: 1, Column: 2}},
		},
		{
			name:        "a single caret at the end of the input",
			src:         `"o"^`,
			expectedErr: ntriples.UnexpectedEndOfInputError{Pos: ntriples.Pos{Line: 1, Column: 5}},
		},
		{
			name:        "an at sign with no tag after it",
			src:         `"o"@`,
			expectedErr: ntriples.UnexpectedEndOfInputError{Pos: ntriples.Pos{Line: 1, Column: 5}},
		},
		{
			name:        "a language tag left hanging on a hyphen",
			src:         `"o"@en-`,
			expectedErr: ntriples.UnexpectedEndOfInputError{Pos: ntriples.Pos{Line: 1, Column: 8}},
		},
		{
			name:        "a UCHAR short of its digits",
			src:         `"\u00`,
			expectedErr: ntriples.UnexpectedEndOfInputError{Pos: ntriples.Pos{Line: 1, Column: 6}},
		},
		{
			name:        "a backslash with nothing after it",
			src:         `"a\`,
			expectedErr: ntriples.UnexpectedEndOfInputError{Pos: ntriples.Pos{Line: 1, Column: 4}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect(ntriples.Tokenize(strings.NewReader(tc.src)))
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
		"\t \t",
		".",
		". ",
		"_:b0",
		"<http://example.com/a>",
		`"text"`,
		`"text"@en`,
		`"text"^^<http://example.com/dt>`,
		"# comment",
		"\n",
		"<http://example.com/s> <http://example.com/p> _:o .",
		"<http://example.com/s> <http://example.com/p> _:o .\n",
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			if _, err := collect(ntriples.Tokenize(strings.NewReader(src))); err != nil {
				t.Errorf("Tokenize() = %v, want nil", err)
			}
		})
	}
}

// TestTokenizeRejectsRDF12 covers the lexemes RDF 1.2 adds. This is the 1.1
// package, so each has to be refused rather than quietly accepted — a document
// using them is not the dialect this package reads.
func TestTokenizeRejectsRDF12(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name:        "the triple term opener",
			src:         `<<( <http://example.com/s> <http://example.com/p> _:o )>> .`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: ntriples.Pos{Line: 1, Column: 2}, R: '<'},
		},
		{
			name:        "the triple term closer",
			src:         `)>> .`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: ntriples.Pos{Line: 1, Column: 1}, R: ')'},
		},
		{
			name:        "a left to right base direction",
			src:         `"text"@en--ltr .`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: ntriples.Pos{Line: 1, Column: 11}, R: '-'},
		},
		{
			name:        "a right to left base direction",
			src:         `"نص"@ar--rtl .`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: ntriples.Pos{Line: 1, Column: 9}, R: '-'},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect(ntriples.Tokenize(strings.NewReader(tc.src)))
			if err != tc.expectedErr {
				t.Errorf("Tokenize() error = %v, want %v", err, tc.expectedErr)
			}
		})
	}
}

// TestTokenizeStopsEarly checks that a caller who stops ranging stops the
// tokenizer with them, rather than the iterator running on to the end of the
// input.
func TestTokenizeStopsEarly(t *testing.T) {
	src := `<http://example.com/s> <http://example.com/p> <http://example.com/o> .`

	var seen int
	for range ntriples.Tokenize(strings.NewReader(src)) {
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
		tokens []ntriples.Token
		errs   []error
	)
	for token, err := range ntriples.Tokenize(strings.NewReader(src)) {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		tokens = append(tokens, token)
	}

	if len(errs) != 1 {
		t.Fatalf("yielded %d errors, want 1", len(errs))
	}
	want := []ntriples.Token{
		tok(1, 1, ntriples.TokenIRIRef, "http://example.com/s"),
		tok(1, 24, ntriples.TokenDot, "."),
		tok(1, 25, ntriples.TokenEOL, "\n"),
	}
	if !equalTokens(tokens, want) {
		t.Errorf("yielded %v, want %v", tokens, want)
	}
}

// TestErrorMessages pins the text of each error, which is what a user reading
// a failed parse actually sees.
func TestErrorMessages(t *testing.T) {
	pos := ntriples.Pos{Line: 3, Column: 12}

	testCases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "unexpected character",
			err:  ntriples.UnexpectedCharacterError{Pos: pos, R: '$'},
			want: `unexpected character '$' at line 3, column 12`,
		},
		{
			name: "unterminated string",
			err:  ntriples.UnterminatedStringError{Pos: pos},
			want: "unterminated string literal at line 3, column 12",
		},
		{
			name: "unterminated iri",
			err:  ntriples.UnterminatedIRIRefError{Pos: pos},
			want: "unterminated IRI at line 3, column 12",
		},
		{
			name: "unexpected end of input",
			err:  ntriples.UnexpectedEndOfInputError{Pos: pos},
			want: "unexpected end of input at line 3, column 12",
		},
		{
			name: "invalid code point",
			err:  ntriples.InvalidCodePointError{Pos: pos, Code: 0xD800},
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
			name: "part way through a line ending",
			src:  ".\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := &failingReader{data: []byte(tc.src), err: errBoom}

			_, err := collect(ntriples.Tokenize(r))
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
			name:  "greek",
			src:   `_:λόγος`,
			label: "λόγος",
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
		{
			name:  "a middle dot, which may not begin a label",
			src:   `_:a·b`,
			label: "a·b",
		},
		{
			name:  "a combining mark, which may not begin a label",
			src:   "_:áb",
			label: "áb",
		},
		{
			name:  "an undertie, which may not begin a label",
			src:   "_:a‿b",
			label: "a‿b",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collect(ntriples.Tokenize(strings.NewReader(tc.src)))
			if err != nil {
				t.Fatalf("Tokenize() = %v, want nil", err)
			}

			want := []ntriples.Token{tok(1, 1, ntriples.TokenBlankNodeLabel, tc.label)}
			if !equalTokens(got, want) {
				t.Errorf("Tokenize() = %v, want %v", got, want)
			}
		})
	}
}

func TestTokenTypeString(t *testing.T) {
	types := map[ntriples.TokenType]string{
		ntriples.TokenIRIRef:         "IRIRef",
		ntriples.TokenBlankNodeLabel: "BlankNodeLabel",
		ntriples.TokenStringLiteral:  "StringLiteral",
		ntriples.TokenLangTag:        "LangTag",
		ntriples.TokenDatatypeMarker: "DatatypeMarker",
		ntriples.TokenDot:            "Dot",
		ntriples.TokenComment:        "Comment",
		ntriples.TokenEOL:            "EOL",
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

		_ = ntriples.TokenType(99).String()
	})
}

func TestTokenString(t *testing.T) {
	got := tok(1, 1, ntriples.TokenIRIRef, "http://example.com/a").String()
	if want := "IRIRef(http://example.com/a)"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestPosString(t *testing.T) {
	got := ntriples.Pos{Line: 3, Column: 12}.String()
	if want := "3:12"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
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
			_, err := collect(ntriples.Tokenize(strings.NewReader(tc.src)))

			// Column 12 is the backslash: the '<' is column 1 and
			// "http://e/a" is the ten characters after it.
			want := ntriples.EscapedIRICharacterError{
				Pos: ntriples.Pos{Line: 1, Column: 12},
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
	tokens, err := collect(ntriples.Tokenize(strings.NewReader(`<http://e/a\u0062c>`)))
	if err != nil {
		t.Fatalf("Tokenize() error = %v, want nil", err)
	}
	if len(tokens) != 1 || string(tokens[0].Value) != "http://e/abc" {
		t.Errorf("Tokenize() = %v, want the iri http://e/abc", tokens)
	}
}
