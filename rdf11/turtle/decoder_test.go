package turtle_test

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"runtime"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/rdf11/ntriples"
	"github.com/z5labs/rdf-go/rdf11/turtle"
	"github.com/z5labs/rdf-go/vocab"
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

func graphOfTriples(t *testing.T, triples []rdf.Triple) *rdf.Graph {
	t.Helper()

	g := rdf.NewGraph()
	for _, triple := range triples {
		if err := g.Add(triple); err != nil {
			t.Fatalf("Add(%s) = %v, want nil", triple, err)
		}
	}
	return g
}

// decodeTurtle lowers a Turtle document, failing the test if it cannot be read.
func decodeTurtle(t *testing.T, src string) []rdf.Triple {
	t.Helper()

	triples, err := collect2(turtle.Decode(strings.NewReader(src)))
	if err != nil {
		t.Fatalf("Decode() = %v, want nil", err)
	}
	return triples
}

// decodeNTriples lowers the N-Triples a test states its expectation in.
//
// Writing what a Turtle document means as N-Triples is how the W3C evaluation
// tests state it, and it is the only readable way to write down an expectation
// involving blank nodes: the labels a scope mints are its own, so the two
// graphs are compared by isomorphism and the labels never have to be guessed.
func decodeNTriples(t *testing.T, src string) []rdf.Triple {
	t.Helper()

	triples, err := collect2(ntriples.Decode(strings.NewReader(src)))
	if err != nil {
		t.Fatalf("ntriples.Decode() = %v, want nil", err)
	}
	return triples
}

func TestDecode(t *testing.T) {
	testCases := []struct {
		name string
		src  string

		// want is the same graph written as N-Triples, which the decoded graph
		// has to be isomorphic to.
		want string
	}{
		{
			name: "an empty document",
			src:  "",
			want: "",
		},
		{
			name: "a document of nothing but a comment",
			src:  "# nothing to say\n",
			want: "",
		},
		{
			name: "a triple of iris written out in full",
			src:  `<http://example.com/s> <http://example.com/p> <http://example.com/o> .`,
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o> .`,
		},
		{
			name: "a prefixed name expands against its prefix",
			src: "@prefix ex: <http://example.com/> .\n" +
				"ex:s ex:p ex:o .",
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o> .`,
		},
		{
			name: "the empty prefix is a prefix like any other",
			src: "@prefix : <http://example.com/> .\n" +
				":s :p :o .",
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o> .`,
		},
		{
			name: "a prefixed name with no local part is the namespace itself",
			src: "@prefix ex: <http://example.com/s> .\n" +
				"ex: <http://example.com/p> <http://example.com/o> .",
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o> .`,
		},
		{
			name: "a local name keeps the characters its escapes stood for",
			src: "@prefix ex: <http://example.com/> .\n" +
				`ex:a\.b ex:p ex:o .`,
			want: `<http://example.com/a.b> <http://example.com/p> <http://example.com/o> .`,
		},
		{
			name: "the sparql prefix form binds the same as the at form",
			src: "PREFIX ex: <http://example.com/>\n" +
				"ex:s ex:p ex:o .",
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o> .`,
		},
		{
			name: "a prefix redefined mid document applies only after the redefinition",
			src: "@prefix ex: <http://example.com/one/> .\n" +
				"ex:s ex:p ex:o .\n" +
				"@prefix ex: <http://example.com/two/> .\n" +
				"ex:s ex:p ex:o .",
			want: "<http://example.com/one/s> <http://example.com/one/p> <http://example.com/one/o> .\n" +
				"<http://example.com/two/s> <http://example.com/two/p> <http://example.com/two/o> .",
		},
		{
			name: "a relative iri resolves against the base",
			src: "@base <http://example.com/dir/doc> .\n" +
				"<s> <p> <o> .",
			want: `<http://example.com/dir/s> <http://example.com/dir/p> <http://example.com/dir/o> .`,
		},
		{
			name: "the empty iri is the base itself",
			src: "@base <http://example.com/dir/doc> .\n" +
				"<> <#p> <../o> .",
			want: `<http://example.com/dir/doc> <http://example.com/dir/doc#p> <http://example.com/o> .`,
		},
		{
			name: "a second base applies only to the iris after it",
			src: "@base <http://example.com/one/> .\n" +
				"<s> <p> <o> .\n" +
				"@base <http://example.com/two/> .\n" +
				"<s> <p> <o> .",
			want: "<http://example.com/one/s> <http://example.com/one/p> <http://example.com/one/o> .\n" +
				"<http://example.com/two/s> <http://example.com/two/p> <http://example.com/two/o> .",
		},
		{
			name: "a base written relative resolves against the base it replaces",
			src: "@base <http://example.com/one/> .\n" +
				"@base <two/> .\n" +
				"<s> <p> <o> .",
			want: `<http://example.com/one/two/s> <http://example.com/one/two/p> <http://example.com/one/two/o> .`,
		},
		{
			name: "the sparql base form sets the same base as the at form",
			src: "BASE <http://example.com/dir/>\n" +
				"<s> <p> <o> .",
			want: `<http://example.com/dir/s> <http://example.com/dir/p> <http://example.com/dir/o> .`,
		},
		{
			name: "a prefix binds the namespace resolved against the base in scope",
			src: "@base <http://example.com/dir/> .\n" +
				"@prefix ex: <sub/> .\n" +
				"ex:s ex:p ex:o .",
			want: `<http://example.com/dir/sub/s> <http://example.com/dir/sub/p> <http://example.com/dir/sub/o> .`,
		},
		{
			name: "a base after a prefix leaves the prefix as it was bound",
			src: "@base <http://example.com/one/> .\n" +
				"@prefix ex: <sub/> .\n" +
				"@base <http://example.com/two/> .\n" +
				"ex:s ex:p ex:o .",
			want: `<http://example.com/one/sub/s> <http://example.com/one/sub/p> <http://example.com/one/sub/o> .`,
		},
		{
			name: "the a keyword is rdf:type",
			src:  `<http://example.com/s> a <http://example.com/Thing> .`,
			want: `<http://example.com/s> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://example.com/Thing> .`,
		},
		{
			name: "a predicate object list says several things about one subject",
			src: "@prefix ex: <http://example.com/> .\n" +
				"ex:s ex:p ex:o ; ex:q ex:r .",
			want: "<http://example.com/s> <http://example.com/p> <http://example.com/o> .\n" +
				"<http://example.com/s> <http://example.com/q> <http://example.com/r> .",
		},
		{
			name: "an object list gives several objects for one verb",
			src: "@prefix ex: <http://example.com/> .\n" +
				"ex:s ex:p ex:o , ex:o2 .",
			want: "<http://example.com/s> <http://example.com/p> <http://example.com/o> .\n" +
				"<http://example.com/s> <http://example.com/p> <http://example.com/o2> .",
		},
		{
			name: "a labelled blank node is one node wherever it is written",
			src: "@prefix ex: <http://example.com/> .\n" +
				"_:a ex:p ex:o .\n" +
				"ex:s ex:q _:a .",
			want: "_:a <http://example.com/p> <http://example.com/o> .\n" +
				"<http://example.com/s> <http://example.com/q> _:a .",
		},
		{
			name: "an anonymous blank node is a node of its own each time",
			src: "@prefix ex: <http://example.com/> .\n" +
				"[] ex:p ex:o .\n" +
				"[] ex:p ex:o2 .",
			want: "_:a <http://example.com/p> <http://example.com/o> .\n" +
				"_:b <http://example.com/p> <http://example.com/o2> .",
		},
		{
			name: "an empty collection is rdf:nil and expands to nothing",
			src: "@prefix ex: <http://example.com/> .\n" +
				"ex:s ex:p () .",
			want: `<http://example.com/s> <http://example.com/p> <http://www.w3.org/1999/02/22-rdf-syntax-ns#nil> .`,
		},
		{
			name: "an empty collection standing as a subject is rdf:nil too",
			src: "@prefix ex: <http://example.com/> .\n" +
				"() ex:p ex:o .",
			want: `<http://www.w3.org/1999/02/22-rdf-syntax-ns#nil> <http://example.com/p> <http://example.com/o> .`,
		},
		{
			name: "a collection expands to a chain ending at rdf:nil",
			src: "@prefix ex: <http://example.com/> .\n" +
				"ex:s ex:p ( ex:a ex:b ) .",
			want: "_:c0 <http://www.w3.org/1999/02/22-rdf-syntax-ns#first> <http://example.com/a> .\n" +
				"_:c0 <http://www.w3.org/1999/02/22-rdf-syntax-ns#rest> _:c1 .\n" +
				"_:c1 <http://www.w3.org/1999/02/22-rdf-syntax-ns#first> <http://example.com/b> .\n" +
				"_:c1 <http://www.w3.org/1999/02/22-rdf-syntax-ns#rest> <http://www.w3.org/1999/02/22-rdf-syntax-ns#nil> .\n" +
				"<http://example.com/s> <http://example.com/p> _:c0 .",
		},
		{
			name: "a collection may stand as a subject",
			src: "@prefix ex: <http://example.com/> .\n" +
				"( ex:a ) ex:p ex:o .",
			want: "_:c0 <http://www.w3.org/1999/02/22-rdf-syntax-ns#first> <http://example.com/a> .\n" +
				"_:c0 <http://www.w3.org/1999/02/22-rdf-syntax-ns#rest> <http://www.w3.org/1999/02/22-rdf-syntax-ns#nil> .\n" +
				"_:c0 <http://example.com/p> <http://example.com/o> .",
		},
		{
			name: "a collection nested in another expands in place",
			src: "@prefix ex: <http://example.com/> .\n" +
				"ex:s ex:p ( ( ex:a ) ex:b ) .",
			want: "_:outer0 <http://www.w3.org/1999/02/22-rdf-syntax-ns#first> _:inner .\n" +
				"_:outer0 <http://www.w3.org/1999/02/22-rdf-syntax-ns#rest> _:outer1 .\n" +
				"_:inner <http://www.w3.org/1999/02/22-rdf-syntax-ns#first> <http://example.com/a> .\n" +
				"_:inner <http://www.w3.org/1999/02/22-rdf-syntax-ns#rest> <http://www.w3.org/1999/02/22-rdf-syntax-ns#nil> .\n" +
				"_:outer1 <http://www.w3.org/1999/02/22-rdf-syntax-ns#first> <http://example.com/b> .\n" +
				"_:outer1 <http://www.w3.org/1999/02/22-rdf-syntax-ns#rest> <http://www.w3.org/1999/02/22-rdf-syntax-ns#nil> .\n" +
				"<http://example.com/s> <http://example.com/p> _:outer0 .",
		},
		{
			name: "a blank node property list is a node described in place",
			src: "@prefix ex: <http://example.com/> .\n" +
				"ex:s ex:p [ ex:q ex:o ] .",
			want: "_:b <http://example.com/q> <http://example.com/o> .\n" +
				"<http://example.com/s> <http://example.com/p> _:b .",
		},
		{
			name: "a blank node property list standing alone says what it holds",
			src: "@prefix ex: <http://example.com/> .\n" +
				"[ ex:p ex:o ] .",
			want: `_:b <http://example.com/p> <http://example.com/o> .`,
		},
		{
			name: "a blank node property list may be a subject and take more verbs",
			src: "@prefix ex: <http://example.com/> .\n" +
				"[ ex:p ex:o ] ex:q ex:r .",
			want: "_:b <http://example.com/p> <http://example.com/o> .\n" +
				"_:b <http://example.com/q> <http://example.com/r> .",
		},
		{
			name: "a blank node property list nested in another describes its own node",
			src: "@prefix ex: <http://example.com/> .\n" +
				"ex:s ex:p [ ex:q [ ex:r ex:o ] ] .",
			want: "_:inner <http://example.com/r> <http://example.com/o> .\n" +
				"_:outer <http://example.com/q> _:inner .\n" +
				"<http://example.com/s> <http://example.com/p> _:outer .",
		},
		{
			name: "a collection holding a blank node property list",
			src: "@prefix ex: <http://example.com/> .\n" +
				"ex:s ex:p ( [ ex:q ex:o ] ) .",
			want: "_:b <http://example.com/q> <http://example.com/o> .\n" +
				"_:c <http://www.w3.org/1999/02/22-rdf-syntax-ns#first> _:b .\n" +
				"_:c <http://www.w3.org/1999/02/22-rdf-syntax-ns#rest> <http://www.w3.org/1999/02/22-rdf-syntax-ns#nil> .\n" +
				"<http://example.com/s> <http://example.com/p> _:c .",
		},
		{
			name: "a plain string takes the xsd:string datatype",
			src:  `<http://example.com/s> <http://example.com/p> "text" .`,
			want: `<http://example.com/s> <http://example.com/p> "text" .`,
		},
		{
			name: "a long string is the same literal as a short one",
			src:  `<http://example.com/s> <http://example.com/p> """te"xt""" .`,
			want: `<http://example.com/s> <http://example.com/p> "te\"xt" .`,
		},
		{
			name: "a tagged string takes the rdf:langString datatype",
			src:  `<http://example.com/s> <http://example.com/p> "text"@en-GB .`,
			want: `<http://example.com/s> <http://example.com/p> "text"@en-GB .`,
		},
		{
			name: "a datatype written out in full is taken as written",
			src:  `<http://example.com/s> <http://example.com/p> "1"^^<http://www.w3.org/2001/XMLSchema#integer> .`,
			want: `<http://example.com/s> <http://example.com/p> "1"^^<http://www.w3.org/2001/XMLSchema#integer> .`,
		},
		{
			name: "a datatype written as a prefixed name expands too",
			src: "@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .\n" +
				`<http://example.com/s> <http://example.com/p> "1"^^xsd:integer .`,
			want: `<http://example.com/s> <http://example.com/p> "1"^^<http://www.w3.org/2001/XMLSchema#integer> .`,
		},
		{
			name: "a datatype written relative resolves against the base",
			src: "@base <http://example.com/> .\n" +
				`<s> <p> "1"^^<dt> .`,
			want: `<http://example.com/s> <http://example.com/p> "1"^^<http://example.com/dt> .`,
		},
		{
			name: "true and false are xsd:boolean",
			src: "<http://example.com/s> <http://example.com/p> true .\n" +
				"<http://example.com/s> <http://example.com/q> false .",
			want: "<http://example.com/s> <http://example.com/p> \"true\"^^<http://www.w3.org/2001/XMLSchema#boolean> .\n" +
				"<http://example.com/s> <http://example.com/q> \"false\"^^<http://www.w3.org/2001/XMLSchema#boolean> .",
		},
		{
			name: "an integer is xsd:integer, sign and all",
			src: "<http://example.com/s> <http://example.com/p> 42 .\n" +
				"<http://example.com/s> <http://example.com/q> -7 .",
			want: "<http://example.com/s> <http://example.com/p> \"42\"^^<http://www.w3.org/2001/XMLSchema#integer> .\n" +
				"<http://example.com/s> <http://example.com/q> \"-7\"^^<http://www.w3.org/2001/XMLSchema#integer> .",
		},
		{
			name: "a number with a point and no exponent is xsd:decimal",
			src:  `<http://example.com/s> <http://example.com/p> 3.14 .`,
			want: `<http://example.com/s> <http://example.com/p> "3.14"^^<http://www.w3.org/2001/XMLSchema#decimal> .`,
		},
		{
			name: "a number with an exponent is xsd:double",
			src: "<http://example.com/s> <http://example.com/p> 1e6 .\n" +
				"<http://example.com/s> <http://example.com/q> 3.0E-2 .",
			want: "<http://example.com/s> <http://example.com/p> \"1e6\"^^<http://www.w3.org/2001/XMLSchema#double> .\n" +
				"<http://example.com/s> <http://example.com/q> \"3.0E-2\"^^<http://www.w3.org/2001/XMLSchema#double> .",
		},
		{
			name: "the shorthands keep the lexical form the document wrote",
			src:  `<http://example.com/s> <http://example.com/p> 007 .`,
			want: `<http://example.com/s> <http://example.com/p> "007"^^<http://www.w3.org/2001/XMLSchema#integer> .`,
		},
		{
			name: "a document mixing everything it has",
			src: "@prefix ex: <http://example.com/> .\n" +
				"@prefix xsd: <http://www.w3.org/2001/XMLSchema#> .\n" +
				"ex:s a ex:Thing ;\n" +
				"  ex:list ( 1 \"two\" [ ex:p ex:o ] ) ;\n" +
				"  ex:label \"name\"@en , \"1\"^^xsd:integer .",
			want: "<http://example.com/s> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://example.com/Thing> .\n" +
				"<http://example.com/s> <http://example.com/list> _:c0 .\n" +
				"_:c0 <http://www.w3.org/1999/02/22-rdf-syntax-ns#first> \"1\"^^<http://www.w3.org/2001/XMLSchema#integer> .\n" +
				"_:c0 <http://www.w3.org/1999/02/22-rdf-syntax-ns#rest> _:c1 .\n" +
				"_:c1 <http://www.w3.org/1999/02/22-rdf-syntax-ns#first> \"two\" .\n" +
				"_:c1 <http://www.w3.org/1999/02/22-rdf-syntax-ns#rest> _:c2 .\n" +
				"_:c2 <http://www.w3.org/1999/02/22-rdf-syntax-ns#first> _:b .\n" +
				"_:c2 <http://www.w3.org/1999/02/22-rdf-syntax-ns#rest> <http://www.w3.org/1999/02/22-rdf-syntax-ns#nil> .\n" +
				"_:b <http://example.com/p> <http://example.com/o> .\n" +
				"<http://example.com/s> <http://example.com/label> \"name\"@en .\n" +
				"<http://example.com/s> <http://example.com/label> \"1\"^^<http://www.w3.org/2001/XMLSchema#integer> .",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeTurtle(t, tc.src)
			want := decodeNTriples(t, tc.want)

			if len(got) != len(want) {
				t.Fatalf("Decode() yielded %d triples, want %d:\n%s", len(got), len(want), formatTriples(got))
			}
			if !rdf.Isomorphic(graphOfTriples(t, got), graphOfTriples(t, want)) {
				t.Errorf("Decode() gave a graph that is not the one wanted:\ngot:\n%swant:\n%s",
					formatTriples(got), formatTriples(want))
			}
		})
	}
}

func formatTriples(triples []rdf.Triple) string {
	var b strings.Builder
	for _, triple := range triples {
		fmt.Fprintf(&b, "  %s\n", triple)
	}
	return b.String()
}

// TestDecodeNestsBeforeItEncloses pins the order [turtle.Decode] documents: the
// triples of a nested collection or property list come before the triple that
// uses it. Nothing about a graph depends on the order, so only the promise the
// doc comment makes is being kept here.
func TestDecodeNestsBeforeItEncloses(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want []rdf.IRI
	}{
		{
			name: "a collection",
			src:  `<http://example.com/s> <http://example.com/p> ( <http://example.com/a> <http://example.com/b> ) .`,
			want: []rdf.IRI{
				vocab.RDFFirst, vocab.RDFRest,
				vocab.RDFFirst, vocab.RDFRest,
				"http://example.com/p",
			},
		},
		{
			name: "a blank node property list",
			src:  `<http://example.com/s> <http://example.com/p> [ <http://example.com/q> <http://example.com/o> ] .`,
			want: []rdf.IRI{"http://example.com/q", "http://example.com/p"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			triples := decodeTurtle(t, tc.src)

			got := make([]rdf.IRI, 0, len(triples))
			for _, triple := range triples {
				got = append(got, triple.Predicate)
			}
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("Decode() yielded predicates %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDecodeUndefinedPrefix(t *testing.T) {
	testCases := []struct {
		name       string
		src        string
		wantPos    turtle.Pos
		wantPrefix string
	}{
		{
			name:       "in the subject",
			src:        `ex:s <http://example.com/p> <http://example.com/o> .`,
			wantPos:    pos(1, 1),
			wantPrefix: "ex",
		},
		{
			name:       "in the verb",
			src:        `<http://example.com/s> ex:p <http://example.com/o> .`,
			wantPos:    pos(1, 24),
			wantPrefix: "ex",
		},
		{
			name:       "in the object",
			src:        `<http://example.com/s> <http://example.com/p> ex:o .`,
			wantPos:    pos(1, 47),
			wantPrefix: "ex",
		},
		{
			name:       "in a datatype",
			src:        `<http://example.com/s> <http://example.com/p> "1"^^xsd:integer .`,
			wantPos:    pos(1, 52),
			wantPrefix: "xsd",
		},
		{
			name:       "the empty prefix names itself",
			src:        `:s <http://example.com/p> <http://example.com/o> .`,
			wantPos:    pos(1, 1),
			wantPrefix: "",
		},
		{
			name: "a prefix used before the directive that binds it",
			src: "ex:s <http://example.com/p> <http://example.com/o> .\n" +
				"@prefix ex: <http://example.com/> .",
			wantPos:    pos(1, 1),
			wantPrefix: "ex",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect2(turtle.Decode(strings.NewReader(tc.src)))

			var undefined turtle.UndefinedPrefixError
			if !errors.As(err, &undefined) {
				t.Fatalf("Decode() error = %#v, want an UndefinedPrefixError", err)
			}
			if undefined.Pos != tc.wantPos {
				t.Errorf("Pos = %s, want %s", undefined.Pos, tc.wantPos)
			}
			if undefined.Prefix != tc.wantPrefix {
				t.Errorf("Prefix = %q, want %q", undefined.Prefix, tc.wantPrefix)
			}
		})
	}
}

func TestDecodeInvalidTerms(t *testing.T) {
	testCases := []struct {
		name    string
		src     string
		wantPos turtle.Pos
		wantErr error
	}{
		{
			name:    "a relative iri with no base",
			src:     `<s> <http://example.com/p> <http://example.com/o> .`,
			wantPos: pos(1, 1),
			wantErr: turtle.ErrNoBase,
		},
		{
			name:    "the empty iri with no base",
			src:     `<http://example.com/s> <> <http://example.com/o> .`,
			wantPos: pos(1, 24),
			wantErr: turtle.ErrNoBase,
		},
		{
			name:    "a relative datatype with no base",
			src:     `<http://example.com/s> <http://example.com/p> "1"^^<dt> .`,
			wantPos: pos(1, 52),
			wantErr: turtle.ErrNoBase,
		},
		{
			name:    "a base written relative with no base to resolve against",
			src:     "@base <dir/> .\n<http://example.com/s> <http://example.com/p> <http://example.com/o> .",
			wantPos: pos(1, 7),
			wantErr: turtle.ErrNoBase,
		},
		{
			name:    "a prefix bound to a relative iri with no base",
			src:     "@prefix ex: <dir/> .\nex:s ex:p ex:o .",
			wantPos: pos(1, 13),
			wantErr: turtle.ErrNoBase,
		},
		{
			name:    "rdf:langString without a language tag",
			src:     `<http://example.com/s> <http://example.com/p> "v"^^<http://www.w3.org/1999/02/22-rdf-syntax-ns#langString> .`,
			wantPos: pos(1, 52),
			wantErr: rdf.ErrReservedDatatype,
		},
		{
			name: "rdf:dirLangString written as a prefixed name",
			src: "@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .\n" +
				`<http://example.com/s> <http://example.com/p> "v"^^rdf:dirLangString .`,
			wantPos: pos(2, 52),
			wantErr: rdf.ErrReservedDatatype,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect2(turtle.Decode(strings.NewReader(tc.src)))
			if err == nil {
				t.Fatal("Decode() = nil, want an error")
			}

			var invalid turtle.InvalidTermError
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

// TestDecodeReportsParseErrors checks that what stopped the parser reaches the
// caller unchanged, rather than being swallowed by the lowering in front of it.
func TestDecodeReportsParseErrors(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name:        "a tokenizer error carries its character position",
			src:         `$ <http://example.com/p> <http://example.com/o> .`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: pos(1, 1), R: '$'},
		},
		{
			name: "a parse error carries its token position",
			src:  `"nope" <http://example.com/p> <http://example.com/o> .`,
			expectedErr: turtle.UnexpectedTokenError{
				Expected: []turtle.TokenType{
					turtle.TokenIRIRef, turtle.TokenPNameNS, turtle.TokenPNameLN,
					turtle.TokenBlankNodeLabel, turtle.TokenAnon,
					turtle.TokenOpenParen, turtle.TokenOpenBracket,
				},
				Actual: turtle.Token{Pos: pos(1, 1), Type: turtle.TokenString, Value: []byte("nope")},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect2(turtle.Decode(strings.NewReader(tc.src)))
			if fmt.Sprint(err) != fmt.Sprint(tc.expectedErr) {
				t.Errorf("Decode() error = %v, want %v", err, tc.expectedErr)
			}
		})
	}
}

// TestDecodeYieldsNothingAfterAnError checks the contract the doc states: the
// triples before the bad statement are handed over, and nothing after it is.
func TestDecodeYieldsNothingAfterAnError(t *testing.T) {
	src := "@prefix ex: <http://example.com/> .\n" +
		"ex:a ex:p \"1\" .\n" +
		"nope:b ex:p \"2\" .\n" +
		"ex:c ex:p \"3\" .\n"

	var (
		triples []rdf.Triple
		errs    []error
	)
	for triple, err := range turtle.Decode(strings.NewReader(src)) {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		triples = append(triples, triple)
	}

	if len(errs) != 1 {
		t.Fatalf("yielded %d errors, want 1", len(errs))
	}
	if len(triples) != 1 {
		t.Fatalf("yielded %d triples, want 1", len(triples))
	}
	if got, want := triples[0].Subject, rdf.IRI("http://example.com/a"); !got.Equal(want) {
		t.Errorf("yielded %s, want %s", got, want)
	}
}

func TestDecodeStopsEarly(t *testing.T) {
	testCases := []struct {
		name string
		src  string
	}{
		{
			name: "between statements",
			src: "@prefix ex: <http://example.com/> .\n" +
				"ex:a ex:p \"1\" .\n" +
				"ex:b ex:p \"2\" .\n",
		},
		{
			// Stopping inside a statement unwinds the lowering as well as the
			// parser, which is the case a collection is the only way to reach.
			name: "part way through a collection",
			src: "@prefix ex: <http://example.com/> .\n" +
				"ex:a ex:p ( ex:x ex:y ex:z ) .\n",
		},
		{
			name: "part way through a blank node property list",
			src: "@prefix ex: <http://example.com/> .\n" +
				"ex:a ex:p [ ex:q ex:o ; ex:r ex:o2 ] .\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var seen int
			for range turtle.Decode(strings.NewReader(tc.src)) {
				seen++
				break
			}
			if want := 1; seen != want {
				t.Errorf("yielded %d triples after break, want %d", seen, want)
			}
		})
	}
}

func TestDecodeReaderError(t *testing.T) {
	errBoom := errors.New("boom")

	src := "<http://example.com/a> <http://example.com/p> \"1\" .\n<http://example.com/b>"
	_, err := collect2(turtle.Decode(&failingReader{data: []byte(src), err: errBoom}))
	if !errors.Is(err, errBoom) {
		t.Errorf("Decode() error = %v, want %v", err, errBoom)
	}
}

// TestLower covers lowering a document already parsed, which has to agree with
// what Decode would have produced from the same source.
func TestLower(t *testing.T) {
	src := "@prefix ex: <http://example.com/> .\n" +
		"_:a ex:p \"text\"@en .\n" +
		"ex:s ex:q _:a ;\n" +
		"  ex:r ( 1 [ ex:t true ] ) .\n"

	doc, err := turtle.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}

	lowered, err := turtle.Lower(doc)
	if err != nil {
		t.Fatalf("Lower() = %v, want nil", err)
	}
	if got, want := len(lowered), 8; got != want {
		t.Fatalf("Lower() returned %d triples, want %d", got, want)
	}

	t.Run("one scope covers the whole document", func(t *testing.T) {
		if !lowered[0].Subject.Equal(lowered[1].Object) {
			t.Errorf("_:a lowered as %s then %s, want one node", lowered[0].Subject, lowered[1].Object)
		}
	})

	t.Run("the graph matches what Decode produces", func(t *testing.T) {
		streamed := decodeTurtle(t, src)

		// The blank node labels differ, each scope having minted its own, so
		// the graphs are isomorphic rather than equal.
		if !rdf.Isomorphic(graphOfTriples(t, lowered), graphOfTriples(t, streamed)) {
			t.Error("Lower() and Decode() produced graphs that are not isomorphic")
		}
	})
}

func TestLowerErrors(t *testing.T) {
	t.Run("a nil document has no triples", func(t *testing.T) {
		triples, err := turtle.Lower(nil)
		if err != nil {
			t.Fatalf("Lower() = %v, want nil", err)
		}
		if triples != nil {
			t.Errorf("Lower() = %v, want nil", triples)
		}
	})

	t.Run("an undefined prefix is reported with its position", func(t *testing.T) {
		doc, err := turtle.Parse(strings.NewReader(`ex:s ex:p ex:o .`))
		if err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}

		_, err = turtle.Lower(doc)
		var undefined turtle.UndefinedPrefixError
		if !errors.As(err, &undefined) {
			t.Fatalf("Lower() error = %#v, want an UndefinedPrefixError", err)
		}
		if got, want := undefined.Pos, pos(1, 1); got != want {
			t.Errorf("Pos = %s, want %s", got, want)
		}
	})

	t.Run("an invalid term is reported with its position", func(t *testing.T) {
		doc, err := turtle.Parse(strings.NewReader(`<http://example.com/s> <http://example.com/p> "v"^^<> .`))
		if err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}

		_, err = turtle.Lower(doc)
		var invalid turtle.InvalidTermError
		if !errors.As(err, &invalid) {
			t.Fatalf("Lower() error = %#v, want an InvalidTermError", err)
		}
		if got, want := invalid.Pos, pos(1, 52); got != want {
			t.Errorf("Pos = %s, want %s", got, want)
		}
	})
}

func TestDecoderErrorMessages(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "an invalid term",
			err:  turtle.InvalidTermError{Pos: pos(3, 12), Err: rdf.ErrEmptyDatatype},
			want: "invalid term at line 3, column 12: rdf: datatype must not be empty",
		},
		{
			name: "an undefined prefix",
			err:  turtle.UndefinedPrefixError{Pos: pos(3, 12), Prefix: "ex"},
			want: `undefined prefix "ex" at line 3, column 12`,
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

// tripleGenerator writes N statements on demand, so that a test can read a
// document far larger than it could hold.
//
// The line buffer is reused rather than allocated per statement, so that what
// the generator costs stays out of what the decoder is measured on.
type tripleGenerator struct {
	remaining int
	scratch   []byte
	pending   []byte
}

func (g *tripleGenerator) Read(p []byte) (int, error) {
	if len(g.pending) == 0 {
		if g.remaining == 0 {
			return 0, io.EOF
		}
		g.remaining--
		g.scratch = fmt.Appendf(g.scratch[:0], "ex:s%d ex:p \"v%d\" .\n", g.remaining, g.remaining)
		g.pending = g.scratch
	}

	n := copy(p, g.pending)
	g.pending = g.pending[n:]
	return n, nil
}

// prefixed prepends the directive the generated statements abbreviate against.
func prefixed(r io.Reader) io.Reader {
	return io.MultiReader(strings.NewReader("@prefix ex: <http://example.com/> .\n"), r)
}

// heapInUse returns the live heap after a collection, so that garbage the
// decoder has already let go of is not counted against it.
func heapInUse() uint64 {
	runtime.GC()

	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

// TestDecodeMemoryDoesNotGrowWithTripleCount is the criterion that makes Decode
// worth having over Parse. It reads a million statements, sampling the live
// heap a tenth of the way in and again at the end: a decoder holding on to what
// it has read would show ten times the heap at the second sample, and one that
// streams shows about the same.
//
// The document is written on demand rather than built up front, since a test
// that had to hold the input in memory would prove nothing about the decoder.
// It names no blank nodes, because remembering those is the one thing Decode
// genuinely cannot do in constant space — see the note on [turtle.Decode].
func TestDecodeMemoryDoesNotGrowWithTripleCount(t *testing.T) {
	const (
		total = 1_000_000
		early = total / 10
	)

	var (
		count            int
		earlyHeap, final uint64
		lastSubject      rdf.Term
	)

	for triple, err := range turtle.Decode(prefixed(&tripleGenerator{remaining: total})) {
		if err != nil {
			t.Fatalf("Decode() = %v, want nil", err)
		}

		count++
		// Keep the most recent triple reachable so the loop cannot be optimized
		// into doing nothing at all.
		lastSubject = triple.Subject

		if count == early {
			earlyHeap = heapInUse()
		}
	}
	final = heapInUse()

	if count != total {
		t.Fatalf("decoded %d triples, want %d", count, total)
	}
	if lastSubject == nil {
		t.Fatal("decoded no subject")
	}

	// Ten times the statements read, so anything retained per statement would
	// be plain at this margin. The allowance is for the heap wandering, not for
	// growth.
	if final > earlyHeap*2 {
		t.Errorf(
			"live heap grew from %d bytes after %d triples to %d bytes after %d",
			earlyHeap, early, final, total,
		)
	}
	t.Logf("live heap: %d bytes after %d triples, %d bytes after %d", earlyHeap, early, final, total)
}
