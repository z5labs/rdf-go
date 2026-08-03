package rdf_test

import (
	"errors"
	"testing"

	rdf "github.com/z5labs/rdf-go"
)

func TestNewLiteral(t *testing.T) {
	t.Run("defaults to xsd:string", func(t *testing.T) {
		l := rdf.NewLiteral("a")

		if got, want := l.Value(), "a"; got != want {
			t.Errorf("Value() = %q, want %q", got, want)
		}
		if got, want := l.Datatype(), rdf.XSDString; got != want {
			t.Errorf("Datatype() = %q, want %q", got, want)
		}
		if got := l.Language(); got != "" {
			t.Errorf("Language() = %q, want empty", got)
		}
		if got, want := l.Direction(), rdf.DirectionNone; got != want {
			t.Errorf("Direction() = %q, want %q", got, want)
		}
	})

	t.Run("the zero literal is the empty xsd:string literal", func(t *testing.T) {
		var l rdf.Literal

		if got := l.Value(); got != "" {
			t.Errorf("Value() = %q, want empty", got)
		}
		if got, want := l.Datatype(), rdf.XSDString; got != want {
			t.Errorf("Datatype() = %q, want %q", got, want)
		}
		if got, want := l.String(), `""`; got != want {
			t.Errorf("String() = %s, want %s", got, want)
		}
		if !l.Equal(rdf.NewLiteral("")) {
			t.Error("zero literal is not equal to the empty xsd:string literal")
		}
	})
}

func TestNewTypedLiteral(t *testing.T) {
	t.Run("accepted", func(t *testing.T) {
		const dt = rdf.IRI("http://example.com/dt")

		l, err := rdf.NewTypedLiteral("a", dt)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := l.Value(), "a"; got != want {
			t.Errorf("Value() = %q, want %q", got, want)
		}
		if got := l.Datatype(); got != dt {
			t.Errorf("Datatype() = %q, want %q", got, dt)
		}
		if got := l.Language(); got != "" {
			t.Errorf("Language() = %q, want empty", got)
		}
	})

	t.Run("xsd:string may be given explicitly", func(t *testing.T) {
		l, err := rdf.NewTypedLiteral("a", rdf.XSDString)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !l.Equal(rdf.NewLiteral("a")) {
			t.Error("explicit xsd:string literal differs from the default one")
		}
	})

	rejected := []struct {
		name     string
		datatype rdf.IRI
		want     error
	}{
		{
			name:     "empty datatype",
			datatype: "",
			want:     rdf.ErrEmptyDatatype,
		},
		{
			name:     "rdf:langString requires a language tag",
			datatype: rdf.RDFLangString,
			want:     rdf.ErrReservedDatatype,
		},
		{
			name:     "rdf:dirLangString requires a language tag",
			datatype: rdf.RDFDirLangString,
			want:     rdf.ErrReservedDatatype,
		},
	}

	for _, test := range rejected {
		t.Run("rejected/"+test.name, func(t *testing.T) {
			l, err := rdf.NewTypedLiteral("a", test.datatype)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if (l != rdf.Literal{}) {
				t.Errorf("literal = %v, want the zero value on error", l)
			}
		})
	}
}

func TestNewLanguageLiteral(t *testing.T) {
	t.Run("accepted", func(t *testing.T) {
		l, err := rdf.NewLanguageLiteral("colour", "en-GB")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := l.Value(), "colour"; got != want {
			t.Errorf("Value() = %q, want %q", got, want)
		}
		if got, want := l.Language(), "en-GB"; got != want {
			t.Errorf("Language() = %q, want %q — tags are preserved as written", got, want)
		}
		if got, want := l.Datatype(), rdf.RDFLangString; got != want {
			t.Errorf("Datatype() = %q, want %q", got, want)
		}
		if got, want := l.Direction(), rdf.DirectionNone; got != want {
			t.Errorf("Direction() = %q, want %q", got, want)
		}
	})

	t.Run("rejected/empty language tag", func(t *testing.T) {
		l, err := rdf.NewLanguageLiteral("a", "")
		if !errors.Is(err, rdf.ErrEmptyLanguage) {
			t.Fatalf("error = %v, want %v", err, rdf.ErrEmptyLanguage)
		}
		if (l != rdf.Literal{}) {
			t.Errorf("literal = %v, want the zero value on error", l)
		}
	})
}

func TestNewDirectionalLiteral(t *testing.T) {
	accepted := []struct {
		name      string
		direction rdf.Direction
	}{
		{name: "left to right", direction: rdf.DirectionLTR},
		{name: "right to left", direction: rdf.DirectionRTL},
	}

	for _, test := range accepted {
		t.Run("accepted/"+test.name, func(t *testing.T) {
			l, err := rdf.NewDirectionalLiteral("a", "en", test.direction)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := l.Direction(); got != test.direction {
				t.Errorf("Direction() = %q, want %q", got, test.direction)
			}
			if got, want := l.Datatype(), rdf.RDFDirLangString; got != want {
				t.Errorf("Datatype() = %q, want %q", got, want)
			}
			if got, want := l.Language(), "en"; got != want {
				t.Errorf("Language() = %q, want %q", got, want)
			}
		})
	}

	rejected := []struct {
		name      string
		language  string
		direction rdf.Direction
		want      error
	}{
		{
			name:      "a direction requires a language tag",
			language:  "",
			direction: rdf.DirectionLTR,
			want:      rdf.ErrEmptyLanguage,
		},
		{
			name:      "the none direction is not a direction",
			language:  "en",
			direction: rdf.DirectionNone,
			want:      rdf.ErrInvalidDirection,
		},
		{
			name:      "direction is case sensitive",
			language:  "en",
			direction: rdf.Direction("LTR"),
			want:      rdf.ErrInvalidDirection,
		},
		{
			name:      "unknown direction",
			language:  "en",
			direction: rdf.Direction("up"),
			want:      rdf.ErrInvalidDirection,
		},
	}

	for _, test := range rejected {
		t.Run("rejected/"+test.name, func(t *testing.T) {
			l, err := rdf.NewDirectionalLiteral("a", test.language, test.direction)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if (l != rdf.Literal{}) {
				t.Errorf("literal = %v, want the zero value on error", l)
			}
		})
	}
}

func TestLiteralEqual(t *testing.T) {
	mustLang := func(t *testing.T, value, language string) rdf.Literal {
		t.Helper()
		l, err := rdf.NewLanguageLiteral(value, language)
		if err != nil {
			t.Fatalf("NewLanguageLiteral(%q, %q): %v", value, language, err)
		}
		return l
	}
	mustDir := func(t *testing.T, value, language string, d rdf.Direction) rdf.Literal {
		t.Helper()
		l, err := rdf.NewDirectionalLiteral(value, language, d)
		if err != nil {
			t.Fatalf("NewDirectionalLiteral(%q, %q, %q): %v", value, language, d, err)
		}
		return l
	}
	mustTyped := func(t *testing.T, value string, dt rdf.IRI) rdf.Literal {
		t.Helper()
		l, err := rdf.NewTypedLiteral(value, dt)
		if err != nil {
			t.Fatalf("NewTypedLiteral(%q, %q): %v", value, dt, err)
		}
		return l
	}

	tests := []struct {
		name  string
		build func(*testing.T) (rdf.Literal, rdf.Term)
		want  bool
	}{
		{
			name: "same value and datatype are equal",
			build: func(t *testing.T) (rdf.Literal, rdf.Term) {
				return rdf.NewLiteral("a"), rdf.NewLiteral("a")
			},
			want: true,
		},
		{
			name: "different values are not equal",
			build: func(t *testing.T) (rdf.Literal, rdf.Term) {
				return rdf.NewLiteral("a"), rdf.NewLiteral("b")
			},
			want: false,
		},
		{
			name: "different datatypes are not equal",
			build: func(t *testing.T) (rdf.Literal, rdf.Term) {
				return rdf.NewLiteral("1"), mustTyped(t, "1", rdf.IRI(rdf.NamespaceXSD+"integer"))
			},
			want: false,
		},
		{
			name: "language tags compare case insensitively",
			build: func(t *testing.T) (rdf.Literal, rdf.Term) {
				return mustLang(t, "a", "en-GB"), mustLang(t, "a", "EN-gb")
			},
			want: true,
		},
		{
			name: "different language tags are not equal",
			build: func(t *testing.T) (rdf.Literal, rdf.Term) {
				return mustLang(t, "a", "en"), mustLang(t, "a", "fr")
			},
			want: false,
		},
		{
			name: "a tagged literal never equals an untagged one",
			build: func(t *testing.T) (rdf.Literal, rdf.Term) {
				return mustLang(t, "a", "en"), rdf.NewLiteral("a")
			},
			want: false,
		},
		{
			name: "same direction is equal",
			build: func(t *testing.T) (rdf.Literal, rdf.Term) {
				return mustDir(t, "a", "en", rdf.DirectionLTR), mustDir(t, "a", "EN", rdf.DirectionLTR)
			},
			want: true,
		},
		{
			name: "different directions are not equal",
			build: func(t *testing.T) (rdf.Literal, rdf.Term) {
				return mustDir(t, "a", "en", rdf.DirectionLTR), mustDir(t, "a", "en", rdf.DirectionRTL)
			},
			want: false,
		},
		{
			name: "a directional literal never equals a merely tagged one",
			build: func(t *testing.T) (rdf.Literal, rdf.Term) {
				return mustDir(t, "a", "en", rdf.DirectionLTR), mustLang(t, "a", "en")
			},
			want: false,
		},
		{
			name: "a literal never equals an iri",
			build: func(t *testing.T) (rdf.Literal, rdf.Term) {
				return rdf.NewLiteral("a"), rdf.IRI("a")
			},
			want: false,
		},
		{
			name: "a literal never equals a blank node",
			build: func(t *testing.T) (rdf.Literal, rdf.Term) {
				return rdf.NewLiteral("a"), rdf.NewBlankNode("a")
			},
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			l, other := test.build(t)
			if got := l.Equal(other); got != test.want {
				t.Errorf("Equal(%v) = %t, want %t", other, got, test.want)
			}
		})
	}
}

func TestLiteralString(t *testing.T) {
	typed := func(t *testing.T, value string, dt rdf.IRI) rdf.Literal {
		t.Helper()
		l, err := rdf.NewTypedLiteral(value, dt)
		if err != nil {
			t.Fatalf("NewTypedLiteral: %v", err)
		}
		return l
	}

	tests := []struct {
		name  string
		build func(*testing.T) rdf.Literal
		want  string
	}{
		{
			name:  "xsd:string omits the datatype",
			build: func(t *testing.T) rdf.Literal { return rdf.NewLiteral("a") },
			want:  `"a"`,
		},
		{
			name: "other datatypes are written out",
			build: func(t *testing.T) rdf.Literal {
				return typed(t, "1", rdf.IRI(rdf.NamespaceXSD+"integer"))
			},
			want: `"1"^^<http://www.w3.org/2001/XMLSchema#integer>`,
		},
		{
			name: "a language tag replaces the datatype",
			build: func(t *testing.T) rdf.Literal {
				l, err := rdf.NewLanguageLiteral("a", "en")
				if err != nil {
					t.Fatalf("NewLanguageLiteral: %v", err)
				}
				return l
			},
			want: `"a"@en`,
		},
		{
			name: "a direction follows the language tag",
			build: func(t *testing.T) rdf.Literal {
				l, err := rdf.NewDirectionalLiteral("a", "ar", rdf.DirectionRTL)
				if err != nil {
					t.Fatalf("NewDirectionalLiteral: %v", err)
				}
				return l
			},
			want: `"a"@ar--rtl`,
		},
		{
			name: "language tag case is preserved",
			build: func(t *testing.T) rdf.Literal {
				l, err := rdf.NewLanguageLiteral("a", "en-GB")
				if err != nil {
					t.Fatalf("NewLanguageLiteral: %v", err)
				}
				return l
			},
			want: `"a"@en-GB`,
		},
		{
			name:  "quotes and backslashes are escaped",
			build: func(t *testing.T) rdf.Literal { return rdf.NewLiteral(`a"b\c`) },
			want:  `"a\"b\\c"`,
		},
		{
			name:  "line feed and carriage return are escaped",
			build: func(t *testing.T) rdf.Literal { return rdf.NewLiteral("a\nb\rc") },
			want:  `"a\nb\rc"`,
		},
		{
			name:  "a tab is canonical written literally",
			build: func(t *testing.T) rdf.Literal { return rdf.NewLiteral("a\tb") },
			want:  "\"a\tb\"",
		},
		{
			name:  "non-ascii passes through unescaped",
			build: func(t *testing.T) rdf.Literal { return rdf.NewLiteral("héllo") },
			want:  `"héllo"`,
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
