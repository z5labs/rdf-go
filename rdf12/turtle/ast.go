// This file began as a copy of rdf11/turtle/ast.go and keeps its shape: the
// same Document, the same Statement and Term interfaces, and the same nodes for
// everything RDF 1.1 could write.
//
// What RDF 1.2 adds here is [TripleTerm], [ReifiedTriple], [Reifier] and
// [AnnotationBlock], and a base direction on [Literal]. The two bracketed forms
// are kept as two nodes rather than one with a flag, because they are not two
// spellings of one thing: a triple term is a term of the data model, and a
// reified triple is sugar that stands for a reifier and an rdf:reifies triple
// relating it to one. A printer asked to write back what the author wrote has
// to be able to tell them apart.
//
// The annotation syntax is why an object list holds [Object] rather than
// [Term]: an annotation is written after one object of the list and belongs to
// that object alone, so it is a field of the object and not of the list.

package turtle

// Document is a parsed Turtle document.
//
//	turtleDoc ::= statement*
type Document struct {
	// Pos is the start of the document, always line 1, column 1.
	Pos Pos

	// Statements are the document's directives and triples, in the order they
	// were written. A directive applies to what follows it, so the order is
	// part of what the document means and not merely how it was laid out.
	Statements []Statement

	// Comments are every comment in the document, in the order they were
	// written, each with the position it was written at. A comment is white
	// space to the grammar and belongs to no statement, so a printer places
	// them by comparing positions.
	Comments []*Comment
}

// Comment is a comment, from its '#' to the end of the line.
type Comment struct {
	Pos  Pos
	Text string
}

// Statement is one statement of a document: a [*PrefixDirective], a
// [*BaseDirective] or a [*Triples].
//
//	statement ::= directive | triples '.'
type Statement interface {
	// Position returns where the statement begins.
	Position() Pos

	// statement seals the interface.
	statement()
}

// PrefixDirective binds a prefix to an IRI.
//
//	prefixID     ::= '@prefix' PNAME_NS IRIREF '.'
//	sparqlPrefix ::= "PREFIX" PNAME_NS IRIREF
//
// Keyword is the word as written, which is what tells the two forms apart:
// only the '@' form is ended by a '.', and only the SPARQL form may be spelled
// in any case. A printer needs it to write back what it read.
type PrefixDirective struct {
	Pos     Pos
	Keyword string
	Prefix  string
	IRI     *IRIRef
}

// Position returns the position of the keyword.
func (d *PrefixDirective) Position() Pos { return d.Pos }

func (*PrefixDirective) statement() {}

// BaseDirective sets the base against which relative IRIs resolve.
//
//	base       ::= '@base' IRIREF '.'
//	sparqlBase ::= "BASE" IRIREF
type BaseDirective struct {
	Pos     Pos
	Keyword string
	IRI     *IRIRef
}

// Position returns the position of the keyword.
func (d *BaseDirective) Position() Pos { return d.Pos }

func (*BaseDirective) statement() {}

// Triples is a subject and the things said about it.
//
//	triples ::= subject predicateObjectList
//	          | blankNodePropertyList predicateObjectList?
//	          | reifiedTriple predicateObjectList?
//
// The second and third forms are why Predicates may be empty: "[ :p :o ] ."
// says everything it has to say inside the brackets, and "<< :s :p :o >> ."
// everything it has to say inside the angles.
type Triples struct {
	Pos        Pos
	Subject    Term
	Predicates []*PredicateObject
}

// Position returns the position of the subject.
func (t *Triples) Position() Pos { return t.Pos }

func (*Triples) statement() {}

// PredicateObject is one verb and the objects given for it.
//
//	predicateObjectList ::= verb objectList (';' (verb objectList)?)*
//	objectList          ::= object annotation (',' object annotation)*
type PredicateObject struct {
	Pos     Pos
	Verb    Verb
	Objects []*Object
}

// Object is one object of an object list, together with the annotation written
// after it.
//
//	objectList ::= object annotation (',' object annotation)*
//	annotation ::= (reifier | annotationBlock)*
//
// The annotation belongs to the object rather than to the list, which is the
// whole of what makes ":s :p :o1, :o2 {| :q :v |}" say something about
// ":s :p :o2" and nothing about ":s :p :o1". Keeping it here is also what lets
// a printer put it back where the author wrote it.
type Object struct {
	// Term is the object itself.
	Term Term

	// Annotation is the reifiers and annotation blocks written after the
	// object, in the order they were written, or nil if there were none.
	//
	// The order matters: an annotation block takes the reifier written
	// immediately before it and mints a blank node when there is none, so
	// "~ :r {| ... |}" and "{| ... |} ~ :r" do not mean the same thing.
	Annotation []Annotation
}

// Position returns the position of the object's first character.
func (o *Object) Position() Pos { return o.Term.Position() }

// Annotation is one item of what may follow an object: a [*Reifier] or an
// [*AnnotationBlock].
//
//	annotation ::= (reifier | annotationBlock)*
//
// The interface is sealed, so a type switch over it is exhaustive and a
// default case can be treated as a programming error.
type Annotation interface {
	// Position returns the position of the item's first character.
	Position() Pos

	// annotation seals the interface.
	annotation()
}

// AnnotationBlock is metadata attached to the triple just written, between
// "{|" and "|}".
//
//	annotationBlock ::= '{|' predicateObjectList '|}'
//
// It is sugar (RDF 1.2 Turtle §7.3): the block's subject is the reifier of the
// triple the annotation is written on — the one named by a '~' immediately
// before the block, or a fresh blank node — and everything inside the block is
// said about that reifier rather than about the triple's own subject.
//
// The predicate object list is the same production as a statement's, so an
// annotation block may hold anything a statement may, annotations included.
type AnnotationBlock struct {
	// Pos is the position of the opening "{|".
	Pos Pos

	// Predicates are the verbs and objects said about the reifier.
	Predicates []*PredicateObject
}

// Position returns the position of the opening "{|".
func (b *AnnotationBlock) Position() Pos { return b.Pos }

func (*AnnotationBlock) annotation() {}

// Verb is what stands in the predicate position: an [*IRIRef], a
// [*PrefixedName], or the [*A] keyword.
//
//	verb ::= predicate | 'a'
type Verb interface {
	Term

	// verb seals the interface.
	verb()
}

// Term is a term of the syntax tree.
//
// The interface is sealed, so a type switch over it is exhaustive and a
// default case can be treated as a programming error.
//
// Where a term may stand is not expressed in the type. RDF 1.2 gives the
// subject and the object of a triple, of a reified triple and of a triple term
// six different productions between them — a triple term may not enclose a
// reified triple, a reified triple's subject may not be a collection, and so on
// — and six marker interfaces over the same handful of nodes would say less
// than the productions do. The parser enforces them, each rule citing the one
// it implements.
type Term interface {
	// Position returns the position of the term's first character.
	Position() Pos

	// term seals the interface.
	term()
}

// Compile-time proof that each node stands where the grammar says it may.
var (
	_ Verb = (*IRIRef)(nil)
	_ Verb = (*PrefixedName)(nil)
	_ Verb = (*A)(nil)

	_ Term = (*BlankNode)(nil)
	_ Term = (*Anon)(nil)
	_ Term = (*Collection)(nil)
	_ Term = (*BlankNodePropertyList)(nil)
	_ Term = (*Literal)(nil)
	_ Term = (*TripleTerm)(nil)
	_ Term = (*ReifiedTriple)(nil)

	_ Annotation = (*Reifier)(nil)
	_ Annotation = (*AnnotationBlock)(nil)

	_ Statement = (*PrefixDirective)(nil)
	_ Statement = (*BaseDirective)(nil)
	_ Statement = (*Triples)(nil)
)

// IRIRef is an IRI written out in full, between angle brackets. Value is the
// IRI, without the brackets and with any UCHAR decoded.
//
// Whether it is absolute is not decided here: a Turtle document may write a
// relative IRI and expect a base to resolve it, which is a later step.
type IRIRef struct {
	Pos   Pos
	Value string
}

// Position returns the position of the opening '<'.
func (i *IRIRef) Position() Pos { return i.Pos }

func (*IRIRef) term() {}
func (*IRIRef) verb() {}

// PrefixedName is an IRI written as a prefix and a local name.
//
//	PrefixedName ::= PNAME_LN | PNAME_NS
//
// It is kept as written rather than expanded, so that a printer can write back
// the abbreviation the author chose. HasLocal tells "ex:" from "ex:" with an
// empty local name, which are the same IRI and not the same text.
type PrefixedName struct {
	Pos      Pos
	Prefix   string
	Local    string
	HasLocal bool
}

// Position returns the position of the prefix.
func (p *PrefixedName) Position() Pos { return p.Pos }

func (*PrefixedName) term() {}
func (*PrefixedName) verb() {}

// A is the keyword "a", which stands for rdf:type in the predicate position.
//
// It is a node of its own rather than an [*IRIRef] holding the rdf:type IRI,
// so that a printer writes back the "a" the author wrote. The two mean the
// same thing and do not read the same.
type A struct {
	Pos Pos
}

// Position returns the position of the keyword.
func (a *A) Position() Pos { return a.Pos }

func (*A) term() {}
func (*A) verb() {}

// BlankNode is a blank node the document names.
//
//	BLANK_NODE_LABEL ::= '_:' ...
type BlankNode struct {
	Pos   Pos
	Label string
}

// Position returns the position of the leading '_'.
func (b *BlankNode) Position() Pos { return b.Pos }

func (*BlankNode) term() {}

// Anon is a blank node the document does not name, written [].
//
//	ANON ::= '[' WS* ']'
type Anon struct {
	Pos Pos
}

// Position returns the position of the opening '['.
func (a *Anon) Position() Pos { return a.Pos }

func (*Anon) term() {}

// Collection is a list written in parentheses.
//
//	collection ::= '(' object* ')'
//
// It is kept as a list rather than expanded into the rdf:first and rdf:rest
// triples it stands for. Expanding it here would mean a printer could only
// write those triples back, and the document said something shorter.
type Collection struct {
	Pos     Pos
	Objects []Term
}

// Position returns the position of the opening '('.
func (c *Collection) Position() Pos { return c.Pos }

func (*Collection) term() {}

// BlankNodePropertyList is a blank node described in place, written in
// brackets.
//
//	blankNodePropertyList ::= '[' predicateObjectList ']'
//
// As with [Collection], it is kept as written rather than expanded into the
// triples about an invented blank node that it stands for.
type BlankNodePropertyList struct {
	Pos        Pos
	Predicates []*PredicateObject
}

// Position returns the position of the opening '['.
func (b *BlankNodePropertyList) Position() Pos { return b.Pos }

func (*BlankNodePropertyList) term() {}

// TripleTerm is a triple standing as a term, written between "<<(" and ")>>".
//
//	tripleTerm ::= '<<(' ttSubject verb ttObject ')>>'
//	ttSubject  ::= iri | BlankNode
//	ttObject   ::= iri | BlankNode | literal | tripleTerm
//
// It is a term of the RDF 1.2 data model and lowers to an [rdf.TripleTerm]
// unchanged, which is what separates it from [ReifiedTriple]: writing one says
// something about the enclosed statement without saying that the statement
// holds, and it stands for no triples of its own.
//
// Subject admits an IRI and a blank node and nothing else, so nesting is on the
// object side alone — and there only triple terms nest, a reified triple being
// sugar for a term the data model has no room for here.
//
// The grammar admits a triple term in the object position only, which is what
// [Term] does not say and the parser does.
type TripleTerm struct {
	// Pos is the position of the opening "<<(".
	Pos Pos

	// Subject is an [*IRIRef], a [*PrefixedName], a [*BlankNode] or an [*Anon].
	Subject Term

	// Verb is an [*IRIRef], a [*PrefixedName] or the [*A] keyword.
	Verb Verb

	// Object is an [*IRIRef], a [*PrefixedName], a [*BlankNode], an [*Anon], a
	// [*Literal] or another [*TripleTerm].
	Object Term
}

// Position returns the position of the opening "<<(".
func (t *TripleTerm) Position() Pos { return t.Pos }

func (*TripleTerm) term() {}

// ReifiedTriple is a triple written where a term is wanted, between "<<" and
// ">>", optionally naming the reifier it is given.
//
//	reifiedTriple ::= '<<' rtSubject verb rtObject reifier? '>>'
//	rtSubject     ::= iri | BlankNode | reifiedTriple
//	rtObject      ::= iri | BlankNode | literal | tripleTerm | reifiedTriple
//
// It is syntactic sugar (RDF 1.2 Turtle §2.11): the term it stands for is its
// reifier, and writing it states one triple, relating that reifier to the
// triple term it encloses with rdf:reifies. The enclosed statement is not
// asserted — "<< :s :p :o >> :q :v ." says something about ":s :p :o" and does
// not say that ":s :p :o" holds.
//
// The enclosed statement is kept as written rather than as the [*TripleTerm] it
// lowers to, so that a printer can write back the sugar. Both forms nest, and
// nest into each other on the object side.
type ReifiedTriple struct {
	// Pos is the position of the opening "<<".
	Pos Pos

	// Subject is an [*IRIRef], a [*PrefixedName], a [*BlankNode], an [*Anon] or
	// another [*ReifiedTriple].
	Subject Term

	// Verb is an [*IRIRef], a [*PrefixedName] or the [*A] keyword.
	Verb Verb

	// Object is an [*IRIRef], a [*PrefixedName], a [*BlankNode], an [*Anon], a
	// [*Literal], a [*TripleTerm] or another [*ReifiedTriple].
	Object Term

	// Reifier is the '~' and whatever followed it, or nil if none was written.
	//
	// A reifier left out and a reifier written bare both mint a blank node, so
	// the two mean the same thing; they do not read the same, and nil is what
	// tells them apart.
	Reifier *Reifier
}

// Position returns the position of the opening "<<".
func (r *ReifiedTriple) Position() Pos { return r.Pos }

func (*ReifiedTriple) term() {}

// Reifier is the identifier a reified triple or an annotated triple is given.
//
//	reifier ::= '~' (iri | BlankNode)?
//
// The identifier is optional: a '~' standing alone reifies under a blank node
// the document does not name, exactly as leaving the reifier out altogether
// does.
//
// One node serves both places the production is named, because it is the same
// production: the '~' inside a "<<" pair identifies the triple the pair
// encloses, and the '~' after an object identifies the triple that object
// completes.
type Reifier struct {
	// Pos is the position of the '~'.
	Pos Pos

	// ID is an [*IRIRef], a [*PrefixedName], a [*BlankNode] or an [*Anon], or
	// nil if the '~' stood alone.
	ID Term
}

// Position returns the position of the '~'.
func (r *Reifier) Position() Pos { return r.Pos }

func (*Reifier) annotation() {}

// LiteralKind is how a literal was written.
type LiteralKind int

const (
	// LiteralString is a quoted string, in any of the four forms.
	LiteralString LiteralKind = iota

	// LiteralInteger is a number written without a point or an exponent.
	LiteralInteger

	// LiteralDecimal is a number written with a point and no exponent.
	LiteralDecimal

	// LiteralDouble is a number written with an exponent.
	LiteralDouble

	// LiteralBoolean is true or false.
	LiteralBoolean
)

// String returns the name of the kind.
func (k LiteralKind) String() string {
	switch k {
	case LiteralString:
		return "String"
	case LiteralInteger:
		return "Integer"
	case LiteralDecimal:
		return "Decimal"
	case LiteralDouble:
		return "Double"
	case LiteralBoolean:
		return "Boolean"
	default:
		return "LiteralKind(" + itoa(int(k)) + ")"
	}
}

// Literal is a literal.
//
//	literal    ::= RDFLiteral | NumericLiteral | BooleanLiteral
//	RDFLiteral ::= String (LANG_DIR | '^^' iri)?
//
// Kind records which of those it was, because the three are written
// differently and mean their datatypes differently: a quoted string says its
// datatype or leaves it implied, while 42 and true carry theirs by being
// written the way they are. Keeping the distinction is what lets a printer
// write 42 back rather than "42"^^xsd:integer.
type Literal struct {
	Pos  Pos
	Kind LiteralKind

	// Value is the lexical form. For a string it is the text between the
	// quotes, decoded; for the others it is the number or the word as written.
	Value string

	// Language is the language tag without its '@' and without any base
	// direction, or "" if there was none. Only a string may carry one.
	Language string

	// Direction is the base direction written after the tag's "--", or "" if
	// there was none. It is what RDF 1.2 adds to the RDF 1.1 LANGTAG.
	//
	// It is recorded as written. That it must read "ltr" or "rtl" is a
	// constraint the specification states where it builds RDF terms rather than
	// in the grammar, so it is enforced on the way to an [rdf.Literal].
	Direction string

	// Datatype is the IRI after the "^^", or nil if none was written. Only a
	// string may carry one, and the datatype the other kinds imply is not
	// filled in here — the tree records what was written.
	Datatype Term
}

// Position returns the position of the literal's first character.
func (l *Literal) Position() Pos { return l.Pos }

func (*Literal) term() {}

// itoa renders a small non-negative int, so that LiteralKind.String needs no
// import.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var digits []byte
	negative := n < 0
	if negative {
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}
