package rdf_test

import (
	"errors"
	"testing"

	rdf "github.com/z5labs/rdf-go"
)

func TestQuadValidate(t *testing.T) {
	triple := rdf.Triple{
		Subject:   rdf.IRI("http://example.com/s"),
		Predicate: "http://example.com/p",
		Object:    rdf.NewLiteral("o"),
	}

	tests := []struct {
		name string
		quad rdf.Quad
		want error
	}{
		{
			name: "an iri graph name is valid",
			quad: rdf.Quad{Triple: triple, Graph: rdf.IRI("http://example.com/g")},
		},
		{
			name: "a blank node graph name is valid",
			quad: rdf.Quad{Triple: triple, Graph: rdf.NewBlankNode("g")},
		},
		{
			name: "no graph name means the default graph and is valid",
			quad: rdf.Quad{Triple: triple},
		},
		{
			name: "a literal graph name is rejected",
			quad: rdf.Quad{Triple: triple, Graph: rdf.NewLiteral("g")},
			want: rdf.ErrInvalidGraphName,
		},
		{
			name: "the triple's constraints are checked too",
			quad: rdf.Quad{
				Triple: rdf.Triple{
					Subject:   rdf.NewLiteral("s"),
					Predicate: "http://example.com/p",
					Object:    rdf.NewLiteral("o"),
				},
				Graph: rdf.IRI("http://example.com/g"),
			},
			want: rdf.ErrInvalidSubject,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.quad.Validate()
			if !errors.Is(err, test.want) {
				t.Errorf("Validate() = %v, want %v", err, test.want)
			}
		})
	}
}

func TestQuadEqual(t *testing.T) {
	triple := rdf.Triple{
		Subject:   rdf.IRI("http://example.com/s"),
		Predicate: "http://example.com/p",
		Object:    rdf.NewLiteral("o"),
	}

	tests := []struct {
		name string
		quad rdf.Quad
		othe rdf.Quad
		want bool
	}{
		{
			name: "same triple in the same named graph",
			quad: rdf.Quad{Triple: triple, Graph: rdf.IRI("http://example.com/g")},
			othe: rdf.Quad{Triple: triple, Graph: rdf.IRI("http://example.com/g")},
			want: true,
		},
		{
			name: "same triple in the default graph",
			quad: rdf.Quad{Triple: triple},
			othe: rdf.Quad{Triple: triple},
			want: true,
		},
		{
			name: "same triple in different named graphs",
			quad: rdf.Quad{Triple: triple, Graph: rdf.IRI("http://example.com/g")},
			othe: rdf.Quad{Triple: triple, Graph: rdf.IRI("http://example.com/h")},
		},
		{
			name: "the default graph is not a named graph",
			quad: rdf.Quad{Triple: triple},
			othe: rdf.Quad{Triple: triple, Graph: rdf.IRI("http://example.com/g")},
		},
		{
			name: "an iri name never equals a blank node name",
			quad: rdf.Quad{Triple: triple, Graph: rdf.IRI("g")},
			othe: rdf.Quad{Triple: triple, Graph: rdf.NewBlankNode("g")},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.quad.Equal(test.othe); got != test.want {
				t.Errorf("Equal() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestQuadString(t *testing.T) {
	triple := rdf.Triple{
		Subject:   rdf.IRI("http://example.com/s"),
		Predicate: "http://example.com/p",
		Object:    rdf.NewLiteral("o"),
	}

	tests := []struct {
		name string
		quad rdf.Quad
		want string
	}{
		{
			name: "a named graph is written as a graph label",
			quad: rdf.Quad{Triple: triple, Graph: rdf.IRI("http://example.com/g")},
			want: `<http://example.com/s> <http://example.com/p> "o" <http://example.com/g> .`,
		},
		{
			name: "a blank node graph label",
			quad: rdf.Quad{Triple: triple, Graph: rdf.NewBlankNode("g")},
			want: `<http://example.com/s> <http://example.com/p> "o" _:g .`,
		},
		{
			name: "the default graph has no label",
			quad: rdf.Quad{Triple: triple},
			want: `<http://example.com/s> <http://example.com/p> "o" .`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.quad.String(); got != test.want {
				t.Errorf("String() = %s, want %s", got, test.want)
			}
		})
	}
}
