package trig_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/rdf11/trig"
	"github.com/z5labs/rdf-go/vocab"
)

func printToString(t *testing.T, doc *trig.Document, opts ...trig.Option) string {
	t.Helper()

	var b strings.Builder
	if err := trig.Print(&b, doc, opts...); err != nil {
		t.Fatalf("Print() = %v, want nil", err)
	}
	return b.String()
}

func encodeToString(t *testing.T, dataset *rdf.Dataset, opts ...trig.Option) string {
	t.Helper()

	var b strings.Builder
	if err := trig.Encode(&b, dataset, opts...); err != nil {
		t.Fatalf("Encode() = %v, want nil", err)
	}
	return b.String()
}

func datasetFromTriG(t *testing.T, src string) *rdf.Dataset {
	t.Helper()

	d, err := trig.Dataset(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Dataset() = %v, want nil", err)
	}
	return d
}

// TestPrint pins the bytes a parsed document prints as, and in particular that
// every form a block was written in is written back as itself.
func TestPrint(t *testing.T) {
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
			name: "a statement outside every block",
			src:  "<http://e/s> <http://e/p> <http://e/o> .",
			want: "<http://e/s> <http://e/p> <http://e/o> .\n",
		},
		{
			name: "a block with no label keeps having none",
			src:  "{ <http://e/s> <http://e/p> <http://e/o> . }",
			want: "{\n    <http://e/s> <http://e/p> <http://e/o> .\n}\n",
		},
		{
			name: "a labelled block is written without a keyword it never had",
			src:  "<http://e/g> { <http://e/s> <http://e/p> <http://e/o> . }",
			want: "<http://e/g> {\n    <http://e/s> <http://e/p> <http://e/o> .\n}\n",
		},
		{
			name: "the keyword is written back in the case it was written in",
			src:  "graph <http://e/g> { }",
			want: "graph <http://e/g> {\n}\n",
		},
		{
			name: "an empty block",
			src:  "<http://e/g> {}",
			want: "<http://e/g> {\n}\n",
		},
		{
			name: "the last dot of a block is supplied",
			src:  "<http://e/g> { <http://e/s> <http://e/p> <http://e/o> }",
			want: "<http://e/g> {\n    <http://e/s> <http://e/p> <http://e/o> .\n}\n",
		},
		{
			name: "several sets of triples, one to a line",
			src: "<http://e/g> { <http://e/s> <http://e/p> <http://e/o> . " +
				"<http://e/t> <http://e/p> <http://e/o> . }",
			want: "<http://e/g> {\n" +
				"    <http://e/s> <http://e/p> <http://e/o> .\n" +
				"    <http://e/t> <http://e/p> <http://e/o> .\n" +
				"}\n",
		},
		{
			name: "the verbs after the first are indented from inside the block",
			src:  "@prefix ex: <http://e/> .\nex:g { ex:s a ex:C ; ex:p 1, 2 . }",
			want: "@prefix ex: <http://e/> .\n" +
				"ex:g {\n" +
				"    ex:s a ex:C ;\n" +
				"        ex:p 1, 2 .\n" +
				"}\n",
		},
		{
			name: "a blank node label on a block",
			src:  "_:g { }",
			want: "_:g {\n}\n",
		},
		{
			name: "an anonymous blank node label on a block",
			src:  "[] { }",
			want: "[] {\n}\n",
		},
		{
			name: "comments are put back between the statements they stood between",
			src:  "# one\n<http://e/g> { }\n# two",
			want: "# one\n<http://e/g> {\n}\n# two\n",
		},
		{
			// The layout a comment inside a block was written against is gone,
			// so it follows the block rather than interrupting it.
			name: "a comment inside a block follows it",
			src:  "<http://e/g> { # inside\n}",
			want: "<http://e/g> {\n}\n# inside\n",
		},
		{
			name: "an indent of nothing puts the braces on their own lines still",
			src:  "<http://e/g> { <http://e/s> <http://e/p> <http://e/o> . }",
			want: "<http://e/g> {\n<http://e/s> <http://e/p> <http://e/o> .\n}\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var opts []trig.Option
			if strings.HasPrefix(tc.name, "an indent of nothing") {
				opts = append(opts, trig.WithIndent(""))
			}

			if got := printToString(t, parse(t, tc.src), opts...); got != tc.want {
				t.Errorf("Print() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPrintRoundTrip covers Parse → Print → Parse, which has to give back the
// tree it started with once positions are set aside. Positions are where the
// document was laid out, and the layout is this package's on the way out.
func TestPrintRoundTrip(t *testing.T) {
	sources := []string{
		"",
		"<http://e/s> <http://e/p> <http://e/o> .",
		"{ <http://e/s> <http://e/p> <http://e/o> . }",
		"{}",
		"<http://e/g> {}",
		"GRAPH <http://e/g> {}",
		"graph <http://e/g> { <http://e/s> <http://e/p> <http://e/o> . }",
		"_:g { _:s <http://e/p> _:o . }",
		"[] { <http://e/s> <http://e/p> <http://e/o> . }",
		"<http://e/g> { <http://e/s> <http://e/p> <http://e/o> }",
		"<http://e/g> { <http://e/s> <http://e/p> <http://e/a> . <http://e/t> <http://e/p> <http://e/b> . }",
		"@prefix ex: <http://e/> .\nex:s ex:p ex:o .\nex:g { ex:s a ex:C ; ex:p 1, 2, 3 . }",
		"@prefix ex: <http://e/> .\nPREFIX f: <http://f/>\nex:g { f:s f:p f:o . }",
		"@base <http://e/> .\nBASE <http://f/>\n<g> { <s> <p> <o> . }",
		"@prefix ex: <http://e/> .\nex:g { ex:s ex:p (1 2 (3 4)) , () . }",
		"@prefix ex: <http://e/> .\nex:g { ex:s ex:p [ ex:q [ ex:r 1 ] ; ex:s 2 ] . }",
		"@prefix ex: <http://e/> .\nex:g { [ ex:p 1 ] . }",
		"@prefix ex: <http://e/> .\nex:g { [] ex:p [] . }",
		"@prefix ex: <http://e/> .\nex:g { ex:s ex:p 42, +42, -0.5, .5, 1e6, 1.0E-6, true, false . }",
		"<http://e/g> { <http://e/s> <http://e/p> \"a\\\"b\", \"\"\"line\nbreak\"\"\", 'single', '''long''' . }",
		`<http://e/g> { <http://e/s> <http://e/p> "v"@en, "v"@en-GB, "1"^^<http://e/dt> . }`,
		"@prefix ex: <http://e/> .\nex:g { ex:a.b ex:p ex:\\-c, ex:d\\., ex:e%20f, ex:_g_ . }",
		"# leading\n<http://e/g> { <http://e/s> <http://e/p> <http://e/o> . }\n# trailing",
		"@prefix ex: <http://e/> .\n" +
			"ex:g { ex:subject ex:predicate ex:objectOne, ex:objectTwo, ex:objectThree, ex:objectFour . }",
		"{ <http://e/s> <http://e/p> <http://e/a> . }\n<http://e/g> { <http://e/s> <http://e/p> <http://e/b> . }\n",
	}

	widths := []int{0, 20, 80}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			for _, width := range widths {
				first := parse(t, src)
				printed := printToString(t, first, trig.WithLineWidth(width))
				second := parse(t, printed)

				zeroDocument(first)
				zeroDocument(second)
				if !reflect.DeepEqual(first, second) {
					t.Errorf(
						"Parse(Print(doc)) differs at width %d\nprinted %q\nfirst  %s\nsecond %s",
						width, printed, formatDocument(first), formatDocument(second),
					)
				}
			}
		})
	}
}

func TestPrintNilDocument(t *testing.T) {
	if err := trig.Print(&strings.Builder{}, nil); !errors.Is(err, trig.ErrNilDocument) {
		t.Errorf("Print(nil) = %v, want %v", err, trig.ErrNilDocument)
	}
}

// failingWriter fails on every write.
type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestPrintWriterError(t *testing.T) {
	errBoom := errors.New("boom")

	doc := parse(t, "@prefix ex: <http://e/> .\nex:g { ex:s a ex:C ; ex:p ( 1 2 ) , [ ex:q 1 ] . }\n# note")
	if err := trig.Print(failingWriter{err: errBoom}, doc, trig.WithLineWidth(10)); !errors.Is(err, errBoom) {
		t.Errorf("Print() = %v, want %v", err, errBoom)
	}
}

func TestEncodeWriterError(t *testing.T) {
	errBoom := errors.New("boom")

	d := datasetFromTriG(t, "<http://e/g> { <http://e/s> <http://e/p> <http://e/o> . }")
	if err := trig.Encode(failingWriter{err: errBoom}, d); !errors.Is(err, errBoom) {
		t.Errorf("Encode() = %v, want %v", err, errBoom)
	}
}

// TestEncode pins what Encode chooses to write for a dataset: where each graph
// goes, and what a graph holding nothing looks like.
func TestEncode(t *testing.T) {
	prefixes := map[string]string{
		"ex":   "http://e/",
		"none": "http://unused/",
	}

	testCases := []struct {
		name   string
		nquads string
		want   string
	}{
		{
			name:   "nothing to say",
			nquads: "",
			want:   "",
		},
		{
			name:   "the default graph stands outside every block",
			nquads: "<http://e/s> <http://e/p> <http://e/o> .",
			want:   "@prefix ex: <http://e/> .\nex:s ex:p ex:o .\n",
		},
		{
			name:   "a named graph becomes a block introduced by its label",
			nquads: "<http://e/s> <http://e/p> <http://e/o> <http://e/g> .",
			want:   "@prefix ex: <http://e/> .\nex:g {\n    ex:s ex:p ex:o .\n}\n",
		},
		{
			name: "the default graph comes before the named ones",
			nquads: "<http://e/s> <http://e/p> <http://e/o> <http://e/g> .\n" +
				"<http://e/s> <http://e/p> <http://e/o> .\n",
			want: "@prefix ex: <http://e/> .\n" +
				"ex:s ex:p ex:o .\n" +
				"ex:g {\n    ex:s ex:p ex:o .\n}\n",
		},
		{
			name: "named graphs keep the order they first arrived in",
			nquads: "<http://e/s> <http://e/p> <http://e/o> <http://e/b> .\n" +
				"<http://e/s> <http://e/p> <http://e/o> <http://e/a> .\n",
			want: "@prefix ex: <http://e/> .\n" +
				"ex:b {\n    ex:s ex:p ex:o .\n}\n" +
				"ex:a {\n    ex:s ex:p ex:o .\n}\n",
		},
		{
			name: "statements about one subject are gathered into one, per graph",
			nquads: "<http://e/s> <http://e/p> <http://e/a> <http://e/g> .\n" +
				"<http://e/s> <http://e/q> <http://e/b> <http://e/g> .\n" +
				"<http://e/s> <http://e/p> <http://e/c> <http://e/g> .\n",
			want: "@prefix ex: <http://e/> .\n" +
				"ex:g {\n" +
				"    ex:s ex:p ex:a, ex:c ;\n" +
				"        ex:q ex:b .\n" +
				"}\n",
		},
		{
			name:   "rdf:type is written a inside a block too",
			nquads: "<http://e/s> <" + string(vocab.RDFType) + "> <http://e/C> <http://e/g> .",
			want:   "@prefix ex: <http://e/> .\nex:g {\n    ex:s a ex:C .\n}\n",
		},
		{
			name:   "a label no prefix matches is written out in full",
			nquads: "<http://e/s> <http://e/p> <http://e/o> <http://other/g> .",
			want:   "@prefix ex: <http://e/> .\n<http://other/g> {\n    ex:s ex:p ex:o .\n}\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := datasetOf(t, decodeNQuads(t, tc.nquads))

			if got := encodeToString(t, d, trig.WithPrefixes(prefixes)); got != tc.want {
				t.Errorf("Encode() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEncodeKeepsEmptyGraphs covers the one thing a dataset can say that a
// sequence of quads cannot.
func TestEncodeKeepsEmptyGraphs(t *testing.T) {
	d := rdf.NewDataset()
	if _, err := d.AddGraph(rdf.IRI("http://e/g")); err != nil {
		t.Fatalf("AddGraph() = %v, want nil", err)
	}

	want := "<http://e/g> {\n}\n"
	if got := encodeToString(t, d); got != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

// TestEncodeBlankNodeGraphLabel covers a graph named by a blank node, whose
// label is kept when TriG can spell it.
func TestEncodeBlankNodeGraphLabel(t *testing.T) {
	d := rdf.NewDataset()
	quad := rdf.Quad{
		Triple: rdf.Triple{
			Subject:   rdf.IRI("http://e/s"),
			Predicate: "http://e/p",
			Object:    rdf.IRI("http://e/o"),
		},
		Graph: rdf.NewBlankNode("g"),
	}
	if err := d.Add(quad); err != nil {
		t.Fatalf("Add() = %v, want nil", err)
	}

	want := "_:g {\n    <http://e/s> <http://e/p> <http://e/o> .\n}\n"
	if got := encodeToString(t, d); got != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

// TestEncodeRelabelsAcrossGraphs covers a blank node mentioned in more than one
// place, which has to be written with one label wherever it stands — as a term
// in either graph, and as a graph's name.
func TestEncodeRelabelsAcrossGraphs(t *testing.T) {
	node := rdf.NewBlankNode("a b")

	d := rdf.NewDataset()
	for _, quad := range []rdf.Quad{
		{Triple: rdf.Triple{Subject: node, Predicate: "http://e/p", Object: node}, Graph: node},
		{Triple: rdf.Triple{Subject: node, Predicate: "http://e/p", Object: node}},
	} {
		if err := d.Add(quad); err != nil {
			t.Fatalf("Add() = %v, want nil", err)
		}
	}

	want := "_:b0 <http://e/p> _:b0 .\n_:b0 {\n    _:b0 <http://e/p> _:b0 .\n}\n"
	if got := encodeToString(t, d); got != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

// TestEncodeRoundTrip covers Decode → Encode → Decode, which has to give back
// the dataset it started with. Blank node labels are not preserved by the
// journey and need not be, so the two are compared by isomorphism.
func TestEncodeRoundTrip(t *testing.T) {
	prefixes := map[string]string{
		"ex":  "http://e/",
		"xsd": "http://www.w3.org/2001/XMLSchema#",
	}

	sources := []string{
		"",
		"<http://e/s> <http://e/p> <http://e/o> .",
		"{ <http://e/s> <http://e/p> <http://e/o> . }",
		"<http://e/g> { <http://e/s> <http://e/p> <http://e/o> . }",
		"GRAPH <http://e/g> { <http://e/s> <http://e/p> <http://e/o> . }",
		"<http://e/g> {}",
		"<http://e/g> {}\n<http://e/h> { <http://e/s> <http://e/p> <http://e/o> . }",
		"_:g { <http://e/s> <http://e/p> <http://e/o> . }",
		"<http://e/s> <http://e/p> <http://e/a> .\n<http://e/g> { <http://e/s> <http://e/p> <http://e/b> . }",
		"<http://e/g> { _:a <http://e/p> _:b . _:b <http://e/p> _:a . }",
		"<http://e/g> { _:a <http://e/p> <http://e/o> . }\n<http://e/h> { _:a <http://e/p> <http://e/o> . }",
		"_:g { _:g <http://e/p> <http://e/o> . }",
		"@prefix ex: <http://e/> .\nex:g { ex:s a ex:C ; ex:p 1, 2, 3 ; ex:q ex:r . }",
		"@prefix ex: <http://e/> .\nex:g { ex:s ex:p (1 2 (3 4)) , () . }",
		"@prefix ex: <http://e/> .\nex:g { ex:s ex:p [ ex:q [ ex:r 1 ] ; ex:s 2 ] . }",
		"@prefix ex: <http://e/> .\nex:g { [] ex:p [] . }",
		"@prefix ex: <http://e/> .\nex:g { ex:s ex:p 42, +42, -0.5, .5, 1e6, 1.0E-6, true, false . }",
		"<http://e/g> { <http://e/s> <http://e/p> \"a\\\"b\", \"\"\"line\nbreak\"\"\", \"tab\\there\" . }",
		`<http://e/g> { <http://e/s> <http://e/p> "v"@en, "v"@en-GB, "1"^^<http://e/dt> . }`,
		"@prefix ex: <http://e/> .\nex:g { ex:a.b ex:p ex:\\-c, ex:d\\., ex:e%20f, ex:_g_ . }",
		`<http://e/g> { <http://e/s> <http://e/p> <http://e/a\u0020b>, <http://e/c\u0009d> . }`,
		"@prefix ex: <http://e/> .\n" +
			"ex:g { ex:subject ex:predicate ex:objectOne, ex:objectTwo, ex:objectThree, ex:objectFour . }",
	}

	widths := []int{0, 20, 80}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			before := datasetFromTriG(t, src)

			for _, width := range widths {
				encoded := encodeToString(t,
					datasetFromTriG(t, src),
					trig.WithPrefixes(prefixes),
					trig.WithLineWidth(width),
				)

				after := datasetFromTriG(t, encoded)
				if !isomorphicDatasets(t, before, after) {
					t.Errorf(
						"Dataset(Encode(d)) is not isomorphic to d at width %d\nencoded %q\nbefore\n%safter\n%s",
						width, encoded, formatDataset(before), formatDataset(after),
					)
				}
			}
		})
	}
}

func TestEncodeErrors(t *testing.T) {
	directional, err := rdf.NewDirectionalLiteral("v", "en", rdf.DirectionLTR)
	if err != nil {
		t.Fatalf("NewDirectionalLiteral() = %v, want nil", err)
	}

	testCases := []struct {
		name    string
		dataset func(*testing.T) *rdf.Dataset
		opts    []trig.Option
		want    error
	}{
		{
			name:    "a nil dataset",
			dataset: func(*testing.T) *rdf.Dataset { return nil },
			want:    trig.ErrNilDataset,
		},
		{
			name:    "a prefix that is not a PN_PREFIX",
			dataset: func(*testing.T) *rdf.Dataset { return rdf.NewDataset() },
			opts:    []trig.Option{trig.WithPrefixes(map[string]string{"1ex": "http://e/"})},
			want:    trig.ErrInvalidPrefix,
		},
		{
			name:    "a namespace that is not absolute",
			dataset: func(*testing.T) *rdf.Dataset { return rdf.NewDataset() },
			opts:    []trig.Option{trig.WithPrefixes(map[string]string{"ex": "/relative"})},
			want:    trig.ErrInvalidPrefix,
		},
		{
			name: "a literal carrying a base direction",
			dataset: func(t *testing.T) *rdf.Dataset {
				return datasetOf(t, []rdf.Quad{{
					Triple: rdf.Triple{
						Subject:   rdf.IRI("http://e/s"),
						Predicate: "http://e/p",
						Object:    directional,
					},
					Graph: rdf.IRI("http://e/g"),
				}})
			},
			want: trig.ErrBaseDirection,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var b strings.Builder

			err := trig.Encode(&b, tc.dataset(t), tc.opts...)
			if !errors.Is(err, tc.want) {
				t.Errorf("Encode() = %v, want %v", err, tc.want)
			}
			if got := b.String(); got != "" {
				t.Errorf("wrote %q before failing, want nothing", got)
			}
		})
	}
}

// TestPrintSuppliesMissingKeywords covers a tree built by hand rather than by
// [trig.Parse], where a directive may carry no keyword at all. Writing the
// directive without one would produce a document this package could not read
// back, so the '@' form is supplied.
func TestPrintSuppliesMissingKeywords(t *testing.T) {
	doc := &trig.Document{
		Statements: []trig.Statement{
			&trig.BaseDirective{IRI: &trig.IRIRef{Value: "http://e/"}},
			&trig.PrefixDirective{Prefix: "ex", IRI: &trig.IRIRef{Value: "http://e/"}},
			&trig.GraphBlock{Label: &trig.IRIRef{Value: "http://e/g"}},
		},
	}

	want := "@base <http://e/> .\n@prefix ex: <http://e/> .\n<http://e/g> {\n}\n"
	if got := printToString(t, doc); got != want {
		t.Errorf("Print() = %q, want %q", got, want)
	}
	if _, err := trig.Parse(strings.NewReader(want)); err != nil {
		t.Errorf("Parse(Print(doc)) = %v, want nil", err)
	}
}

// zeroDocument clears every position in the tree, so that two trees can be
// compared for what they say rather than for where it was written.
func zeroDocument(doc *trig.Document) {
	doc.Pos = trig.Pos{}
	for _, comment := range doc.Comments {
		comment.Pos = trig.Pos{}
	}
	for _, statement := range doc.Statements {
		zeroStatement(statement)
	}
}

func zeroStatement(s trig.Statement) {
	switch n := s.(type) {
	case *trig.PrefixDirective:
		n.Pos = trig.Pos{}
		zeroTerm(n.IRI)
	case *trig.BaseDirective:
		n.Pos = trig.Pos{}
		zeroTerm(n.IRI)
	case *trig.Triples:
		n.Pos = trig.Pos{}
		zeroTerm(n.Subject)
		zeroPredicates(n.Predicates)
	case *trig.GraphBlock:
		n.Pos = trig.Pos{}
		if n.Label != nil {
			zeroTerm(n.Label)
		}
		for _, t := range n.Triples {
			zeroStatement(t)
		}
	}
}

func zeroPredicates(list []*trig.PredicateObject) {
	for _, po := range list {
		po.Pos = trig.Pos{}
		zeroTerm(po.Verb)
		for _, object := range po.Objects {
			zeroTerm(object)
		}
	}
}

func zeroTerm(node trig.Term) {
	switch n := node.(type) {
	case *trig.IRIRef:
		n.Pos = trig.Pos{}
	case *trig.PrefixedName:
		n.Pos = trig.Pos{}
	case *trig.A:
		n.Pos = trig.Pos{}
	case *trig.BlankNode:
		n.Pos = trig.Pos{}
	case *trig.Anon:
		n.Pos = trig.Pos{}
	case *trig.Literal:
		n.Pos = trig.Pos{}
		if n.Datatype != nil {
			zeroTerm(n.Datatype)
		}
	case *trig.Collection:
		n.Pos = trig.Pos{}
		for _, object := range n.Objects {
			zeroTerm(object)
		}
	case *trig.BlankNodePropertyList:
		n.Pos = trig.Pos{}
		zeroPredicates(n.Predicates)
	}
}

// formatDocument renders a tree for a failure message, since a tree of
// pointers prints as addresses.
func formatDocument(doc *trig.Document) string {
	var b strings.Builder
	if err := trig.Print(&b, doc); err != nil {
		return "<unprintable: " + err.Error() + ">"
	}
	return b.String()
}
