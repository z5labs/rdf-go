package nquads_test

import (
	"errors"
	"fmt"
	"iter"
	"reflect"
	"slices"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/rdf11/nquads"
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

// collectQuads drains a decoded sequence, returning the quads yielded before
// any error and the error that stopped it.
func collectQuads(seq iter.Seq2[rdf.Quad, error]) ([]rdf.Quad, error) {
	var quads []rdf.Quad
	for quad, err := range seq {
		if err != nil {
			return quads, err
		}
		quads = append(quads, quad)
	}
	return quads, nil
}

func decode(t *testing.T, src string) []rdf.Quad {
	t.Helper()

	quads, err := collectQuads(nquads.Decode(strings.NewReader(src)))
	if err != nil {
		t.Fatalf("Decode() = %v, want nil", err)
	}
	return quads
}

func parse(t *testing.T, src string) *nquads.Document {
	t.Helper()

	doc, err := nquads.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}
	return doc
}

func printToString(t *testing.T, doc *nquads.Document) string {
	t.Helper()

	var b strings.Builder
	if err := nquads.Print(&b, doc); err != nil {
		t.Fatalf("Print() = %v, want nil", err)
	}
	return b.String()
}

func encodeToString(t *testing.T, quads []rdf.Quad) string {
	t.Helper()

	var b strings.Builder
	if err := nquads.Encode(&b, slices.Values(quads)); err != nil {
		t.Fatalf("Encode() = %v, want nil", err)
	}
	return b.String()
}

// TestParseGraphLabel covers the one production N-Quads adds. Everything else
// about the grammar is N-Triples', and is exercised by the tokenizer tests
// this package shares with it.
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
				Quads: []*nquads.Quad{
					{
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
				Quads: []*nquads.Quad{
					{
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
				Quads: []*nquads.Quad{
					{
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
			name: "a literal object followed by a label",
			src:  `<http://e/s> <http://e/p> "v"@en <http://e/g> .`,
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Quads: []*nquads.Quad{
					{
						Pos:       pos(1, 1),
						Subject:   iriRef(1, 1, "http://e/s"),
						Predicate: iriRef(1, 14, "http://e/p"),
						Object:    &nquads.Literal{Pos: pos(1, 27), Value: "v", LangPos: pos(1, 30), Language: "en"},
						Graph:     iriRef(1, 34, "http://e/g"),
					},
				},
			},
		},
		{
			name: "a typed literal object followed by a label",
			src:  `<http://e/s> <http://e/p> "1"^^<http://e/dt> _:g .`,
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Quads: []*nquads.Quad{
					{
						Pos:       pos(1, 1),
						Subject:   iriRef(1, 1, "http://e/s"),
						Predicate: iriRef(1, 14, "http://e/p"),
						Object: &nquads.Literal{
							Pos:      pos(1, 27),
							Value:    "1",
							Datatype: iriRef(1, 32, "http://e/dt"),
						},
						Graph: blankNode(1, 46, "g"),
					},
				},
			},
		},
		{
			name: "labelled and unlabelled statements together",
			src:  "<http://e/a> <http://e/p> <http://e/o> <http://e/g> .\n<http://e/b> <http://e/p> <http://e/o> .",
			expected: &nquads.Document{
				Pos: pos(1, 1),
				Quads: []*nquads.Quad{
					{
						Pos:       pos(1, 1),
						Subject:   iriRef(1, 1, "http://e/a"),
						Predicate: iriRef(1, 14, "http://e/p"),
						Object:    iriRef(1, 27, "http://e/o"),
						Graph:     iriRef(1, 40, "http://e/g"),
					},
					{
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
			got := parse(t, tc.src)
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("Parse() = %s, want %s", formatDoc(got), formatDoc(tc.expected))
			}
		})
	}
}

func TestParseGraphLabelErrors(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
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
			name: "the input ends after the object",
			src:  `<http://e/s> <http://e/p> <http://e/o>`,
			expectedErr: nquads.UnexpectedEndOfTokensError{
				Expected: []nquads.TokenType{nquads.TokenDot},
				Pos:      pos(1, 27),
			},
		},
		{
			name: "a literal in subject position, as in N-Triples",
			src:  `"nope" <http://e/p> <http://e/o> .`,
			expectedErr: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenIRIRef, nquads.TokenBlankNodeLabel},
				Actual: nquads.Token{
					Pos:   pos(1, 1),
					Type:  nquads.TokenStringLiteral,
					Value: []byte("nope"),
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

// TestDecodeGraphs covers the criterion that a statement with no label belongs
// to the default graph, and that a labelled one belongs to the graph named.
func TestDecodeGraphs(t *testing.T) {
	t.Run("no label gives a nil graph, which is the default graph", func(t *testing.T) {
		quads := decode(t, `<http://e/s> <http://e/p> <http://e/o> .`)
		if len(quads) != 1 {
			t.Fatalf("decoded %d quads, want 1", len(quads))
		}
		if quads[0].Graph != nil {
			t.Errorf("Graph = %s, want nil", quads[0].Graph)
		}
	})

	t.Run("an iri label names the graph", func(t *testing.T) {
		quads := decode(t, `<http://e/s> <http://e/p> <http://e/o> <http://e/g> .`)
		if len(quads) != 1 {
			t.Fatalf("decoded %d quads, want 1", len(quads))
		}
		if want := rdf.IRI("http://e/g"); !quads[0].Graph.Equal(want) {
			t.Errorf("Graph = %s, want %s", quads[0].Graph, want)
		}
	})

	t.Run("a blank node label is scoped like any other", func(t *testing.T) {
		// The same label as a term and as a graph label is one node.
		quads := decode(t, "_:g <http://e/p> <http://e/o> _:g .")
		if len(quads) != 1 {
			t.Fatalf("decoded %d quads, want 1", len(quads))
		}
		if !quads[0].Subject.Equal(quads[0].Graph) {
			t.Errorf("_:g decoded as %s as a subject and %s as a label, want one node",
				quads[0].Subject, quads[0].Graph)
		}
	})

	t.Run("the quads land in the dataset they name", func(t *testing.T) {
		src := "<http://e/a> <http://e/p> \"1\" .\n" +
			"<http://e/b> <http://e/p> \"2\" <http://e/g> .\n" +
			"<http://e/c> <http://e/p> \"3\" <http://e/g> .\n" +
			"<http://e/d> <http://e/p> \"4\" <http://e/h> .\n"

		d := rdf.NewDataset()
		for _, quad := range decode(t, src) {
			if err := d.Add(quad); err != nil {
				t.Fatalf("Add(%s) = %v, want nil", quad, err)
			}
		}

		if got, want := d.Default().Len(), 1; got != want {
			t.Errorf("the default graph holds %d triples, want %d", got, want)
		}
		for _, tc := range []struct {
			name rdf.Term
			want int
		}{
			{name: rdf.IRI("http://e/g"), want: 2},
			{name: rdf.IRI("http://e/h"), want: 1},
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

func TestDecodeInvalidTerms(t *testing.T) {
	src := `<http://e/s> <> "v" <http://e/g> .`

	_, err := collectQuads(nquads.Decode(strings.NewReader(src)))
	var invalid nquads.InvalidTermError
	if !errors.As(err, &invalid) {
		t.Fatalf("Decode() error = %#v, want an InvalidTermError", err)
	}
	if got, want := invalid.Pos, pos(1, 14); got != want {
		t.Errorf("Pos = %s, want %s", got, want)
	}
	if !errors.Is(err, nquads.ErrRelativeIRI) {
		t.Errorf("Decode() error = %v, want it to wrap %v", err, nquads.ErrRelativeIRI)
	}
}

func TestQuadsLowersADocument(t *testing.T) {
	src := "_:a <http://e/p> \"v\" <http://e/g> .\n<http://e/s> <http://e/q> _:a .\n"

	lowered, err := nquads.Quads(parse(t, src))
	if err != nil {
		t.Fatalf("Quads() = %v, want nil", err)
	}
	if got, want := len(lowered), 2; got != want {
		t.Fatalf("Quads() returned %d quads, want %d", got, want)
	}

	t.Run("one scope covers the whole document", func(t *testing.T) {
		if !lowered[0].Subject.Equal(lowered[1].Object) {
			t.Errorf("_:a lowered as %s then %s, want one node", lowered[0].Subject, lowered[1].Object)
		}
	})

	t.Run("it agrees with Decode", func(t *testing.T) {
		if !isomorphicDatasets(t, lowered, decode(t, src)) {
			t.Error("Quads() and Decode() produced datasets that are not isomorphic")
		}
	})

	t.Run("a nil document has no quads", func(t *testing.T) {
		quads, err := nquads.Quads(nil)
		if err != nil {
			t.Fatalf("Quads() = %v, want nil", err)
		}
		if quads != nil {
			t.Errorf("Quads() = %v, want nil", quads)
		}
	})
}

// TestPrintCanonicalForm pins the bytes a document prints as. N-Quads has no
// canonical section of its own; these are N-Triples §4's rules with the graph
// label written between the object and the '.'.
func TestPrintCanonicalForm(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "an empty document prints as nothing",
			src:  "",
			want: "",
		},
		{
			name: "one space before the label and one after it",
			src:  `<http://e/s>   <http://e/p>\t<http://e/o>    <http://e/g>   .`,
			want: "<http://e/s> <http://e/p> <http://e/o> <http://e/g> .\n",
		},
		{
			name: "a statement with no label is written without one",
			src:  `<http://e/s> <http://e/p> <http://e/o> .`,
			want: "<http://e/s> <http://e/p> <http://e/o> .\n",
		},
		{
			name: "a blank node label",
			src:  `_:s <http://e/p> _:o _:g .`,
			want: "_:s <http://e/p> _:o _:g .\n",
		},
		{
			name: "a tagged literal before a label",
			src:  `<http://e/s> <http://e/p> "v"@en-GB <http://e/g> .`,
			want: "<http://e/s> <http://e/p> \"v\"@en-GB <http://e/g> .\n",
		},
		{
			name: "only the four characters that must be escaped are",
			src:  `<http://e/s> <http://e/p> "q\" b\\ n\n r\r t\t" <http://e/g> .`,
			want: "<http://e/s> <http://e/p> \"q\\\" b\\\\ n\\n r\\r t\t\" <http://e/g> .\n",
		},
		{
			name: "a final newline is always written",
			src:  "<http://e/a> <http://e/p> <http://e/o> <http://e/g> .\n<http://e/b> <http://e/p> <http://e/o> .",
			want: "<http://e/a> <http://e/p> <http://e/o> <http://e/g> .\n" +
				"<http://e/b> <http://e/p> <http://e/o> .\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// The tab in one source has to survive being written here.
			src := strings.ReplaceAll(tc.src, `\t<`, "\t<")

			if got := printToString(t, parse(t, src)); got != tc.want {
				t.Errorf("Print() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPrintKeepsComments(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "a comment on its own line",
			src:  "# a note\n<http://e/s> <http://e/p> _:o <http://e/g> .",
			want: "# a note\n<http://e/s> <http://e/p> _:o <http://e/g> .\n",
		},
		{
			name: "a comment trailing a labelled statement",
			src:  "<http://e/s> <http://e/p> _:o <http://e/g> . # why",
			want: "<http://e/s> <http://e/p> _:o <http://e/g> . # why\n",
		},
		{
			name: "comments before, trailing and after",
			src: "# first\n" +
				"<http://e/a> <http://e/p> _:x <http://e/g> . # trailing\n" +
				"# last",
			want: "# first\n" +
				"<http://e/a> <http://e/p> _:x <http://e/g> . # trailing\n" +
				"# last\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := printToString(t, parse(t, tc.src)); got != tc.want {
				t.Errorf("Print() = %q, want %q", got, tc.want)
			}
		})
	}
}

// withoutPositions returns a copy of doc with every position cleared, so two
// trees can be compared on what they say rather than where it was written.
func withoutPositions(doc *nquads.Document) *nquads.Document {
	if doc == nil {
		return nil
	}

	out := &nquads.Document{}
	for _, q := range doc.Quads {
		next := &nquads.Quad{
			Subject:   termWithoutPosition(q.Subject).(nquads.SubjectTerm),
			Predicate: termWithoutPosition(q.Predicate).(*nquads.IRIRef),
			Object:    termWithoutPosition(q.Object),
		}
		if q.Graph != nil {
			next.Graph = termWithoutPosition(q.Graph).(nquads.GraphTerm)
		}
		out.Quads = append(out.Quads, next)
	}
	for _, c := range doc.Comments {
		out.Comments = append(out.Comments, &nquads.Comment{Text: c.Text})
	}
	return out
}

func termWithoutPosition(node nquads.Term) nquads.Term {
	switch n := node.(type) {
	case *nquads.IRIRef:
		return &nquads.IRIRef{Value: n.Value}
	case *nquads.BlankNode:
		return &nquads.BlankNode{Label: n.Label}
	case *nquads.Literal:
		out := &nquads.Literal{Value: n.Value, Language: n.Language}
		if n.Datatype != nil {
			out.Datatype = &nquads.IRIRef{Value: n.Datatype.Value}
		}
		return out
	default:
		panic("unknown term node")
	}
}

var roundTripSources = []string{
	"",
	"<http://e/s> <http://e/p> <http://e/o> .",
	"<http://e/s> <http://e/p> <http://e/o> <http://e/g> .",
	"<http://e/s> <http://e/p> <http://e/o> _:g .",
	"_:a <http://e/p> _:b <http://e/g> .\n_:b <http://e/p> _:a .",
	"_:a <http://e/p> _:b _:g .\n_:b <http://e/p> _:a _:g .",
	`<http://e/s> <http://e/p> "text"@en-GB <http://e/g> .`,
	`<http://e/s> <http://e/p> "1"^^<http://e/dt> _:g .`,
	`<http://e/s> <http://e/p> "a\"b\\c\nd\re\tf" <http://e/g> .`,
	"<http://e/a> <http://e/p> \"1\" .\n<http://e/b> <http://e/p> \"2\" <http://e/g> .\n<http://e/c> <http://e/p> \"3\" <http://e/h> .",
	"# a note\n<http://e/s> <http://e/p> _:o <http://e/g> . # trailing\n# last",
}

// TestPrintRoundTrip covers the first round trip: parsing what was printed
// gives back the tree that was printed, once positions are set aside.
func TestPrintRoundTrip(t *testing.T) {
	for _, src := range roundTripSources {
		t.Run(src, func(t *testing.T) {
			first := parse(t, src)
			printed := printToString(t, first)
			second := parse(t, printed)

			if !reflect.DeepEqual(withoutPositions(first), withoutPositions(second)) {
				t.Errorf("round trip changed the tree; printed = %q", printed)
			}
		})
	}
}

// reify turns a set of quads into a single graph, so that two datasets can be
// compared with [rdf.Isomorphic].
//
// Comparing datasets graph by graph would not do: a blank node may appear in
// several graphs, or label one, so the bijection has to be found across the
// whole dataset at once. Giving each quad a fresh blank node carrying its four
// parts as edges reduces that to one graph, and a bijection between the
// reified graphs restricted to the original blank nodes is exactly a bijection
// between the datasets.
func reify(t *testing.T, quads []rdf.Quad) *rdf.Graph {
	t.Helper()

	const (
		subject   = rdf.IRI("urn:test:subject")
		predicate = rdf.IRI("urn:test:predicate")
		object    = rdf.IRI("urn:test:object")
		graph     = rdf.IRI("urn:test:graph")

		// A statement in the default graph has no label. Something has to
		// stand in for it, and an IRI no document would write is the safest
		// thing to choose.
		defaultGraph = rdf.IRI("urn:test:defaultGraph")
	)

	scope := rdf.NewBlankNodeScope()

	g := rdf.NewGraph()
	for i, quad := range quads {
		statement := scope.Node(fmt.Sprintf("statement%d", i))

		label := rdf.Term(defaultGraph)
		if quad.Graph != nil {
			label = quad.Graph
		}

		for _, edge := range []struct {
			predicate rdf.IRI
			object    rdf.Term
		}{
			{predicate: subject, object: quad.Subject},
			{predicate: predicate, object: quad.Predicate},
			{predicate: object, object: quad.Object},
			{predicate: graph, object: label},
		} {
			triple := rdf.Triple{Subject: statement, Predicate: edge.predicate, Object: edge.object}
			if err := g.Add(triple); err != nil {
				t.Fatalf("Add(%s) = %v, want nil", triple, err)
			}
		}
	}
	return g
}

func isomorphicDatasets(t *testing.T, a, b []rdf.Quad) bool {
	t.Helper()

	return rdf.Isomorphic(reify(t, a), reify(t, b))
}

// TestEncodeRoundTrip covers the second round trip: decoding what was encoded
// gives back the same dataset.
func TestEncodeRoundTrip(t *testing.T) {
	for _, src := range roundTripSources {
		t.Run(src, func(t *testing.T) {
			first := decode(t, src)
			encoded := encodeToString(t, first)
			second := decode(t, encoded)

			if !isomorphicDatasets(t, first, second) {
				t.Errorf("round trip changed the dataset; encoded = %q", encoded)
			}
		})
	}
}

// TestReifyTellsDatasetsApart guards the comparison the round trip leans on.
// A helper that called everything isomorphic would make that test pass whatever
// the encoder did.
func TestReifyTellsDatasetsApart(t *testing.T) {
	testCases := []struct {
		name string
		a, b string
		want bool
	}{
		{
			name: "the same document twice",
			a:    "<http://e/s> <http://e/p> <http://e/o> <http://e/g> .",
			b:    "<http://e/s> <http://e/p> <http://e/o> <http://e/g> .",
			want: true,
		},
		{
			name: "a relabelled blank node",
			a:    "_:a <http://e/p> <http://e/o> <http://e/g> .",
			b:    "_:zzz <http://e/p> <http://e/o> <http://e/g> .",
			want: true,
		},
		{
			name: "the same triple in different graphs",
			a:    "<http://e/s> <http://e/p> <http://e/o> <http://e/g> .",
			b:    "<http://e/s> <http://e/p> <http://e/o> <http://e/h> .",
		},
		{
			name: "a labelled statement against an unlabelled one",
			a:    "<http://e/s> <http://e/p> <http://e/o> <http://e/g> .",
			b:    "<http://e/s> <http://e/p> <http://e/o> .",
		},
		{
			name: "two graphs sharing a blank node against two that do not",
			a:    "_:a <http://e/p> <http://e/o> <http://e/g> .\n_:a <http://e/p> <http://e/o> <http://e/h> .",
			b:    "_:a <http://e/p> <http://e/o> <http://e/g> .\n_:b <http://e/p> <http://e/o> <http://e/h> .",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isomorphicDatasets(t, decode(t, tc.a), decode(t, tc.b)); got != tc.want {
				t.Errorf("isomorphicDatasets() = %t, want %t", got, tc.want)
			}
		})
	}
}

func TestEncodeIsStable(t *testing.T) {
	src := "<http://e/a> <http://e/p> \"a\tb\"@en <http://e/g> .\n<http://e/b> <http://e/p> \"1\"^^<http://e/dt> .\n"

	once := encodeToString(t, decode(t, src))
	if twice := encodeToString(t, decode(t, once)); twice != once {
		t.Errorf("encoding twice gave %q, want %q", twice, once)
	}
}

// TestPrintMatchesEncodeWithoutComments checks the claim the doc makes: a
// document carrying no comments prints in canonical form.
//
// Documents naming blank nodes are left out, and not because the claim fails
// for them. Print writes the label the document used and Encode writes the one
// the scope minted, so the two agree on the form and differ on the spelling —
// which is the blank node caveat on [nquads.Encode], not a difference in
// canonicality. Comparing them here would be comparing labels.
func TestPrintMatchesEncodeWithoutComments(t *testing.T) {
	for _, src := range roundTripSources {
		if strings.Contains(src, "#") || strings.Contains(src, "_:") {
			continue
		}

		t.Run(src, func(t *testing.T) {
			doc := parse(t, src)
			quads, err := nquads.Quads(doc)
			if err != nil {
				t.Fatalf("Quads() = %v, want nil", err)
			}

			printed := printToString(t, doc)
			if encoded := encodeToString(t, quads); printed != encoded {
				t.Errorf("Print() = %q, Encode() = %q, want them equal", printed, encoded)
			}
		})
	}
}

// failingWriter fails after letting a fixed number of bytes through.
type failingWriter struct {
	allow int
	err   error
}

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.allow <= 0 {
		return 0, w.err
	}
	if len(p) > w.allow {
		n := w.allow
		w.allow = 0
		return n, w.err
	}
	w.allow -= len(p)
	return len(p), nil
}

func TestPrintErrors(t *testing.T) {
	errBoom := errors.New("boom")

	t.Run("a nil document", func(t *testing.T) {
		var b strings.Builder
		if err := nquads.Print(&b, nil); !errors.Is(err, nquads.ErrNilDocument) {
			t.Errorf("Print() = %v, want %v", err, nquads.ErrNilDocument)
		}
	})

	t.Run("a writer that fails", func(t *testing.T) {
		doc := parse(t, "# note\n<http://e/s> <http://e/p> _:o <http://e/g> .\n")
		if err := nquads.Print(&failingWriter{err: errBoom}, doc); !errors.Is(err, errBoom) {
			t.Errorf("Print() = %v, want %v", err, errBoom)
		}
	})
}

func TestEncodeErrors(t *testing.T) {
	errBoom := errors.New("boom")

	t.Run("a writer that fails", func(t *testing.T) {
		quads := decode(t, "<http://e/s> <http://e/p> _:o <http://e/g> .")

		err := nquads.Encode(&failingWriter{err: errBoom}, slices.Values(quads))
		if !errors.Is(err, errBoom) {
			t.Errorf("Encode() = %v, want %v", err, errBoom)
		}
	})

	t.Run("a quad that cannot be written as N-Quads", func(t *testing.T) {
		// A literal graph label has no N-Quads form, so writing it would
		// produce a document this package could not read back.
		invalid := rdf.Quad{
			Triple: rdf.Triple{
				Subject:   rdf.IRI("http://e/s"),
				Predicate: "http://e/p",
				Object:    rdf.IRI("http://e/o"),
			},
			Graph: rdf.NewLiteral("not a graph"),
		}

		var b strings.Builder
		err := nquads.Encode(&b, slices.Values([]rdf.Quad{invalid}))
		if !errors.Is(err, rdf.ErrInvalidGraphName) {
			t.Errorf("Encode() = %v, want %v", err, rdf.ErrInvalidGraphName)
		}
		if b.String() != "" {
			t.Errorf("Encode() wrote %q, want nothing", b.String())
		}
	})

	t.Run("a triple term object", func(t *testing.T) {
		// The shared term types model RDF 1.2, so a triple term reaches this
		// package's Encode even though its own parser can never produce one.
		// RDF 1.1 N-Quads has no syntax for it.
		invalid := rdf.Quad{
			Triple: rdf.Triple{
				Subject:   rdf.IRI("http://e/s"),
				Predicate: "http://e/p",
				Object: rdf.TripleTerm{
					Subject:   rdf.IRI("http://e/s2"),
					Predicate: "http://e/p2",
					Object:    rdf.IRI("http://e/o2"),
				},
			},
			Graph: rdf.IRI("http://e/g"),
		}

		var b strings.Builder
		err := nquads.Encode(&b, slices.Values([]rdf.Quad{invalid}))
		if !errors.Is(err, nquads.ErrTripleTerm) {
			t.Errorf("Encode() = %v, want %v", err, nquads.ErrTripleTerm)
		}
		if b.String() != "" {
			t.Errorf("Encode() wrote %q, want nothing", b.String())
		}
	})

	t.Run("the statements before an invalid one are still written", func(t *testing.T) {
		valid := rdf.Quad{
			Triple: rdf.Triple{
				Subject:   rdf.IRI("http://e/s"),
				Predicate: "http://e/p",
				Object:    rdf.IRI("http://e/o"),
			},
			Graph: rdf.IRI("http://e/g"),
		}
		invalid := rdf.Quad{
			Triple: rdf.Triple{
				Subject:   rdf.IRI("http://e/s"),
				Predicate: "",
				Object:    rdf.IRI("http://e/o"),
			},
		}

		var b strings.Builder
		err := nquads.Encode(&b, slices.Values([]rdf.Quad{valid, invalid}))
		if !errors.Is(err, rdf.ErrInvalidPredicate) {
			t.Errorf("Encode() = %v, want %v", err, rdf.ErrInvalidPredicate)
		}
		if want := valid.String() + "\n"; b.String() != want {
			t.Errorf("Encode() wrote %q, want %q", b.String(), want)
		}
	})
}

// formatDoc renders a document for a failure message.
func formatDoc(doc *nquads.Document) string {
	var b strings.Builder
	b.WriteString("Document{")
	for i, q := range doc.Quads {
		if i > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "%s %s %s", formatTerm(q.Subject), formatTerm(q.Predicate), formatTerm(q.Object))
		if q.Graph != nil {
			fmt.Fprintf(&b, " %s", formatTerm(q.Graph))
		}
	}
	for _, c := range doc.Comments {
		fmt.Fprintf(&b, "; %s@%s", c.Text, c.Pos)
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
		}
		if t.Datatype != nil {
			s += "^^<" + t.Datatype.Value + ">"
		}
		return s + "@" + t.Pos.String()
	default:
		return "<nil>"
	}
}
