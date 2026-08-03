package rdf_test

import (
	"errors"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
)

// mustLanguageLiteral fails the test if the literal cannot be constructed. The
// constructors reject invariant violations, none of which these tables intend
// to exercise.
func mustLanguageLiteral(t *testing.T, value, language string) rdf.Literal {
	t.Helper()

	l, err := rdf.NewLanguageLiteral(value, language)
	if err != nil {
		t.Fatalf("NewLanguageLiteral(%q, %q) = %v", value, language, err)
	}
	return l
}

func mustTypedLiteral(t *testing.T, value string, datatype rdf.IRI) rdf.Literal {
	t.Helper()

	l, err := rdf.NewTypedLiteral(value, datatype)
	if err != nil {
		t.Fatalf("NewTypedLiteral(%q, %q) = %v", value, datatype, err)
	}
	return l
}

func TestTripleValidate(t *testing.T) {
	tests := []struct {
		name   string
		triple rdf.Triple
		want   error
	}{
		{
			name: "an iri subject is valid",
			triple: rdf.Triple{
				Subject:   rdf.IRI("http://example.com/s"),
				Predicate: "http://example.com/p",
				Object:    rdf.IRI("http://example.com/o"),
			},
		},
		{
			name: "a blank node subject is valid",
			triple: rdf.Triple{
				Subject:   rdf.NewBlankNode("b0"),
				Predicate: "http://example.com/p",
				Object:    rdf.IRI("http://example.com/o"),
			},
		},
		{
			name: "a literal object is valid",
			triple: rdf.Triple{
				Subject:   rdf.IRI("http://example.com/s"),
				Predicate: "http://example.com/p",
				Object:    rdf.NewLiteral("o"),
			},
		},
		{
			name: "a blank node object is valid",
			triple: rdf.Triple{
				Subject:   rdf.IRI("http://example.com/s"),
				Predicate: "http://example.com/p",
				Object:    rdf.NewBlankNode("b0"),
			},
		},
		{
			name: "a relative predicate is not rejected here",
			triple: rdf.Triple{
				Subject:   rdf.IRI("http://example.com/s"),
				Predicate: "p",
				Object:    rdf.IRI("http://example.com/o"),
			},
		},
		{
			name: "a literal subject is rejected",
			triple: rdf.Triple{
				Subject:   rdf.NewLiteral("s"),
				Predicate: "http://example.com/p",
				Object:    rdf.IRI("http://example.com/o"),
			},
			want: rdf.ErrInvalidSubject,
		},
		{
			name: "a missing subject is rejected",
			triple: rdf.Triple{
				Predicate: "http://example.com/p",
				Object:    rdf.IRI("http://example.com/o"),
			},
			want: rdf.ErrInvalidSubject,
		},
		{
			name: "an empty predicate is rejected",
			triple: rdf.Triple{
				Subject: rdf.IRI("http://example.com/s"),
				Object:  rdf.IRI("http://example.com/o"),
			},
			want: rdf.ErrInvalidPredicate,
		},
		{
			name: "a missing object is rejected",
			triple: rdf.Triple{
				Subject:   rdf.IRI("http://example.com/s"),
				Predicate: "http://example.com/p",
			},
			want: rdf.ErrInvalidObject,
		},
		{
			name:   "the zero triple is rejected",
			triple: rdf.Triple{},
			want:   rdf.ErrInvalidSubject,
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

// TestValidateNamesTheOffendingTerm covers the context wrapped around the
// position errors: a caller reading the message should see which term was
// rejected, not only that one was.
func TestValidateNamesTheOffendingTerm(t *testing.T) {
	triple := rdf.Triple{
		Subject:   rdf.IRI("http://example.com/s"),
		Predicate: "http://example.com/p",
		Object:    rdf.NewLiteral("o"),
	}

	tests := []struct {
		name     string
		validate func() error
		want     string
	}{
		{
			name: "a literal subject",
			validate: func() error {
				invalid := triple
				invalid.Subject = rdf.NewLiteral("s")
				return invalid.Validate()
			},
			want: `rdf.Literal "s"`,
		},
		{
			name: "a missing subject",
			validate: func() error {
				invalid := triple
				invalid.Subject = nil
				return invalid.Validate()
			},
			want: "<nil>",
		},
		{
			name: "a literal graph name",
			validate: func() error {
				return rdf.Quad{Triple: triple, Graph: rdf.NewLiteral("g")}.Validate()
			},
			want: `rdf.Literal "g"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.validate()
			if err == nil {
				t.Fatal("Validate() = nil, want an error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("Validate() = %q, want it to mention %s", err, test.want)
			}
		})
	}
}

func TestTripleEqual(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T) (rdf.Triple, rdf.Triple)
		want  bool
	}{
		{
			name: "identical triples are equal",
			build: func(*testing.T) (rdf.Triple, rdf.Triple) {
				tr := rdf.Triple{
					Subject:   rdf.IRI("http://example.com/s"),
					Predicate: "http://example.com/p",
					Object:    rdf.NewLiteral("o"),
				}
				return tr, tr
			},
			want: true,
		},
		{
			name: "language tags compare case insensitively",
			build: func(t *testing.T) (rdf.Triple, rdf.Triple) {
				return rdf.Triple{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    mustLanguageLiteral(t, "o", "en-GB"),
					}, rdf.Triple{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    mustLanguageLiteral(t, "o", "EN-gb"),
					}
			},
			want: true,
		},
		{
			name: "a different subject is not equal",
			build: func(*testing.T) (rdf.Triple, rdf.Triple) {
				return rdf.Triple{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    rdf.NewLiteral("o"),
					}, rdf.Triple{
						Subject:   rdf.NewBlankNode("s"),
						Predicate: "http://example.com/p",
						Object:    rdf.NewLiteral("o"),
					}
			},
		},
		{
			name: "a different predicate is not equal",
			build: func(*testing.T) (rdf.Triple, rdf.Triple) {
				return rdf.Triple{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    rdf.NewLiteral("o"),
					}, rdf.Triple{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/q",
						Object:    rdf.NewLiteral("o"),
					}
			},
		},
		{
			name: "a different object is not equal",
			build: func(*testing.T) (rdf.Triple, rdf.Triple) {
				return rdf.Triple{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    rdf.NewLiteral("o"),
					}, rdf.Triple{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    rdf.NewLiteral("other"),
					}
			},
		},
		{
			name: "two zero triples are equal",
			build: func(*testing.T) (rdf.Triple, rdf.Triple) {
				return rdf.Triple{}, rdf.Triple{}
			},
			want: true,
		},
		{
			name: "a missing object never equals a present one",
			build: func(*testing.T) (rdf.Triple, rdf.Triple) {
				return rdf.Triple{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
					}, rdf.Triple{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    rdf.NewLiteral("o"),
					}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			a, b := test.build(t)
			if got := a.Equal(b); got != test.want {
				t.Errorf("Equal() = %t, want %t", got, test.want)
			}
			if got := b.Equal(a); got != test.want {
				t.Errorf("Equal() is not symmetric: reversed = %t, want %t", got, test.want)
			}
		})
	}
}

func TestTripleString(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T) rdf.Triple
		want  string
	}{
		{
			name: "iri object",
			build: func(*testing.T) rdf.Triple {
				return rdf.Triple{
					Subject:   rdf.IRI("http://example.com/s"),
					Predicate: "http://example.com/p",
					Object:    rdf.IRI("http://example.com/o"),
				}
			},
			want: "<http://example.com/s> <http://example.com/p> <http://example.com/o> .",
		},
		{
			name: "blank node subject and typed literal object",
			build: func(t *testing.T) rdf.Triple {
				return rdf.Triple{
					Subject:   rdf.NewBlankNode("b0"),
					Predicate: "http://example.com/p",
					Object:    mustTypedLiteral(t, "1", "http://www.w3.org/2001/XMLSchema#integer"),
				}
			},
			want: `_:b0 <http://example.com/p> "1"^^<http://www.w3.org/2001/XMLSchema#integer> .`,
		},
		{
			name: "an unvalidated triple renders its empty positions",
			build: func(*testing.T) rdf.Triple {
				return rdf.Triple{Predicate: "http://example.com/p"}
			},
			want: "<nil> <http://example.com/p> <nil> .",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.build(t).String(); got != test.want {
				t.Errorf("String() = %s, want %s", got, test.want)
			}
		})
	}
}
