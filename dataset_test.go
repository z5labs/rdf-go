package rdf_test

import (
	"errors"
	"slices"
	"testing"

	rdf "github.com/z5labs/rdf-go"
)

// quadOf pairs a triple with the graph it belongs to.
func quadOf(graph rdf.Term, subject, predicate rdf.IRI, object rdf.Term) rdf.Quad {
	return rdf.Quad{Triple: tripleOf(subject, predicate, object), Graph: graph}
}

func addQuads(t *testing.T, d *rdf.Dataset, quads ...rdf.Quad) {
	t.Helper()

	for _, q := range quads {
		if err := d.Add(q); err != nil {
			t.Fatalf("Add(%s) = %v, want nil", q, err)
		}
	}
}

func TestDatasetAdd(t *testing.T) {
	t.Run("the zero dataset is ready to use", func(t *testing.T) {
		var d rdf.Dataset

		addQuads(t, &d, quadOf(rdf.IRI("http://example.com/g"), "http://example.com/s", "http://example.com/p", rdf.NewLiteral("o")))
		if got, want := d.Len(), 1; got != want {
			t.Errorf("Len() = %d, want %d", got, want)
		}
	})

	t.Run("a quad with no graph name goes to the default graph", func(t *testing.T) {
		d := rdf.NewDataset()

		q := quadOf(nil, "http://example.com/s", "http://example.com/p", rdf.NewLiteral("o"))
		addQuads(t, d, q)

		if got, want := d.Default().Len(), 1; got != want {
			t.Errorf("Default().Len() = %d, want %d", got, want)
		}
		if !d.Default().Has(q.Triple) {
			t.Errorf("Default().Has(%s) = false, want true", q.Triple)
		}
		if got := len(slices.Collect(d.GraphNames())); got != 0 {
			t.Errorf("GraphNames() yielded %d names, want 0", got)
		}
	})

	t.Run("a repeated quad is a no-op", func(t *testing.T) {
		d := rdf.NewDataset()

		q := quadOf(rdf.IRI("http://example.com/g"), "http://example.com/s", "http://example.com/p", rdf.NewLiteral("o"))
		addQuads(t, d, q, q)

		if got, want := d.Len(), 1; got != want {
			t.Errorf("Len() = %d, want %d", got, want)
		}
	})

	t.Run("the same triple in two graphs is held by both", func(t *testing.T) {
		d := rdf.NewDataset()

		def := quadOf(nil, "http://example.com/s", "http://example.com/p", rdf.NewLiteral("o"))
		named := quadOf(rdf.IRI("http://example.com/g"), "http://example.com/s", "http://example.com/p", rdf.NewLiteral("o"))
		addQuads(t, d, def, named)

		if got, want := d.Len(), 2; got != want {
			t.Errorf("Len() = %d, want %d", got, want)
		}
		if !d.Has(def) || !d.Has(named) {
			t.Errorf("Has(default) = %t, Has(named) = %t, want both true", d.Has(def), d.Has(named))
		}
	})

	t.Run("an invalid quad leaves no graph behind", func(t *testing.T) {
		d := rdf.NewDataset()

		q := rdf.Quad{
			Triple: rdf.Triple{
				Subject:   rdf.NewLiteral("s"),
				Predicate: "http://example.com/p",
				Object:    rdf.NewLiteral("o"),
			},
			Graph: rdf.IRI("http://example.com/g"),
		}
		if err := d.Add(q); !errors.Is(err, rdf.ErrInvalidSubject) {
			t.Errorf("Add() = %v, want %v", err, rdf.ErrInvalidSubject)
		}
		if _, ok := d.NamedGraph(rdf.IRI("http://example.com/g")); ok {
			t.Error("NamedGraph() found a graph, want none")
		}
	})

	t.Run("a literal graph name is rejected", func(t *testing.T) {
		d := rdf.NewDataset()

		q := quadOf(rdf.NewLiteral("g"), "http://example.com/s", "http://example.com/p", rdf.NewLiteral("o"))
		if err := d.Add(q); !errors.Is(err, rdf.ErrInvalidGraphName) {
			t.Errorf("Add() = %v, want %v", err, rdf.ErrInvalidGraphName)
		}
		if got, want := d.Len(), 0; got != want {
			t.Errorf("Len() = %d, want %d", got, want)
		}
	})
}

func TestDatasetNamedGraph(t *testing.T) {
	d := rdf.NewDataset()
	addQuads(t,
		d,
		quadOf(rdf.IRI("http://example.com/g"), "http://example.com/s", "http://example.com/p", rdf.NewLiteral("iri graph")),
		quadOf(rdf.NewBlankNode("g"), "http://example.com/s", "http://example.com/p", rdf.NewLiteral("blank node graph")),
		quadOf(nil, "http://example.com/s", "http://example.com/p", rdf.NewLiteral("default graph")),
	)

	tests := []struct {
		name   string
		lookup rdf.Term
		want   string // the object of the graph's single triple, "" when absent
	}{
		{
			name:   "an iri names its graph",
			lookup: rdf.IRI("http://example.com/g"),
			want:   "iri graph",
		},
		{
			name:   "a blank node names its graph",
			lookup: rdf.NewBlankNode("g"),
			want:   "blank node graph",
		},
		{
			name:   "an iri does not find a blank node's graph",
			lookup: rdf.IRI("g"),
		},
		{
			name:   "a blank node does not find an iri's graph",
			lookup: rdf.NewBlankNode("http://example.com/g"),
		},
		{
			name:   "the default graph is not reachable by name",
			lookup: nil,
		},
		{
			name:   "a name that was never added",
			lookup: rdf.IRI("http://example.com/missing"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g, ok := d.NamedGraph(test.lookup)
			if ok != (test.want != "") {
				t.Fatalf("NamedGraph() found = %t, want %t", ok, test.want != "")
			}
			if !ok {
				return
			}

			got := slices.Collect(g.All())
			if len(got) != 1 {
				t.Fatalf("All() yielded %d triples, want 1", len(got))
			}
			if want := rdf.NewLiteral(test.want); !got[0].Object.Equal(want) {
				t.Errorf("object = %s, want %s", got[0].Object, want)
			}
		})
	}
}

func TestDatasetAddGraph(t *testing.T) {
	t.Run("an empty named graph can be added", func(t *testing.T) {
		d := rdf.NewDataset()

		name := rdf.IRI("http://example.com/g")
		g, err := d.AddGraph(name)
		if err != nil {
			t.Fatalf("AddGraph() = %v, want nil", err)
		}
		if got, want := g.Len(), 0; got != want {
			t.Errorf("Len() = %d, want %d", got, want)
		}

		found, ok := d.NamedGraph(name)
		if !ok {
			t.Fatal("NamedGraph() found no graph, want one")
		}
		if found != g {
			t.Error("NamedGraph() returned a different graph than AddGraph()")
		}
		if got := slices.Collect(d.GraphNames()); len(got) != 1 || !got[0].Equal(name) {
			t.Errorf("GraphNames() = %v, want [%s]", got, name)
		}
	})

	t.Run("adding an existing graph returns it unchanged", func(t *testing.T) {
		d := rdf.NewDataset()

		name := rdf.IRI("http://example.com/g")
		addQuads(t, d, quadOf(name, "http://example.com/s", "http://example.com/p", rdf.NewLiteral("o")))

		g, err := d.AddGraph(name)
		if err != nil {
			t.Fatalf("AddGraph() = %v, want nil", err)
		}
		if got, want := g.Len(), 1; got != want {
			t.Errorf("Len() = %d, want %d", got, want)
		}
		if got, want := len(slices.Collect(d.GraphNames())), 1; got != want {
			t.Errorf("GraphNames() yielded %d names, want %d", got, want)
		}
	})

	t.Run("writing to the returned graph writes to the dataset", func(t *testing.T) {
		d := rdf.NewDataset()

		name := rdf.IRI("http://example.com/g")
		g, err := d.AddGraph(name)
		if err != nil {
			t.Fatalf("AddGraph() = %v, want nil", err)
		}

		q := quadOf(name, "http://example.com/s", "http://example.com/p", rdf.NewLiteral("o"))
		if err := g.Add(q.Triple); err != nil {
			t.Fatalf("Add() = %v, want nil", err)
		}
		if !d.Has(q) {
			t.Errorf("Has(%s) = false, want true", q)
		}
	})

	t.Run("writing to the default graph writes to the dataset", func(t *testing.T) {
		d := rdf.NewDataset()

		q := quadOf(nil, "http://example.com/s", "http://example.com/p", rdf.NewLiteral("o"))
		if err := d.Default().Add(q.Triple); err != nil {
			t.Fatalf("Add() = %v, want nil", err)
		}
		if !d.Has(q) {
			t.Errorf("Has(%s) = false, want true", q)
		}
		if got, want := d.Len(), 1; got != want {
			t.Errorf("Len() = %d, want %d", got, want)
		}
	})

	tests := []struct {
		name  string
		graph rdf.Term
	}{
		{
			name:  "the default graph has no name",
			graph: nil,
		},
		{
			name:  "a literal cannot name a graph",
			graph: rdf.NewLiteral("g"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d := rdf.NewDataset()

			if _, err := d.AddGraph(test.graph); !errors.Is(err, rdf.ErrInvalidGraphName) {
				t.Errorf("AddGraph() = %v, want %v", err, rdf.ErrInvalidGraphName)
			}
		})
	}
}

// TestDatasetAllIsInsertionOrdered pins the documented iteration order: the
// default graph first, then named graphs in the order they were first added,
// each graph in its own insertion order.
func TestDatasetAllIsInsertionOrdered(t *testing.T) {
	want := []rdf.Quad{
		quadOf(nil, "http://example.com/s", "http://example.com/p", rdf.NewLiteral("default 1")),
		quadOf(nil, "http://example.com/s", "http://example.com/p", rdf.NewLiteral("default 2")),
		quadOf(rdf.IRI("http://example.com/z"), "http://example.com/s", "http://example.com/p", rdf.NewLiteral("z 1")),
		quadOf(rdf.IRI("http://example.com/z"), "http://example.com/s", "http://example.com/p", rdf.NewLiteral("z 2")),
		quadOf(rdf.NewBlankNode("a"), "http://example.com/s", "http://example.com/p", rdf.NewLiteral("a 1")),
		quadOf(rdf.IRI("http://example.com/m"), "http://example.com/s", "http://example.com/p", rdf.NewLiteral("m 1")),
	}

	// Interleaved so that neither the graph names nor the quads arrive in the
	// order All must report them.
	d := rdf.NewDataset()
	addQuads(t, d, want[2], want[0], want[4], want[3], want[5], want[1])

	wantNames := []rdf.Term{
		rdf.IRI("http://example.com/z"),
		rdf.NewBlankNode("a"),
		rdf.IRI("http://example.com/m"),
	}

	for i := range 2 {
		got := slices.Collect(d.All())
		if !slices.EqualFunc(got, want, rdf.Quad.Equal) {
			t.Errorf("All() pass %d = %v, want %v", i, got, want)
		}

		gotNames := slices.Collect(d.GraphNames())
		if !slices.EqualFunc(gotNames, wantNames, func(a, b rdf.Term) bool { return a.Equal(b) }) {
			t.Errorf("GraphNames() pass %d = %v, want %v", i, gotNames, wantNames)
		}
	}
}

func TestDatasetAllStopsEarly(t *testing.T) {
	d := rdf.NewDataset()
	addQuads(t,
		d,
		quadOf(nil, "http://example.com/s", "http://example.com/p", rdf.NewLiteral("1")),
		quadOf(rdf.IRI("http://example.com/g"), "http://example.com/s", "http://example.com/p", rdf.NewLiteral("2")),
		quadOf(rdf.IRI("http://example.com/h"), "http://example.com/s", "http://example.com/p", rdf.NewLiteral("3")),
	)

	tests := []struct {
		name  string
		after int
	}{
		{name: "in the default graph", after: 1},
		{name: "in the first named graph", after: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var seen int
			for range d.All() {
				seen++
				if seen == test.after {
					break
				}
			}
			if seen != test.after {
				t.Errorf("visited %d quads, want %d", seen, test.after)
			}
		})
	}
}
