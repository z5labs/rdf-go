// Package ntriples implements RDF 1.2 N-Triples, the line-based serialization
// defined at https://www.w3.org/TR/rdf12-n-triples/.
//
// It is a sibling of the rdf11/ntriples package rather than a replacement for
// it. The two read different dialects of the same grammar and produce the
// shared term types of the [github.com/z5labs/rdf-go] package, so a graph read
// by one can be written by the other.
//
// [Tokenize] reads a document and yields its terminals. What RDF 1.2 adds to
// the RDF 1.1 grammar is three lexemes:
//
//   - "<<(" and ")>>", which bracket a triple term — a triple standing as the
//     object of another triple.
//   - LANG_DIR, which extends LANGTAG with an initial base direction written
//     after a second hyphen: "en--ltr", "ar--rtl".
//   - the VERSION keyword, which introduces the directive announcing the RDF
//     version a document targets.
//
// It reads in two ways, which share one implementation of the grammar.
//
// [Decode] streams: it yields each triple as an [github.com/z5labs/rdf-go].Triple
// the moment its '.' is read, keeping nothing, so a document larger than
// memory can be read. A triple term becomes an
// [github.com/z5labs/rdf-go].TripleTerm, and a literal written with a base
// direction becomes one typed rdf:dirLangString.
//
// [Parse] instead builds a [Document], a syntax tree whose every node carries
// the position it was written at and which keeps what the grammar treats as
// white space — the comments — along with the version directives a decoder has
// no use for. [Triples] lowers such a tree the same way [Decode] does, for a
// caller who wanted both.
//
// Everything RDF 1.1 N-Triples accepts is accepted here unchanged.
package ntriples
