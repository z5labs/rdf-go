package rdf

import (
	"fmt"
	"iter"
	"slices"
)

// Dataset is a collection of RDF graphs: one default graph, which has no name,
// and zero or more named graphs keyed by an [IRI] or a [BlankNode] (RDF 1.1
// Concepts §4).
//
// The graphs are independent sets. The same triple may appear in the default
// graph and in any number of named graphs; each holds its own copy.
//
// Iteration order is insertion order throughout: [Dataset.GraphNames] visits
// names in the order their graphs were first added, [Dataset.All] visits the
// default graph before any named graph, and each graph iterates as [Graph]
// documents.
//
// The zero Dataset is an empty dataset, ready to use. As with [Graph], a
// Dataset must not be copied after its first insertion; pass a *Dataset.
type Dataset struct {
	def   Graph
	named map[Term]*Graph
	order []Term
}

// NewDataset returns an empty dataset. The zero [Dataset] is equally usable;
// this is for the common case of needing a pointer.
func NewDataset() *Dataset {
	return new(Dataset)
}

// Add inserts q into the graph it names, creating that graph if the dataset
// does not already have it. A quad with no graph name is added to the default
// graph. Adding a quad the dataset already holds is a no-op and not an error.
//
// It reports the error from [Quad.Validate] if q violates a position
// constraint, in which case the dataset is unchanged — no empty graph is left
// behind.
func (d *Dataset) Add(q Quad) error {
	if err := q.Validate(); err != nil {
		return err
	}
	if q.Graph == nil {
		return d.def.Add(q.Triple)
	}

	g, err := d.AddGraph(q.Graph)
	if err != nil {
		return err
	}
	return g.Add(q.Triple)
}

// Has reports whether the dataset holds q in the graph q names.
func (d *Dataset) Has(q Quad) bool {
	if q.Graph == nil {
		return d.def.Has(q.Triple)
	}
	g, ok := d.named[q.Graph]
	return ok && g.Has(q.Triple)
}

// Len returns the total number of triples across the default graph and every
// named graph. A triple held by two graphs counts twice.
func (d *Dataset) Len() int {
	n := d.def.Len()
	for _, g := range d.named {
		n += g.Len()
	}
	return n
}

// Default returns the dataset's default graph, the one graph that has no name.
// It is never nil, and writing to it writes to the dataset.
func (d *Dataset) Default() *Graph {
	return &d.def
}

// NamedGraph returns the graph named name and whether the dataset has it.
//
// Names are compared as terms, so an [IRI] never finds a graph named by a
// [BlankNode] with the same label. Use [Dataset.AddGraph] to create a graph
// that does not exist yet.
func (d *Dataset) NamedGraph(name Term) (*Graph, bool) {
	g, ok := d.named[name]
	return g, ok
}

// AddGraph returns the graph named name, adding an empty one to the dataset if
// it does not exist yet. Calling it again with the same name returns the same
// graph, so a dataset can carry a named graph that holds no triples.
//
// It reports [ErrInvalidGraphName] if name is nil — the default graph is
// reached with [Dataset.Default], not by name — or is a [Literal].
func (d *Dataset) AddGraph(name Term) (*Graph, error) {
	if name == nil {
		return nil, fmt.Errorf("%w: got nil", ErrInvalidGraphName)
	}
	if err := validateGraphName(name); err != nil {
		return nil, err
	}

	if g, ok := d.named[name]; ok {
		return g, nil
	}
	if d.named == nil {
		d.named = make(map[Term]*Graph)
	}
	g := NewGraph()
	d.named[name] = g
	d.order = append(d.order, name)
	return g, nil
}

// GraphNames returns an iterator over the names of the dataset's named graphs,
// in the order their graphs were first added. The default graph has no name
// and is not visited.
//
// The iterator observes the dataset as it was when GraphNames was called, so
// graphs added while ranging over it are not visited.
func (d *Dataset) GraphNames() iter.Seq[Term] {
	return slices.Values(d.order)
}

// All returns an iterator over every quad in the dataset: the default graph's
// triples first, with a nil graph name, then each named graph in the order it
// was first added.
//
// The iterator observes the dataset as it was when All was called, so quads
// added while ranging over it are not visited.
func (d *Dataset) All() iter.Seq[Quad] {
	names := d.order
	defaults := d.def.All()

	return func(yield func(Quad) bool) {
		for t := range defaults {
			if !yield(Quad{Triple: t}) {
				return
			}
		}
		for _, name := range names {
			for t := range d.named[name].All() {
				if !yield(Quad{Triple: t, Graph: name}) {
					return
				}
			}
		}
	}
}
