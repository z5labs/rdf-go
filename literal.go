package rdf

import (
	"errors"
	"fmt"
	"strings"
)

// Direction is the base direction of a language-tagged literal's text,
// introduced in RDF 1.2 for text whose display order cannot be inferred from
// its language alone.
type Direction string

// The base directions permitted by RDF 1.2. Any other value is rejected at
// construction.
const (
	// DirectionNone is the zero Direction, carried by every literal that has
	// no base direction.
	DirectionNone Direction = ""

	// DirectionLTR is left-to-right base direction.
	DirectionLTR Direction = "ltr"

	// DirectionRTL is right-to-left base direction.
	DirectionRTL Direction = "rtl"
)

// Errors reported when a literal would violate an invariant of the RDF data
// model. Each is wrapped with context, so test with [errors.Is].
var (
	// ErrEmptyLanguage is reported when a language-tagged literal is
	// constructed without a language tag.
	ErrEmptyLanguage = errors.New("rdf: language tag must not be empty")

	// ErrEmptyDatatype is reported when a typed literal is constructed with an
	// empty datatype IRI.
	ErrEmptyDatatype = errors.New("rdf: datatype must not be empty")

	// ErrReservedDatatype is reported when rdf:langString or
	// rdf:dirLangString is used as the datatype of a literal that has no
	// language tag.
	ErrReservedDatatype = errors.New("rdf: datatype requires a language tag")

	// ErrInvalidDirection is reported when a base direction is neither "ltr"
	// nor "rtl".
	ErrInvalidDirection = errors.New("rdf: direction must be ltr or rtl")
)

// Literal is an RDF literal: a lexical form paired with a datatype IRI, and
// optionally a language tag and a base direction.
//
// The fields are unexported and the constructors return an error because the
// data model's invariants only hold if they are checked once, at construction:
//
//   - a language tag implies the datatype rdf:langString
//   - a base direction implies the datatype rdf:dirLangString and requires a
//     non-empty language tag
//   - a base direction is either "ltr" or "rtl"
//   - a literal with neither a language tag nor an explicit datatype is
//     xsd:string
//
// Exported fields would let a caller construct a literal that no parser could
// ever produce — a direction with no language, say — and every printer and
// comparison downstream would then need to re-validate.
//
// # Language tag comparison
//
// Language tags compare case-insensitively, so "EN-GB" and "en-gb" tag the
// same literal. This follows RDF 1.2, which changed the rule: under RDF 1.1
// two literals were equal only if their language tags matched exactly, after
// the tag had been normalized to lowercase by the parser. Tags are preserved
// as written by [Literal.Language] and by [Literal.String]; only comparison
// folds case.
//
// The zero Literal is the empty xsd:string literal.
type Literal struct {
	value     string
	datatype  IRI
	language  string
	direction Direction
}

// NewLiteral returns a literal with the given lexical form and the datatype
// xsd:string.
func NewLiteral(value string) Literal {
	return Literal{value: value, datatype: XSDString}
}

// NewTypedLiteral returns a literal with the given lexical form and datatype.
//
// It reports [ErrEmptyDatatype] if datatype is empty, and
// [ErrReservedDatatype] if datatype is rdf:langString or rdf:dirLangString,
// neither of which may appear without a language tag. Use
// [NewLanguageLiteral] or [NewDirectionalLiteral] for those.
func NewTypedLiteral(value string, datatype IRI) (Literal, error) {
	switch datatype {
	case "":
		return Literal{}, ErrEmptyDatatype
	case RDFLangString, RDFDirLangString:
		return Literal{}, fmt.Errorf("%w: %s", ErrReservedDatatype, datatype)
	}
	return Literal{value: value, datatype: datatype}, nil
}

// NewLanguageLiteral returns a literal with the given lexical form and
// language tag. Its datatype is rdf:langString.
//
// It reports [ErrEmptyLanguage] if language is empty.
func NewLanguageLiteral(value, language string) (Literal, error) {
	if language == "" {
		return Literal{}, ErrEmptyLanguage
	}
	return Literal{value: value, datatype: RDFLangString, language: language}, nil
}

// NewDirectionalLiteral returns a literal with the given lexical form,
// language tag and base direction. Its datatype is rdf:dirLangString.
//
// It reports [ErrEmptyLanguage] if language is empty — a base direction
// without a language tag is not expressible in the data model — and
// [ErrInvalidDirection] if direction is neither [DirectionLTR] nor
// [DirectionRTL].
func NewDirectionalLiteral(value, language string, direction Direction) (Literal, error) {
	if language == "" {
		return Literal{}, ErrEmptyLanguage
	}
	switch direction {
	case DirectionLTR, DirectionRTL:
	default:
		return Literal{}, fmt.Errorf("%w: %q", ErrInvalidDirection, string(direction))
	}
	return Literal{
		value:     value,
		datatype:  RDFDirLangString,
		language:  language,
		direction: direction,
	}, nil
}

// Value returns the literal's lexical form.
func (l Literal) Value() string { return l.value }

// Datatype returns the literal's datatype IRI. The zero Literal reports
// xsd:string.
func (l Literal) Datatype() IRI {
	if l.datatype == "" {
		return XSDString
	}
	return l.datatype
}

// Language returns the literal's language tag as written, or the empty string
// if it has none. Compare tags with [strings.EqualFold] rather than ==; see
// the note on [Literal].
func (l Literal) Language() string { return l.language }

// Direction returns the literal's base direction, or [DirectionNone] if it has
// none.
func (l Literal) Direction() Direction { return l.direction }

func (Literal) isTerm() {}

// Equal reports whether other is a literal with the same lexical form,
// datatype and base direction, and a language tag equal under case folding.
func (l Literal) Equal(other Term) bool {
	o, ok := other.(Literal)
	if !ok {
		return false
	}
	return l.value == o.value &&
		l.Datatype() == o.Datatype() &&
		l.direction == o.direction &&
		strings.EqualFold(l.language, o.language)
}

// String renders the literal in canonical N-Triples form.
//
// The datatype is written only when it carries information the rest of the
// syntax does not: it is omitted for xsd:string, which is the default, and for
// the language-tagged datatypes, which the language tag itself implies.
//
//	"plain"
//	"typed"^^<http://example.com/dt>
//	"tagged"@en
//	"directional"@en--ltr
func (l Literal) String() string {
	var b strings.Builder
	b.Grow(len(l.value) + 2)
	b.WriteByte('"')
	escapeString(&b, l.value)
	b.WriteByte('"')

	switch {
	case l.language != "":
		b.WriteByte('@')
		b.WriteString(l.language)
		if l.direction != DirectionNone {
			b.WriteString("--")
			b.WriteString(string(l.direction))
		}
	case l.Datatype() != XSDString:
		b.WriteString("^^")
		b.WriteString(l.Datatype().String())
	}

	return b.String()
}
