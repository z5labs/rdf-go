package nquads

import (
	"errors"
	"fmt"
	"io"
	"iter"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/iri"
)

// InvalidTermError is reported when a term the grammar accepts does not
// describe a term of the RDF data model — a literal typed rdf:langString
// without a language tag, say, which N-Quads can write but RDF cannot mean.
//
// Pos is the position of the offending term. The underlying error is one of
// the rdf package's, and can be reached with [errors.Is] or [errors.As].
type InvalidTermError struct {
	Pos Pos
	Err error
}

// Error implements the [error] interface.
func (e InvalidTermError) Error() string {
	return fmt.Sprintf("invalid term at line %d, column %d: %v", e.Pos.Line, e.Pos.Column, e.Err)
}

// Unwrap returns the underlying error.
func (e InvalidTermError) Unwrap() error { return e.Err }

// ErrRelativeIRI is reported when an IRI is not absolute. Every IRI in the
// RDF abstract syntax must be (RDF 1.1 Concepts §3.2), and neither N-Triples
// nor N-Quads has a base to resolve a relative one against.
var ErrRelativeIRI = errors.New("nquads: IRI must be absolute")

// errConsumerStopped unwinds the parser when the caller stops ranging over
// [Decode]. It never reaches the caller: it is the parser being told there is
// no point reading on, not a failure to report.
var errConsumerStopped = errors.New("nquads: consumer stopped")

// Decode reads the N-Quads document in r and yields its statements as data
// model quads, one at a time.
//
// A statement is yielded as soon as its '.' is read, and nothing is kept
// afterwards, so a document is never held in memory in full and one larger
// than memory can be read. What does accumulate is the blank node scope: every
// distinct blank node label in the document has to be remembered, so that
// _:b0 written on the first line and the last means the same node both times.
// A document of a billion statements costs nothing to hold; one naming a
// billion distinct blank nodes costs a billion labels, and there is no reading
// it correctly for less.
//
// A statement written without a graph label belongs to the dataset's default
// graph, and its quad carries a nil [rdf.Quad.Graph] to say so.
//
// Iteration stops at the first error, which is yielded with the zero
// [rdf.Quad] and followed by nothing:
//
//	for quad, err := range nquads.Decode(r) {
//		if err != nil {
//			return err
//		}
//		// ...
//	}
//
// Every error carries the position it came from — a tokenizer error at the
// character, a parse error at the token, and an [InvalidTermError] at the term
// the data model refused.
//
// Comments are dropped. A [Document] from [Parse] keeps them, for a caller who
// wants to write the document back rather than read what it says.
func Decode(r io.Reader) iter.Seq2[rdf.Quad, error] {
	return func(yield func(rdf.Quad, error) bool) {
		d := &decoder{
			scope: rdf.NewBlankNodeScope(),
			yield: yield,
		}

		err := parse(r, d)
		switch {
		case err == nil, errors.Is(err, errConsumerStopped):
			return
		default:
			yield(rdf.Quad{}, err)
		}
	}
}

// decoder is the [sink] that lowers each statement and hands it straight on.
type decoder struct {
	scope *rdf.BlankNodeScope
	yield func(rdf.Quad, error) bool
}

// statement lowers the parsed statement and yields it.
func (d *decoder) statement(t *Quad) error {
	quad, err := lower(t, d.scope)
	if err != nil {
		return err
	}
	if !d.yield(quad, nil) {
		return errConsumerStopped
	}
	return nil
}

// comment drops the comment. Streaming reports what a document says, and a
// comment says nothing.
//
// Taking the text raw is what makes dropping it free: a document thick with
// comments costs a decoder nothing to read past.
func (d *decoder) comment(Pos, []byte) {}

// Quads lowers an already-parsed document into data model quads.
//
// This is [Decode] for a caller who wanted the syntax tree as well — the same
// lowering, over a document already in memory. Blank node labels resolve
// through one scope for the whole document, so the quads it returns can be
// added to a dataset alongside each other but not alongside those of another
// document, whose _:b0 is a different node.
//
// A nil document has no quads and is not an error.
func Quads(doc *Document) ([]rdf.Quad, error) {
	if doc == nil {
		return nil, nil
	}

	scope := rdf.NewBlankNodeScope()
	quads := make([]rdf.Quad, 0, len(doc.Quads))
	for _, t := range doc.Quads {
		quad, err := lower(t, scope)
		if err != nil {
			return nil, err
		}
		quads = append(quads, quad)
	}
	return quads, nil
}

// lower turns a parsed statement into a data model quad, resolving its blank
// node labels through scope.
func lower(t *Quad, scope *rdf.BlankNodeScope) (rdf.Quad, error) {
	object, err := lowerTerm(t.Object, scope)
	if err != nil {
		return rdf.Quad{}, err
	}

	subject, err := lowerSubject(t.Subject, scope)
	if err != nil {
		return rdf.Quad{}, err
	}

	predicate, err := lowerIRI(t.Predicate)
	if err != nil {
		return rdf.Quad{}, err
	}

	quad := rdf.Quad{
		Triple: rdf.Triple{
			Subject:   subject,
			Predicate: predicate,
			Object:    object,
		},
	}

	// A statement with no label belongs to the default graph, which the data
	// model also writes as a nil graph, so there is nothing to translate.
	if t.Graph != nil {
		if quad.Graph, err = lowerSubject(t.Graph, scope); err != nil {
			return rdf.Quad{}, err
		}
	}

	// The syntax tree already rules out a literal subject, a blank node
	// predicate and a literal graph label, so what is left for the data model
	// to refuse is an empty predicate IRI, which <> writes and no IRI denotes.
	if err := quad.Validate(); err != nil {
		return rdf.Quad{}, InvalidTermError{Pos: t.Predicate.Pos, Err: err}
	}
	return quad, nil
}

// lowerIRI turns a parsed IRI reference into a data model IRI, refusing one
// that is not absolute.
func lowerIRI(node *IRIRef) (rdf.IRI, error) {
	if !iri.IsAbsolute(node.Value) {
		return "", InvalidTermError{Pos: node.Pos, Err: ErrRelativeIRI}
	}
	return rdf.IRI(node.Value), nil
}

// lowerSubject turns a parsed subject into a data model term.
//
// It can fail only on a relative IRI. A literal, the other term the data model
// has something to say about, is not a [SubjectTerm] — the grammar's
// restriction on subjects and graph labels alike,
// which the syntax tree already carries in its types, is what makes this total.
func lowerSubject(subject SubjectTerm, scope *rdf.BlankNodeScope) (rdf.Term, error) {
	switch s := subject.(type) {
	case *IRIRef:
		return lowerIRI(s)
	case *BlankNode:
		return scope.Node(s.Label), nil
	default:
		panic(fmt.Sprintf("unknown subject node: %T", subject))
	}
}

// lowerTerm turns a parsed term into a data model term.
func lowerTerm(term Term, scope *rdf.BlankNodeScope) (rdf.Term, error) {
	switch t := term.(type) {
	case *IRIRef:
		return lowerIRI(t)
	case *BlankNode:
		// The scope is what keeps this document's labels from colliding with
		// another's, and what makes two mentions of one label the same node.
		return scope.Node(t.Label), nil
	case *Literal:
		return lowerLiteral(t)
	default:
		panic(fmt.Sprintf("unknown term node: %T", term))
	}
}

// lowerLiteral turns a parsed literal into a data model literal, supplying the
// datatype the syntax leaves implied.
//
// The three forms the grammar allows map onto the three constructors:
//
//	"plain"                  xsd:string, the datatype of a literal written
//	                         with neither a tag nor a type
//	"tagged"@en              rdf:langString, which a language tag implies
//	"typed"^^<http://ex/dt>  the datatype as written
func lowerLiteral(l *Literal) (rdf.Literal, error) {
	switch {
	case l.Language != "":
		// An empty tag is unreachable from the grammar — LANGTAG demands a
		// letter — but a tag the grammar admits and RFC 5646 does not is
		// reachable: the production allows any run of letters, and
		// well-formedness is stated where RDF terms are built rather than in
		// the grammar. The position is the tag's, not the literal's.
		literal, err := rdf.NewLanguageLiteral(l.Value, l.Language)
		if err != nil {
			return rdf.Literal{}, InvalidTermError{Pos: l.LangPos, Err: err}
		}
		return literal, nil

	case l.Datatype != nil:
		datatype, err := lowerIRI(l.Datatype)
		if err != nil {
			return rdf.Literal{}, err
		}

		literal, err := rdf.NewTypedLiteral(l.Value, datatype)
		if err != nil {
			return rdf.Literal{}, InvalidTermError{Pos: l.Datatype.Pos, Err: err}
		}
		return literal, nil

	default:
		return rdf.NewLiteral(l.Value), nil
	}
}
