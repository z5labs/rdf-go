// Package ntriples implements RDF 1.1 N-Triples, the line-based serialization
// defined at https://www.w3.org/TR/n-triples/.
//
// So far it lexes: [Tokenize] turns a document into the terminals of the
// grammar, each with the position it was written at. Parsing and printing are
// still to come.
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
package ntriples
