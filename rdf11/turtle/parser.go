package turtle

import (
	"fmt"
	"io"
	"iter"
	"slices"
	"strings"
)

// UnexpectedTokenError is reported when a token appears where the grammar does
// not allow it.
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

// Parse reads the Turtle document in r and returns its syntax tree.
//
// The tree keeps what the document wrote rather than what it means. A prefixed
// name stays a prefix and a local name; the "a" keyword stays itself; a
// collection stays a list and a blank node property list stays a list of
// predicates. None of it is expanded into the triples it stands for, because a
// printer given the expansion could only write the expansion back, and the
// document said something shorter.
//
// Every node carries the position it was written at, and comments are kept so
// a printer can put them back.
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
// [Document]; [Decode] lowers each statement and hands its triples on the
// moment the statement is complete, keeping nothing but the directives still in
// scope, which is what lets it read a document larger than memory. Splitting
// the grammar in two instead would leave two things to keep in step with the
// specification.
type sink interface {
	// statement receives a parsed statement. Returning an error stops parsing,
	// and the error is what parse reports.
	statement(Statement) error

	// comment receives a comment at pos, with text as the tokenizer read it.
	//
	// The text is passed raw, and is only good until the next token is read.
	// An implementation that keeps a comment has to copy it, and one that
	// drops it — which streaming does — then pays nothing for a comment it was
	// never going to look at.
	comment(pos Pos, text []byte)
}

// statement appends the statement to the document.
func (d *Document) statement(s Statement) error {
	d.Statements = append(d.Statements, s)
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

	p := &parser{next: next, pos: Pos{Line: 1, Column: 1}, sink: s}

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
	sink    sink
}

// pull takes the next token that is not a comment, keeping the comments it
// passes over.
//
// A comment is white space in Turtle and may stand anywhere between two
// terminals — between an object and the '.' that ends its statement as
// readily as between statements. Skipping them here rather than in the rules
// means no rule has to allow for one, and none can forget to.
func (p *parser) pull() (Token, error, bool) {
	for {
		tok, err, ok := p.next()
		if err != nil || !ok {
			return tok, err, ok
		}
		if tok.Type != TokenComment {
			return tok, nil, true
		}
		p.sink.comment(tok.Pos, tok.Value)
	}
}

func (p *parser) read() (Token, error, bool) {
	if p.pending != nil {
		tok := *p.pending
		p.pending = nil
		p.pos = tok.Pos
		return tok, nil, true
	}

	tok, err, ok := p.pull()
	if ok && err == nil {
		p.pos = tok.Pos
	}
	return tok, err, ok
}

func (p *parser) peek() (Token, error, bool) {
	if p.pending != nil {
		return *p.pending, nil, true
	}

	tok, err, ok := p.pull()
	if err != nil {
		return Token{}, err, false
	}
	if !ok {
		return Token{}, nil, false
	}
	p.pending = &tok
	return tok, nil, true
}

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

// parseDocument either ends the document or reads the next statement.
//
//	turtleDoc ::= statement*
//
// Comments need no handling here: the parser passes over them wherever they
// stand, which is anywhere at all.
func parseDocument(p *parser, s sink) (parserAction[sink], error) {
	_, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return parseStatement, nil
}

// parseStatement reads one directive or one set of triples.
//
//	statement ::= directive | triples '.'
func parseStatement(p *parser, s sink) (parserAction[sink], error) {
	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	var statement Statement
	switch tok.Type {
	case TokenPrefix:
		statement, err = parsePrefixDirective(p)
	case TokenBase:
		statement, err = parseBaseDirective(p)
	default:
		statement, err = parseTriples(p)
	}
	if err != nil {
		return nil, err
	}

	if err := s.statement(statement); err != nil {
		return nil, err
	}
	return parseDocument, nil
}

// parsePrefixDirective reads a prefix binding in either of its forms.
//
//	prefixID     ::= '@prefix' PNAME_NS IRIREF '.'
//	sparqlPrefix ::= "PREFIX" PNAME_NS IRIREF
//
// Only the '@' form takes a trailing '.', which is why the keyword is kept:
// what may follow depends on which was written.
func parsePrefixDirective(p *parser) (*PrefixDirective, error) {
	keyword, err := p.expect(TokenPrefix)
	if err != nil {
		return nil, err
	}

	name, err := p.expect(TokenPNameNS)
	if err != nil {
		return nil, err
	}

	iri, err := p.expect(TokenIRIRef)
	if err != nil {
		return nil, err
	}

	directive := &PrefixDirective{
		Pos:     keyword.Pos,
		Keyword: string(keyword.Value),
		Prefix:  string(name.Value),
		IRI:     &IRIRef{Pos: iri.Pos, Value: string(iri.Value)},
	}

	if isAtDirective(keyword) {
		if _, err := p.expect(TokenDot); err != nil {
			return nil, err
		}
	}
	return directive, nil
}

// parseBaseDirective reads a base declaration in either of its forms.
//
//	base       ::= '@base' IRIREF '.'
//	sparqlBase ::= "BASE" IRIREF
func parseBaseDirective(p *parser) (*BaseDirective, error) {
	keyword, err := p.expect(TokenBase)
	if err != nil {
		return nil, err
	}

	iri, err := p.expect(TokenIRIRef)
	if err != nil {
		return nil, err
	}

	directive := &BaseDirective{
		Pos:     keyword.Pos,
		Keyword: string(keyword.Value),
		IRI:     &IRIRef{Pos: iri.Pos, Value: string(iri.Value)},
	}

	if isAtDirective(keyword) {
		if _, err := p.expect(TokenDot); err != nil {
			return nil, err
		}
	}
	return directive, nil
}

// isAtDirective reports whether the keyword was written with an '@', which is
// the form the grammar ends with a '.'.
func isAtDirective(keyword Token) bool {
	return len(keyword.Value) > 0 && keyword.Value[0] == '@'
}

// parseTriples reads a subject and everything said about it.
//
//	triples ::= subject predicateObjectList
//	          | blankNodePropertyList predicateObjectList?
//
// A blank node property list has already said something by the time it closes,
// so what follows it may be nothing at all.
func parseTriples(p *parser) (*Triples, error) {
	subject, err := parseSubject(p)
	if err != nil {
		return nil, err
	}

	triples := &Triples{Pos: subject.Position(), Subject: subject}

	_, isPropertyList := subject.(*BlankNodePropertyList)
	if isPropertyList {
		tok, err, ok := p.peek()
		if err != nil {
			return nil, err
		}
		if ok && tok.Type == TokenDot {
			p.discard()
			return triples, nil
		}
	}

	triples.Predicates, err = parsePredicateObjectList(p)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(TokenDot); err != nil {
		return nil, err
	}
	return triples, nil
}

// parseSubject reads what a statement is about.
//
//	subject ::= iri | BlankNode | collection
//
// A blank node property list is admitted too, the grammar's second form of
// triples putting one there.
func parseSubject(p *parser) (Term, error) {
	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, UnexpectedEndOfTokensError{Expected: subjectTokens, Pos: p.pos}
	}

	switch tok.Type {
	case TokenOpenParen:
		return parseCollection(p)
	case TokenOpenBracket:
		return parseBlankNodePropertyList(p)
	case TokenIRIRef, TokenPNameNS, TokenPNameLN, TokenBlankNodeLabel, TokenAnon:
		p.discard()
		return simpleTerm(tok), nil
	default:
		return nil, UnexpectedTokenError{Expected: subjectTokens, Actual: tok}
	}
}

var subjectTokens = []TokenType{
	TokenIRIRef, TokenPNameNS, TokenPNameLN,
	TokenBlankNodeLabel, TokenAnon, TokenOpenParen, TokenOpenBracket,
}

// simpleTerm builds the node for a term that is one token and no more.
func simpleTerm(tok Token) Term {
	switch tok.Type {
	case TokenIRIRef:
		return &IRIRef{Pos: tok.Pos, Value: string(tok.Value)}
	case TokenPNameNS:
		return &PrefixedName{Pos: tok.Pos, Prefix: string(tok.Value)}
	case TokenPNameLN:
		prefix, local, _ := strings.Cut(string(tok.Value), ":")
		return &PrefixedName{Pos: tok.Pos, Prefix: prefix, Local: local, HasLocal: true}
	case TokenBlankNodeLabel:
		return &BlankNode{Pos: tok.Pos, Label: string(tok.Value)}
	case TokenAnon:
		return &Anon{Pos: tok.Pos}
	default:
		panic(fmt.Sprintf("not a one-token term: %s", tok.Type))
	}
}

// parsePredicateObjectList reads the verbs and objects given for a subject.
//
//	predicateObjectList ::= verb objectList (';' (verb objectList)?)*
//
// A ';' may be followed by another verb or by nothing, which is what lets a
// document end a list with a trailing semicolon.
func parsePredicateObjectList(p *parser) ([]*PredicateObject, error) {
	var list []*PredicateObject

	for {
		verb, err := parseVerb(p)
		if err != nil {
			return nil, err
		}

		objects, err := parseObjectList(p)
		if err != nil {
			return nil, err
		}
		list = append(list, &PredicateObject{Pos: verb.Position(), Verb: verb, Objects: objects})

		// Any number of semicolons may follow, and the list ends when what
		// comes after one is not a verb.
		var sawSemicolon bool
		for {
			tok, err, ok := p.peek()
			if err != nil {
				return nil, err
			}
			if !ok || tok.Type != TokenSemicolon {
				break
			}
			p.discard()
			sawSemicolon = true
		}
		if !sawSemicolon {
			return list, nil
		}

		tok, err, ok := p.peek()
		if err != nil {
			return nil, err
		}
		if !ok || !isVerbToken(tok.Type) {
			return list, nil
		}
	}
}

func isVerbToken(t TokenType) bool {
	switch t {
	case TokenIRIRef, TokenPNameNS, TokenPNameLN, TokenA:
		return true
	default:
		return false
	}
}

// parseVerb reads what stands in the predicate position.
//
//	verb ::= predicate | 'a'
func parseVerb(p *parser) (Verb, error) {
	tok, err := p.expect(TokenIRIRef, TokenPNameNS, TokenPNameLN, TokenA)
	if err != nil {
		return nil, err
	}

	if tok.Type == TokenA {
		return &A{Pos: tok.Pos}, nil
	}
	return simpleTerm(tok).(Verb), nil
}

// parseObjectList reads the objects given for one verb.
//
//	objectList ::= object (',' object)*
func parseObjectList(p *parser) ([]Term, error) {
	var objects []Term

	for {
		object, err := parseObject(p)
		if err != nil {
			return nil, err
		}
		objects = append(objects, object)

		tok, err, ok := p.peek()
		if err != nil {
			return nil, err
		}
		if !ok || tok.Type != TokenComma {
			return objects, nil
		}
		p.discard()
	}
}

// parseObject reads one object.
//
//	object ::= iri | BlankNode | collection | blankNodePropertyList | literal
func parseObject(p *parser) (Term, error) {
	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, UnexpectedEndOfTokensError{Expected: objectTokens, Pos: p.pos}
	}

	switch tok.Type {
	case TokenOpenParen:
		return parseCollection(p)
	case TokenOpenBracket:
		return parseBlankNodePropertyList(p)
	case TokenIRIRef, TokenPNameNS, TokenPNameLN, TokenBlankNodeLabel, TokenAnon:
		p.discard()
		return simpleTerm(tok), nil
	case TokenString:
		return parseRDFLiteral(p)
	case TokenInteger, TokenDecimal, TokenDouble, TokenBoolean:
		p.discard()
		return numericOrBooleanLiteral(tok), nil
	default:
		return nil, UnexpectedTokenError{Expected: objectTokens, Actual: tok}
	}
}

var objectTokens = []TokenType{
	TokenIRIRef, TokenPNameNS, TokenPNameLN, TokenBlankNodeLabel, TokenAnon,
	TokenOpenParen, TokenOpenBracket, TokenString,
	TokenInteger, TokenDecimal, TokenDouble, TokenBoolean,
}

// numericOrBooleanLiteral builds the node for a literal written as itself
// rather than in quotes.
//
//	NumericLiteral ::= INTEGER | DECIMAL | DOUBLE
//	BooleanLiteral ::= 'true' | 'false'
//
// The datatype each implies is left off: the tree records that the document
// wrote 42, not that 42 means "42"^^xsd:integer.
func numericOrBooleanLiteral(tok Token) *Literal {
	kind := LiteralInteger
	switch tok.Type {
	case TokenDecimal:
		kind = LiteralDecimal
	case TokenDouble:
		kind = LiteralDouble
	case TokenBoolean:
		kind = LiteralBoolean
	}
	return &Literal{Pos: tok.Pos, Kind: kind, Value: string(tok.Value)}
}

// parseRDFLiteral reads a quoted string and whatever qualifies it.
//
//	RDFLiteral ::= String (LANGTAG | '^^' iri)?
//
// The two are alternatives, so a string carries a language tag or a datatype
// or neither, and never both.
func parseRDFLiteral(p *parser) (*Literal, error) {
	str, err := p.expect(TokenString)
	if err != nil {
		return nil, err
	}

	literal := &Literal{Pos: str.Pos, Kind: LiteralString, Value: string(str.Value)}

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
		literal.LangPos, literal.Language = tok.Pos, string(tok.Value)
	case TokenDatatypeMarker:
		p.discard()

		datatype, err := p.expect(TokenIRIRef, TokenPNameNS, TokenPNameLN)
		if err != nil {
			return nil, err
		}
		literal.Datatype = simpleTerm(datatype)
	}
	return literal, nil
}

// parseCollection reads a list written in parentheses.
//
//	collection ::= '(' object* ')'
//
// An empty collection is legal and stands for rdf:nil, which is why Objects
// may be empty rather than the parentheses being an error.
func parseCollection(p *parser) (*Collection, error) {
	open, err := p.expect(TokenOpenParen)
	if err != nil {
		return nil, err
	}

	collection := &Collection{Pos: open.Pos}
	for {
		tok, err, ok := p.peek()
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, UnexpectedEndOfTokensError{
				Expected: append(slices.Clone(objectTokens), TokenCloseParen),
				Pos:      p.pos,
			}
		}
		if tok.Type == TokenCloseParen {
			p.discard()
			return collection, nil
		}

		object, err := parseObject(p)
		if err != nil {
			return nil, err
		}
		collection.Objects = append(collection.Objects, object)
	}
}

// parseBlankNodePropertyList reads a blank node described in place.
//
//	blankNodePropertyList ::= '[' predicateObjectList ']'
//
// The list is required: a pair of brackets with nothing between them is an
// ANON, which the tokenizer has already recognised as one.
func parseBlankNodePropertyList(p *parser) (*BlankNodePropertyList, error) {
	open, err := p.expect(TokenOpenBracket)
	if err != nil {
		return nil, err
	}

	predicates, err := parsePredicateObjectList(p)
	if err != nil {
		return nil, err
	}

	if _, err := p.expect(TokenCloseBracket); err != nil {
		return nil, err
	}
	return &BlankNodePropertyList{Pos: open.Pos, Predicates: predicates}, nil
}
