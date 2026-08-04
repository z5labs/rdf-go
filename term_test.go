package rdf_test

import (
	"errors"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
)

func TestIRIString(t *testing.T) {
	tests := []struct {
		name string
		iri  rdf.IRI
		want string
	}{
		{
			name: "absolute iri is wrapped in angle brackets",
			iri:  "http://example.com/a",
			want: "<http://example.com/a>",
		},
		{
			name: "empty iri",
			iri:  "",
			want: "<>",
		},
		{
			name: "non-ascii passes through unescaped",
			iri:  "http://example.com/ünïcødé",
			want: "<http://example.com/ünïcødé>",
		},
		{
			name: "a percent escape is not a backslash escape and passes through",
			iri:  "http://example.com/a%20b",
			want: "<http://example.com/a%20b>",
		},
		{
			// The value has no IRIREF at all, so String has none to write.
			// Escaping it would only produce something that looks like one:
			// IRIREF excludes a space however it is written, and the
			// tokenizers refuse a UCHAR naming a space exactly where they
			// refuse the space itself.
			name: "a character no IRIREF admits is written as it stands, not escaped",
			iri:  "http://example.com/a b",
			want: "<http://example.com/a b>",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.iri.String(); got != test.want {
				t.Errorf("String() = %s, want %s", got, test.want)
			}
		})
	}
}

// TestIRIValidate covers the decision that an IRI holding a character IRIREF
// excludes is refused rather than escaped: there is no escape that would help,
// so the value is stopped before a printer is asked to write it.
func TestIRIValidate(t *testing.T) {
	tests := []struct {
		name string
		iri  rdf.IRI
		ok   bool
	}{
		{name: "an absolute iri", iri: "http://example.com/a", ok: true},
		{name: "the empty iri", iri: "", ok: true},
		{name: "a relative reference", iri: "../a", ok: true},
		{name: "non-ascii", iri: "http://example.com/ünïcødé", ok: true},
		{name: "a percent escape", iri: "http://example.com/a%20b", ok: true},
		{name: "a space", iri: "http://example.com/a b"},
		{name: "a less-than sign", iri: "http://example.com/a<b"},
		{name: "a greater-than sign", iri: "http://example.com/a>b"},
		{name: "a quotation mark", iri: "http://example.com/a\"b"},
		{name: "a left brace", iri: "http://example.com/a{b"},
		{name: "a right brace", iri: "http://example.com/a}b"},
		{name: "a vertical line", iri: "http://example.com/a|b"},
		{name: "a circumflex", iri: "http://example.com/a^b"},
		{name: "a grave accent", iri: "http://example.com/a`b"},
		{name: "a backslash", iri: "http://example.com/a\\b"},
		{name: "a line feed", iri: "http://example.com/a\nb"},
		{name: "a tab", iri: "http://example.com/a\tb"},
		{name: "a null", iri: "http://example.com/a\x00b"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.iri.Validate()
			switch {
			case test.ok && err != nil:
				t.Errorf("Validate() = %v, want nil", err)
			case !test.ok && !errors.Is(err, rdf.ErrInvalidIRI):
				t.Errorf("Validate() = %v, want %v", err, rdf.ErrInvalidIRI)
			}
		})
	}
}

// TestIRIValidateReportsTheCharacter covers the error naming what has to be
// rewritten, a caller with a rejected IRI otherwise having only the whole
// string to look through.
func TestIRIValidateReportsTheCharacter(t *testing.T) {
	err := rdf.IRI("http://example.com/a b").Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want an error")
	}
	if !strings.Contains(err.Error(), `' '`) {
		t.Errorf("Validate() = %q, want it to name the offending character", err)
	}
}

// TestIRIValidateAcceptsEveryCharacterAnIRIMayHold checks the predicate
// against the production directly, rather than against the handful of
// characters the table above spells out.
//
//	IRIREF ::= '<' ([^#x00-#x20<>"{}|^`\] | UCHAR)* '>'
func TestIRIValidateAcceptsEveryCharacterAnIRIMayHold(t *testing.T) {
	const excluded = "<>\"{}|^`\\"

	for c := 0x21; c < 0x80; c++ {
		if strings.ContainsRune(excluded, rune(c)) {
			continue
		}

		iri := rdf.IRI("http://example.com/" + string(rune(c)))
		if err := iri.Validate(); err != nil {
			t.Errorf("Validate() = %v for %q, want nil", err, rune(c))
		}
	}
}

func TestIRIEqual(t *testing.T) {
	tests := []struct {
		name  string
		iri   rdf.IRI
		other rdf.Term
		want  bool
	}{
		{
			name:  "identical iris are equal",
			iri:   "http://example.com/a",
			other: rdf.IRI("http://example.com/a"),
			want:  true,
		},
		{
			name:  "different iris are not equal",
			iri:   "http://example.com/a",
			other: rdf.IRI("http://example.com/b"),
			want:  false,
		},
		{
			name:  "comparison is case sensitive",
			iri:   "http://example.com/a",
			other: rdf.IRI("http://example.com/A"),
			want:  false,
		},
		{
			name:  "percent encoding is not normalized",
			iri:   "http://example.com/a%20b",
			other: rdf.IRI("http://example.com/a b"),
			want:  false,
		},
		{
			name:  "an iri never equals a blank node",
			iri:   "http://example.com/a",
			other: rdf.NewBlankNode("a"),
			want:  false,
		},
		{
			name:  "an iri never equals a literal",
			iri:   "http://example.com/a",
			other: rdf.NewLiteral("http://example.com/a"),
			want:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.iri.Equal(test.other); got != test.want {
				t.Errorf("Equal(%v) = %t, want %t", test.other, got, test.want)
			}
		})
	}
}

func TestBlankNode(t *testing.T) {
	t.Run("label is preserved verbatim", func(t *testing.T) {
		b := rdf.NewBlankNode("b0")
		if got, want := b.Label(), "b0"; got != want {
			t.Errorf("Label() = %q, want %q", got, want)
		}
	})

	t.Run("string form carries the underscore colon prefix", func(t *testing.T) {
		b := rdf.NewBlankNode("b0")
		if got, want := b.String(), "_:b0"; got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	})

	t.Run("zero value has an empty label", func(t *testing.T) {
		var b rdf.BlankNode
		if got := b.Label(); got != "" {
			t.Errorf("Label() = %q, want empty", got)
		}
	})
}

func TestBlankNodeEqual(t *testing.T) {
	tests := []struct {
		name  string
		node  rdf.BlankNode
		other rdf.Term
		want  bool
	}{
		{
			name:  "same label is equal",
			node:  rdf.NewBlankNode("b0"),
			other: rdf.NewBlankNode("b0"),
			want:  true,
		},
		{
			name:  "different label is not equal",
			node:  rdf.NewBlankNode("b0"),
			other: rdf.NewBlankNode("b1"),
			want:  false,
		},
		{
			name:  "label comparison is case sensitive",
			node:  rdf.NewBlankNode("b0"),
			other: rdf.NewBlankNode("B0"),
			want:  false,
		},
		{
			name:  "a blank node never equals an iri",
			node:  rdf.NewBlankNode("b0"),
			other: rdf.IRI("b0"),
			want:  false,
		},
		{
			name:  "a blank node never equals a literal",
			node:  rdf.NewBlankNode("b0"),
			other: rdf.NewLiteral("b0"),
			want:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.node.Equal(test.other); got != test.want {
				t.Errorf("Equal(%v) = %t, want %t", test.other, got, test.want)
			}
		})
	}
}

// TestTermIsSealed asserts in test form what the unexported marker method
// guarantees at compile time: the three term types satisfy Term, and a type
// switch over them is exhaustive.
func TestTermIsSealed(t *testing.T) {
	terms := []rdf.Term{
		rdf.IRI("http://example.com/a"),
		rdf.NewBlankNode("b0"),
		rdf.NewLiteral("a"),
	}

	for _, term := range terms {
		switch term.(type) {
		case rdf.IRI, rdf.BlankNode, rdf.Literal:
		default:
			t.Errorf("%T is not one of the three sealed term types", term)
		}
	}
}
