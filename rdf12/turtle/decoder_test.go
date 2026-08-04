package turtle_test

import (
	"errors"
	"fmt"
	"iter"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/rdf12/ntriples"
	"github.com/z5labs/rdf-go/rdf12/turtle"
	"github.com/z5labs/rdf-go/vocab"
)

// prefixed is the prologue every source below is read with, so that the cases
// can be written in the abbreviated form the syntax exists for.
const prefixed = "@prefix : <http://example.com/> .\n"

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

// decodeNTriples lowers the RDF 1.2 N-Triples a test states its expectation in.
//
// Writing what a Turtle document means as N-Triples is how the W3C evaluation
// tests state it, and it is the only readable way to write down an expectation
// involving blank nodes: the labels a scope mints are its own, so the two
// graphs are compared by isomorphism and the labels never have to be guessed.
//
// A blank node inside a triple term is the exception, since [rdf.Isomorphic]
// compares those by label rather than mapping them, so the cases below name a
// reifier explicitly wherever one would otherwise end up nested in a term.
func decodeNTriples(t *testing.T, src string) []rdf.Triple {
	t.Helper()

	triples, err := collect2(ntriples.Decode(strings.NewReader(src)))
	if err != nil {
		t.Fatalf("ntriples.Decode() = %v, want nil", err)
	}
	return triples
}

func formatTriples(triples []rdf.Triple) string {
	var b strings.Builder
	for _, triple := range triples {
		fmt.Fprintf(&b, "  %s\n", triple)
	}
	return b.String()
}

const reifies = "<http://www.w3.org/1999/02/22-rdf-syntax-ns#reifies>"

func TestDecodeRDF12(t *testing.T) {
	testCases := []struct {
		name string
		src  string

		// want is the same graph written as RDF 1.2 N-Triples, which the
		// decoded graph has to be isomorphic to.
		want string
	}{
		{
			name: "a triple term is a term and expands to no triples of its own",
			src:  `:s :p <<( :a :b :c )>> .`,
			want: `<http://example.com/s> <http://example.com/p> ` +
				`<<( <http://example.com/a> <http://example.com/b> <http://example.com/c> )>> .`,
		},
		{
			name: "the a keyword inside a triple term is rdf:type",
			src:  `:s :p <<( :a a :C )>> .`,
			want: `<http://example.com/s> <http://example.com/p> ` +
				`<<( <http://example.com/a> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://example.com/C> )>> .`,
		},
		{
			name: "a triple term holding a literal",
			src:  `:s :p <<( :a :b "o"@en )>> .`,
			want: `<http://example.com/s> <http://example.com/p> ` +
				`<<( <http://example.com/a> <http://example.com/b> "o"@en )>> .`,
		},
		{
			name: "triple terms nest",
			src:  `:s :p <<( :a :b <<( :c :d :e )>> )>> .`,
			want: `<http://example.com/s> <http://example.com/p> ` +
				`<<( <http://example.com/a> <http://example.com/b> ` +
				`<<( <http://example.com/c> <http://example.com/d> <http://example.com/e> )>> )>> .`,
		},
		{
			name: "a reified triple with no reifier mints a blank node",
			src:  `:s :p << :a :b :c >> .`,
			want: "_:r " + reifies + ` <<( <http://example.com/a> <http://example.com/b> <http://example.com/c> )>> .` + "\n" +
				`<http://example.com/s> <http://example.com/p> _:r .`,
		},
		{
			name: "a bare reifier mints a blank node too",
			src:  `:s :p << :a :b :c ~ >> .`,
			want: "_:r " + reifies + ` <<( <http://example.com/a> <http://example.com/b> <http://example.com/c> )>> .` + "\n" +
				`<http://example.com/s> <http://example.com/p> _:r .`,
		},
		{
			name: "a named reifier is the term the triple stands for",
			src:  `:s :p << :a :b :c ~ :r >> .`,
			want: `<http://example.com/r> ` + reifies + ` <<( <http://example.com/a> <http://example.com/b> <http://example.com/c> )>> .` + "\n" +
				`<http://example.com/s> <http://example.com/p> <http://example.com/r> .`,
		},
		{
			name: "a blank node reifier is the node the document named",
			src:  `:s :p << :a :b :c ~ _:r >> .`,
			want: "_:r " + reifies + ` <<( <http://example.com/a> <http://example.com/b> <http://example.com/c> )>> .` + "\n" +
				`<http://example.com/s> <http://example.com/p> _:r .`,
		},
		{
			name: "one reifier may be given to two triples",
			src:  `:s :p << :a :b :c ~ :r >> , << :d :e :f ~ :r >> .`,
			want: `<http://example.com/r> ` + reifies + ` <<( <http://example.com/a> <http://example.com/b> <http://example.com/c> )>> .` + "\n" +
				`<http://example.com/r> ` + reifies + ` <<( <http://example.com/d> <http://example.com/e> <http://example.com/f> )>> .` + "\n" +
				`<http://example.com/s> <http://example.com/p> <http://example.com/r> .` + "\n" +
				`<http://example.com/s> <http://example.com/p> <http://example.com/r> .`,
		},
		{
			name: "a reified triple in the subject position",
			src:  `<< :s :p :o >> :q :r .`,
			want: "_:b " + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .` + "\n" +
				`_:b <http://example.com/q> <http://example.com/r> .`,
		},
		{
			name: "a reified triple standing alone still states its reification",
			src:  `<< :s :p :o >> .`,
			want: "_:b " + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .`,
		},
		{
			name: "reified triples nest on the object side",
			src:  `:s :p << :a :b << :c :d :e ~ :i >> ~ :o >> .`,
			want: `<http://example.com/i> ` + reifies + ` <<( <http://example.com/c> <http://example.com/d> <http://example.com/e> )>> .` + "\n" +
				`<http://example.com/o> ` + reifies + ` <<( <http://example.com/a> <http://example.com/b> <http://example.com/i> )>> .` + "\n" +
				`<http://example.com/s> <http://example.com/p> <http://example.com/o> .`,
		},
		{
			name: "reified triples nest on the subject side",
			src:  `<< << :s :p :o ~ :i >> :q :r ~ :o >> :x :y .`,
			want: `<http://example.com/i> ` + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .` + "\n" +
				`<http://example.com/o> ` + reifies + ` <<( <http://example.com/i> <http://example.com/q> <http://example.com/r> )>> .` + "\n" +
				`<http://example.com/o> <http://example.com/x> <http://example.com/y> .`,
		},
		{
			name: "a triple term inside a reified triple",
			src:  `:s :p << :a :b <<( :c :d :e )>> ~ :r >> .`,
			want: `<http://example.com/r> ` + reifies + ` <<( <http://example.com/a> <http://example.com/b> ` +
				`<<( <http://example.com/c> <http://example.com/d> <http://example.com/e> )>> )>> .` + "\n" +
				`<http://example.com/s> <http://example.com/p> <http://example.com/r> .`,
		},
		{
			name: "a directional literal takes rdf:dirLangString",
			src:  `:s :p "o"@ar--rtl .`,
			want: `<http://example.com/s> <http://example.com/p> "o"@ar--rtl .`,
		},
		{
			name: "an annotation block asserts the triple and describes it",
			src:  `:s :p :o {| :q :v |} .`,
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o> .` + "\n" +
				"_:r " + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .` + "\n" +
				`_:r <http://example.com/q> <http://example.com/v> .`,
		},
		{
			name: "a reifier before a block is the block's subject",
			src:  `:s :p :o ~ :r {| :q :v |} .`,
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o> .` + "\n" +
				`<http://example.com/r> ` + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .` + "\n" +
				`<http://example.com/r> <http://example.com/q> <http://example.com/v> .`,
		},
		{
			name: "a bare reifier reifies under a blank node and says nothing else",
			src:  `:s :p :o ~ .`,
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o> .` + "\n" +
				"_:r " + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .`,
		},
		{
			name: "each of two reifiers gets its own rdf:reifies triple",
			src:  `:s :p :o ~ :r1 ~ :r2 .`,
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o> .` + "\n" +
				`<http://example.com/r1> ` + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .` + "\n" +
				`<http://example.com/r2> ` + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .`,
		},
		{
			name: "two blocks each reify the triple under a node of their own",
			src:  `:s :p :o {| :q :v |} {| :x :y |} .`,
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o> .` + "\n" +
				"_:r1 " + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .` + "\n" +
				`_:r1 <http://example.com/q> <http://example.com/v> .` + "\n" +
				"_:r2 " + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .` + "\n" +
				`_:r2 <http://example.com/x> <http://example.com/y> .`,
		},
		{
			name: "a block spends the reifier before it, so the next block mints one",
			src:  `:s :p :o ~ :r {| :q :v |} {| :x :y |} .`,
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o> .` + "\n" +
				`<http://example.com/r> ` + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .` + "\n" +
				`<http://example.com/r> <http://example.com/q> <http://example.com/v> .` + "\n" +
				"_:r2 " + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .` + "\n" +
				`_:r2 <http://example.com/x> <http://example.com/y> .`,
		},
		{
			name: "a reifier after a block belongs to no block and still reifies",
			src:  `:s :p :o {| :q :v |} ~ :r .`,
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o> .` + "\n" +
				"_:r1 " + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .` + "\n" +
				`_:r1 <http://example.com/q> <http://example.com/v> .` + "\n" +
				`<http://example.com/r> ` + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .`,
		},
		{
			name: "a block holds a predicate object list like any other",
			src:  `:s :p :o ~ :r {| :q :v , :w ; :x :y |} .`,
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o> .` + "\n" +
				`<http://example.com/r> ` + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .` + "\n" +
				`<http://example.com/r> <http://example.com/q> <http://example.com/v> .` + "\n" +
				`<http://example.com/r> <http://example.com/q> <http://example.com/w> .` + "\n" +
				`<http://example.com/r> <http://example.com/x> <http://example.com/y> .`,
		},
		{
			name: "an annotation nested inside an annotation describes the inner triple",
			src:  `:s :p :o ~ :r1 {| :q :v ~ :r2 {| :x :y |} |} .`,
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o> .` + "\n" +
				`<http://example.com/r1> ` + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .` + "\n" +
				`<http://example.com/r1> <http://example.com/q> <http://example.com/v> .` + "\n" +
				`<http://example.com/r2> ` + reifies + ` <<( <http://example.com/r1> <http://example.com/q> <http://example.com/v> )>> .` + "\n" +
				`<http://example.com/r2> <http://example.com/x> <http://example.com/y> .`,
		},
		{
			name: "an annotation after a comma describes the second object's triple",
			src:  `:s :p :o1 , :o2 ~ :r {| :q :v |} .`,
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o1> .` + "\n" +
				`<http://example.com/s> <http://example.com/p> <http://example.com/o2> .` + "\n" +
				`<http://example.com/r> ` + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o2> )>> .` + "\n" +
				`<http://example.com/r> <http://example.com/q> <http://example.com/v> .`,
		},
		{
			name: "an annotation before a comma describes the first object's triple",
			src:  `:s :p :o1 ~ :r {| :q :v |} , :o2 .`,
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o1> .` + "\n" +
				`<http://example.com/s> <http://example.com/p> <http://example.com/o2> .` + "\n" +
				`<http://example.com/r> ` + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o1> )>> .` + "\n" +
				`<http://example.com/r> <http://example.com/q> <http://example.com/v> .`,
		},
		{
			name: "an annotation before a semicolon describes the first verb's triple",
			src:  `:s :p :o ~ :r {| :q :v |} ; :x :y .`,
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o> .` + "\n" +
				`<http://example.com/s> <http://example.com/x> <http://example.com/y> .` + "\n" +
				`<http://example.com/r> ` + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .` + "\n" +
				`<http://example.com/r> <http://example.com/q> <http://example.com/v> .`,
		},
		{
			name: "every object of a list may carry an annotation of its own",
			src:  `:s :p :o1 ~ :r1 , :o2 ~ :r2 .`,
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o1> .` + "\n" +
				`<http://example.com/s> <http://example.com/p> <http://example.com/o2> .` + "\n" +
				`<http://example.com/r1> ` + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o1> )>> .` + "\n" +
				`<http://example.com/r2> ` + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o2> )>> .`,
		},
		{
			name: "an annotation on a reified triple describes the triple that uses its reifier",
			src:  `:s :p << :a :b :c ~ :i >> ~ :o {| :q :v |} .`,
			want: `<http://example.com/i> ` + reifies + ` <<( <http://example.com/a> <http://example.com/b> <http://example.com/c> )>> .` + "\n" +
				`<http://example.com/s> <http://example.com/p> <http://example.com/i> .` + "\n" +
				`<http://example.com/o> ` + reifies + ` <<( <http://example.com/s> <http://example.com/p> <http://example.com/i> )>> .` + "\n" +
				`<http://example.com/o> <http://example.com/q> <http://example.com/v> .`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeTurtle(t, prefixed+tc.src)
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

// TestDecodeRDF11Constructs covers the promise that everything RDF 1.1 Turtle
// meant is still meant here.
func TestDecodeRDF11Constructs(t *testing.T) {
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
			name: "a prefixed name expands against its prefix",
			src:  `:s :p :o .`,
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o> .`,
		},
		{
			name: "the a keyword is rdf:type",
			src:  `:s a :Thing .`,
			want: `<http://example.com/s> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://example.com/Thing> .`,
		},
		{
			name: "a predicate object list says several things about one subject",
			src:  `:s :p :o ; :q :r .`,
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o> .` + "\n" +
				`<http://example.com/s> <http://example.com/q> <http://example.com/r> .`,
		},
		{
			name: "a collection expands into its cells",
			src:  `:s :p ( :a :b ) .`,
			want: "_:c0 <http://www.w3.org/1999/02/22-rdf-syntax-ns#first> <http://example.com/a> .\n" +
				"_:c0 <http://www.w3.org/1999/02/22-rdf-syntax-ns#rest> _:c1 .\n" +
				"_:c1 <http://www.w3.org/1999/02/22-rdf-syntax-ns#first> <http://example.com/b> .\n" +
				"_:c1 <http://www.w3.org/1999/02/22-rdf-syntax-ns#rest> <http://www.w3.org/1999/02/22-rdf-syntax-ns#nil> .\n" +
				"<http://example.com/s> <http://example.com/p> _:c0 .",
		},
		{
			name: "an empty collection is rdf:nil",
			src:  `:s :p () .`,
			want: `<http://example.com/s> <http://example.com/p> <http://www.w3.org/1999/02/22-rdf-syntax-ns#nil> .`,
		},
		{
			name: "a blank node property list describes a node of its own",
			src:  `:s :p [ :q :o ] .`,
			want: "_:b <http://example.com/q> <http://example.com/o> .\n" +
				"<http://example.com/s> <http://example.com/p> _:b .",
		},
		{
			name: "the shorthands carry the datatype their form implies",
			src:  `:s :p 42 , 3.14 , 1e6 , true .`,
			want: `<http://example.com/s> <http://example.com/p> "42"^^<http://www.w3.org/2001/XMLSchema#integer> .` + "\n" +
				`<http://example.com/s> <http://example.com/p> "3.14"^^<http://www.w3.org/2001/XMLSchema#decimal> .` + "\n" +
				`<http://example.com/s> <http://example.com/p> "1e6"^^<http://www.w3.org/2001/XMLSchema#double> .` + "\n" +
				`<http://example.com/s> <http://example.com/p> "true"^^<http://www.w3.org/2001/XMLSchema#boolean> .`,
		},
		{
			name: "a quoted string with neither tag nor type is xsd:string",
			src:  `:s :p "o" , "o"@en , "1"^^<http://www.w3.org/2001/XMLSchema#integer> .`,
			want: `<http://example.com/s> <http://example.com/p> "o" .` + "\n" +
				`<http://example.com/s> <http://example.com/p> "o"@en .` + "\n" +
				`<http://example.com/s> <http://example.com/p> "1"^^<http://www.w3.org/2001/XMLSchema#integer> .`,
		},
		{
			name: "an anonymous blank node is a node nothing can name again",
			src:  `:s :p [] .`,
			want: `<http://example.com/s> <http://example.com/p> _:b .`,
		},
		{
			name: "a datatype may be written as a prefixed name",
			src:  `:s :p "1"^^:dt .`,
			want: `<http://example.com/s> <http://example.com/p> "1"^^<http://example.com/dt> .`,
		},
		{
			name: "comments say nothing and are dropped",
			src:  "# one\n:s :p :o . # two",
			want: `<http://example.com/s> <http://example.com/p> <http://example.com/o> .`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := decodeTurtle(t, prefixed+tc.src)
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

// TestDecodeBaseResolution covers the directives, which a reified triple's
// terms go through exactly as a statement's do.
func TestDecodeBaseResolution(t *testing.T) {
	got := decodeTurtle(t, "@base <http://example.com/> .\n<< <s> <p> <o> ~ <r> >> <q> <v> .")
	want := decodeNTriples(t,
		`<http://example.com/r> `+reifies+` <<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>> .`+"\n"+
			`<http://example.com/r> <http://example.com/q> <http://example.com/v> .`)

	if !rdf.Isomorphic(graphOfTriples(t, got), graphOfTriples(t, want)) {
		t.Errorf("Decode() gave a graph that is not the one wanted:\ngot:\n%swant:\n%s",
			formatTriples(got), formatTriples(want))
	}
}

// TestDecodeDoesNotAssertTheReifiedTriple pins the rule that separates the
// sugar from an assertion: writing "<< :s :p :o >>" says something about
// ":s :p :o" and does not say that ":s :p :o" holds (RDF 1.2 Turtle §2.11).
func TestDecodeDoesNotAssertTheReifiedTriple(t *testing.T) {
	sources := []string{
		`<< :s :p :o >> :q :r .`,
		`:x :y << :s :p :o >> .`,
		`<< :s :p :o >> .`,
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			for _, triple := range decodeTurtle(t, prefixed+src) {
				if triple.Predicate == "http://example.com/p" {
					t.Errorf("Decode() asserted the reified triple: %s", triple)
				}
			}
		})
	}
}

// TestDecodeAssertsTheAnnotatedTriple pins what separates the annotation
// syntax from the reified-triple sugar: ":s :p :o {| :q :v |}" says that
// ":s :p :o" holds and then says something about it, where "<< :s :p :o >>"
// says only the second (RDF 1.2 Turtle §7.3).
func TestDecodeAssertsTheAnnotatedTriple(t *testing.T) {
	sources := []string{
		`:s :p :o {| :q :v |} .`,
		`:s :p :o ~ :r .`,
		`:s :p :o ~ :r {| :q :v |} .`,
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			var asserted bool
			for _, triple := range decodeTurtle(t, prefixed+src) {
				if triple.Predicate == "http://example.com/p" {
					asserted = true
				}
			}
			if !asserted {
				t.Error("Decode() did not assert the annotated triple")
			}
		})
	}
}

// TestDecodeAnnotationOfABlankNodeSubject covers an annotation on a triple
// whose subject is a node the document did not name, which is the one case
// [rdf.Isomorphic] cannot state as an expected graph: it compares the blank
// nodes inside a triple term by label rather than mapping them, and the label a
// scope mints is its own. So the triples are read back instead.
func TestDecodeAnnotationOfABlankNodeSubject(t *testing.T) {
	triples := decodeTurtle(t, prefixed+`:x :y [ :p :o ~ :r {| :q :v |} ] .`)

	// The node the property list describes is whatever ":x :y" points at.
	var node rdf.Term
	for _, triple := range triples {
		if triple.Predicate == "http://example.com/y" {
			node = triple.Object
		}
	}
	if _, ok := node.(rdf.BlankNode); !ok {
		t.Fatalf("the property list lowered to %T, want an rdf.BlankNode", node)
	}

	want := rdf.Triple{
		Subject:   rdf.IRI("http://example.com/r"),
		Predicate: vocab.RDFReifies,
		Object: rdf.TripleTerm{
			Subject:   node,
			Predicate: "http://example.com/p",
			Object:    rdf.IRI("http://example.com/o"),
		},
	}
	if !containsTriple(triples, want) {
		t.Errorf("Decode() did not reify the triple about the property list's node:\n%s", formatTriples(triples))
	}
}

func containsTriple(triples []rdf.Triple, want rdf.Triple) bool {
	for _, triple := range triples {
		if triple.Equal(want) {
			return true
		}
	}
	return false
}

// TestDecodeMintsAFreshReifierEachTime covers the criterion that a reifier the
// document did not name is a blank node of its own, so two triples written the
// same way are still two reifications.
func TestDecodeMintsAFreshReifierEachTime(t *testing.T) {
	sources := []string{
		`:s :p << :a :b :c >> , << :a :b :c >> .`,
		`:s :p << :a :b :c ~ >> , << :a :b :c ~ >> .`,
		`:s :p :o {| :q :v |} {| :q :v |} .`,
		`:s :p :o ~ , :o ~ .`,
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			reifiers := make(map[rdf.Term]bool)
			for _, triple := range decodeTurtle(t, prefixed+src) {
				if triple.Predicate != vocab.RDFReifies {
					continue
				}
				if _, ok := triple.Subject.(rdf.BlankNode); !ok {
					t.Fatalf("reifier is %T, want an rdf.BlankNode", triple.Subject)
				}
				reifiers[triple.Subject] = true
			}

			if len(reifiers) != 2 {
				t.Errorf("Decode() used %d distinct reifiers, want 2", len(reifiers))
			}
		})
	}
}

// TestDecodeNestsBeforeItEncloses pins the order [turtle.Decode] documents: the
// triples of anything nested come before the triple that uses it. Nothing about
// a graph depends on the order, so only the promise the doc comment makes is
// being kept here.
func TestDecodeNestsBeforeItEncloses(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want []rdf.IRI
	}{
		{
			name: "a reified triple in the object position",
			src:  `:s :p << :a :b :c >> .`,
			want: []rdf.IRI{vocab.RDFReifies, "http://example.com/p"},
		},
		{
			name: "a reified triple in the subject position",
			src:  `<< :s :p :o >> :q :r .`,
			want: []rdf.IRI{vocab.RDFReifies, "http://example.com/q"},
		},
		{
			name: "reified triples nested in one another",
			src:  `:s :p << :a :b << :c :d :e >> >> .`,
			want: []rdf.IRI{vocab.RDFReifies, vocab.RDFReifies, "http://example.com/p"},
		},
		{
			name: "an annotation comes after the triple it describes",
			src:  `:s :p :o {| :q :v |} .`,
			want: []rdf.IRI{"http://example.com/p", vocab.RDFReifies, "http://example.com/q"},
		},
		{
			name: "an annotation of a reified triple waits for both",
			src:  `:s :p << :a :b :c >> {| :q :v |} .`,
			want: []rdf.IRI{
				vocab.RDFReifies, "http://example.com/p",
				vocab.RDFReifies, "http://example.com/q",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := make([]rdf.IRI, 0)
			for _, triple := range decodeTurtle(t, prefixed+tc.src) {
				got = append(got, triple.Predicate)
			}
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("Decode() yielded predicates %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDecodeErrors(t *testing.T) {
	testCases := []struct {
		name string
		src  string
	}{
		{
			name: "an undefined prefix in a triple term",
			src:  `<http://example.com/s> <http://example.com/p> <<( ex:a ex:b ex:c )>> .`,
		},
		{
			name: "an undefined prefix in a reifier",
			src:  `<http://example.com/s> <http://example.com/p> << <http://example.com/a> <http://example.com/b> <http://example.com/c> ~ ex:r >> .`,
		},
		{
			name: "a relative iri in a triple term with no base",
			src:  `<http://example.com/s> <http://example.com/p> <<( <a> <b> <c> )>> .`,
		},
		{
			name: "a relative iri in a reified triple with no base",
			src:  `<< <s> <http://example.com/p> <http://example.com/o> >> <http://example.com/q> <http://example.com/r> .`,
		},
		{
			name: "an empty predicate iri in a triple term",
			src:  `<http://example.com/s> <http://example.com/p> <<( <http://example.com/a> <> <http://example.com/c> )>> .`,
		},
		{
			name: "a base direction that is neither ltr nor rtl",
			src:  `<http://example.com/s> <http://example.com/p> "o"@en--up .`,
		},
		{
			name: "an undefined prefix in an annotation's reifier",
			src:  `<http://example.com/s> <http://example.com/p> <http://example.com/o> ~ ex:r .`,
		},
		{
			name: "an undefined prefix inside an annotation block",
			src:  `<http://example.com/s> <http://example.com/p> <http://example.com/o> {| ex:q <http://example.com/v> |} .`,
		},
		{
			name: "a relative iri inside an annotation block with no base",
			src:  `<http://example.com/s> <http://example.com/p> <http://example.com/o> {| <q> <http://example.com/v> |} .`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := collect2(turtle.Decode(strings.NewReader(tc.src))); err == nil {
				t.Error("Decode() = nil, want an error")
			}
		})
	}
}

func TestDecodeUndefinedPrefixPosition(t *testing.T) {
	src := `<http://example.com/s> <http://example.com/p> <<( ex:a <http://example.com/b> <http://example.com/c> )>> .`

	_, err := collect2(turtle.Decode(strings.NewReader(src)))

	var undefined turtle.UndefinedPrefixError
	if !errors.As(err, &undefined) {
		t.Fatalf("Decode() error = %#v, want an UndefinedPrefixError", err)
	}
	if want := pos(1, 51); undefined.Pos != want {
		t.Errorf("Pos = %s, want %s", undefined.Pos, want)
	}
	if undefined.Prefix != "ex" {
		t.Errorf("Prefix = %q, want %q", undefined.Prefix, "ex")
	}
}

func TestDecodeNoBase(t *testing.T) {
	src := `<< <s> <http://example.com/p> <http://example.com/o> >> <http://example.com/q> <http://example.com/r> .`

	_, err := collect2(turtle.Decode(strings.NewReader(src)))
	if !errors.Is(err, turtle.ErrNoBase) {
		t.Errorf("Decode() error = %v, want %v", err, turtle.ErrNoBase)
	}

	var invalid turtle.InvalidTermError
	if !errors.As(err, &invalid) {
		t.Fatalf("Decode() error = %#v, want an InvalidTermError", err)
	}
	if want := pos(1, 4); invalid.Pos != want {
		t.Errorf("Pos = %s, want %s", invalid.Pos, want)
	}
}

func TestDecodeStopsWhenTheConsumerDoes(t *testing.T) {
	testCases := []struct {
		name string
		src  string
	}{
		{
			// The first triple of this document is the reification, which is
			// emitted from inside the term rather than from the statement, so
			// stopping here is what unwinds the lowering through its deepest
			// path.
			name: "inside a reified triple",
			src:  prefixed + "<< :s :p :o >> :q :r .\n:a :b :c .\n",
		},
		{
			// An annotation emits after the statement's own triple rather than
			// before it, so stopping unwinds a path the case above does not
			// reach.
			name: "inside an annotation",
			src:  prefixed + ":s :p :o {| :q :v |} .\n:a :b :c .\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var seen int
			for range turtle.Decode(strings.NewReader(tc.src)) {
				seen++
				break
			}
			if seen != 1 {
				t.Errorf("yielded %d triples after break, want 1", seen)
			}
		})
	}
}

func TestDecodeErrorMessages(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "an invalid term",
			err:  turtle.InvalidTermError{Pos: pos(3, 12), Err: turtle.ErrNoBase},
			want: "invalid term at line 3, column 12: turtle: relative IRI with no base in scope",
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

func TestLower(t *testing.T) {
	t.Run("a nil document has no triples", func(t *testing.T) {
		triples, err := turtle.Lower(nil)
		if err != nil {
			t.Fatalf("Lower() = %v, want nil", err)
		}
		if len(triples) != 0 {
			t.Errorf("Lower() yielded %d triples, want 0", len(triples))
		}
	})

	t.Run("a parsed document lowers as Decode does", func(t *testing.T) {
		src := prefixed + `:s :p << :a :b :c ~ :r >> ~ :a1 {| :q :v |} .`

		got, err := turtle.Lower(parse(t, src))
		if err != nil {
			t.Fatalf("Lower() = %v, want nil", err)
		}

		want := decodeTurtle(t, src)
		if !rdf.Isomorphic(graphOfTriples(t, got), graphOfTriples(t, want)) {
			t.Errorf("Lower() gave a graph that is not the one Decode gave:\ngot:\n%swant:\n%s",
				formatTriples(got), formatTriples(want))
		}
	})

	t.Run("a document the data model refuses is an error", func(t *testing.T) {
		doc := parse(t, `:s :p <<( :a :b :c )>> .`)

		if _, err := turtle.Lower(doc); err == nil {
			t.Error("Lower() = nil, want an error")
		}
	})
}
