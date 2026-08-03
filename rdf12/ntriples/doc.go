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
// Everything RDF 1.1 N-Triples accepts is accepted here unchanged.
package ntriples
