// Package trig implements RDF 1.2 TriG, the dataset serialization defined at
// https://www.w3.org/TR/rdf12-trig/.
//
// It is a sibling of the rdf11/trig package rather than a replacement for it,
// and the graph-block half of rdf12/turtle. The four read different dialects of
// one grammar and produce the shared term types of the
// [github.com/z5labs/rdf-go] package, so a dataset read by any of them can be
// written by any other.
//
// TriG is Turtle with graph blocks around it. Everything RDF 1.2 Turtle can
// write it can write, meaning the same thing and written the same way, and what
// it adds is three terminals — '{', '}' and the "GRAPH" keyword — with which a
// document says which graph of a dataset a statement belongs to:
//
//	@prefix ex: <http://example.com/> .
//	VERSION "1.2"
//
//	ex:s ex:p ex:o .                    # the default graph
//
//	ex:g {                              # a named graph
//	    ex:s ex:p ex:o ~ ex:r {| ex:q "annotated" |} .
//	    ex:t ex:p <<( ex:s ex:p ex:o )>> .
//	}
//
//	GRAPH ex:h { ex:s ex:p "text"@ar--rtl . }
//
//	{ ex:s ex:q ex:r . }                # the default graph again
//
// A statement written outside every block belongs to the default graph, as do
// the statements of a block that named no graph; the two are one graph, and a
// document may add to it from either place (TriG §2.2). The "GRAPH" keyword is
// optional wherever a label stands and, being written in double quotes in the
// grammar, is case-insensitive as "PREFIX", "BASE" and "VERSION" are. A
// directive is a statement of the document rather than of a graph, so a prefix,
// a base or a version bound anywhere applies from there to the end of the
// document, blocks included.
//
// # The two braces
//
// This is the only one of the four syntaxes where a '{' is two terminals: it
// opens a graph block here, and with a '|' after it an annotation block, which
// RDF 1.2 Turtle already writes. [Tokenize] settles which with one character of
// lookahead and puts back what it did not use, so a graph block whose first
// terminal follows the brace immediately — "{ex:s ex:p ex:o}" — reads exactly as
// one written with a space. Nothing else in the grammar begins with a '|', so
// the lookahead can never take a graph block's contents for the annotation's
// bar, and the two never have to be told apart again further on.
//
// # Reading
//
// Reading is in three layers. [Tokenize] turns a document into the terminals of
// the grammar, and [Parse] turns those into a [Document] — a syntax tree that
// keeps what the author wrote rather than what it means. A prefixed name stays
// a prefix and a local name, the "a" keyword stays itself, a collection stays a
// list rather than becoming the triples it stands for, a reified triple stays
// sugar rather than becoming the rdf:reifies triple it stands for, an
// annotation stays attached to the [Object] it was written after, and a graph
// block keeps the keyword and the label it was written with, so that a printer
// can write the document back as it was.
//
// [Decode] reads what a document means instead, yielding
// [github.com/z5labs/rdf-go.Quad] values: prefixed names expanded, relative
// IRIs resolved against the base, collections and blank node property lists
// expanded into their triples, the shorthands given the datatypes they imply,
// the sugar of a reification spent, and every quad carrying the label of the
// block it was written in. A triple term is the one construct that becomes a
// term rather than quads, and neither it nor a reified triple asserts the
// statement it encloses. Decoding streams, holding no more than the directives
// in scope and the blank node labels seen so far, so a document larger than
// memory can be read.
//
// [Quads] does the same to a [Document] already parsed, for a caller who wanted
// the tree as well, and [Dataset] gathers the quads into an
// [github.com/z5labs/rdf-go.Dataset] — which is the only one of the three that
// can keep a named graph holding nothing, no quad being able to say that a
// graph is empty.
//
// # Writing
//
// Writing mirrors the reading. [Print] writes a [Document] back, abbreviations
// and comments and all, so that [Parse] and [Print] together rewrite a document
// rather than replace it. [Encode] writes a dataset, choosing how to say it:
// the default graph outside every block, each named graph as a block,
// statements about one subject gathered into one, rdf:type written "a", and
// IRIs abbreviated against the prefixes a caller supplies with [WithPrefixes].
// Both lay a document out within a line width, configurable along with the
// indentation by [WithLineWidth] and [WithIndent].
//
// The sugar is [Print]'s alone. [Encode] is given a dataset and not a document,
// so it cannot know whether a reification was written as "<< ... ~ :r >>", as
// an annotation, or as the rdf:reifies triple both stand for; it writes the
// last of those, which says what the dataset says and nothing more. Its
// documentation sets out why.
//
// Everything RDF 1.1 TriG accepts is accepted here unchanged. What this package
// adds is that neither of the two terms the RDF 1.1 encoder has to refuse — a
// triple term, and a literal carrying a base direction — is refused here: RDF
// 1.2 TriG has a syntax for both.
package trig
