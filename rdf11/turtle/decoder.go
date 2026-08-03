package turtle

import (
	"errors"
	"fmt"
	"io"
	"iter"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/iri"
	"github.com/z5labs/rdf-go/vocab"
)

// InvalidTermError is reported when a term the grammar accepts does not
// describe a term of the RDF data model — a literal typed rdf:langString
// without a language tag, say, which Turtle can write but RDF cannot mean.
//
// Pos is the position of the offending term. The underlying error is one of the
// rdf package's, the iri package's, or this package's [ErrNoBase], and can be
// reached with [errors.Is] or [errors.As].
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

// UndefinedPrefixError is reported when a prefixed name names a prefix no
// directive has bound at that point in the document (Turtle §6.3).
//
// Prefix is the label as written, without its colon, and Pos is where the name
// stands — the two things needed to find it, since a document may use the same
// prefix in many places and bind it in none.
type UndefinedPrefixError struct {
	Pos    Pos
	Prefix string
}

// Error implements the [error] interface.
func (e UndefinedPrefixError) Error() string {
	return fmt.Sprintf(
		"undefined prefix %q at line %d, column %d",
		e.Prefix, e.Pos.Line, e.Pos.Column,
	)
}

// ErrNoBase is reported when a relative IRI is written where no base is in
// scope. Every IRI in the RDF abstract syntax must be absolute (RDF 1.1
// Concepts §3.2), and a relative one has nothing to become absolute against
// until a document sets its base.
var ErrNoBase = errors.New("turtle: relative IRI with no base in scope")

// errConsumerStopped unwinds the parser when the caller stops ranging over
// [Decode]. It never reaches the caller: it is the parser being told there is
// no point reading on, not a failure to report.
var errConsumerStopped = errors.New("turtle: consumer stopped")

// Decode reads the Turtle document in r and yields its triples as data model
// triples, one at a time.
//
// This is where a document stops saying what its author wrote and starts saying
// what it means. Prefixed names are expanded against the prefixes in scope,
// relative IRIs are resolved against the base in scope, collections and blank
// node property lists become the triples they stand for, "a" becomes rdf:type,
// and a number or a boolean written as itself takes the datatype its form
// implies.
//
// The triples of a statement are yielded as soon as its '.' is read, and
// nothing about the statement is kept afterwards, so a document is never held
// in memory in full and one larger than memory can be read. What does
// accumulate is the state a directive leaves behind — the base, the prefix map,
// and the blank node scope, which has to remember every distinct blank node
// label so that _:b0 written on the first line and the last means the same node
// both times.
//
// A statement expands to more than one triple whenever it nests, and the
// triples of what is nested come first: the cells of a collection are yielded
// before the triple that uses the collection, and the contents of a blank node
// property list before the triple that uses the node. The order of triples in a
// graph means nothing, so this is a choice rather than a requirement, but it is
// the order the specification's own examples are written in (Turtle §7.3).
//
// Iteration stops at the first error, which is yielded with the zero
// [rdf.Triple] and followed by nothing:
//
//	for triple, err := range turtle.Decode(r) {
//		if err != nil {
//			return err
//		}
//		// ...
//	}
//
// Every error carries the position it came from — a tokenizer error at the
// character, a parse error at the token, an [UndefinedPrefixError] at the
// prefixed name, and an [InvalidTermError] at the term the data model refused.
//
// Comments are dropped. A [Document] from [Parse] keeps them, for a caller who
// wants to write the document back rather than read what it says.
func Decode(r io.Reader) iter.Seq2[rdf.Triple, error] {
	return func(yield func(rdf.Triple, error) bool) {
		d := &decoder{yield: yield}
		d.lowering = newLowering(d.emit)

		err := parse(r, d)
		switch {
		case err == nil, errors.Is(err, errConsumerStopped):
			return
		default:
			yield(rdf.Triple{}, err)
		}
	}
}

// decoder is the [sink] that lowers each statement and hands its triples
// straight on.
type decoder struct {
	lowering *lowering
	yield    func(rdf.Triple, error) bool
}

// statement lowers the statement, yielding whatever triples it expands to.
func (d *decoder) statement(s Statement) error { return d.lowering.statement(s) }

// comment drops the comment. Streaming reports what a document says, and a
// comment says nothing.
//
// Taking the text raw is what makes dropping it free: a document thick with
// comments costs a decoder nothing to read past.
func (d *decoder) comment(Pos, []byte) {}

// emit hands one triple to the caller, reporting errConsumerStopped once the
// caller has stopped listening.
func (d *decoder) emit(triple rdf.Triple) error {
	if !d.yield(triple, nil) {
		return errConsumerStopped
	}
	return nil
}

// Lower turns an already-parsed document into data model triples.
//
// This is [Decode] for a caller who wanted the syntax tree as well — the same
// lowering, over a document already in memory. Blank node labels resolve
// through one scope for the whole document, so the triples it returns can be
// added to a graph alongside each other but not alongside those of another
// document, whose _:b0 is a different node.
//
// It is not called Triples because [Triples] is the syntax tree's node for a
// subject and the things said about it, which this returns none of.
//
// A nil document has no triples and is not an error.
func Lower(doc *Document) ([]rdf.Triple, error) {
	if doc == nil {
		return nil, nil
	}

	var triples []rdf.Triple
	l := newLowering(func(triple rdf.Triple) error {
		triples = append(triples, triple)
		return nil
	})

	for _, statement := range doc.Statements {
		if err := l.statement(statement); err != nil {
			return nil, err
		}
	}
	return triples, nil
}

// lowering turns statements into triples, carrying the state the directives
// leave behind.
//
// A directive applies to what follows it and to nothing before it, so the state
// is read as each statement is lowered rather than gathered up front: a second
// @base changes only the IRIs after it, and a prefix bound twice means the
// second binding only from where it was written.
type lowering struct {
	base     string
	hasBase  bool
	prefixes map[string]string
	scope    *rdf.BlankNodeScope
	emit     func(rdf.Triple) error
}

func newLowering(emit func(rdf.Triple) error) *lowering {
	return &lowering{
		prefixes: make(map[string]string),
		scope:    rdf.NewBlankNodeScope(),
		emit:     emit,
	}
}

// statement lowers one statement of the document.
func (l *lowering) statement(s Statement) error {
	switch st := s.(type) {
	case *PrefixDirective:
		return l.prefixDirective(st)
	case *BaseDirective:
		return l.baseDirective(st)
	case *Triples:
		return l.triples(st)
	default:
		panic(fmt.Sprintf("unknown statement node: %T", s))
	}
}

// prefixDirective binds a prefix from here on.
//
//	prefixID ::= '@prefix' PNAME_NS IRIREF '.'
//
// The namespace is resolved against the base in scope now rather than each time
// the prefix is used, both because that is what the specification says happens
// (Turtle §6.3) and because a later @base would otherwise reach back and change
// what an earlier prefix means.
func (l *lowering) prefixDirective(d *PrefixDirective) error {
	namespace, err := l.resolveIRIRef(d.IRI)
	if err != nil {
		return err
	}
	l.prefixes[d.Prefix] = string(namespace)
	return nil
}

// baseDirective sets the base from here on.
//
//	base ::= '@base' IRIREF '.'
//
// A base may itself be written relative, in which case it is resolved against
// the base it replaces (Turtle §6.3). The first one therefore has to be
// absolute, there being nothing for it to resolve against.
func (l *lowering) baseDirective(d *BaseDirective) error {
	base, err := l.resolveIRIRef(d.IRI)
	if err != nil {
		return err
	}
	l.base = string(base)
	l.hasBase = true
	return nil
}

// triples lowers a subject and everything said about it.
//
//	triples ::= subject predicateObjectList
//	          | blankNodePropertyList predicateObjectList?
func (l *lowering) triples(t *Triples) error {
	subject, err := l.term(t.Subject)
	if err != nil {
		return err
	}
	return l.predicateObjects(subject, t.Predicates)
}

// predicateObjects emits one triple for every object of every verb given for
// subject.
//
//	predicateObjectList ::= verb objectList (';' (verb objectList)?)*
func (l *lowering) predicateObjects(subject rdf.Term, list []*PredicateObject) error {
	for _, po := range list {
		predicate, err := l.verb(po.Verb)
		if err != nil {
			return err
		}

		for _, node := range po.Objects {
			object, err := l.term(node)
			if err != nil {
				return err
			}

			triple := rdf.Triple{Subject: subject, Predicate: predicate, Object: object}
			// The syntax tree already rules out a literal subject and a blank
			// node predicate, and an IRI is only accepted once it has been made
			// absolute, so there is nothing left here for the data model to
			// refuse. It is checked all the same, since a graph will check it
			// on insert and a failure found here can say where it was written.
			if err := triple.Validate(); err != nil {
				return InvalidTermError{Pos: po.Verb.Position(), Err: err}
			}
			if err := l.emit(triple); err != nil {
				return err
			}
		}
	}
	return nil
}

// verb lowers what stands in the predicate position.
//
//	verb ::= predicate | 'a'
func (l *lowering) verb(v Verb) (rdf.IRI, error) {
	switch t := v.(type) {
	case *A:
		// "a" is a keyword and not an IRI, and rdf:type is what it abbreviates
		// (Turtle §6.5).
		return vocab.RDFType, nil
	case *IRIRef:
		return l.resolveIRIRef(t)
	case *PrefixedName:
		return l.expandPrefixedName(t)
	default:
		panic(fmt.Sprintf("unknown verb node: %T", v))
	}
}

// term lowers one term, emitting the triples of anything nested inside it
// first.
func (l *lowering) term(node Term) (rdf.Term, error) {
	switch t := node.(type) {
	case *IRIRef:
		return l.resolveIRIRef(t)
	case *PrefixedName:
		return l.expandPrefixedName(t)
	case *BlankNode:
		// The scope is what keeps this document's labels from colliding with
		// another's, and what makes two mentions of one label the same node.
		return l.scope.Node(t.Label), nil
	case *Anon:
		// [] names a node the document does not, so nothing may refer to it
		// again and it needs no entry in the scope's mapping.
		return l.scope.New(), nil
	case *Collection:
		return l.collection(t)
	case *BlankNodePropertyList:
		return l.blankNodePropertyList(t)
	case *Literal:
		return l.literal(t)
	default:
		panic(fmt.Sprintf("unknown term node: %T", node))
	}
}

// collection expands a list into the cells it stands for, returning the term
// that stands where the list was written.
//
//	collection ::= '(' object* ')'
//
// Each cell is a fresh blank node holding the item in rdf:first and the rest of
// the list in rdf:rest, the last cell pointing at rdf:nil. An empty collection
// is rdf:nil itself and expands to no triples at all (Turtle §7.3).
func (l *lowering) collection(c *Collection) (rdf.Term, error) {
	if len(c.Objects) == 0 {
		return vocab.RDFNil, nil
	}

	head := l.scope.New()
	cell := head
	for i, node := range c.Objects {
		first, err := l.term(node)
		if err != nil {
			return nil, err
		}
		if err := l.emit(rdf.Triple{Subject: cell, Predicate: vocab.RDFFirst, Object: first}); err != nil {
			return nil, err
		}

		var rest rdf.Term = vocab.RDFNil
		next := cell
		if i < len(c.Objects)-1 {
			next = l.scope.New()
			rest = next
		}
		if err := l.emit(rdf.Triple{Subject: cell, Predicate: vocab.RDFRest, Object: rest}); err != nil {
			return nil, err
		}
		cell = next
	}
	return head, nil
}

// blankNodePropertyList expands a node described in place, returning the node
// itself.
//
//	blankNodePropertyList ::= '[' predicateObjectList ']'
//
// The node is minted before its contents are lowered, so a list nested inside
// another is described against its own node and not its parent's (Turtle §7.2).
func (l *lowering) blankNodePropertyList(b *BlankNodePropertyList) (rdf.Term, error) {
	node := l.scope.New()
	if err := l.predicateObjects(node, b.Predicates); err != nil {
		return nil, err
	}
	return node, nil
}

// literal lowers a literal, supplying the datatype the syntax leaves implied.
//
// A quoted string works as it does in N-Triples: a language tag implies
// rdf:langString, an explicit datatype is taken as written, and neither means
// xsd:string. What Turtle adds is the shorthands, each of which carries its
// datatype by being written the way it is (Turtle §2.5.1, §2.5.2):
//
//	true, false  xsd:boolean
//	42           xsd:integer
//	3.14         xsd:decimal
//	1e6          xsd:double
func (l *lowering) literal(node *Literal) (rdf.Literal, error) {
	if datatype, ok := shorthandDatatype(node.Kind); ok {
		literal, err := rdf.NewTypedLiteral(node.Value, datatype)
		if err != nil {
			return rdf.Literal{}, InvalidTermError{Pos: node.Pos, Err: err}
		}
		return literal, nil
	}

	switch {
	case node.Language != "":
		// The only way this fails is an empty tag, which the LANGTAG production
		// cannot produce — it demands a letter. Reported rather than dropped
		// all the same, since a constructor that can fail should not be assumed
		// not to.
		literal, err := rdf.NewLanguageLiteral(node.Value, node.Language)
		if err != nil {
			return rdf.Literal{}, InvalidTermError{Pos: node.Pos, Err: err}
		}
		return literal, nil

	case node.Datatype != nil:
		datatype, err := l.datatype(node.Datatype)
		if err != nil {
			return rdf.Literal{}, err
		}

		literal, err := rdf.NewTypedLiteral(node.Value, datatype)
		if err != nil {
			return rdf.Literal{}, InvalidTermError{Pos: node.Datatype.Position(), Err: err}
		}
		return literal, nil

	default:
		return rdf.NewLiteral(node.Value), nil
	}
}

// shorthandDatatype returns the datatype a literal written as itself implies,
// reporting false for a quoted string, which implies nothing.
func shorthandDatatype(kind LiteralKind) (rdf.IRI, bool) {
	switch kind {
	case LiteralBoolean:
		return vocab.XSDBoolean, true
	case LiteralInteger:
		return vocab.XSDInteger, true
	case LiteralDecimal:
		return vocab.XSDDecimal, true
	case LiteralDouble:
		return vocab.XSDDouble, true
	default:
		return "", false
	}
}

// datatype lowers the IRI after a "^^", which the grammar allows to be written
// out or abbreviated.
//
//	RDFLiteral ::= String (LANGTAG | '^^' iri)?
func (l *lowering) datatype(node Term) (rdf.IRI, error) {
	switch t := node.(type) {
	case *IRIRef:
		return l.resolveIRIRef(t)
	case *PrefixedName:
		return l.expandPrefixedName(t)
	default:
		panic(fmt.Sprintf("unknown datatype node: %T", node))
	}
}

// resolveIRIRef turns an IRI written out in full into a data model IRI,
// resolving it against the base in scope.
//
// Resolution is RFC 3986 §5.2, which leaves an absolute reference as it stands
// save for removing dot segments, so it is applied whether or not the reference
// looks relative. With no base in scope there is nothing to resolve against,
// and a reference that is not already absolute is [ErrNoBase] rather than a
// relative IRI in a graph, which the data model has no room for.
func (l *lowering) resolveIRIRef(node *IRIRef) (rdf.IRI, error) {
	if !l.hasBase {
		if !iri.IsAbsolute(node.Value) {
			return "", InvalidTermError{Pos: node.Pos, Err: ErrNoBase}
		}
		return rdf.IRI(node.Value), nil
	}

	resolved, err := iri.Resolve(l.base, node.Value)
	if err != nil {
		return "", InvalidTermError{Pos: node.Pos, Err: err}
	}
	return rdf.IRI(resolved), nil
}

// expandPrefixedName turns an abbreviated name into the IRI it stands for, by
// appending its local part to the namespace its prefix is bound to (Turtle
// §6.3).
//
// The namespace was resolved when the prefix was bound, so the result is
// absolute and is not resolved again: a local name is not an IRI reference, and
// putting one through the resolver would let a "/" or a ":" in it change what
// the name means.
func (l *lowering) expandPrefixedName(node *PrefixedName) (rdf.IRI, error) {
	namespace, ok := l.prefixes[node.Prefix]
	if !ok {
		return "", UndefinedPrefixError{Pos: node.Pos, Prefix: node.Prefix}
	}
	return rdf.IRI(namespace + node.Local), nil
}
