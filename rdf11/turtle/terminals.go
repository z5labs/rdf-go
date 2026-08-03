package turtle

import (
	"bytes"
	"errors"
	"io"
	"strings"
)

// tokenizeDocument reads the next terminal of the document, dispatching on the
// character that begins it.
//
// Most terminals are settled by that character alone. Three are not, and each
// is left to the rule it hands off to: a '[' may open a property list or be
// half of an ANON, a name may turn out to be a keyword, and a number may end
// at a '.' that belongs to the statement rather than to it.
func tokenizeDocument(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
	return skipSpace(func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		pos := t.pos

		r, err := t.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return yieldErrorOr(err, nil)
		}

		switch {
		case r == '<':
			return tokenizeIRIRef(pos)
		case r == '_':
			return tokenizeBlankNodeLabel(pos)
		case r == '"' || r == '\'':
			return tokenizeString(pos, r)
		case r == '@':
			return tokenizeAtKeyword(pos)
		case r == '^':
			return tokenizeDatatypeMarker(pos)
		case r == '#':
			return tokenizeComment(pos)
		case r == '[':
			return tokenizeAnonOrOpenBracket(pos)
		case r == ':' || isPNCharsBase(r):
			return tokenizeName(pos, r)
		case r == '+' || r == '-' || isDigit(r):
			return tokenizeNumber(pos, r)
		case r == '.':
			return tokenizeDotOrDecimal(pos)
		case r == ';':
			return symbol(pos, TokenSemicolon, ";")
		case r == ',':
			return symbol(pos, TokenComma, ",")
		case r == '(':
			return symbol(pos, TokenOpenParen, "(")
		case r == ')':
			return symbol(pos, TokenCloseParen, ")")
		case r == ']':
			return symbol(pos, TokenCloseBracket, "]")
		default:
			// RDF 1.2 arrives here, or in tokenizeIRIRef: "<<(" fails on the
			// second '<', which the IRIREF production excludes.
			return yieldErrorOr(UnexpectedCharacterError{Pos: pos, R: r}, nil)
		}
	})
}

// symbol emits a one-character terminal and carries on.
func symbol(pos Pos, kind TokenType, text string) tokenizerAction {
	return yieldTokenThen(Token{Pos: pos, Type: kind, Value: []byte(text)}, tokenizeDocument)
}

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

// tokenizeString reads a string literal, its first quote having been read.
//
//	STRING_LITERAL_QUOTE        ::= '"' ([^#x22#x5C#xA#xD] | ECHAR | UCHAR)* '"'
//	STRING_LITERAL_SINGLE_QUOTE ::= "'" ([^#x27#x5C#xA#xD] | ECHAR | UCHAR)* "'"
//
// and their long forms, delimited by three quotes rather than one, which may
// hold a raw line ending and up to two of their own quote in a row.
//
// Which of the four this is comes down to the quote and whether two more
// follow it. All four decode to the same [TokenString]: they differ in what
// they can hold, not in what they mean.
func tokenizeString(pos Pos, quote rune) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		r, ok, err := t.peek()
		if err != nil {
			return yieldErrorOr(err, nil)
		}
		if !ok || r != quote {
			return tokenizeShortString(pos, quote)
		}
		if _, err := t.next(); err != nil {
			return yieldErrorOr(err, nil)
		}

		third, ok, err := t.peek()
		if err != nil {
			return yieldErrorOr(err, nil)
		}
		if !ok || third != quote {
			// Two quotes and no third: an empty short string, already
			// complete.
			return yieldTokenThen(Token{Pos: pos, Type: TokenString}, tokenizeDocument)
		}
		if _, err := t.next(); err != nil {
			return yieldErrorOr(err, nil)
		}
		return tokenizeLongString(pos, quote)
	}
}

// tokenizeShortString reads the rest of a one-quote string, which may not span
// lines.
func tokenizeShortString(pos Pos, quote rune) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		var str bytes.Buffer

		for {
			charPos := t.pos

			r, err := t.next()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return yieldErrorOr(UnterminatedStringError{Pos: pos}, nil)
				}
				return yieldErrorOr(err, nil)
			}

			switch {
			case r == quote:
				return yieldTokenThen(
					Token{Pos: pos, Type: TokenString, Value: str.Bytes()},
					tokenizeDocument,
				)
			case r == '\n' || r == '\r':
				// A short string ends with its line. The long forms are what a
				// document uses to hold one that does not.
				return yieldErrorOr(UnterminatedStringError{Pos: pos}, nil)
			case r == '\\':
				decoded, err := t.readEscape(charPos, true)
				if err != nil {
					return yieldErrorOr(err, nil)
				}
				str.WriteRune(decoded)
			default:
				str.WriteRune(r)
			}
		}
	}
}

// tokenizeLongString reads the rest of a three-quote string.
//
// Quotes are held back until something else arrives, since it takes three in a
// row to end the string and one or two are ordinary characters within it.
func tokenizeLongString(pos Pos, quote rune) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		var str bytes.Buffer
		var quotes int

		for {
			charPos := t.pos

			r, err := t.next()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return yieldErrorOr(UnterminatedStringError{Pos: pos}, nil)
				}
				return yieldErrorOr(err, nil)
			}

			if r == quote {
				quotes++
				if quotes == 3 {
					return yieldTokenThen(
						Token{Pos: pos, Type: TokenString, Value: str.Bytes()},
						tokenizeDocument,
					)
				}
				continue
			}

			for range quotes {
				str.WriteRune(quote)
			}
			quotes = 0

			if r == '\\' {
				decoded, err := t.readEscape(charPos, true)
				if err != nil {
					return yieldErrorOr(err, nil)
				}
				str.WriteRune(decoded)
				continue
			}
			str.WriteRune(r)
		}
	}
}

// tokenizeAtKeyword reads what follows an '@': either a directive or a
// language tag.
//
//	LANGTAG ::= '@' [a-zA-Z]+ ('-' [a-zA-Z0-9]+)*
//
// The two overlap. "@prefix" is a directive, and it is also a language tag by
// the production above, which a document may legally write after a string.
// Nothing in the character stream tells them apart, so this rule reads the
// letters and calls exactly "@prefix" and "@base" directives.
//
// A language tag of exactly "prefix" or "base" is therefore lexed as a
// directive and will be refused by the parser. It is the one place where this
// tokenizer decides something the grammar leaves to context; a tag with a
// subtag, "@prefix-x", is unaffected, as is every tag that is not one of those
// two words.
func tokenizeAtKeyword(pos Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		var tag bytes.Buffer

		err := t.copyAtLeastOne(&tag, isAlpha)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return yieldErrorOr(err, nil)
		}
		atEnd := errors.Is(err, io.ErrUnexpectedEOF)

		switch tag.String() {
		case "prefix":
			return yieldFinalToken(err, Token{Pos: pos, Type: TokenPrefix, Value: []byte("@prefix")}, tokenizeDocument)
		case "base":
			return yieldFinalToken(err, Token{Pos: pos, Type: TokenBase, Value: []byte("@base")}, tokenizeDocument)
		}

		if atEnd {
			return yieldTokenThen(Token{Pos: pos, Type: TokenLangTag, Value: tag.Bytes()}, nil)
		}
		return tokenizeLangTagSubtags(pos, &tag)
	}
}

// tokenizeLangTagSubtags reads the hyphenated subtags of a language tag, the
// first subtag having been read.
//
// Every subtag after the first must have at least one character, which is what
// refuses RDF 1.2's "--ltr" and "--rtl": the second hyphen arrives where a
// letter or digit is required.
func tokenizeLangTagSubtags(pos Pos, tag *bytes.Buffer) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		for {
			r, ok, err := t.peek()
			if err != nil {
				return yieldErrorOr(err, nil)
			}
			if !ok || r != '-' {
				break
			}

			if _, err := t.next(); err != nil {
				return yieldErrorOr(err, nil)
			}
			tag.WriteRune('-')

			err = t.copyAtLeastOne(tag, isAlphanumeric)
			if err != nil {
				return yieldFinalToken(err, Token{Pos: pos, Type: TokenLangTag, Value: tag.Bytes()}, tokenizeDocument)
			}
		}

		return yieldTokenThen(Token{Pos: pos, Type: TokenLangTag, Value: tag.Bytes()}, tokenizeDocument)
	}
}

// tokenizeAnonOrOpenBracket decides what a '[' opened.
//
//	ANON ::= '[' WS* ']'
//
// Only a ']' with nothing but white space before it makes an ANON, so this
// looks past the white space to find out. What it skips is not put back, white
// space between terminals meaning nothing either way.
func tokenizeAnonOrOpenBracket(pos Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		if err := t.skipIf(isWhitespace); err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				// The document ends here, so the bracket opened nothing.
				return yieldTokenThen(Token{Pos: pos, Type: TokenOpenBracket, Value: []byte("[")}, nil)
			}
			return yieldErrorOr(err, nil)
		}

		r, ok, err := t.peek()
		if err != nil {
			return yieldErrorOr(err, nil)
		}
		if !ok || r != ']' {
			return yieldTokenThen(
				Token{Pos: pos, Type: TokenOpenBracket, Value: []byte("[")},
				tokenizeDocument,
			)
		}
		if _, err := t.next(); err != nil {
			return yieldErrorOr(err, nil)
		}

		return yieldTokenThen(Token{Pos: pos, Type: TokenAnon, Value: []byte("[]")}, tokenizeDocument)
	}
}

// tokenizeName reads a prefixed name or a keyword, its first character having
// been read.
//
//	PNAME_NS ::= PN_PREFIX? ':'
//	PNAME_LN ::= PNAME_NS PN_LOCAL
//	PN_PREFIX ::= PN_CHARS_BASE ((PN_CHARS | '.')* PN_CHARS)?
//
// A name and a keyword begin alike, so the prefix is read first and what it
// turned out to be is settled afterwards: a ':' makes it a prefixed name, and
// without one it can only be "a", "PREFIX" or "BASE".
func tokenizeName(pos Pos, first rune) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		var prefix bytes.Buffer

		if first != ':' {
			prefix.WriteRune(first)

			// A dot inside a prefix is only part of it if a name character
			// follows, as in a blank node label.
			trailing, err := t.copyNameChars(&prefix)
			if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
				return yieldErrorOr(err, nil)
			}

			r, ok, peekErr := t.peek()
			if peekErr != nil {
				return yieldErrorOr(peekErr, nil)
			}
			if !ok || r != ':' {
				return keywordThen(pos, prefix.String(), trailing)
			}
			if _, err := t.next(); err != nil {
				return yieldErrorOr(err, nil)
			}
			if len(trailing) > 0 {
				// "pre.:x" — the dot ended the prefix and cannot, a prefix
				// having to end in a name character.
				return yieldErrorOr(UnexpectedCharacterError{Pos: trailing[0], R: '.'}, nil)
			}
		}

		return tokenizeLocalName(pos, prefix.String())
	}
}

// keywordThen emits the keyword a bare name turned out to be, or reports that
// it was not one.
//
// The dots held back while reading the name are emitted after it, as they are
// after a blank node label: "a." is the keyword and the '.' that ends the
// statement.
func keywordThen(pos Pos, name string, trailing []Pos) tokenizerAction {
	var tok Token

	switch {
	case name == "a":
		tok = Token{Pos: pos, Type: TokenA, Value: []byte("a")}
	case strings.EqualFold(name, "prefix"):
		// SPARQL-style, and case-insensitive where the '@' form is not.
		tok = Token{Pos: pos, Type: TokenPrefix, Value: []byte(name)}
	case strings.EqualFold(name, "base"):
		tok = Token{Pos: pos, Type: TokenBase, Value: []byte(name)}
	default:
		return yieldErrorOr(UnexpectedNameError{Pos: pos, Name: name}, nil)
	}

	next := tokenizeDocument
	for i := len(trailing) - 1; i >= 0; i-- {
		next = yieldTokenThen(Token{Pos: trailing[i], Type: TokenDot, Value: []byte(".")}, next)
	}
	return yieldTokenThen(tok, next)
}

// tokenizeLocalName reads what follows the ':' of a prefixed name.
//
//	PN_LOCAL ::= (PN_CHARS_U | ':' | [0-9] | PLX)
//	             ((PN_CHARS | '.' | ':' | PLX)* (PN_CHARS | ':' | PLX))?
//	PLX      ::= PERCENT | PN_LOCAL_ESC
//
// An empty local name is a PNAME_NS rather than an error — ":" and "foaf:" are
// how a document names a namespace itself.
func tokenizeLocalName(pos Pos, prefix string) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		var local bytes.Buffer

		// The first character of a local name admits a ':' and a digit, which
		// PN_CHARS_BASE does not.
		r, ok, err := t.peek()
		if err != nil {
			return yieldErrorOr(err, nil)
		}
		if !ok || !(isPNCharsU(r) || r == ':' || isDigit(r) || r == '%' || r == '\\') {
			return yieldTokenThen(
				Token{Pos: pos, Type: TokenPNameNS, Value: []byte(prefix)},
				tokenizeDocument,
			)
		}

		trailing, err := t.copyLocalName(&local)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			return yieldErrorOr(err, nil)
		}

		name := prefix + ":" + local.String()
		tok := Token{Pos: pos, Type: TokenPNameLN, Value: []byte(name)}

		next := tokenizeDocument
		if errors.Is(err, io.ErrUnexpectedEOF) {
			next = nil
		}
		for i := len(trailing) - 1; i >= 0; i-- {
			next = yieldTokenThen(Token{Pos: trailing[i], Type: TokenDot, Value: []byte(".")}, next)
		}
		return yieldTokenThen(tok, next)
	}
}

// copyNameChars copies the rest of a PN_PREFIX into dst, returning the
// positions of the dots it read that turned out not to belong to it.
//
// A prefix may hold a dot but may not end with one, so a run of dots is only
// part of the name if a name character follows.
func (t *tokenizer) copyNameChars(dst *bytes.Buffer) ([]Pos, error) {
	var trailing []Pos

	for {
		before := t.mark()

		r, err := t.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return trailing, io.ErrUnexpectedEOF
			}
			return trailing, err
		}

		switch {
		case r == '.':
			trailing = append(trailing, before.pos)
		case isPNChars(r):
			for range trailing {
				dst.WriteRune('.')
			}
			trailing = trailing[:0]
			dst.WriteRune(r)
		default:
			return trailing, t.backup(before)
		}
	}
}

// copyLocalName copies a PN_LOCAL into dst, returning the positions of the
// dots that turned out to end the statement rather than to belong to the name.
//
// A local name admits ':' and the escapes PLX allows, neither of which a
// prefix does.
func (t *tokenizer) copyLocalName(dst *bytes.Buffer) ([]Pos, error) {
	var trailing []Pos

	for {
		before := t.mark()

		r, err := t.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return trailing, io.ErrUnexpectedEOF
			}
			return trailing, err
		}

		if r == '.' {
			trailing = append(trailing, before.pos)
			continue
		}

		var pending bytes.Buffer

		ok, err := t.localNameChar(&pending, r)
		if err != nil {
			return trailing, err
		}
		if !ok {
			return trailing, t.backup(before)
		}

		for range trailing {
			dst.WriteRune('.')
		}
		trailing = trailing[:0]
		dst.Write(pending.Bytes())
	}
}

// localNameChar reads one character of a local name, resolving the two escape
// forms PLX allows, and reports whether r could begin one at all.
//
//	PERCENT      ::= '%' HEX HEX
//	PN_LOCAL_ESC ::= '\' ('_' | '~' | '.' | '-' | '!' | '$' | '&' | "'" | '(' |
//	                      ')' | '*' | '+' | ',' | ';' | '=' | '/' | '?' | '#' |
//	                      '@' | '%')
//
// The two are not alike. A backslash escape stands for the character after it,
// which is how a name holds a character the grammar would otherwise read as
// punctuation. A percent escape is left as written, being part of the IRI the
// name expands to rather than a way of spelling something else.
func (t *tokenizer) localNameChar(dst *bytes.Buffer, r rune) (bool, error) {
	switch {
	case r == '\\':
		escPos := t.pos

		e, err := t.next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return false, UnexpectedEndOfInputError{Pos: escPos}
			}
			return false, err
		}
		if !isPNLocalEsc(e) {
			return false, UnexpectedCharacterError{Pos: escPos, R: e}
		}
		dst.WriteRune(e)
		return true, nil

	case r == '%':
		dst.WriteRune('%')
		for range 2 {
			digitPos := t.pos

			h, err := t.next()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return false, UnexpectedEndOfInputError{Pos: digitPos}
				}
				return false, err
			}
			if _, ok := hexValue(h); !ok {
				return false, UnexpectedCharacterError{Pos: digitPos, R: h}
			}
			dst.WriteRune(h)
		}
		return true, nil

	case isPNChars(r) || r == ':':
		dst.WriteRune(r)
		return true, nil

	default:
		return false, nil
	}
}

// isPNLocalEsc reports whether r is one of the characters a backslash may
// escape in a local name.
func isPNLocalEsc(r rune) bool {
	return strings.ContainsRune(`_~.-!$&'()*+,;=/?#@%`, r)
}

// tokenizeNumber reads a numeric literal, its first character having been
// read.
//
//	INTEGER  ::= [+-]? [0-9]+
//	DECIMAL  ::= [+-]? [0-9]* '.' [0-9]+
//	DOUBLE   ::= [+-]? ([0-9]+ '.' [0-9]* EXPONENT | '.' [0-9]+ EXPONENT | [0-9]+ EXPONENT)
//	EXPONENT ::= [eE] [+-]? [0-9]+
//
// Which of the three it is only becomes clear at the end: a '.' with a digit
// after it makes a decimal, and an exponent makes a double whatever came
// before. A '.' with no digit after it belongs to the statement, not to the
// number, which is why "1." is an integer and a dot rather than a malformed
// decimal.
func tokenizeNumber(pos Pos, first rune) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		var num bytes.Buffer
		num.WriteRune(first)

		if first == '+' || first == '-' {
			// A sign has to be followed by the number it signs.
			signPos := t.pos

			r, ok, err := t.peek()
			if err != nil {
				return yieldErrorOr(err, nil)
			}
			if !ok {
				return yieldErrorOr(UnexpectedEndOfInputError{Pos: signPos}, nil)
			}
			if !isDigit(r) && r != '.' {
				return yieldErrorOr(UnexpectedCharacterError{Pos: signPos, R: r}, nil)
			}
		}

		return readNumberBody(pos, &num, first == '.')
	}
}

// tokenizeDotOrDecimal decides whether a '.' began a number or ended a
// statement.
func tokenizeDotOrDecimal(pos Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		r, ok, err := t.peek()
		if err != nil {
			return yieldErrorOr(err, nil)
		}
		if !ok || !isDigit(r) {
			return symbol(pos, TokenDot, ".")
		}

		var num bytes.Buffer
		num.WriteRune('.')
		return readNumberBody(pos, &num, true)
	}
}

// readNumberBody reads the rest of a numeric literal and decides which of the
// three it is. seenDot says whether the decimal point has already been read.
func readNumberBody(pos Pos, num *bytes.Buffer, seenDot bool) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		kind := TokenInteger
		if seenDot {
			kind = TokenDecimal
		}

		var trailingDot *Pos
		for {
			before := t.mark()

			r, err := t.next()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return numberThen(pos, num, kind, trailingDot, nil)
				}
				return yieldErrorOr(err, nil)
			}

			switch {
			case isDigit(r):
				if trailingDot != nil {
					// The dot had a digit after it after all, so it was a
					// decimal point.
					num.WriteRune('.')
					trailingDot = nil
					kind = TokenDecimal
				}
				num.WriteRune(r)

			case r == '.' && !seenDot && trailingDot == nil:
				// Held back: it is a decimal point only if a digit follows.
				at := before.pos
				trailingDot = &at

			case r == 'e' || r == 'E':
				if trailingDot != nil {
					// "1.e6" is a double, DOUBLE admitting no digits at all
					// between the point and the exponent. The point was a
					// decimal point after all.
					num.WriteRune('.')
					trailingDot = nil
				}
				num.WriteRune(r)
				return readExponent(pos, num)

			default:
				if err := t.backup(before); err != nil {
					return yieldErrorOr(err, nil)
				}
				return numberThen(pos, num, kind, trailingDot, tokenizeDocument)
			}
		}
	}
}

// readExponent reads the rest of an EXPONENT, its 'e' having been read.
func readExponent(pos Pos, num *bytes.Buffer) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		r, ok, err := t.peek()
		if err != nil {
			return yieldErrorOr(err, nil)
		}
		if ok && (r == '+' || r == '-') {
			if _, err := t.next(); err != nil {
				return yieldErrorOr(err, nil)
			}
			num.WriteRune(r)
		}

		digitsPos := t.pos
		err = t.copyAtLeastOne(num, isDigit)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
			if errors.Is(err, io.EOF) {
				return yieldErrorOr(UnexpectedEndOfInputError{Pos: digitsPos}, nil)
			}
			return yieldErrorOr(err, nil)
		}

		tok := Token{Pos: pos, Type: TokenDouble, Value: num.Bytes()}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return yieldTokenThen(tok, nil)
		}
		return yieldTokenThen(tok, tokenizeDocument)
	}
}

// numberThen emits the number, and after it the '.' that turned out to end the
// statement rather than to be a decimal point.
func numberThen(pos Pos, num *bytes.Buffer, kind TokenType, trailingDot *Pos, next tokenizerAction) tokenizerAction {
	if trailingDot != nil {
		next = yieldTokenThen(Token{Pos: *trailingDot, Type: TokenDot, Value: []byte(".")}, next)
	}
	return yieldTokenThen(Token{Pos: pos, Type: kind, Value: num.Bytes()}, next)
}
