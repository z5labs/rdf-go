// This file began as a copy of rdf11/ntriples/tokenizer.go and keeps its
// structure: the same tokenizer struct and reading primitives, the same
// tokenizerAction combinators, and the same rules for IRIREF, BLANK_NODE_LABEL,
// LANGTAG, UCHAR, ECHAR and the PN_CHARS classes.
//
// The last two of those are no longer written here: the PN_CHARS classes and
// the escape forms all four syntaxes share come from internal/lex (#22). Pos,
// Token and the error types stay, being what this package's callers name, and
// so do the rules themselves, a rule being where two grammars may differ even
// when the characters it reads are the same.
//
// What Turtle adds is everything else in this file — prefixed names, four
// string forms, numbers, directives and punctuation — and one thing it takes
// away: a line ending is white space here, not a terminal, so there is no EOL
// token to emit.

package turtle

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"iter"

	"github.com/z5labs/rdf-go/internal/lex"
)

// Pos is the position of a character in the input, counted in lines and in
// characters within a line, both starting at 1.
//
// A column counts characters rather than bytes, so the column of a token
// following a non-ASCII IRI is the one an editor would show.
type Pos struct {
	Line   int
	Column int
}

// String renders the position as "line:column".
func (p Pos) String() string {
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

// Token is one terminal of the N-Triples grammar, with the position of its
// first character.
//
// Value carries the token's meaning rather than its source text: the
// delimiters that identify a terminal are stripped, and the escape sequences
// inside one are decoded. An IRIREF written <http://example.com/ab> has
// the value "http://example.com/ab", and the string literal "a\nb" has a value
// three characters long. A [TokenComment] is the exception, keeping its '#'
// and its text exactly as written, there being nothing in a comment to decode.
type Token struct {
	Pos   Pos
	Type  TokenType
	Value []byte
}

// String renders the token as its type and value, for test output and
// debugging.
func (t Token) String() string {
	return fmt.Sprintf("%s(%s)", t.Type, t.Value)
}

// TokenType is the kind of a [Token], one per terminal of the N-Triples
// grammar (https://www.w3.org/TR/n-triples/#sec-grammar).
type TokenType int

const (
	// TokenIRIRef is an IRIREF, written <...>. Its value is the IRI, without
	// the angle brackets and with any UCHAR decoded.
	TokenIRIRef TokenType = iota

	// TokenPNameNS is a PNAME_NS, a prefix and its colon with no local name:
	// "foaf:" or the bare ":". Its value is the prefix, without the colon.
	TokenPNameNS

	// TokenPNameLN is a PNAME_LN, a prefixed name: "foaf:name". Its value is
	// the whole of it, colon included, with every PN_LOCAL_ESC resolved.
	TokenPNameLN

	// TokenBlankNodeLabel is a BLANK_NODE_LABEL, written _:name. Its value is
	// the label, without the leading "_:".
	TokenBlankNodeLabel

	// TokenAnon is an ANON, written [] with nothing but white space between
	// the brackets. It stands for a blank node the document does not name.
	TokenAnon

	// TokenString is a string literal in any of the four forms Turtle allows:
	// "...", '...', """...""" and \'\'\'...\'\'\'. Its value is the string, without
	// the quotes and with every ECHAR and UCHAR decoded, so which form was
	// used is not recorded — the four differ in what they can hold, not in
	// what they mean.
	TokenString

	// TokenLangTag is a LANGTAG, written @en-GB. Its value is the tag, without
	// the leading '@'.
	TokenLangTag

	// TokenInteger is an INTEGER, written 1 or -42.
	TokenInteger

	// TokenDecimal is a DECIMAL, written 1.0 or -.5: a number with a decimal
	// point and no exponent.
	TokenDecimal

	// TokenDouble is a DOUBLE, written 1e6 or -1.0E-3: a number with an
	// exponent.
	TokenDouble

	// TokenA is the keyword "a", which abbreviates rdf:type in the predicate
	// position.
	TokenA

	// TokenBoolean is a BooleanLiteral, written true or false.
	//
	//	BooleanLiteral ::= 'true' | 'false'
	//
	// Unlike the SPARQL-style directives, the two are case-sensitive: "TRUE"
	// is a name the grammar has no use for, not a boolean.
	TokenBoolean

	// TokenPrefix is a prefix directive, written either "@prefix" or the
	// SPARQL-style "PREFIX". Its value is the keyword as written, which is
	// what tells the two apart: only the first is ended by a '.'.
	TokenPrefix

	// TokenBase is a base directive, written either "@base" or the
	// SPARQL-style "BASE". Its value is the keyword as written.
	TokenBase

	// TokenDatatypeMarker is the "^^" that introduces a literal's datatype.
	TokenDatatypeMarker

	// TokenDot is the "." that ends a triple or a directive.
	TokenDot

	// TokenSemicolon is the ";" that repeats a subject.
	TokenSemicolon

	// TokenComma is the "," that repeats a subject and a predicate.
	TokenComma

	// TokenOpenParen is the "(" that opens a collection.
	TokenOpenParen

	// TokenCloseParen is the ")" that closes one.
	TokenCloseParen

	// TokenOpenBracket is the "[" that opens a blank node property list. An
	// empty pair of brackets is a [TokenAnon] instead.
	TokenOpenBracket

	// TokenCloseBracket is the "]" that closes one.
	TokenCloseBracket

	// TokenComment is a comment, from '#' to the end of the line. Its value is
	// the whole comment, '#' included. The grammar treats a comment as white
	// space, but it is emitted as a token so that a printer can put it back.
	TokenComment
)

// String returns the name of the token type.
func (tt TokenType) String() string {
	switch tt {
	case TokenIRIRef:
		return "IRIRef"
	case TokenPNameNS:
		return "PNameNS"
	case TokenPNameLN:
		return "PNameLN"
	case TokenBlankNodeLabel:
		return "BlankNodeLabel"
	case TokenAnon:
		return "Anon"
	case TokenString:
		return "String"
	case TokenLangTag:
		return "LangTag"
	case TokenInteger:
		return "Integer"
	case TokenDecimal:
		return "Decimal"
	case TokenDouble:
		return "Double"
	case TokenA:
		return "A"
	case TokenBoolean:
		return "Boolean"
	case TokenPrefix:
		return "Prefix"
	case TokenBase:
		return "Base"
	case TokenDatatypeMarker:
		return "DatatypeMarker"
	case TokenDot:
		return "Dot"
	case TokenSemicolon:
		return "Semicolon"
	case TokenComma:
		return "Comma"
	case TokenOpenParen:
		return "OpenParen"
	case TokenCloseParen:
		return "CloseParen"
	case TokenOpenBracket:
		return "OpenBracket"
	case TokenCloseBracket:
		return "CloseBracket"
	case TokenComment:
		return "Comment"
	default:
		panic(fmt.Sprintf("unknown token type: %d", tt))
	}
}

// UnexpectedCharacterError is reported when a character cannot appear where it
// does: at the start of a term, inside an IRIREF, or after the backslash of an
// escape sequence.
//
// Pos is the position of the offending character itself.
type UnexpectedCharacterError struct {
	Pos Pos
	R   rune
}

// Error implements the [error] interface.
func (e UnexpectedCharacterError) Error() string {
	return fmt.Sprintf("unexpected character %q at line %d, column %d", e.R, e.Pos.Line, e.Pos.Column)
}

// UnterminatedStringError is reported when a string literal reaches the end of
// its line, or the end of the input, without a closing quote. A literal may
// not span lines: the grammar excludes a raw line feed and carriage return
// from STRING_LITERAL_QUOTE, which are written \n and \r instead.
//
// Pos is the position of the opening quote, which is where the reader has to
// look to fix it.
type UnterminatedStringError struct {
	Pos Pos
}

// Error implements the [error] interface.
func (e UnterminatedStringError) Error() string {
	return fmt.Sprintf("unterminated string literal at line %d, column %d", e.Pos.Line, e.Pos.Column)
}

// UnterminatedIRIRefError is reported when an IRIREF reaches the end of its
// line, or the end of the input, without a closing '>'.
//
// Pos is the position of the opening '<'.
type UnterminatedIRIRefError struct {
	Pos Pos
}

// Error implements the [error] interface.
func (e UnterminatedIRIRefError) Error() string {
	return fmt.Sprintf("unterminated IRI at line %d, column %d", e.Pos.Line, e.Pos.Column)
}

// UnexpectedEndOfInputError is reported when the input ends part way through a
// terminal, at a point where the grammar still required a character: a
// blank node label with nothing after its "_:", a language tag left hanging on
// a hyphen, a UCHAR short of its digits.
//
// Ending between terminals is not this error, or any error: that is simply the
// end of the document.
//
// Pos is where the missing character would have been.
type UnexpectedEndOfInputError struct {
	Pos Pos
}

// Error implements the [error] interface.
func (e UnexpectedEndOfInputError) Error() string {
	return fmt.Sprintf("unexpected end of input at line %d, column %d", e.Pos.Line, e.Pos.Column)
}

// InvalidCodePointError is reported when a UCHAR names something that is not a
// character: a value above the highest code point, or one of the surrogates,
// which exist only to encode other code points in UTF-16 and stand for nothing
// on their own.
//
// Pos is the position of the backslash that began the escape.
type InvalidCodePointError struct {
	Pos  Pos
	Code uint32
}

// Error implements the [error] interface.
func (e InvalidCodePointError) Error() string {
	return fmt.Sprintf("escape names no character, U+%04X, at line %d, column %d", e.Code, e.Pos.Line, e.Pos.Column)
}

// EscapedIRICharacterError is reported when a UCHAR inside an IRIREF names a
// character the production excludes — a space, a '<', a '>'. An escape is a
// way of spelling a character, not a way of getting one past the grammar, so
// \u0020 is refused exactly where a space is.
//
// Pos is the position of the backslash that began the escape, which is what
// has to be rewritten: a space in an IRI is written %20.
type EscapedIRICharacterError struct {
	Pos Pos
	R   rune
}

// Error implements the [error] interface.
func (e EscapedIRICharacterError) Error() string {
	return fmt.Sprintf("escape names %q, which an IRI may not hold, at line %d, column %d", e.R, e.Pos.Line, e.Pos.Column)
}

// Tokenize reads the N-Triples document in r and yields its tokens in order.
//
// Iteration stops at the end of the input, and at the first error — an error
// is yielded with the zero [Token], and nothing follows it. A caller ranging
// over the sequence must therefore check the error on every step:
//
//	for token, err := range ntriples.Tokenize(r) {
//		if err != nil {
//			return err
//		}
//		// ...
//	}
//
// White space between terminals is dropped, that being all the grammar makes
// of a tab or a space. A line ending is not white space and is yielded as
// [TokenEOL], and a comment is yielded as [TokenComment] rather than dropped,
// so that everything but indentation survives a read and a write.
func Tokenize(r io.Reader) iter.Seq2[Token, error] {
	return func(yield func(Token, error) bool) {
		t := &tokenizer{
			pos: Pos{Line: 1, Column: 1},
			buf: bufio.NewReader(r),
		}

		for action := tokenizeDocument; action != nil; {
			action = action(t, yield)
		}
	}
}

// tokenizer holds the reader and the position within it. Everything else the
// tokenizer needs to remember is held by the action it is about to run, which
// is what makes the actions readable as grammar rules.
type tokenizer struct {
	pos Pos
	buf *bufio.Reader

	// afterCR records that the last character was a carriage return, so that
	// the line feed of a CRLF pair does not count as a second line ending.
	afterCR bool
}

// next reads one character and advances the position past it.
func (t *tokenizer) next() (rune, error) {
	r, _, err := t.buf.ReadRune()
	if err != nil {
		return 0, err
	}
	t.advance(r)
	return r, nil
}

// advance moves the position past r.
func (t *tokenizer) advance(r rune) {
	switch {
	case r == '\n' && t.afterCR:
		// The second half of a CRLF pair. The line ended at the carriage
		// return; this only finishes it.
		t.afterCR = false
	case r == '\n' || r == '\r':
		t.pos.Line++
		t.pos.Column = 1
		t.afterCR = r == '\r'
	default:
		t.pos.Column++
		t.afterCR = false
	}
}

// mark is everything reading a character changes, and so everything undoing
// one has to put back.
//
// The carriage return flag belongs here as much as the position does. Reading
// the line feed of a CRLF pair clears it, so a backup that restored only the
// position would leave the tokenizer believing the next line feed begins a
// line of its own, and count the line twice.
type mark struct {
	pos     Pos
	afterCR bool
}

// mark records where the tokenizer is, so that backup can return to it.
func (t *tokenizer) mark() mark {
	return mark{pos: t.pos, afterCR: t.afterCR}
}

// backup returns the last character read to the reader and undoes what reading
// it changed.
func (t *tokenizer) backup(m mark) error {
	if err := t.buf.UnreadRune(); err != nil {
		return err
	}
	t.pos = m.pos
	t.afterCR = m.afterCR
	return nil
}

// peek reports what the next character is without consuming it, and whether
// there was one.
func (t *tokenizer) peek() (rune, bool, error) {
	previous := t.mark()

	r, err := t.next()
	if errors.Is(err, io.EOF) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return r, true, t.backup(previous)
}

// copyIf copies characters satisfying cond into dst, stopping at the first
// that does not and leaving it unread.
//
// Reaching the end of the input is reported as [io.ErrUnexpectedEOF], which
// tells the caller that whatever it was reading ran out rather than ended.
// Whether that is an error or simply the end of the document is for the caller
// to say.
func (t *tokenizer) copyIf(dst *bytes.Buffer, cond func(rune) bool) error {
	for {
		r, _, err := t.buf.ReadRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return io.ErrUnexpectedEOF
			}
			return err
		}

		if !cond(r) {
			return t.buf.UnreadRune()
		}
		if _, err := dst.WriteRune(r); err != nil {
			return err
		}
		t.advance(r)
	}
}

// copyAtLeastOne is copyIf where the grammar writes '+' rather than '*': it
// reports [UnexpectedCharacterError] if not even the first character
// qualifies, and [UnexpectedEndOfInputError] if there is no first character at
// all.
//
// That second error is what separates a document that ended from one that was
// cut off. Running out here is not the end of anything — a terminal was part
// way through and the grammar was still asking for a character — so it must
// not be quietly taken for the end of the input.
func (t *tokenizer) copyAtLeastOne(dst *bytes.Buffer, cond func(rune) bool) error {
	pos := t.pos

	r, err := t.next()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return UnexpectedEndOfInputError{Pos: pos}
		}
		return err
	}
	if !cond(r) {
		return UnexpectedCharacterError{Pos: pos, R: r}
	}
	if _, err := dst.WriteRune(r); err != nil {
		return err
	}

	return t.copyIf(dst, cond)
}

// skipIf discards characters satisfying cond, stopping at the first that does
// not and leaving it unread.
func (t *tokenizer) skipIf(cond func(rune) bool) error {
	for {
		r, _, err := t.buf.ReadRune()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return io.ErrUnexpectedEOF
			}
			return err
		}

		if !cond(r) {
			return t.buf.UnreadRune()
		}
		t.advance(r)
	}
}

// tokenizerAction is one step of tokenizing: it consumes some input, may yield
// a token or an error, and returns the step to run next, or nil to stop.
//
// Writing the tokenizer as actions that return actions rather than as a loop
// over a state variable means each rule of the grammar reads as a function
// whose return value says what may legally follow it.
type tokenizerAction func(t *tokenizer, yield func(Token, error) bool) tokenizerAction

// yieldErrorOr runs next if err is nil, and otherwise reports err and stops.
//
// [io.ErrUnexpectedEOF] is not reported: it means the input ran out, which at
// the level of a whole document is simply the end of it. A rule for which
// running out really is an error turns it into a more specific one before
// handing it here.
func yieldErrorOr(err error, next tokenizerAction) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		if err == nil {
			return next
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil
		}
		if !yield(Token{}, err) {
			return nil
		}
		return nil
	}
}

// yieldTokenThen emits tok and runs next, stopping if the caller has stopped
// ranging.
func yieldTokenThen(tok Token, next tokenizerAction) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		if !yield(tok, nil) {
			return nil
		}
		return next
	}
}

// yieldFinalToken emits tok and stops if err says the input ran out, and
// otherwise emits it and carries on with next.
func yieldFinalToken(err error, tok Token, next tokenizerAction) tokenizerAction {
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return yieldTokenThen(tok, nil)
	}
	return yieldErrorOr(err, yieldTokenThen(tok, next))
}

// skipSpace discards the white space before the next terminal and then runs
// next.
//
//	WS ::= #x20 | #x9 | #xD | #xA
//
// A line ending is white space here, where N-Triples makes it a terminal.
// Turtle statements end at a '.' and may be laid out however the author likes,
// so nothing downstream needs to be told where the lines were — only the
// positions record that.
func skipSpace(next tokenizerAction) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		return yieldErrorOr(t.skipIf(isWhitespace), next)
	}
}

func isWhitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n'
}

// tokenizeIRIRef reads the rest of an IRIREF, the '<' having been consumed:
//
//	IRIREF ::= '<' ([^#x00-#x20<>"{}|^`\] | UCHAR)* '>'
//
// The excluded characters have no escape of their own inside an IRI, so a
// backslash here can only begin a UCHAR.
func tokenizeIRIRef(pos Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		var iri bytes.Buffer

		for {
			charPos := t.pos

			r, err := t.next()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return yieldErrorOr(UnterminatedIRIRefError{Pos: pos}, nil)
				}
				return yieldErrorOr(err, nil)
			}

			switch {
			case r == '>':
				return yieldTokenThen(
					Token{Pos: pos, Type: TokenIRIRef, Value: iri.Bytes()},
					tokenizeDocument,
				)
			case r == '\n' || r == '\r':
				// An IRI cannot span lines, so the '>' is missing rather than
				// this character being merely unexpected.
				return yieldErrorOr(UnterminatedIRIRefError{Pos: pos}, nil)
			case r == '\\':
				decoded, err := t.readEscape(charPos, false)
				if err != nil {
					return yieldErrorOr(err, nil)
				}
				if !lex.IsIRIChar(decoded) {
					// A UCHAR spells a character; it does not exempt one. The
					// characters IRIREF leaves out are left out however they
					// are written.
					return yieldErrorOr(EscapedIRICharacterError{Pos: charPos, R: decoded}, nil)
				}
				iri.WriteRune(decoded)
			case !lex.IsIRIChar(r):
				// The second '<' of RDF 1.2's "<<(" is refused here.
				return yieldErrorOr(UnexpectedCharacterError{Pos: charPos, R: r}, nil)
			default:
				iri.WriteRune(r)
			}
		}
	}
}

// readEscape reads the escape sequence following a backslash and returns the
// character it denotes. The backslash is at pos.
//
//	UCHAR ::= '\u' HEX HEX HEX HEX | '\U' HEX HEX HEX HEX HEX HEX HEX HEX
//	ECHAR ::= '\' [tbnrf"'\]
//
// An IRIREF admits only the first of the two, so echar says which caller this
// is. Anything else after the backslash is an unexpected character, which is
// how an unknown escape is reported.
//
// [lex.Escape] is what decodes the sequence. What this adds is the two
// positions lex does not track: the backslash, for an escape that names no
// character, and the character last read, for one that cannot appear where it
// does.
func (t *tokenizer) readEscape(pos Pos, echar bool) (rune, error) {
	next, at := t.scan()

	r, err := lex.Escape(next, echar)
	if err == nil {
		return r, nil
	}

	var invalid lex.InvalidCodePointError
	if errors.As(err, &invalid) {
		return 0, InvalidCodePointError{Pos: pos, Code: invalid.Code}
	}
	return 0, positioned(err, *at)
}

// scan returns a [lex.NextFunc] reading from t, together with the position of
// the character it last read.
//
// That position is what internal/lex gives up by not reading the input itself:
// it hands back the character it could not use and leaves saying where that
// character stood to the caller. Reading is also where the input can run out,
// which is this package's error to report rather than lex's, so the conversion
// happens here too.
func (t *tokenizer) scan() (lex.NextFunc, *Pos) {
	at := new(Pos)

	return func() (rune, error) {
		*at = t.pos

		r, err := t.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return 0, UnexpectedEndOfInputError{Pos: *at}
			}
			return 0, err
		}
		return r, nil
	}, at
}

// positioned reports an error from internal/lex as this package's own. lex
// hands back the character it could not use without saying where that
// character stood; at is where it stood.
func positioned(err error, at Pos) error {
	var unexpected lex.UnexpectedCharacterError
	if errors.As(err, &unexpected) {
		return UnexpectedCharacterError{Pos: at, R: unexpected.R}
	}
	return err
}

// tokenizeBlankNodeLabel reads the rest of a BLANK_NODE_LABEL, the '_' having
// been consumed:
//
//	BLANK_NODE_LABEL ::= '_:' (PN_CHARS_U | [0-9]) ((PN_CHARS | '.')* PN_CHARS)?
//
// The label may contain a dot but may not end with one, which is what keeps
// "_:b0." from swallowing the dot that ends the triple. A run of dots is only
// part of the label if a label character follows it; the run that does not is
// emitted as the [TokenDot] it turns out to be.
func tokenizeBlankNodeLabel(pos Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		colonPos := t.pos

		r, err := t.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return yieldErrorOr(UnexpectedEndOfInputError{Pos: colonPos}, nil)
			}
			return yieldErrorOr(err, nil)
		}
		if r != ':' {
			return yieldErrorOr(UnexpectedCharacterError{Pos: colonPos, R: r}, nil)
		}

		var label bytes.Buffer
		err = t.copyAtLeastOne(&label, func(r rune) bool {
			return lex.IsPNCharsU(r) || (r >= '0' && r <= '9')
		})
		if err != nil {
			return yieldFinalToken(
				err,
				Token{Pos: pos, Type: TokenBlankNodeLabel, Value: label.Bytes()},
				tokenizeDocument,
			)
		}

		// Dots are held back until a label character justifies them.
		var trailing []Pos
		for {
			before := t.mark()

			r, err := t.next()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return yieldErrorOr(err, nil)
			}

			switch {
			case r == '.':
				trailing = append(trailing, before.pos)
			case lex.IsPNChars(r):
				for range trailing {
					label.WriteRune('.')
				}
				trailing = trailing[:0]
				label.WriteRune(r)
			default:
				if err := t.backup(before); err != nil {
					return yieldErrorOr(err, nil)
				}
				return blankNodeLabelThen(pos, label.Bytes(), trailing)
			}
		}
		return blankNodeLabelThen(pos, label.Bytes(), trailing)
	}
}

// blankNodeLabelThen emits the label, then the dots that turned out not to
// belong to it, and then carries on with the document.
func blankNodeLabelThen(pos Pos, label []byte, trailing []Pos) tokenizerAction {
	next := tokenizeDocument
	for i := len(trailing) - 1; i >= 0; i-- {
		next = yieldTokenThen(Token{Pos: trailing[i], Type: TokenDot, Value: []byte(".")}, next)
	}
	return yieldTokenThen(Token{Pos: pos, Type: TokenBlankNodeLabel, Value: label}, next)
}

func isAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

func isAlphanumeric(r rune) bool {
	return isAlpha(r) || (r >= '0' && r <= '9')
}

// tokenizeDatatypeMarker reads the second '^' of the "^^" that introduces a
// literal's datatype, the first having been consumed.
func tokenizeDatatypeMarker(pos Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		secondPos := t.pos

		r, err := t.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return yieldErrorOr(UnexpectedEndOfInputError{Pos: secondPos}, nil)
			}
			return yieldErrorOr(err, nil)
		}
		if r != '^' {
			return yieldErrorOr(UnexpectedCharacterError{Pos: secondPos, R: r}, nil)
		}

		return yieldTokenThen(
			Token{Pos: pos, Type: TokenDatatypeMarker, Value: []byte("^^")},
			tokenizeDocument,
		)
	}
}

// tokenizeComment reads a comment, the '#' having been consumed. A comment
// runs to the end of the line, which is left for [tokenizeEOL].
func tokenizeComment(pos Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		comment := bytes.NewBuffer([]byte{'#'})

		err := t.copyIf(comment, func(r rune) bool {
			return r != '\n' && r != '\r'
		})

		return yieldFinalToken(
			err,
			Token{Pos: pos, Type: TokenComment, Value: comment.Bytes()},
			tokenizeDocument,
		)
	}
}

// UnexpectedNameError is reported when a bare name is none of the keywords
// Turtle allows there.
//
// Only "a", "PREFIX" and "BASE" may stand without a colon after them. Anything
// else that looks like a name is a prefixed name missing its colon, which is
// what this says rather than pointing at whatever followed it.
type UnexpectedNameError struct {
	Pos  Pos
	Name string
}

// Error implements the [error] interface.
func (e UnexpectedNameError) Error() string {
	return fmt.Sprintf("unexpected name %q at line %d, column %d", e.Name, e.Pos.Line, e.Pos.Column)
}
