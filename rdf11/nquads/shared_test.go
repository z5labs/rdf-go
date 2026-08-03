package nquads_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/rdf11/nquads"
)

// The behaviour covered here is N-Triples' rather than N-Quads', reaching this
// package by way of the shared grammar. It is exercised again because the two
// copies are separate code until internal/lex takes them over, and a fix to
// one that missed the other would show up nowhere else.

func TestDecodeLiteralDatatypes(t *testing.T) {
	testCases := []struct {
		name  string
		src   string
		build func(t *testing.T) rdf.Literal
	}{
		{
			name: "a plain literal takes the xsd:string datatype",
			src:  `<http://e/s> <http://e/p> "text" <http://e/g> .`,
			build: func(*testing.T) rdf.Literal {
				return rdf.NewLiteral("text")
			},
		},
		{
			name: "a tagged literal takes the rdf:langString datatype",
			src:  `<http://e/s> <http://e/p> "text"@en-GB <http://e/g> .`,
			build: func(t *testing.T) rdf.Literal {
				literal, err := rdf.NewLanguageLiteral("text", "en-GB")
				if err != nil {
					t.Fatalf("NewLanguageLiteral() = %v", err)
				}
				return literal
			},
		},
		{
			name: "a typed literal keeps what it was written with",
			src:  `<http://e/s> <http://e/p> "1"^^<http://e/dt> <http://e/g> .`,
			build: func(t *testing.T) rdf.Literal {
				literal, err := rdf.NewTypedLiteral("1", "http://e/dt")
				if err != nil {
					t.Fatalf("NewTypedLiteral() = %v", err)
				}
				return literal
			},
		},
		{
			name: "escapes are decoded on the way through",
			src:  `<http://e/s> <http://e/p> "a\nbé" <http://e/g> .`,
			build: func(*testing.T) rdf.Literal {
				return rdf.NewLiteral("a\nbé")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			quads := decode(t, tc.src)
			if len(quads) != 1 {
				t.Fatalf("decoded %d quads, want 1", len(quads))
			}

			want := tc.build(t)
			if !quads[0].Object.Equal(want) {
				t.Errorf("Object = %s, want %s", quads[0].Object, want)
			}
		})
	}
}

func TestDecodeDropsComments(t *testing.T) {
	src := "# a note\n<http://e/s> <http://e/p> <http://e/o> <http://e/g> . # trailing\n# last\n"

	if got, want := len(decode(t, src)), 1; got != want {
		t.Errorf("decoded %d quads, want %d", got, want)
	}
}

func TestErrorsCarryTheirPosition(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name:        "a character that begins no terminal",
			src:         `$ <http://e/p> <http://e/o> .`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: pos(1, 1), R: '$'},
		},
		{
			name:        "an unterminated iri",
			src:         "<http://e/s\n",
			expectedErr: nquads.UnterminatedIRIRefError{Pos: pos(1, 1)},
		},
		{
			name:        "an unterminated literal",
			src:         `<http://e/s> <http://e/p> "oops`,
			expectedErr: nquads.UnterminatedStringError{Pos: pos(1, 27)},
		},
		{
			name:        "an rdf 1.2 base direction, which this package does not read",
			src:         `<http://e/s> <http://e/p> "v"@en--ltr <http://e/g> .`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: pos(1, 34), R: '-'},
		},
		{
			name:        "an rdf 1.2 triple term opener",
			src:         `<<( <http://e/s> <http://e/p> <http://e/o> )>> .`,
			expectedErr: nquads.UnexpectedCharacterError{Pos: pos(1, 2), R: '<'},
		},
		{
			name:        "input cut off part way through a terminal",
			src:         `<http://e/s> <http://e/p> "v"@`,
			expectedErr: nquads.UnexpectedEndOfInputError{Pos: pos(1, 31)},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("through Parse", func(t *testing.T) {
				if _, err := nquads.Parse(strings.NewReader(tc.src)); err != tc.expectedErr {
					t.Errorf("Parse() error = %v, want %v", err, tc.expectedErr)
				}
			})
			t.Run("through Decode", func(t *testing.T) {
				if _, err := collectQuads(nquads.Decode(strings.NewReader(tc.src))); err != tc.expectedErr {
					t.Errorf("Decode() error = %v, want %v", err, tc.expectedErr)
				}
			})
		})
	}
}

func TestTwoStatementsOnOneLine(t *testing.T) {
	src := `<http://e/a> <http://e/p> <http://e/o> . <http://e/b> <http://e/p> <http://e/o> .`

	_, err := nquads.Parse(strings.NewReader(src))
	want := nquads.UnexpectedTokenError{
		Expected: []nquads.TokenType{nquads.TokenEOL, nquads.TokenComment},
		Actual: nquads.Token{
			Pos:   pos(1, 42),
			Type:  nquads.TokenIRIRef,
			Value: []byte("http://e/b"),
		},
	}
	if !errors.Is(err, error(want)) && err.Error() != want.Error() {
		t.Errorf("Parse() error = %#v, want %#v", err, want)
	}
}

func TestReaderError(t *testing.T) {
	errBoom := errors.New("boom")
	src := []byte("<http://e/s> <http://e/p> <http://e/o>")

	t.Run("through Parse", func(t *testing.T) {
		_, err := nquads.Parse(&failingReader{data: src, err: errBoom})
		if !errors.Is(err, errBoom) {
			t.Errorf("Parse() error = %v, want %v", err, errBoom)
		}
	})
	t.Run("through Decode", func(t *testing.T) {
		_, err := collectQuads(nquads.Decode(&failingReader{data: src, err: errBoom}))
		if !errors.Is(err, errBoom) {
			t.Errorf("Decode() error = %v, want %v", err, errBoom)
		}
	})
}

func TestDecodeStopsEarly(t *testing.T) {
	src := "<http://e/a> <http://e/p> \"1\" .\n<http://e/b> <http://e/p> \"2\" .\n"

	var seen int
	for range nquads.Decode(strings.NewReader(src)) {
		seen++
		break
	}
	if want := 1; seen != want {
		t.Errorf("yielded %d quads after break, want %d", seen, want)
	}
}

func TestDecodeYieldsNothingAfterAnError(t *testing.T) {
	src := "<http://e/a> <http://e/p> \"1\" .\n$ <http://e/p> \"2\" .\n<http://e/c> <http://e/p> \"3\" .\n"

	var (
		quads []rdf.Quad
		errs  []error
	)
	for quad, err := range nquads.Decode(strings.NewReader(src)) {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		quads = append(quads, quad)
	}

	if len(errs) != 1 {
		t.Fatalf("yielded %d errors, want 1", len(errs))
	}
	if len(quads) != 1 {
		t.Fatalf("yielded %d quads, want 1", len(quads))
	}
}

func TestTermPositions(t *testing.T) {
	doc := parse(t, `<http://e/s> <http://e/p> "v"^^<http://e/dt> <http://e/g> .`)
	if len(doc.Quads) != 1 {
		t.Fatalf("parsed %d statements, want 1", len(doc.Quads))
	}

	quad := doc.Quads[0]
	literal, ok := quad.Object.(*nquads.Literal)
	if !ok {
		t.Fatalf("object is %T, want *nquads.Literal", quad.Object)
	}

	positions := []struct {
		name string
		got  nquads.Pos
		want nquads.Pos
	}{
		{name: "document", got: doc.Pos, want: pos(1, 1)},
		{name: "statement", got: quad.Pos, want: pos(1, 1)},
		{name: "subject", got: quad.Subject.Position(), want: pos(1, 1)},
		{name: "predicate", got: quad.Predicate.Position(), want: pos(1, 14)},
		{name: "object", got: quad.Object.Position(), want: pos(1, 27)},
		{name: "datatype", got: literal.Datatype.Position(), want: pos(1, 32)},
		{name: "graph", got: quad.Graph.Position(), want: pos(1, 46)},
	}

	for _, p := range positions {
		t.Run(p.name, func(t *testing.T) {
			if p.got != p.want {
				t.Errorf("Pos = %s, want %s", p.got, p.want)
			}
		})
	}
}

func TestParseErrorMessages(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "unexpected token",
			err: nquads.UnexpectedTokenError{
				Expected: []nquads.TokenType{nquads.TokenIRIRef, nquads.TokenBlankNodeLabel},
				Actual: nquads.Token{
					Pos:   pos(2, 7),
					Type:  nquads.TokenStringLiteral,
					Value: []byte("v"),
				},
			},
			want: "unexpected token at line 2, column 7: StringLiteral(v), " +
				"expected one of: IRIRef, BlankNodeLabel",
		},
		{
			name: "unexpected end of tokens",
			err: nquads.UnexpectedEndOfTokensError{
				Expected: []nquads.TokenType{nquads.TokenDot},
				Pos:      pos(4, 12),
			},
			want: "unexpected end of tokens at line 4, column 12, expected one of: Dot",
		},
		{
			name: "invalid term",
			err:  nquads.InvalidTermError{Pos: pos(3, 12), Err: rdf.ErrEmptyDatatype},
			want: "invalid term at line 3, column 12: rdf: datatype must not be empty",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

// manyStatements builds a document long enough that writing it fills the
// output buffer several times over.
func manyStatements(count int) string {
	var b strings.Builder
	for i := range count {
		fmt.Fprintf(&b, "<http://example.com/subject%d> <http://example.com/predicate> \"value %d\" <http://example.com/graph> .\n", i, i)
	}
	return b.String()
}

// TestWriteFailsPartWayThrough covers the failure a small document cannot
// reach: output is buffered, so a writer that refuses everything is not
// noticed until the flush at the end.
func TestWriteFailsPartWayThrough(t *testing.T) {
	const allow = 5000
	errBoom := errors.New("boom")

	src := manyStatements(500)

	t.Run("Print", func(t *testing.T) {
		w := &failingWriter{allow: allow, err: errBoom}
		if err := nquads.Print(w, parse(t, src)); !errors.Is(err, errBoom) {
			t.Errorf("Print() = %v, want %v", err, errBoom)
		}
	})

	t.Run("Encode", func(t *testing.T) {
		w := &failingWriter{allow: allow, err: errBoom}
		if err := nquads.Encode(w, slices.Values(decode(t, src))); !errors.Is(err, errBoom) {
			t.Errorf("Encode() = %v, want %v", err, errBoom)
		}
	})
}
