package rdf_test

import (
	"errors"
	"slices"
	"testing"

	rdf "github.com/z5labs/rdf-go"
)

// tripleOf builds a triple from an IRI subject and predicate, the shape most
// of these tables need.
func tripleOf(subject, predicate rdf.IRI, object rdf.Term) rdf.Triple {
	return rdf.Triple{Subject: subject, Predicate: predicate, Object: object}
}

// equalTriples reports whether got and want hold the same triples in the same
// order, by RDF term equality.
func equalTriples(got, want []rdf.Triple) bool {
	return slices.EqualFunc(got, want, rdf.Triple.Equal)
}

func TestGraphAdd(t *testing.T) {
	t.Run("the zero graph is ready to use", func(t *testing.T) {
		var g rdf.Graph

		tr := tripleOf("http://example.com/s", "http://example.com/p", rdf.NewLiteral("o"))
		if err := g.Add(tr); err != nil {
			t.Fatalf("Add() = %v, want nil", err)
		}
		if got, want := g.Len(), 1; got != want {
			t.Errorf("Len() = %d, want %d", got, want)
		}
	})

	t.Run("an invalid triple is rejected and the graph is unchanged", func(t *testing.T) {
		g := rdf.NewGraph()

		tr := rdf.Triple{
			Subject:   rdf.NewLiteral("s"),
			Predicate: "http://example.com/p",
			Object:    rdf.NewLiteral("o"),
		}
		if err := g.Add(tr); !errors.Is(err, rdf.ErrInvalidSubject) {
			t.Errorf("Add() = %v, want %v", err, rdf.ErrInvalidSubject)
		}
		if got, want := g.Len(), 0; got != want {
			t.Errorf("Len() = %d, want %d", got, want)
		}
	})
}

// TestGraphDeduplication covers the set semantics of Graph: a triple already
// held is a no-op, where "already held" is RDF term equality rather than ==.
func TestGraphDeduplication(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T) (first, second rdf.Triple)
		want  int
	}{
		{
			name: "the same triple twice",
			build: func(*testing.T) (rdf.Triple, rdf.Triple) {
				tr := tripleOf("http://example.com/s", "http://example.com/p", rdf.NewLiteral("o"))
				return tr, tr
			},
			want: 1,
		},
		{
			name: "literals differing only in language tag case",
			build: func(t *testing.T) (rdf.Triple, rdf.Triple) {
				return tripleOf("http://example.com/s", "http://example.com/p", mustLanguageLiteral(t, "o", "en-GB")),
					tripleOf("http://example.com/s", "http://example.com/p", mustLanguageLiteral(t, "o", "EN-gb"))
			},
			want: 1,
		},
		{
			name: "the zero literal and the empty xsd:string literal",
			build: func(*testing.T) (rdf.Triple, rdf.Triple) {
				var zero rdf.Literal
				return tripleOf("http://example.com/s", "http://example.com/p", zero),
					tripleOf("http://example.com/s", "http://example.com/p", rdf.NewLiteral(""))
			},
			want: 1,
		},
		{
			name: "an explicit xsd:string datatype and the default one",
			build: func(t *testing.T) (rdf.Triple, rdf.Triple) {
				return tripleOf("http://example.com/s", "http://example.com/p", mustTypedLiteral(t, "o", rdf.XSDString)),
					tripleOf("http://example.com/s", "http://example.com/p", rdf.NewLiteral("o"))
			},
			want: 1,
		},
		{
			// The folding a key applies reaches into a triple term, because
			// TripleTerm.Equal compares its positions with term equality too.
			name: "triple terms enclosing literals that differ only in language tag case",
			build: func(t *testing.T) (rdf.Triple, rdf.Triple) {
				enclosing := func(object rdf.Term) rdf.Triple {
					return tripleOf("http://example.com/s", "http://example.com/p", rdf.TripleTerm{
						Subject:   rdf.IRI("http://example.com/a"),
						Predicate: "http://example.com/b",
						Object:    object,
					})
				}
				return enclosing(mustLanguageLiteral(t, "o", "en-GB")),
					enclosing(mustLanguageLiteral(t, "o", "EN-gb"))
			},
			want: 1,
		},
		{
			name: "a different object is a different triple",
			build: func(*testing.T) (rdf.Triple, rdf.Triple) {
				return tripleOf("http://example.com/s", "http://example.com/p", rdf.NewLiteral("o")),
					tripleOf("http://example.com/s", "http://example.com/p", rdf.NewLiteral("other"))
			},
			want: 2,
		},
		{
			name: "an iri object and a blank node with the same text differ",
			build: func(*testing.T) (rdf.Triple, rdf.Triple) {
				return tripleOf("http://example.com/s", "http://example.com/p", rdf.IRI("o")),
					tripleOf("http://example.com/s", "http://example.com/p", rdf.NewBlankNode("o"))
			},
			want: 2,
		},
		{
			name: "a literal object and an iri with the same text differ",
			build: func(*testing.T) (rdf.Triple, rdf.Triple) {
				return tripleOf("http://example.com/s", "http://example.com/p", rdf.NewLiteral("o")),
					tripleOf("http://example.com/s", "http://example.com/p", rdf.IRI("o"))
			},
			want: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first, second := test.build(t)

			g := rdf.NewGraph()
			for _, tr := range []rdf.Triple{first, second} {
				if err := g.Add(tr); err != nil {
					t.Fatalf("Add(%s) = %v, want nil", tr, err)
				}
			}

			if got := g.Len(); got != test.want {
				t.Errorf("Len() = %d, want %d", got, test.want)
			}
			if got := len(slices.Collect(g.All())); got != test.want {
				t.Errorf("All() yielded %d triples, want %d", got, test.want)
			}
			for _, tr := range []rdf.Triple{first, second} {
				if !g.Has(tr) {
					t.Errorf("Has(%s) = false, want true", tr)
				}
			}
		})
	}
}

// TestGraphDeduplicationKeepsFirstForm asserts the documented tie-break: the
// graph keeps the triple as it was first added, even though a later equal
// triple would render differently.
func TestGraphDeduplicationKeepsFirstForm(t *testing.T) {
	first := tripleOf("http://example.com/s", "http://example.com/p", mustLanguageLiteral(t, "o", "en-GB"))
	second := tripleOf("http://example.com/s", "http://example.com/p", mustLanguageLiteral(t, "o", "EN-GB"))

	g := rdf.NewGraph()
	for _, tr := range []rdf.Triple{first, second} {
		if err := g.Add(tr); err != nil {
			t.Fatalf("Add(%s) = %v, want nil", tr, err)
		}
	}

	got := slices.Collect(g.All())
	if len(got) != 1 {
		t.Fatalf("All() yielded %d triples, want 1", len(got))
	}
	if want := first.String(); got[0].String() != want {
		t.Errorf("kept %s, want %s", got[0], want)
	}
}

func TestGraphHas(t *testing.T) {
	held := tripleOf("http://example.com/s", "http://example.com/p", rdf.NewLiteral("o"))

	g := rdf.NewGraph()
	if err := g.Add(held); err != nil {
		t.Fatalf("Add() = %v, want nil", err)
	}

	tests := []struct {
		name   string
		triple rdf.Triple
		want   bool
	}{
		{
			name:   "a triple that was added",
			triple: held,
			want:   true,
		},
		{
			name:   "a triple that was not added",
			triple: tripleOf("http://example.com/s", "http://example.com/p", rdf.NewLiteral("other")),
		},
		{
			name: "an invalid triple is never held",
			triple: rdf.Triple{
				Subject:   rdf.NewLiteral("s"),
				Predicate: "http://example.com/p",
				Object:    rdf.NewLiteral("o"),
			},
		},
		{
			name:   "the zero triple is never held",
			triple: rdf.Triple{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := g.Has(test.triple); got != test.want {
				t.Errorf("Has() = %t, want %t", got, test.want)
			}
		})
	}
}

// TestGraphAllIsInsertionOrdered pins the documented iteration order. Graph
// indexes its triples in a map, whose range order is randomized, so this would
// fail were the order not maintained separately.
func TestGraphAllIsInsertionOrdered(t *testing.T) {
	want := []rdf.Triple{
		tripleOf("http://example.com/c", "http://example.com/p", rdf.NewLiteral("3")),
		tripleOf("http://example.com/a", "http://example.com/p", rdf.NewLiteral("1")),
		tripleOf("http://example.com/b", "http://example.com/p", rdf.NewLiteral("2")),
		tripleOf("http://example.com/a", "http://example.com/q", rdf.NewLiteral("4")),
		tripleOf("http://example.com/b", "http://example.com/p", rdf.NewLiteral("5")),
	}

	g := rdf.NewGraph()
	for _, tr := range want {
		if err := g.Add(tr); err != nil {
			t.Fatalf("Add(%s) = %v, want nil", tr, err)
		}
	}
	// A repeat of an earlier triple keeps its original position rather than
	// moving to the end.
	if err := g.Add(want[0]); err != nil {
		t.Fatalf("Add(%s) = %v, want nil", want[0], err)
	}

	// Ranging twice asserts the order is stable, not merely deterministic
	// within a single call.
	for i := range 2 {
		if got := slices.Collect(g.All()); !equalTriples(got, want) {
			t.Errorf("All() pass %d = %v, want %v", i, got, want)
		}
	}
}

func TestGraphAllStopsEarly(t *testing.T) {
	g := rdf.NewGraph()
	for _, o := range []string{"1", "2", "3"} {
		tr := tripleOf("http://example.com/s", "http://example.com/p", rdf.NewLiteral(o))
		if err := g.Add(tr); err != nil {
			t.Fatalf("Add(%s) = %v, want nil", tr, err)
		}
	}

	var seen int
	for range g.All() {
		seen++
		break
	}
	if want := 1; seen != want {
		t.Errorf("visited %d triples after break, want %d", seen, want)
	}
}

// TestGraphAllSnapshotsTheGraph covers the documented behaviour of adding to a
// graph while ranging over it: the iterator does not visit the new triples.
func TestGraphAllSnapshotsTheGraph(t *testing.T) {
	g := rdf.NewGraph()
	if err := g.Add(tripleOf("http://example.com/s", "http://example.com/p", rdf.NewLiteral("1"))); err != nil {
		t.Fatalf("Add() = %v, want nil", err)
	}

	var seen int
	for range g.All() {
		seen++
		if seen > 10 {
			t.Fatal("All() did not terminate")
		}
		if err := g.Add(tripleOf("http://example.com/s", "http://example.com/p", rdf.NewLiteral("2"))); err != nil {
			t.Fatalf("Add() = %v, want nil", err)
		}
	}

	if want := 1; seen != want {
		t.Errorf("visited %d triples, want %d", seen, want)
	}
	if got, want := g.Len(), 2; got != want {
		t.Errorf("Len() = %d, want %d", got, want)
	}
}
