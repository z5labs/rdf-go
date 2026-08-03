// Package turtle implements RDF 1.1 Turtle, the readable member of the family
// defined at https://www.w3.org/TR/turtle/.
//
// So far it reads: [Tokenize] turns a document into the terminals of the
// grammar, and [Parse] turns those into a [Document] — a syntax tree that
// keeps what the author wrote rather than what it means. A prefixed name stays
// a prefix and a local name, the "a" keyword stays itself, and a collection
// stays a list rather than becoming the triples it stands for, so that a
// printer can write the document back as it was. Producing the terms of the
// rdf package from that tree, and printing, are still to come.
//
// Turtle is by far the largest of the four grammars this module reads. What it
// adds to N-Triples is nearly everything that makes a document readable:
// prefixed names, so an IRI need not be written out in full; four ways to
// quote a string, two of which may span lines; numbers written as themselves
// rather than as typed literals; the "a" keyword for rdf:type; blank nodes
// written [] or [ ... ]; collections written ( ... ); and the ';' and ','
// that let a subject or a predicate be given once and used again.
//
// It also takes something away. A line ending is white space here, not a
// terminal — a statement ends at its '.', and may be laid out however its
// author likes.
//
// This package implements RDF 1.1 only. The lexemes RDF 1.2 adds — the triple
// term brackets "<<(" and ")>>", and the base direction suffixes "--ltr" and
// "--rtl" — are rejected here, and belong to the rdf12/turtle package.
package turtle
