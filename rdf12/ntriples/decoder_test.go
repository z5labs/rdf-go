package ntriples_test

import (
	"errors"
	"iter"
	"reflect"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/rdf12/ntriples"
)

// collect2 drains a decoded sequence, returning the triples yielded before any
// error and the error that stopped it.
func collect2(seq iter.Seq2[rdf.Triple, error]) ([]rdf.Triple, error) {
	var triples []rdf.Triple
	for triple, err := range seq {
		if err != nil {
			return triples, err
		}
		triples = append(triples, triple)
	}
	return triples, nil
}

func mustLanguageLiteral(t *testing.T, value, language string) rdf.Literal {
	t.Helper()

	literal, err := rdf.NewLanguageLiteral(value, language)
	if err != nil {
		t.Fatalf("NewLanguageLiteral(%q, %q) = %v", value, language, err)
	}
	return literal
}

func mustDirectionalLiteral(t *testing.T, value, language string, direction rdf.Direction) rdf.Literal {
	t.Helper()

	literal, err := rdf.NewDirectionalLiteral(value, language, direction)
	if err != nil {
		t.Fatalf("NewDirectionalLiteral(%q, %q, %q) = %v", value, language, direction, err)
	}
	return literal
}

func TestDecode(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected func(t *testing.T) []rdf.Triple
	}{
		{
			name: "an empty document",
			src:  "",
			expected: func(*testing.T) []rdf.Triple {
				return nil
			},
		},
		{
			name: "a triple term object",
			src:  `<http://example.com/s> <http://example.com/p> <<( <http://example.com/a> <http://example.com/b> "v" )>> .`,
			expected: func(*testing.T) []rdf.Triple {
				return []rdf.Triple{
					{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object: rdf.TripleTerm{
							Subject:   rdf.IRI("http://example.com/a"),
							Predicate: "http://example.com/b",
							Object:    rdf.NewLiteral("v"),
						},
					},
				}
			},
		},
		{
			name: "a triple term nested in a triple term",
			src: `<http://example.com/s> <http://example.com/p> ` +
				`<<( <http://example.com/a> <http://example.com/b> ` +
				`<<( <http://example.com/c> <http://example.com/d> "v" )>> )>> .`,
			expected: func(*testing.T) []rdf.Triple {
				return []rdf.Triple{
					{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object: rdf.TripleTerm{
							Subject:   rdf.IRI("http://example.com/a"),
							Predicate: "http://example.com/b",
							Object: rdf.TripleTerm{
								Subject:   rdf.IRI("http://example.com/c"),
								Predicate: "http://example.com/d",
								Object:    rdf.NewLiteral("v"),
							},
						},
					},
				}
			},
		},
		{
			name: "a base direction takes the rdf:dirLangString datatype",
			src:  `<http://example.com/s> <http://example.com/p> "text"@en--ltr .`,
			expected: func(t *testing.T) []rdf.Triple {
				return []rdf.Triple{
					{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    mustDirectionalLiteral(t, "text", "en", rdf.DirectionLTR),
					},
				}
			},
		},
		{
			name: "a right to left direction after several subtags",
			src:  `<http://example.com/s> <http://example.com/p> "text"@es-419--rtl .`,
			expected: func(t *testing.T) []rdf.Triple {
				return []rdf.Triple{
					{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    mustDirectionalLiteral(t, "text", "es-419", rdf.DirectionRTL),
					},
				}
			},
		},
		{
			name: "a tag with no direction keeps the rdf:langString datatype",
			src:  `<http://example.com/s> <http://example.com/p> "text"@en-GB .`,
			expected: func(t *testing.T) []rdf.Triple {
				return []rdf.Triple{
					{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    mustLanguageLiteral(t, "text", "en-GB"),
					},
				}
			},
		},
		{
			name: "a version directive contributes no triple",
			src:  "VERSION \"1.2\"\n<http://example.com/s> <http://example.com/p> \"v\" .\n",
			expected: func(*testing.T) []rdf.Triple {
				return []rdf.Triple{
					{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    rdf.NewLiteral("v"),
					},
				}
			},
		},
		{
			// The announcement is a hint the specification mandates no
			// behaviour from, so a document claiming 1.1 and writing a triple
			// term is read as written rather than refused.
			name: "a triple term under a directive announcing rdf 1.1",
			src:  "VERSION \"1.1\"\n<http://example.com/s> <http://example.com/p> <<( <http://example.com/a> <http://example.com/b> \"v\" )>> .\n",
			expected: func(*testing.T) []rdf.Triple {
				return []rdf.Triple{
					{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object: rdf.TripleTerm{
							Subject:   rdf.IRI("http://example.com/a"),
							Predicate: "http://example.com/b",
							Object:    rdf.NewLiteral("v"),
						},
					},
				}
			},
		},
		{
			name: "an rdf 1.1 document reads as it did there",
			src: "# a comment\n" +
				"<http://example.com/s> <http://example.com/p> <http://example.com/o> .\n" +
				"<http://example.com/s> <http://example.com/q> \"1\"^^<http://www.w3.org/2001/XMLSchema#integer> .\n",
			expected: func(t *testing.T) []rdf.Triple {
				typed, err := rdf.NewTypedLiteral("1", "http://www.w3.org/2001/XMLSchema#integer")
				if err != nil {
					t.Fatalf("NewTypedLiteral() = %v, want nil", err)
				}
				return []rdf.Triple{
					{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    rdf.IRI("http://example.com/o"),
					},
					{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/q",
						Object:    typed,
					},
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collect2(ntriples.Decode(strings.NewReader(tc.src)))
			if err != nil {
				t.Fatalf("Decode() = %v, want nil", err)
			}

			want := tc.expected(t)
			if len(got) != len(want) {
				t.Fatalf("Decode() yielded %d triples, want %d", len(got), len(want))
			}
			for i := range got {
				if !got[i].Equal(want[i]) {
					t.Errorf("Decode()[%d] = %s, want %s", i, got[i], want[i])
				}
			}
		})
	}
}

// TestDecodeBlankNodesInTripleTerms checks that a triple term shares the
// document's blank node scope: _:a inside one and _:a outside it are the same
// node.
func TestDecodeBlankNodesInTripleTerms(t *testing.T) {
	src := "<http://example.com/s> <http://example.com/p> <<( _:a <http://example.com/q> \"v\" )>> .\n" +
		"<http://example.com/s> <http://example.com/r> _:a .\n"

	triples, err := collect2(ntriples.Decode(strings.NewReader(src)))
	if err != nil {
		t.Fatalf("Decode() = %v, want nil", err)
	}
	if len(triples) != 2 {
		t.Fatalf("Decode() yielded %d triples, want 2", len(triples))
	}

	term, ok := triples[0].Object.(rdf.TripleTerm)
	if !ok {
		t.Fatalf("object is %T, want rdf.TripleTerm", triples[0].Object)
	}
	if !term.Subject.Equal(triples[1].Object) {
		t.Errorf("_:a in a triple term is %s, and outside one %s; want the same node",
			term.Subject, triples[1].Object)
	}
}

// TestDecodeTripleTermsAreValid checks that what Decode yields is what a graph
// will accept: the data model's position constraints hold to the bottom of the
// nesting.
func TestDecodeTripleTermsAreValid(t *testing.T) {
	src := "<http://example.com/s> <http://example.com/p> " +
		"<<( _:a <http://example.com/q> <<( _:b <http://example.com/r> \"v\"@en--rtl )>> )>> .\n"

	triples, err := collect2(ntriples.Decode(strings.NewReader(src)))
	if err != nil {
		t.Fatalf("Decode() = %v, want nil", err)
	}

	g := rdf.NewGraph()
	for _, triple := range triples {
		if err := g.Add(triple); err != nil {
			t.Fatalf("Add(%s) = %v, want nil", triple, err)
		}
	}
}

func TestDecodeErrors(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name:        "a tokenizer error carries its character position",
			src:         `$ <http://example.com/p> _:o .`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: pos(1, 1), R: '$'},
		},
		{
			name: "a parse error carries its token position",
			src:  `<<( _:a <http://example.com/p> _:b )>> <http://example.com/q> _:c .`,
			expectedErr: ntriples.UnexpectedTokenError{
				Expected: []ntriples.TokenType{ntriples.TokenIRIRef, ntriples.TokenBlankNodeLabel},
				Actual: ntriples.Token{
					Pos:   pos(1, 1),
					Type:  ntriples.TokenTripleTermOpen,
					Value: []byte("<<("),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect2(ntriples.Decode(strings.NewReader(tc.src)))
			if !reflect.DeepEqual(err, tc.expectedErr) {
				t.Errorf("Decode() error = %#v, want %#v", err, tc.expectedErr)
			}
		})
	}
}

// TestDecodeInvalidTerms covers what the grammar accepts but the data model
// refuses. Each has to name the position of the term at fault, and to keep the
// rdf package's error reachable underneath.
func TestDecodeInvalidTerms(t *testing.T) {
	testCases := []struct {
		name    string
		src     string
		wantPos ntriples.Pos
		wantErr error
	}{
		{
			// LANG_DIR admits any run of letters, so the grammar has nothing
			// to say about a fourteen character primary language subtag. RDF
			// 1.2 N-Triples §6.2 does: the tag must be well-formed by RFC 5646
			// §2.2.9, which caps a subtag at eight characters. This is the
			// W3C test ntriples-langdir-bad-4.
			name:    "a language tag that is not well-formed",
			src:     `<http://example.com/s> <http://example.com/p> "v"@cantbethislong .`,
			wantPos: pos(1, 50),
			wantErr: rdf.ErrInvalidLanguage,
		},
		{
			// LANG_DIR admits any run of letters after the "--"; only ltr and
			// rtl are base directions.
			name:    "a base direction that is neither ltr nor rtl",
			src:     `<http://example.com/s> <http://example.com/p> "v"@en--xyz .`,
			wantPos: pos(1, 50),
			wantErr: rdf.ErrInvalidDirection,
		},
		{
			name:    "rdf:dirLangString written as a datatype, which leaves it no language tag",
			src:     `<http://example.com/s> <http://example.com/p> "v"^^<http://www.w3.org/1999/02/22-rdf-syntax-ns#dirLangString> .`,
			wantPos: pos(1, 52),
			wantErr: rdf.ErrReservedDatatype,
		},
		{
			name:    "rdf:langString written as a datatype",
			src:     `<http://example.com/s> <http://example.com/p> "v"^^<http://www.w3.org/1999/02/22-rdf-syntax-ns#langString> .`,
			wantPos: pos(1, 52),
			wantErr: rdf.ErrReservedDatatype,
		},
		{
			name:    "a relative iri as the subject of a triple term",
			src:     `<http://example.com/s> <http://example.com/p> <<( <a> <http://example.com/q> "v" )>> .`,
			wantPos: pos(1, 51),
			wantErr: ntriples.ErrRelativeIRI,
		},
		{
			name:    "a relative iri as the predicate of a triple term",
			src:     `<http://example.com/s> <http://example.com/p> <<( _:a <q> "v" )>> .`,
			wantPos: pos(1, 55),
			wantErr: ntriples.ErrRelativeIRI,
		},
		{
			name:    "a relative iri nested two triple terms deep",
			src:     `_:s <http://example.com/p> <<( _:a <http://example.com/q> <<( _:b <http://example.com/r> <o> )>> )>> .`,
			wantPos: pos(1, 90),
			wantErr: ntriples.ErrRelativeIRI,
		},
		{
			name:    "a relative iri",
			src:     `<http://example.com/s> <http://example.com/p> <o> .`,
			wantPos: pos(1, 47),
			wantErr: ntriples.ErrRelativeIRI,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect2(ntriples.Decode(strings.NewReader(tc.src)))
			if err == nil {
				t.Fatal("Decode() = nil, want an error")
			}

			var invalid ntriples.InvalidTermError
			if !errors.As(err, &invalid) {
				t.Fatalf("Decode() error = %#v, want an InvalidTermError", err)
			}
			if invalid.Pos != tc.wantPos {
				t.Errorf("Pos = %s, want %s", invalid.Pos, tc.wantPos)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Decode() error = %v, want it to wrap %v", err, tc.wantErr)
			}
		})
	}
}

func TestDecodeStopsEarly(t *testing.T) {
	src := "<http://example.com/a> <http://example.com/p> \"1\" .\n" +
		"<http://example.com/b> <http://example.com/p> \"2\" .\n" +
		"<http://example.com/c> <http://example.com/p> \"3\" .\n"

	var seen int
	for range ntriples.Decode(strings.NewReader(src)) {
		seen++
		break
	}
	if want := 1; seen != want {
		t.Errorf("yielded %d triples after break, want %d", seen, want)
	}
}

func TestDecodeReaderError(t *testing.T) {
	errBoom := errors.New("boom")

	src := "<http://example.com/a> <http://example.com/p> \"1\" .\n<http://example.com/b>"
	_, err := collect2(ntriples.Decode(&failingReader{data: []byte(src), err: errBoom}))
	if !errors.Is(err, errBoom) {
		t.Errorf("Decode() error = %v, want %v", err, errBoom)
	}
}

// TestTriples covers lowering a document already parsed, which has to agree
// with what Decode would have produced from the same source — and which has to
// skip the version directives, since they contribute no triples.
func TestTriples(t *testing.T) {
	src := "VERSION \"1.2\"\n" +
		"<http://example.com/s> <http://example.com/p> \"text\"@en--ltr .\n" +
		"<http://example.com/s> <http://example.com/q> <<( <http://example.com/a> <http://example.com/r> \"v\" )>> .\n"

	doc, err := ntriples.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}

	lowered, err := ntriples.Triples(doc)
	if err != nil {
		t.Fatalf("Triples() = %v, want nil", err)
	}

	t.Run("the version directive contributes no triple", func(t *testing.T) {
		if got, want := len(lowered), 2; got != want {
			t.Errorf("Triples() returned %d triples, want %d", got, want)
		}
	})

	t.Run("the triples match what Decode produces", func(t *testing.T) {
		streamed, err := collect2(ntriples.Decode(strings.NewReader(src)))
		if err != nil {
			t.Fatalf("Decode() = %v, want nil", err)
		}
		if len(lowered) != len(streamed) {
			t.Fatalf("Triples() returned %d triples, Decode() yielded %d", len(lowered), len(streamed))
		}
		for i := range lowered {
			if !lowered[i].Equal(streamed[i]) {
				t.Errorf("Triples()[%d] = %s, want %s", i, lowered[i], streamed[i])
			}
		}
	})
}

// TestTriplesSharesOneScope checks that a document lowered in one go resolves
// a blank node label to one node however deep in the document it is written —
// including inside a triple term.
func TestTriplesSharesOneScope(t *testing.T) {
	src := "<http://example.com/s> <http://example.com/p> <<( _:a <http://example.com/q> \"v\" )>> .\n" +
		"<http://example.com/s> <http://example.com/r> _:a .\n"

	doc, err := ntriples.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}

	lowered, err := ntriples.Triples(doc)
	if err != nil {
		t.Fatalf("Triples() = %v, want nil", err)
	}
	if len(lowered) != 2 {
		t.Fatalf("Triples() returned %d triples, want 2", len(lowered))
	}

	term, ok := lowered[0].Object.(rdf.TripleTerm)
	if !ok {
		t.Fatalf("object is %T, want rdf.TripleTerm", lowered[0].Object)
	}
	if !term.Subject.Equal(lowered[1].Object) {
		t.Errorf("_:a lowered as %s then %s, want one node", term.Subject, lowered[1].Object)
	}
}

func TestTriplesNilDocument(t *testing.T) {
	triples, err := ntriples.Triples(nil)
	if err != nil {
		t.Errorf("Triples(nil) = %v, want nil", err)
	}
	if len(triples) != 0 {
		t.Errorf("Triples(nil) returned %d triples, want none", len(triples))
	}
}

func TestTriplesReportsInvalidTerms(t *testing.T) {
	doc, err := ntriples.Parse(strings.NewReader(
		`<http://example.com/s> <http://example.com/p> <<( _:a <http://example.com/q> "v"@en--xyz )>> .`))
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}

	_, err = ntriples.Triples(doc)
	if !errors.Is(err, rdf.ErrInvalidDirection) {
		t.Errorf("Triples() = %v, want it to wrap %v", err, rdf.ErrInvalidDirection)
	}
}

func TestInvalidTermErrorMessage(t *testing.T) {
	err := ntriples.InvalidTermError{Pos: pos(3, 9), Err: rdf.ErrInvalidDirection}

	want := "invalid term at line 3, column 9: " + rdf.ErrInvalidDirection.Error()
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
