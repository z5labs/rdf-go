package rdf

import (
	"iter"
	"slices"
	"strings"
)

// Graph is a set of RDF triples.
//
// Adding a triple the graph already holds is a no-op. Membership is RDF term
// equality rather than ==, so two triples whose literals differ only in the
// case of a language tag are the same triple (see [Literal]); the form added
// first is the form the graph keeps.
//
// Iteration order is insertion order — the order in which triples were first
// added. That is a guarantee rather than an accident of the implementation:
// printers and tests may rely on it, so a document read into a graph and
// written back out keeps the order it was written in.
//
// The zero Graph is an empty graph, ready to use. A Graph must not be copied
// after its first insertion — the copy would share the membership index but
// not the insertion order — so pass a *Graph.
type Graph struct {
	index map[tripleKey]struct{}
	order []Triple
}

// NewGraph returns an empty graph. The zero [Graph] is equally usable; this is
// for the common case of needing a pointer.
func NewGraph() *Graph {
	return new(Graph)
}

// Add inserts t into the graph. Adding a triple the graph already holds is a
// no-op and not an error.
//
// It reports the error from [Triple.Validate] if t violates a position
// constraint, in which case the graph is unchanged.
func (g *Graph) Add(t Triple) error {
	if err := t.Validate(); err != nil {
		return err
	}

	k := keyOf(t)
	if _, ok := g.index[k]; ok {
		return nil
	}
	if g.index == nil {
		g.index = make(map[tripleKey]struct{})
	}
	g.index[k] = struct{}{}
	g.order = append(g.order, t)
	return nil
}

// Has reports whether the graph holds t. An invalid triple is never held, so
// Has reports false for it rather than an error.
func (g *Graph) Has(t Triple) bool {
	_, ok := g.index[keyOf(t)]
	return ok
}

// Len returns the number of distinct triples in the graph.
func (g *Graph) Len() int {
	return len(g.order)
}

// All returns an iterator over the graph's triples in insertion order.
//
// The iterator observes the graph as it was when All was called, so triples
// added while ranging over it are not visited.
func (g *Graph) All() iter.Seq[Triple] {
	return slices.Values(g.order)
}

// tripleKey is the set-membership key of a triple.
//
// A [Triple] is comparable and could be a map key as it stands, but == on one
// is finer than [Triple.Equal] in two places: a literal's language tag
// compares case-insensitively, and the zero [Literal] reports the xsd:string
// datatype it does not store. Folding both out of the terms first makes == on
// a key agree with term equality.
type tripleKey struct {
	subject   Term
	predicate IRI
	object    Term
}

// keyOf returns the set-membership key of t.
func keyOf(t Triple) tripleKey {
	return tripleKey{
		subject:   foldTerm(t.Subject),
		predicate: t.Predicate,
		object:    foldTerm(t.Object),
	}
}

// foldTerm returns t with the differences RDF term equality ignores removed: a
// literal gets its implied datatype written down and its language tag
// lowercased, and every other term is returned unchanged.
//
// Lowercasing agrees with the [strings.EqualFold] comparison [Literal.Equal]
// performs because a language tag is ASCII (BCP 47 §2.1).
func foldTerm(t Term) Term {
	l, ok := t.(Literal)
	if !ok {
		return t
	}
	l.datatype = l.Datatype()
	l.language = strings.ToLower(l.language)
	return l
}
