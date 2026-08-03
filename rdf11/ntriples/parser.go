package ntriples

import (
	"fmt"
	"io"
	"iter"
	"slices"
	"strings"
)

// Document is a parsed N-Triples document.
//
//	ntriplesDoc ::= triple? (EOL triple?)*
type Document struct {
	// Pos is the start of the document, which is always line 1, column 1. It
	// is here so that a Document can be handled alongside the nodes within it.
	Pos Pos

	// Triples are the document's statements, in the order they were written.
	Triples []*Triple

	// Comments are every comment in the document, in the order they were
	// written.
	//
	// A comment is white space to the grammar and belongs to no statement, so
	// they are kept here rather than hung off the triples. Each carries the
	// position it was written at, which is what a printer needs to put it
	// back: a comment sharing a line with a triple trailed it, and one that
	// shares its line with nothing stood alone.
	Comments []*Comment
}

// Comment is a comment, from its '#' to the end of the line. Text is the
// comment as written, '#' included.
type Comment struct {
	Pos  Pos
	Text string
}

// Triple is one statement of a document.
//
//	triple ::= subject predicate object '.'
//
// The positions the grammar allows each term are expressed in the field types:
// a predicate can only ever be an [*IRIRef], and a [*Literal] is not a
// [SubjectTerm] and so cannot be a subject.
type Triple struct {
	// Pos is the position of the subject, where the statement begins.
	Pos Pos

	// Subject is an [*IRIRef] or a [*BlankNode].
	Subject SubjectTerm

	// Predicate is always an [*IRIRef].
	Predicate *IRIRef

	// Object is an [*IRIRef], a [*BlankNode] or a [*Literal].
	Object Term
}

// Term is a term of a statement: an [*IRIRef], a [*BlankNode] or a
// [*Literal].
//
// The interface is sealed by an unexported method, so a type switch over a
// Term is exhaustive and a default case can be treated as a programming error.
type Term interface {
	// Position returns the position of the term's first character.
	Position() Pos

	// term seals the interface.
	term()
}

// SubjectTerm is a term that may stand as the subject of a statement: an
// [*IRIRef] or a [*BlankNode].
//
//	subject ::= IRIREF | BLANK_NODE_LABEL
//
// A [*Literal] deliberately does not implement it. The parser still reports a
// literal subject as a parse error, since a document may well contain one, but
// no caller assembling a [Triple] by hand can express it.
type SubjectTerm interface {
	Term

	// subjectTerm seals the interface.
	subjectTerm()
}

// Compile-time proof that each term node stands where the grammar says it may.
var (
	_ SubjectTerm = (*IRIRef)(nil)
	_ SubjectTerm = (*BlankNode)(nil)
	_ Term        = (*Literal)(nil)
)

// IRIRef is an IRIREF, written between angle brackets. Value is the IRI it
// denotes: the brackets removed and any UCHAR decoded.
//
// Whether the IRI is absolute is not checked here. A relative one is a
// document-level error rather than a syntactic one, and resolving it against a
// base is the job of the iri package.
type IRIRef struct {
	Pos   Pos
	Value string
}

// Position returns the position of the opening '<'.
func (i *IRIRef) Position() Pos { return i.Pos }

func (*IRIRef) term()        {}
func (*IRIRef) subjectTerm() {}

// BlankNode is a BLANK_NODE_LABEL, written _:name. Label is the name, without
// the leading "_:".
//
// The label is the one the document wrote. It means nothing outside this
// document, and mapping it to one that does is the job of the scope in the rdf
// package.
type BlankNode struct {
	Pos   Pos
	Label string
}

// Position returns the position of the leading '_'.
func (b *BlankNode) Position() Pos { return b.Pos }

func (*BlankNode) term()        {}
func (*BlankNode) subjectTerm() {}

// Literal is a literal.
//
//	literal ::= STRING_LITERAL_QUOTE ('^^' IRIREF | LANGTAG)?
//
// The three forms a literal may take are distinguished by which of Language
// and Datatype is set, and the grammar admits at most one of them:
//
//	"plain"                     Language "", Datatype nil
//	"tagged"@en                 Language "en", Datatype nil
//	"typed"^^<http://ex/dt>     Language "", Datatype the IRI
//
// A literal with neither is xsd:string in the data model, but that is a fact
// about what it means rather than about what was written, so Datatype stays
// nil here and the default is applied on the way to an [rdf.Literal].
type Literal struct {
	// Pos is the position of the opening quote.
	Pos Pos

	// Value is the lexical form, without the quotes and with every ECHAR and
	// UCHAR decoded.
	Value string

	// Language is the language tag without its '@', or "" if there was none.
	Language string

	// Datatype is the IRI after the "^^", or nil if there was none.
	Datatype *IRIRef
}

// Position returns the position of the opening quote.
func (l *Literal) Position() Pos { return l.Pos }

func (*Literal) term() {}

// UnexpectedTokenError is reported when a token appears where the grammar does
// not allow it: a literal as a subject, a blank node as a predicate, or a term
// where the '.' ending a statement was due.
type UnexpectedTokenError struct {
	Expected []TokenType
	Actual   Token
}

// Error implements the [error] interface.
func (e UnexpectedTokenError) Error() string {
	return fmt.Sprintf(
		"unexpected token at line %d, column %d: %s, expected one of: %s",
		e.Actual.Pos.Line,
		e.Actual.Pos.Column,
		e.Actual.String(),
		joinTokenTypes(e.Expected),
	)
}

// UnexpectedEndOfTokensError is reported when the document ends while a
// statement is still unfinished.
//
// Pos is the last token read, which is as far as the parser got.
type UnexpectedEndOfTokensError struct {
	Expected []TokenType
	Pos      Pos
}

// Error implements the [error] interface.
func (e UnexpectedEndOfTokensError) Error() string {
	return fmt.Sprintf(
		"unexpected end of tokens at line %d, column %d, expected one of: %s",
		e.Pos.Line,
		e.Pos.Column,
		joinTokenTypes(e.Expected),
	)
}

func joinTokenTypes(types []TokenType) string {
	names := make([]string, 0, len(types))
	for _, t := range types {
		names = append(names, t.String())
	}
	return strings.Join(names, ", ")
}

// Parse reads the N-Triples document in r and returns its syntax tree.
//
// The tree records what the document said rather than what it means: an IRI is
// the text between its brackets, a blank node label is the one the document
// wrote, and a literal with no datatype has none rather than having
// xsd:string. Turning that into the terms of the rdf package, where those
// distinctions are resolved, is a separate step.
//
// Every node carries the position it was written at, so a tool reading the
// tree can point at the source, and comments are kept so a printer can put
// them back.
//
// Parsing stops at the first error, which is either one of this package's
// parse errors or the tokenizer error that prevented reading further.
func Parse(r io.Reader) (*Document, error) {
	doc := &Document{Pos: Pos{Line: 1, Column: 1}}
	if err := parse(r, doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// sink receives what the parser reads, one statement at a time.
//
// There is only one implementation of the grammar, and this is how it serves
// two callers with opposite needs. [Parse] collects everything into a
// [Document]; [Decode] lowers each statement and hands it on the moment its
// '.' arrives, keeping nothing, which is what lets it read a document larger
// than memory. Splitting the grammar in two instead would leave two things to
// keep in step with the specification.
type sink interface {
	// statement receives a parsed statement. Returning an error stops
	// parsing, and the error is what parse reports.
	statement(*Triple) error

	// comment receives a comment at pos, with text as the tokenizer read it.
	//
	// The text is passed raw, and is only good until the next token is read.
	// An implementation that keeps a comment has to copy it, and one that
	// drops it — which streaming does — then pays nothing for a comment it
	// was never going to look at.
	comment(pos Pos, text []byte)
}

// statement appends the statement to the document.
func (d *Document) statement(t *Triple) error {
	d.Triples = append(d.Triples, t)
	return nil
}

// comment appends a copy of the comment to the document.
func (d *Document) comment(pos Pos, text []byte) {
	d.Comments = append(d.Comments, &Comment{Pos: pos, Text: string(text)})
}

// parse reads the document in r, handing each statement and comment to s.
func parse(r io.Reader, s sink) error {
	next, stop := iter.Pull2(Tokenize(r))
	defer stop()

	p := &parser{
		next: next,
		pos:  Pos{Line: 1, Column: 1},
	}

	var err error
	for action := parseDocument; action != nil && err == nil; {
		action, err = action(p, s)
	}
	return err
}

// parser reads the token stream one token at a time, with a single token of
// lookahead.
type parser struct {
	next    func() (Token, error, bool)
	pending *Token
	pos     Pos
}

// read consumes the next token, reporting whether there was one.
func (p *parser) read() (Token, error, bool) {
	if p.pending != nil {
		tok := *p.pending
		p.pending = nil
		p.pos = tok.Pos
		return tok, nil, true
	}

	tok, err, ok := p.next()
	if ok && err == nil {
		p.pos = tok.Pos
	}
	return tok, err, ok
}

// peek reports what the next token is without consuming it.
func (p *parser) peek() (Token, error, bool) {
	if p.pending != nil {
		return *p.pending, nil, true
	}

	tok, err, ok := p.next()
	if err != nil {
		return Token{}, err, false
	}
	if !ok {
		return Token{}, nil, false
	}
	p.pending = &tok
	return tok, nil, true
}

// discard consumes the token that peek returned.
func (p *parser) discard() {
	_, _, _ = p.read()
}

// expect consumes the next token, requiring it to be one of the given types.
func (p *parser) expect(expected ...TokenType) (Token, error) {
	tok, err, ok := p.read()
	if err != nil {
		return Token{}, err
	}
	if !ok {
		return Token{}, UnexpectedEndOfTokensError{Expected: expected, Pos: p.pos}
	}

	if slices.Contains(expected, tok.Type) {
		return tok, nil
	}
	return Token{}, UnexpectedTokenError{Expected: expected, Actual: tok}
}

// parserAction is one step of parsing: it consumes some tokens, adds to what
// is being built, and returns the step to run next, or nil when there is
// nothing left to parse.
type parserAction[T any] func(p *parser, t T) (parserAction[T], error)

// parseDocument reads whatever comes between statements — line endings, which
// separate them, and comments, which the grammar counts as white space — and
// then either ends the document or hands over to parseTriple.
func parseDocument(p *parser, s sink) (parserAction[sink], error) {
	for {
		tok, err, ok := p.peek()
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}

		switch tok.Type {
		case TokenEOL:
			p.discard()
		case TokenComment:
			p.discard()
			s.comment(tok.Pos, tok.Value)
		default:
			return parseTriple, nil
		}
	}
}

// parseTriple reads one statement.
//
//	triple ::= subject predicate object '.'
//
// Nothing may come between the four, not even a comment: a comment runs to the
// end of its line, and a statement does not outlive its own line.
func parseTriple(p *parser, s sink) (parserAction[sink], error) {
	subject, err := parseSubject(p)
	if err != nil {
		return nil, err
	}

	predicate, err := parsePredicate(p)
	if err != nil {
		return nil, err
	}

	object, err := parseObject(p)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(TokenDot); err != nil {
		return nil, err
	}

	err = s.statement(&Triple{
		Pos:       subject.Position(),
		Subject:   subject,
		Predicate: predicate,
		Object:    object,
	})
	if err != nil {
		return nil, err
	}

	return parseTripleEnd, nil
}

// parseTripleEnd requires a statement to be the last thing on its line.
//
//	ntriplesDoc ::= triple? (EOL triple?)*
//
// The grammar separates statements with a line ending, so a second one on the
// same line is refused rather than quietly accepted. A comment may follow,
// having nowhere to run to but the end of the line.
func parseTripleEnd(p *parser, s sink) (parserAction[sink], error) {
	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	switch tok.Type {
	case TokenEOL, TokenComment:
		return parseDocument, nil
	default:
		return nil, UnexpectedTokenError{
			Expected: []TokenType{TokenEOL, TokenComment},
			Actual:   tok,
		}
	}
}

// parseSubject reads the subject of a statement.
//
//	subject ::= IRIREF | BLANK_NODE_LABEL
func parseSubject(p *parser) (SubjectTerm, error) {
	tok, err := p.expect(TokenIRIRef, TokenBlankNodeLabel)
	if err != nil {
		return nil, err
	}

	if tok.Type == TokenIRIRef {
		return &IRIRef{Pos: tok.Pos, Value: string(tok.Value)}, nil
	}
	return &BlankNode{Pos: tok.Pos, Label: string(tok.Value)}, nil
}

// parsePredicate reads the predicate of a statement.
//
//	predicate ::= IRIREF
//
// A predicate names a relation, and only an IRI can. A blank node here is
// refused however well it would have read.
func parsePredicate(p *parser) (*IRIRef, error) {
	tok, err := p.expect(TokenIRIRef)
	if err != nil {
		return nil, err
	}
	return &IRIRef{Pos: tok.Pos, Value: string(tok.Value)}, nil
}

// parseObject reads the object of a statement.
//
//	object ::= IRIREF | BLANK_NODE_LABEL | literal
func parseObject(p *parser) (Term, error) {
	tok, err := p.expect(TokenIRIRef, TokenBlankNodeLabel, TokenStringLiteral)
	if err != nil {
		return nil, err
	}

	switch tok.Type {
	case TokenIRIRef:
		return &IRIRef{Pos: tok.Pos, Value: string(tok.Value)}, nil
	case TokenBlankNodeLabel:
		return &BlankNode{Pos: tok.Pos, Label: string(tok.Value)}, nil
	default:
		return parseLiteral(p, tok)
	}
}

// parseLiteral reads what may follow a quoted string, the string itself having
// been read.
//
//	literal ::= STRING_LITERAL_QUOTE ('^^' IRIREF | LANGTAG)?
//
// Both the tag and the datatype are optional and mutually exclusive, so a
// string standing alone is a complete literal and anything that is neither is
// simply not part of it.
func parseLiteral(p *parser, quote Token) (*Literal, error) {
	literal := &Literal{Pos: quote.Pos, Value: string(quote.Value)}

	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return literal, nil
	}

	switch tok.Type {
	case TokenLangTag:
		p.discard()
		literal.Language = string(tok.Value)
	case TokenDatatypeMarker:
		p.discard()

		datatype, err := p.expect(TokenIRIRef)
		if err != nil {
			return nil, err
		}
		literal.Datatype = &IRIRef{Pos: datatype.Pos, Value: string(datatype.Value)}
	}

	return literal, nil
}
