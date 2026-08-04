// This file began as a copy of rdf12/turtle/decoder.go and keeps its shape: the
// same lowering struct carrying the base, the prefixes and the blank node
// scope, and the same rules for everything RDF 1.2 Turtle can write — the
// triple term that lowers to an [rdf.TripleTerm], the reified triple that
// lowers to its reifier and states one rdf:reifies triple, and the annotation
// that states one per reifier and says the block's contents about it.
//
// What TriG adds is the fourth position. Every triple those rules produce is
// emitted into the graph being lowered into, which the lowering carries in one
// field and a graph block sets while its triples are read. That is the whole of
// the delta: a triple term is a term wherever it is written, and an annotation
// reifies within its graph rather than across the dataset.

package trig

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
// without a language tag, say, or one whose base direction is neither "ltr" nor
// "rtl", both of which TriG can write and RDF cannot mean.
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
// scope. Every IRI in the RDF abstract syntax must be absolute (RDF 1.2
// Concepts §3.2), and a relative one has nothing to become absolute against
// until a document sets its base.
var ErrNoBase = errors.New("trig: relative IRI with no base in scope")

// errConsumerStopped unwinds the parser when the caller stops ranging over
// [Decode]. It never reaches the caller: it is the parser being told there is
// no point reading on, not a failure to report.
var errConsumerStopped = errors.New("trig: consumer stopped")

// Decode reads the RDF 1.2 TriG document in r and yields its statements as data
// model quads, one at a time.
//
// This is where a document stops saying what its author wrote and starts saying
// what it means. Prefixed names are expanded against the prefixes in scope,
// relative IRIs are resolved against the base in scope, collections and blank
// node property lists become the triples they stand for, "a" becomes rdf:type,
// a number or a boolean written as itself takes the datatype its form implies,
// a reified triple becomes the rdf:reifies triple it is sugar for, and an
// annotation becomes the reification of the triple it was written on together
// with what the annotation block said about it.
//
// A triple term is the one construct that becomes a term rather than triples:
// it yields an [rdf.TripleTerm], nested to whatever depth the document wrote.
// Writing one says something about the enclosed statement without asserting it,
// so the enclosed statement is not among the quads yielded — nor is it when a
// reified triple encloses it (RDF 1.2 Turtle §2.11).
//
// What TriG adds is the fourth position. A quad read inside a graph block
// carries that block's label, and one read outside any block — or inside a
// block that named no graph — carries none, which is how the default graph is
// spelled (TriG §2.2). Everything a statement expands to belongs to the same
// graph the statement was written in, the cells of a collection and the
// rdf:reifies triples of a reification included. A prefix, a base or a version
// announced anywhere applies from there to the end of the document, graph
// blocks included, since a directive is a statement of the document and not of
// a graph.
//
// The quads of a statement are yielded as soon as its '.' is read, and nothing
// about the statement is kept afterwards, so a document is never held in memory
// in full and one larger than memory can be read. What does accumulate is the
// state a directive leaves behind — the base, the prefix map, and the blank node
// scope, which has to remember every distinct blank node label so that _:b0
// written on the first line and the last means the same node both times, in
// whichever graphs they were written.
//
// A statement expands to more than one quad whenever it nests, and the quads of
// what is nested come first: the cells of a collection are yielded before the
// quad that uses the collection, the contents of a blank node property list
// before the quad that uses the node, and the rdf:reifies triple of a reified
// triple before the quad that uses its reifier. The order of triples in a graph
// means nothing, so this is a choice rather than a requirement, but it is the
// order the specification's own examples are written in (Turtle §7.3).
//
// An annotation is the one thing that comes after rather than before, since a
// reifier is written after the triple it describes and cannot be built until
// that triple is: ":s :p :o {| :q :v |}" yields ":s :p :o" first.
//
// Iteration stops at the first error, which is yielded with the zero [rdf.Quad]
// and followed by nothing:
//
//	for quad, err := range trig.Decode(r) {
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
// An empty graph block yields no quads, there being no quad that says a graph
// exists and holds nothing. A caller that needs to keep an empty named graph
// wants [Dataset], which builds an [rdf.Dataset] and can.
//
// Comments and version directives are dropped: neither says anything about the
// dataset, a version announcement being a hint that mandates no parser
// behaviour (Turtle §7.1). A [Document] from [Parse] keeps both, for a caller
// who wants to write the document back rather than read what it says.
func Decode(r io.Reader) iter.Seq2[rdf.Quad, error] {
	return func(yield func(rdf.Quad, error) bool) {
		d := &decoder{yield: yield}
		d.lowering = newLowering(d.emit, nil)

		err := parse(r, d)
		switch {
		case err == nil, errors.Is(err, errConsumerStopped):
			return
		default:
			yield(rdf.Quad{}, err)
		}
	}
}

// decoder is the [sink] that lowers each statement and hands its quads
// straight on.
type decoder struct {
	lowering *lowering
	yield    func(rdf.Quad, error) bool
}

// statement lowers the statement, yielding whatever quads it expands to.
func (d *decoder) statement(s Statement) error { return d.lowering.statement(s) }

// comment drops the comment. Streaming reports what a document says, and a
// comment says nothing.
//
// Taking the text raw is what makes dropping it free: a document thick with
// comments costs a decoder nothing to read past.
func (d *decoder) comment(Pos, []byte) {}

// emit hands one quad to the caller, reporting errConsumerStopped once the
// caller has stopped listening.
func (d *decoder) emit(quad rdf.Quad) error {
	if !d.yield(quad, nil) {
		return errConsumerStopped
	}
	return nil
}

// Quads turns an already-parsed document into data model quads.
//
// This is [Decode] for a caller who wanted the syntax tree as well — the same
// lowering, over a document already in memory. Blank node labels resolve
// through one scope for the whole document, so the quads it returns can be
// added to a dataset alongside each other but not alongside those of another
// document, whose _:b0 is a different node.
//
// As with [Decode], an empty graph block yields no quads.
//
// A nil document has no quads and is not an error.
func Quads(doc *Document) ([]rdf.Quad, error) {
	if doc == nil {
		return nil, nil
	}

	var quads []rdf.Quad
	l := newLowering(func(quad rdf.Quad) error {
		quads = append(quads, quad)
		return nil
	}, nil)

	for _, statement := range doc.Statements {
		if err := l.statement(statement); err != nil {
			return nil, err
		}
	}
	return quads, nil
}

// Dataset reads the RDF 1.2 TriG document in r and returns the dataset it
// describes.
//
// It is [Decode] gathered up: the same lowering, with every quad added to an
// [rdf.Dataset] rather than handed to a caller. The one thing it can say that a
// sequence of quads cannot is that a named graph exists and is empty, which a
// document writes as a block with nothing between its braces — so a dataset
// read here and written by [Encode] keeps its empty graphs.
//
// It reports the first error [Decode] would have yielded.
func Dataset(r io.Reader) (*rdf.Dataset, error) {
	dataset := rdf.NewDataset()

	l := newLowering(func(quad rdf.Quad) error {
		return dataset.Add(quad)
	}, func(name rdf.Term) error {
		_, err := dataset.AddGraph(name)
		return err
	})

	if err := parse(r, &collector{lowering: l}); err != nil {
		return nil, err
	}
	return dataset, nil
}

// collector is the [sink] that lowers each statement into a dataset.
type collector struct {
	lowering *lowering
}

func (c *collector) statement(s Statement) error { return c.lowering.statement(s) }

func (c *collector) comment(Pos, []byte) {}

// lowering turns statements into quads, carrying the state the directives
// leave behind.
//
// A directive applies to what follows it and to nothing before it, so the state
// is read as each statement is lowered rather than gathered up front: a second
// @base changes only the IRIs after it, and a prefix bound twice means the
// second binding only from where it was written. None of it is reset by a graph
// block, a directive belonging to the document rather than to a graph.
type lowering struct {
	base     string
	hasBase  bool
	prefixes map[string]string
	scope    *rdf.BlankNodeScope
	emit     func(rdf.Quad) error

	// graph is the name of the graph being lowered into, and nil for the
	// default graph. It is set while a block's triples are lowered and put back
	// afterwards, which is cheaper than carrying it through every method that
	// nests, and correct for the same reason: a block cannot contain another.
	graph rdf.Term

	// declare is told the name of every graph a block opens, empty ones
	// included, or is nil for a caller that only wants quads. It is what lets
	// [Dataset] keep a named graph that holds nothing, which no quad can say.
	declare func(rdf.Term) error
}

func newLowering(emit func(rdf.Quad) error, declare func(rdf.Term) error) *lowering {
	return &lowering{
		prefixes: make(map[string]string),
		scope:    rdf.NewBlankNodeScope(),
		emit:     emit,
		declare:  declare,
	}
}

// statement lowers one statement of the document.
func (l *lowering) statement(s Statement) error {
	switch st := s.(type) {
	case *PrefixDirective:
		return l.prefixDirective(st)
	case *BaseDirective:
		return l.baseDirective(st)
	case *VersionDirective:
		return l.versionDirective(st)
	case *Triples:
		// Written outside any block, and so said of the default graph.
		return l.triples(st)
	case *GraphBlock:
		return l.graphBlock(st)
	default:
		panic(fmt.Sprintf("unknown statement node: %T", s))
	}
}

// graphBlock lowers the triples of a block into the graph it names.
//
//	wrappedGraph ::= '{' triplesBlock? '}'
//
// A block with no label says its triples of the default graph, which is the
// same graph the statements outside every block belong to: the two are one
// graph and a document may add to it from either place (TriG §2.2).
func (l *lowering) graphBlock(b *GraphBlock) error {
	var name rdf.Term
	if b.Label != nil {
		term, err := l.term(b.Label)
		if err != nil {
			return err
		}

		if err := validateGraphName(term); err != nil {
			return InvalidTermError{Pos: b.Label.Position(), Err: err}
		}
		name = term

		if l.declare != nil {
			if err := l.declare(name); err != nil {
				return InvalidTermError{Pos: b.Label.Position(), Err: err}
			}
		}
	}

	previous := l.graph
	l.graph = name
	defer func() { l.graph = previous }()

	for _, t := range b.Triples {
		if err := l.triples(t); err != nil {
			return err
		}
	}
	return nil
}

// emitTriple emits a triple the syntax stands for rather than writes — the
// cells of a collection, and the rdf:reifies triple of a reification — in the
// graph being lowered into.
func (l *lowering) emitTriple(t rdf.Triple) error {
	return l.emit(rdf.Quad{Triple: t, Graph: l.graph})
}

// validateGraphName reports the data model's own error for a term that cannot
// name a graph.
//
//	labelOrSubject ::= iri | BlankNode
//
// The production admits nothing else, so only a tree built by hand can reach
// this with a literal or a triple term. The check is here rather than left to
// [rdf.Quad.Validate] because an empty block carries a name and no quads, and
// a name nothing is said with would otherwise go unexamined.
func validateGraphName(name rdf.Term) error {
	switch name.(type) {
	case rdf.IRI, rdf.BlankNode:
		return nil
	default:
		return fmt.Errorf("%w: got %s", rdf.ErrInvalidGraphName, name)
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

// versionDirective drops the announcement, leaving no state behind and no
// triple.
//
//	version       ::= '@version' VersionSpecifier '.'
//	sparqlVersion ::= "VERSION" VersionSpecifier
//
// The specification calls the announcement a hint and mandates no parser
// behaviour from it (Turtle §7.1), so this package reads the version it is
// named for whatever a document claims: one announcing "1.1" and then writing a
// triple term is read exactly as one announcing "1.2" would be.
//
// It is a rule of its own rather than an empty case so that the reason is
// written where a reader of the statement switch looks for it.
func (l *lowering) versionDirective(*VersionDirective) error { return nil }

// triples lowers a subject and everything said about it.
//
//	triples ::= subject predicateObjectList
//	          | blankNodePropertyList predicateObjectList?
//	          | reifiedTriple predicateObjectList?
//
// A subject that is a reified triple has already said something by the time it
// is lowered — its rdf:reifies triple — which is why an empty predicate list is
// not an empty statement.
func (l *lowering) triples(t *Triples) error {
	subject, err := l.term(t.Subject)
	if err != nil {
		return err
	}
	return l.predicateObjects(subject, t.Predicates)
}

// predicateObjects emits one quad for every object of every verb given for
// subject, each in the graph being lowered into, followed by whatever each
// object's annotation says about it.
//
//	predicateObjectList ::= verb objectList (';' (verb objectList)?)*
//	objectList          ::= object annotation (',' object annotation)*
func (l *lowering) predicateObjects(subject rdf.Term, list []*PredicateObject) error {
	for _, po := range list {
		predicate, err := l.verb(po.Verb)
		if err != nil {
			return err
		}

		for _, node := range po.Objects {
			object, err := l.term(node.Term)
			if err != nil {
				return err
			}

			triple := rdf.Triple{Subject: subject, Predicate: predicate, Object: object}
			quad := rdf.Quad{Triple: triple, Graph: l.graph}
			// The syntax tree already rules out a literal subject and a blank
			// node predicate, and an IRI is only accepted once it has been made
			// absolute, so there is nothing left here for the data model to
			// refuse. It is checked all the same, since a dataset will check it
			// on insert and a failure found here can say where it was written.
			if err := quad.Validate(); err != nil {
				return InvalidTermError{Pos: po.Verb.Position(), Err: err}
			}
			if err := l.emit(quad); err != nil {
				return err
			}

			if err := l.annotation(triple, node.Annotation); err != nil {
				return err
			}
		}
	}
	return nil
}

// annotation emits what the reifiers and annotation blocks written after an
// object say about the triple that object completes.
//
//	annotation      ::= (reifier | annotationBlock)*
//	annotationBlock ::= '{|' predicateObjectList '|}'
//
// The triple is annotated by way of a reifier, exactly as a reified triple is:
// the triple term standing for it is built once, and every reifier in the
// sequence is related to that one term with rdf:reifies (RDF 1.2 Turtle §7.3).
// The triple itself has already been emitted, which is the difference between
// the two forms — an annotation asserts the triple it describes and a reified
// triple does not.
//
// A block takes the reifier written immediately before it, and the '~' is
// spent doing so: in "~ :r {| :q :v |} {| :x :y |}" the first block is said of
// ":r" and the second of a blank node minted for it. A block with no reifier
// before it mints one and states its rdf:reifies too, which is why two blocks
// written alike still describe two reifications.
func (l *lowering) annotation(triple rdf.Triple, items []Annotation) error {
	if len(items) == 0 {
		return nil
	}

	// A conversion rather than three assignments: the triple term an annotation
	// reifies is exactly the triple just emitted, standing as a term. The two
	// types have the same three fields, which is what makes it legal, and a
	// field added to either would be a compile error here rather than a
	// silently dropped one.
	term := rdf.TripleTerm(triple)

	// The reifier in scope, or nil when none is: cleared at the start of the
	// annotation and again as each block spends the one it was given.
	var reifier rdf.Term
	for _, item := range items {
		switch it := item.(type) {
		case *Reifier:
			r, err := l.reifier(it)
			if err != nil {
				return err
			}
			if err := l.reifies(r, term, it.Pos); err != nil {
				return err
			}
			reifier = r

		case *AnnotationBlock:
			if reifier == nil {
				reifier = l.scope.New()
				if err := l.reifies(reifier, term, it.Pos); err != nil {
					return err
				}
			}
			if err := l.predicateObjects(reifier, it.Predicates); err != nil {
				return err
			}
			reifier = nil

		default:
			panic(fmt.Sprintf("unknown annotation node: %T", item))
		}
	}
	return nil
}

// reifies states that reifier identifies term, which is the one triple a
// reifier stands for wherever it is written.
//
// pos is where to report a term the data model refuses. Nothing the grammar
// admits in a reifier can fail the check — an IRI or a blank node is what the
// subject position wants, and rdf:reifies is not the empty IRI — so this is the
// same belt-and-braces validation the rest of the lowering does.
func (l *lowering) reifies(reifier rdf.Term, term rdf.TripleTerm, pos Pos) error {
	triple := rdf.Triple{Subject: reifier, Predicate: vocab.RDFReifies, Object: term}
	if err := triple.Validate(); err != nil {
		return InvalidTermError{Pos: pos, Err: err}
	}
	return l.emitTriple(triple)
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
	case *TripleTerm:
		return l.tripleTerm(t)
	case *ReifiedTriple:
		return l.reifiedTriple(t)
	default:
		panic(fmt.Sprintf("unknown term node: %T", node))
	}
}

// tripleTerm lowers a triple standing as a term.
//
//	tripleTerm ::= '<<(' ttSubject verb ttObject ')>>'
//
// It emits nothing. The enclosed statement is a term of the data model here and
// not an assertion, so the only triple it can take part in is the one that has
// it as an object.
//
// A nested triple term lowers through [lowering.term] and so through here
// again, which is what carries the recursion of the production into the data
// model. Blank nodes inside one resolve through the same scope as the rest of
// the document: _:b0 in a triple term and _:b0 in a statement are the same
// node.
func (l *lowering) tripleTerm(t *TripleTerm) (rdf.TripleTerm, error) {
	subject, err := l.term(t.Subject)
	if err != nil {
		return rdf.TripleTerm{}, err
	}

	predicate, err := l.verb(t.Verb)
	if err != nil {
		return rdf.TripleTerm{}, err
	}

	object, err := l.term(t.Object)
	if err != nil {
		return rdf.TripleTerm{}, err
	}

	return rdf.TripleTerm{Subject: subject, Predicate: predicate, Object: object}, nil
}

// reifiedTriple lowers a triple written where a term is wanted, returning the
// reifier that stands where it was written.
//
//	reifiedTriple ::= '<<' rtSubject verb rtObject reifier? '>>'
//
// The form is sugar for one triple relating a reifier to a triple term with
// rdf:reifies (RDF 1.2 Turtle §2.11), and that triple is emitted here. The
// enclosed statement itself is not: "<< :s :p :o >> :q :v ." says something
// about ":s :p :o" and does not say that ":s :p :o" holds.
//
// The reifier is what the term evaluates to, so a reified triple in the subject
// position is described by whatever follows it, and one in the object position
// is what the enclosing triple points at.
func (l *lowering) reifiedTriple(r *ReifiedTriple) (rdf.Term, error) {
	term, err := l.tripleTerm(&TripleTerm{
		Pos:     r.Pos,
		Subject: r.Subject,
		Verb:    r.Verb,
		Object:  r.Object,
	})
	if err != nil {
		return nil, err
	}

	reifier, err := l.reifier(r.Reifier)
	if err != nil {
		return nil, err
	}

	if err := l.reifies(reifier, term, r.Pos); err != nil {
		return nil, err
	}
	return reifier, nil
}

// reifier lowers the identifier a reified triple or an annotation is given,
// minting a blank node when the document named none.
//
//	reifier ::= '~' (iri | BlankNode)?
//
// A reifier left out, and a '~' written with nothing after it, come to the same
// thing: the specification mints a fresh blank node for both. They are two
// spellings and not two meanings, which is why nil is handled here rather than
// in the caller.
func (l *lowering) reifier(r *Reifier) (rdf.Term, error) {
	if r == nil || r.ID == nil {
		return l.scope.New(), nil
	}
	return l.term(r.ID)
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
		if err := l.emitTriple(rdf.Triple{Subject: cell, Predicate: vocab.RDFFirst, Object: first}); err != nil {
			return nil, err
		}

		var rest rdf.Term = vocab.RDFNil
		next := cell
		if i < len(c.Objects)-1 {
			next = l.scope.New()
			rest = next
		}
		if err := l.emitTriple(rdf.Triple{Subject: cell, Predicate: vocab.RDFRest, Object: rest}); err != nil {
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
// rdf:langString, a base direction after it implies rdf:dirLangString, an
// explicit datatype is taken as written, and none of them means xsd:string.
// What Turtle adds is the shorthands, each of which carries its datatype by
// being written the way it is (Turtle §2.5.1, §2.5.2):
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
	case node.Direction != "":
		// A direction with no language tag is unreachable from the grammar —
		// LANG_DIR demands a letter before the "--" — but a direction that is
		// neither "ltr" nor "rtl" is not: the production admits any run of
		// letters, and the constraint is stated where RDF terms are built
		// rather than in the grammar.
		literal, err := rdf.NewDirectionalLiteral(node.Value, node.Language, rdf.Direction(node.Direction))
		if err != nil {
			return rdf.Literal{}, InvalidTermError{Pos: node.Pos, Err: err}
		}
		return literal, nil

	case node.Language != "":
		// The only way this fails is an empty tag, which the LANG_DIR production
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
//	RDFLiteral ::= String (LANG_DIR | '^^' iri)?
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
