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
//	ntriplesDoc ::= statement? (EOL statement)* EOL?
type Document struct {
	// Pos is the start of the document, which is always line 1, column 1. It
	// is here so that a Document can be handled alongside the nodes within it.
	Pos Pos

	// Statements are the document's statements, in the order they were
	// written: a [*Triple] for each triple and a [*VersionDirective] for each
	// version announcement.
	//
	// RDF 1.1 N-Triples had only triples to record, and the rdf11 package
	// keeps them in a list of its own. Here the two kinds share a list because
	// the order they were written in is what a version directive means: it
	// announces the version of the part of the document that follows it, so a
	// directive that has lost its place among the triples has lost what it
	// said.
	Statements []Statement

	// Comments are every comment in the document, in the order they were
	// written.
	//
	// A comment is white space to the grammar and belongs to no statement, so
	// they are kept here rather than hung off the statements. Each carries the
	// position it was written at, which is what a printer needs to put it
	// back: a comment sharing a line with a statement trailed it, and one that
	// shares its line with nothing stood alone.
	Comments []*Comment
}

// Comment is a comment, from its '#' to the end of the line. Text is the
// comment as written, '#' included.
type Comment struct {
	Pos  Pos
	Text string
}

// Statement is one statement of a document: a [*Triple] or a
// [*VersionDirective].
//
//	statement ::= directive | triple
//	directive ::= versionDirective
//
// The interface is sealed by an unexported method, so a type switch over a
// Statement is exhaustive and a default case can be treated as a programming
// error.
type Statement interface {
	// Position returns the position of the statement's first character.
	Position() Pos

	// statement seals the interface.
	statement()
}

// Compile-time proof that exactly the intended nodes are statements.
var (
	_ Statement = (*Triple)(nil)
	_ Statement = (*VersionDirective)(nil)
)

// VersionDirective is a version announcement.
//
//	versionDirective ::= 'VERSION' versionSpecifier
//	versionSpecifier ::= STRING_LITERAL_QUOTE
//
// It is a statement rather than a document header. The grammar admits one
// wherever a triple may stand, and RDF 1.2 Turtle §2.4 — which N-Triples
// refers to for the rules the two share — says a document may carry more than
// one, each applying to the part of the document that follows it. So the
// parser records every directive where it stands and refuses none for its
// placement.
//
// The version is recorded rather than acted on. The specification calls the
// announcement a hint and mandates no parser behaviour from it, so a document
// announcing "1.1" and then writing a triple term is read here exactly as one
// announcing "1.2" would be.
type VersionDirective struct {
	// Pos is the position of the 'V' that begins the VERSION keyword.
	Pos Pos

	// Version is the version specifier's string, without its quotes and with
	// every escape decoded. Which strings are version labels is settled by RDF
	// 1.2 Concepts §2.1 rather than by the grammar, and is not checked here.
	Version string
}

// Position returns the position of the VERSION keyword.
func (v *VersionDirective) Position() Pos { return v.Pos }

func (*VersionDirective) statement() {}

// Triple is one triple statement of a document.
//
//	triple ::= subject predicate object '.'
//
// The positions the grammar allows each term are expressed in the field types:
// a predicate can only ever be an [*IRIRef], and neither a [*Literal] nor a
// [*TripleTerm] is a [SubjectTerm], so neither can be a subject.
type Triple struct {
	// Pos is the position of the subject, where the statement begins.
	Pos Pos

	// Subject is an [*IRIRef] or a [*BlankNode].
	Subject SubjectTerm

	// Predicate is always an [*IRIRef].
	Predicate *IRIRef

	// Object is an [*IRIRef], a [*BlankNode], a [*Literal] or a
	// [*TripleTerm].
	Object Term
}

// Position returns the position of the subject, where the statement begins.
func (t *Triple) Position() Pos { return t.Pos }

func (*Triple) statement() {}

// Term is a term of a statement: an [*IRIRef], a [*BlankNode], a [*Literal] or
// a [*TripleTerm].
//
// The interface is sealed by an unexported method, so a type switch over a
// Term is exhaustive and a default case can be treated as a programming error.
type Term interface {
	// Position returns the position of the term's first character.
	Position() Pos

	// term seals the interface.
	term()
}

// SubjectTerm is a term that may stand as the subject of a statement, or of a
// triple term: an [*IRIRef] or a [*BlankNode].
//
//	subject ::= IRIREF | BLANK_NODE_LABEL
//
// Neither a [*Literal] nor a [*TripleTerm] implements it. The parser still
// reports either in subject position as a parse error, since a document may
// well contain one, but no caller assembling a [Triple] or a [TripleTerm] by
// hand can express it.
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
	_ Term        = (*TripleTerm)(nil)
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
//	literal ::= STRING_LITERAL_QUOTE (('^^' IRIREF) | LANG_DIR)?
//
// The forms a literal may take are distinguished by which of Language,
// Direction and Datatype is set, and the grammar admits either a LANG_DIR or a
// datatype but not both:
//
//	"plain"                     Language "", Direction "", Datatype nil
//	"tagged"@en                 Language "en"
//	"directional"@en--ltr       Language "en", Direction "ltr"
//	"typed"^^<http://ex/dt>     Datatype the IRI
//
// A literal with none of them is xsd:string in the data model, a tagged one is
// rdf:langString and a directional one rdf:dirLangString, but those are facts
// about what was meant rather than about what was written, so Datatype stays
// nil here and the implied datatype is applied on the way to an [rdf.Literal].
type Literal struct {
	// Pos is the position of the opening quote.
	Pos Pos

	// Value is the lexical form, without the quotes and with every ECHAR and
	// UCHAR decoded.
	Value string

	// Language is the language tag, without its '@' and without the base
	// direction, or "" if there was no LANG_DIR.
	Language string

	// LangPos is the position of the '@' beginning the LANG_DIR, or the zero
	// [Pos] if there was none. It is what an error about the language tag or
	// the base direction points at, both being constraints on this token that
	// the grammar does not state.
	LangPos Pos

	// Direction is the base direction written after the tag's "--", or "" if
	// there was none.
	//
	// It is recorded as written. That it must read "ltr" or "rtl" is a
	// constraint the specification states where it builds RDF terms
	// (RDF 1.2 N-Triples §6.2) rather than in the grammar, so it is enforced
	// on the way to an [rdf.Literal], alongside the rule that an IRI must be
	// absolute.
	Direction string

	// Datatype is the IRI after the "^^", or nil if there was none.
	Datatype *IRIRef
}

// Position returns the position of the opening quote.
func (l *Literal) Position() Pos { return l.Pos }

func (*Literal) term() {}

// TripleTerm is a triple standing as a term, which RDF 1.2 admits in the
// object position alone.
//
//	tripleTerm ::= '<<(' subject predicate object ')>>'
//
// The object it encloses is any object, so triple terms nest to any depth. Its
// subject is a [SubjectTerm], which a TripleTerm is not, so the nesting is on
// the object side only — and that is the grammar's restriction rather than an
// arbitrary one: the subject production a triple term shares with a triple
// admits an IRIREF and a blank node label and nothing else.
//
// A triple term is not an assertion: writing it says something about the
// enclosed statement without saying that the statement holds.
type TripleTerm struct {
	// Pos is the position of the opening "<<(".
	Pos Pos

	// Subject is an [*IRIRef] or a [*BlankNode].
	Subject SubjectTerm

	// Predicate is always an [*IRIRef].
	Predicate *IRIRef

	// Object is an [*IRIRef], a [*BlankNode], a [*Literal] or a
	// [*TripleTerm].
	Object Term
}

// Position returns the position of the opening "<<(".
func (t *TripleTerm) Position() Pos { return t.Pos }

func (*TripleTerm) term() {}

// UnexpectedTokenError is reported when a token appears where the grammar does
// not allow it: a literal as a subject, a triple term as a subject, a blank
// node as a predicate, or a term where the '.' ending a statement was due.
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
// wrote, a base direction is the word written after the tag's "--", and a
// literal with no datatype has none rather than having xsd:string. Turning
// that into the terms of the rdf package, where those distinctions are
// resolved, is a separate step.
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
// [Document]; [Decode] lowers each triple and hands it on the moment its '.'
// arrives, keeping nothing, which is what lets it read a document larger than
// memory. Splitting the grammar in two instead would leave two things to keep
// in step with the specification.
type sink interface {
	// triple receives a parsed triple. Returning an error stops parsing, and
	// the error is what parse reports.
	triple(*Triple) error

	// version receives a parsed version directive.
	//
	// It reports no error, unlike triple: a directive announces a version
	// rather than saying anything, so there is nothing here for a sink to
	// refuse and nothing to hand a consumer that could ask it to stop.
	version(*VersionDirective)

	// comment receives a comment at pos, with text as the tokenizer read it.
	//
	// The text is passed raw, and is only good until the next token is read.
	// An implementation that keeps a comment has to copy it, and one that
	// drops it — which streaming does — then pays nothing for a comment it
	// was never going to look at.
	comment(pos Pos, text []byte)
}

// triple appends the triple to the document.
func (d *Document) triple(t *Triple) error {
	d.Statements = append(d.Statements, t)
	return nil
}

// version appends the directive to the document, in its place among the
// triples it announces the version of.
func (d *Document) version(v *VersionDirective) {
	d.Statements = append(d.Statements, v)
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
// then either ends the document or hands over to the statement that follows.
//
//	statement ::= directive | triple
//
// The token that begins the statement is what tells the two apart, so the
// production needs no rule of its own: VERSION begins no term, and every term
// begins a triple.
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
		case TokenVersion:
			p.discard()
			return parseVersionDirective(tok), nil
		default:
			return parseTriple, nil
		}
	}
}

// parseVersionDirective reads a version announcement, the VERSION keyword
// having been read.
//
//	versionDirective ::= 'VERSION' versionSpecifier
//	versionSpecifier ::= STRING_LITERAL_QUOTE
//
// No '.' ends it: a directive is a statement in its own right and runs to the
// end of its line, which is what [parseStatementEnd] then requires.
func parseVersionDirective(keyword Token) parserAction[sink] {
	return func(p *parser, s sink) (parserAction[sink], error) {
		specifier, err := p.expect(TokenStringLiteral)
		if err != nil {
			return nil, err
		}

		s.version(&VersionDirective{
			Pos:     keyword.Pos,
			Version: string(specifier.Value),
		})

		return parseStatementEnd, nil
	}
}

// parseTriple reads one triple.
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

	err = s.triple(&Triple{
		Pos:       subject.Position(),
		Subject:   subject,
		Predicate: predicate,
		Object:    object,
	})
	if err != nil {
		return nil, err
	}

	return parseStatementEnd, nil
}

// parseStatementEnd requires a statement to be the last thing on its line.
//
//	ntriplesDoc ::= statement? (EOL statement)* EOL?
//
// The grammar separates statements with a line ending, so a second one on the
// same line is refused rather than quietly accepted. A comment may follow,
// having nowhere to run to but the end of the line.
func parseStatementEnd(p *parser, s sink) (parserAction[sink], error) {
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

// parseSubject reads the subject of a triple or of a triple term.
//
//	subject ::= IRIREF | BLANK_NODE_LABEL
//
// One production serves both, which is what confines a triple term to the
// object position: the subject a triple term encloses is read by the same rule
// as the subject of a statement, and neither admits "<<(".
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

// parsePredicate reads the predicate of a triple or of a triple term.
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

// parseObject reads the object of a triple or of a triple term.
//
//	object ::= IRIREF | BLANK_NODE_LABEL | literal | tripleTerm
//
// One production serves both, so the object of a triple term may be another
// triple term, to any depth.
func parseObject(p *parser) (Term, error) {
	tok, err := p.expect(TokenIRIRef, TokenBlankNodeLabel, TokenStringLiteral, TokenTripleTermOpen)
	if err != nil {
		return nil, err
	}

	switch tok.Type {
	case TokenIRIRef:
		return &IRIRef{Pos: tok.Pos, Value: string(tok.Value)}, nil
	case TokenBlankNodeLabel:
		return &BlankNode{Pos: tok.Pos, Label: string(tok.Value)}, nil
	case TokenTripleTermOpen:
		return parseTripleTerm(p, tok)
	default:
		return parseLiteral(p, tok)
	}
}

// parseTripleTerm reads the statement a triple term encloses, the "<<(" having
// been read.
//
//	tripleTerm ::= '<<(' subject predicate object ')>>'
//
// The recursion is [parseObject]'s: the object here is read by the same rule
// as the object of a statement, so a triple term nests within a triple term
// for as long as the document keeps opening them.
func parseTripleTerm(p *parser, open Token) (*TripleTerm, error) {
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

	if _, err := p.expect(TokenTripleTermClose); err != nil {
		return nil, err
	}

	return &TripleTerm{
		Pos:       open.Pos,
		Subject:   subject,
		Predicate: predicate,
		Object:    object,
	}, nil
}

// parseLiteral reads what may follow a quoted string, the string itself having
// been read.
//
//	literal ::= STRING_LITERAL_QUOTE (('^^' IRIREF) | LANG_DIR)?
//
// Both the LANG_DIR and the datatype are optional and mutually exclusive, so a
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
	case TokenLangDir:
		p.discard()
		literal.LangPos = tok.Pos
		literal.Language, literal.Direction = splitLangDir(string(tok.Value))
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

// splitLangDir separates a LANG_DIR into the language tag and the base
// direction that may follow it.
//
//	LANG_DIR ::= '@' [a-zA-Z]+ ('-' [a-zA-Z0-9]+)* ('--' [a-zA-Z]+)?
//
// The "--" appears at most once and only before the direction: every other
// hyphen the production allows is followed by at least one alphanumeric
// character, so no subtag can end in one and produce a second "--". Cutting at
// the first is therefore cutting at the only one.
func splitLangDir(value string) (language, direction string) {
	language, direction, _ = strings.Cut(value, "--")
	return language, direction
}
