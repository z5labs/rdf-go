// Package trig implements RDF 1.1 TriG, the dataset serialization defined at
// https://www.w3.org/TR/trig/.
//
// TriG is Turtle with graph blocks around it. Everything Turtle can write it
// can write, meaning the same thing and written the same way, and what it adds
// is three terminals — '{', '}' and the "GRAPH" keyword — with which a
// document says which graph of a dataset a statement belongs to:
//
//	@prefix ex: <http://example.com/> .
//
//	ex:s ex:p ex:o .            # the default graph
//
//	ex:g {                      # a named graph
//	    ex:s ex:p ex:o .
//	}
//
//	GRAPH ex:h { ex:s ex:p ex:o . }
//
//	{ ex:s ex:q ex:r . }        # the default graph again
//
// A statement written outside every block belongs to the default graph, as do
// the statements of a block that named no graph; the two are one graph, and a
// document may add to it from either place (TriG §2.2). The "GRAPH" keyword is
// optional wherever a label stands and, being written in double quotes in the
// grammar, is case-insensitive as "PREFIX" and "BASE" are. A directive is a
// statement of the document rather than of a graph, so a prefix or a base bound
// anywhere applies from there to the end of the document, blocks included.
//
// Reading is in two layers. [Tokenize] turns a document into the terminals of
// the grammar, and [Parse] turns those into a [Document] — a syntax tree that
// keeps what the author wrote rather than what it means. A prefixed name stays
// a prefix and a local name, the "a" keyword stays itself, a collection stays a
// list rather than becoming the triples it stands for, and a graph block keeps
// the keyword and the label it was written with, so that a printer can write
// the document back as it was.
//
// [Decode] reads what a document means instead, yielding [github.com/z5labs/rdf-go.Quad]
// values: prefixed names expanded, relative IRIs resolved against the base,
// collections and blank node property lists expanded into their triples, the
// shorthands given the datatypes they imply, and every quad carrying the label
// of the block it was written in. It streams, holding no more than the
// directives in scope and the blank node labels seen so far, so a document
// larger than memory can be read. [Quads] does the same to a [Document] already
// parsed, for a caller who wanted the tree as well, and [Dataset] gathers the
// quads into an [github.com/z5labs/rdf-go.Dataset] — which is the only one of
// the three that can keep a named graph holding nothing, no quad being able to
// say that a graph is empty.
//
// Writing mirrors the reading. [Print] writes a [Document] back, abbreviations
// and comments and all, so that [Parse] and [Print] together rewrite a document
// rather than replace it. [Encode] writes a dataset, choosing how to say it:
// the default graph outside every block, each named graph as a block, statements
// about one subject gathered into one, rdf:type written "a", and IRIs
// abbreviated against the prefixes a caller supplies with [WithPrefixes]. Both
// lay a document out within a line width, configurable along with the
// indentation by [WithLineWidth] and [WithIndent].
//
// This package implements RDF 1.1 only. The lexemes RDF 1.2 adds — the triple
// term brackets "<<(" and ")>>", and the base direction suffixes "--ltr" and
// "--rtl" — are rejected here, and belong to the rdf12/trig package. Both
// produce the shared term types of the rdf package, so a dataset read by one
// can be written by the other.
//
// Writing goes the same way. Those shared term types model RDF 1.2, so a
// triple term or a literal carrying a base direction can reach [Encode] even
// though this package's own parser can never produce one; both are reported,
// as [ErrTripleTerm] and [ErrBaseDirection], rather than written in a spelling
// RDF 1.1 TriG does not have.
package trig
