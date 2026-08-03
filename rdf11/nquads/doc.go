// Package nquads implements RDF 1.1 N-Quads, the line-based serialization
// defined at https://www.w3.org/TR/n-quads/.
//
// N-Quads is N-Triples with one addition: a statement may carry the label of
// the graph it belongs to, written after the object, and a statement written
// without one belongs to the dataset's default graph. That single optional
// production is the whole of the difference, which is why the two packages
// look so alike.
//
// It reads in two ways, which share one implementation of the grammar, and
// writes in two ways to match.
//
// [Decode] streams: it yields each statement as a quad of the
// [github.com/z5labs/rdf-go] package the moment its '.' is read, keeping
// nothing, so a document larger than memory can be read.
//
// [Parse] instead builds a [Document], a syntax tree whose every node carries
// the position it was written at and which keeps the comments the grammar
// would otherwise discard. [Quads] lowers such a tree the same way [Decode]
// does, for a caller who wanted both.
//
// [Encode] writes quads back out in canonical form, which settles the spacing,
// the escaping and the absence of comments, so that a ground dataset written
// twice is the same bytes whoever writes it. It does not settle which labels
// blank nodes carry, and so does not make a dataset with blank nodes
// byte-stable; see [Encode]. [Print] writes a [Document] back out instead,
// keeping its comments where they were — which canonical form cannot do, and a
// tool rewriting a file needs.
//
// This package implements RDF 1.1 only. The lexemes RDF 1.2 adds — the triple
// term brackets "<<(" and ")>>", and the base direction suffixes "--ltr" and
// "--rtl" — are rejected here, and belong to the rdf12/nquads package. Both
// produce the shared term types of the rdf package, so a dataset read by one
// can be written by the other.
//
// Those shared term types model RDF 1.2, so a triple term can reach [Encode]
// even though this package's own parser can never produce one. It is reported
// as [ErrTripleTerm] rather than written in a spelling RDF 1.1 N-Quads does
// not have.
package nquads
