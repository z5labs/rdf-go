// Package nquads implements RDF 1.2 N-Quads, the line-based serialization
// defined at https://www.w3.org/TR/rdf12-n-quads/.
//
// N-Quads is N-Triples with one addition: a statement may carry the label of
// the graph it belongs to, written after the object, and a statement written
// without one belongs to the dataset's default graph. That single optional
// production is the whole of the difference, which is why this package and
// rdf12/ntriples look so alike.
//
// It is a sibling of the rdf11/nquads package rather than a replacement for
// it. The two read different dialects of the same grammar and produce the
// shared term types of the [github.com/z5labs/rdf-go] package, so a dataset
// read by one can be written by the other.
//
// [Tokenize] reads a document and yields its terminals. What RDF 1.2 adds to
// the RDF 1.1 grammar is three lexemes:
//
//   - "<<(" and ")>>", which bracket a triple term — a triple standing as the
//     object of another statement.
//   - LANG_DIR, which extends LANGTAG with an initial base direction written
//     after a second hyphen: "en--ltr", "ar--rtl".
//   - the VERSION keyword, which introduces the directive announcing the RDF
//     version a document targets.
//
// None of the three reaches the graph label. graphLabel is the RDF 1.1
// production unchanged, admitting an IRIREF and a blank node label and nothing
// else, so a triple term is a term of a statement and never the name of a
// graph.
//
// It reads in two ways, which share one implementation of the grammar.
//
// [Decode] streams: it yields each statement as an
// [github.com/z5labs/rdf-go].Quad the moment its '.' is read, keeping nothing,
// so a document larger than memory can be read. A triple term becomes an
// [github.com/z5labs/rdf-go].TripleTerm, a literal written with a base
// direction becomes one typed rdf:dirLangString, and a statement written with
// no graph label becomes a quad whose graph is nil, which is the default
// graph.
//
// [Parse] instead builds a [Document], a syntax tree whose every node carries
// the position it was written at and which keeps what the grammar treats as
// white space — the comments — along with the version directives a decoder has
// no use for. [Quads] lowers such a tree the same way [Decode] does, for a
// caller who wanted both.
//
// It writes in two ways to match.
//
// [Encode] writes quads back out in canonical form, which settles the spacing,
// the escaping, the case of a language tag and the absence of both comments
// and a VERSION directive, so that a ground dataset written twice is the same
// bytes whoever writes it. It does not settle which labels blank nodes carry,
// and so does not make a dataset with blank nodes byte-stable; see [Encode],
// which also says where the canonical form of N-Quads comes from, the
// specification defining none of its own. [Print] writes a [Document] back out
// instead, keeping its comments and its version directives where they were —
// which canonical form cannot do, and a tool rewriting a file needs.
//
// What RDF 1.2 asks of a quoted string is not what RDF 1.1 asked: seven
// characters now take an ECHAR where four did, so a tab canonical N-Quads once
// wrote as itself is written "\t" here. That is why this package renders a
// literal itself rather than through [github.com/z5labs/rdf-go].Literal, whose
// String method writes the RDF 1.1 form the rdf11 packages need.
//
// Everything RDF 1.1 N-Quads accepts is accepted here unchanged.
package nquads
