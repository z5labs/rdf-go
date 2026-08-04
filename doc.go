// Package rdf provides types and utilities for working with RDF 1.1 and
// RDF 1.2 data, along with readers and writers for the Turtle family of
// concrete syntaxes: N-Triples, N-Quads, Turtle and TriG.
//
// Each syntax has a package of its own, one per version of the specification,
// under rdf11 and rdf12. A caller handed a document it did not write picks
// among them with the [github.com/z5labs/rdf-go/format] package, which maps a
// media type or a file extension to the decoder that reads it.
package rdf
