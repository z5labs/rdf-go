package rdf_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
)

// termOf reads the shorthand these tables are written in: "_:x" is a blank
// node, a quoted string is a literal, and anything else is an IRI.
func termOf(t *testing.T, s string) rdf.Term {
	t.Helper()

	switch {
	case strings.HasPrefix(s, "_:"):
		return rdf.NewBlankNode(strings.TrimPrefix(s, "_:"))
	case strings.HasPrefix(s, `"`):
		value := strings.Trim(s, `"`)
		if before, tag, found := strings.Cut(value, "@"); found {
			literal, err := rdf.NewLanguageLiteral(before, tag)
			if err != nil {
				t.Fatalf("NewLanguageLiteral(%q, %q) = %v", before, tag, err)
			}
			return literal
		}
		return rdf.NewLiteral(value)
	default:
		return rdf.IRI(s)
	}
}

// graphOf builds a graph from triples written in the shorthand of [termOf].
func graphOf(t *testing.T, triples ...[3]string) *rdf.Graph {
	t.Helper()

	g := rdf.NewGraph()
	for _, spec := range triples {
		triple := rdf.Triple{
			Subject:   termOf(t, spec[0]),
			Predicate: rdf.IRI(spec[1]),
			Object:    termOf(t, spec[2]),
		}
		if err := g.Add(triple); err != nil {
			t.Fatalf("Add(%s) = %v, want nil", triple, err)
		}
	}
	return g
}

// cycle builds a directed cycle of blank nodes labelled with the given prefix,
// each also carrying the same ground triple so that no node can be told from
// another by its ground context alone.
func cycle(t *testing.T, prefix string, n int) [][3]string {
	t.Helper()

	triples := make([][3]string, 0, 2*n)
	for i := range n {
		from := fmt.Sprintf("_:%s%d", prefix, i)
		to := fmt.Sprintf("_:%s%d", prefix, (i+1)%n)
		triples = append(triples,
			[3]string{from, "http://example.com/next", to},
			[3]string{from, "http://example.com/kind", `"cell"`},
		)
	}
	return triples
}

// cycles builds a disjoint union of directed cycles, each of length size.
func cycles(t *testing.T, prefix string, count, size int) [][3]string {
	t.Helper()

	var triples [][3]string
	for c := range count {
		triples = append(triples, cycle(t, prefix+strconv.Itoa(c)+"n", size)...)
	}
	return triples
}

// star builds n blank nodes that are pairwise indistinguishable: each is the
// subject of the same two triples and touches nothing else.
func star(t *testing.T, prefix string, n int) [][3]string {
	t.Helper()

	triples := make([][3]string, 0, 2*n)
	for i := range n {
		node := fmt.Sprintf("_:%s%d", prefix, i)
		triples = append(triples,
			[3]string{node, "http://example.com/kind", `"leaf"`},
			[3]string{node, "http://example.com/of", "http://example.com/root"},
		)
	}
	return triples
}

func TestIsomorphic(t *testing.T) {
	tests := []struct {
		name string
		a    [][3]string
		b    [][3]string
		want bool
	}{
		{
			name: "two empty graphs",
			want: true,
		},
		{
			name: "an empty graph and a ground one",
			b:    [][3]string{{"http://example.com/s", "http://example.com/p", "http://example.com/o"}},
		},
		{
			name: "identical ground graphs",
			a: [][3]string{
				{"http://example.com/s", "http://example.com/p", "http://example.com/o"},
				{"http://example.com/s", "http://example.com/q", `"literal"`},
			},
			b: [][3]string{
				{"http://example.com/s", "http://example.com/p", "http://example.com/o"},
				{"http://example.com/s", "http://example.com/q", `"literal"`},
			},
			want: true,
		},
		{
			name: "ground graphs are compared as sets, not sequences",
			a: [][3]string{
				{"http://example.com/s", "http://example.com/p", "http://example.com/o"},
				{"http://example.com/s", "http://example.com/q", `"literal"`},
			},
			b: [][3]string{
				{"http://example.com/s", "http://example.com/q", `"literal"`},
				{"http://example.com/s", "http://example.com/p", "http://example.com/o"},
			},
			want: true,
		},
		{
			name: "ground graphs of the same size differing in one object",
			a: [][3]string{
				{"http://example.com/s", "http://example.com/p", "http://example.com/o"},
				{"http://example.com/s", "http://example.com/q", `"literal"`},
			},
			b: [][3]string{
				{"http://example.com/s", "http://example.com/p", "http://example.com/other"},
				{"http://example.com/s", "http://example.com/q", `"literal"`},
			},
		},
		{
			name: "ground terms may not be relabelled",
			a:    [][3]string{{"http://example.com/s", "http://example.com/p", "http://example.com/o"}},
			b:    [][3]string{{"http://example.com/other", "http://example.com/p", "http://example.com/o"}},
		},
		{
			name: "a blank node is not a ground term",
			a:    [][3]string{{"_:a", "http://example.com/p", "http://example.com/o"}},
			b:    [][3]string{{"http://example.com/s", "http://example.com/p", "http://example.com/o"}},
		},
		{
			name: "one relabelled blank node",
			a:    [][3]string{{"_:a", "http://example.com/p", "http://example.com/o"}},
			b:    [][3]string{{"_:completelyDifferentLabel", "http://example.com/p", "http://example.com/o"}},
			want: true,
		},
		{
			name: "blank nodes relabelled across several triples",
			a: [][3]string{
				{"_:a", "http://example.com/p", "_:b"},
				{"_:b", "http://example.com/p", "http://example.com/o"},
			},
			b: [][3]string{
				{"_:x", "http://example.com/p", "_:y"},
				{"_:y", "http://example.com/p", "http://example.com/o"},
			},
			want: true,
		},
		{
			name: "a relabelling that would have to swap two distinguishable nodes",
			a: [][3]string{
				{"_:a", "http://example.com/p", "_:b"},
				{"_:b", "http://example.com/p", "http://example.com/o"},
			},
			b: [][3]string{
				{"_:x", "http://example.com/p", "_:y"},
				{"_:x", "http://example.com/p", "http://example.com/o"},
			},
		},
		{
			name: "the same shape with a different predicate",
			a:    [][3]string{{"_:a", "http://example.com/p", "_:b"}},
			b:    [][3]string{{"_:x", "http://example.com/q", "_:y"}},
		},
		{
			name: "different numbers of blank nodes",
			a: [][3]string{
				{"_:a", "http://example.com/p", "_:b"},
				{"_:b", "http://example.com/p", "_:c"},
			},
			b: [][3]string{
				{"_:x", "http://example.com/p", "_:y"},
				{"_:y", "http://example.com/p", "_:x"},
			},
		},
		{
			name: "a self loop and a two node cycle",
			a:    [][3]string{{"_:a", "http://example.com/p", "_:a"}},
			b:    [][3]string{{"_:x", "http://example.com/p", "_:y"}},
		},
		{
			name: "a relabelled self loop",
			a:    [][3]string{{"_:a", "http://example.com/p", "_:a"}},
			b:    [][3]string{{"_:x", "http://example.com/p", "_:x"}},
			want: true,
		},
		{
			name: "literals differing only in language tag case are one term",
			a:    [][3]string{{"_:a", "http://example.com/p", `"text@en-GB"`}},
			b:    [][3]string{{"_:x", "http://example.com/p", `"text@EN-gb"`}},
			want: true,
		},
		{
			name: "a relabelled three node cycle",
			a:    cycle(t, "a", 3),
			b:    cycle(t, "x", 3),
			want: true,
		},
		{
			name: "a relabelled six node cycle",
			a:    cycle(t, "a", 6),
			b:    cycle(t, "x", 6),
			want: true,
		},
		{
			// The classic case colour refinement cannot settle on its own:
			// every node of both graphs has one predecessor, one successor and
			// the same ground triple, so all twelve nodes keep the same colour
			// forever and only the search can tell the graphs apart.
			name: "a six node cycle and two three node cycles",
			a:    cycle(t, "a", 6),
			b:    cycles(t, "x", 2, 3),
		},
		{
			name: "two three node cycles, relabelled",
			a:    cycles(t, "a", 2, 3),
			b:    cycles(t, "x", 2, 3),
			want: true,
		},
		{
			name: "a four node cycle and two two node cycles",
			a:    cycle(t, "a", 4),
			b:    cycles(t, "x", 2, 2),
		},
		{
			name: "eight indistinguishable blank nodes, relabelled",
			a:    star(t, "a", 8),
			b:    star(t, "x", 8),
			want: true,
		},
		{
			name: "seven indistinguishable blank nodes against eight",
			a:    star(t, "a", 7),
			b:    star(t, "x", 8),
		},
		{
			name: "indistinguishable nodes against a cycle of the same size",
			a:    star(t, "a", 6),
			b:    cycle(t, "x", 6),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, b := graphOf(t, test.a...), graphOf(t, test.b...)

			if got := rdf.Isomorphic(a, b); got != test.want {
				t.Errorf("Isomorphic(a, b) = %t, want %t", got, test.want)
			}
			// Isomorphism is symmetric, and the two directions take different
			// paths through the search.
			if got := rdf.Isomorphic(b, a); got != test.want {
				t.Errorf("Isomorphic(b, a) = %t, want %t", got, test.want)
			}
		})
	}
}

func TestIsomorphicIsReflexive(t *testing.T) {
	tests := []struct {
		name  string
		graph [][3]string
	}{
		{
			name: "the empty graph",
		},
		{
			name:  "a ground graph",
			graph: [][3]string{{"http://example.com/s", "http://example.com/p", `"o"`}},
		},
		{
			name:  "a cycle",
			graph: cycle(t, "a", 5),
		},
		{
			name:  "indistinguishable nodes",
			graph: star(t, "a", 6),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := graphOf(t, test.graph...)

			if !rdf.Isomorphic(g, g) {
				t.Error("Isomorphic(g, g) = false, want true")
			}
			// A separately built copy has the same labels but is a different
			// graph value.
			if !rdf.Isomorphic(g, graphOf(t, test.graph...)) {
				t.Error("Isomorphic(g, copy) = false, want true")
			}
		})
	}
}

func TestIsomorphicNilGraphs(t *testing.T) {
	empty := rdf.NewGraph()
	nonEmpty := graphOf(t, [3]string{"http://example.com/s", "http://example.com/p", "http://example.com/o"})

	tests := []struct {
		name string
		a, b *rdf.Graph
		want bool
	}{
		{
			name: "two nil graphs",
			want: true,
		},
		{
			name: "nil and empty",
			b:    empty,
			want: true,
		},
		{
			name: "empty and nil",
			a:    empty,
			want: true,
		},
		{
			name: "nil and non-empty",
			b:    nonEmpty,
		},
		{
			name: "non-empty and nil",
			a:    nonEmpty,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := rdf.Isomorphic(test.a, test.b); got != test.want {
				t.Errorf("Isomorphic() = %t, want %t", got, test.want)
			}
		})
	}
}

// TestIsomorphicTerminatesOnSymmetricGraphs is the termination guarantee in
// test form. Twelve mutually indistinguishable blank nodes admit 12! — nearly
// half a billion — bijections, so a search that explored them naively would
// not finish. Both the matching and the mismatching case have to return, and
// the test failing to complete is itself the failure.
func TestIsomorphicTerminatesOnSymmetricGraphs(t *testing.T) {
	tests := []struct {
		name string
		a, b [][3]string
		want bool
	}{
		{
			name: "twelve indistinguishable nodes against a relabelling",
			a:    star(t, "a", 12),
			b:    star(t, "x", 12),
			want: true,
		},
		{
			name: "twelve indistinguishable nodes against eleven and an odd one",
			a:    star(t, "a", 12),
			b: append(star(t, "x", 11),
				[3]string{"_:odd", "http://example.com/kind", `"leaf"`},
				[3]string{"_:odd", "http://example.com/of", "http://example.com/elsewhere"},
			),
		},
		{
			name: "four three node cycles against three four node cycles",
			a:    cycles(t, "a", 4, 3),
			b:    cycles(t, "x", 3, 4),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, b := graphOf(t, test.a...), graphOf(t, test.b...)

			if got := rdf.Isomorphic(a, b); got != test.want {
				t.Errorf("Isomorphic(a, b) = %t, want %t", got, test.want)
			}
		})
	}
}

// TestIsomorphicIgnoresInsertionOrder guards the property the evaluation tests
// lean on hardest: a parser may emit the same triples in any order, and the
// graphs still have to compare equal.
func TestIsomorphicIgnoresInsertionOrder(t *testing.T) {
	forwards := cycle(t, "a", 5)

	backwards := make([][3]string, len(forwards))
	for i, triple := range forwards {
		backwards[len(forwards)-1-i] = triple
	}

	if !rdf.Isomorphic(graphOf(t, forwards...), graphOf(t, backwards...)) {
		t.Error("Isomorphic() = false, want true")
	}
}
