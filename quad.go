package rdf

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidGraphName is reported when a graph name is neither an IRI nor a
// blank node. It is wrapped with context, so test with [errors.Is].
var ErrInvalidGraphName = errors.New("rdf: graph name must be an IRI or a blank node")

// Quad is a triple paired with the name of the graph it belongs to.
//
// RDF 1.1 Concepts §4 gives every graph in a dataset either a name — an IRI or
// a blank node — or no name at all, the latter being the dataset's single
// default graph. A nil Graph is that default graph, not a missing value.
type Quad struct {
	Triple

	// Graph names the graph holding the triple: an [IRI] or a [BlankNode], or
	// nil for the dataset's default graph.
	Graph Term
}

// Validate reports whether the quad satisfies the position constraints of the
// RDF abstract syntax: those of [Triple.Validate], plus a graph name that is
// an IRI, a blank node, or nil for the default graph.
func (q Quad) Validate() error {
	if err := q.Triple.Validate(); err != nil {
		return err
	}
	return validateGraphName(q.Graph)
}

// Equal reports whether q and other are the same statement in the same graph.
func (q Quad) Equal(other Quad) bool {
	return q.Triple.Equal(other.Triple) && termEqual(q.Graph, other.Graph)
}

// String renders the quad in canonical N-Quads form, implementing the
// statement production of the N-Quads grammar:
//
//	statement ::= subject predicate object graphLabel? '.'
//
// The graph label is omitted for a quad in the default graph, which is what
// leaving it out means:
//
//	<http://example.com/s> <http://example.com/p> "o" <http://example.com/g> .
//	<http://example.com/s> <http://example.com/p> "o" .
func (q Quad) String() string {
	var b strings.Builder
	q.Triple.writeTo(&b)
	if q.Graph != nil {
		b.WriteByte(' ')
		writeTerm(&b, q.Graph)
	}
	b.WriteString(" .")
	return b.String()
}

// validateGraphName reports whether name may label a graph. A nil name is the
// default graph and is accepted here; [Dataset.AddGraph], which names a graph
// rather than referring to one, rejects it.
func validateGraphName(name Term) error {
	switch name.(type) {
	case nil, IRI, BlankNode:
		return nil
	default:
		return fmt.Errorf("%w: got %T", ErrInvalidGraphName, name)
	}
}
