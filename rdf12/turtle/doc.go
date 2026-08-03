// Package turtle implements RDF 1.2 Turtle, the readable member of the family
// defined at https://www.w3.org/TR/rdf12-turtle/.
//
// It is a sibling of the rdf11/turtle package rather than a replacement for
// it. The two read different dialects of the same grammar and produce the
// shared term types of the [github.com/z5labs/rdf-go] package, so a graph read
// by one can be written by the other.
//
// [Tokenize] reads a document and yields its terminals. What RDF 1.2 adds to
// the RDF 1.1 grammar is a handful of lexemes, and all of them are delimiters
// that begin the way something older does:
//
//   - "<<" and ">>", which bracket a reified triple — a triple written where a
//     term is wanted, so that a document can say something about it.
//   - "<<(" and ")>>", which bracket a triple term — a triple standing as the
//     object of another triple.
//   - "~", which introduces the identifier a reified triple or an annotation
//     is given.
//   - "{|" and "|}", which bracket an annotation block: statements about the
//     triple just written.
//   - LANG_DIR, which extends LANGTAG with an initial base direction written
//     after a second hyphen: "en--ltr", "ar--rtl".
//   - the VERSION keyword and the "@version" directive, which announce the RDF
//     version a document targets.
//
// The overlaps are what makes the tokenizer's job here harder than in the
// other three syntaxes. A '<' opens an IRIREF, a reified triple or a triple
// term, and which is settled only by the two characters after it; a ')' closes
// a collection or a triple term; and a '{' opens an annotation block here and
// a graph block in TriG. Each is read with lookahead that puts back what it
// did not use, so a longer delimiter that fails to match leaves the input
// exactly where the shorter one needs it.
//
// Everything RDF 1.1 Turtle accepts is accepted here unchanged.
package turtle
