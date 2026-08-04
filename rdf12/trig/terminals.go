package trig

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"github.com/z5labs/rdf-go/internal/lex"
)

// tokenizeDocument reads the next terminal of the document, dispatching on the
// character that begins it.
//
// RDF 1.2 TriG is where that character stops settling the matter, and it is the
// only one of the four syntaxes where a '{' is genuinely two terminals. A '<'
// opens an IRIREF, a reified triple or a triple term; a ')' closes a collection
// or a triple term; and a '{' opens a graph block or, with a '|' after it, an
// annotation block. Each is left to the rule it hands off to, as the three RDF
// 1.1 already left there: a '[' may open a property list or be half of an ANON,
// a name may turn out to be a keyword, and a number may end at a '.' that
// belongs to the statement rather than to it.
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
			return tokenizeAngle(pos)
		case r == '>':
			return tokenizeReifiedTripleClose(pos)
		case r == '_':
			return tokenizeBlankNodeLabel(pos)
		case r == '"' || r == '\'':
			return tokenizeString(pos, r)
		case r == '@':
			return tokenizeAtKeyword(pos)
		case r == '^':
			return tokenizeDatatypeMarker(pos)
		case r == '#':
			return tokenizeComment(pos, tokenizeDocument)
		case r == '[':
			return tokenizeAnonOrOpenBracket(pos)
		case r == ':' || lex.IsPNCharsBase(r):
			return tokenizeName(pos, r)
		case r == '+' || r == '-' || lex.IsDigit(r):
			return tokenizeNumber(pos, r)
		case r == '.':
			return tokenizeDotOrDecimal(pos)
		case r == ';':
			return symbol(pos, TokenSemicolon, ";")
		case r == ',':
			return symbol(pos, TokenComma, ",")
		case r == '~':
			return symbol(pos, TokenReifier, "~")
		case r == '(':
			return symbol(pos, TokenOpenParen, "(")
		case r == ')':
			return tokenizeCloseParen(pos)
		case r == '{':
			return tokenizeOpenBrace(pos)
		case r == '}':
			return symbol(pos, TokenCloseBrace, "}")
		case r == '|':
			return tokenizeAnnotationClose(pos)
		case r == ']':
			return symbol(pos, TokenCloseBracket, "]")
		default:
			return yieldErrorOr(UnexpectedCharacterError{Pos: pos, R: r}, nil)
		}
	})
}

// tokenizeAngle settles which of the three terminals beginning with '<' this
// is, the '<' having been consumed:
//
//	IRIREF        ::= '<' ([^#x00-#x20<>"{}|^`\] | UCHAR)* '>'
//	reifiedTriple ::= '<<' rtSubject verb rtObject reifier? '>>'
//	tripleTerm    ::= '<<(' ttSubject verb ttObject ')>>'
//
// Two characters of lookahead settle it, and both are only ever lookahead. A
// second '<' rules out an IRIREF, the production excluding it, so the choice is
// then between the two openers and is made on the third character: a '(' makes
// it a triple term, and anything else — a space, an IRI, the end of the input —
// leaves the "<<" standing on its own.
//
// Each character that turns out not to belong to the terminal is put back
// unread, so the rule that follows begins where it would have had this one
// never looked. That is what keeps "<<<http://e/s>" reading as a reified
// triple whose subject is an IRI rather than as either a longer delimiter or
// an error.
func tokenizeAngle(pos Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		second, ok, err := t.peek()
		if err != nil {
			return yieldErrorOr(err, nil)
		}
		if !ok || second != '<' {
			// A '<' at the very end of the input opens an IRIREF that never
			// closes, which is what tokenizeIRIRef reports.
			return tokenizeIRIRef(pos)
		}
		if _, err := t.next(); err != nil {
			return yieldErrorOr(err, nil)
		}

		third, ok, err := t.peek()
		if err != nil {
			return yieldErrorOr(err, nil)
		}
		if !ok || third != '(' {
			return yieldTokenThen(
				Token{Pos: pos, Type: TokenReifiedTripleOpen, Value: []byte("<<")},
				tokenizeDocument,
			)
		}
		if _, err := t.next(); err != nil {
			return yieldErrorOr(err, nil)
		}

		return yieldTokenThen(
			Token{Pos: pos, Type: TokenTripleTermOpen, Value: []byte("<<(")},
			tokenizeDocument,
		)
	}
}

// tokenizeReifiedTripleClose reads the second '>' of the ">>" that closes a
// reified triple, the first having been consumed.
//
// Nothing else in the grammar begins with a '>' — an IRIREF's closing one is
// read by [tokenizeIRIRef] and never reaches here — so the second is required
// rather than looked ahead at, and whatever arrives instead is reported where
// it stands.
func tokenizeReifiedTripleClose(pos Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		if err := t.expect(">"); err != nil {
			return yieldErrorOr(err, nil)
		}

		return yieldTokenThen(
			Token{Pos: pos, Type: TokenReifiedTripleClose, Value: []byte(">>")},
			tokenizeDocument,
		)
	}
}

// tokenizeCloseParen decides whether a ')' closed a collection or began the
// ")>>" that closes a triple term, the ')' having been consumed.
//
//	collection ::= '(' object* ')'
//	tripleTerm ::= '<<(' ttSubject verb ttObject ')>>'
//
// One character of lookahead is enough: a '>' can only be the delimiter's, a
// collection being followed by a term or by punctuation and never by a '>'.
// Anything else is put back and the ')' stands alone.
//
// Once that '>' is consumed the second is required rather than looked ahead
// at, since ")>" ends nothing in the grammar. This is the one place the rule
// cannot put back what it read — a reader can unread one character — and the
// one place it need not.
func tokenizeCloseParen(pos Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		r, ok, err := t.peek()
		if err != nil {
			return yieldErrorOr(err, nil)
		}
		if !ok || r != '>' {
			return symbol(pos, TokenCloseParen, ")")
		}
		if _, err := t.next(); err != nil {
			return yieldErrorOr(err, nil)
		}

		if err := t.expect(">"); err != nil {
			return yieldErrorOr(err, nil)
		}

		return yieldTokenThen(
			Token{Pos: pos, Type: TokenTripleTermClose, Value: []byte(")>>")},
			tokenizeDocument,
		)
	}
}

// tokenizeOpenBrace decides which of the two braces a '{' opened, the '{'
// having been consumed.
//
//	wrappedGraph    ::= '{' triplesBlock? '}'
//	annotationBlock ::= '{|' predicateObjectList '|}'
//
// This is the one place TriG has to tell the two apart, and it is the reason
// the ambiguity is settled here rather than in the parser: one character of
// lookahead is enough and nothing beyond it is needed. A '|' after the brace
// makes an annotation block; anything else — a term, a '}', a comment, the end
// of the input — leaves the brace standing on its own as the opener of a graph
// block, and is put back unread so the rule that follows begins where it would
// have had this one never looked.
//
// Nothing in the grammar begins with a '|' other than the "|}" that closes an
// annotation block, so no graph block can begin with one and the lookahead can
// never take a graph block's first terminal for the annotation's bar. That is
// what keeps "{| :q :v |}" and "{ :s :p :o }" apart wherever they stand, the
// two being one character deep and no more.
func tokenizeOpenBrace(pos Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		r, ok, err := t.peek()
		if err != nil {
			return yieldErrorOr(err, nil)
		}
		if !ok || r != '|' {
			return symbol(pos, TokenOpenBrace, "{")
		}
		if _, err := t.next(); err != nil {
			return yieldErrorOr(err, nil)
		}

		return yieldTokenThen(
			Token{Pos: pos, Type: TokenAnnotationOpen, Value: []byte("{|")},
			tokenizeDocument,
		)
	}
}

// tokenizeAnnotationClose reads the '}' of the "|}" that closes an annotation
// block, the '|' having been consumed.
//
// A '|' begins nothing else, so the '}' is required rather than looked ahead
// at.
func tokenizeAnnotationClose(pos Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		if err := t.expect("}"); err != nil {
			return yieldErrorOr(err, nil)
		}

		return yieldTokenThen(
			Token{Pos: pos, Type: TokenAnnotationClose, Value: []byte("|}")},
			tokenizeDocument,
		)
	}
}

// symbol emits a one-character terminal and carries on.
func symbol(pos Pos, kind TokenType, text string) tokenizerAction {
	return yieldTokenThen(Token{Pos: pos, Type: kind, Value: []byte(text)}, tokenizeDocument)
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
// language tag with an optional base direction.
//
//	LANG_DIR ::= '@' [a-zA-Z]+ ('-' [a-zA-Z0-9]+)* ('--' [a-zA-Z]+)?
//
// The two overlap. "@prefix" is a directive, and it is also a language tag by
// the production above, which a document may legally write after a string.
// Nothing in the character stream tells them apart, so this rule reads the
// letters and calls exactly "@prefix", "@base" and "@version" directives.
//
// A language tag of exactly one of those three words is therefore lexed as a
// directive and will be refused by the parser. It is the one place where this
// tokenizer decides something the grammar leaves to context; a tag with a
// subtag, "@prefix-x", is unaffected, as is every tag that is not one of those
// three words.
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
		case "version":
			return yieldFinalToken(err, Token{Pos: pos, Type: TokenVersion, Value: []byte("@version")}, tokenizeVersionSpecifier)
		}

		if atEnd {
			return yieldTokenThen(Token{Pos: pos, Type: TokenLangDir, Value: tag.Bytes()}, nil)
		}
		return tokenizeLangDirSubtags(pos, &tag)
	}
}

// tokenizeLangDirSubtags reads the hyphenated subtags of a LANG_DIR and the
// base direction that may end it, the first subtag having been read.
//
// This is where RDF 1.2 extends the RDF 1.1 LANGTAG: a second hyphen ends the
// subtags and begins the initial base direction, which the grammar leaves as
// any run of letters. That the direction must read "ltr" or "rtl" is a
// constraint the spec states where it builds RDF terms rather than in the
// grammar, and it is enforced there — the tokenizer's job is to say where the
// terminal ends.
//
// A hyphen is therefore read with one character of lookahead: a second hyphen
// is the direction's, and anything else belongs to another subtag. The
// direction ends the terminal, since the grammar allows no subtag after it.
func tokenizeLangDirSubtags(pos Pos, tag *bytes.Buffer) tokenizerAction {
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

			second, ok, err := t.peek()
			if err != nil {
				return yieldErrorOr(err, nil)
			}
			if ok && second == '-' {
				if _, err := t.next(); err != nil {
					return yieldErrorOr(err, nil)
				}
				tag.WriteRune('-')

				// The direction is the last thing a LANG_DIR may carry, so the
				// terminal ends with it however it turns out.
				return yieldFinalToken(
					t.copyAtLeastOne(tag, isAlpha),
					Token{Pos: pos, Type: TokenLangDir, Value: tag.Bytes()},
					tokenizeDocument,
				)
			}

			err = t.copyAtLeastOne(tag, isAlphanumeric)
			if err != nil {
				return yieldFinalToken(err, Token{Pos: pos, Type: TokenLangDir, Value: tag.Bytes()}, tokenizeDocument)
			}
		}

		return yieldTokenThen(Token{Pos: pos, Type: TokenLangDir, Value: tag.Bytes()}, tokenizeDocument)
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
// without one it can only be a keyword — "a", "true", "false", "PREFIX",
// "BASE", "VERSION" or "GRAPH".
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
//
// A name followed by a ':' never reaches here, so "graph:name" is still a
// prefixed name on the "graph" prefix rather than the keyword and a stray
// colon.
func keywordThen(pos Pos, name string, trailing []Pos) tokenizerAction {
	var tok Token

	switch {
	case name == "a":
		tok = Token{Pos: pos, Type: TokenA, Value: []byte("a")}
	case name == "true", name == "false":
		// Case-sensitive, where the SPARQL-style directives below are not.
		tok = Token{Pos: pos, Type: TokenBoolean, Value: []byte(name)}
	case strings.EqualFold(name, "prefix"):
		// SPARQL-style, and case-insensitive where the '@' form is not.
		tok = Token{Pos: pos, Type: TokenPrefix, Value: []byte(name)}
	case strings.EqualFold(name, "base"):
		tok = Token{Pos: pos, Type: TokenBase, Value: []byte(name)}
	case strings.EqualFold(name, "version"):
		// sparqlVersion ::= "VERSION" VersionSpecifier, and a keyword in double
		// quotes is matched however it was capitalized.
		tok = Token{Pos: pos, Type: TokenVersion, Value: []byte(name)}
	case strings.EqualFold(name, "graph"):
		// The keyword TriG adds, and case-insensitive for the same reason the
		// three above are: the grammar writes it in double quotes.
		tok = Token{Pos: pos, Type: TokenGraph, Value: []byte(name)}
	default:
		return yieldErrorOr(UnexpectedNameError{Pos: pos, Name: name}, nil)
	}

	next := tokenizeDocument
	if tok.Type == TokenVersion {
		// What may follow the keyword is narrower than what may follow a
		// statement, so the specifier gets a rule of its own.
		next = tokenizeVersionSpecifier
	}
	for i := len(trailing) - 1; i >= 0; i-- {
		next = yieldTokenThen(Token{Pos: trailing[i], Type: TokenDot, Value: []byte(".")}, next)
	}
	return yieldTokenThen(tok, next)
}

// tokenizeVersionSpecifier reads the string a version directive announces, the
// keyword having been emitted.
//
//	version          ::= '@version' VersionSpecifier '.'
//	sparqlVersion    ::= "VERSION" VersionSpecifier
//	VersionSpecifier ::= STRING_LITERAL_QUOTE | STRING_LITERAL_SINGLE_QUOTE
//
// A version specifier is the one place the grammar asks for a string and admits
// only two of the four forms TriG writes one in. Which form a string was
// written in is not recorded in the token — the four differ in what they can
// hold, not in what they mean — so the restriction is kept where the string is
// read, at the one point that knows a version keyword came immediately before
// it.
//
// That restriction is the whole of what this rule adds. A version keyword
// followed by a number, or by nothing at all, is handed back to
// [tokenizeDocument] and reported by the parser, which can name the token that
// arrived where a string was wanted rather than the character that began it.
//
// A comment may stand between the keyword and the specifier as readily as
// anywhere else, being white space to the grammar, so one is read and the
// search for the specifier resumes after it.
func tokenizeVersionSpecifier(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
	return skipSpace(func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		pos := t.pos

		r, ok, err := t.peek()
		if err != nil {
			return yieldErrorOr(err, nil)
		}
		if !ok || (r != '#' && r != '"' && r != '\'') {
			return tokenizeDocument
		}
		if _, err := t.next(); err != nil {
			return yieldErrorOr(err, nil)
		}

		if r == '#' {
			return tokenizeComment(pos, tokenizeVersionSpecifier)
		}
		return tokenizeVersionString(pos, r)
	})
}

// tokenizeVersionString reads a version specifier's string, its first quote
// having been read.
//
//	VersionSpecifier            ::= STRING_LITERAL_QUOTE | STRING_LITERAL_SINGLE_QUOTE
//	STRING_LITERAL_QUOTE        ::= '"' ([^#x22#x5C#xA#xD] | ECHAR | UCHAR)* '"'
//	STRING_LITERAL_SINGLE_QUOTE ::= "'" ([^#x27#x5C#xA#xD] | ECHAR | UCHAR)* "'"
//
// Two quotes and a third is where a long string begins, and a long string is no
// version specifier: that third quote is the character the directive may not
// write, so that is where it is reported. Two quotes and no third are the empty
// string, complete as it stands.
func tokenizeVersionString(pos Pos, quote rune) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		second, ok, err := t.peek()
		if err != nil {
			return yieldErrorOr(err, nil)
		}
		if !ok || second != quote {
			return tokenizeShortString(pos, quote)
		}
		if _, err := t.next(); err != nil {
			return yieldErrorOr(err, nil)
		}

		thirdPos := t.pos

		third, ok, err := t.peek()
		if err != nil {
			return yieldErrorOr(err, nil)
		}
		if ok && third == quote {
			return yieldErrorOr(UnexpectedCharacterError{Pos: thirdPos, R: third}, nil)
		}
		return yieldTokenThen(Token{Pos: pos, Type: TokenString}, tokenizeDocument)
	}
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
		if !ok || !(lex.IsPNCharsU(r) || r == ':' || lex.IsDigit(r) || r == '%' || r == '\\') {
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
		case lex.IsPNChars(r):
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
		if !lex.IsPNLocalEsc(e) {
			return false, UnexpectedCharacterError{Pos: escPos, R: e}
		}
		dst.WriteRune(e)
		return true, nil

	case r == '%':
		next, at := t.scan()

		hi, lo, err := lex.PercentEscape(next)
		if err != nil {
			return false, positioned(err, *at)
		}
		dst.WriteRune('%')
		dst.WriteRune(hi)
		dst.WriteRune(lo)
		return true, nil

	case lex.IsPNChars(r) || r == ':':
		dst.WriteRune(r)
		return true, nil

	default:
		return false, nil
	}
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
			if !lex.IsDigit(r) && r != '.' {
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
		if !ok || !lex.IsDigit(r) {
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
					if !hasDigit(num.Bytes()) {
						// "+." or "-.", cut off where a digit was required.
						return yieldErrorOr(UnexpectedEndOfInputError{Pos: t.pos}, nil)
					}
					return numberThen(pos, num, kind, trailingDot, nil)
				}
				return yieldErrorOr(err, nil)
			}

			switch {
			case lex.IsDigit(r):
				if trailingDot != nil {
					// The dot had a digit after it after all, so it was a
					// decimal point — and, since every production allows only
					// one, the last one this number may hold.
					num.WriteRune('.')
					trailingDot = nil
					seenDot = true
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
				if !hasDigit(num.Bytes()) {
					// "-.e6": every form of DOUBLE has digits before the
					// exponent, and a sign and a point are not digits.
					return yieldErrorOr(UnexpectedCharacterError{Pos: before.pos, R: r}, nil)
				}
				num.WriteRune(r)
				return readExponent(pos, num)

			default:
				if !hasDigit(num.Bytes()) {
					// "-.x": the sign and the point were read and the digit
					// they promised is not what arrived.
					return yieldErrorOr(UnexpectedCharacterError{Pos: before.pos, R: r}, nil)
				}
				if err := t.backup(before); err != nil {
					return yieldErrorOr(err, nil)
				}
				return numberThen(pos, num, kind, trailingDot, tokenizeDocument)
			}
		}
	}
}

// hasDigit reports whether the number read so far holds a digit.
//
//	INTEGER  ::= [+-]? [0-9]+
//	DECIMAL  ::= [+-]? [0-9]* '.' [0-9]+
//	DOUBLE   ::= [+-]? ([0-9]+ '.' [0-9]* EXPONENT | '.' [0-9]+ EXPONENT | [0-9]+ EXPONENT)
//
// All three require one, so a sign and a decimal point on their own are not a
// number however the rest of the input goes on. Only "+." and "-." can get
// this far without a digit: a sign is followed by a digit or a point, and a
// number opening with a point is only read once a digit is known to follow it.
func hasDigit(b []byte) bool {
	return bytes.ContainsFunc(b, lex.IsDigit)
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
		err = t.copyAtLeastOne(num, lex.IsDigit)
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
