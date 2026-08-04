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
//
// The second form is why Predicates may be empty: "[ :p :o ] ." says
// everything it has to say inside the brackets.
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
//	objectList          ::= object (',' object)*
type PredicateObject struct {
	Pos     Pos
	Verb    Verb
	Objects []Term
}

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
//	RDFLiteral ::= String (LANGTAG | '^^' iri)?
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

	// Language is the language tag without its '@', or "" if there was none.
	// Only a string may carry one.
	Language string

	// LangPos is the position of the '@' beginning the LANGTAG, or the zero
	// [Pos] if there was none. It is what an error about the language tag
	// points at, RFC 5646 well-formedness being a constraint on this token
	// that the grammar does not state.
	LangPos Pos

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
