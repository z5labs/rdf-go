package rdf

import (
	"fmt"
	"strings"

	"github.com/z5labs/rdf-go/internal/lex"
)

// Well-known namespace IRIs used by the term types themselves.
//
// The vocab package is the general home for well-known IRIs, but it depends on
// the IRI type declared here, so the handful of namespaces this package needs
// to enforce literal invariants are declared locally to avoid an import cycle.
const (
	// NamespaceRDF is the RDF namespace IRI.
	NamespaceRDF = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"

	// NamespaceXSD is the XML Schema datatypes namespace IRI.
	NamespaceXSD = "http://www.w3.org/2001/XMLSchema#"
)

// Datatype IRIs with special meaning to Literal.
const (
	// XSDString is the datatype of a literal carrying neither a language tag
	// nor an explicit datatype.
	XSDString IRI = NamespaceXSD + "string"

	// RDFLangString is the datatype of every literal carrying a language tag
	// but no base direction.
	RDFLangString IRI = NamespaceRDF + "langString"

	// RDFDirLangString is the datatype of every literal carrying both a
	// language tag and a base direction. It was introduced in RDF 1.2.
	RDFDirLangString IRI = NamespaceRDF + "dirLangString"
)

// Term is an RDF term: an [IRI], a [BlankNode], a [Literal] or a
// [TripleTerm].
//
// The interface is sealed by an unexported method, so these four types are the
// only implementations that can ever exist. A type switch over a Term is
// therefore exhaustive, and a default case can be treated as a programming
// error rather than as something a future release might start returning.
//
// [TripleTerm] is the one RDF 1.2 adds, and the one position constraints have
// something to say about: it may only be an object. The other three may stand
// wherever the abstract syntax allows a term of their kind.
//
// Terms are values. A pointer to one satisfies this interface as well — the
// methods take value receivers, so the pointer's method set holds the sealing
// method too — but nothing in this module produces a *IRI, a *BlankNode, a
// *Literal or a *TripleTerm: every constructor returns a value, every position
// on a [Triple] and a [Quad] holds one, and every parser yields one. The type
// switches that read a Term account for the four value types alone. Store and
// pass terms as values.
type Term interface {
	// Equal reports whether t and other are the same RDF term.
	Equal(other Term) bool

	// String renders the term in canonical N-Triples form.
	String() string

	// isTerm seals the interface.
	isTerm()
}

// Compile-time proof that exactly the intended types implement Term.
var (
	_ Term = IRI("")
	_ Term = BlankNode{}
	_ Term = Literal{}
	_ Term = TripleTerm{}
)

// IRI is an Internationalized Resource Identifier, the absolute form of which
// identifies a resource.
//
// IRI is string-backed and deliberately performs no validation on conversion:
// parsers are responsible for rejecting malformed IRIs at their own boundary,
// where they can report a useful position. Resolving a relative reference
// against a base is the job of the iri package.
//
// A Go string can nevertheless hold characters no IRI may — a space, a '<' —
// and such a value has no written form in any RDF syntax. [IRI.Validate] is
// what says so, and every statement-level Validate performs it, so a value
// that cannot be written is refused before anything tries to write it.
type IRI string

func (IRI) isTerm() {}

// Validate reports whether the IRI can be written in RDF syntax, returning
// [ErrInvalidIRI] wrapped with the offending character when it cannot.
//
// Every RDF syntax writes an IRI as an IRIREF, and the production leaves out
// the ASCII control range, the space, and a handful of delimiters:
//
//	IRIREF ::= '<' ([^#x00-#x20<>"{}|^`\] | UCHAR)* '>'
//
// They are left out however they are written. A UCHAR spells a character
// rather than exempting one, so an escape naming one is refused exactly where
// the character itself is — the W3C tests turtle-syntax-bad-uri-escape-01..03
// and their TriG twins are negative syntax tests for that reason, and this
// module's tokenizers refuse both forms. An IRI holding one of these
// characters therefore has no rendering at all, not merely an awkward one.
//
// Nothing is lost by refusing it: RFC 3987 admits none of these characters in
// an IRI either, so only a value already outside the data model can fail here,
// and the way to carry a space in an IRI is %20.
//
// Nothing else about the IRI is checked. Whether it is absolute is the iri
// package's job, and a relative reference is a legitimate thing for an IRI to
// hold while it waits to be resolved.
func (i IRI) Validate() error {
	// Every excluded character is ASCII, and no byte of a multi-byte UTF-8
	// sequence is ever below 0x80, so byte iteration cannot mistake part of
	// one for a delimiter.
	for j := 0; j < len(i); j++ {
		if c := i[j]; !lex.IsIRIChar(rune(c)) {
			return fmt.Errorf("%w: %q at byte %d of %q", ErrInvalidIRI, c, j, string(i))
		}
	}
	return nil
}

// Equal reports whether other is an IRI with the same character sequence.
//
// IRIs are compared by exact codepoint equality. No normalization of case,
// percent-encoding or Unicode form is performed, matching RDF's simple string
// comparison of IRIs.
func (i IRI) Equal(other Term) bool {
	o, ok := other.(IRI)
	return ok && i == o
}

// String renders the IRI in canonical N-Triples form, enclosed in angle
// brackets.
//
// The IRI is written as it stands. Nothing is escaped, because an IRI holds
// nothing that needs escaping: every character IRIREF excludes is one RFC 3987
// excludes as well, so a conforming IRI passes through unchanged.
//
// An IRI that does hold one is still written as it stands, and the result is
// not a document any parser will read back. A UCHAR would not be either — the
// production excludes those characters however they are written, see
// [IRI.Validate] — so there is no rendering that would be, and escaping would
// only make the output look like one. String cannot report an error; it
// renders the value so that it can be read in the error that refused it.
func (i IRI) String() string {
	var b strings.Builder
	b.Grow(len(i) + 2)
	b.WriteByte('<')
	b.WriteString(string(i))
	b.WriteByte('>')
	return b.String()
}

// BlankNode is a term that indicates the existence of a resource without
// identifying it with an IRI or a literal.
//
// The label distinguishes one blank node from another within a scope, but
// carries no meaning of its own: two blank nodes with the same label in
// different documents are different resources. Allocating labels that are
// unique within a scope is the job of the generator in this package's
// scoping support, not of this type.
type BlankNode struct {
	label string
}

// NewBlankNode returns a blank node with the given label.
//
// The label is stored as given and is not validated; a parser is responsible
// for rejecting labels that its syntax disallows.
func NewBlankNode(label string) BlankNode {
	return BlankNode{label: label}
}

// Label returns the blank node's label, without the leading "_:".
func (b BlankNode) Label() string { return b.label }

func (BlankNode) isTerm() {}

// Equal reports whether other is a blank node with the same label.
func (b BlankNode) Equal(other Term) bool {
	o, ok := other.(BlankNode)
	return ok && b.label == o.label
}

// String renders the blank node in canonical N-Triples form as "_:" followed
// by its label.
func (b BlankNode) String() string { return "_:" + b.label }
