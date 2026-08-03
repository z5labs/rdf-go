package rdf

import (
	"strconv"
	"testing"
)

// leafGraph builds n blank nodes that nothing can tell apart: each is the
// subject of the same two triples and touches no other blank node. Every one
// of the n! bijections between two such graphs is a valid one, which is what
// makes them the cheapest way to put the search under load.
func leafGraph(t *testing.T, prefix string, n int) *Graph {
	t.Helper()

	g := NewGraph()
	for i := range n {
		node := NewBlankNode(prefix + strconv.Itoa(i))
		for _, triple := range []Triple{
			{Subject: node, Predicate: "http://example.com/kind", Object: NewLiteral("leaf")},
			{Subject: node, Predicate: "http://example.com/of", Object: IRI("http://example.com/root")},
		} {
			if err := g.Add(triple); err != nil {
				t.Fatalf("Add(%s) = %v, want nil", triple, err)
			}
		}
	}
	return g
}

// newMatcher arranges a and b for search the way Isomorphic does, but with a
// caller-chosen budget.
func newMatcher(t *testing.T, a, b *Graph, budget int) *matcher {
	t.Helper()

	x, y := newIsoGraph(a), newIsoGraph(b)
	return &matcher{
		a:          x,
		b:          y,
		colours:    x.refine(),
		candidates: byColour(y.refine()),
		forward:    make(map[BlankNode]BlankNode, len(x.blanks)),
		used:       make(map[BlankNode]bool, len(y.blanks)),
		budget:     budget,
	}
}

// TestMatcherGivesUpOnBudget pins the documented cutoff, including the part of
// it that is a wrong answer: two graphs that really are isomorphic report false
// once the search runs out of budget. The exported [Isomorphic] cannot tell a
// caller which kind of false it returned, so the behaviour is pinned here
// rather than left to the doc comment alone.
func TestMatcherGivesUpOnBudget(t *testing.T) {
	a, b := leafGraph(t, "a", 6), leafGraph(t, "x", 6)

	t.Run("a budget of zero gives up before trying anything", func(t *testing.T) {
		m := newMatcher(t, a, b, 0)

		if got := m.match(m.order(byColour(m.colours)), 0); got {
			t.Error("match() = true, want false once the budget is spent")
		}
		if !m.exhausted {
			t.Error("exhausted = false, want true")
		}
		if got, want := m.steps, 1; got != want {
			t.Errorf("steps = %d, want %d: the search should stop at the first assignment", got, want)
		}
	})

	t.Run("running out part way unwinds the whole search", func(t *testing.T) {
		// Enough budget to descend a few levels, not enough to finish, so the
		// give-up has to propagate back through the recursion instead of
		// letting the outer levels carry on with other candidates.
		m := newMatcher(t, a, b, 3)

		if got := m.match(m.order(byColour(m.colours)), 0); got {
			t.Error("match() = true, want false once the budget is spent")
		}
		if !m.exhausted {
			t.Error("exhausted = false, want true")
		}
		if got, want := m.steps, 4; got != want {
			t.Errorf("steps = %d, want %d: one assignment past the budget and no more", got, want)
		}
	})

	t.Run("the same graphs are isomorphic given the real budget", func(t *testing.T) {
		if !Isomorphic(a, b) {
			t.Error("Isomorphic() = false, want true")
		}
	})

	t.Run("a budget the search stays under is not reported as exhausted", func(t *testing.T) {
		m := newMatcher(t, a, b, maxIsomorphismSteps)

		if got := m.match(m.order(byColour(m.colours)), 0); !got {
			t.Error("match() = false, want true")
		}
		if m.exhausted {
			t.Error("exhausted = true, want false")
		}
		if m.steps > m.budget {
			t.Errorf("steps = %d, want no more than the budget of %d", m.steps, m.budget)
		}
	})
}

// TestRefinementSeparatesWhatItCan records where colour refinement stops being
// enough, which is the whole reason the backtracking search exists. A cycle of
// six and two cycles of three are the standard pair it cannot tell apart: every
// node of both has one predecessor and one successor, so all of them keep the
// same colour however long refinement runs.
func TestRefinementSeparatesWhatItCan(t *testing.T) {
	chain := func(t *testing.T, prefix string, n int) *Graph {
		t.Helper()

		g := NewGraph()
		for i := range n {
			triple := Triple{
				Subject:   NewBlankNode(prefix + strconv.Itoa(i)),
				Predicate: "http://example.com/next",
				Object:    NewBlankNode(prefix + strconv.Itoa((i+1)%n)),
			}
			if err := g.Add(triple); err != nil {
				t.Fatalf("Add(%s) = %v, want nil", triple, err)
			}
		}
		return g
	}

	tests := []struct {
		name   string
		graph  *Graph
		blanks int
		want   int
	}{
		{
			name:   "a cycle leaves every node the same colour",
			graph:  chain(t, "a", 6),
			blanks: 6,
			want:   1,
		},
		{
			name:   "so does a disjoint pair of smaller cycles",
			graph:  mergeGraphs(t, chain(t, "x", 3), chain(t, "y", 3)),
			blanks: 6,
			want:   1,
		},
		{
			name: "a chain of distinguishable nodes splits completely",
			graph: mustGraph(t,
				Triple{Subject: NewBlankNode("a"), Predicate: "http://example.com/p", Object: NewBlankNode("b")},
				Triple{Subject: NewBlankNode("b"), Predicate: "http://example.com/p", Object: IRI("http://example.com/end")},
			),
			blanks: 2,
			want:   2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			iso := newIsoGraph(test.graph)
			if got := len(iso.blanks); got != test.blanks {
				t.Fatalf("graph holds %d blank nodes, want %d", got, test.blanks)
			}
			if got := countDistinct(iso.refine()); got != test.want {
				t.Errorf("refinement found %d colour classes, want %d", got, test.want)
			}
		})
	}
}

func mustGraph(t *testing.T, triples ...Triple) *Graph {
	t.Helper()

	g := NewGraph()
	for _, triple := range triples {
		if err := g.Add(triple); err != nil {
			t.Fatalf("Add(%s) = %v, want nil", triple, err)
		}
	}
	return g
}

func mergeGraphs(t *testing.T, graphs ...*Graph) *Graph {
	t.Helper()

	merged := NewGraph()
	for _, g := range graphs {
		for triple := range g.All() {
			if err := merged.Add(triple); err != nil {
				t.Fatalf("Add(%s) = %v, want nil", triple, err)
			}
		}
	}
	return merged
}
