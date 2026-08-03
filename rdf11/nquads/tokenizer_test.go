package nquads_test

import (
	"errors"
	"iter"
	"slices"
	"strings"
	"testing"

	"github.com/z5labs/rdf-go/rdf11/nquads"
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
			name: "a language tagged literal",
			src:  `<http://example.com/s> <http://example.com/p> "text"@en-GB .`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenIRIRef, "http://example.com/s"),
				tok(1, 24, nquads.TokenIRIRef, "http://example.com/p"),
				tok(1, 47, nquads.TokenStringLiteral, "text"),
				tok(1, 53, nquads.TokenLangTag, "en-GB"),
				tok(1, 60, nquads.TokenDot, "."),
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
			name: "a comment at the end of a triple",
			src:  `_:a <http://example.com/p> _:b . # why`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenBlankNodeLabel, "a"),
				tok(1, 5, nquads.TokenIRIRef, "http://example.com/p"),
				tok(1, 28, nquads.TokenBlankNodeLabel, "b"),
				tok(1, 32, nquads.TokenDot, "."),
				tok(1, 34, nquads.TokenComment, "# why"),
			},
		},
		{
			name: "a comment with no text",
			src:  "#",
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenComment, "#"),
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
			name: "hexadecimal digits in either case",
			src:  "\"\\u00e9\\u00C9\"",
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenStringLiteral, "éÉ"),
			},
		},
		{
			name: "an escaped quote does not end the literal",
			src:  `"say \"hi\"" .`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenStringLiteral, `say "hi"`),
				tok(1, 14, nquads.TokenDot, "."),
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
		{
			name: "non ascii text passes through",
			src:  `"héllo wörld" <http://example.com/ünïcødé>`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenStringLiteral, "héllo wörld"),
				tok(1, 15, nquads.TokenIRIRef, "http://example.com/ünïcødé"),
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

// TestTokenizeBlankNodeLabels covers the one place N-Triples needs more than a
// single character of lookahead: a dot inside a label is only part of it when
// a label character follows.
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
			name: "a trailing dot before white space",
			src:  `_:b0. `,
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
			name: "a label may begin with a digit",
			src:  `_:0`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenBlankNodeLabel, "0"),
			},
		},
		{
			name: "a label may contain hyphens and digits",
			src:  `_:a-1-b`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenBlankNodeLabel, "a-1-b"),
			},
		},
		{
			name: "a label of one character",
			src:  `_:x .`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenBlankNodeLabel, "x"),
				tok(1, 5, nquads.TokenDot, "."),
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

func TestTokenizeLangTags(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []nquads.Token
	}{
		{
			name: "a primary subtag alone",
			src:  `@en`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenLangTag, "en"),
			},
		},
		{
			name: "several subtags",
			src:  `@zh-Hant-TW`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenLangTag, "zh-Hant-TW"),
			},
		},
		{
			name: "digits are allowed after the first subtag",
			src:  `@es-419`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenLangTag, "es-419"),
			},
		},
		{
			name: "a tag ends at the dot",
			src:  `@en.`,
			expected: []nquads.Token{
				tok(1, 1, nquads.TokenLangTag, "en"),
				tok(1, 4, nquads.TokenDot, "."),
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
			name:        "a blank node label that begins with a hyphen",
			src:         `_:-a`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 3}, R: '-'},
		},
		{
			name:        "a colon may not appear in a label",
			src:         `_::a`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 3}, R: ':'},
		},
		{
			name:        "nor part way through one",
			src:         `_:abc:def`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 6}, R: ':'},
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
			name:        "a delimiter the IRIREF production excludes",
			src:         `<http://example.com/{}>`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 21}, R: '{'},
		},
		{
			name:        "an unterminated iri at end of input",
			src:         `<http://example.com/a`,
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
			name:        "a string broken by a line ending",
			src:         "\"hello\nworld\"",
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
			name:        "an ECHAR is not allowed inside an iri",
			src:         `<http://example.com/\n>`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 22}, R: 'n'},
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
			name:        "a UCHAR above the highest code point",
			src:         `"\U00110000"`,
			expectedErr: nquads.InvalidCodePointError{Pos: nquads.Pos{Line: 1, Column: 2}, Code: 0x110000},
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
			name:        "an underscore with nothing after it",
			src:         `_`,
			expectedErr: nquads.UnexpectedEndOfInputError{Pos: nquads.Pos{Line: 1, Column: 2}},
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
		{
			name:        "a backslash with nothing after it",
			src:         `"a\`,
			expectedErr: nquads.UnexpectedEndOfInputError{Pos: nquads.Pos{Line: 1, Column: 4}},
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
			if _, err := collect(nquads.Tokenize(strings.NewReader(src))); err != nil {
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
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 2}, R: '<'},
		},
		{
			name:        "the triple term closer",
			src:         `)>> .`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 1}, R: ')'},
		},
		{
			name:        "a left to right base direction",
			src:         `"text"@en--ltr .`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 11}, R: '-'},
		},
		{
			name:        "a right to left base direction",
			src:         `"نص"@ar--rtl .`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: nquads.Pos{Line: 1, Column: 9}, R: '-'},
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

// TestTokenizeStopsEarly checks that a caller who stops ranging stops the
// tokenizer with them, rather than the iterator running on to the end of the
// input.
func TestTokenizeStopsEarly(t *testing.T) {
	src := `<http://example.com/s> <http://example.com/p> <http://example.com/o> .`

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
		nquads.TokenIRIRef:         "IRIRef",
		nquads.TokenBlankNodeLabel: "BlankNodeLabel",
		nquads.TokenStringLiteral:  "StringLiteral",
		nquads.TokenLangTag:        "LangTag",
		nquads.TokenDatatypeMarker: "DatatypeMarker",
		nquads.TokenDot:            "Dot",
		nquads.TokenComment:        "Comment",
		nquads.TokenEOL:            "EOL",
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
	got := tok(1, 1, nquads.TokenIRIRef, "http://example.com/a").String()
	if want := "IRIRef(http://example.com/a)"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestPosString(t *testing.T) {
	got := nquads.Pos{Line: 3, Column: 12}.String()
	if want := "3:12"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
