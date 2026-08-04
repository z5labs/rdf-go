package turtle_test

import (
	"errors"
	"iter"
	"slices"
	"strings"
	"testing"

	turtle11 "github.com/z5labs/rdf-go/rdf11/turtle"
	"github.com/z5labs/rdf-go/rdf12/turtle"
)

// collect drains a token sequence, returning the tokens yielded before any
// error and the error that stopped it.
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

// tok builds an expected token, keeping the tables readable.
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

// TestTokenizeAmbiguousPrefixes is the heart of this tokenizer: every place
// RDF 1.2 gives two terminals the same opening characters, and every place a
// longer match fails and the input has to be left where the shorter one needs
// it.
//
// The three ambiguities are '<', which opens an IRIREF, a reified triple and a
// triple term; ')', which closes a collection and begins the ")>>" of a triple
// term; and '{', which opens an annotation block here and a graph block in
// TriG. Each case below is paired with the one where the lookahead comes back
// empty-handed, since a rule that consumed what it looked at would pass the
// first and fail the second.
func TestTokenizeAmbiguousPrefixes(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []turtle.Token
	}{
		{
			name:     "one angle bracket opens an iri",
			src:      `<http://e/s>`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenIRIRef, "http://e/s")},
		},
		{
			name:     "two open a reified triple",
			src:      `<<`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenReifiedTripleOpen, "<<")},
		},
		{
			name:     "two and a parenthesis open a triple term",
			src:      `<<(`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenTripleTermOpen, "<<(")},
		},
		{
			name: "a third angle bracket is the subject's, not the delimiter's",
			src:  `<<<http://e/s>`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenReifiedTripleOpen, "<<"),
				tok(1, 3, turtle.TokenIRIRef, "http://e/s"),
			},
		},
		{
			name: "a blank node may follow the opener with no space",
			src:  `<<_:b0`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenReifiedTripleOpen, "<<"),
				tok(1, 3, turtle.TokenBlankNodeLabel, "b0"),
			},
		},
		{
			name: "a parenthesis one character later opens a collection",
			src:  `<< (`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenReifiedTripleOpen, "<<"),
				tok(1, 4, turtle.TokenOpenParen, "("),
			},
		},
		{
			name: "backing up over a line ending counts it once",
			src:  "<<\r\n(",
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenReifiedTripleOpen, "<<"),
				tok(2, 1, turtle.TokenOpenParen, "("),
			},
		},
		{
			name:     "a parenthesis and two angle brackets close a triple term",
			src:      `)>>`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenTripleTermClose, ")>>")},
		},
		{
			name:     "a parenthesis alone closes a collection",
			src:      `)`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenCloseParen, ")")},
		},
		{
			name: "what follows a collection is put back",
			src:  `( ).`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenOpenParen, "("),
				tok(1, 3, turtle.TokenCloseParen, ")"),
				tok(1, 4, turtle.TokenDot, "."),
			},
		},
		{
			name: "an empty collection inside a triple term",
			src:  `<<()`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenTripleTermOpen, "<<("),
				tok(1, 4, turtle.TokenCloseParen, ")"),
			},
		},
		{
			name:     "a brace and a bar open an annotation",
			src:      `{|`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenAnnotationOpen, "{|")},
		},
		{
			name:     "a bar and a brace close one",
			src:      `|}`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenAnnotationClose, "|}")},
		},
		{
			name:     "a tilde stands alone",
			src:      `~`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenReifier, "~")},
		},
		{
			name:     "two angle brackets close a reified triple",
			src:      `>>`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenReifiedTripleClose, ">>")},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
	}
}

// TestTokenizeReifiedTriples covers the brackets RDF 1.2 puts around a triple
// written where a term is wanted, and the '~' that names it.
//
//	reifiedTriple ::= '<<' rtSubject verb rtObject reifier? '>>'
//	reifier       ::= '~' (iri | BlankNode)?
func TestTokenizeReifiedTriples(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []turtle.Token
	}{
		{
			name: "a reified triple as a subject",
			src:  `<< ex:s ex:p ex:o >> ex:q ex:r .`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenReifiedTripleOpen, "<<"),
				tok(1, 4, turtle.TokenPNameLN, "ex:s"),
				tok(1, 9, turtle.TokenPNameLN, "ex:p"),
				tok(1, 14, turtle.TokenPNameLN, "ex:o"),
				tok(1, 19, turtle.TokenReifiedTripleClose, ">>"),
				tok(1, 22, turtle.TokenPNameLN, "ex:q"),
				tok(1, 27, turtle.TokenPNameLN, "ex:r"),
				tok(1, 32, turtle.TokenDot, "."),
			},
		},
		{
			name: "a reifier naming the triple",
			src:  `<< ex:s ex:p ex:o ~ ex:id >> .`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenReifiedTripleOpen, "<<"),
				tok(1, 4, turtle.TokenPNameLN, "ex:s"),
				tok(1, 9, turtle.TokenPNameLN, "ex:p"),
				tok(1, 14, turtle.TokenPNameLN, "ex:o"),
				tok(1, 19, turtle.TokenReifier, "~"),
				tok(1, 21, turtle.TokenPNameLN, "ex:id"),
				tok(1, 27, turtle.TokenReifiedTripleClose, ">>"),
				tok(1, 30, turtle.TokenDot, "."),
			},
		},
		{
			name: "a reifier naming nothing",
			src:  `<< ex:s ex:p ex:o ~ >> .`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenReifiedTripleOpen, "<<"),
				tok(1, 4, turtle.TokenPNameLN, "ex:s"),
				tok(1, 9, turtle.TokenPNameLN, "ex:p"),
				tok(1, 14, turtle.TokenPNameLN, "ex:o"),
				tok(1, 19, turtle.TokenReifier, "~"),
				tok(1, 21, turtle.TokenReifiedTripleClose, ">>"),
				tok(1, 24, turtle.TokenDot, "."),
			},
		},
		{
			name: "a reified triple nested in another",
			src:  `<<<<ex:s ex:p ex:o>> ex:q ex:r>>`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenReifiedTripleOpen, "<<"),
				tok(1, 3, turtle.TokenReifiedTripleOpen, "<<"),
				tok(1, 5, turtle.TokenPNameLN, "ex:s"),
				tok(1, 10, turtle.TokenPNameLN, "ex:p"),
				tok(1, 15, turtle.TokenPNameLN, "ex:o"),
				tok(1, 19, turtle.TokenReifiedTripleClose, ">>"),
				tok(1, 22, turtle.TokenPNameLN, "ex:q"),
				tok(1, 27, turtle.TokenPNameLN, "ex:r"),
				tok(1, 31, turtle.TokenReifiedTripleClose, ">>"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
	}
}

// TestTokenizeTripleTerms covers the brackets around a triple standing as the
// object of another.
//
//	tripleTerm ::= '<<(' ttSubject verb ttObject ')>>'
func TestTokenizeTripleTerms(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []turtle.Token
	}{
		{
			name: "a triple term as an object",
			src:  `ex:s ex:p <<( ex:a ex:b ex:c )>> .`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenPNameLN, "ex:s"),
				tok(1, 6, turtle.TokenPNameLN, "ex:p"),
				tok(1, 11, turtle.TokenTripleTermOpen, "<<("),
				tok(1, 15, turtle.TokenPNameLN, "ex:a"),
				tok(1, 20, turtle.TokenPNameLN, "ex:b"),
				tok(1, 25, turtle.TokenPNameLN, "ex:c"),
				tok(1, 30, turtle.TokenTripleTermClose, ")>>"),
				tok(1, 34, turtle.TokenDot, "."),
			},
		},
		{
			name: "written without spaces around an iri",
			src:  `<<(<http://e/a> <http://e/b> "c")>>`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenTripleTermOpen, "<<("),
				tok(1, 4, turtle.TokenIRIRef, "http://e/a"),
				tok(1, 17, turtle.TokenIRIRef, "http://e/b"),
				tok(1, 30, turtle.TokenString, "c"),
				tok(1, 33, turtle.TokenTripleTermClose, ")>>"),
			},
		},
		{
			name: "a triple term spanning lines",
			src:  "<<(\n\tex:a ex:b ex:c\n)>>",
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenTripleTermOpen, "<<("),
				tok(2, 2, turtle.TokenPNameLN, "ex:a"),
				tok(2, 7, turtle.TokenPNameLN, "ex:b"),
				tok(2, 12, turtle.TokenPNameLN, "ex:c"),
				tok(3, 1, turtle.TokenTripleTermClose, ")>>"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
	}
}

// TestTokenizeAnnotations covers the block a document writes after a triple to
// say something about it.
//
//	annotation      ::= (reifier | annotationBlock)*
//	annotationBlock ::= '{|' predicateObjectList '|}'
func TestTokenizeAnnotations(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []turtle.Token
	}{
		{
			name: "an annotation block after a triple",
			src:  `ex:s ex:p ex:o {| ex:q ex:r |} .`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenPNameLN, "ex:s"),
				tok(1, 6, turtle.TokenPNameLN, "ex:p"),
				tok(1, 11, turtle.TokenPNameLN, "ex:o"),
				tok(1, 16, turtle.TokenAnnotationOpen, "{|"),
				tok(1, 19, turtle.TokenPNameLN, "ex:q"),
				tok(1, 24, turtle.TokenPNameLN, "ex:r"),
				tok(1, 29, turtle.TokenAnnotationClose, "|}"),
				tok(1, 32, turtle.TokenDot, "."),
			},
		},
		{
			name: "a reifier and a block together",
			src:  `ex:s ex:p ex:o ~ex:id {|ex:q ex:r|} .`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenPNameLN, "ex:s"),
				tok(1, 6, turtle.TokenPNameLN, "ex:p"),
				tok(1, 11, turtle.TokenPNameLN, "ex:o"),
				tok(1, 16, turtle.TokenReifier, "~"),
				tok(1, 17, turtle.TokenPNameLN, "ex:id"),
				tok(1, 23, turtle.TokenAnnotationOpen, "{|"),
				tok(1, 25, turtle.TokenPNameLN, "ex:q"),
				tok(1, 30, turtle.TokenPNameLN, "ex:r"),
				tok(1, 34, turtle.TokenAnnotationClose, "|}"),
				tok(1, 37, turtle.TokenDot, "."),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
	}
}

// TestTokenizeVersionDirective covers both spellings of the directive RDF 1.2
// adds.
//
//	version       ::= '@version' VersionSpecifier '.'
//	sparqlVersion ::= "VERSION" VersionSpecifier
//
// The SPARQL-style keyword is case-insensitive, as "PREFIX" and "BASE" already
// were, and the '@' form is not, as '@prefix' and '@base' already were not.
func TestTokenizeVersionDirective(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []turtle.Token
	}{
		{
			name: "the sparql style keyword",
			src:  `VERSION "1.2"`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenVersion, "VERSION"),
				tok(1, 9, turtle.TokenString, "1.2"),
			},
		},
		{
			name: "spelled in lower case",
			src:  `version "1.2"`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenVersion, "version"),
				tok(1, 9, turtle.TokenString, "1.2"),
			},
		},
		{
			name: "spelled in mixed case, with a single quoted specifier",
			src:  `Version '1.2'`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenVersion, "Version"),
				tok(1, 9, turtle.TokenString, "1.2"),
			},
		},
		{
			name: "the at style directive, which ends in a dot",
			src:  `@version "1.2" .`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenVersion, "@version"),
				tok(1, 10, turtle.TokenString, "1.2"),
				tok(1, 16, turtle.TokenDot, "."),
			},
		},
		{
			name:     "the at form is case sensitive, so this is a language tag",
			src:      `@VERSION`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenLangDir, "VERSION")},
		},
		{
			name: "a language tag of exactly version is read as the directive",
			src:  `"x"@version`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenString, "x"),
				tok(1, 4, turtle.TokenVersion, "@version"),
			},
		},
		{
			name:     "a prefixed name may still be called version",
			src:      `ex:version`,
			expected: []turtle.Token{tok(1, 1, turtle.TokenPNameLN, "ex:version")},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
	}
}

// TestTokenizeVersionSpecifier covers the one place Turtle asks for a string
// and admits only two of the four forms it writes strings in.
//
//	VersionSpecifier ::= STRING_LITERAL_QUOTE | STRING_LITERAL_SINGLE_QUOTE
//
// A long string is a string everywhere else and is not a version specifier, so
// the third quote — the character the directive may not write — is where it is
// reported.
func TestTokenizeVersionSpecifier(t *testing.T) {
	t.Run("the short forms are read as strings", func(t *testing.T) {
		testCases := []struct {
			name     string
			src      string
			expected []turtle.Token
		}{
			{
				name: "an empty specifier is two quotes and no third",
				src:  `VERSION ""`,
				expected: []turtle.Token{
					tok(1, 1, turtle.TokenVersion, "VERSION"),
					tok(1, 9, turtle.TokenString, ""),
				},
			},
			{
				name: "a specifier is read as the string it is",
				src:  `VERSION "1.2"`,
				expected: []turtle.Token{
					tok(1, 1, turtle.TokenVersion, "VERSION"),
					tok(1, 9, turtle.TokenString, "1.2"),
				},
			},
			{
				// A specifier is a STRING_LITERAL_QUOTE and so admits every
				// escape one does, decoded here as anywhere else.
				name: "a specifier holding an escape is decoded",
				src:  "VERSION \"1\\u002E2\"",
				expected: []turtle.Token{
					tok(1, 1, turtle.TokenVersion, "VERSION"),
					tok(1, 9, turtle.TokenString, "1.2"),
				},
			},
			{
				name: "a comment may stand between the keyword and the specifier",
				src:  "@version # which version\n \"1.2\" .",
				expected: []turtle.Token{
					tok(1, 1, turtle.TokenVersion, "@version"),
					tok(1, 10, turtle.TokenComment, "# which version"),
					tok(2, 2, turtle.TokenString, "1.2"),
					tok(2, 8, turtle.TokenDot, "."),
				},
			},
			{
				name: "what follows the keyword is only special once",
				src:  `VERSION "1.2" :s :p """long""" .`,
				expected: []turtle.Token{
					tok(1, 1, turtle.TokenVersion, "VERSION"),
					tok(1, 9, turtle.TokenString, "1.2"),
					tok(1, 15, turtle.TokenPNameLN, ":s"),
					tok(1, 18, turtle.TokenPNameLN, ":p"),
					tok(1, 21, turtle.TokenString, "long"),
					tok(1, 32, turtle.TokenDot, "."),
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				run(t, tc.src, tc.expected)
			})
		}
	})

	t.Run("the long forms are refused at the third quote", func(t *testing.T) {
		testCases := []struct {
			name        string
			src         string
			expectedErr error
		}{
			{
				name:        "the sparql keyword with a long double quoted specifier",
				src:         `VERSION """1.2"""`,
				expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 11}, R: '"'},
			},
			{
				name:        "the sparql keyword with a long single quoted specifier",
				src:         `VERSION '''1.2'''`,
				expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 11}, R: '\''},
			},
			{
				name:        "the at directive with a long double quoted specifier",
				src:         `@version """1.2""" .`,
				expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 12}, R: '"'},
			},
			{
				name:        "the at directive with a long single quoted specifier",
				src:         `@version '''1.2''' .`,
				expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 12}, R: '\''},
			},
			{
				name:        "a comment does not excuse a long specifier",
				src:         "VERSION # note\n\"\"\"1.2\"\"\"",
				expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 2, Column: 3}, R: '"'},
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
	})

	t.Run("anything else is left to the parser", func(t *testing.T) {
		// The keyword and the number are both tokens the tokenizer has no
		// quarrel with; that a number is no version specifier is the parser's
		// to say, and it can name the token rather than the character.
		run(t, `VERSION 1.2`, []turtle.Token{
			tok(1, 1, turtle.TokenVersion, "VERSION"),
			tok(1, 9, turtle.TokenDecimal, "1.2"),
		})
	})
}

// TestTokenizeLangDir covers LANG_DIR, which is the RDF 1.1 LANGTAG plus an
// initial base direction after a second hyphen.
//
//	LANG_DIR ::= '@' [a-zA-Z]+ ('-' [a-zA-Z0-9]+)* ('--' [a-zA-Z]+)?
func TestTokenizeLangDir(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []turtle.Token
	}{
		{
			name: "a left to right base direction",
			src:  `"text"@en--ltr`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenString, "text"),
				tok(1, 7, turtle.TokenLangDir, "en--ltr"),
			},
		},
		{
			name: "a right to left base direction",
			src:  `"نص"@ar--rtl`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenString, "نص"),
				tok(1, 5, turtle.TokenLangDir, "ar--rtl"),
			},
		},
		{
			name: "subtags before the direction",
			src:  `"x"@en-GB--ltr`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenString, "x"),
				tok(1, 4, turtle.TokenLangDir, "en-GB--ltr"),
			},
		},
		{
			name: "no direction at all, which is the rdf 1.1 langtag",
			src:  `"x"@zh-Hant-TW`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenString, "x"),
				tok(1, 4, turtle.TokenLangDir, "zh-Hant-TW"),
			},
		},
		{
			name: "a direction ends the terminal, so a dot after it is the statement's",
			src:  `"x"@en--ltr .`,
			expected: []turtle.Token{
				tok(1, 1, turtle.TokenString, "x"),
				tok(1, 4, turtle.TokenLangDir, "en--ltr"),
				tok(1, 13, turtle.TokenDot, "."),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			run(t, tc.src, tc.expected)
		})
	}
}

// TestTokenizeRDF12Errors covers the ways the delimiters RDF 1.2 adds can be
// written wrong, and where each is reported. A delimiter half written is a
// terminal part way through, so the input running out is not the end of the
// document.
func TestTokenizeRDF12Errors(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name:        "one angle bracket closes nothing",
			src:         `>`,
			expectedErr: turtle.UnexpectedEndOfInputError{Pos: turtle.Pos{Line: 1, Column: 2}},
		},
		{
			name:        "an angle bracket followed by something else",
			src:         `>x`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 2}, R: 'x'},
		},
		{
			name:        "a triple term closer short of its second bracket",
			src:         `)>`,
			expectedErr: turtle.UnexpectedEndOfInputError{Pos: turtle.Pos{Line: 1, Column: 3}},
		},
		{
			name:        "a triple term closer interrupted",
			src:         `)>x`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 3}, R: 'x'},
		},
		{
			name:        "a lone brace opens nothing in turtle",
			src:         `{`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 1}, R: '{'},
		},
		{
			name:        "a brace is reported at itself, not at what followed it",
			src:         `{ ex:s ex:p ex:o }`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 1}, R: '{'},
		},
		{
			name:        "a bar with nothing after it",
			src:         `|`,
			expectedErr: turtle.UnexpectedEndOfInputError{Pos: turtle.Pos{Line: 1, Column: 2}},
		},
		{
			name:        "a bar followed by something other than a brace",
			src:         `|x`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 2}, R: 'x'},
		},
		{
			name:        "a base direction with no language tag",
			src:         `"x"@--ltr`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 5}, R: '-'},
		},
		{
			name:        "a base direction with no letters",
			src:         `"x"@en--`,
			expectedErr: turtle.UnexpectedEndOfInputError{Pos: turtle.Pos{Line: 1, Column: 9}},
		},
		{
			name:        "a base direction written with a digit",
			src:         `"x"@en--1`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 9}, R: '1'},
		},
		{
			name:        "an angle bracket is still not an iri character",
			src:         `<http://e/<>`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: turtle.Pos{Line: 1, Column: 11}, R: '<'},
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

// rdf11Sources are documents written in RDF 1.1 Turtle, which RDF 1.2 Turtle
// must go on accepting unchanged.
var rdf11Sources = []string{
	"",
	" \t\n\r\n  ",
	`<http://e/s> <http://e/p> <http://e/o> .`,
	"<http://e/s>\n<http://e/p>\n<http://e/o> .",
	`@prefix ex: <http://e/> .`,
	`@base <http://e/> .`,
	`PREFIX ex: <http://e/>`,
	`base <http://e/>`,
	`ex:alice a ex:Person ; ex:age 42 .`,
	`: :p : .`,
	`ex:s ex:p "text"@en-GB .`,
	`ex:s ex:p "1"^^xsd:integer .`,
	`[ ex:p ex:o ] .`,
	`[] ex:p [  ] .`,
	`ex:s ex:p ( 1 2.0 3e4 ) .`,
	`ex:s ex:p (( ) ) .`,
	`ex:s ex:p "a", 'b', """c""", '''d''' .`,
	"ex:s ex:p \"\"\"a\nb\"\"\" .",
	`_:b0 ex:p _:b1 .`,
	`_:a.b ex:p _:b0.`,
	`ex:s ex:p -.5, +42, 1.0E-3, 1. `,
	`ex:s.t ex:p ex:o .`,
	"# a note\n<http://e/s> . # another",
	`ex:a\~b ex:p ex:a\.b .`,
	`ex:a%20b ex:p ex:o .`,
	`ex:s ex:p true, false .`,
	`ex:s ex:p "say \"hi\"" .`,
	`"\t\b\n\r\f\"\'\\"`,
	`"é\U0001F600éÉ"`,
	`<http://e/a\U0000006C>`,
	`"héllo wörld" <http://e/ünïcødé>`,
	`ex:café ex:p ex:日本語 .`,
	`@en`,
	`@zh-Hant-TW`,
	`@es-419`,
	`"text"@en`,
	`"text"^^<http://e/dt>`,
	`a`,
}

// TestTokenizeAcceptsRDF11 is one half of the promise that RDF 1.2 is a
// superset: every document RDF 1.1 accepts is tokenized here without error.
func TestTokenizeAcceptsRDF11(t *testing.T) {
	for _, src := range rdf11Sources {
		t.Run(src, func(t *testing.T) {
			if _, err := collect(turtle.Tokenize(strings.NewReader(src))); err != nil {
				t.Errorf("Tokenize() = %v, want nil", err)
			}
		})
	}
}

// TestTokenizeAgreesWithRDF11 is the other half: accepting an RDF 1.1 document
// is not enough, the tokens have to be the same ones, at the same positions,
// carrying the same values. A rule that swallowed a character or moved a
// position — the lookahead on '<' and ')' does both if it forgets to back up —
// would pass [TestTokenizeAcceptsRDF11] and be caught here.
//
// Only LANGTAG changed name, LANG_DIR being the same production with a
// direction appended, so the two token streams are compared through
// [rdf12TokenType].
func TestTokenizeAgreesWithRDF11(t *testing.T) {
	for _, src := range rdf11Sources {
		t.Run(src, func(t *testing.T) {
			var want []turtle.Token
			for token, err := range turtle11.Tokenize(strings.NewReader(src)) {
				if err != nil {
					t.Fatalf("rdf11 Tokenize() = %v, want nil", err)
				}
				want = append(want, turtle.Token{
					Pos:   turtle.Pos{Line: token.Pos.Line, Column: token.Pos.Column},
					Type:  rdf12TokenType(t, token.Type),
					Value: token.Value,
				})
			}

			got, err := collect(turtle.Tokenize(strings.NewReader(src)))
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
func rdf12TokenType(t *testing.T, tt turtle11.TokenType) turtle.TokenType {
	t.Helper()

	switch tt {
	case turtle11.TokenIRIRef:
		return turtle.TokenIRIRef
	case turtle11.TokenPNameNS:
		return turtle.TokenPNameNS
	case turtle11.TokenPNameLN:
		return turtle.TokenPNameLN
	case turtle11.TokenBlankNodeLabel:
		return turtle.TokenBlankNodeLabel
	case turtle11.TokenAnon:
		return turtle.TokenAnon
	case turtle11.TokenString:
		return turtle.TokenString
	case turtle11.TokenLangTag:
		return turtle.TokenLangDir
	case turtle11.TokenInteger:
		return turtle.TokenInteger
	case turtle11.TokenDecimal:
		return turtle.TokenDecimal
	case turtle11.TokenDouble:
		return turtle.TokenDouble
	case turtle11.TokenA:
		return turtle.TokenA
	case turtle11.TokenBoolean:
		return turtle.TokenBoolean
	case turtle11.TokenPrefix:
		return turtle.TokenPrefix
	case turtle11.TokenBase:
		return turtle.TokenBase
	case turtle11.TokenDatatypeMarker:
		return turtle.TokenDatatypeMarker
	case turtle11.TokenDot:
		return turtle.TokenDot
	case turtle11.TokenSemicolon:
		return turtle.TokenSemicolon
	case turtle11.TokenComma:
		return turtle.TokenComma
	case turtle11.TokenOpenParen:
		return turtle.TokenOpenParen
	case turtle11.TokenCloseParen:
		return turtle.TokenCloseParen
	case turtle11.TokenOpenBracket:
		return turtle.TokenOpenBracket
	case turtle11.TokenCloseBracket:
		return turtle.TokenCloseBracket
	case turtle11.TokenComment:
		return turtle.TokenComment
	default:
		t.Fatalf("no RDF 1.2 counterpart for %v", tt)
		return 0
	}
}

// TestTokenizeDocument runs a document using most of what RDF 1.2 adds at
// once, to check the rules hand over to each other correctly.
func TestTokenizeDocument(t *testing.T) {
	src := `@prefix ex: <http://e/> .
@version "1.2" .
VERSION "1.2"

<< ex:alice ex:knows ex:bob ~ ex:r1 >> ex:certainty 0.8 .

ex:alice ex:name "Alice"@en--ltr {| ex:source ex:doc |} .

ex:bob ex:said <<( ex:s ex:p "o" )>> .
`

	got, err := collect(turtle.Tokenize(strings.NewReader(src)))
	if err != nil {
		t.Fatalf("Tokenize() = %v, want nil", err)
	}

	want := []turtle.TokenType{
		turtle.TokenPrefix, turtle.TokenPNameNS, turtle.TokenIRIRef, turtle.TokenDot,
		turtle.TokenVersion, turtle.TokenString, turtle.TokenDot,
		turtle.TokenVersion, turtle.TokenString,
		turtle.TokenReifiedTripleOpen, turtle.TokenPNameLN, turtle.TokenPNameLN,
		turtle.TokenPNameLN, turtle.TokenReifier, turtle.TokenPNameLN,
		turtle.TokenReifiedTripleClose, turtle.TokenPNameLN, turtle.TokenDecimal,
		turtle.TokenDot,
		turtle.TokenPNameLN, turtle.TokenPNameLN, turtle.TokenString, turtle.TokenLangDir,
		turtle.TokenAnnotationOpen, turtle.TokenPNameLN, turtle.TokenPNameLN,
		turtle.TokenAnnotationClose, turtle.TokenDot,
		turtle.TokenPNameLN, turtle.TokenPNameLN, turtle.TokenTripleTermOpen,
		turtle.TokenPNameLN, turtle.TokenPNameLN, turtle.TokenString,
		turtle.TokenTripleTermClose, turtle.TokenDot,
	}

	var kinds []turtle.TokenType
	for _, token := range got {
		kinds = append(kinds, token.Type)
	}
	if !slices.Equal(kinds, want) {
		t.Errorf("Tokenize() gave %v,\nwant %v", kinds, want)
	}
}

// TestTokenizeStopsEarly checks that a caller who stops ranging stops the
// tokenizer with it.
func TestTokenizeStopsEarly(t *testing.T) {
	var count int
	for range turtle.Tokenize(strings.NewReader(`<< ex:s ex:p ex:o >> .`)) {
		count++
		if count == 2 {
			break
		}
	}

	if count != 2 {
		t.Errorf("yielded %d tokens after the caller stopped at 2", count)
	}
}

// TestTokenizeYieldsNothingAfterAnError checks the contract the doc comment
// states: an error ends the sequence.
func TestTokenizeYieldsNothingAfterAnError(t *testing.T) {
	var seen int
	var errs int

	for _, err := range turtle.Tokenize(strings.NewReader(`ex:s ex:p ex:o {x`)) {
		if err != nil {
			errs++
			continue
		}
		if errs > 0 {
			seen++
		}
	}

	if errs != 1 {
		t.Errorf("yielded %d errors, want 1", errs)
	}
	if seen != 0 {
		t.Errorf("yielded %d tokens after the error, want 0", seen)
	}
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

// TestTokenizeReaderError checks that a failure of the underlying reader is
// reported rather than mistaken for the end of the input. Each source below
// stops where a rule is looking ahead, which is where the new delimiters make
// the tokenizer read past the terminal it has begun.
func TestTokenizeReaderError(t *testing.T) {
	errBoom := errors.New("boom")

	sources := []string{
		"   ", `<`, `<<`, `<<(`, `)`, `>`, `{`, `|`, `~`,
		`"o"@en`, `"o"@en-`, `"o"@en--`, `VERSION`,
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

func TestTokenTypeString(t *testing.T) {
	types := map[turtle.TokenType]string{
		turtle.TokenIRIRef:             "IRIRef",
		turtle.TokenPNameNS:            "PNameNS",
		turtle.TokenPNameLN:            "PNameLN",
		turtle.TokenBlankNodeLabel:     "BlankNodeLabel",
		turtle.TokenAnon:               "Anon",
		turtle.TokenString:             "String",
		turtle.TokenLangDir:            "LangDir",
		turtle.TokenInteger:            "Integer",
		turtle.TokenDecimal:            "Decimal",
		turtle.TokenDouble:             "Double",
		turtle.TokenA:                  "A",
		turtle.TokenBoolean:            "Boolean",
		turtle.TokenPrefix:             "Prefix",
		turtle.TokenBase:               "Base",
		turtle.TokenVersion:            "Version",
		turtle.TokenDatatypeMarker:     "DatatypeMarker",
		turtle.TokenTripleTermOpen:     "TripleTermOpen",
		turtle.TokenTripleTermClose:    "TripleTermClose",
		turtle.TokenReifiedTripleOpen:  "ReifiedTripleOpen",
		turtle.TokenReifiedTripleClose: "ReifiedTripleClose",
		turtle.TokenReifier:            "Reifier",
		turtle.TokenAnnotationOpen:     "AnnotationOpen",
		turtle.TokenAnnotationClose:    "AnnotationClose",
		turtle.TokenDot:                "Dot",
		turtle.TokenSemicolon:          "Semicolon",
		turtle.TokenComma:              "Comma",
		turtle.TokenOpenParen:          "OpenParen",
		turtle.TokenCloseParen:         "CloseParen",
		turtle.TokenOpenBracket:        "OpenBracket",
		turtle.TokenCloseBracket:       "CloseBracket",
		turtle.TokenComment:            "Comment",
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

		_ = turtle.TokenType(99).String()
	})
}

func TestTokenString(t *testing.T) {
	got := tok(1, 1, turtle.TokenTripleTermOpen, "<<(").String()
	if want := "TripleTermOpen(<<()"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestPosString(t *testing.T) {
	got := turtle.Pos{Line: 3, Column: 12}.String()
	if want := "3:12"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
