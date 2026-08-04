package turtle_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/rdf11/turtle"
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
// abbreviation the document wrote is written back as itself rather than as
// what it stands for.
func TestPrint(t *testing.T) {
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
			name: "a triple of iris",
			src:  `<http://e/s> <http://e/p> <http://e/o> .`,
			want: "<http://e/s> <http://e/p> <http://e/o> .\n",
		},
		{
			name: "layout is this package's, not the author's",
			src:  "<http://e/s>\n\t<http://e/p>\n\t\t<http://e/o>\n.",
			want: "<http://e/s> <http://e/p> <http://e/o> .\n",
		},
		{
			name: "an at prefix directive keeps its dot",
			src:  "@prefix ex: <http://e/> .\nex:s ex:p ex:o .",
			want: "@prefix ex: <http://e/> .\nex:s ex:p ex:o .\n",
		},
		{
			name: "a sparql prefix directive keeps its case and takes no dot",
			src:  "PREFIX ex: <http://e/>\nex:s ex:p ex:o .",
			want: "PREFIX ex: <http://e/>\nex:s ex:p ex:o .\n",
		},
		{
			name: "the empty prefix",
			src:  "@prefix : <http://e/> .\n:s :p : .",
			want: "@prefix : <http://e/> .\n:s :p : .\n",
		},
		{
			name: "base directives in both forms",
			src:  "@base <http://e/> .\nBASE <http://f/>\n<s> <p> <o> .",
			want: "@base <http://e/> .\nBASE <http://f/>\n<s> <p> <o> .\n",
		},
		{
			name: "the a keyword is written back as itself",
			src:  `<http://e/s> a <http://e/C> .`,
			want: "<http://e/s> a <http://e/C> .\n",
		},
		{
			name: "objects of one verb are separated by commas",
			src:  `<http://e/s> <http://e/p> <http://e/a>, <http://e/b> .`,
			want: "<http://e/s> <http://e/p> <http://e/a>, <http://e/b> .\n",
		},
		{
			name: "verbs are separated by semicolons, one to a line",
			src:  "@prefix ex: <http://e/> .\nex:s a ex:C ; ex:p 1 ; ex:q 2 .",
			want: "@prefix ex: <http://e/> .\n" +
				"ex:s a ex:C ;\n" +
				"    ex:p 1 ;\n" +
				"    ex:q 2 .\n",
		},
		{
			name: "a trailing semicolon is not written back",
			src:  "@prefix ex: <http://e/> .\nex:s ex:p 1 ; .",
			want: "@prefix ex: <http://e/> .\nex:s ex:p 1 .\n",
		},
		{
			name: "a collection stays a collection",
			src:  "@prefix ex: <http://e/> .\nex:s ex:p (1 2 3) .",
			want: "@prefix ex: <http://e/> .\nex:s ex:p ( 1 2 3 ) .\n",
		},
		{
			name: "an empty collection",
			src:  "@prefix ex: <http://e/> .\nex:s ex:p () .",
			want: "@prefix ex: <http://e/> .\nex:s ex:p () .\n",
		},
		{
			name: "a collection in the subject position",
			src:  "@prefix ex: <http://e/> .\n(1 2) ex:p ex:o .",
			want: "@prefix ex: <http://e/> .\n( 1 2 ) ex:p ex:o .\n",
		},
		{
			name: "a blank node property list stays one",
			src:  "@prefix ex: <http://e/> .\nex:s ex:p [ ex:q 1 ] .",
			want: "@prefix ex: <http://e/> .\nex:s ex:p [ ex:q 1 ] .\n",
		},
		{
			name: "a blank node property list with nothing said about it",
			src:  "@prefix ex: <http://e/> .\n[ ex:p 1 ] .",
			want: "@prefix ex: <http://e/> .\n[ ex:p 1 ] .\n",
		},
		{
			name: "an anonymous blank node",
			src:  "@prefix ex: <http://e/> .\n[] ex:p [] .",
			want: "@prefix ex: <http://e/> .\n[] ex:p [] .\n",
		},
		{
			name: "a labelled blank node keeps its label",
			src:  `_:someLabel <http://e/p> _:other .`,
			want: "_:someLabel <http://e/p> _:other .\n",
		},
		{
			name: "numbers and booleans are written as themselves",
			src:  "@prefix ex: <http://e/> .\nex:s ex:p 42, -3.14, 1e6, true .",
			want: "@prefix ex: <http://e/> .\nex:s ex:p 42, -3.14, 1e6, true .\n",
		},
		{
			name: "a language tagged literal",
			src:  `<http://e/s> <http://e/p> "text"@en-GB .`,
			want: "<http://e/s> <http://e/p> \"text\"@en-GB .\n",
		},
		{
			name: "a typed literal keeps the abbreviation of its datatype",
			src: "@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .\n" +
				`<http://e/s> <http://e/p> "1"^^xsd:byte .`,
			want: "@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .\n" +
				"<http://e/s> <http://e/p> \"1\"^^xsd:byte .\n",
		},
		{
			name: "every quote form prints as the short double quoted one",
			src: "@prefix ex: <http://e/> .\n" +
				"ex:s ex:p 'a', \"\"\"b\"\"\", '''c''' .",
			want: "@prefix ex: <http://e/> .\nex:s ex:p \"a\", \"b\", \"c\" .\n",
		},
		{
			name: "a comment stands on a line of its own",
			src:  "# a note\n<http://e/s> <http://e/p> <http://e/o> .",
			want: "# a note\n<http://e/s> <http://e/p> <http://e/o> .\n",
		},
		{
			name: "a comment written inside a statement follows it",
			src:  "<http://e/s> # a note\n<http://e/p> <http://e/o> .",
			want: "<http://e/s> <http://e/p> <http://e/o> .\n# a note\n",
		},
		{
			name: "a comment after the last statement stays after it",
			src:  "<http://e/s> <http://e/p> <http://e/o> .\n# a note",
			want: "<http://e/s> <http://e/p> <http://e/o> .\n# a note\n",
		},
		{
			name: "a document of nothing but comments",
			src:  "# one\n# two",
			want: "# one\n# two\n",
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

// TestPrintEscapes covers what has to be escaped and, as much, what does not.
//
// Every case is checked by reading the output back rather than only by pinning
// it, since the point of an escape is that it re-reads as the character it
// stands for.
func TestPrintEscapes(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "a quote and a backslash inside a literal",
			src:  `<http://e/s> <http://e/p> "a\"b\\c" .`,
			want: `<http://e/s> <http://e/p> "a\"b\\c" .` + "\n",
		},
		{
			name: "a line feed inside a long literal becomes an escape",
			src:  "<http://e/s> <http://e/p> \"\"\"a\nb\"\"\" .",
			want: `<http://e/s> <http://e/p> "a\nb" .` + "\n",
		},
		{
			name: "a carriage return inside a long literal becomes an escape",
			src:  "<http://e/s> <http://e/p> \"\"\"a\rb\"\"\" .",
			want: `<http://e/s> <http://e/p> "a\rb" .` + "\n",
		},
		{
			name: "a quote a long literal could hold raw is escaped",
			src:  "<http://e/s> <http://e/p> \"\"\"a\"b\"\"\" .",
			want: `<http://e/s> <http://e/p> "a\"b" .` + "\n",
		},
		{
			name: "a tab is written as itself, having no need of an escape",
			src:  `<http://e/s> <http://e/p> "a\tb" .`,
			want: "<http://e/s> <http://e/p> \"a\tb\" .\n",
		},
		{
			// A character an IRI may not hold has no escaped form either — the
			// production excludes it however it is written — so a document
			// carrying one is refused by Parse and never reaches Print; see
			// TestTokenizeRejectsAnEscapedIRICharacter. A space in an IRI is
			// written %20, and that is written back as itself.
			name: "a percent escape in an iri is written as it stands",
			src:  `<http://e/s> <http://e/p> <http://e/a%20b> .`,
			want: `<http://e/s> <http://e/p> <http://e/a%20b> .` + "\n",
		},
		{
			name: "a local name keeps the dots it may hold",
			src:  "@prefix ex: <http://e/> .\nex:a.b ex:p 1 .",
			want: "@prefix ex: <http://e/> .\nex:a.b ex:p 1 .\n",
		},
		{
			name: "a local name ending in a dot is escaped",
			src:  "@prefix ex: <http://e/> .\nex:a\\. ex:p 1 .",
			want: "@prefix ex: <http://e/> .\nex:a\\. ex:p 1 .\n",
		},
		{
			name: "a local name beginning with a hyphen is escaped",
			src:  "@prefix ex: <http://e/> .\nex:\\-a ex:p 1 .",
			want: "@prefix ex: <http://e/> .\nex:\\-a ex:p 1 .\n",
		},
		{
			name: "a percent escape in a local name is left as written",
			src:  "@prefix ex: <http://e/> .\nex:a%20b ex:p 1 .",
			want: "@prefix ex: <http://e/> .\nex:a%20b ex:p 1 .\n",
		},
		{
			name: "an escaped percent that is not a percent escape stays escaped",
			src:  "@prefix ex: <http://e/> .\nex:a\\%zz ex:p 1 .",
			want: "@prefix ex: <http://e/> .\nex:a\\%zz ex:p 1 .\n",
		},
		{
			name: "punctuation in a local name is escaped",
			src:  "@prefix ex: <http://e/> .\nex:a\\&b ex:p 1 .",
			want: "@prefix ex: <http://e/> .\nex:a\\&b ex:p 1 .\n",
		},
		{
			name: "an underscore in a local name needs no escape",
			src:  "@prefix ex: <http://e/> .\nex:_a_ ex:p 1 .",
			want: "@prefix ex: <http://e/> .\nex:_a_ ex:p 1 .\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := printToString(t, parse(t, tc.src))
			if got != tc.want {
				t.Errorf("Print() = %q, want %q", got, tc.want)
			}

			// The escaping is only right if the output says what the input did.
			before := graphOfTriples(t, decodeTurtle(t, tc.src))
			after := graphOfTriples(t, decodeTurtle(t, got))
			if !rdf.Isomorphic(before, after) {
				t.Errorf("printing changed the graph:\nbefore\n%safter\n%s", formatGraph(before), formatGraph(after))
			}
		})
	}
}

// TestPrintWrapsLongLines covers the two layout options, which is also the
// only way to see that wrapping happens at all: the default width is wider
// than most of what a test writes.
func TestPrintWrapsLongLines(t *testing.T) {
	const src = "@prefix ex: <http://e/> .\n" +
		"ex:subject ex:predicate ex:objectOne, ex:objectTwo, ex:objectThree .\n" +
		"ex:other ex:p [ ex:a 1 ; ex:b 2 ] , ( ex:one ex:two ) ."

	testCases := []struct {
		name string
		opts []turtle.Option
		want string
	}{
		{
			name: "wrapping is off when there is no width",
			opts: []turtle.Option{turtle.WithLineWidth(0)},
			want: "@prefix ex: <http://e/> .\n" +
				"ex:subject ex:predicate ex:objectOne, ex:objectTwo, ex:objectThree .\n" +
				"ex:other ex:p [ ex:a 1 ; ex:b 2 ], ( ex:one ex:two ) .\n",
		},
		{
			name: "an object list wraps a level in from its verb",
			opts: []turtle.Option{turtle.WithLineWidth(40)},
			want: "@prefix ex: <http://e/> .\n" +
				"ex:subject ex:predicate ex:objectOne,\n" +
				"        ex:objectTwo, ex:objectThree .\n" +
				"ex:other ex:p [ ex:a 1 ; ex:b 2 ],\n" +
				"        ( ex:one ex:two ) .\n",
		},
		{
			name: "the indent is the caller's",
			opts: []turtle.Option{turtle.WithLineWidth(40), turtle.WithIndent("\t")},
			want: "@prefix ex: <http://e/> .\n" +
				"ex:subject ex:predicate ex:objectOne,\n" +
				"\t\tex:objectTwo, ex:objectThree .\n" +
				"ex:other ex:p [ ex:a 1 ; ex:b 2 ],\n" +
				"\t\t( ex:one ex:two ) .\n",
		},
		{
			name: "a collection and a property list too long for their line open out",
			opts: []turtle.Option{turtle.WithLineWidth(20)},
			want: "@prefix ex: <http://e/> .\n" +
				"ex:subject ex:predicate ex:objectOne,\n" +
				"        ex:objectTwo,\n" +
				"        ex:objectThree .\n" +
				"ex:other ex:p [\n" +
				"        ex:a 1 ;\n" +
				"        ex:b 2\n" +
				"    ],\n" +
				"        (\n" +
				"            ex:one\n" +
				"            ex:two\n" +
				"        ) .\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := printToString(t, parse(t, src), tc.opts...)
			if got != tc.want {
				t.Errorf("Print() = %q, want %q", got, tc.want)
			}

			// However it is laid out, it has to still say the same thing.
			before := graphOfTriples(t, decodeTurtle(t, src))
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
func TestPrintRoundTrip(t *testing.T) {
	sources := []string{
		"",
		`<http://e/s> <http://e/p> <http://e/o> .`,
		"@prefix ex: <http://e/> .\nPREFIX f: <http://f/>\nex:s f:p ex:o .",
		"@base <http://e/> .\nBASE <http://f/>\n<s> <p> <o> .",
		"@prefix ex: <http://e/> .\nex:s a ex:C ; ex:p 1, 2, 3 ; ex:q ex:r .",
		"@prefix ex: <http://e/> .\nex:s ex:p (1 2 (3 4)) , () .",
		"@prefix ex: <http://e/> .\nex:s ex:p [ ex:q [ ex:r 1 ] ; ex:s 2 ] .",
		"@prefix ex: <http://e/> .\nex:s ex:p [ ex:q 1, 2 ; ex:r 3 ] .",
		"@prefix ex: <http://e/> .\n[ ex:p 1 ] .",
		"@prefix ex: <http://e/> .\n[] ex:p [] .",
		"_:a <http://e/p> _:b .",
		"@prefix ex: <http://e/> .\nex:s ex:p 42, +42, -0.5, .5, 1e6, 1.0E-6, true, false .",
		"<http://e/s> <http://e/p> \"a\\\"b\", \"\"\"line\nbreak\"\"\", 'single', '''long''' .",
		`<http://e/s> <http://e/p> "v"@en, "v"@en-GB, "1"^^<http://e/dt> .`,
		"@prefix ex: <http://e/> .\nex:s ex:p \"1\"^^ex:dt .",
		"@prefix ex: <http://e/> .\nex:a.b ex:p ex:\\-c, ex:d\\., ex:e%20f, ex:_g_ .",
		"# leading\n<http://e/s> # inside\n<http://e/p> <http://e/o> . # trailing\n# last",
		"@prefix ex: <http://e/> .\nex:subject ex:predicate ex:objectOne, ex:objectTwo, ex:objectThree, ex:objectFour, ex:objectFive .",
		"@prefix ex: <http://e/> .\nex:s ex:p [ ex:aLongVerbName ex:aLongObjectName ; ex:anotherLongVerb ex:anotherLongObject ] .",
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

	doc := parse(t, "@prefix ex: <http://e/> .\nex:s a ex:C ; ex:p ( 1 2 ) , [ ex:q 1 ] .\n# note")
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

// TestEncode pins what Encode chooses to write for a graph: the grouping, the
// shorthands, and the prefixes it declares.
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
			name:     "an iri no prefix matches is written out in full",
			ntriples: "<http://other/s> <http://other/p> <http://other/o> .",
			want:     "<http://other/s> <http://other/p> <http://other/o> .\n",
		},
		{
			name:     "an iri that is a namespace itself",
			ntriples: "<http://e/> <http://e/p> <http://e/o> .",
			want:     "@prefix ex: <http://e/> .\nex: ex:p ex:o .\n",
		},
		{
			name:     "a local part no PN_LOCAL can hold falls back to the full iri",
			ntriples: "<http://e/s> <http://e/p> <http://e/a[b> .",
			want:     "@prefix ex: <http://e/> .\nex:s ex:p <http://e/a[b> .\n",
		},
		{
			name:     "rdf:type is written a",
			ntriples: "<http://e/s> <" + string(vocab.RDFType) + "> <http://e/C> .",
			want:     "@prefix ex: <http://e/> .\nex:s a ex:C .\n",
		},
		{
			name: "statements about one subject are gathered into one",
			ntriples: "<http://e/s> <http://e/p> <http://e/a> .\n" +
				"<http://e/s> <http://e/q> <http://e/b> .\n" +
				"<http://e/s> <http://e/p> <http://e/c> .\n",
			want: "@prefix ex: <http://e/> .\n" +
				"ex:s ex:p ex:a, ex:c ;\n" +
				"    ex:q ex:b .\n",
		},
		{
			name: "subjects keep the order they first arrived in",
			ntriples: "<http://e/b> <http://e/p> 	<http://e/x> .\n" +
				"<http://e/a> <http://e/p> <http://e/y> .\n",
			want: "@prefix ex: <http://e/> .\n" +
				"ex:b ex:p ex:x .\n" +
				"ex:a ex:p ex:y .\n",
		},
		{
			name:     "a plain string says nothing about its datatype",
			ntriples: `<http://e/s> <http://e/p> "text" .`,
			want:     "@prefix ex: <http://e/> .\nex:s ex:p \"text\" .\n",
		},
		{
			name:     "a language tag is kept",
			ntriples: `<http://e/s> <http://e/p> "text"@en-GB .`,
			want:     "@prefix ex: <http://e/> .\nex:s ex:p \"text\"@en-GB .\n",
		},
		{
			name: "the shorthand datatypes are written as themselves",
			ntriples: `<http://e/s> <http://e/p> "42"^^<` + string(vocab.XSDInteger) + "> .\n" +
				`<http://e/s> <http://e/p> "-3.14"^^<` + string(vocab.XSDDecimal) + "> .\n" +
				`<http://e/s> <http://e/p> "1.0E6"^^<` + string(vocab.XSDDouble) + "> .\n" +
				`<http://e/s> <http://e/p> "true"^^<` + string(vocab.XSDBoolean) + "> .\n",
			want: "@prefix ex: <http://e/> .\nex:s ex:p 42, -3.14, 1.0E6, true .\n",
		},
		{
			name: "a lexical form the production does not admit keeps its datatype",
			ntriples: `<http://e/s> <http://e/p> "0x2A"^^<` + string(vocab.XSDInteger) + "> .\n" +
				`<http://e/s> <http://e/p> "1.0"^^<` + string(vocab.XSDDouble) + "> .\n" +
				`<http://e/s> <http://e/p> "TRUE"^^<` + string(vocab.XSDBoolean) + "> .\n",
			want: "@prefix ex: <http://e/> .\n@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .\n" +
				"ex:s ex:p \"0x2A\"^^xsd:integer, \"1.0\"^^xsd:double, \"TRUE\"^^xsd:boolean .\n",
		},
		{
			name:     "any other datatype is written, abbreviated where it can be",
			ntriples: `<http://e/s> <http://e/p> "v"^^<http://e/dt> .`,
			want:     "@prefix ex: <http://e/> .\nex:s ex:p \"v\"^^ex:dt .\n",
		},
		{
			name:     "a quote and a line break in a literal are escaped",
			ntriples: `<http://e/s> <http://e/p> "a\"b\nc" .`,
			want:     "@prefix ex: <http://e/> .\nex:s ex:p \"a\\\"b\\nc\" .\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			triples := decodeNTriples(t, tc.ntriples)

			got := encodeToString(t, triples, turtle.WithPrefixes(prefixes))
			if got != tc.want {
				t.Errorf("Encode() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEncodeChoosesTheLongestPrefix covers what happens when two namespaces
// both match: the more specific one is the one worth having.
func TestEncodeChoosesTheLongestPrefix(t *testing.T) {
	prefixes := map[string]string{
		"e":  "http://e/",
		"ev": "http://e/vocab/",
	}

	triples := decodeNTriples(t, "<http://e/vocab/s> <http://e/p> <http://e/vocab/o> .")

	want := "@prefix e: <http://e/> .\n@prefix ev: <http://e/vocab/> .\nev:s e:p ev:o .\n"
	if got := encodeToString(t, triples, turtle.WithPrefixes(prefixes)); got != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

// TestEncodeRelabelsBlankNodes covers the two labels Turtle cannot be given:
// one its grammar has no room for, and one already spoken for.
func TestEncodeRelabelsBlankNodes(t *testing.T) {
	testCases := []struct {
		name   string
		labels []string
		want   string
	}{
		{
			name:   "a label the grammar admits is kept",
			labels: []string{"abc"},
			want:   "_:abc <http://e/p> _:abc .\n",
		},
		{
			name:   "a label ending in a dot is replaced",
			labels: []string{"a."},
			want:   "_:b0 <http://e/p> _:b0 .\n",
		},
		{
			name:   "a label the grammar has no room for is replaced",
			labels: []string{"a:b"},
			want:   "_:b0 <http://e/p> _:b0 .\n",
		},
		{
			name:   "a minted label does not collide with one still to come",
			labels: []string{"a.", "b0"},
			want:   "_:b0 <http://e/p> _:b0 .\n_:b1 <http://e/p> _:b1 .\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var triples []rdf.Triple
			for _, label := range tc.labels {
				node := rdf.NewBlankNode(label)
				triples = append(triples, rdf.Triple{
					Subject:   node,
					Predicate: "http://e/p",
					Object:    node,
				})
			}

			if got := encodeToString(t, triples); got != tc.want {
				t.Errorf("Encode() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEncodeRoundTrip covers Decode → Encode → Decode, which has to give back
// the graph it started with. Blank node labels are not preserved by the
// journey and need not be, so the two are compared by isomorphism.
func TestEncodeRoundTrip(t *testing.T) {
	prefixes := map[string]string{
		"ex":  "http://e/",
		"xsd": "http://www.w3.org/2001/XMLSchema#",
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
		"@prefix ex: <http://e/> .\nex:subject ex:predicate ex:objectOne, ex:objectTwo, ex:objectThree, ex:objectFour .",
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
	directional, err := rdf.NewDirectionalLiteral("v", "en", rdf.DirectionLTR)
	if err != nil {
		t.Fatalf("NewDirectionalLiteral() = %v, want nil", err)
	}

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
			name: "a literal carrying a base direction",
			triples: []rdf.Triple{
				{Subject: rdf.IRI("http://e/s"), Predicate: "http://e/p", Object: directional},
			},
			want: turtle.ErrBaseDirection,
		},
		{
			name: "a literal in the subject position",
			triples: []rdf.Triple{
				{Subject: rdf.NewLiteral("nope"), Predicate: "http://e/p", Object: rdf.IRI("http://e/o")},
			},
			want: rdf.ErrInvalidSubject,
		},
		{
			name: "a triple term object",
			triples: []rdf.Triple{
				{
					Subject:   rdf.IRI("http://e/s"),
					Predicate: "http://e/p",
					Object: rdf.TripleTerm{
						Subject:   rdf.IRI("http://e/s2"),
						Predicate: "http://e/p2",
						Object:    rdf.IRI("http://e/o2"),
					},
				},
			},
			want: turtle.ErrTripleTerm,
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
			zeroTerm(object)
		}
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
		{datatype: vocab.XSDInteger, value: "+42", want: "+42"},
		{datatype: vocab.XSDInteger, value: "-0042", want: "-0042"},
		{datatype: vocab.XSDInteger, value: "", want: `""^^<http://www.w3.org/2001/XMLSchema#integer>`},
		{datatype: vocab.XSDInteger, value: "4 2", want: `"4 2"^^<http://www.w3.org/2001/XMLSchema#integer>`},
		{datatype: vocab.XSDInteger, value: "4.2", want: `"4.2"^^<http://www.w3.org/2001/XMLSchema#integer>`},

		{datatype: vocab.XSDDecimal, value: "3.14", want: "3.14"},
		{datatype: vocab.XSDDecimal, value: "-.5", want: "-.5"},
		{datatype: vocab.XSDDecimal, value: "5.", want: `"5."^^<http://www.w3.org/2001/XMLSchema#decimal>`},
		{datatype: vocab.XSDDecimal, value: "5", want: `"5"^^<http://www.w3.org/2001/XMLSchema#decimal>`},

		{datatype: vocab.XSDDouble, value: "1e6", want: "1e6"},
		{datatype: vocab.XSDDouble, value: "1.0E-6", want: "1.0E-6"},
		{datatype: vocab.XSDDouble, value: "1.e6", want: "1.e6"},
		{datatype: vocab.XSDDouble, value: ".5e6", want: ".5e6"},
		{datatype: vocab.XSDDouble, value: "1.0", want: `"1.0"^^<http://www.w3.org/2001/XMLSchema#double>`},
		{datatype: vocab.XSDDouble, value: "1e", want: `"1e"^^<http://www.w3.org/2001/XMLSchema#double>`},
		{datatype: vocab.XSDDouble, value: "1e6.0", want: `"1e6.0"^^<http://www.w3.org/2001/XMLSchema#double>`},
		{datatype: vocab.XSDDouble, value: ".e6", want: `".e6"^^<http://www.w3.org/2001/XMLSchema#double>`},
		{datatype: vocab.XSDDouble, value: "e6", want: `"e6"^^<http://www.w3.org/2001/XMLSchema#double>`},

		{datatype: vocab.XSDBoolean, value: "true", want: "true"},
		{datatype: vocab.XSDBoolean, value: "1", want: `"1"^^<http://www.w3.org/2001/XMLSchema#boolean>`},
	}

	for _, tc := range testCases {
		t.Run(string(tc.datatype)+" "+tc.value, func(t *testing.T) {
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

			// Whichever form was chosen, it has to read back as the literal it
			// was chosen for.
			read := decodeTurtle(t, want)
			if len(read) != 1 || !read[0].Object.Equal(literal) {
				t.Errorf("Decode(Encode(%s)) = %s, want the literal back", literal, formatTriples(read))
			}
		})
	}
}

// TestEncodeLocalNamesThatCannotBeWritten covers the local parts PN_LOCAL has
// no room for, which have to fall back to the IRI written out in full.
func TestEncodeLocalNamesThatCannotBeWritten(t *testing.T) {
	testCases := []struct {
		name  string
		value rdf.IRI
		want  string
	}{
		{
			name:  "a combining mark may not begin a local name",
			value: "http://e/̀x",
			want:  "<http://e/̀x>",
		},
		{
			name:  "a character PN_CHARS has no room for",
			value: "http://e/a b",
			want:  "<http://e/a b>",
		},
		{
			name:  "a digit may begin one",
			value: "http://e/0a",
			want:  "ex:0a",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			triples := []rdf.Triple{
				{Subject: rdf.IRI("http://e/s"), Predicate: "http://e/p", Object: tc.value},
			}

			got := encodeToString(t, triples, turtle.WithPrefixes(map[string]string{"ex": "http://e/"}))
			want := "@prefix ex: <http://e/> .\nex:s ex:p " + tc.want + " .\n"
			if got != want {
				t.Errorf("Encode() = %q, want %q", got, want)
			}
		})
	}
}

// TestEncodeBlankNodeLabels covers which labels survive the journey and which
// have to be replaced.
func TestEncodeBlankNodeLabels(t *testing.T) {
	testCases := []struct {
		name  string
		label string
		want  string
	}{
		{name: "a dot inside a label is allowed", label: "a.b", want: "a.b"},
		{name: "a digit may begin one", label: "0", want: "0"},
		{name: "a character PN_CHARS has no room for", label: "a b", want: "b0"},
		{name: "the empty label", label: "", want: "b0"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			node := rdf.NewBlankNode(tc.label)
			triples := []rdf.Triple{
				{Subject: node, Predicate: "http://e/p", Object: node},
			}

			want := "_:" + tc.want + " <http://e/p> _:" + tc.want + " .\n"
			if got := encodeToString(t, triples); got != want {
				t.Errorf("Encode() = %q, want %q", got, want)
			}
		})
	}
}

// TestPrintSuppliesMissingKeywords covers a tree built by hand rather than by
// [turtle.Parse], where a directive may carry no keyword at all. Writing the
// directive without one would produce a document this package could not read
// back, so the '@' form is supplied.
func TestPrintSuppliesMissingKeywords(t *testing.T) {
	doc := &turtle.Document{
		Statements: []turtle.Statement{
			&turtle.BaseDirective{IRI: &turtle.IRIRef{Value: "http://e/"}},
			&turtle.PrefixDirective{Prefix: "ex", IRI: &turtle.IRIRef{Value: "http://e/"}},
		},
	}

	want := "@base <http://e/> .\n@prefix ex: <http://e/> .\n"
	if got := printToString(t, doc); got != want {
		t.Errorf("Print() = %q, want %q", got, want)
	}
	if _, err := turtle.Parse(strings.NewReader(want)); err != nil {
		t.Errorf("Parse(Print(doc)) = %v, want nil", err)
	}
}
