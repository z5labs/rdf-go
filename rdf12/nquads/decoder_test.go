package nquads_test

import (
	"errors"
	"iter"
	"reflect"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/rdf12/nquads"
)

// collect2 drains a decoded sequence, returning the quads yielded before any
// error and the error that stopped it.
func collect2(seq iter.Seq2[rdf.Quad, error]) ([]rdf.Quad, error) {
	var quads []rdf.Quad
	for quad, err := range seq {
		if err != nil {
			return quads, err
		}
		quads = append(quads, quad)
	}
	return quads, nil
}

// quad assembles a data model quad. The triple is embedded rather than named,
// which reads poorly inline in a table, so the four terms are taken in the
// order a statement writes them.
func quad(subject rdf.Term, predicate rdf.IRI, object, graph rdf.Term) rdf.Quad {
	return rdf.Quad{
		Triple: rdf.Triple{
			Subject:   subject,
			Predicate: predicate,
			Object:    object,
		},
		Graph: graph,
	}
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
		expected func(t *testing.T) []rdf.Quad
	}{
		{
			name: "an empty document",
			src:  "",
			expected: func(*testing.T) []rdf.Quad {
				return nil
			},
		},
		{
			name: "a triple term object",
			src:  `<http://example.com/s> <http://example.com/p> <<( <http://example.com/a> <http://example.com/b> "v" )>> .`,
			expected: func(*testing.T) []rdf.Quad {
				return []rdf.Quad{
					quad(
						rdf.IRI("http://example.com/s"),
						"http://example.com/p",
						rdf.TripleTerm{
							Subject:   rdf.IRI("http://example.com/a"),
							Predicate: "http://example.com/b",
							Object:    rdf.NewLiteral("v"),
						},
						nil,
					),
				}
			},
		},
		{
			name: "a triple term object in a named graph",
			src: `<http://example.com/s> <http://example.com/p> ` +
				`<<( <http://example.com/a> <http://example.com/b> "v" )>> <http://example.com/g> .`,
			expected: func(*testing.T) []rdf.Quad {
				return []rdf.Quad{
					quad(
						rdf.IRI("http://example.com/s"),
						"http://example.com/p",
						rdf.TripleTerm{
							Subject:   rdf.IRI("http://example.com/a"),
							Predicate: "http://example.com/b",
							Object:    rdf.NewLiteral("v"),
						},
						rdf.IRI("http://example.com/g"),
					),
				}
			},
		},
		{
			name: "a triple term nested in a triple term",
			src: `<http://example.com/s> <http://example.com/p> ` +
				`<<( <http://example.com/a> <http://example.com/b> ` +
				`<<( <http://example.com/c> <http://example.com/d> "v" )>> )>> .`,
			expected: func(*testing.T) []rdf.Quad {
				return []rdf.Quad{
					quad(
						rdf.IRI("http://example.com/s"),
						"http://example.com/p",
						rdf.TripleTerm{
							Subject:   rdf.IRI("http://example.com/a"),
							Predicate: "http://example.com/b",
							Object: rdf.TripleTerm{
								Subject:   rdf.IRI("http://example.com/c"),
								Predicate: "http://example.com/d",
								Object:    rdf.NewLiteral("v"),
							},
						},
						nil,
					),
				}
			},
		},
		{
			name: "a base direction takes the rdf:dirLangString datatype",
			src:  `<http://example.com/s> <http://example.com/p> "text"@en--ltr .`,
			expected: func(t *testing.T) []rdf.Quad {
				return []rdf.Quad{
					quad(
						rdf.IRI("http://example.com/s"),
						"http://example.com/p",
						mustDirectionalLiteral(t, "text", "en", rdf.DirectionLTR),
						nil,
					),
				}
			},
		},
		{
			name: "a directional literal in a named graph",
			src:  `<http://example.com/s> <http://example.com/p> "text"@ar--rtl <http://example.com/g> .`,
			expected: func(t *testing.T) []rdf.Quad {
				return []rdf.Quad{
					quad(
						rdf.IRI("http://example.com/s"),
						"http://example.com/p",
						mustDirectionalLiteral(t, "text", "ar", rdf.DirectionRTL),
						rdf.IRI("http://example.com/g"),
					),
				}
			},
		},
		{
			name: "a right to left direction after several subtags",
			src:  `<http://example.com/s> <http://example.com/p> "text"@es-419--rtl .`,
			expected: func(t *testing.T) []rdf.Quad {
				return []rdf.Quad{
					quad(
						rdf.IRI("http://example.com/s"),
						"http://example.com/p",
						mustDirectionalLiteral(t, "text", "es-419", rdf.DirectionRTL),
						nil,
					),
				}
			},
		},
		{
			name: "a tag with no direction keeps the rdf:langString datatype",
			src:  `<http://example.com/s> <http://example.com/p> "text"@en-GB .`,
			expected: func(t *testing.T) []rdf.Quad {
				return []rdf.Quad{
					quad(
						rdf.IRI("http://example.com/s"),
						"http://example.com/p",
						mustLanguageLiteral(t, "text", "en-GB"),
						nil,
					),
				}
			},
		},
		{
			name: "a version directive contributes no quad",
			src:  "VERSION \"1.2\"\n<http://example.com/s> <http://example.com/p> \"v\" <http://example.com/g> .\n",
			expected: func(*testing.T) []rdf.Quad {
				return []rdf.Quad{
					quad(
						rdf.IRI("http://example.com/s"),
						"http://example.com/p",
						rdf.NewLiteral("v"),
						rdf.IRI("http://example.com/g"),
					),
				}
			},
		},
		{
			// The announcement is a hint the specification mandates no
			// behaviour from, so a document claiming 1.1 and writing a triple
			// term is read as written rather than refused.
			name: "a triple term under a directive announcing rdf 1.1",
			src:  "VERSION \"1.1\"\n<http://example.com/s> <http://example.com/p> <<( <http://example.com/a> <http://example.com/b> \"v\" )>> .\n",
			expected: func(*testing.T) []rdf.Quad {
				return []rdf.Quad{
					quad(
						rdf.IRI("http://example.com/s"),
						"http://example.com/p",
						rdf.TripleTerm{
							Subject:   rdf.IRI("http://example.com/a"),
							Predicate: "http://example.com/b",
							Object:    rdf.NewLiteral("v"),
						},
						nil,
					),
				}
			},
		},
		{
			name: "an rdf 1.1 document reads as it did there",
			src: "# a comment\n" +
				"<http://example.com/s> <http://example.com/p> <http://example.com/o> .\n" +
				"<http://example.com/s> <http://example.com/q> \"1\"^^<http://www.w3.org/2001/XMLSchema#integer> <http://example.com/g> .\n",
			expected: func(t *testing.T) []rdf.Quad {
				typed, err := rdf.NewTypedLiteral("1", "http://www.w3.org/2001/XMLSchema#integer")
				if err != nil {
					t.Fatalf("NewTypedLiteral() = %v, want nil", err)
				}
				return []rdf.Quad{
					quad(
						rdf.IRI("http://example.com/s"),
						"http://example.com/p",
						rdf.IRI("http://example.com/o"),
						nil,
					),
					quad(
						rdf.IRI("http://example.com/s"),
						"http://example.com/q",
						typed,
						rdf.IRI("http://example.com/g"),
					),
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collect2(nquads.Decode(strings.NewReader(tc.src)))
			if err != nil {
				t.Fatalf("Decode() = %v, want nil", err)
			}

			want := tc.expected(t)
			if len(got) != len(want) {
				t.Fatalf("Decode() yielded %d quads, want %d", len(got), len(want))
			}
			for i := range got {
				if !got[i].Equal(want[i]) {
					t.Errorf("Decode()[%d] = %s, want %s", i, got[i], want[i])
				}
			}
		})
	}
}

// TestDecodeGraphs covers what N-Quads adds to N-Triples: the optional graph
// label, and the absence of one meaning the default graph.
func TestDecodeGraphs(t *testing.T) {
	t.Run("no label gives a nil graph, which is the default graph", func(t *testing.T) {
		quads, err := collect2(nquads.Decode(strings.NewReader(
			`<http://example.com/s> <http://example.com/p> <http://example.com/o> .`)))
		if err != nil {
			t.Fatalf("Decode() = %v, want nil", err)
		}
		if len(quads) != 1 {
			t.Fatalf("decoded %d quads, want 1", len(quads))
		}
		if quads[0].Graph != nil {
			t.Errorf("Graph = %s, want nil", quads[0].Graph)
		}
	})

	t.Run("a blank node label is scoped like any other", func(t *testing.T) {
		// The same label as a term and as a graph label is one node, and a
		// triple term is no exception: the scope covers the whole document.
		quads, err := collect2(nquads.Decode(strings.NewReader(
			`_:g <http://example.com/p> <<( _:g <http://example.com/q> "v" )>> _:g .`)))
		if err != nil {
			t.Fatalf("Decode() = %v, want nil", err)
		}
		if len(quads) != 1 {
			t.Fatalf("decoded %d quads, want 1", len(quads))
		}

		term, ok := quads[0].Object.(rdf.TripleTerm)
		if !ok {
			t.Fatalf("object is %T, want rdf.TripleTerm", quads[0].Object)
		}
		if !quads[0].Subject.Equal(quads[0].Graph) || !term.Subject.Equal(quads[0].Graph) {
			t.Errorf("_:g decoded as %s as a subject, %s inside a triple term and %s as a label; want one node",
				quads[0].Subject, term.Subject, quads[0].Graph)
		}
	})

	t.Run("the quads land in the graph they name", func(t *testing.T) {
		src := "<http://example.com/a> <http://example.com/p> \"1\" .\n" +
			"<http://example.com/b> <http://example.com/p> " +
			"<<( <http://example.com/x> <http://example.com/y> \"z\" )>> <http://example.com/g> .\n" +
			"<http://example.com/c> <http://example.com/p> \"3\" <http://example.com/g> .\n" +
			"<http://example.com/d> <http://example.com/p> \"4\" <http://example.com/h> .\n"

		quads, err := collect2(nquads.Decode(strings.NewReader(src)))
		if err != nil {
			t.Fatalf("Decode() = %v, want nil", err)
		}

		d := rdf.NewDataset()
		for _, q := range quads {
			if err := d.Add(q); err != nil {
				t.Fatalf("Add(%s) = %v, want nil", q, err)
			}
		}

		if got, want := d.Default().Len(), 1; got != want {
			t.Errorf("the default graph holds %d triples, want %d", got, want)
		}
		for _, tc := range []struct {
			name rdf.Term
			want int
		}{
			{name: rdf.IRI("http://example.com/g"), want: 2},
			{name: rdf.IRI("http://example.com/h"), want: 1},
		} {
			g, ok := d.NamedGraph(tc.name)
			if !ok {
				t.Fatalf("NamedGraph(%s) found nothing", tc.name)
			}
			if got := g.Len(); got != tc.want {
				t.Errorf("the graph named %s holds %d triples, want %d", tc.name, got, tc.want)
			}
		}
		if got, want := d.Len(), 4; got != want {
			t.Errorf("Len() = %d, want %d", got, want)
		}
	})
}

// TestDecodeBlankNodesInTripleTerms checks that a triple term shares the
// document's blank node scope: _:a inside one and _:a outside it are the same
// node.
func TestDecodeBlankNodesInTripleTerms(t *testing.T) {
	src := "<http://example.com/s> <http://example.com/p> <<( _:a <http://example.com/q> \"v\" )>> .\n" +
		"<http://example.com/s> <http://example.com/r> _:a <http://example.com/g> .\n"

	quads, err := collect2(nquads.Decode(strings.NewReader(src)))
	if err != nil {
		t.Fatalf("Decode() = %v, want nil", err)
	}
	if len(quads) != 2 {
		t.Fatalf("Decode() yielded %d quads, want 2", len(quads))
	}

	term, ok := quads[0].Object.(rdf.TripleTerm)
	if !ok {
		t.Fatalf("object is %T, want rdf.TripleTerm", quads[0].Object)
	}
	if !term.Subject.Equal(quads[1].Object) {
		t.Errorf("_:a in a triple term is %s, and outside one %s; want the same node",
			term.Subject, quads[1].Object)
	}
}

// TestDecodeTripleTermsAreValid checks that what Decode yields is what a
// dataset will accept: the data model's position constraints hold to the bottom
// of the nesting, and alongside a graph label.
func TestDecodeTripleTermsAreValid(t *testing.T) {
	src := "<http://example.com/s> <http://example.com/p> " +
		"<<( _:a <http://example.com/q> <<( _:b <http://example.com/r> \"v\"@en--rtl )>> )>> _:g .\n"

	quads, err := collect2(nquads.Decode(strings.NewReader(src)))
	if err != nil {
		t.Fatalf("Decode() = %v, want nil", err)
	}

	d := rdf.NewDataset()
	for _, q := range quads {
		if err := d.Add(q); err != nil {
			t.Fatalf("Add(%s) = %v, want nil", q, err)
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
			expectedErr: nquads.UnexpectedCharacterError{Pos: pos(1, 1), R: '$'},
		},
		{
			name: "a parse error carries its token position",
			src:  `<<( _:a <http://example.com/p> _:b )>> <http://example.com/q> _:c .`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenIRIRef, nquads.TokenBlankNodeLabel},
				Actual: nquads.Token{
					Pos:   pos(1, 1),
					Type:  nquads.TokenTripleTermOpen,
					Value: []byte("<<("),
				},
			},
		},
		{
			// graphLabel is the RDF 1.1 production unchanged, so the "<<(" is
			// not a label the parser could read. What it is instead is the
			// token standing where the '.' was due.
			name: "a triple term is not a graph label",
			src:  `<http://example.com/s> <http://example.com/p> "v" <<( _:a <http://example.com/q> "w" )>> .`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenDot},
				Actual: nquads.Token{
					Pos:   pos(1, 51),
					Type:  nquads.TokenTripleTermOpen,
					Value: []byte("<<("),
				},
			},
		},
		{
			name: "a literal is not a graph label",
			src:  `<http://example.com/s> <http://example.com/p> "v" "g" .`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenDot},
				Actual: nquads.Token{
					Pos:   pos(1, 51),
					Type:  nquads.TokenStringLiteral,
					Value: []byte("g"),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect2(nquads.Decode(strings.NewReader(tc.src)))
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
		wantPos nquads.Pos
		wantErr error
	}{
		{
			// LANG_DIR admits any run of letters after the "--"; only ltr and
			// rtl are base directions.
			name:    "a base direction that is neither ltr nor rtl",
			src:     `<http://example.com/s> <http://example.com/p> "v"@en--xyz .`,
			wantPos: pos(1, 47),
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
			wantErr: nquads.ErrRelativeIRI,
		},
		{
			name:    "a relative iri as the predicate of a triple term",
			src:     `<http://example.com/s> <http://example.com/p> <<( _:a <q> "v" )>> .`,
			wantPos: pos(1, 55),
			wantErr: nquads.ErrRelativeIRI,
		},
		{
			name:    "a relative iri nested two triple terms deep",
			src:     `_:s <http://example.com/p> <<( _:a <http://example.com/q> <<( _:b <http://example.com/r> <o> )>> )>> .`,
			wantPos: pos(1, 90),
			wantErr: nquads.ErrRelativeIRI,
		},
		{
			name:    "a relative iri",
			src:     `<http://example.com/s> <http://example.com/p> <o> .`,
			wantPos: pos(1, 47),
			wantErr: nquads.ErrRelativeIRI,
		},
		{
			name:    "a relative iri as the graph label",
			src:     `<http://example.com/s> <http://example.com/p> "v" <g> .`,
			wantPos: pos(1, 51),
			wantErr: nquads.ErrRelativeIRI,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect2(nquads.Decode(strings.NewReader(tc.src)))
			if err == nil {
				t.Fatal("Decode() = nil, want an error")
			}

			var invalid nquads.InvalidTermError
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
	for range nquads.Decode(strings.NewReader(src)) {
		seen++
		break
	}
	if want := 1; seen != want {
		t.Errorf("yielded %d quads after break, want %d", seen, want)
	}
}

func TestDecodeReaderError(t *testing.T) {
	errBoom := errors.New("boom")

	src := "<http://example.com/a> <http://example.com/p> \"1\" .\n<http://example.com/b>"
	_, err := collect2(nquads.Decode(&failingReader{data: []byte(src), err: errBoom}))
	if !errors.Is(err, errBoom) {
		t.Errorf("Decode() error = %v, want %v", err, errBoom)
	}
}

// TestQuads covers lowering a document already parsed, which has to agree with
// what Decode would have produced from the same source — and which has to skip
// the version directives, since they contribute no quads.
func TestQuads(t *testing.T) {
	src := "VERSION \"1.2\"\n" +
		"<http://example.com/s> <http://example.com/p> \"text\"@en--ltr <http://example.com/g> .\n" +
		"<http://example.com/s> <http://example.com/q> <<( <http://example.com/a> <http://example.com/r> \"v\" )>> .\n"

	doc, err := nquads.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}

	lowered, err := nquads.Quads(doc)
	if err != nil {
		t.Fatalf("Quads() = %v, want nil", err)
	}

	t.Run("the version directive contributes no quad", func(t *testing.T) {
		if got, want := len(lowered), 2; got != want {
			t.Errorf("Quads() returned %d quads, want %d", got, want)
		}
	})

	t.Run("the quads match what Decode produces", func(t *testing.T) {
		streamed, err := collect2(nquads.Decode(strings.NewReader(src)))
		if err != nil {
			t.Fatalf("Decode() = %v, want nil", err)
		}
		if len(lowered) != len(streamed) {
			t.Fatalf("Quads() returned %d quads, Decode() yielded %d", len(lowered), len(streamed))
		}
		for i := range lowered {
			if !lowered[i].Equal(streamed[i]) {
				t.Errorf("Quads()[%d] = %s, want %s", i, lowered[i], streamed[i])
			}
		}
	})
}

// TestQuadsSharesOneScope checks that a document lowered in one go resolves a
// blank node label to one node however deep in the document it is written —
// including inside a triple term and as a graph label.
func TestQuadsSharesOneScope(t *testing.T) {
	src := "<http://example.com/s> <http://example.com/p> <<( _:a <http://example.com/q> \"v\" )>> .\n" +
		"<http://example.com/s> <http://example.com/r> \"w\" _:a .\n"

	doc, err := nquads.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}

	lowered, err := nquads.Quads(doc)
	if err != nil {
		t.Fatalf("Quads() = %v, want nil", err)
	}
	if len(lowered) != 2 {
		t.Fatalf("Quads() returned %d quads, want 2", len(lowered))
	}

	term, ok := lowered[0].Object.(rdf.TripleTerm)
	if !ok {
		t.Fatalf("object is %T, want rdf.TripleTerm", lowered[0].Object)
	}
	if !term.Subject.Equal(lowered[1].Graph) {
		t.Errorf("_:a lowered as %s then %s, want one node", term.Subject, lowered[1].Graph)
	}
}

func TestQuadsNilDocument(t *testing.T) {
	quads, err := nquads.Quads(nil)
	if err != nil {
		t.Errorf("Quads(nil) = %v, want nil", err)
	}
	if len(quads) != 0 {
		t.Errorf("Quads(nil) returned %d quads, want none", len(quads))
	}
}

func TestQuadsReportsInvalidTerms(t *testing.T) {
	doc, err := nquads.Parse(strings.NewReader(
		`<http://example.com/s> <http://example.com/p> <<( _:a <http://example.com/q> "v"@en--xyz )>> .`))
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}

	_, err = nquads.Quads(doc)
	if !errors.Is(err, rdf.ErrInvalidDirection) {
		t.Errorf("Quads() = %v, want it to wrap %v", err, rdf.ErrInvalidDirection)
	}
}

func TestInvalidTermErrorMessage(t *testing.T) {
	err := nquads.InvalidTermError{Pos: pos(3, 9), Err: rdf.ErrInvalidDirection}

	want := "invalid term at line 3, column 9: " + rdf.ErrInvalidDirection.Error()
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
