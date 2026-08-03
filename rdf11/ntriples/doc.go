// Package ntriples implements RDF 1.1 N-Triples, the line-based serialization
// defined at https://www.w3.org/TR/n-triples/.
//
// It reads in two ways, which share one implementation of the grammar, and
// writes in two ways to match.
//
// [Decode] streams: it yields each statement as a triple of the
// [github.com/z5labs/rdf-go] package the moment its '.' is read, keeping
// nothing, so a document larger than memory can be read. That is what to
// reach for to load a dump.
//
// [Parse] instead builds a [Document], a syntax tree whose every node carries
// the position it was written at and which keeps the comments the grammar
// would otherwise discard — what a tool reporting errors or writing the
// document back needs. [Triples] lowers such a tree the same way [Decode]
// does, for a caller who wanted both.
//
// [Encode] writes triples back out in the canonical form of §4, which settles
// the spacing, the escaping and the absence of comments, so that a ground
// graph written twice is the same bytes whoever writes it. It does not settle
// which labels blank nodes carry, and so does not make a graph with blank
// nodes byte-stable; see [Encode]. [Print]
// writes a [Document] back out instead, keeping its comments where they were —
// which canonical form cannot do, and a tool rewriting a file needs.
//
// N-Triples is the smallest complete grammar in the Turtle family: one triple
// per line, every term written out in full, no prefixes and no abbreviations.
// Its terminals — IRIREF, BLANK_NODE_LABEL, STRING_LITERAL_QUOTE, LANGTAG and
// the escape forms UCHAR and ECHAR — are shared with N-Quads, Turtle and TriG.
//
// This package implements RDF 1.1 only. The lexemes RDF 1.2 adds — the triple
// term brackets "<<(" and ")>>", and the base direction suffixes "--ltr" and
// "--rtl" — are rejected here, and belong to the rdf12/ntriples package. Both
// produce the shared term types of the rdf package, so a graph read by one can
// be written by the other.
//
// Those shared term types model RDF 1.2, so a triple term can reach [Encode]
// even though this package's own parser can never produce one. It is reported
// as [ErrTripleTerm] rather than written in a spelling RDF 1.1 N-Triples does
// not have.
package ntriples
