package rdf

import (
	"errors"
	"fmt"
	"strings"
)

// Errors reported when a statement would violate a position constraint of the
// RDF abstract syntax. An error naming a term that could have been anything is
// wrapped with the term that was actually there, so test with [errors.Is]
// rather than ==.
var (
	// ErrInvalidSubject is reported when a subject is neither an IRI nor a
	// blank node.
	ErrInvalidSubject = errors.New("rdf: subject must be an IRI or a blank node")

	// ErrInvalidPredicate is reported when a predicate is the empty IRI.
	ErrInvalidPredicate = errors.New("rdf: predicate must not be the empty IRI")

	// ErrInvalidObject is reported when a statement has no object.
	ErrInvalidObject = errors.New("rdf: object must not be nil")

	// ErrInvalidIRI is reported when an IRI holds a character the IRIREF
	// production excludes, and so has no form any RDF syntax can write. See
	// [IRI.Validate].
	ErrInvalidIRI = errors.New("rdf: IRI holds a character no RDF syntax can write")
)

// Triple is an RDF statement: a subject, a predicate and an object.
//
// The positions are constrained by the abstract syntax (RDF 1.1 Concepts
// §3.1): the subject is an IRI or a blank node, the predicate is an IRI, and
// the object is any term. Only the predicate's constraint can be expressed in
// a field type; the other two are checked by [Triple.Validate], which every
// insertion into a [Graph] or a [Dataset] performs.
//
// A Triple is comparable — the four term types are — but == is finer than RDF
// term equality, which folds the case of language tags. Compare with
// [Triple.Equal].
type Triple struct {
	// Subject is the resource the statement is about: an [IRI] or a
	// [BlankNode]. A [Literal] here is rejected on insert.
	Subject Term

	// Predicate names the relation between Subject and Object. Whether it is
	// an absolute IRI is not checked here; a parser rejects malformed IRIs at
	// its own boundary, where it can report a position.
	Predicate IRI

	// Object is the value of the relation. Every term is valid in this
	// position, including a [TripleTerm], which is valid in no other.
	Object Term
}

// Validate reports whether the triple satisfies the position constraints of
// the RDF abstract syntax, returning [ErrInvalidSubject],
// [ErrInvalidPredicate] or [ErrInvalidObject] wrapped with the offending
// value.
//
// Every IRI the statement carries — the subject, the predicate, the object,
// and a literal object's datatype — is checked by [IRI.Validate] as well, so a
// statement holding an IRI no RDF syntax can write is refused here rather than
// written out as a document nothing can read back. That check is why
// [ErrInvalidIRI] can come out of a statement-level Validate.
//
// An object that is a [TripleTerm] is validated in turn, so a triple term
// nested anywhere in the object is checked to the bottom.
//
// Nothing further is checked. A predicate that is non-empty but not an
// absolute IRI passes, because deciding that is the job of the iri package and
// of the parsers that read concrete syntax.
func (t Triple) Validate() error {
	return validateStatement(t.Subject, t.Predicate, t.Object)
}

// validateStatement checks the position constraints shared by [Triple] and
// [TripleTerm], which are the same ones: a subject that is an IRI or a blank
// node, a non-empty predicate, and an object that is present.
//
// A [TripleTerm] fails the subject check by falling to the default case, which
// is the rule that keeps triple terms to the object position. An object that
// is one recurses, so the rule holds however deep the nesting goes.
func validateStatement(subject Term, predicate IRI, object Term) error {
	switch subject.(type) {
	case IRI, BlankNode:
	default:
		return fmt.Errorf("%w: got %s", ErrInvalidSubject, describeTerm(subject))
	}
	if predicate == "" {
		return ErrInvalidPredicate
	}
	if object == nil {
		return ErrInvalidObject
	}
	if err := validateIRIs(subject); err != nil {
		return err
	}
	if err := predicate.Validate(); err != nil {
		return err
	}
	if nested, ok := object.(TripleTerm); ok {
		return nested.Validate()
	}
	return validateIRIs(object)
}

// validateIRIs checks the IRIs a term carries, which is the term itself for an
// [IRI] and the datatype for a [Literal]. A [BlankNode] carries none, and a
// [TripleTerm] is validated as a statement rather than here.
//
// This is what keeps a statement that cannot be written from reaching a
// printer: an IRI holding a character IRIREF excludes has no rendering, and a
// literal's datatype is written as an IRIREF like any other IRI.
func validateIRIs(t Term) error {
	switch term := t.(type) {
	case IRI:
		return term.Validate()
	case Literal:
		return term.Datatype().Validate()
	default:
		return nil
	}
}

// Equal reports whether t and other are the same RDF statement, comparing each
// position with the term equality of [Term.Equal].
func (t Triple) Equal(other Triple) bool {
	return t.Predicate == other.Predicate &&
		termEqual(t.Subject, other.Subject) &&
		termEqual(t.Object, other.Object)
}

// String renders the triple in canonical N-Triples form, implementing the
// triple production of the N-Triples grammar:
//
//	triple ::= subject predicate object '.'
//
// For example:
//
//	<http://example.com/s> <http://example.com/p> "o" .
//
// A position left empty in a statement that has not been validated renders as
// <nil> rather than panicking, so String stays usable in error and test
// output.
func (t Triple) String() string {
	var b strings.Builder
	t.writeTo(&b)
	b.WriteString(" .")
	return b.String()
}

// writeTo writes the subject, predicate and object to b, separated by spaces
// and without the statement's trailing '.', which N-Quads places after the
// graph label.
func (t Triple) writeTo(b *strings.Builder) {
	writeTerm(b, t.Subject)
	b.WriteByte(' ')
	b.WriteString(t.Predicate.String())
	b.WriteByte(' ')
	writeTerm(b, t.Object)
}

// termEqual reports whether a and b are the same term, treating the nil that
// only an unvalidated statement can carry as equal to itself alone.
func termEqual(a, b Term) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(b)
}

// writeTerm writes t to b in canonical N-Triples form, or as <nil> if the
// position is empty.
func writeTerm(b *strings.Builder, t Term) {
	if t == nil {
		b.WriteString("<nil>")
		return
	}
	b.WriteString(t.String())
}

// describeTerm renders t for an error message as its kind followed by its
// canonical form, which together say both what was rejected and why.
func describeTerm(t Term) string {
	if t == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%T %s", t, t)
}
