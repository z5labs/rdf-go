package rdf_test

import (
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
			name: "space is escaped",
			iri:  "http://example.com/a b",
			want: "<http://example.com/a\\u0020b>",
		},
		{
			name: "delimiters excluded by IRIREF are escaped",
			iri:  "http://example.com/<>\"{}|^`\\",
			want: "<http://example.com/" +
				"\\u003C\\u003E\\u0022\\u007B\\u007D\\u007C\\u005E\\u0060\\u005C>",
		},
		{
			name: "control characters are escaped",
			iri:  "http://example.com/\n\t\x00",
			want: "<http://example.com/\\u000A\\u0009\\u0000>",
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
