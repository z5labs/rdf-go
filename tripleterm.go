package rdf

import "strings"

// TripleTerm is a triple used as a term: the fourth kind of RDF term, added by
// RDF 1.2 so that a statement can be made about a statement.
//
// Its positions are those of [Triple], with one restriction more. RDF 1.2
// Concepts confines a triple term to the object position, and the grammars
// that spell it say the same:
//
//	tripleTerm ::= '<<(' ttSubject predicate ttObject ')>>'
//	ttSubject  ::= iri | BlankNode
//	ttObject   ::= iri | BlankNode | literal | tripleTerm
//
// So a triple term's own subject is an IRI or a blank node and never another
// triple term, while its object may be one — triple terms nest, to any depth,
// on the object side alone. [TripleTerm.Validate] enforces both, and
// [Triple.Validate] runs it for an object that is a triple term, so nothing a
// [Graph] or a [Dataset] accepts carries a malformed one.
//
// A triple term is not an assertion. Writing
//
//	<s> <p> <<( <a> <b> <c> )>> .
//
// says something about the statement <a> <b> <c>; it does not say that the
// statement holds. That is what separates a triple term from the triple it is
// built from.
//
// A TripleTerm is comparable, but == is finer than RDF term equality for the
// same reason it is on [Triple]: it does not fold the case of a nested
// literal's language tag. Compare with [TripleTerm.Equal].
//
// # Spec versions
//
// The type lives in this package, which models RDF 1.2, rather than under
// rdf12, so that a graph built by any parser can be handed to any printer. The
// rdf11 packages never produce one — their tokenizers reject the "<<(" and
// ")>>" brackets — and their printers refuse to write one rather than invent a
// spelling RDF 1.1 does not have. Only the rdf12 packages read and write them.
//
// # Blank nodes
//
// [Isomorphic] does not look inside a triple term: a blank node nested in one
// is compared by its label rather than mapped by the bijection. Two graphs
// that differ only in the label of such a blank node are reported as not
// isomorphic. Blank nodes in the subject and object positions of a statement
// are mapped as always.
type TripleTerm struct {
	// Subject is the resource the enclosed statement is about: an [IRI] or a
	// [BlankNode]. A [Literal] or another TripleTerm here is rejected by
	// [TripleTerm.Validate].
	Subject Term

	// Predicate names the relation between Subject and Object. As on [Triple],
	// whether it is an absolute IRI is not checked here.
	Predicate IRI

	// Object is the value of the relation, and may itself be a TripleTerm.
	Object Term
}

func (TripleTerm) isTerm() {}

// Validate reports whether the enclosed statement satisfies the position
// constraints of the RDF abstract syntax, returning [ErrInvalidSubject],
// [ErrInvalidPredicate] or [ErrInvalidObject] wrapped with the offending
// value.
//
// The check is recursive: an object that is itself a triple term is validated
// too, so a nested subject holding a triple term is reported however deep it
// is.
func (t TripleTerm) Validate() error {
	return validateStatement(t.Subject, t.Predicate, t.Object)
}

// Equal reports whether other is a triple term enclosing the same statement,
// comparing each position with the term equality of [Term.Equal].
//
// Comparison is structural and recursive: nested triple terms are compared the
// same way, position by position, all the way down.
func (t TripleTerm) Equal(other Term) bool {
	o, ok := other.(TripleTerm)
	if !ok {
		return false
	}
	return t.Predicate == o.Predicate &&
		termEqual(t.Subject, o.Subject) &&
		termEqual(t.Object, o.Object)
}

// String renders the triple term in canonical N-Triples form, implementing the
// tripleTerm production:
//
//	<<( <http://example.com/s> <http://example.com/p> "o" )>>
//
// A position left empty in a term that has not been validated renders as <nil>
// rather than panicking, so String stays usable in error and test output.
func (t TripleTerm) String() string {
	var b strings.Builder
	b.WriteString("<<( ")
	writeTerm(&b, t.Subject)
	b.WriteByte(' ')
	b.WriteString(t.Predicate.String())
	b.WriteByte(' ')
	writeTerm(&b, t.Object)
	b.WriteString(" )>>")
	return b.String()
}
