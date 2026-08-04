package rdf

import (
	"fmt"
	"strings"
)

// wellFormedLanguageTag reports whether tag is well-formed by RFC 5646 §2.2.9,
// which defines well-formedness as conformance to the Language-Tag ABNF of
// RFC 5646 §2.1:
//
//	Language-Tag  = langtag / privateuse / grandfathered
//
//	langtag       = language ["-" script] ["-" region] *("-" variant)
//	                *("-" extension) ["-" privateuse]
//	language      = 2*3ALPHA ["-" extlang] / 4ALPHA / 5*8ALPHA
//	extlang       = 3ALPHA *2("-" 3ALPHA)
//	script        = 4ALPHA
//	region        = 2ALPHA / 3DIGIT
//	variant       = 5*8alphanum / (DIGIT 3alphanum)
//	extension     = singleton 1*("-" (2*8alphanum))
//	singleton     = DIGIT / %x41-57 / %x59-5A / %x61-77 / %x79-7A
//	privateuse    = "x" 1*("-" (1*8alphanum))
//
// The ABNF is walked left to right over the hyphen-separated subtags, taking
// each one as the first production that can accept it. That is unambiguous
// because the productions a subtag could be mistaken between are disjoint on
// length and character class: a 3ALPHA subtag after a 2*3ALPHA primary
// language is an extlang and not a region, which is 2ALPHA or 3DIGIT; a
// 4-character subtag is a script if it is all letters and a variant if it
// begins with a digit; and a one-character subtag is a singleton, or "x" and
// therefore the start of the private use sequence.
//
// Every subtag is bounded at eight characters, which is what refuses
// "cantbethislong" — a fourteen-letter primary language subtag matches none of
// the three language alternatives.
func wellFormedLanguageTag(tag string) error {
	if grandfathered(tag) {
		return nil
	}

	subtags := strings.Split(tag, "-")

	// privateuse standing alone, as opposed to trailing a langtag.
	if isSingletonX(subtags[0]) {
		return parsePrivateUse(tag, subtags, 0)
	}

	i, err := parseLanguage(tag, subtags)
	if err != nil {
		return err
	}

	// script ::= 4ALPHA
	if i < len(subtags) && len(subtags[i]) == 4 && allAlpha(subtags[i]) {
		i++
	}

	// region ::= 2ALPHA | 3DIGIT
	if i < len(subtags) && isRegion(subtags[i]) {
		i++
	}

	// *("-" variant)
	for i < len(subtags) && isVariant(subtags[i]) {
		i++
	}

	// *("-" extension), each singleton followed by at least one 2*8alphanum.
	for i < len(subtags) && isSingleton(subtags[i]) {
		singleton := subtags[i]
		i++

		n := 0
		for i < len(subtags) && len(subtags[i]) >= 2 && len(subtags[i]) <= 8 && allAlphanum(subtags[i]) {
			i++
			n++
		}
		if n == 0 {
			return fmt.Errorf("%w: extension %q carries no subtag in %q", ErrInvalidLanguage, singleton, tag)
		}
	}

	// ["-" privateuse]
	if i < len(subtags) && isSingletonX(subtags[i]) {
		return parsePrivateUse(tag, subtags, i)
	}

	if i < len(subtags) {
		return fmt.Errorf("%w: subtag %q is not well-formed in %q", ErrInvalidLanguage, subtags[i], tag)
	}
	return nil
}

// parseLanguage consumes the language production and returns the index of the
// first subtag it did not take.
//
//	language ::= 2*3ALPHA ["-" extlang] | 4ALPHA | 5*8ALPHA
//	extlang  ::= 3ALPHA *2("-" 3ALPHA)
func parseLanguage(tag string, subtags []string) (int, error) {
	primary := subtags[0]
	if !allAlpha(primary) || len(primary) < 2 || len(primary) > 8 {
		return 0, fmt.Errorf("%w: primary language subtag %q is not 2 to 8 letters in %q", ErrInvalidLanguage, primary, tag)
	}

	i := 1
	if len(primary) > 3 {
		// 4ALPHA and 5*8ALPHA take no extlang.
		return i, nil
	}

	// An extlang is three letters, which no other production following the
	// language accepts: region is 2ALPHA or 3DIGIT, script is 4ALPHA, and a
	// variant is at least four characters. Taking them greedily is therefore
	// the only reading.
	for n := 0; n < 3 && i < len(subtags); n++ {
		if len(subtags[i]) != 3 || !allAlpha(subtags[i]) {
			break
		}
		i++
	}
	return i, nil
}

// parsePrivateUse consumes the private use sequence, which runs to the end of
// the tag.
//
//	privateuse ::= "x" 1*("-" (1*8alphanum))
func parsePrivateUse(tag string, subtags []string, i int) error {
	i++ // the "x" itself
	if i == len(subtags) {
		return fmt.Errorf("%w: private use sequence carries no subtag in %q", ErrInvalidLanguage, tag)
	}
	for ; i < len(subtags); i++ {
		if len(subtags[i]) < 1 || len(subtags[i]) > 8 || !allAlphanum(subtags[i]) {
			return fmt.Errorf("%w: private use subtag %q is not 1 to 8 alphanumerics in %q", ErrInvalidLanguage, subtags[i], tag)
		}
	}
	return nil
}

// irregular lists the grandfathered tags RFC 5646 §2.1 records as not matching
// the langtag production. They were registered under RFC 3066 and remain
// well-formed by name alone.
//
// The regular grandfathered tags are not listed: art-lojban, cel-gaulish,
// no-bok, no-nyn, zh-guoyu, zh-hakka, zh-min, zh-min-nan and zh-xiang all
// match langtag — "bok" and "min" as extlang subtags, "lojban" and "guoyu" as
// variants — so the walk accepts them without being told.
var irregular = []string{
	"en-GB-oed",
	"i-ami",
	"i-bnn",
	"i-default",
	"i-enochian",
	"i-hak",
	"i-klingon",
	"i-lux",
	"i-mingo",
	"i-navajo",
	"i-pwn",
	"i-tao",
	"i-tay",
	"i-tsu",
	"i-wa",
	"sgn-BE-FR",
	"sgn-BE-NL",
	"sgn-CH-DE",
}

// grandfathered reports whether tag is one of the irregular grandfathered
// tags. Language tags are case-insensitive (RFC 5646 §2.1.1), so the
// comparison folds case.
func grandfathered(tag string) bool {
	for _, g := range irregular {
		if strings.EqualFold(tag, g) {
			return true
		}
	}
	return false
}

// isRegion reports whether subtag is a region.
//
//	region ::= 2ALPHA | 3DIGIT
func isRegion(subtag string) bool {
	return (len(subtag) == 2 && allAlpha(subtag)) ||
		(len(subtag) == 3 && allDigit(subtag))
}

// isVariant reports whether subtag is a variant.
//
//	variant ::= 5*8alphanum | (DIGIT 3alphanum)
func isVariant(subtag string) bool {
	if len(subtag) >= 5 && len(subtag) <= 8 && allAlphanum(subtag) {
		return true
	}
	return len(subtag) == 4 && isDigit(subtag[0]) && allAlphanum(subtag)
}

// isSingleton reports whether subtag introduces an extension.
//
//	singleton ::= DIGIT | %x41-57 | %x59-5A | %x61-77 | %x79-7A
//
// The two letter ranges are A-Z and a-z with "x" and "X" left out: those
// introduce the private use sequence instead.
func isSingleton(subtag string) bool {
	return len(subtag) == 1 && isAlphanum(subtag[0]) && subtag[0] != 'x' && subtag[0] != 'X'
}

// isSingletonX reports whether subtag is the "x" introducing a private use
// sequence.
func isSingletonX(subtag string) bool {
	return subtag == "x" || subtag == "X"
}

// Character classes of RFC 5646 §2.1, which are ASCII-only: ALPHA is A-Z and
// a-z, DIGIT is 0-9, and alphanum is either. Byte iteration is therefore
// enough — no byte of a multi-byte UTF-8 sequence is below 0x80, so one can
// never be mistaken for a letter or a digit.

func isAlpha(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isAlphanum(c byte) bool { return isAlpha(c) || isDigit(c) }

func allAlpha(s string) bool { return all(s, isAlpha) }

func allDigit(s string) bool { return all(s, isDigit) }

func allAlphanum(s string) bool { return all(s, isAlphanum) }

// all reports whether s is non-empty and every byte of it is in the class.
// The empty string is not, so an empty subtag — which a leading, trailing or
// doubled hyphen produces — fails every production it is offered to.
func all(s string, in func(byte) bool) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !in(s[i]) {
			return false
		}
	}
	return true
}
