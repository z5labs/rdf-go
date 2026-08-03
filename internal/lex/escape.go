package lex

import (
	"fmt"
	"unicode"
)

// NextFunc reads the next character of the input.
//
// The tokenizer that supplies one owns the reader, so it also owns what
// happens at the end of it: an escape sequence cut short is an error, and
// which error it is — with which position — is the caller's to say. This
// package passes anything a NextFunc returns straight back to it.
type NextFunc func() (rune, error)

// UnexpectedCharacterError is reported when a character cannot appear where it
// does: after the backslash that begins an escape sequence, or among the
// hexadecimal digits one requires.
//
// It carries no position, only the character. The caller read that character
// out of its own input and knows where it stood, so it is the caller that
// reports it with a position, in whatever error its package documents.
type UnexpectedCharacterError struct {
	R rune
}

// Error implements the [error] interface.
func (e UnexpectedCharacterError) Error() string {
	return fmt.Sprintf("unexpected character %q", e.R)
}

// InvalidCodePointError is reported when a UCHAR names something that is not a
// character: a value above the highest code point, or one of the surrogates,
// which exist only to encode other code points in UTF-16 and stand for nothing
// on their own.
//
// As with [UnexpectedCharacterError], the position of the escape is the
// caller's to supply.
type InvalidCodePointError struct {
	Code uint32
}

// Error implements the [error] interface.
func (e InvalidCodePointError) Error() string {
	return fmt.Sprintf("escape names no character, U+%04X", e.Code)
}

// Escape decodes the escape sequence following a backslash, reading it from
// next, and returns the character it denotes. The backslash itself has already
// been read.
//
//	UCHAR ::= '\u' HEX HEX HEX HEX | '\U' HEX HEX HEX HEX HEX HEX HEX HEX
//	ECHAR ::= '\' [tbnrf"'\]
//
// An IRIREF admits only the first of the two, so echar says which caller this
// is. Anything else after the backslash is [UnexpectedCharacterError], which is
// how an unknown escape is reported.
func Escape(next NextFunc, echar bool) (rune, error) {
	r, err := next()
	if err != nil {
		return 0, err
	}

	switch r {
	case 'u':
		return codePoint(next, 4)
	case 'U':
		return codePoint(next, 8)
	}

	if echar {
		switch r {
		case 't':
			return '\t', nil
		case 'b':
			return '\b', nil
		case 'n':
			return '\n', nil
		case 'r':
			return '\r', nil
		case 'f':
			return '\f', nil
		case '"':
			return '"', nil
		case '\'':
			return '\'', nil
		case '\\':
			return '\\', nil
		}
	}

	return 0, UnexpectedCharacterError{R: r}
}

// codePoint reads the hexadecimal digits of a UCHAR and returns the character
// they name.
func codePoint(next NextFunc, digits int) (rune, error) {
	var code uint32

	for range digits {
		r, err := next()
		if err != nil {
			return 0, err
		}

		value, ok := hexValue(r)
		if !ok {
			return 0, UnexpectedCharacterError{R: r}
		}
		code = code<<4 | uint32(value)
	}

	// Eight hex digits can name far more than Unicode has, and the surrogate
	// range names nothing at all, so neither is simply written out as U+FFFD.
	if code > unicode.MaxRune || (code >= 0xD800 && code <= 0xDFFF) {
		return 0, InvalidCodePointError{Code: code}
	}
	return rune(code), nil
}

// PercentEscape reads the two hexadecimal digits of a PERCENT escape from
// next, the '%' having already been read, and returns them as they were
// written.
//
//	PERCENT ::= '%' HEX HEX
//
// The digits come back undecoded on purpose. A percent escape is part of the
// IRI that a prefixed name expands to rather than a way of spelling some other
// character, so it is carried through as written — which is the whole of how
// PERCENT differs from the backslash escape that stands beside it in PLX.
func PercentEscape(next NextFunc) (hi, lo rune, err error) {
	digits := [2]rune{}

	for i := range digits {
		r, err := next()
		if err != nil {
			return 0, 0, err
		}
		if !IsHex(r) {
			return 0, 0, UnexpectedCharacterError{R: r}
		}
		digits[i] = r
	}

	return digits[0], digits[1], nil
}
