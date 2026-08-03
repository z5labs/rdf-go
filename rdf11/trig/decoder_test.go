package trig_test

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"slices"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/rdf11/nquads"
	"github.com/z5labs/rdf-go/rdf11/trig"
	"github.com/z5labs/rdf-go/vocab"
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

// decodeTrig lowers a TriG document, failing the test if it cannot be read.
func decodeTrig(t *testing.T, src string) []rdf.Quad {
	t.Helper()

	quads, err := collect2(trig.Decode(strings.NewReader(src)))
	if err != nil {
		t.Fatalf("Decode() = %v, want nil", err)
	}
	return quads
}

// decodeNQuads lowers the N-Quads a test states its expectation in.
//
// Writing what a TriG document means as N-Quads is how the W3C evaluation
// tests state it, and it is the only readable way to write down an expectation
// involving blank nodes: the labels a scope mints are its own, so the two
// datasets are compared by isomorphism and the labels never have to be
// guessed.
func decodeNQuads(t *testing.T, src string) []rdf.Quad {
	t.Helper()

	quads, err := collect2(nquads.Decode(strings.NewReader(src)))
	if err != nil {
		t.Fatalf("nquads.Decode() = %v, want nil", err)
	}
	return quads
}

func datasetOf(t *testing.T, quads []rdf.Quad) *rdf.Dataset {
	t.Helper()

	d := rdf.NewDataset()
	for _, quad := range quads {
		if err := d.Add(quad); err != nil {
			t.Fatalf("Add(%s) = %v, want nil", quad, err)
		}
	}
	return d
}

func formatQuads(quads []rdf.Quad) string {
	var b strings.Builder
	for _, quad := range quads {
		fmt.Fprintf(&b, "  %s\n", quad)
	}
	return b.String()
}

func formatDataset(d *rdf.Dataset) string {
	return formatQuads(slices.Collect(d.All()))
}

// isomorphicDatasets reports whether two datasets say the same thing, up to
// the labels their blank nodes carry.
//
// It works by reifying every quad onto a blank node of its own — one arc for
// each of the four positions — and asking whether the two graphs that result
// are isomorphic. A blank node standing in a quad keeps its arcs whichever
// dataset it came from, so a mapping exists between the two reified graphs
// exactly when one exists between the datasets. Going through [rdf.Isomorphic]
// rather than writing a second search is what keeps this a test helper rather
// than a second implementation to get wrong.
func isomorphicDatasets(t *testing.T, a, b *rdf.Dataset) bool {
	t.Helper()
	return rdf.Isomorphic(reify(t, a), reify(t, b))
}

const reifyNamespace = "urn:rdf-go:test:reify:"

func reify(t *testing.T, d *rdf.Dataset) *rdf.Graph {
	t.Helper()

	g := rdf.NewGraph()
	scope := rdf.NewBlankNodeScope()

	add := func(subject rdf.Term, predicate rdf.IRI, object rdf.Term) {
		t.Helper()
		if err := g.Add(rdf.Triple{Subject: subject, Predicate: predicate, Object: object}); err != nil {
			t.Fatalf("Add() = %v, want nil", err)
		}
	}

	for quad := range d.All() {
		cell := scope.New()
		add(cell, reifyNamespace+"subject", quad.Subject)
		add(cell, reifyNamespace+"predicate", quad.Predicate)
		add(cell, reifyNamespace+"object", quad.Object)
		if quad.Graph != nil {
			add(cell, reifyNamespace+"graph", quad.Graph)
		}
	}

	// A named graph holding nothing says something no quad does, so it is
	// given an arc of its own.
	for name := range d.GraphNames() {
		add(scope.New(), reifyNamespace+"names", name)
	}
	return g
}

// TestDecode states what each document means as N-Quads, which is how the W3C
// evaluation tests state it.
func TestDecode(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "an empty document",
			src:  "",
			want: "",
		},
		{
			name: "a statement outside every block is in the default graph",
			src:  "<http://e/s> <http://e/p> <http://e/o> .",
			want: "<http://e/s> <http://e/p> <http://e/o> .",
		},
		{
			name: "a block with no label is the default graph too",
			src:  "{ <http://e/s> <http://e/p> <http://e/o> . }",
			want: "<http://e/s> <http://e/p> <http://e/o> .",
		},
		{
			name: "a labelled block names the graph its statements belong to",
			src:  "<http://e/g> { <http://e/s> <http://e/p> <http://e/o> . }",
			want: "<http://e/s> <http://e/p> <http://e/o> <http://e/g> .",
		},
		{
			name: "the keyword changes nothing about what a block means",
			src:  "GRAPH <http://e/g> { <http://e/s> <http://e/p> <http://e/o> . }",
			want: "<http://e/s> <http://e/p> <http://e/o> <http://e/g> .",
		},
		{
			name: "a blank node may name a graph",
			src:  "_:g { <http://e/s> <http://e/p> <http://e/o> . }",
			want: "<http://e/s> <http://e/p> <http://e/o> _:g .",
		},
		{
			name: "the default graph may be added to from either place",
			src: "<http://e/s> <http://e/p> <http://e/a> .\n" +
				"{ <http://e/s> <http://e/p> <http://e/b> . }\n",
			want: "<http://e/s> <http://e/p> <http://e/a> .\n" +
				"<http://e/s> <http://e/p> <http://e/b> .\n",
		},
		{
			name: "one graph may be written in two blocks",
			src: "<http://e/g> { <http://e/s> <http://e/p> <http://e/a> . }\n" +
				"<http://e/g> { <http://e/s> <http://e/p> <http://e/b> . }\n",
			want: "<http://e/s> <http://e/p> <http://e/a> <http://e/g> .\n" +
				"<http://e/s> <http://e/p> <http://e/b> <http://e/g> .\n",
		},
		{
			name: "an empty block says nothing",
			src:  "<http://e/g> { }",
			want: "",
		},
		{
			name: "a prefix bound before a block applies inside it",
			src: "@prefix ex: <http://e/> .\n" +
				"ex:g { ex:s ex:p ex:o . }\n",
			want: "<http://e/s> <http://e/p> <http://e/o> <http://e/g> .",
		},
		{
			name: "a prefix bound between two blocks applies to the second",
			src: "@prefix ex: <http://e/> .\n" +
				"ex:g { ex:s ex:p ex:o . }\n" +
				"PREFIX f: <http://f/>\n" +
				"f:g { f:s f:p f:o . }\n",
			want: "<http://e/s> <http://e/p> <http://e/o> <http://e/g> .\n" +
				"<http://f/s> <http://f/p> <http://f/o> <http://f/g> .\n",
		},
		{
			name: "a base applies to the labels and the terms of every block",
			src: "@base <http://e/> .\n" +
				"<g> { <s> <p> <o> . }\n",
			want: "<http://e/s> <http://e/p> <http://e/o> <http://e/g> .",
		},
		{
			name: "a second base changes only what follows it",
			src: "@base <http://e/> .\n" +
				"<g> { <s> <p> <o> . }\n" +
				"BASE <http://f/>\n" +
				"<g> { <s> <p> <o> . }\n",
			want: "<http://e/s> <http://e/p> <http://e/o> <http://e/g> .\n" +
				"<http://f/s> <http://f/p> <http://f/o> <http://f/g> .\n",
		},
		{
			name: "one blank node label means one node across graphs",
			src: "<http://e/g> { _:b <http://e/p> <http://e/a> . }\n" +
				"<http://e/h> { _:b <http://e/p> <http://e/b> . }\n",
			want: "_:b <http://e/p> <http://e/a> <http://e/g> .\n" +
				"_:b <http://e/p> <http://e/b> <http://e/h> .\n",
		},
		{
			name: "a node naming a graph is the node standing in it",
			src:  "_:g { _:g <http://e/p> <http://e/o> . }",
			want: "_:g <http://e/p> <http://e/o> _:g .",
		},
		{
			name: "the abbreviations Turtle allows work inside a block",
			src: "@prefix ex: <http://e/> .\n" +
				"ex:g { ex:s a ex:C ; ex:p 1, true, \"v\"@en . }\n",
			want: "<http://e/s> <" + string(vocab.RDFType) + "> <http://e/C> <http://e/g> .\n" +
				`<http://e/s> <http://e/p> "1"^^<` + string(vocab.XSDInteger) + "> <http://e/g> .\n" +
				`<http://e/s> <http://e/p> "true"^^<` + string(vocab.XSDBoolean) + "> <http://e/g> .\n" +
				`<http://e/s> <http://e/p> "v"@en <http://e/g> .` + "\n",
		},
		{
			name: "a collection inside a block expands into that block's graph",
			src:  "<http://e/g> { <http://e/s> <http://e/p> ( <http://e/a> ) . }",
			want: "_:c <" + string(vocab.RDFFirst) + "> <http://e/a> <http://e/g> .\n" +
				"_:c <" + string(vocab.RDFRest) + "> <" + string(vocab.RDFNil) + "> <http://e/g> .\n" +
				"<http://e/s> <http://e/p> _:c <http://e/g> .\n",
		},
		{
			name: "a blank node property list inside a block stays in it",
			src:  "<http://e/g> { <http://e/s> <http://e/p> [ <http://e/q> <http://e/r> ] . }",
			want: "_:n <http://e/q> <http://e/r> <http://e/g> .\n" +
				"<http://e/s> <http://e/p> _:n <http://e/g> .\n",
		},
		{
			name: "a property list standing alone inside a block",
			src:  "<http://e/g> { [ <http://e/p> <http://e/o> ] }",
			want: "_:n <http://e/p> <http://e/o> <http://e/g> .",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := datasetOf(t, decodeTrig(t, tc.src))
			want := datasetOf(t, decodeNQuads(t, tc.want))

			if !isomorphicDatasets(t, got, want) {
				t.Errorf("Decode() =\n%swant\n%s", formatDataset(got), formatDataset(want))
			}
		})
	}
}

// TestDecodeNestsBeforeItEncloses pins the order [trig.Decode] documents: the
// quads of a nested collection or property list come before the quad that uses
// it, and every one of them carries the label of the block it was written in.
func TestDecodeNestsBeforeItEncloses(t *testing.T) {
	quads := decodeTrig(t, "<http://e/g> { <http://e/s> <http://e/p> ( <http://e/a> ) . }")

	if len(quads) != 3 {
		t.Fatalf("Decode() yielded %d quads, want 3:\n%s", len(quads), formatQuads(quads))
	}
	if quads[2].Predicate != "http://e/p" {
		t.Errorf("the enclosing quad came at %d, want last:\n%s", 2, formatQuads(quads))
	}
	for i, quad := range quads {
		if !rdf.IRI("http://e/g").Equal(quad.Graph) {
			t.Errorf("quad %d is in graph %v, want <http://e/g>", i, quad.Graph)
		}
	}
}

func TestDecodeErrors(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want error
	}{
		{
			name: "an undefined prefix on a term",
			src:  "<http://e/g> { ex:s <http://e/p> <http://e/o> . }",
			want: trig.UndefinedPrefixError{Pos: pos(1, 16), Prefix: "ex"},
		},
		{
			name: "an undefined prefix on a graph label",
			src:  "ex:g { <http://e/s> <http://e/p> <http://e/o> . }",
			want: trig.UndefinedPrefixError{Pos: pos(1, 1), Prefix: "ex"},
		},
		{
			name: "a relative label with no base in scope",
			src:  "<g> { <http://e/s> <http://e/p> <http://e/o> . }",
			want: trig.InvalidTermError{Pos: pos(1, 1), Err: trig.ErrNoBase},
		},
		{
			name: "a relative iri with no base in scope",
			src:  "<http://e/g> { <s> <http://e/p> <http://e/o> . }",
			want: trig.InvalidTermError{Pos: pos(1, 16), Err: trig.ErrNoBase},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect2(trig.Decode(strings.NewReader(tc.src)))
			if !errors.Is(err, tc.want) {
				t.Errorf("Decode() = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestDecodeStopsEarly(t *testing.T) {
	var seen int
	for range trig.Decode(strings.NewReader("<http://e/g> { <http://e/s> <http://e/p> <http://e/a>, <http://e/b> . }")) {
		seen++
		break
	}
	if want := 1; seen != want {
		t.Errorf("yielded %d quads after break, want %d", seen, want)
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

func TestDecodeReaderError(t *testing.T) {
	errBoom := errors.New("boom")

	_, err := collect2(trig.Decode(&failingReader{data: []byte("<http://e/g> { "), err: errBoom}))
	if !errors.Is(err, errBoom) {
		t.Errorf("Decode() = %v, want %v", err, errBoom)
	}
}

// TestQuads covers the same lowering over a document already parsed.
func TestQuads(t *testing.T) {
	src := "@prefix ex: <http://e/> .\nex:s ex:p ex:o .\nex:g { ex:s ex:p ex:o . }\n"

	quads, err := trig.Quads(parse(t, src))
	if err != nil {
		t.Fatalf("Quads() = %v, want nil", err)
	}

	got := datasetOf(t, quads)
	want := datasetOf(t, decodeNQuads(t,
		"<http://e/s> <http://e/p> <http://e/o> .\n"+
			"<http://e/s> <http://e/p> <http://e/o> <http://e/g> .\n"))

	if !isomorphicDatasets(t, got, want) {
		t.Errorf("Quads() =\n%swant\n%s", formatDataset(got), formatDataset(want))
	}
}

func TestQuadsNilDocument(t *testing.T) {
	quads, err := trig.Quads(nil)
	if err != nil {
		t.Errorf("Quads(nil) = %v, want nil", err)
	}
	if len(quads) != 0 {
		t.Errorf("Quads(nil) returned %d quads, want none", len(quads))
	}
}

func TestQuadsReportsErrors(t *testing.T) {
	doc := parse(t, "ex:g { <http://e/s> <http://e/p> <http://e/o> . }")

	_, err := trig.Quads(doc)
	want := trig.UndefinedPrefixError{Pos: pos(1, 1), Prefix: "ex"}
	if !errors.Is(err, want) {
		t.Errorf("Quads() = %v, want %v", err, want)
	}
}

// TestDataset covers what only a dataset can say: that a named graph exists
// and holds nothing.
func TestDataset(t *testing.T) {
	src := "@prefix ex: <http://e/> .\n" +
		"ex:s ex:p ex:o .\n" +
		"ex:g { ex:s ex:p ex:o . }\n" +
		"ex:empty { }\n"

	d, err := trig.Dataset(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Dataset() = %v, want nil", err)
	}

	if got, want := d.Default().Len(), 1; got != want {
		t.Errorf("the default graph holds %d triples, want %d", got, want)
	}

	names := slices.Collect(d.GraphNames())
	if len(names) != 2 {
		t.Fatalf("named %d graphs, want 2: %v", len(names), names)
	}

	empty, ok := d.NamedGraph(rdf.IRI("http://e/empty"))
	if !ok {
		t.Fatal("the empty block named no graph, want one")
	}
	if got := empty.Len(); got != 0 {
		t.Errorf("the empty graph holds %d triples, want 0", got)
	}
}

func TestDatasetReportsErrors(t *testing.T) {
	_, err := trig.Dataset(strings.NewReader("ex:g { }"))
	want := trig.UndefinedPrefixError{Pos: pos(1, 1), Prefix: "ex"}
	if !errors.Is(err, want) {
		t.Errorf("Dataset() = %v, want %v", err, want)
	}
}

func TestDatasetReaderError(t *testing.T) {
	errBoom := errors.New("boom")

	_, err := trig.Dataset(&failingReader{data: []byte("<http://e/g> { "), err: errBoom})
	if !errors.Is(err, errBoom) {
		t.Errorf("Dataset() = %v, want %v", err, errBoom)
	}
}

// TestDecodeStreams covers the promise the doc comment makes: the quads of a
// statement are handed on as it is read, not once the document has been.
func TestDecodeStreams(t *testing.T) {
	src := "<http://e/g> { <http://e/s> <http://e/p> <http://e/o> . }\n$"

	var seen int
	for _, err := range trig.Decode(strings.NewReader(src)) {
		if err != nil {
			break
		}
		seen++
	}
	if want := 1; seen != want {
		t.Errorf("yielded %d quads before the error, want %d", seen, want)
	}
}

var _ io.Reader = (*failingReader)(nil)
