package ntriples_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/z5labs/rdf-go/rdf11/ntriples"
)

func pos(line, column int) ntriples.Pos {
	return ntriples.Pos{Line: line, Column: column}
}

func iriRef(line, column int, value string) *ntriples.IRIRef {
	return &ntriples.IRIRef{Pos: pos(line, column), Value: value}
}

func blankNode(line, column int, label string) *ntriples.BlankNode {
	return &ntriples.BlankNode{Pos: pos(line, column), Label: label}
}

func TestParse(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected *ntriples.Document
	}{
		{
			name:     "an empty document",
			src:      "",
			expected: &ntriples.Document{Pos: pos(1, 1)},
		},
		{
			name:     "a document of only line endings",
			src:      "\n\n\n",
			expected: &ntriples.Document{Pos: pos(1, 1)},
		},
		{
			name: "a triple of iris",
			src:  `<http://example.com/s> <http://example.com/p> <http://example.com/o> .`,
			expected: &ntriples.Document{
				Pos: pos(1, 1),
				Triples: []*ntriples.Triple{
					{
						Pos:       pos(1, 1),
						Subject:   iriRef(1, 1, "http://example.com/s"),
						Predicate: iriRef(1, 24, "http://example.com/p"),
						Object:    iriRef(1, 47, "http://example.com/o"),
					},
				},
			},
		},
		{
			name: "a blank node subject and object",
			src:  `_:a <http://example.com/p> _:b .`,
			expected: &ntriples.Document{
				Pos: pos(1, 1),
				Triples: []*ntriples.Triple{
					{
						Pos:       pos(1, 1),
						Subject:   blankNode(1, 1, "a"),
						Predicate: iriRef(1, 5, "http://example.com/p"),
						Object:    blankNode(1, 28, "b"),
					},
				},
			},
		},
		{
			name: "a plain literal object",
			src:  `_:a <http://example.com/p> "text" .`,
			expected: &ntriples.Document{
				Pos: pos(1, 1),
				Triples: []*ntriples.Triple{
					{
						Pos:       pos(1, 1),
						Subject:   blankNode(1, 1, "a"),
						Predicate: iriRef(1, 5, "http://example.com/p"),
						Object:    &ntriples.Literal{Pos: pos(1, 28), Value: "text"},
					},
				},
			},
		},
		{
			name: "a language tagged literal",
			src:  `_:a <http://example.com/p> "text"@en-GB .`,
			expected: &ntriples.Document{
				Pos: pos(1, 1),
				Triples: []*ntriples.Triple{
					{
						Pos:       pos(1, 1),
						Subject:   blankNode(1, 1, "a"),
						Predicate: iriRef(1, 5, "http://example.com/p"),
						Object: &ntriples.Literal{
							Pos:      pos(1, 28),
							Value:    "text",
							LangPos:  pos(1, 34),
							Language: "en-GB",
						},
					},
				},
			},
		},
		{
			name: "a typed literal",
			src:  `_:a <http://example.com/p> "1"^^<http://www.w3.org/2001/XMLSchema#integer> .`,
			expected: &ntriples.Document{
				Pos: pos(1, 1),
				Triples: []*ntriples.Triple{
					{
						Pos:       pos(1, 1),
						Subject:   blankNode(1, 1, "a"),
						Predicate: iriRef(1, 5, "http://example.com/p"),
						Object: &ntriples.Literal{
							Pos:      pos(1, 28),
							Value:    "1",
							Datatype: iriRef(1, 33, "http://www.w3.org/2001/XMLSchema#integer"),
						},
					},
				},
			},
		},
		{
			name: "a literal whose escapes are decoded",
			src:  `_:a <http://example.com/p> "a\nbé" .`,
			expected: &ntriples.Document{
				Pos: pos(1, 1),
				Triples: []*ntriples.Triple{
					{
						Pos:       pos(1, 1),
						Subject:   blankNode(1, 1, "a"),
						Predicate: iriRef(1, 5, "http://example.com/p"),
						Object:    &ntriples.Literal{Pos: pos(1, 28), Value: "a\nbé"},
					},
				},
			},
		},
		{
			name: "several statements",
			src:  "_:a <http://example.com/p> _:b .\n_:b <http://example.com/q> \"v\" .\n",
			expected: &ntriples.Document{
				Pos: pos(1, 1),
				Triples: []*ntriples.Triple{
					{
						Pos:       pos(1, 1),
						Subject:   blankNode(1, 1, "a"),
						Predicate: iriRef(1, 5, "http://example.com/p"),
						Object:    blankNode(1, 28, "b"),
					},
					{
						Pos:       pos(2, 1),
						Subject:   blankNode(2, 1, "b"),
						Predicate: iriRef(2, 5, "http://example.com/q"),
						Object:    &ntriples.Literal{Pos: pos(2, 28), Value: "v"},
					},
				},
			},
		},
		{
			name: "blank lines between statements",
			src:  "_:a <http://example.com/p> _:b .\n\n\n_:c <http://example.com/q> _:d .",
			expected: &ntriples.Document{
				Pos: pos(1, 1),
				Triples: []*ntriples.Triple{
					{
						Pos:       pos(1, 1),
						Subject:   blankNode(1, 1, "a"),
						Predicate: iriRef(1, 5, "http://example.com/p"),
						Object:    blankNode(1, 28, "b"),
					},
					{
						Pos:       pos(4, 1),
						Subject:   blankNode(4, 1, "c"),
						Predicate: iriRef(4, 5, "http://example.com/q"),
						Object:    blankNode(4, 28, "d"),
					},
				},
			},
		},
		{
			name: "leading white space",
			src:  "   _:a <http://example.com/p> _:b .",
			expected: &ntriples.Document{
				Pos: pos(1, 1),
				Triples: []*ntriples.Triple{
					{
						Pos:       pos(1, 4),
						Subject:   blankNode(1, 4, "a"),
						Predicate: iriRef(1, 8, "http://example.com/p"),
						Object:    blankNode(1, 31, "b"),
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ntriples.Parse(strings.NewReader(tc.src))
			if err != nil {
				t.Fatalf("Parse() = %v, want nil", err)
			}
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("Parse() = %s, want %s", format(got), format(tc.expected))
			}
		})
	}
}

// TestParseKeepsComments covers the criterion a printer depends on: a comment
// is white space to the grammar, but losing it would mean a document could not
// be written back as it was read.
func TestParseKeepsComments(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected []*ntriples.Comment
	}{
		{
			name:     "a comment on its own line",
			src:      "# a note\n_:a <http://example.com/p> _:b .",
			expected: []*ntriples.Comment{{Pos: pos(1, 1), Text: "# a note"}},
		},
		{
			name:     "a comment trailing a statement",
			src:      "_:a <http://example.com/p> _:b . # why",
			expected: []*ntriples.Comment{{Pos: pos(1, 34), Text: "# why"}},
		},
		{
			name: "comments before, between and after",
			src:  "# first\n_:a <http://example.com/p> _:b . # trailing\n# between\n_:c <http://example.com/q> _:d .\n# last",
			expected: []*ntriples.Comment{
				{Pos: pos(1, 1), Text: "# first"},
				{Pos: pos(2, 34), Text: "# trailing"},
				{Pos: pos(3, 1), Text: "# between"},
				{Pos: pos(5, 1), Text: "# last"},
			},
		},
		{
			name:     "a document of nothing but a comment",
			src:      "# alone",
			expected: []*ntriples.Comment{{Pos: pos(1, 1), Text: "# alone"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := ntriples.Parse(strings.NewReader(tc.src))
			if err != nil {
				t.Fatalf("Parse() = %v, want nil", err)
			}
			if !reflect.DeepEqual(doc.Comments, tc.expected) {
				t.Errorf("Comments = %v, want %v", doc.Comments, tc.expected)
			}
		})
	}
}

// TestParseGrammarErrors covers the positions the grammar refuses. Each error
// has to name where the offending token was, so that a tool can point at it.
func TestParseGrammarErrors(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name: "a literal in subject position",
			src:  `"nope" <http://example.com/p> _:o .`,
			expectedErr: ntriples.UnexpectedTokenError{
				Expected: []ntriples.TokenType{ntriples.TokenIRIRef, ntriples.TokenBlankNodeLabel},
				Actual: ntriples.Token{
					Pos:   pos(1, 1),
					Type:  ntriples.TokenStringLiteral,
					Value: []byte("nope"),
				},
			},
		},
		{
			name: "a blank node in predicate position",
			src:  `_:s _:p _:o .`,
			expectedErr: ntriples.UnexpectedTokenError{
				Expected: []ntriples.TokenType{ntriples.TokenIRIRef},
				Actual: ntriples.Token{
					Pos:   pos(1, 5),
					Type:  ntriples.TokenBlankNodeLabel,
					Value: []byte("p"),
				},
			},
		},
		{
			name: "a literal in predicate position",
			src:  `_:s "p" _:o .`,
			expectedErr: ntriples.UnexpectedTokenError{
				Expected: []ntriples.TokenType{ntriples.TokenIRIRef},
				Actual: ntriples.Token{
					Pos:   pos(1, 5),
					Type:  ntriples.TokenStringLiteral,
					Value: []byte("p"),
				},
			},
		},
		{
			name: "a dot where the object should be",
			src:  `_:s <http://example.com/p> .`,
			expectedErr: ntriples.UnexpectedTokenError{
				Expected: []ntriples.TokenType{
					ntriples.TokenIRIRef,
					ntriples.TokenBlankNodeLabel,
					ntriples.TokenStringLiteral,
				},
				Actual: ntriples.Token{Pos: pos(1, 28), Type: ntriples.TokenDot, Value: []byte(".")},
			},
		},
		{
			name: "a missing trailing dot",
			src:  "_:s <http://example.com/p> _:o\n",
			expectedErr: ntriples.UnexpectedTokenError{
				Expected: []ntriples.TokenType{ntriples.TokenDot},
				Actual:   ntriples.Token{Pos: pos(1, 31), Type: ntriples.TokenEOL, Value: []byte("\n")},
			},
		},
		{
			name: "a second statement on the same line",
			src:  `_:a <http://example.com/p> _:b . _:c <http://example.com/q> _:d .`,
			expectedErr: ntriples.UnexpectedTokenError{
				Expected: []ntriples.TokenType{ntriples.TokenEOL, ntriples.TokenComment},
				Actual: ntriples.Token{
					Pos:   pos(1, 34),
					Type:  ntriples.TokenBlankNodeLabel,
					Value: []byte("c"),
				},
			},
		},
		{
			name: "a language tag where the datatype iri should be",
			src:  `_:s <http://example.com/p> "v"^^@en .`,
			expectedErr: ntriples.UnexpectedTokenError{
				Expected: []ntriples.TokenType{ntriples.TokenIRIRef},
				Actual:   ntriples.Token{Pos: pos(1, 33), Type: ntriples.TokenLangTag, Value: []byte("en")},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ntriples.Parse(strings.NewReader(tc.src))
			if !reflect.DeepEqual(err, tc.expectedErr) {
				t.Errorf("Parse() error = %#v, want %#v", err, tc.expectedErr)
			}
		})
	}
}

// TestParseEndOfTokens covers a document that stops with a statement still
// unfinished, which is a different failure from one that says something the
// grammar refuses.
func TestParseEndOfTokens(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name: "nothing after the subject",
			src:  `_:s`,
			expectedErr: ntriples.UnexpectedEndOfTokensError{
				Expected: []ntriples.TokenType{ntriples.TokenIRIRef},
				Pos:      pos(1, 1),
			},
		},
		{
			name: "nothing after the predicate",
			src:  `_:s <http://example.com/p>`,
			expectedErr: ntriples.UnexpectedEndOfTokensError{
				Expected: []ntriples.TokenType{
					ntriples.TokenIRIRef,
					ntriples.TokenBlankNodeLabel,
					ntriples.TokenStringLiteral,
				},
				Pos: pos(1, 5),
			},
		},
		{
			name: "no dot at the end of the input",
			src:  `_:s <http://example.com/p> _:o`,
			expectedErr: ntriples.UnexpectedEndOfTokensError{
				Expected: []ntriples.TokenType{ntriples.TokenDot},
				Pos:      pos(1, 28),
			},
		},
		{
			name: "nothing after the datatype marker",
			src:  `_:s <http://example.com/p> "v"^^`,
			expectedErr: ntriples.UnexpectedEndOfTokensError{
				Expected: []ntriples.TokenType{ntriples.TokenIRIRef},
				Pos:      pos(1, 31),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ntriples.Parse(strings.NewReader(tc.src))
			if !reflect.DeepEqual(err, tc.expectedErr) {
				t.Errorf("Parse() error = %#v, want %#v", err, tc.expectedErr)
			}
		})
	}
}

// TestParseReportsTokenizerErrors checks that a document the tokenizer cannot
// read fails with the tokenizer's own error, rather than being reported as
// some confusing consequence of it.
func TestParseReportsTokenizerErrors(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name:        "a character that begins no terminal",
			src:         `$ <http://example.com/p> _:o .`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: pos(1, 1), R: '$'},
		},
		{
			name:        "an unterminated iri",
			src:         "<http://example.com/s\n<http://example.com/p> _:o .",
			expectedErr: ntriples.UnterminatedIRIRefError{Pos: pos(1, 1)},
		},
		{
			name:        "a space inside an iri, which is excluded rather than missing a bracket",
			src:         `<http://example.com/a b> <http://example.com/p> _:o .`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: pos(1, 22), R: ' '},
		},
		{
			name:        "an unterminated literal",
			src:         `_:s <http://example.com/p> "oops`,
			expectedErr: ntriples.UnterminatedStringError{Pos: pos(1, 28)},
		},
		{
			name:        "an rdf 1.2 base direction",
			src:         `_:s <http://example.com/p> "v"@en--ltr .`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: pos(1, 35), R: '-'},
		},
		{
			// The parser looks ahead here to see whether a tag or a datatype
			// follows the string, so the tokenizer fails during that look
			// rather than on a token the parser asked for outright.
			name:        "a bad character where a tag or datatype might have followed",
			src:         `_:s <http://example.com/p> "v"$`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: pos(1, 31), R: '$'},
		},
		{
			// Likewise when looking ahead past the dot to check that the
			// statement was the last thing on its line.
			name:        "a bad character after a complete statement",
			src:         `_:a <http://example.com/p> _:b .$`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: pos(1, 33), R: '$'},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ntriples.Parse(strings.NewReader(tc.src))
			if err != tc.expectedErr {
				t.Errorf("Parse() error = %v, want %v", err, tc.expectedErr)
			}
		})
	}
}

func TestParseReaderError(t *testing.T) {
	errBoom := errors.New("boom")

	_, err := ntriples.Parse(&failingReader{data: []byte("_:s <http://example.com/p>"), err: errBoom})
	if !errors.Is(err, errBoom) {
		t.Errorf("Parse() error = %v, want %v", err, errBoom)
	}
}

// TestTermPositions checks that every node reports where it was written, which
// is what lets a tool point at the source rather than merely complain about it.
func TestTermPositions(t *testing.T) {
	src := `<http://example.com/s> <http://example.com/p> "v"^^<http://example.com/dt> .`

	doc, err := ntriples.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}
	if len(doc.Triples) != 1 {
		t.Fatalf("parsed %d triples, want 1", len(doc.Triples))
	}

	triple := doc.Triples[0]
	literal, ok := triple.Object.(*ntriples.Literal)
	if !ok {
		t.Fatalf("object is %T, want *ntriples.Literal", triple.Object)
	}

	positions := []struct {
		name string
		got  ntriples.Pos
		want ntriples.Pos
	}{
		{name: "document", got: doc.Pos, want: pos(1, 1)},
		{name: "triple", got: triple.Pos, want: pos(1, 1)},
		{name: "subject", got: triple.Subject.Position(), want: pos(1, 1)},
		{name: "predicate", got: triple.Predicate.Position(), want: pos(1, 24)},
		{name: "object", got: triple.Object.Position(), want: pos(1, 47)},
		{name: "datatype", got: literal.Datatype.Position(), want: pos(1, 52)},
	}

	for _, p := range positions {
		t.Run(p.name, func(t *testing.T) {
			if p.got != p.want {
				t.Errorf("Pos = %s, want %s", p.got, p.want)
			}
		})
	}
}

// TestTermsAreSealed asserts in test form what the unexported marker methods
// guarantee: a type switch over a Term is exhaustive, and a literal is not a
// term the grammar allows as a subject.
func TestTermsAreSealed(t *testing.T) {
	terms := []ntriples.Term{
		iriRef(1, 1, "http://example.com/a"),
		blankNode(1, 1, "b"),
		&ntriples.Literal{Pos: pos(1, 1), Value: "v"},
	}

	for _, term := range terms {
		switch term.(type) {
		case *ntriples.IRIRef, *ntriples.BlankNode, *ntriples.Literal:
		default:
			t.Errorf("%T is not one of the three term nodes", term)
		}
	}

	var subjects []ntriples.SubjectTerm
	for _, term := range terms {
		if subject, ok := term.(ntriples.SubjectTerm); ok {
			subjects = append(subjects, subject)
		}
	}
	if got, want := len(subjects), 2; got != want {
		t.Errorf("%d of the term nodes may be a subject, want %d", got, want)
	}
}

func TestParseErrorMessages(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "unexpected token",
			err: ntriples.UnexpectedTokenError{
				Expected: []ntriples.TokenType{ntriples.TokenIRIRef, ntriples.TokenBlankNodeLabel},
				Actual: ntriples.Token{
					Pos:   pos(2, 7),
					Type:  ntriples.TokenStringLiteral,
					Value: []byte("v"),
				},
			},
			want: "unexpected token at line 2, column 7: StringLiteral(v), " +
				"expected one of: IRIRef, BlankNodeLabel",
		},
		{
			name: "unexpected end of tokens",
			err: ntriples.UnexpectedEndOfTokensError{
				Expected: []ntriples.TokenType{ntriples.TokenDot},
				Pos:      pos(4, 12),
			},
			want: "unexpected end of tokens at line 4, column 12, expected one of: Dot",
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

// format renders a document for a failure message, since the default rendering
// of a tree of pointers says nothing useful.
func format(doc *ntriples.Document) string {
	var b strings.Builder
	b.WriteString("Document{")
	for i, triple := range doc.Triples {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(formatTerm(triple.Subject))
		b.WriteByte(' ')
		b.WriteString(formatTerm(triple.Predicate))
		b.WriteByte(' ')
		b.WriteString(formatTerm(triple.Object))
	}
	for _, comment := range doc.Comments {
		b.WriteString("; ")
		b.WriteString(comment.Text)
		b.WriteString("@" + comment.Pos.String())
	}
	b.WriteString("}")
	return b.String()
}

func formatTerm(term ntriples.Term) string {
	switch t := term.(type) {
	case *ntriples.IRIRef:
		return "<" + t.Value + ">@" + t.Pos.String()
	case *ntriples.BlankNode:
		return "_:" + t.Label + "@" + t.Pos.String()
	case *ntriples.Literal:
		s := `"` + t.Value + `"`
		if t.Language != "" {
			s += "@" + t.Language
		}
		if t.Datatype != nil {
			s += "^^<" + t.Datatype.Value + ">"
		}
		return s + "@" + t.Pos.String()
	default:
		return "<nil>"
	}
}
