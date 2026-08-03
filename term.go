package rdf

import "strings"

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
type IRI string

func (IRI) isTerm() {}

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
// brackets with the characters disallowed by the IRIREF production escaped.
func (i IRI) String() string {
	var b strings.Builder
	b.Grow(len(i) + 2)
	b.WriteByte('<')
	escapeIRI(&b, string(i))
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
