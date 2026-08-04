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
// [Parse] reads a document into a syntax tree, and [Decode] reads it straight
// into the triples it means, one at a time. The tree keeps what the author
// wrote — a reified triple stays sugar rather than becoming the rdf:reifies
// triple it stands for, and an annotation stays attached to the [Object] it
// was written after — and the decoder is where the sugar is spent: a triple
// term becomes an [github.com/z5labs/rdf-go.TripleTerm], and a reified triple
// becomes one triple relating its reifier to that term. Neither asserts the
// statement it encloses.
//
// An annotation is the other way about. Writing ":s :p :o {| :q :v |}" asserts
// ":s :p :o" and then describes it, by reifying it under the identifier the
// '~' named or a blank node minted for the block and saying the block's
// contents about that identifier.
//
// Writing mirrors the two. [Print] writes a [Document] back, abbreviations and
// comments and all, so that [Parse] and [Print] together rewrite a document
// rather than replace it — a reified triple comes back as a reified triple, an
// annotation stays on the object it was written after, and a version directive
// keeps its place among the statements. [Encode] writes data model triples,
// choosing how to say them: statements about one subject gathered into one,
// rdf:type written "a", and IRIs abbreviated against the prefixes a caller
// supplies with [WithPrefixes]. Both lay a document out within a line width,
// configurable along with the indentation by [WithLineWidth] and [WithIndent].
//
// The sugar is [Print]'s alone. [Encode] is given triples and not a document,
// so it cannot know whether a reification was written as "<< ... ~ :r >>", as
// an annotation, or as the rdf:reifies triple both stand for; it writes the
// last of those, which says what the graph says and nothing more. Its
// documentation sets out why.
//
// Everything RDF 1.1 Turtle accepts is accepted here unchanged, and everything
// the RDF 1.1 printer writes is written here unchanged. What this package adds
// is that neither of the two terms the RDF 1.1 encoder has to refuse — a triple
// term, and a literal carrying a base direction — is refused here: RDF 1.2
// Turtle has a syntax for both.
package turtle
