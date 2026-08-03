package turtle_test

import (
	"iter"
	"slices"
	"strings"
	"testing"

	"github.com/z5labs/rdf-go/rdf11/turtle"
)

func collect(seq iter.Seq2[turtle.Token, error]) ([]turtle.Token, error) {
	var tokens []turtle.Token
	for token, err := range seq {
		if err != nil {
			return tokens, err
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

func tok(line, column int, tt turtle.TokenType, value string) turtle.Token {
	return turtle.Token{
		Pos:   turtle.Pos{Line: line, Column: column},
		Type:  tt,
		Value: []byte(value),
	}
}

func equalTokens(got, want []turtle.Token) bool {
	return slices.EqualFunc(got, want, func(a, b turtle.Token) bool {
		return a.Pos == b.Pos && a.Type == b.Type && string(a.Value) == string(b.Value)
	})
}

func run(t *testing.T, src string, want []turtle.Token) {
	t.Helper()

	got, err := collect(turtle.Tokenize(strings.NewReader(src)))
	if err != nil {
		t.Fatalf("Tokenize() = %v, want nil", err)
	}
	if !equalTokens(got, want) {
		t.Errorf("Tokenize() = %v, want %v", got, want)
	}
}

func TestTokenize(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []turtle.Token
	}{
		{
			name:     "empty input",
			src:      "",
			expected: nil,
		},
		{
			name:     "only white space, line endings included",
			src:      " \t\n\r\n  ",
			expected: nil,
		},
		{
			name: "a triple of iris",
			src:  `<http://e/s> <http://e/p> <http://e/o> .`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenIRIRef, "http://e/s"),
				tok(1, 14, turtle.TokenIRIRef, "http://e/p"),
				tok(1, 27, turtle.TokenIRIRef, "http://e/o"),
				tok(1, 40, turtle.TokenDot, "."),
			},
		},
		{
			name: "a line ending is white space, not a terminal",
			src:  "<http://e/s>\n<http://e/p>\n<http://e/o> .",
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenIRIRef, "http://e/s"),
				tok(2, 1, turtle.TokenIRIRef, "http://e/p"),
				tok(3, 1, turtle.TokenIRIRef, "http://e/o"),
				tok(3, 14, turtle.TokenDot, "."),
			},
		},
		{
			name: "the punctuation of a predicate object list",
			src:  `; , ( ) ] ^^`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenSemicolon, ";"),
				tok(1, 3, turtle.TokenComma, ","),
				tok(1, 5, turtle.TokenOpenParen, "("),
				tok(1, 7, turtle.TokenCloseParen, ")"),
				tok(1, 9, turtle.TokenCloseBracket, "]"),
				tok(1, 11, turtle.TokenDatatypeMarker, "^^"),
			},
		},
		{
			name: "the a keyword",
			src:  `<http://e/s> a <http://e/C> .`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenIRIRef, "http://e/s"),
				tok(1, 14, turtle.TokenA, "a"),
				tok(1, 16, turtle.TokenIRIRef, "http://e/C"),
				tok(1, 29, turtle.TokenDot, "."),
			},
		},
		{
			name: "a blank node label",
			src:  `_:b0 a _:b1 .`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenBlankNodeLabel, "b0"),
				tok(1, 6, turtle.TokenA, "a"),
				tok(1, 8, turtle.TokenBlankNodeLabel, "b1"),
				tok(1, 13, turtle.TokenDot, "."),
			},
		},
		{
			name: "a comment is a token",
			src:  "# a note\n<http://e/s> .",
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenComment, "# a note"),
				tok(2, 1, turtle.TokenIRIRef, "http://e/s"),
				tok(2, 14, turtle.TokenDot, "."),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
	}
}

// TestTokenizePrefixedNames covers PNAME_NS and PNAME_LN, which is where most
// of what Turtle adds to N-Triples lives.
func TestTokenizePrefixedNames(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []turtle.Token
	}{
		{
			name:     "a namespace with no local name",
			src:      `foaf:`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenPNameNS, "foaf")},
		},
		{
			name:     "the empty prefix",
			src:      `:`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenPNameNS, "")},
		},
		{
			name:     "a prefixed name",
			src:      `foaf:name`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenPNameLN, "foaf:name")},
		},
		{
			name:     "a local name on the empty prefix",
			src:      `:name`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenPNameLN, ":name")},
		},
		{
			name:     "a local name may begin with a digit",
			src:      `ex:0123`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenPNameLN, "ex:0123")},
		},
		{
			name:     "a local name may hold a colon",
			src:      `ex:a:b:c`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenPNameLN, "ex:a:b:c")},
		},
		{
			name:     "a prefix may hold a dot",
			src:      `a.b:c`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenPNameLN, "a.b:c")},
		},
		{
			name:     "a local name may hold a dot",
			src:      `ex:a.b`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenPNameLN, "ex:a.b")},
		},
		{
			name: "a trailing dot ends the statement instead",
			src:  `ex:name.`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenPNameLN, "ex:name"),
				tok(1, 8, turtle.TokenDot, "."),
			},
		},
		{
			name: "a namespace followed by a dot",
			src:  `ex: .`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenPNameNS, "ex"),
				tok(1, 5, turtle.TokenDot, "."),
			},
		},
		{
			name:     "a percent escape is kept as written",
			src:      `ex:a%20b`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenPNameLN, "ex:a%20b")},
		},
		{
			name:     "every character PN_LOCAL_ESC allows",
			src:      `ex:\_\~\.\-\!\$\&\'\(\)\*\+\,\;\=\/\?\#\@\%`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenPNameLN, `ex:_~.-!$&'()*+,;=/?#@%`)},
		},
		{
			name: "an escaped dot is part of the name, not the end of the statement",
			src:  `ex:a\.b .`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenPNameLN, "ex:a.b"),
				tok(1, 9, turtle.TokenDot, "."),
			},
		},
		{
			name:     "a name may hold non ascii letters",
			src:      `ex:café`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenPNameLN, "ex:café")},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
	}
}

// TestTokenizeStrings covers all four quoting forms.
func TestTokenizeStrings(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []turtle.Token
	}{
		{
			name:     "a double quoted string",
			src:      `"text"`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenString, "text")},
		},
		{
			name:     "a single quoted string",
			src:      `'text'`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenString, "text")},
		},
		{
			name:     "an empty double quoted string",
			src:      `""`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenString, "")},
		},
		{
			name:     "an empty single quoted string",
			src:      `''`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenString, "")},
		},
		{
			name:     "a long double quoted string",
			src:      `"""text"""`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenString, "text")},
		},
		{
			name:     "a long single quoted string",
			src:      `'''text'''`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenString, "text")},
		},
		{
			name:     "an empty long string",
			src:      `""""""`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenString, "")},
		},
		{
			name:     "a single quote inside a double quoted string",
			src:      `"it's"`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenString, "it's")},
		},
		{
			name:     "a double quote inside a single quoted string",
			src:      `'say "hi"'`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenString, `say "hi"`)},
		},
		{
			name:     "one and two quotes inside a long string",
			src:      `"""a "b" c ""d"""`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenString, `a "b" c ""d`)},
		},
		{
			name:     "escapes are decoded",
			src:      `"a\tb\nc\"d\\e"`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenString, "a\tb\nc\"d\\e")},
		},
		{
			name:     "a UCHAR is decoded",
			src:      "\"caf\\u00E9\"",
			expected: []turtle.Token{tok(1, 1, turtle.TokenString, "café")},
		},
		{
			name:     "a long string may hold a raw line ending",
			src:      "\"\"\"one\ntwo\"\"\"",
			expected: []turtle.Token{tok(1, 1, turtle.TokenString, "one\ntwo")},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
	}
}

// TestTokenizeLongStringPositions covers the criterion that a string spanning
// lines leaves every token after it correctly placed.
func TestTokenizeLongStringPositions(t *testing.T) {
	src := "<http://e/s> <http://e/p> \"\"\"one\ntwo\nthree\"\"\" .\n<http://e/x> ."

	run(t, src, []turtle.Token{
		tok(1, 1, turtle.TokenIRIRef, "http://e/s"),
		tok(1, 14, turtle.TokenIRIRef, "http://e/p"),
		tok(1, 27, turtle.TokenString, "one\ntwo\nthree"),
		// The string ends on line 3: "three" is 5 characters from column 1,
		// then three closing quotes, so the dot is at column 10.
		tok(3, 10, turtle.TokenDot, "."),
		tok(4, 1, turtle.TokenIRIRef, "http://e/x"),
		tok(4, 14, turtle.TokenDot, "."),
	})
}

func TestTokenizeNumbers(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []turtle.Token
	}{
		{
			name:     "an integer",
			src:      `42`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenInteger, "42")},
		},
		{
			name:     "a signed integer",
			src:      `-42`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenInteger, "-42")},
		},
		{
			name:     "a positive integer",
			src:      `+7`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenInteger, "+7")},
		},
		{
			name:     "a decimal",
			src:      `3.14`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenDecimal, "3.14")},
		},
		{
			name:     "a decimal with no integer part",
			src:      `.5`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenDecimal, ".5")},
		},
		{
			name:     "a signed decimal with no integer part",
			src:      `-.5`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenDecimal, "-.5")},
		},
		{
			name:     "a double",
			src:      `1e6`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenDouble, "1e6")},
		},
		{
			name:     "a double with a capital exponent and a sign",
			src:      `-1.0E-3`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenDouble, "-1.0E-3")},
		},
		{
			name:     "a double with no digits after the point",
			src:      `1.e6`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenDouble, "1.e6")},
		},
		{
			name: "an integer followed by the dot that ends the statement",
			src:  `42.`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenInteger, "42"),
				tok(1, 3, turtle.TokenDot, "."),
			},
		},
		{
			name: "a decimal followed by the dot that ends the statement",
			src:  `3.14 .`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenDecimal, "3.14"),
				tok(1, 6, turtle.TokenDot, "."),
			},
		},
		{
			name: "a number in a statement",
			src:  `<http://e/s> <http://e/p> 42 .`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenIRIRef, "http://e/s"),
				tok(1, 14, turtle.TokenIRIRef, "http://e/p"),
				tok(1, 27, turtle.TokenInteger, "42"),
				tok(1, 30, turtle.TokenDot, "."),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
	}
}

func TestTokenizeDirectives(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []turtle.Token
	}{
		{
			name: "an at prefix directive",
			src:  `@prefix ex: <http://e/> .`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenPrefix, "@prefix"),
				tok(1, 9, turtle.TokenPNameNS, "ex"),
				tok(1, 13, turtle.TokenIRIRef, "http://e/"),
				tok(1, 25, turtle.TokenDot, "."),
			},
		},
		{
			name: "an at base directive",
			src:  `@base <http://e/> .`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenBase, "@base"),
				tok(1, 7, turtle.TokenIRIRef, "http://e/"),
				tok(1, 19, turtle.TokenDot, "."),
			},
		},
		{
			name: "a SPARQL style PREFIX, which takes no dot",
			src:  `PREFIX ex: <http://e/>`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenPrefix, "PREFIX"),
				tok(1, 8, turtle.TokenPNameNS, "ex"),
				tok(1, 12, turtle.TokenIRIRef, "http://e/"),
			},
		},
		{
			name: "a SPARQL style BASE",
			src:  `BASE <http://e/>`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenBase, "BASE"),
				tok(1, 6, turtle.TokenIRIRef, "http://e/"),
			},
		},
		{
			name: "the SPARQL forms are case insensitive",
			src:  `pReFiX ex: <http://e/> BaSe <http://e/>`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenPrefix, "pReFiX"),
				tok(1, 8, turtle.TokenPNameNS, "ex"),
				tok(1, 12, turtle.TokenIRIRef, "http://e/"),
				tok(1, 24, turtle.TokenBase, "BaSe"),
				tok(1, 29, turtle.TokenIRIRef, "http://e/"),
			},
		},
		{
			name: "a language tag",
			src:  `"text"@en-GB`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenString, "text"),
				tok(1, 7, turtle.TokenLangTag, "en-GB"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
	}
}

func TestTokenizeAnon(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []turtle.Token
	}{
		{
			name:     "an anon with nothing between the brackets",
			src:      `[]`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenAnon, "[]")},
		},
		{
			name:     "an anon with white space between them",
			src:      "[  \n ]",
			expected: []turtle.Token{tok(1, 1, turtle.TokenAnon, "[]")},
		},
		{
			name: "a bracket that opens a property list",
			src:  `[ a <http://e/C> ]`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenOpenBracket, "["),
				tok(1, 3, turtle.TokenA, "a"),
				tok(1, 5, turtle.TokenIRIRef, "http://e/C"),
				tok(1, 18, turtle.TokenCloseBracket, "]"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
	}
}

// TestPNCharsBoundaries checks each Unicode range the grammar names, at both
// ends. A range written down one code point out is the kind of mistake that
// only shows up on a document nobody has yet tried to read.
func TestPNCharsBoundaries(t *testing.T) {
	// PN_CHARS_BASE, which is what a prefix may begin with.
	base := []struct {
		lo, hi rune
	}{
		{'A', 'Z'}, {'a', 'z'},
		{0x00C0, 0x00D6}, {0x00D8, 0x00F6}, {0x00F8, 0x02FF},
		{0x0370, 0x037D}, {0x037F, 0x1FFF},
		{0x200C, 0x200D}, {0x2070, 0x218F}, {0x2C00, 0x2FEF},
		{0x3001, 0xD7FF}, {0xF900, 0xFDCF}, {0xFDF0, 0xFFFD},
		{0x10000, 0xEFFFF},
	}

	for _, r := range base {
		for _, c := range []rune{r.lo, r.hi} {
			name := string(c) + ":"

			got, err := collect(turtle.Tokenize(strings.NewReader(name)))
			if err != nil {
				t.Errorf("U+%04X is a PN_CHARS_BASE but Tokenize(%q) = %v", c, name, err)
				continue
			}
			want := []turtle.Token{tok(1, 1, turtle.TokenPNameNS, string(c))}
			if !equalTokens(got, want) {
				t.Errorf("U+%04X: Tokenize(%q) = %v, want %v", c, name, got, want)
			}
		}
	}

	// The gaps between those ranges are not PN_CHARS_BASE, so a prefix cannot
	// begin with one.
	for _, c := range []rune{0x00D7, 0x00F7, 0x0300, 0x037E, 0x2000, 0x200B, 0x200E, 0x2FF0} {
		if _, err := collect(turtle.Tokenize(strings.NewReader(string(c) + ":"))); err == nil {
			t.Errorf("U+%04X is not a PN_CHARS_BASE but began a name without error", c)
		}
	}

	// PN_CHARS adds these, which may appear in a name but not begin one.
	for _, c := range []rune{'-', '0', '9', 0x00B7, 0x0300, 0x036F, 0x203F, 0x2040} {
		src := "a" + string(c) + ":"

		got, err := collect(turtle.Tokenize(strings.NewReader(src)))
		if err != nil {
			t.Errorf("U+%04X is a PN_CHARS but Tokenize(%q) = %v", c, src, err)
			continue
		}
		want := []turtle.Token{tok(1, 1, turtle.TokenPNameNS, "a"+string(c))}
		if !equalTokens(got, want) {
			t.Errorf("U+%04X: Tokenize(%q) = %v, want %v", c, src, got, want)
		}
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
			src:         `$`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 1}, R: '$'},
		},
		{
			name:        "a bare name that is no keyword",
			src:         `notAKeyword`,
			expectedErr: turtle.UnexpectedNameError{Pos: turtle.Pos{Line: 1, Column: 1}, Name: "notAKeyword"},
		},
		{
			name:        "an unterminated short string",
			src:         `"oops`,
			expectedErr: turtle.UnterminatedStringError{Pos: turtle.Pos{Line: 1, Column: 1}},
		},
		{
			name:        "a short string broken by a line ending",
			src:         "\"oops\nmore\"",
			expectedErr: turtle.UnterminatedStringError{Pos: turtle.Pos{Line: 1, Column: 1}},
		},
		{
			name:        "an unterminated long string",
			src:         `"""oops`,
			expectedErr: turtle.UnterminatedStringError{Pos: turtle.Pos{Line: 1, Column: 1}},
		},
		{
			name:        "an unknown escape in a local name",
			src:         `ex:a\qb`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 6}, R: 'q'},
		},
		{
			name:        "a bad hex digit in a percent escape",
			src:         `ex:a%2Zb`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 7}, R: 'Z'},
		},
		{
			name:        "a sign with no number after it",
			src:         `+ `,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 2}, R: ' '},
		},
		{
			name:        "an unterminated iri",
			src:         "<http://e/s\n",
			expectedErr: turtle.UnterminatedIRIRefError{Pos: turtle.Pos{Line: 1, Column: 1}},
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

// TestTokenizeRejectsRDF12 covers the lexemes RDF 1.2 adds. This is the 1.1
// package, so each has to be refused.
func TestTokenizeRejectsRDF12(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name:        "the triple term opener",
			src:         `<<( <http://e/s> <http://e/p> <http://e/o> )>>`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 2}, R: '<'},
		},
		{
			name:        "a left to right base direction",
			src:         `"text"@en--ltr`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 11}, R: '-'},
		},
		{
			name:        "a right to left base direction",
			src:         `"نص"@ar--rtl`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 9}, R: '-'},
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

// TestTokenizeDocument runs a document using most of the grammar at once, to
// check the rules hand over to each other correctly.
func TestTokenizeDocument(t *testing.T) {
	src := `@prefix ex: <http://e/> .

ex:alice a ex:Person ;
	ex:age 42 ;
	ex:name "Alice"@en ;
	ex:height 1.75 ;
	ex:knows [ ex:name 'Bob' ], ex:carol ;
	ex:note """long
text""" .
`

	got, err := collect(turtle.Tokenize(strings.NewReader(src)))
	if err != nil {
		t.Fatalf("Tokenize() = %v, want nil", err)
	}

	want := []turtle.TokenType{
		turtle.TokenPrefix, turtle.TokenPNameNS, turtle.TokenIRIRef, turtle.TokenDot,
		turtle.TokenPNameLN, turtle.TokenA, turtle.TokenPNameLN, turtle.TokenSemicolon,
		turtle.TokenPNameLN, turtle.TokenInteger, turtle.TokenSemicolon,
		turtle.TokenPNameLN, turtle.TokenString, turtle.TokenLangTag, turtle.TokenSemicolon,
		turtle.TokenPNameLN, turtle.TokenDecimal, turtle.TokenSemicolon,
		turtle.TokenPNameLN, turtle.TokenOpenBracket, turtle.TokenPNameLN, turtle.TokenString,
		turtle.TokenCloseBracket, turtle.TokenComma, turtle.TokenPNameLN, turtle.TokenSemicolon,
		turtle.TokenPNameLN, turtle.TokenString, turtle.TokenDot,
	}

	var kinds []turtle.TokenType
	for _, token := range got {
		kinds = append(kinds, token.Type)
	}
	if !slices.Equal(kinds, want) {
		t.Errorf("Tokenize() gave %v,\nwant %v", kinds, want)
	}
}

// TestTokenizeBooleans covers BooleanLiteral, which the grammar makes a
// keyword rather than a name and which is case-sensitive where the
// SPARQL-style directives are not.
func TestTokenizeBooleans(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []turtle.Token
	}{
		{
			name:     "true",
			src:      `true`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenBoolean, "true")},
		},
		{
			name:     "false",
			src:      `false`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenBoolean, "false")},
		},
		{
			name: "a boolean object",
			src:  `<http://e/s> <http://e/p> true .`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenIRIRef, "http://e/s"),
				tok(1, 14, turtle.TokenIRIRef, "http://e/p"),
				tok(1, 27, turtle.TokenBoolean, "true"),
				tok(1, 32, turtle.TokenDot, "."),
			},
		},
		{
			name:     "a prefixed name may still be called true",
			src:      `ex:true`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenPNameLN, "ex:true")},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
	}

	t.Run("the spelling is case sensitive", func(t *testing.T) {
		_, err := collect(turtle.Tokenize(strings.NewReader(`TRUE`)))
		want := turtle.UnexpectedNameError{Pos: turtle.Pos{Line: 1, Column: 1}, Name: "TRUE"}
		if err != want {
			t.Errorf("Tokenize() error = %v, want %v", err, want)
		}
	})
}
