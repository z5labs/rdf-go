package turtle_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/rdf12/turtle"
	"github.com/z5labs/rdf-go/vocab"
)

func printToString(t *testing.T, doc *turtle.Document, opts ...turtle.Option) string {
	t.Helper()

	var b strings.Builder
	if err := turtle.Print(&b, doc, opts...); err != nil {
		t.Fatalf("Print() = %v, want nil", err)
	}
	return b.String()
}

func encodeToString(t *testing.T, triples []rdf.Triple, opts ...turtle.Option) string {
	t.Helper()

	var b strings.Builder
	if err := turtle.Encode(&b, slices.Values(triples), opts...); err != nil {
		t.Fatalf("Encode() = %v, want nil", err)
	}
	return b.String()
}

// TestPrint pins the bytes a parsed document prints as.
//
// What is pinned is the layout this package chose — a statement per subject,
// the verbs after the first one to a line — and, more importantly, that every
// RDF 1.2 construct is written back as the construct it was rather than as
// what it stands for: a reified triple does not become an rdf:reifies triple,
// an annotation does not move out of the object it was written after, and a
// triple term does not become either of the other two.
func TestPrint(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "a triple term object",
			src:  prefixed + ":s :p <<( :a :b :c )>> .",
			want: prefixed + ":s :p <<( :a :b :c )>> .\n",
		},
		{
			name: "a nested triple term",
			src:  prefixed + ":s :p <<( :a :b <<( :d :e :f )>> )>> .",
			want: prefixed + ":s :p <<( :a :b <<( :d :e :f )>> )>> .\n",
		},
		{
			name: "the a keyword inside a triple term",
			src:  prefixed + ":s :p <<( :a a :C )>> .",
			want: prefixed + ":s :p <<( :a a :C )>> .\n",
		},
		{
			name: "a reified triple with no reifier",
			src:  prefixed + "<< :a :b :c >> :p :o .",
			want: prefixed + "<< :a :b :c >> :p :o .\n",
		},
		{
			name: "a reified triple with a named reifier",
			src:  prefixed + "<< :a :b :c ~ :r >> :p :o .",
			want: prefixed + "<< :a :b :c ~ :r >> :p :o .\n",
		},
		{
			name: "a reified triple with a bare reifier",
			src:  prefixed + "<< :a :b :c ~ >> :p :o .",
			want: prefixed + "<< :a :b :c ~ >> :p :o .\n",
		},
		{
			name: "a reified triple standing alone",
			src:  prefixed + "<< :a :b :c >> .",
			want: prefixed + "<< :a :b :c >> .\n",
		},
		{
			name: "a reified triple in the object position",
			src:  prefixed + ":s :p << :a :b :c ~ :r >> .",
			want: prefixed + ":s :p << :a :b :c ~ :r >> .\n",
		},
		{
			name: "an annotation block",
			src:  prefixed + ":s :p :o {| :q :v |} .",
			want: prefixed + ":s :p :o {| :q :v |} .\n",
		},
		{
			name: "a reifier before an annotation block",
			src:  prefixed + ":s :p :o ~ :r {| :q :v |} .",
			want: prefixed + ":s :p :o ~ :r {| :q :v |} .\n",
		},
		{
			name: "a reifier after an annotation block",
			src:  prefixed + ":s :p :o {| :q :v |} ~ :r .",
			want: prefixed + ":s :p :o {| :q :v |} ~ :r .\n",
		},
		{
			name: "an annotation on the object it was written after",
			src:  prefixed + ":s :p :o1, :o2 {| :q :v |} .",
			want: prefixed + ":s :p :o1, :o2 {| :q :v |} .\n",
		},
		{
			name: "an annotation block holding a list of its own",
			src:  prefixed + ":s :p :o {| :q :v1, :v2 ; :r :w |} .",
			want: prefixed + ":s :p :o {| :q :v1, :v2 ; :r :w |} .\n",
		},
		{
			name: "a directional literal",
			src:  prefixed + `:s :p "v"@en--ltr, "w"@ar--rtl, "x"@en .`,
			want: prefixed + `:s :p "v"@en--ltr, "w"@ar--rtl, "x"@en .` + "\n",
		},
		{
			name: "a version directive in each form",
			src:  "@version \"1.2\" .\nVERSION \"1.2\"\n" + prefixed + ":s :p :o .",
			want: "@version \"1.2\" .\nVERSION \"1.2\"\n" + prefixed + ":s :p :o .\n",
		},
		{
			name: "a version specifier written with single quotes",
			src:  "VERSION '1.2'\n" + prefixed + ":s :p :o .",
			want: "VERSION \"1.2\"\n" + prefixed + ":s :p :o .\n",
		},
		{
			name: "a version directive between statements",
			src:  prefixed + ":s :p :o .\n@version \"1.2\" .\n:s :q :v .",
			want: prefixed + ":s :p :o .\n@version \"1.2\" .\n:s :q :v .\n",
		},
		{
			name: "the RDF 1.1 constructs the printer still writes",
			src:  prefixed + ":s a :C ; :p 1, 2 ; :q ( 1 ( 2 ) ) , [ :r 3 ] , [] , _:b .",
			want: prefixed + ":s a :C ;\n    :p 1, 2 ;\n    :q ( 1 ( 2 ) ), [ :r 3 ], [], _:b .\n",
		},
		{
			name: "comments between statements",
			src:  "# leading\n" + prefixed + ":s :p :o {| :q :v |} . # trailing\n# last",
			want: "# leading\n" + prefixed + ":s :p :o {| :q :v |} .\n# trailing\n# last\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := printToString(t, parse(t, tc.src))
			if got != tc.want {
				t.Errorf("Print() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPrintWrapsLongLines covers what the printer breaks and what it refuses
// to break.
//
// An annotation block is a predicate object list and breaks like one. A triple
// term and a reified triple are one statement standing where a term is wanted,
// and are written whole however long they are.
func TestPrintWrapsLongLines(t *testing.T) {
	testCases := []struct {
		name  string
		src   string
		width int
		want  string
	}{
		{
			name:  "an annotation block breaks like a property list",
			src:   prefixed + ":s :p :o ~ :r {| :aLongVerb :aLongObject ; :anotherVerb :anotherObject |} .",
			width: 40,
			want: prefixed + ":s :p :o ~ :r {|\n" +
				"        :aLongVerb :aLongObject ;\n" +
				"        :anotherVerb :anotherObject\n" +
				"    |} .\n",
		},
		{
			name:  "a triple term is written whole",
			src:   prefixed + ":s :p <<( :aLongSubject :aLongPredicate :aLongObject )>> .",
			width: 20,
			want:  prefixed + ":s :p <<( :aLongSubject :aLongPredicate :aLongObject )>> .\n",
		},
		{
			name:  "a reified triple is written whole",
			src:   prefixed + ":s :p << :aLongSubject :aLongPredicate :aLongObject ~ :r >> .",
			width: 20,
			want:  prefixed + ":s :p << :aLongSubject :aLongPredicate :aLongObject ~ :r >> .\n",
		},
		{
			name:  "an object list wraps with its annotation attached",
			src:   prefixed + ":s :p :objectOne {| :q :v |}, :objectTwo {| :q :w |} .",
			width: 30,
			want: prefixed + ":s :p :objectOne {| :q :v |},\n" +
				"        :objectTwo {| :q :w |} .\n",
		},
		{
			name:  "wrapping is off at a width of zero",
			src:   prefixed + ":s :p :objectOne {| :q :v |}, :objectTwo {| :q :w |} .",
			width: 0,
			want:  prefixed + ":s :p :objectOne {| :q :v |}, :objectTwo {| :q :w |} .\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := printToString(t, parse(t, tc.src), turtle.WithLineWidth(tc.width))
			if got != tc.want {
				t.Errorf("Print() = %q, want %q", got, tc.want)
			}

			// Whatever the layout, the document has to still mean what it did.
			before := graphOfTriples(t, decodeTurtle(t, tc.src))
			after := graphOfTriples(t, decodeTurtle(t, got))
			if !rdf.Isomorphic(before, after) {
				t.Errorf("wrapping changed the graph:\nbefore\n%safter\n%s", formatGraph(before), formatGraph(after))
			}
		})
	}
}

// TestPrintRoundTrip covers Parse → Print → Parse, which has to give back the
// tree it started with once positions are set aside. Positions are where the
// document was laid out, and the layout is this package's on the way out.
//
// The sources between them write every construct RDF 1.2 adds — both bracketed
// forms, a reifier in each of its three spellings, an annotation block before
// and after a reifier, a version directive in both forms, and a directional
// literal — alongside everything RDF 1.1 could write.
func TestPrintRoundTrip(t *testing.T) {
	sources := []string{
		"",
		`<http://e/s> <http://e/p> <http://e/o> .`,
		"@prefix ex: <http://e/> .\nPREFIX f: <http://f/>\nex:s f:p ex:o .",
		"@base <http://e/> .\nBASE <http://f/>\n<s> <p> <o> .",
		"@version \"1.2\" .\nVERSION \"1.2\"\n<http://e/s> <http://e/p> <http://e/o> .",
		"@prefix ex: <http://e/> .\nex:s a ex:C ; ex:p 1, 2, 3 ; ex:q ex:r .",
		"@prefix ex: <http://e/> .\nex:s ex:p (1 2 (3 4)) , () .",
		"@prefix ex: <http://e/> .\nex:s ex:p [ ex:q [ ex:r 1 ] ; ex:s 2 ] .",
		"@prefix ex: <http://e/> .\n[ ex:p 1 ] .",
		"@prefix ex: <http://e/> .\n[] ex:p [] .",
		"_:a <http://e/p> _:b .",
		"@prefix ex: <http://e/> .\nex:s ex:p 42, +42, -0.5, .5, 1e6, 1.0E-6, true, false .",
		"<http://e/s> <http://e/p> \"a\\\"b\", \"\"\"line\nbreak\"\"\", 'single', '''long''' .",
		`<http://e/s> <http://e/p> "v"@en, "v"@en-GB, "1"^^<http://e/dt> .`,
		"@prefix ex: <http://e/> .\nex:a.b ex:p ex:\\-c, ex:d\\., ex:e%20f, ex:_g_ .",
		"# leading\n<http://e/s> # inside\n<http://e/p> <http://e/o> . # trailing\n# last",

		// The RDF 1.2 constructs.
		"@prefix ex: <http://e/> .\nex:s ex:p <<( ex:a ex:b ex:c )>> .",
		"@prefix ex: <http://e/> .\nex:s ex:p <<( ex:a ex:b <<( ex:d ex:e \"x\" )>> )>> .",
		"@prefix ex: <http://e/> .\nex:s ex:p <<( _:a a ex:C )>> , <<( [] ex:b 1 )>> .",
		"@prefix ex: <http://e/> .\n<< ex:a ex:b ex:c >> ex:p ex:o .",
		"@prefix ex: <http://e/> .\n<< ex:a ex:b ex:c ~ ex:r >> ex:p ex:o .",
		"@prefix ex: <http://e/> .\n<< ex:a ex:b ex:c ~ >> ex:p ex:o .",
		"@prefix ex: <http://e/> .\n<< ex:a ex:b ex:c ~ _:r >> .",
		"@prefix ex: <http://e/> .\nex:s ex:p << ex:a ex:b << ex:c ex:d ex:e >> >> .",
		"@prefix ex: <http://e/> .\nex:s ex:p << << ex:a ex:b ex:c >> ex:d ex:e >> .",
		"@prefix ex: <http://e/> .\nex:s ex:p ex:o {| ex:q ex:v |} .",
		"@prefix ex: <http://e/> .\nex:s ex:p ex:o ~ ex:r {| ex:q ex:v |} .",
		"@prefix ex: <http://e/> .\nex:s ex:p ex:o {| ex:q ex:v |} ~ ex:r .",
		"@prefix ex: <http://e/> .\nex:s ex:p ex:o ~ {| ex:q ex:v |} {| ex:x ex:y |} .",
		"@prefix ex: <http://e/> .\nex:s ex:p ex:o1 {| ex:q 1 |} , ex:o2 ~ ex:r .",
		"@prefix ex: <http://e/> .\nex:s ex:p ex:o {| ex:q ex:v {| ex:deep 1 |} ; ex:r 2 |} .",
		"@prefix ex: <http://e/> .\nex:s ex:p \"v\"@en--ltr, \"w\"@ar--rtl .",
		"@prefix ex: <http://e/> .\nex:s ex:p ex:aVeryLongObjectName ~ ex:aVeryLongReifierName {| ex:aLongVerb ex:aLongObject ; ex:another ex:one |} .",
	}

	widths := []int{0, 20, 80}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			for _, width := range widths {
				first := parse(t, src)
				printed := printToString(t, first, turtle.WithLineWidth(width))
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
	if err := turtle.Print(&strings.Builder{}, nil); !errors.Is(err, turtle.ErrNilDocument) {
		t.Errorf("Print(nil) = %v, want %v", err, turtle.ErrNilDocument)
	}
}

// failingWriter fails on every write.
type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestPrintWriterError(t *testing.T) {
	errBoom := errors.New("boom")

	doc := parse(t, prefixed+":s a :C ; :p ( 1 2 ) , [ :q 1 ] , <<( :a :b :c )>> ~ :r {| :x :y |} .\n# note")
	err := turtle.Print(failingWriter{err: errBoom}, doc, turtle.WithLineWidth(10))
	if !errors.Is(err, errBoom) {
		t.Errorf("Print() = %v, want %v", err, errBoom)
	}
}

func TestEncodeWriterError(t *testing.T) {
	errBoom := errors.New("boom")

	triples := decodeTurtle(t, `<http://e/s> <http://e/p> <http://e/o> .`)
	err := turtle.Encode(failingWriter{err: errBoom}, slices.Values(triples))
	if !errors.Is(err, errBoom) {
		t.Errorf("Encode() = %v, want %v", err, errBoom)
	}
}

// TestPrintSuppliesMissingKeywords covers a tree built by hand rather than
// parsed, whose directives name no keyword. The '@' forms are the defaults,
// each written with the '.' the grammar gives it.
func TestPrintSuppliesMissingKeywords(t *testing.T) {
	doc := &turtle.Document{
		Statements: []turtle.Statement{
			&turtle.BaseDirective{IRI: &turtle.IRIRef{Value: "http://e/"}},
			&turtle.PrefixDirective{Prefix: "ex", IRI: &turtle.IRIRef{Value: "http://e/"}},
			&turtle.VersionDirective{Version: "1.2"},
		},
	}

	want := "@base <http://e/> .\n@prefix ex: <http://e/> .\n@version \"1.2\" .\n"
	if got := printToString(t, doc); got != want {
		t.Errorf("Print() = %q, want %q", got, want)
	}
}

// TestEncode pins what Encode chooses to write for a graph: the grouping, the
// shorthands, the prefixes it declares, and the RDF 1.2 terms the RDF 1.1
// encoder has to refuse.
func TestEncode(t *testing.T) {
	prefixes := map[string]string{
		"ex":   "http://e/",
		"xsd":  "http://www.w3.org/2001/XMLSchema#",
		"rdf":  rdf.NamespaceRDF,
		"none": "http://unused/",
	}

	testCases := []struct {
		name     string
		ntriples string
		want     string
	}{
		{
			name:     "nothing to say",
			ntriples: "",
			want:     "",
		},
		{
			name:     "an iri is abbreviated against the prefixes it was given",
			ntriples: "<http://e/s> <http://e/p> <http://e/o> .",
			want:     "@prefix ex: <http://e/> .\nex:s ex:p ex:o .\n",
		},
		{
			name:     "statements about one subject are gathered into one",
			ntriples: "<http://e/s> <http://e/p> <http://e/o> .\n<http://e/s> <http://e/q> <http://e/v> .",
			want:     "@prefix ex: <http://e/> .\nex:s ex:p ex:o ;\n    ex:q ex:v .\n",
		},
		{
			name:     "a triple term object",
			ntriples: "<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> <http://e/c> )>> .",
			want:     "@prefix ex: <http://e/> .\nex:s ex:p <<( ex:a ex:b ex:c )>> .\n",
		},
		{
			name: "a nested triple term",
			ntriples: "<http://e/s> <http://e/p> " +
				"<<( <http://e/a> <http://e/b> <<( <http://e/d> <http://e/e> \"x\" )>> )>> .",
			want: "@prefix ex: <http://e/> .\nex:s ex:p <<( ex:a ex:b <<( ex:d ex:e \"x\" )>> )>> .\n",
		},
		{
			name: "rdf:type inside a triple term is written a",
			ntriples: "<http://e/s> <http://e/p> " +
				"<<( <http://e/a> <" + rdf.NamespaceRDF + "type> <http://e/C> )>> .",
			want: "@prefix ex: <http://e/> .\nex:s ex:p <<( ex:a a ex:C )>> .\n",
		},
		{
			name:     "a directional literal",
			ntriples: `<http://e/s> <http://e/p> "v"@en--ltr .`,
			want:     "@prefix ex: <http://e/> .\nex:s ex:p \"v\"@en--ltr .\n",
		},
		{
			name:     "a language literal keeps its tag and nothing more",
			ntriples: `<http://e/s> <http://e/p> "v"@en .`,
			want:     "@prefix ex: <http://e/> .\nex:s ex:p \"v\"@en .\n",
		},
		{
			name:     "the shorthands a literal may be written in",
			ntriples: "<http://e/s> <http://e/p> \"42\"^^<http://www.w3.org/2001/XMLSchema#integer> .",
			want:     "@prefix ex: <http://e/> .\nex:s ex:p 42 .\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := encodeToString(t, decodeNTriples(t, tc.ntriples), turtle.WithPrefixes(prefixes))
			if got != tc.want {
				t.Errorf("Encode() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEncodeBlankNodeInsideATripleTerm covers a blank node standing in a term
// rather than in a statement, which the encoder relabels through the same
// mapping either way: two mentions of one node stay one node however deeply
// either is nested.
//
// The triples are built by hand rather than decoded, so that the labels are the
// test's and not a scope's, which mints its own.
func TestEncodeBlankNodeInsideATripleTerm(t *testing.T) {
	triples := []rdf.Triple{
		{
			Subject:   rdf.IRI("http://e/s"),
			Predicate: "http://e/p",
			Object: rdf.TripleTerm{
				Subject:   rdf.NewBlankNode("a"),
				Predicate: "http://e/b",
				Object:    rdf.NewBlankNode("c"),
			},
		},
		{Subject: rdf.NewBlankNode("a"), Predicate: "http://e/q", Object: rdf.IRI("http://e/v")},
	}

	want := "@prefix ex: <http://e/> .\n" +
		"ex:s ex:p <<( _:a ex:b _:c )>> .\n" +
		"_:a ex:q ex:v .\n"

	got := encodeToString(t, triples, turtle.WithPrefixes(map[string]string{"ex": "http://e/"}))
	if got != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

// TestEncodeWritesReificationExplicitly pins the choice Encode's documentation
// makes: a reification is written as the triples it is, never as one of the two
// sugars that would say the same thing.
//
// The graph below is the one every spelling agrees on — the triple, its
// reification, and something said about the reifier — so it is the case where
// ":s :p :o ~ :r {| :q :v |} ." would have been available and is not taken.
func TestEncodeWritesReificationExplicitly(t *testing.T) {
	prefixes := map[string]string{
		"ex":  "http://e/",
		"rdf": rdf.NamespaceRDF,
	}

	ntriples := "<http://e/s> <http://e/p> <http://e/o> .\n" +
		"<http://e/r> <" + rdf.NamespaceRDF + "reifies> <<( <http://e/s> <http://e/p> <http://e/o> )>> .\n" +
		"<http://e/r> <http://e/q> <http://e/v> .\n"

	want := "@prefix ex: <http://e/> .\n" +
		"@prefix rdf: <" + rdf.NamespaceRDF + "> .\n" +
		"ex:s ex:p ex:o .\n" +
		"ex:r rdf:reifies <<( ex:s ex:p ex:o )>> ;\n" +
		"    ex:q ex:v .\n"

	got := encodeToString(t, decodeNTriples(t, ntriples), turtle.WithPrefixes(prefixes))
	if got != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

// TestEncodeRoundTrip covers Decode → Encode → Decode, which has to give back
// the graph it started with. Blank node labels are not preserved by the
// journey and need not be, so the two are compared by isomorphism.
//
// [rdf.Isomorphic] compares blank nodes inside a triple term by label rather
// than mapping them, and the labels a scope mints differ between two reads of
// one document, so the sources that write a triple term name every node inside
// one. What is being checked there is the syntax the encoder chose, not the
// labelling, which the sources without triple terms cover.
func TestEncodeRoundTrip(t *testing.T) {
	prefixes := map[string]string{
		"ex":  "http://e/",
		"xsd": "http://www.w3.org/2001/XMLSchema#",
		"rdf": rdf.NamespaceRDF,
	}

	sources := []string{
		"",
		`<http://e/s> <http://e/p> <http://e/o> .`,
		"@prefix ex: <http://e/> .\nex:s a ex:C ; ex:p 1, 2, 3 ; ex:q ex:r .",
		"@prefix ex: <http://e/> .\nex:s ex:p (1 2 (3 4)) , () .",
		"@prefix ex: <http://e/> .\nex:s ex:p [ ex:q [ ex:r 1 ] ; ex:s 2 ] .",
		"@prefix ex: <http://e/> .\n[] ex:p [] .",
		"_:a <http://e/p> _:b . _:b <http://e/p> _:a .",
		"@prefix ex: <http://e/> .\nex:s ex:p 42, +42, -0.5, .5, 1e6, 1.0E-6, true, false .",
		"<http://e/s> <http://e/p> \"a\\\"b\", \"\"\"line\nbreak\"\"\", \"tab\\there\" .",
		`<http://e/s> <http://e/p> "v"@en, "v"@en-GB, "1"^^<http://e/dt> .`,
		"@prefix ex: <http://e/> .\nex:a.b ex:p ex:\\-c, ex:d\\., ex:e%20f, ex:_g_ .",
		// The local parts here are ones PN_LOCAL has no room for and
		// PN_LOCAL_ESC cannot rescue, so both have to come back as an IRIREF
		// written out in full.
		"<http://e/s> <http://e/p> <http://e/a[b>, <http://e/c]d> .",
		"@version \"1.2\" .\n<http://e/s> <http://e/p> <http://e/o> .",

		// The RDF 1.2 constructs, every blank node inside a triple term named.
		"@prefix ex: <http://e/> .\nex:s ex:p \"v\"@en--ltr, \"w\"@ar--rtl .",
		"@prefix ex: <http://e/> .\nex:s ex:p <<( ex:a ex:b ex:c )>> .",
		"@prefix ex: <http://e/> .\nex:s ex:p <<( ex:a ex:b <<( ex:d ex:e \"x\"@en--rtl )>> )>> .",
		"@prefix ex: <http://e/> .\nex:s ex:p <<( ex:a a ex:C )>> .",
		"@prefix ex: <http://e/> .\n<< ex:a ex:b ex:c ~ ex:r >> ex:p ex:o .",
		"@prefix ex: <http://e/> .\n<< ex:a ex:b ex:c ~ ex:r >> .",
		"@prefix ex: <http://e/> .\nex:s ex:p ex:o ~ ex:r {| ex:q ex:v |} .",
		"@prefix ex: <http://e/> .\nex:s ex:p ex:o1 ~ ex:r1 {| ex:q 1 |} , ex:o2 ~ ex:r2 {| ex:q 2 |} .",
		"@prefix ex: <http://e/> .\nex:s ex:p ex:o ~ ex:r {| ex:q ex:v ~ ex:r2 {| ex:deep 1 |} |} .",
	}

	widths := []int{0, 20, 80}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			before := graphOfTriples(t, decodeTurtle(t, src))

			for _, width := range widths {
				encoded := encodeToString(t,
					decodeTurtle(t, src),
					turtle.WithPrefixes(prefixes),
					turtle.WithLineWidth(width),
				)

				after := graphOfTriples(t, decodeTurtle(t, encoded))
				if !rdf.Isomorphic(before, after) {
					t.Errorf(
						"Decode(Encode(g)) is not isomorphic to g at width %d\nencoded %q\nbefore\n%safter\n%s",
						width, encoded, formatGraph(before), formatGraph(after),
					)
				}
			}
		})
	}
}

func TestEncodeErrors(t *testing.T) {
	testCases := []struct {
		name    string
		triples []rdf.Triple
		opts    []turtle.Option
		want    error
	}{
		{
			name:    "a prefix that is not a PN_PREFIX",
			opts:    []turtle.Option{turtle.WithPrefixes(map[string]string{"1ex": "http://e/"})},
			triples: nil,
			want:    turtle.ErrInvalidPrefix,
		},
		{
			name:    "a prefix ending in a dot",
			opts:    []turtle.Option{turtle.WithPrefixes(map[string]string{"ex.": "http://e/"})},
			triples: nil,
			want:    turtle.ErrInvalidPrefix,
		},
		{
			name:    "a prefix holding a character the grammar has no room for",
			opts:    []turtle.Option{turtle.WithPrefixes(map[string]string{"e x": "http://e/"})},
			triples: nil,
			want:    turtle.ErrInvalidPrefix,
		},
		{
			name:    "a namespace that is not absolute",
			opts:    []turtle.Option{turtle.WithPrefixes(map[string]string{"ex": "/relative"})},
			triples: nil,
			want:    turtle.ErrInvalidPrefix,
		},
		{
			name: "a literal in the subject position",
			triples: []rdf.Triple{
				{Subject: rdf.NewLiteral("nope"), Predicate: "http://e/p", Object: rdf.IRI("http://e/o")},
			},
			want: rdf.ErrInvalidSubject,
		},
		{
			name: "a triple term in the subject position",
			triples: []rdf.Triple{
				{
					Subject: rdf.TripleTerm{
						Subject:   rdf.IRI("http://e/s2"),
						Predicate: "http://e/p2",
						Object:    rdf.IRI("http://e/o2"),
					},
					Predicate: "http://e/p",
					Object:    rdf.IRI("http://e/o"),
				},
			},
			want: rdf.ErrInvalidSubject,
		},
		{
			name: "a triple term whose own subject is a triple term",
			triples: []rdf.Triple{
				{
					Subject:   rdf.IRI("http://e/s"),
					Predicate: "http://e/p",
					Object: rdf.TripleTerm{
						Subject: rdf.TripleTerm{
							Subject:   rdf.IRI("http://e/a"),
							Predicate: "http://e/b",
							Object:    rdf.IRI("http://e/c"),
						},
						Predicate: "http://e/p2",
						Object:    rdf.IRI("http://e/o2"),
					},
				},
			},
			want: rdf.ErrInvalidSubject,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var b strings.Builder

			err := turtle.Encode(&b, slices.Values(tc.triples), tc.opts...)
			if !errors.Is(err, tc.want) {
				t.Errorf("Encode() = %v, want %v", err, tc.want)
			}
			if got := b.String(); got != "" {
				t.Errorf("wrote %q before failing, want nothing", got)
			}
		})
	}
}

// TestEncodeAcceptsTheEmptyPrefix covers the one label PN_PREFIX does not
// admit and PNAME_NS does.
func TestEncodeAcceptsTheEmptyPrefix(t *testing.T) {
	triples := decodeNTriples(t, "<http://e/s> <http://e/p> <http://e/o> .")

	want := "@prefix : <http://e/> .\n:s :p :o .\n"
	got := encodeToString(t, triples, turtle.WithPrefixes(map[string]string{"": "http://e/"}))
	if got != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

// TestEncodeShorthandLexicalForms covers the productions a literal has to
// match before it may be written without quotes. The datatype is not enough:
// what is written has to read back as the literal it was written for.
func TestEncodeShorthandLexicalForms(t *testing.T) {
	testCases := []struct {
		datatype rdf.IRI
		value    string
		want     string
	}{
		{datatype: vocab.XSDInteger, value: "42", want: "42"},
		{datatype: vocab.XSDInteger, value: "-0042", want: "-0042"},
		{datatype: vocab.XSDInteger, value: "4.2", want: `"4.2"^^<http://www.w3.org/2001/XMLSchema#integer>`},
		{datatype: vocab.XSDDecimal, value: ".5", want: ".5"},
		{datatype: vocab.XSDDecimal, value: "5", want: `"5"^^<http://www.w3.org/2001/XMLSchema#decimal>`},
		{datatype: vocab.XSDDouble, value: "1.0E-6", want: "1.0E-6"},
		{datatype: vocab.XSDDouble, value: "1.0", want: `"1.0"^^<http://www.w3.org/2001/XMLSchema#double>`},
		{datatype: vocab.XSDBoolean, value: "true", want: "true"},
		{datatype: vocab.XSDBoolean, value: "TRUE", want: `"TRUE"^^<http://www.w3.org/2001/XMLSchema#boolean>`},
	}

	for _, tc := range testCases {
		t.Run(tc.value+" as "+string(tc.datatype), func(t *testing.T) {
			literal, err := rdf.NewTypedLiteral(tc.value, tc.datatype)
			if err != nil {
				t.Fatalf("NewTypedLiteral() = %v, want nil", err)
			}

			triples := []rdf.Triple{
				{Subject: rdf.IRI("http://e/s"), Predicate: "http://e/p", Object: literal},
			}

			want := "<http://e/s> <http://e/p> " + tc.want + " .\n"
			if got := encodeToString(t, triples); got != want {
				t.Errorf("Encode() = %q, want %q", got, want)
			}
		})
	}
}

// zeroDocument clears every position in the tree, so that two trees can be
// compared for what they say rather than for where it was written.
func zeroDocument(doc *turtle.Document) {
	doc.Pos = turtle.Pos{}
	for _, comment := range doc.Comments {
		comment.Pos = turtle.Pos{}
	}
	for _, statement := range doc.Statements {
		zeroStatement(statement)
	}
}

func zeroStatement(s turtle.Statement) {
	switch n := s.(type) {
	case *turtle.PrefixDirective:
		n.Pos = turtle.Pos{}
		zeroTerm(n.IRI)
	case *turtle.BaseDirective:
		n.Pos = turtle.Pos{}
		zeroTerm(n.IRI)
	case *turtle.VersionDirective:
		n.Pos = turtle.Pos{}
	case *turtle.Triples:
		n.Pos = turtle.Pos{}
		zeroTerm(n.Subject)
		zeroPredicates(n.Predicates)
	}
}

func zeroPredicates(list []*turtle.PredicateObject) {
	for _, po := range list {
		po.Pos = turtle.Pos{}
		zeroTerm(po.Verb)
		for _, object := range po.Objects {
			zeroTerm(object.Term)
			for _, item := range object.Annotation {
				zeroAnnotation(item)
			}
		}
	}
}

func zeroAnnotation(item turtle.Annotation) {
	switch n := item.(type) {
	case *turtle.Reifier:
		n.Pos = turtle.Pos{}
		if n.ID != nil {
			zeroTerm(n.ID)
		}
	case *turtle.AnnotationBlock:
		n.Pos = turtle.Pos{}
		zeroPredicates(n.Predicates)
	}
}

func zeroTerm(node turtle.Term) {
	switch n := node.(type) {
	case *turtle.IRIRef:
		n.Pos = turtle.Pos{}
	case *turtle.PrefixedName:
		n.Pos = turtle.Pos{}
	case *turtle.A:
		n.Pos = turtle.Pos{}
	case *turtle.BlankNode:
		n.Pos = turtle.Pos{}
	case *turtle.Anon:
		n.Pos = turtle.Pos{}
	case *turtle.Literal:
		n.Pos = turtle.Pos{}
		if n.Datatype != nil {
			zeroTerm(n.Datatype)
		}
	case *turtle.Collection:
		n.Pos = turtle.Pos{}
		for _, object := range n.Objects {
			zeroTerm(object)
		}
	case *turtle.BlankNodePropertyList:
		n.Pos = turtle.Pos{}
		zeroPredicates(n.Predicates)
	case *turtle.TripleTerm:
		n.Pos = turtle.Pos{}
		zeroTerm(n.Subject)
		zeroTerm(n.Verb)
		zeroTerm(n.Object)
	case *turtle.ReifiedTriple:
		n.Pos = turtle.Pos{}
		zeroTerm(n.Subject)
		zeroTerm(n.Verb)
		zeroTerm(n.Object)
		if n.Reifier != nil {
			zeroAnnotation(n.Reifier)
		}
	}
}

// formatGraph renders a graph for a failure message.
func formatGraph(g *rdf.Graph) string {
	return formatTriples(slices.Collect(g.All()))
}

// formatDocument renders a tree for a failure message, since a tree of
// pointers prints as addresses.
func formatDocument(doc *turtle.Document) string {
	var b strings.Builder
	if err := turtle.Print(&b, doc); err != nil {
		return "<unprintable: " + err.Error() + ">"
	}
	return b.String()
}
