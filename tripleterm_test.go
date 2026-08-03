package rdf_test

import (
	"errors"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
)

// tripleTerm builds a triple term over the three positions, keeping the tables
// below to one line a term.
func tripleTerm(subject rdf.Term, predicate rdf.IRI, object rdf.Term) rdf.TripleTerm {
	return rdf.TripleTerm{Subject: subject, Predicate: predicate, Object: object}
}

func TestTripleTermString(t *testing.T) {
	tests := []struct {
		name string
		term func(t *testing.T) rdf.TripleTerm
		want string
	}{
		{
			name: "iri positions",
			term: func(*testing.T) rdf.TripleTerm {
				return tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", rdf.IRI("http://example.com/o"))
			},
			want: "<<( <http://example.com/s> <http://example.com/p> <http://example.com/o> )>>",
		},
		{
			name: "a blank node subject",
			term: func(*testing.T) rdf.TripleTerm {
				return tripleTerm(rdf.NewBlankNode("b0"), "http://example.com/p", rdf.IRI("http://example.com/o"))
			},
			want: "<<( _:b0 <http://example.com/p> <http://example.com/o> )>>",
		},
		{
			name: "a literal object is escaped as it is elsewhere",
			term: func(*testing.T) rdf.TripleTerm {
				return tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", rdf.NewLiteral("a\"b\n"))
			},
			want: `<<( <http://example.com/s> <http://example.com/p> "a\"b\n" )>>`,
		},
		{
			name: "a nested triple term",
			term: func(*testing.T) rdf.TripleTerm {
				inner := tripleTerm(rdf.IRI("http://example.com/s2"), "http://example.com/p2", rdf.NewLiteral("o"))
				return tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", inner)
			},
			want: `<<( <http://example.com/s> <http://example.com/p> ` +
				`<<( <http://example.com/s2> <http://example.com/p2> "o" )>> )>>`,
		},
		{
			name: "the zero triple term renders its empty positions",
			term: func(*testing.T) rdf.TripleTerm { return rdf.TripleTerm{} },
			want: "<<( <nil> <> <nil> )>>",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.term(t).String(); got != test.want {
				t.Errorf("String() = %s, want %s", got, test.want)
			}
		})
	}
}

// TestTripleTermStringInAStatement covers the term where it is actually met:
// as the object of a triple, which is the only position it may hold.
func TestTripleTermStringInAStatement(t *testing.T) {
	triple := rdf.Triple{
		Subject:   rdf.IRI("http://example.com/s"),
		Predicate: "http://example.com/p",
		Object: tripleTerm(
			rdf.IRI("http://example.com/s2"),
			"http://example.com/p2",
			rdf.IRI("http://example.com/o2"),
		),
	}

	want := "<http://example.com/s> <http://example.com/p> " +
		"<<( <http://example.com/s2> <http://example.com/p2> <http://example.com/o2> )>> ."
	if got := triple.String(); got != want {
		t.Errorf("String() = %s, want %s", got, want)
	}
}

func TestTripleTermEqual(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T) (rdf.TripleTerm, rdf.Term)
		want  bool
	}{
		{
			name: "identical triple terms are equal",
			build: func(*testing.T) (rdf.TripleTerm, rdf.Term) {
				term := tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", rdf.NewLiteral("o"))
				return term, term
			},
			want: true,
		},
		{
			name: "a different subject is not equal",
			build: func(*testing.T) (rdf.TripleTerm, rdf.Term) {
				return tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", rdf.NewLiteral("o")),
					tripleTerm(rdf.IRI("http://example.com/other"), "http://example.com/p", rdf.NewLiteral("o"))
			},
		},
		{
			name: "a different predicate is not equal",
			build: func(*testing.T) (rdf.TripleTerm, rdf.Term) {
				return tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", rdf.NewLiteral("o")),
					tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/other", rdf.NewLiteral("o"))
			},
		},
		{
			name: "a different object is not equal",
			build: func(*testing.T) (rdf.TripleTerm, rdf.Term) {
				return tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", rdf.NewLiteral("o")),
					tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", rdf.NewLiteral("other"))
			},
		},
		{
			name: "a term of another kind is not equal",
			build: func(*testing.T) (rdf.TripleTerm, rdf.Term) {
				return tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", rdf.NewLiteral("o")),
					rdf.IRI("http://example.com/s")
			},
		},
		{
			name: "equal nested triple terms are equal",
			build: func(*testing.T) (rdf.TripleTerm, rdf.Term) {
				inner := func() rdf.TripleTerm {
					return tripleTerm(rdf.IRI("http://example.com/s2"), "http://example.com/p2", rdf.NewLiteral("o"))
				}
				return tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", inner()),
					tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", inner())
			},
			want: true,
		},
		{
			name: "nested triple terms differing at the bottom are not equal",
			build: func(*testing.T) (rdf.TripleTerm, rdf.Term) {
				outer := func(deepest rdf.Term) rdf.TripleTerm {
					inner := tripleTerm(rdf.IRI("http://example.com/s3"), "http://example.com/p3", deepest)
					middle := tripleTerm(rdf.IRI("http://example.com/s2"), "http://example.com/p2", inner)
					return tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", middle)
				}
				return outer(rdf.NewLiteral("o")), outer(rdf.NewLiteral("other"))
			},
		},
		{
			name: "a nested term is compared with term equality, not ==",
			build: func(t *testing.T) (rdf.TripleTerm, rdf.Term) {
				lower := mustLanguageLiteral(t, "o", "en-gb")
				upper := mustLanguageLiteral(t, "o", "EN-GB")
				return tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", lower),
					tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", upper)
			},
			want: true,
		},
		{
			name: "a triple term of a different depth is not equal",
			build: func(*testing.T) (rdf.TripleTerm, rdf.Term) {
				inner := tripleTerm(rdf.IRI("http://example.com/s2"), "http://example.com/p2", rdf.NewLiteral("o"))
				return tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", inner),
					tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", rdf.NewLiteral("o"))
			},
		},
		{
			name: "zero triple terms are equal",
			build: func(*testing.T) (rdf.TripleTerm, rdf.Term) {
				return rdf.TripleTerm{}, rdf.TripleTerm{}
			},
			want: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			term, other := test.build(t)

			if got := term.Equal(other); got != test.want {
				t.Errorf("Equal() = %t, want %t", got, test.want)
			}

			// Term equality is symmetric, and a comparison written the other
			// way round has to agree.
			if o, ok := other.(rdf.TripleTerm); ok {
				if got := o.Equal(term); got != test.want {
					t.Errorf("reversed Equal() = %t, want %t", got, test.want)
				}
			}
		})
	}
}

func TestTripleTermValidate(t *testing.T) {
	tests := []struct {
		name string
		term func(t *testing.T) rdf.TripleTerm
		want error
	}{
		{
			name: "an iri subject is valid",
			term: func(*testing.T) rdf.TripleTerm {
				return tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", rdf.IRI("http://example.com/o"))
			},
		},
		{
			name: "a blank node subject is valid",
			term: func(*testing.T) rdf.TripleTerm {
				return tripleTerm(rdf.NewBlankNode("b0"), "http://example.com/p", rdf.NewLiteral("o"))
			},
		},
		{
			name: "a nested triple term object is valid",
			term: func(*testing.T) rdf.TripleTerm {
				inner := tripleTerm(rdf.IRI("http://example.com/s2"), "http://example.com/p2", rdf.NewLiteral("o"))
				return tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", inner)
			},
		},
		{
			name: "a triple term subject is rejected",
			term: func(*testing.T) rdf.TripleTerm {
				inner := tripleTerm(rdf.IRI("http://example.com/s2"), "http://example.com/p2", rdf.NewLiteral("o"))
				return tripleTerm(inner, "http://example.com/p", rdf.NewLiteral("o"))
			},
			want: rdf.ErrInvalidSubject,
		},
		{
			name: "a triple term subject nested two deep is rejected",
			term: func(*testing.T) rdf.TripleTerm {
				deepest := tripleTerm(rdf.IRI("http://example.com/s3"), "http://example.com/p3", rdf.NewLiteral("o"))
				inner := tripleTerm(deepest, "http://example.com/p2", rdf.NewLiteral("o"))
				return tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", inner)
			},
			want: rdf.ErrInvalidSubject,
		},
		{
			name: "a literal subject is rejected",
			term: func(*testing.T) rdf.TripleTerm {
				return tripleTerm(rdf.NewLiteral("s"), "http://example.com/p", rdf.NewLiteral("o"))
			},
			want: rdf.ErrInvalidSubject,
		},
		{
			name: "a missing subject is rejected",
			term: func(*testing.T) rdf.TripleTerm {
				return tripleTerm(nil, "http://example.com/p", rdf.NewLiteral("o"))
			},
			want: rdf.ErrInvalidSubject,
		},
		{
			name: "an empty predicate is rejected",
			term: func(*testing.T) rdf.TripleTerm {
				return tripleTerm(rdf.IRI("http://example.com/s"), "", rdf.NewLiteral("o"))
			},
			want: rdf.ErrInvalidPredicate,
		},
		{
			name: "a missing object is rejected",
			term: func(*testing.T) rdf.TripleTerm {
				return tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", nil)
			},
			want: rdf.ErrInvalidObject,
		},
		{
			name: "a missing object nested one deep is rejected",
			term: func(*testing.T) rdf.TripleTerm {
				inner := tripleTerm(rdf.IRI("http://example.com/s2"), "http://example.com/p2", nil)
				return tripleTerm(rdf.IRI("http://example.com/s"), "http://example.com/p", inner)
			},
			want: rdf.ErrInvalidObject,
		},
		{
			name: "the zero triple term is rejected",
			term: func(*testing.T) rdf.TripleTerm { return rdf.TripleTerm{} },
			want: rdf.ErrInvalidSubject,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.term(t).Validate()
			if !errors.Is(err, test.want) {
				t.Errorf("Validate() = %v, want %v", err, test.want)
			}
		})
	}
}

// TestTripleValidateWithTripleTerms covers the two rules a statement enforces
// about triple terms: one may stand as the object, and nowhere else.
func TestTripleValidateWithTripleTerms(t *testing.T) {
	valid := tripleTerm(rdf.IRI("http://example.com/s2"), "http://example.com/p2", rdf.NewLiteral("o"))

	tests := []struct {
		name   string
		triple rdf.Triple
		want   error
	}{
		{
			name: "a triple term object is valid",
			triple: rdf.Triple{
				Subject:   rdf.IRI("http://example.com/s"),
				Predicate: "http://example.com/p",
				Object:    valid,
			},
		},
		{
			name: "a triple term subject is rejected",
			triple: rdf.Triple{
				Subject:   valid,
				Predicate: "http://example.com/p",
				Object:    rdf.NewLiteral("o"),
			},
			want: rdf.ErrInvalidSubject,
		},
		{
			name: "a malformed triple term object is rejected",
			triple: rdf.Triple{
				Subject:   rdf.IRI("http://example.com/s"),
				Predicate: "http://example.com/p",
				Object:    tripleTerm(rdf.NewLiteral("s2"), "http://example.com/p2", rdf.NewLiteral("o")),
			},
			want: rdf.ErrInvalidSubject,
		},
		{
			name: "a malformed triple term nested in the object is rejected",
			triple: rdf.Triple{
				Subject:   rdf.IRI("http://example.com/s"),
				Predicate: "http://example.com/p",
				Object: tripleTerm(
					rdf.IRI("http://example.com/s2"),
					"http://example.com/p2",
					tripleTerm(rdf.IRI("http://example.com/s3"), "", rdf.NewLiteral("o")),
				),
			},
			want: rdf.ErrInvalidPredicate,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.triple.Validate()
			if !errors.Is(err, test.want) {
				t.Errorf("Validate() = %v, want %v", err, test.want)
			}
		})
	}
}

// TestTripleTermIsRejectedAsAGraphName covers the graph label position, which
// takes fewer kinds of term than the subject does and must refuse this one
// too.
func TestTripleTermIsRejectedAsAGraphName(t *testing.T) {
	quad := rdf.Quad{
		Triple: rdf.Triple{
			Subject:   rdf.IRI("http://example.com/s"),
			Predicate: "http://example.com/p",
			Object:    rdf.NewLiteral("o"),
		},
		Graph: tripleTerm(rdf.IRI("http://example.com/s2"), "http://example.com/p2", rdf.NewLiteral("o")),
	}

	err := quad.Validate()
	if !errors.Is(err, rdf.ErrInvalidGraphName) {
		t.Fatalf("Validate() = %v, want %v", err, rdf.ErrInvalidGraphName)
	}
	if want := "rdf.TripleTerm <<("; !strings.Contains(err.Error(), want) {
		t.Errorf("Validate() = %q, want it to mention %s", err, want)
	}
}

// TestTripleTermIsATerm covers the sealed interface: a triple term is one of
// the four kinds a type switch over a [rdf.Term] has to account for.
func TestTripleTermIsATerm(t *testing.T) {
	var term rdf.Term = tripleTerm(
		rdf.IRI("http://example.com/s"),
		"http://example.com/p",
		rdf.NewLiteral("o"),
	)

	if _, ok := term.(rdf.TripleTerm); !ok {
		t.Fatalf("term is a %T, want a rdf.TripleTerm", term)
	}
}
