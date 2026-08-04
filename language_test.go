package rdf_test

import (
	"errors"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
)

// TestLanguageTagWellFormedness walks the Language-Tag grammar of RFC 5646
// §2.1, which §2.2.9 defines well-formedness as. The tags are exercised
// through the two constructors that enforce the rule rather than through the
// checker directly, because the constructors are the contract: every parser in
// this module reaches the check through one of them.
//
// Validity — whether the subtags are registered — is deliberately not checked,
// so a well-formed tag built only of unassigned subtags is accepted here. The
// accepted list says so out loud, since that is the decision this test is
// pinning down as much as the grammar.
func TestLanguageTagWellFormedness(t *testing.T) {
	wellFormed := []struct {
		name string
		tag  string
	}{
		{name: "two letter language", tag: "en"},
		{name: "three letter language", tag: "haw"},
		{name: "four letter language, reserved for future use", tag: "qaaa"},
		{name: "five letter registered language", tag: "abcde"},
		{name: "eight letter registered language, the longest allowed", tag: "abcdefgh"},
		{name: "language and extlang", tag: "zh-yue"},
		{name: "language and three extlangs, the most allowed", tag: "zh-abc-def-ghi"},
		{name: "language and script", tag: "sr-Latn"},
		{name: "language and alphabetic region", tag: "en-GB"},
		{name: "language and numeric region", tag: "es-419"},
		{name: "language, script and region", tag: "zh-Hant-TW"},
		{name: "language, region and variant", tag: "sl-IT-nedis"},
		{name: "eight character variant, the longest allowed", tag: "de-CH-1901abcd"},
		{name: "digit-led four character variant", tag: "de-1901"},
		{name: "two variants", tag: "sl-rozaj-biske"},
		{name: "extension", tag: "de-DE-u-co-phonebk"},
		{name: "digit singleton extension", tag: "en-1-abc"},
		{name: "two extensions", tag: "en-a-bbb-c-ddd"},
		{name: "private use standing alone", tag: "x-whatever"},
		{name: "private use after a langtag", tag: "en-US-x-twain"},
		{name: "extension then private use", tag: "en-a-bbb-x-private"},
		{name: "case is not significant", tag: "EN-latn-gb-BOONT"},
		{name: "irregular grandfathered tag", tag: "i-klingon"},
		{name: "irregular grandfathered tag with subtags", tag: "en-GB-oed"},
		{name: "irregular grandfathered tag, case folded", tag: "SGN-be-fr"},
		{name: "regular grandfathered tag reachable through langtag", tag: "art-lojban"},
		{name: "regular grandfathered tag with an extlang", tag: "zh-min-nan"},
		{name: "well-formed but not valid: no subtag here is assigned", tag: "qaa-Qaaa-QM-x-southern"},
	}

	for _, test := range wellFormed {
		t.Run("well-formed/"+test.name, func(t *testing.T) {
			if _, err := rdf.NewLanguageLiteral("a", test.tag); err != nil {
				t.Errorf("NewLanguageLiteral(%q) = %v, want nil", test.tag, err)
			}
			if _, err := rdf.NewDirectionalLiteral("a", test.tag, rdf.DirectionLTR); err != nil {
				t.Errorf("NewDirectionalLiteral(%q) = %v, want nil", test.tag, err)
			}
		})
	}

	malformed := []struct {
		name string
		tag  string
	}{
		{name: "one letter primary language", tag: "a"},
		{
			name: "nine letter primary language, past the eight character cap",
			tag:  "abcdefghi",
		},
		{
			name: "the fourteen letter tag the W3C suite refuses",
			tag:  "cantbethislong",
		},
		{name: "digits in the primary language", tag: "e1"},
		{name: "leading hyphen", tag: "-en"},
		{name: "trailing hyphen", tag: "en-"},
		{name: "doubled hyphen", tag: "en--GB"},
		{name: "four extlangs, one past the most allowed", tag: "zh-abc-def-ghi-jkl"},
		{name: "extlang after a four letter language", tag: "qaaa-abc"},
		{name: "script after a region", tag: "en-GB-Latn"},
		{name: "two regions", tag: "en-GB-US"},
		{name: "two letter subtag after a variant", tag: "sl-nedis-GB"},
		{name: "extension singleton with no subtag", tag: "en-a"},
		{name: "extension subtag of one character", tag: "en-a-b-cc"},
		{name: "private use with no subtag", tag: "en-x"},
		{name: "private use standing alone with no subtag", tag: "x"},
		{name: "private use subtag of nine characters", tag: "x-abcdefghi"},
		{name: "subtag after private use that is not part of it", tag: "x-abc-"},
		{name: "underscore rather than hyphen", tag: "en_GB"},
		{name: "non-ASCII letters", tag: "ünicode"},
		{name: "a grandfathered tag that is not one of them", tag: "i-notregistered"},
	}

	for _, test := range malformed {
		t.Run("malformed/"+test.name, func(t *testing.T) {
			l, err := rdf.NewLanguageLiteral("a", test.tag)
			if !errors.Is(err, rdf.ErrInvalidLanguage) {
				t.Errorf("NewLanguageLiteral(%q) = %v, want %v", test.tag, err, rdf.ErrInvalidLanguage)
			}
			if (l != rdf.Literal{}) {
				t.Errorf("literal = %v, want the zero value on error", l)
			}

			l, err = rdf.NewDirectionalLiteral("a", test.tag, rdf.DirectionLTR)
			if !errors.Is(err, rdf.ErrInvalidLanguage) {
				t.Errorf("NewDirectionalLiteral(%q) = %v, want %v", test.tag, err, rdf.ErrInvalidLanguage)
			}
			if (l != rdf.Literal{}) {
				t.Errorf("literal = %v, want the zero value on error", l)
			}
		})
	}
}

// TestLanguageTagErrorNamesTheTag checks that the error carries the tag it
// refused. The position comes from the parser that called the constructor, so
// the tag itself is all the data model layer can say about where the trouble
// is — and an error that only said "not well-formed" would leave a caller
// building literals from its own data with nothing to look at.
func TestLanguageTagErrorNamesTheTag(t *testing.T) {
	_, err := rdf.NewLanguageLiteral("a", "cantbethislong")
	if err == nil {
		t.Fatal("NewLanguageLiteral() = nil, want an error")
	}
	if got := err.Error(); !strings.Contains(got, "cantbethislong") {
		t.Errorf("error = %q, want it to name the tag", got)
	}
}
