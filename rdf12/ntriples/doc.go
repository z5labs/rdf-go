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
// It writes in two ways to match.
//
// [Encode] writes triples back out in the canonical form of §3, which settles
// the spacing, the escaping, the case of a language tag and the absence of
// both comments and a VERSION directive, so that a ground graph written twice
// is the same bytes whoever writes it. It does not settle which labels blank
// nodes carry, and so does not make a graph with blank nodes byte-stable; see
// [Encode]. [Print] writes a [Document] back out instead, keeping its comments
// and its version directives where they were — which canonical form cannot do,
// and a tool rewriting a file needs.
//
// What §3 asks of a quoted string is not what RDF 1.1 asked: seven characters
// now take an ECHAR where four did, so a tab canonical N-Triples once wrote as
// itself is written "\t" here. That is why this package renders a literal
// itself rather than through [github.com/z5labs/rdf-go].Literal, whose String
// method writes the RDF 1.1 form the rdf11 packages need.
//
// Everything RDF 1.1 N-Triples accepts is accepted here unchanged.
package ntriples
