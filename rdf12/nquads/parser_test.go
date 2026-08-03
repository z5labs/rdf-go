package nquads_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/z5labs/rdf-go/rdf12/nquads"
)

func pos(line, column int) nquads.Pos {
	return nquads.Pos{Line: line, Column: column}
}

func iriRef(line, column int, value string) *nquads.IRIRef {
	return &nquads.IRIRef{Pos: pos(line, column), Value: value}
}

func blankNode(line, column int, label string) *nquads.BlankNode {
	return &nquads.BlankNode{Pos: pos(line, column), Label: label}
}

// TestParseGraphLabel covers the production N-Quads adds to N-Triples, and
// what RDF 1.2 does and does not change about it: a triple term is an object
// and never a label.
func TestParseGraphLabel(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected *nquads.Document
	}{
		{
			name:     "an empty document",
			src:      "",
			expected: &nquads.Document{Pos: pos(1, 1)},
		},
		{
			name: "no label means the default graph",
			src:  `<http://e/s> <http://e/p> <http://e/o> .`,
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Statements: []nquads.Statement{
					&nquads.Quad{
						Pos:       pos(1, 1),
						Subject:   iriRef(1, 1, "http://e/s"),
						Predicate: iriRef(1, 14, "http://e/p"),
						Object:    iriRef(1, 27, "http://e/o"),
					},
				},
			},
		},
		{
			name: "an iri label",
			src:  `<http://e/s> <http://e/p> <http://e/o> <http://e/g> .`,
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Statements: []nquads.Statement{
					&nquads.Quad{
						Pos:       pos(1, 1),
						Subject:   iriRef(1, 1, "http://e/s"),
						Predicate: iriRef(1, 14, "http://e/p"),
						Object:    iriRef(1, 27, "http://e/o"),
						Graph:     iriRef(1, 40, "http://e/g"),
					},
				},
			},
		},
		{
			name: "a blank node label",
			src:  `<http://e/s> <http://e/p> <http://e/o> _:g .`,
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Statements: []nquads.Statement{
					&nquads.Quad{
						Pos:       pos(1, 1),
						Subject:   iriRef(1, 1, "http://e/s"),
						Predicate: iriRef(1, 14, "http://e/p"),
						Object:    iriRef(1, 27, "http://e/o"),
						Graph:     blankNode(1, 40, "g"),
					},
				},
			},
		},
		{
			name: "a triple term object followed by a label",
			src:  `<http://e/s> <http://e/p> <<( _:a <http://e/q> "v" )>> <http://e/g> .`,
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Statements: []nquads.Statement{
					&nquads.Quad{
						Pos:       pos(1, 1),
						Subject:   iriRef(1, 1, "http://e/s"),
						Predicate: iriRef(1, 14, "http://e/p"),
						Object: &nquads.TripleTerm{
							Pos:       pos(1, 27),
							Subject:   blankNode(1, 31, "a"),
							Predicate: iriRef(1, 35, "http://e/q"),
							Object:    &nquads.Literal{Pos: pos(1, 48), Value: "v"},
						},
						Graph: iriRef(1, 56, "http://e/g"),
					},
				},
			},
		},
		{
			name: "a directional literal object followed by a label",
			src:  `<http://e/s> <http://e/p> "v"@ar--rtl _:g .`,
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Statements: []nquads.Statement{
					&nquads.Quad{
						Pos:       pos(1, 1),
						Subject:   iriRef(1, 1, "http://e/s"),
						Predicate: iriRef(1, 14, "http://e/p"),
						Object: &nquads.Literal{
							Pos:       pos(1, 27),
							Value:     "v",
							Language:  "ar",
							Direction: "rtl",
						},
						Graph: blankNode(1, 39, "g"),
					},
				},
			},
		},
		{
			name: "a version directive above a labelled statement",
			src:  "VERSION \"1.2\"\n<http://e/s> <http://e/p> <http://e/o> <http://e/g> .\n",
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Statements: []nquads.Statement{
					&nquads.VersionDirective{Pos: pos(1, 1), Version: "1.2"},
					&nquads.Quad{
						Pos:       pos(2, 1),
						Subject:   iriRef(2, 1, "http://e/s"),
						Predicate: iriRef(2, 14, "http://e/p"),
						Object:    iriRef(2, 27, "http://e/o"),
						Graph:     iriRef(2, 40, "http://e/g"),
					},
				},
			},
		},
		{
			name: "labelled and unlabelled statements together",
			src:  "<http://e/a> <http://e/p> <http://e/o> <http://e/g> .\n<http://e/b> <http://e/p> <http://e/o> .",
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Statements: []nquads.Statement{
					&nquads.Quad{
						Pos:       pos(1, 1),
						Subject:   iriRef(1, 1, "http://e/a"),
						Predicate: iriRef(1, 14, "http://e/p"),
						Object:    iriRef(1, 27, "http://e/o"),
						Graph:     iriRef(1, 40, "http://e/g"),
					},
					&nquads.Quad{
						Pos:       pos(2, 1),
						Subject:   iriRef(2, 1, "http://e/b"),
						Predicate: iriRef(2, 14, "http://e/p"),
						Object:    iriRef(2, 27, "http://e/o"),
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := nquads.Parse(strings.NewReader(tc.src))
			if err != nil {
				t.Fatalf("Parse() = %v, want nil", err)
			}
			if !reflect.DeepEqual(doc, tc.expected) {
				t.Errorf("Parse() = %s, want %s", format(doc), format(tc.expected))
			}
		})
	}
}

// TestParseGraphLabelErrors covers what may not label a graph. The '.' is what
// the parser asks for once the object is read, so anything that is not one of
// the two label terminals is reported as the token standing where the '.' was
// due — a triple term included, RDF 1.2 having widened the object production
// alone.
func TestParseGraphLabelErrors(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name: "a triple term cannot label a graph",
			src:  `<http://e/s> <http://e/p> <http://e/o> <<( _:a <http://e/q> "v" )>> .`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenDot},
				Actual: nquads.Token{
					Pos:   pos(1, 40),
					Type:  nquads.TokenTripleTermOpen,
					Value: []byte("<<("),
				},
			},
		},
		{
			name: "a literal cannot label a graph",
			src:  `<http://e/s> <http://e/p> <http://e/o> "g" .`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenDot},
				Actual: nquads.Token{
					Pos:   pos(1, 40),
					Type:  nquads.TokenStringLiteral,
					Value: []byte("g"),
				},
			},
		},
		{
			name: "a directional literal cannot label a graph either",
			src:  `<http://e/s> <http://e/p> <http://e/o> "g"@en--ltr .`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenDot},
				Actual: nquads.Token{
					Pos:   pos(1, 40),
					Type:  nquads.TokenStringLiteral,
					Value: []byte("g"),
				},
			},
		},
		{
			name: "two labels are one too many",
			src:  `<http://e/s> <http://e/p> <http://e/o> <http://e/g> <http://e/h> .`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenDot},
				Actual: nquads.Token{
					Pos:   pos(1, 53),
					Type:  nquads.TokenIRIRef,
					Value: []byte("http://e/h"),
				},
			},
		},
		{
			name: "a label with no dot after it",
			src:  "<http://e/s> <http://e/p> <http://e/o> <http://e/g>\n",
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenDot},
				Actual:   nquads.Token{Pos: pos(1, 52), Type: nquads.TokenEOL, Value: []byte("\n")},
			},
		},
		{
			name: "the input ends after the label",
			src:  `<http://e/s> <http://e/p> <http://e/o> <http://e/g>`,
			expectedErr: nquads.UnexpectedEndOfTokensError{
				Expected: []nquads.TokenType{nquads.TokenDot},
				Pos:      pos(1, 40),
			},
		},
		{
			name: "a version directive carries no graph label",
			src:  `VERSION "1.2" <http://e/g>`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenEOL, nquads.TokenComment},
				Actual: nquads.Token{
					Pos:   pos(1, 15),
					Type:  nquads.TokenIRIRef,
					Value: []byte("http://e/g"),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := nquads.Parse(strings.NewReader(tc.src))
			if !reflect.DeepEqual(err, tc.expectedErr) {
				t.Errorf("Parse() error = %#v, want %#v", err, tc.expectedErr)
			}
		})
	}
}

// TestParseTripleTerms covers the production RDF 1.2 adds to the object of a
// statement, and the nesting it allows on that side alone.
func TestParseTripleTerms(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected *nquads.Document
	}{
		{
			name: "a triple term as an object",
			src:  `<http://example.com/s> <http://example.com/p> <<( <http://example.com/a> <http://example.com/b> _:c )>> .`,
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Statements: []nquads.Statement{
					&nquads.Quad{
						Pos:       pos(1, 1),
						Subject:   iriRef(1, 1, "http://example.com/s"),
						Predicate: iriRef(1, 24, "http://example.com/p"),
						Object: &nquads.TripleTerm{
							Pos:       pos(1, 47),
							Subject:   iriRef(1, 51, "http://example.com/a"),
							Predicate: iriRef(1, 74, "http://example.com/b"),
							Object:    blankNode(1, 97, "c"),
						},
					},
				},
			},
		},
		{
			name: "a triple term whose object is a literal",
			src:  `_:s <http://example.com/p> <<( _:a <http://example.com/q> "v"@en--ltr )>> .`,
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Statements: []nquads.Statement{
					&nquads.Quad{
						Pos:       pos(1, 1),
						Subject:   blankNode(1, 1, "s"),
						Predicate: iriRef(1, 5, "http://example.com/p"),
						Object: &nquads.TripleTerm{
							Pos:       pos(1, 28),
							Subject:   blankNode(1, 32, "a"),
							Predicate: iriRef(1, 36, "http://example.com/q"),
							Object: &nquads.Literal{
								Pos:       pos(1, 59),
								Value:     "v",
								Language:  "en",
								Direction: "ltr",
							},
						},
					},
				},
			},
		},
		{
			name: "triple terms nest on the object side",
			src:  `_:s <http://example.com/p> <<( _:a <http://example.com/q> <<( _:b <http://example.com/r> "v" )>> )>> .`,
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Statements: []nquads.Statement{
					&nquads.Quad{
						Pos:       pos(1, 1),
						Subject:   blankNode(1, 1, "s"),
						Predicate: iriRef(1, 5, "http://example.com/p"),
						Object: &nquads.TripleTerm{
							Pos:       pos(1, 28),
							Subject:   blankNode(1, 32, "a"),
							Predicate: iriRef(1, 36, "http://example.com/q"),
							Object: &nquads.TripleTerm{
								Pos:       pos(1, 59),
								Subject:   blankNode(1, 63, "b"),
								Predicate: iriRef(1, 67, "http://example.com/r"),
								Object:    &nquads.Literal{Pos: pos(1, 90), Value: "v"},
							},
						},
					},
				},
			},
		},
		{
			name: "a triple term needs no white space inside its brackets",
			src:  `_:s <http://example.com/p> <<(_:a <http://example.com/q> "v")>> .`,
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Statements: []nquads.Statement{
					&nquads.Quad{
						Pos:       pos(1, 1),
						Subject:   blankNode(1, 1, "s"),
						Predicate: iriRef(1, 5, "http://example.com/p"),
						Object: &nquads.TripleTerm{
							Pos:       pos(1, 28),
							Subject:   blankNode(1, 31, "a"),
							Predicate: iriRef(1, 35, "http://example.com/q"),
							Object:    &nquads.Literal{Pos: pos(1, 58), Value: "v"},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := nquads.Parse(strings.NewReader(tc.src))
			if err != nil {
				t.Fatalf("Parse() = %v, want nil", err)
			}
			if !reflect.DeepEqual(doc, tc.expected) {
				t.Errorf("Parse() = %s, want %s", format(doc), format(tc.expected))
			}
		})
	}
}

// TestParseVersionDirective covers the version announcement, which is a
// statement rather than a header: it stands wherever a triple may, ends with
// its line rather than with a '.', and may appear more than once.
func TestParseVersionDirective(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected *nquads.Document
	}{
		{
			name: "a directive alone",
			src:  `VERSION "1.2"`,
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Statements: []nquads.Statement{
					&nquads.VersionDirective{Pos: pos(1, 1), Version: "1.2"},
				},
			},
		},
		{
			name: "a directive above the triples it announces",
			src:  "VERSION \"1.2\"\n_:s <http://example.com/p> _:o .\n",
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Statements: []nquads.Statement{
					&nquads.VersionDirective{Pos: pos(1, 1), Version: "1.2"},
					&nquads.Quad{
						Pos:       pos(2, 1),
						Subject:   blankNode(2, 1, "s"),
						Predicate: iriRef(2, 5, "http://example.com/p"),
						Object:    blankNode(2, 28, "o"),
					},
				},
			},
		},
		{
			// RDF 1.2 Turtle §2.4, which N-Quads defers to by way of
			// N-Triples for the rules they share, allows more than one and says
			// each applies to the part of the document following it. Keeping
			// them in one ordered list is what preserves that.
			name: "a second directive part way down the document",
			src:  "_:s <http://example.com/p> _:o .\nVERSION \"1.2\"\n_:s <http://example.com/q> _:o .\n",
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Statements: []nquads.Statement{
					&nquads.Quad{
						Pos:       pos(1, 1),
						Subject:   blankNode(1, 1, "s"),
						Predicate: iriRef(1, 5, "http://example.com/p"),
						Object:    blankNode(1, 28, "o"),
					},
					&nquads.VersionDirective{Pos: pos(2, 1), Version: "1.2"},
					&nquads.Quad{
						Pos:       pos(3, 1),
						Subject:   blankNode(3, 1, "s"),
						Predicate: iriRef(3, 5, "http://example.com/q"),
						Object:    blankNode(3, 28, "o"),
					},
				},
			},
		},
		{
			name: "a directive trailed by a comment",
			src:  "VERSION \"1.2\" # announced\n",
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Statements: []nquads.Statement{
					&nquads.VersionDirective{Pos: pos(1, 1), Version: "1.2"},
				},
				Comments: []*nquads.Comment{
					{Pos: pos(1, 15), Text: "# announced"},
				},
			},
		},
		{
			// The specifier is a string literal like any other, so its escapes
			// are decoded, and which strings are version labels is not the
			// grammar's business.
			name: "a specifier the grammar admits and the version labels do not",
			src:  `VERSION "1.9"`,
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Statements: []nquads.Statement{
					&nquads.VersionDirective{Pos: pos(1, 1), Version: "1.9"},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := nquads.Parse(strings.NewReader(tc.src))
			if err != nil {
				t.Fatalf("Parse() = %v, want nil", err)
			}
			if !reflect.DeepEqual(doc, tc.expected) {
				t.Errorf("Parse() = %s, want %s", format(doc), format(tc.expected))
			}
		})
	}
}

// TestParseLangDir covers the split of a LANG_DIR into the tag and the base
// direction the tokenizer hands over together.
func TestParseLangDir(t *testing.T) {
	testCases := []struct {
		name          string
		src           string
		wantLanguage  string
		wantDirection string
	}{
		{
			name:         "a tag with no direction reads as the rdf 1.1 langtag it is",
			src:          `_:s <http://example.com/p> "v"@en-GB .`,
			wantLanguage: "en-GB",
		},
		{
			name:          "a left to right direction",
			src:           `_:s <http://example.com/p> "v"@en--ltr .`,
			wantLanguage:  "en",
			wantDirection: "ltr",
		},
		{
			name:          "a right to left direction after several subtags",
			src:           `_:s <http://example.com/p> "v"@es-419--rtl .`,
			wantLanguage:  "es-419",
			wantDirection: "rtl",
		},
		{
			// The grammar admits any run of letters here. Refusing one that is
			// not a direction is left to the moment the literal becomes an
			// rdf.Literal, alongside refusing an IRI that is not absolute.
			name:          "a direction the data model will refuse",
			src:           `_:s <http://example.com/p> "v"@en--xyz .`,
			wantLanguage:  "en",
			wantDirection: "xyz",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := nquads.Parse(strings.NewReader(tc.src))
			if err != nil {
				t.Fatalf("Parse() = %v, want nil", err)
			}

			literal := onlyLiteral(t, doc)
			if literal.Language != tc.wantLanguage {
				t.Errorf("Language = %q, want %q", literal.Language, tc.wantLanguage)
			}
			if literal.Direction != tc.wantDirection {
				t.Errorf("Direction = %q, want %q", literal.Direction, tc.wantDirection)
			}
		})
	}
}

// onlyLiteral returns the object of the document's single statement, which the
// test requires to be a literal.
func onlyLiteral(t *testing.T, doc *nquads.Document) *nquads.Literal {
	t.Helper()

	if len(doc.Statements) != 1 {
		t.Fatalf("parsed %d statements, want 1", len(doc.Statements))
	}
	quad, ok := doc.Statements[0].(*nquads.Quad)
	if !ok {
		t.Fatalf("statement is %T, want *nquads.Quad", doc.Statements[0])
	}
	literal, ok := quad.Object.(*nquads.Literal)
	if !ok {
		t.Fatalf("object is %T, want *nquads.Literal", quad.Object)
	}
	return literal
}

// TestParseAcceptsRDF11 checks the backward compatibility RDF 1.2 claims: a
// document written for RDF 1.1 N-Quads parses here to the same tree it would
// have there, graph labels included.
func TestParseAcceptsRDF11(t *testing.T) {
	src := "# a document of every rdf 1.1 form\n" +
		"<http://example.com/s> <http://example.com/p> <http://example.com/o> .\n" +
		"_:a <http://example.com/p> \"plain\" .\n" +
		"_:a <http://example.com/p> \"tagged\"@en-GB .\n" +
		"_:a <http://example.com/p> \"1\"^^<http://www.w3.org/2001/XMLSchema#integer> <http://example.com/g> . # trailing\n" +
		"\n" +
		"_:a <http://example.com/p> \"a\\nb\\u00e9\" _:g .\n"

	doc, err := nquads.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}

	if got, want := len(doc.Statements), 5; got != want {
		t.Fatalf("parsed %d statements, want %d", got, want)
	}
	if got, want := len(doc.Comments), 2; got != want {
		t.Fatalf("parsed %d comments, want %d", got, want)
	}

	last, ok := doc.Statements[4].(*nquads.Quad)
	if !ok {
		t.Fatalf("statement is %T, want *nquads.Quad", doc.Statements[4])
	}
	literal, ok := last.Object.(*nquads.Literal)
	if !ok {
		t.Fatalf("object is %T, want *nquads.Literal", last.Object)
	}
	if got, want := literal.Value, "a\nbé"; got != want {
		t.Errorf("Value = %q, want %q", got, want)
	}
}

func TestParseErrors(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			// The rule that keeps a triple term to the object position: the
			// subject production admits an IRIREF and a blank node label, and
			// "<<(" is neither.
			name: "a triple term as a subject",
			src:  `<<( _:a <http://example.com/p> _:b )>> <http://example.com/q> _:c .`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenIRIRef, nquads.TokenBlankNodeLabel},
				Actual:   nquads.Token{Pos: pos(1, 1), Type: nquads.TokenTripleTermOpen, Value: []byte("<<(")},
			},
		},
		{
			name: "a triple term as the subject of a triple term",
			src:  `_:s <http://example.com/p> <<( <<( _:a <http://example.com/q> _:b )>> <http://example.com/r> _:c )>> .`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenIRIRef, nquads.TokenBlankNodeLabel},
				Actual:   nquads.Token{Pos: pos(1, 32), Type: nquads.TokenTripleTermOpen, Value: []byte("<<(")},
			},
		},
		{
			name: "a literal as the predicate of a triple term",
			src:  `_:s <http://example.com/p> <<( _:a "v" _:b )>> .`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenIRIRef},
				Actual:   nquads.Token{Pos: pos(1, 36), Type: nquads.TokenStringLiteral, Value: []byte("v")},
			},
		},
		{
			name: "nothing after the datatype marker",
			src:  `_:s <http://example.com/p> "v"^^`,
			expectedErr: nquads.UnexpectedEndOfTokensError{
				Expected: []nquads.TokenType{nquads.TokenIRIRef},
				Pos:      pos(1, 31),
			},
		},
		{
			name: "a triple term as a predicate",
			src:  `_:s <<( _:a <http://example.com/p> _:b )>> _:o .`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenIRIRef},
				Actual:   nquads.Token{Pos: pos(1, 5), Type: nquads.TokenTripleTermOpen, Value: []byte("<<(")},
			},
		},
		{
			name: "a triple term left open",
			src:  `_:s <http://example.com/p> <<( _:a <http://example.com/q> _:b .`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenTripleTermClose},
				Actual:   nquads.Token{Pos: pos(1, 63), Type: nquads.TokenDot, Value: []byte(".")},
			},
		},
		{
			name: "a triple term missing its object",
			src:  `_:s <http://example.com/p> <<( _:a <http://example.com/q> )>> .`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{
					nquads.TokenIRIRef,
					nquads.TokenBlankNodeLabel,
					nquads.TokenStringLiteral,
					nquads.TokenTripleTermOpen,
				},
				Actual: nquads.Token{Pos: pos(1, 59), Type: nquads.TokenTripleTermClose, Value: []byte(")>>")},
			},
		},
		{
			name: "a triple term cut off by the end of the input",
			src:  `_:s <http://example.com/p> <<( _:a <http://example.com/q> _:b`,
			expectedErr: nquads.UnexpectedEndOfTokensError{
				Expected: []nquads.TokenType{nquads.TokenTripleTermClose},
				Pos:      pos(1, 59),
			},
		},
		{
			// A directive is a statement, not a triple: it carries no '.'.
			name: "a version directive ended with a dot",
			src:  `VERSION "1.2" .`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenEOL, nquads.TokenComment},
				Actual:   nquads.Token{Pos: pos(1, 15), Type: nquads.TokenDot, Value: []byte(".")},
			},
		},
		{
			name: "a version directive whose specifier is an iri",
			src:  `VERSION <http://example.com/1.2>`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenStringLiteral},
				Actual:   nquads.Token{Pos: pos(1, 9), Type: nquads.TokenIRIRef, Value: []byte("http://example.com/1.2")},
			},
		},
		{
			name: "a version keyword with no specifier",
			src:  `VERSION`,
			expectedErr: nquads.UnexpectedEndOfTokensError{
				Expected: []nquads.TokenType{nquads.TokenStringLiteral},
				Pos:      pos(1, 1),
			},
		},
		{
			name: "a triple following a directive on the same line",
			src:  `VERSION "1.2" _:s <http://example.com/p> _:o .`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenEOL, nquads.TokenComment},
				Actual:   nquads.Token{Pos: pos(1, 15), Type: nquads.TokenBlankNodeLabel, Value: []byte("s")},
			},
		},
		{
			name: "a directive following a triple on the same line",
			src:  `_:s <http://example.com/p> _:o . VERSION "1.2"`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenEOL, nquads.TokenComment},
				Actual:   nquads.Token{Pos: pos(1, 34), Type: nquads.TokenVersion, Value: []byte("VERSION")},
			},
		},
		{
			name: "a literal as a subject",
			src:  `"nope" <http://example.com/p> _:o .`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenIRIRef, nquads.TokenBlankNodeLabel},
				Actual:   nquads.Token{Pos: pos(1, 1), Type: nquads.TokenStringLiteral, Value: []byte("nope")},
			},
		},
		{
			name: "no dot at the end of the input",
			src:  `_:s <http://example.com/p> _:o`,
			expectedErr: nquads.UnexpectedEndOfTokensError{
				Expected: []nquads.TokenType{nquads.TokenDot},
				Pos:      pos(1, 28),
			},
		},
		{
			// The literal is complete when the input runs out, so what is
			// missing is the '.' rather than anything the literal wanted.
			name: "a literal at the end of the input",
			src:  `_:s <http://example.com/p> "v"`,
			expectedErr: nquads.UnexpectedEndOfTokensError{
				Expected: []nquads.TokenType{nquads.TokenDot},
				Pos:      pos(1, 28),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := nquads.Parse(strings.NewReader(tc.src))
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
			name:        "the rdf 1.1 star spelling of a quoted triple",
			src:         `<<_:a <http://example.com/p> _:b>> <http://example.com/q> _:c .`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: pos(1, 2), R: '<'},
		},
		{
			name:        "the version keyword in lower case, which is case-sensitive",
			src:         `version "1.2"`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: pos(1, 1), R: 'v'},
		},
		{
			// LANG_DIR demands a letter before the "--", so a direction with no
			// language tag is refused before the parser ever sees it.
			name:        "a base direction with no language tag",
			src:         `_:s <http://example.com/p> "v"@--ltr .`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: pos(1, 32), R: '-'},
		},
		{
			name:        "a closer missing its second angle bracket",
			src:         `_:s <http://example.com/p> <<( _:a <http://example.com/q> _:b )> .`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: pos(1, 65), R: ' '},
		},
		{
			// The parser looks ahead here to see whether a tag or a datatype
			// follows the string, so the tokenizer fails during that look
			// rather than on a token the parser asked for outright.
			name:        "a bad character where a tag or datatype might have followed",
			src:         `_:s <http://example.com/p> "v"$`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: pos(1, 31), R: '$'},
		},
		{
			// Likewise when looking ahead past the dot to check that the
			// statement was the last thing on its line.
			name:        "a bad character after a complete triple",
			src:         `_:a <http://example.com/p> _:b .$`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: pos(1, 33), R: '$'},
		},
		{
			name:        "a bad character after a version directive",
			src:         `VERSION "1.2"$`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: pos(1, 14), R: '$'},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := nquads.Parse(strings.NewReader(tc.src))
			if err != tc.expectedErr {
				t.Errorf("Parse() error = %v, want %v", err, tc.expectedErr)
			}
		})
	}
}

func TestParseReaderError(t *testing.T) {
	errBoom := errors.New("boom")

	_, err := nquads.Parse(&failingReader{data: []byte("_:s <http://example.com/p>"), err: errBoom})
	if !errors.Is(err, errBoom) {
		t.Errorf("Parse() error = %v, want %v", err, errBoom)
	}
}

// TestNodePositions checks that every node reports where it was written, which
// is what lets a tool point at the source rather than merely complain about it.
func TestNodePositions(t *testing.T) {
	src := "VERSION \"1.2\"\n" +
		`<http://example.com/s> <http://example.com/p> <<( _:a <http://example.com/q> "v"^^<http://example.com/dt> )>> <http://example.com/g> .`

	doc, err := nquads.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}
	if len(doc.Statements) != 2 {
		t.Fatalf("parsed %d statements, want 2", len(doc.Statements))
	}

	directive, ok := doc.Statements[0].(*nquads.VersionDirective)
	if !ok {
		t.Fatalf("first statement is %T, want *nquads.VersionDirective", doc.Statements[0])
	}
	quad, ok := doc.Statements[1].(*nquads.Quad)
	if !ok {
		t.Fatalf("second statement is %T, want *nquads.Quad", doc.Statements[1])
	}
	term, ok := quad.Object.(*nquads.TripleTerm)
	if !ok {
		t.Fatalf("object is %T, want *nquads.TripleTerm", quad.Object)
	}
	literal, ok := term.Object.(*nquads.Literal)
	if !ok {
		t.Fatalf("triple term object is %T, want *nquads.Literal", term.Object)
	}

	positions := []struct {
		name string
		got  nquads.Pos
		want nquads.Pos
	}{
		{name: "document", got: doc.Pos, want: pos(1, 1)},
		{name: "directive", got: directive.Position(), want: pos(1, 1)},
		{name: "quad", got: quad.Position(), want: pos(2, 1)},
		{name: "subject", got: quad.Subject.Position(), want: pos(2, 1)},
		{name: "predicate", got: quad.Predicate.Position(), want: pos(2, 24)},
		{name: "triple term", got: term.Position(), want: pos(2, 47)},
		{name: "triple term subject", got: term.Subject.Position(), want: pos(2, 51)},
		{name: "triple term predicate", got: term.Predicate.Position(), want: pos(2, 55)},
		{name: "triple term object", got: literal.Position(), want: pos(2, 78)},
		{name: "datatype", got: literal.Datatype.Position(), want: pos(2, 83)},
		{name: "graph label", got: quad.Graph.Position(), want: pos(2, 111)},
	}

	for _, p := range positions {
		t.Run(p.name, func(t *testing.T) {
			if p.got != p.want {
				t.Errorf("Pos = %s, want %s", p.got, p.want)
			}
		})
	}
}

// TestNodesAreSealed asserts in test form what the unexported marker methods
// guarantee: a type switch over a Statement or a Term is exhaustive, and
// neither a literal nor a triple term is a term the grammar allows as a
// subject.
func TestNodesAreSealed(t *testing.T) {
	terms := []nquads.Term{
		iriRef(1, 1, "http://example.com/a"),
		blankNode(1, 1, "b"),
		&nquads.Literal{Pos: pos(1, 1), Value: "v"},
		&nquads.TripleTerm{
			Pos:       pos(1, 1),
			Subject:   blankNode(1, 5, "a"),
			Predicate: iriRef(1, 9, "http://example.com/p"),
			Object:    blankNode(1, 33, "b"),
		},
	}

	for _, term := range terms {
		switch term.(type) {
		case *nquads.IRIRef, *nquads.BlankNode, *nquads.Literal, *nquads.TripleTerm:
		default:
			t.Errorf("%T is not one of the four term nodes", term)
		}
	}

	var subjects []nquads.SubjectTerm
	for _, term := range terms {
		if subject, ok := term.(nquads.SubjectTerm); ok {
			subjects = append(subjects, subject)
		}
	}
	if got, want := len(subjects), 2; got != want {
		t.Errorf("%d of the term nodes may be a subject, want %d", got, want)
	}

	statements := []nquads.Statement{
		&nquads.Quad{
			Pos:       pos(1, 1),
			Subject:   blankNode(1, 1, "s"),
			Predicate: iriRef(1, 5, "http://example.com/p"),
			Object:    blankNode(1, 28, "o"),
		},
		&nquads.VersionDirective{Pos: pos(2, 1), Version: "1.2"},
	}
	for _, statement := range statements {
		switch statement.(type) {
		case *nquads.Quad, *nquads.VersionDirective:
		default:
			t.Errorf("%T is not one of the two statement nodes", statement)
		}
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
			err: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenIRIRef, nquads.TokenBlankNodeLabel},
				Actual: nquads.Token{
					Pos:   pos(2, 7),
					Type:  nquads.TokenTripleTermOpen,
					Value: []byte("<<("),
				},
			},
			want: "unexpected token at line 2, column 7: TripleTermOpen(<<(), " +
				"expected one of: IRIRef, BlankNodeLabel",
		},
		{
			name: "unexpected end of tokens",
			err: nquads.UnexpectedEndOfTokensError{
				Expected: []nquads.TokenType{nquads.TokenTripleTermClose},
				Pos:      pos(4, 12),
			},
			want: "unexpected end of tokens at line 4, column 12, expected one of: TripleTermClose",
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
func format(doc *nquads.Document) string {
	var b strings.Builder
	b.WriteString("Document{")
	for i, statement := range doc.Statements {
		if i > 0 {
			b.WriteString("; ")
		}
		switch s := statement.(type) {
		case *nquads.Quad:
			b.WriteString(formatTerm(s.Subject))
			b.WriteByte(' ')
			b.WriteString(formatTerm(s.Predicate))
			b.WriteByte(' ')
			b.WriteString(formatTerm(s.Object))
			if s.Graph != nil {
				b.WriteByte(' ')
				b.WriteString(formatTerm(s.Graph))
			}
		case *nquads.VersionDirective:
			b.WriteString(`VERSION "` + s.Version + `"@` + s.Pos.String())
		}
	}
	for _, comment := range doc.Comments {
		b.WriteString("; ")
		b.WriteString(comment.Text)
		b.WriteString("@" + comment.Pos.String())
	}
	b.WriteString("}")
	return b.String()
}

func formatTerm(term nquads.Term) string {
	switch t := term.(type) {
	case *nquads.IRIRef:
		return "<" + t.Value + ">@" + t.Pos.String()
	case *nquads.BlankNode:
		return "_:" + t.Label + "@" + t.Pos.String()
	case *nquads.Literal:
		s := `"` + t.Value + `"`
		if t.Language != "" {
			s += "@" + t.Language
			if t.Direction != "" {
				s += "--" + t.Direction
			}
		}
		if t.Datatype != nil {
			s += "^^<" + t.Datatype.Value + ">"
		}
		return s + "@" + t.Pos.String()
	case *nquads.TripleTerm:
		return "<<( " + formatTerm(t.Subject) +
			" " + formatTerm(t.Predicate) +
			" " + formatTerm(t.Object) + " )>>@" + t.Pos.String()
	default:
		return "<nil>"
	}
}
