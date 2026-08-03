// Package lex holds the lexical rules that the RDF concrete syntaxes share.
//
// N-Triples, N-Quads, Turtle and TriG are four grammars written over one set
// of characters. The same PN_CHARS classes name their blank node labels and
// prefixed names, and the same three escape forms — ECHAR, UCHAR and PERCENT —
// stand wherever a character cannot be written directly. What differs between
// them is the productions built from those pieces, so the productions are what
// each package writes for itself and the pieces are what they all take from
// here.
//
// Only rules that RDF 1.1 and RDF 1.2 agree on belong in this package.
// Anything the two versions spell differently stays in the package that
// implements the version: the point of sharing these is that the RDF 1.2
// packages duplicate grammar rules rather than character tables.
//
// Nothing here reads input or tracks a position. A tokenizer owns its reader,
// the line and column of every character it has read, and the error types its
// own package documents, so the functions that consume characters take a
// [NextFunc] and report what went wrong without saying where — leaving the
// caller, which knows, to attach the position.
package lex
