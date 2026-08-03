package ntriples_test

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
	ntriples11 "github.com/z5labs/rdf-go/rdf11/ntriples"
	"github.com/z5labs/rdf-go/rdf12/ntriples"
)

func printToString(t *testing.T, doc *ntriples.Document) string {
	t.Helper()

	var b strings.Builder
	if err := ntriples.Print(&b, doc); err != nil {
		t.Fatalf("Print() = %v, want nil", err)
	}
	return b.String()
}

func encodeToString(t *testing.T, triples []rdf.Triple) string {
	t.Helper()

	var b strings.Builder
	if err := ntriples.Encode(&b, slices.Values(triples)); err != nil {
		t.Fatalf("Encode() = %v, want nil", err)
	}
	return b.String()
}

func graphOfTriples(t *testing.T, triples []rdf.Triple) *rdf.Graph {
	t.Helper()

	g := rdf.NewGraph()
	for _, triple := range triples {
		if err := g.Add(triple); err != nil {
			t.Fatalf("Add(%s) = %v, want nil", triple, err)
		}
	}
	return g
}

// TestPrint pins the bytes a document prints as, for the forms RDF 1.1 and RDF
// 1.2 share.
func TestPrint(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "an empty document prints as nothing",
			src:  "",
			want: "",
		},
		{
			name: "a triple of iris",
			src:  `<http://example.com/s> <http://example.com/p> <http://example.com/o> .`,
			want: "<http://example.com/s> <http://example.com/p> <http://example.com/o> .\n",
		},
		{
			name: "extra spacing is not reproduced",
			src:  "   <http://example.com/s>    <http://example.com/p>\t<http://example.com/o>   .   ",
			want: "<http://example.com/s> <http://example.com/p> <http://example.com/o> .\n",
		},
		{
			name: "a blank node keeps the label it was written with",
			src:  `_:someLabel <http://example.com/p> _:other .`,
			want: "_:someLabel <http://example.com/p> _:other .\n",
		},
		{
			name: "a plain literal",
			src:  `<http://example.com/s> <http://example.com/p> "text" .`,
			want: "<http://example.com/s> <http://example.com/p> \"text\" .\n",
		},
		{
			name: "a language tagged literal keeps the case it was written with",
			src:  `<http://example.com/s> <http://example.com/p> "text"@en-GB .`,
			want: "<http://example.com/s> <http://example.com/p> \"text\"@en-GB .\n",
		},
		{
			name: "a typed literal",
			src:  `<http://example.com/s> <http://example.com/p> "1"^^<http://example.com/dt> .`,
			want: "<http://example.com/s> <http://example.com/p> \"1\"^^<http://example.com/dt> .\n",
		},
		{
			name: "several statements, one to a line",
			src:  "<http://example.com/a> <http://example.com/p> _:x .\n<http://example.com/b> <http://example.com/p> _:y .",
			want: "<http://example.com/a> <http://example.com/p> _:x .\n" +
				"<http://example.com/b> <http://example.com/p> _:y .\n",
		},
		{
			name: "blank lines are not reproduced",
			src:  "<http://example.com/a> <http://example.com/p> _:x .\n\n\n<http://example.com/b> <http://example.com/p> _:y .",
			want: "<http://example.com/a> <http://example.com/p> _:x .\n" +
				"<http://example.com/b> <http://example.com/p> _:y .\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := ntriples.Parse(strings.NewReader(tc.src))
			if err != nil {
				t.Fatalf("Parse() = %v, want nil", err)
			}

			if got := printToString(t, doc); got != tc.want {
				t.Errorf("Print() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPrintTripleTerms covers the criterion that says a triple term is written
// as "<<( s p o )>>", nested ones included. The spacing is §3's: white space
// stands after the subject, the predicate, the object and the "<<(", and
// nowhere else, so the ")>>" is preceded by the space that follows the object
// and by nothing more.
func TestPrintTripleTerms(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "a triple term of iris",
			src:  `<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> <http://e/c> )>> .`,
			want: "<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> <http://e/c> )>> .\n",
		},
		{
			name: "extra spacing inside a triple term is not reproduced",
			src:  "<http://e/s> <http://e/p> <<(<http://e/a>   <http://e/b>\t<http://e/c>)>> .",
			want: "<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> <http://e/c> )>> .\n",
		},
		{
			name: "a triple term with a blank node subject",
			src:  `<http://e/s> <http://e/p> <<( _:b <http://e/b> "v" )>> .`,
			want: "<http://e/s> <http://e/p> <<( _:b <http://e/b> \"v\" )>> .\n",
		},
		{
			name: "a triple term with a literal object",
			src:  `<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> "v"@en )>> .`,
			want: "<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> \"v\"@en )>> .\n",
		},
		{
			name: "a nested triple term",
			src:  `<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> <<( <http://e/x> <http://e/y> <http://e/z> )>> )>> .`,
			want: "<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> <<( <http://e/x> <http://e/y> <http://e/z> )>> )>> .\n",
		},
		{
			name: "triple terms nested three deep",
			src: `<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> ` +
				`<<( <http://e/c> <http://e/d> <<( <http://e/x> <http://e/y> "deep" )>> )>> )>> .`,
			want: "<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> " +
				"<<( <http://e/c> <http://e/d> <<( <http://e/x> <http://e/y> \"deep\" )>> )>> )>> .\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := ntriples.Parse(strings.NewReader(tc.src))
			if err != nil {
				t.Fatalf("Parse() = %v, want nil", err)
			}

			if got := printToString(t, doc); got != tc.want {
				t.Errorf("Print() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPrintDirectionalLiterals covers the criterion that says a base direction
// is written after the language tag as "--dir".
func TestPrintDirectionalLiterals(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "left to right",
			src:  `<http://e/s> <http://e/p> "text"@en--ltr .`,
			want: "<http://e/s> <http://e/p> \"text\"@en--ltr .\n",
		},
		{
			name: "right to left",
			src:  `<http://e/s> <http://e/p> "مرحبا"@ar--rtl .`,
			want: "<http://e/s> <http://e/p> \"مرحبا\"@ar--rtl .\n",
		},
		{
			name: "a tag with subtags keeps them all",
			src:  `<http://e/s> <http://e/p> "text"@en-gb-oed--ltr .`,
			want: "<http://e/s> <http://e/p> \"text\"@en-gb-oed--ltr .\n",
		},
		{
			name: "inside a triple term",
			src:  `<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> "text"@he--rtl )>> .`,
			want: "<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> \"text\"@he--rtl )>> .\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := ntriples.Parse(strings.NewReader(tc.src))
			if err != nil {
				t.Fatalf("Parse() = %v, want nil", err)
			}

			if got := printToString(t, doc); got != tc.want {
				t.Errorf("Print() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPrintEscapesCanonically covers §3's escaping rule, which is as much
// about what must not be escaped as about what must — and which is where RDF
// 1.2 parts from RDF 1.1, whose canonical form escaped four characters where
// this one escapes seven.
func TestPrintEscapesCanonically(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "a quote and a backslash are escaped",
			src:  `<http://e/s> <http://e/p> "a\"b\\c" .`,
			want: `<http://e/s> <http://e/p> "a\"b\\c" .` + "\n",
		},
		{
			name: "a line feed and a carriage return are escaped",
			src:  `<http://e/s> <http://e/p> "a\nb\rc" .`,
			want: `<http://e/s> <http://e/p> "a\nb\rc" .` + "\n",
		},
		{
			name: "a tab written as itself is escaped, unlike in RDF 1.1",
			src:  "<http://e/s> <http://e/p> \"a\tb\" .",
			want: `<http://e/s> <http://e/p> "a\tb" .` + "\n",
		},
		{
			name: "a backspace and a form feed are escaped too",
			src:  `<http://e/s> <http://e/p> "a\bb\fc" .`,
			want: `<http://e/s> <http://e/p> "a\bb\fc" .` + "\n",
		},
		{
			name: "an escaped apostrophe needs no escape at all",
			src:  `<http://e/s> <http://e/p> "it\'s" .`,
			want: "<http://e/s> <http://e/p> \"it's\" .\n",
		},
		{
			name: "a vertical tab has no ECHAR and takes a UCHAR",
			src:  `<http://e/s> <http://e/p> "a\u000Bb" .`,
			want: `<http://e/s> <http://e/p> "a\u000Bb" .` + "\n",
		},
		{
			name: "a null and the controls below the backspace take a UCHAR",
			src:  `<http://e/s> <http://e/p> "\u0000\u0007" .`,
			want: `<http://e/s> <http://e/p> "\u0000\u0007" .` + "\n",
		},
		{
			name: "the controls above the carriage return take a UCHAR",
			src:  `<http://e/s> <http://e/p> "\u000E\u001F" .`,
			want: `<http://e/s> <http://e/p> "\u000E\u001F" .` + "\n",
		},
		{
			name: "delete takes a UCHAR",
			src:  `<http://e/s> <http://e/p> "\u007F" .`,
			want: `<http://e/s> <http://e/p> "\u007F" .` + "\n",
		},
		{
			name: "hex digits are uppercase",
			src:  `<http://e/s> <http://e/p> "\u001a" .`,
			want: `<http://e/s> <http://e/p> "\u001A" .` + "\n",
		},
		{
			name: "the two code points XML excludes above the ascii range take a UCHAR",
			src:  `<http://e/s> <http://e/p> "\uFFFE\uFFFF" .`,
			want: `<http://e/s> <http://e/p> "\uFFFE\uFFFF" .` + "\n",
		},
		{
			name: "a UCHAR is written as the character it names",
			src:  "<http://e/s> <http://e/p> \"caf\\u00E9\" .",
			want: "<http://e/s> <http://e/p> \"café\" .\n",
		},
		{
			name: "non ascii text is not escaped",
			src:  `<http://e/s> <http://e/p> "héllo wörld" .`,
			want: "<http://e/s> <http://e/p> \"héllo wörld\" .\n",
		},
		{
			name: "a character above the basic multilingual plane is not escaped",
			src:  "<http://e/s> <http://e/p> \"\\U0001F600\" .",
			want: "<http://e/s> <http://e/p> \"\U0001F600\" .\n",
		},
		{
			name: "a non ascii iri is not escaped",
			src:  `<http://example.com/ünïcødé> <http://e/p> "v" .`,
			want: "<http://example.com/ünïcødé> <http://e/p> \"v\" .\n",
		},
		{
			name: "an iri written with a UCHAR prints as the character",
			src:  "<http://example.com/\\u0061> <http://e/p> \"v\" .",
			want: "<http://example.com/a> <http://e/p> \"v\" .\n",
		},
		{
			name: "an iri character the production excludes stays a UCHAR, in uppercase hex",
			src:  "<http://example.com/a\\u007Cb> <http://e/p> \"v\" .",
			want: `<http://example.com/a\u007Cb> <http://e/p> "v" .` + "\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := ntriples.Parse(strings.NewReader(tc.src))
			if err != nil {
				t.Fatalf("Parse() = %v, want nil", err)
			}

			if got := printToString(t, doc); got != tc.want {
				t.Errorf("Print() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestWritesInvalidUTF8Through pins what happens to a byte that is not UTF-8
// at all. No parser produces one — a tokenizer reads characters — so it can
// only arrive on a term built by hand, and there is nothing the grammar can
// spell it as. Substituting U+FFFD would change the literal silently, so the
// byte goes out as it stands.
func TestWritesInvalidUTF8Through(t *testing.T) {
	const value = "a\xffb"

	t.Run("Print", func(t *testing.T) {
		doc := &ntriples.Document{
			Statements: []ntriples.Statement{
				&ntriples.Triple{
					Subject:   &ntriples.IRIRef{Value: "http://e/s"},
					Predicate: &ntriples.IRIRef{Value: "http://e/p"},
					Object:    &ntriples.Literal{Value: value},
				},
			},
		}

		want := "<http://e/s> <http://e/p> \"" + value + "\" .\n"
		if got := printToString(t, doc); got != want {
			t.Errorf("Print() = %q, want %q", got, want)
		}
	})

	t.Run("Encode", func(t *testing.T) {
		triple := rdf.Triple{
			Subject:   rdf.IRI("http://e/s"),
			Predicate: "http://e/p",
			Object:    rdf.NewLiteral(value),
		}

		want := "<http://e/s> <http://e/p> \"" + value + "\" .\n"
		if got := encodeToString(t, []rdf.Triple{triple}); got != want {
			t.Errorf("Encode() = %q, want %q", got, want)
		}
	})
}

// TestPrintKeepsComments covers the criterion that keeps Print from being
// merely Encode with extra steps: a document's comments survive it.
func TestPrintKeepsComments(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "a comment on its own line",
			src:  "# a note\n<http://e/s> <http://e/p> _:o .",
			want: "# a note\n<http://e/s> <http://e/p> _:o .\n",
		},
		{
			name: "a comment trailing a statement stays on its line",
			src:  "<http://e/s> <http://e/p> _:o . # why",
			want: "<http://e/s> <http://e/p> _:o . # why\n",
		},
		{
			name: "a comment trailing a version directive stays on its line",
			src:  "VERSION \"1.2\" # why",
			want: "VERSION \"1.2\" # why\n",
		},
		{
			name: "a comment after the last statement",
			src:  "<http://e/s> <http://e/p> _:o .\n# afterwards",
			want: "<http://e/s> <http://e/p> _:o .\n# afterwards\n",
		},
		{
			name: "a document of nothing but comments",
			src:  "# one\n# two\n# three",
			want: "# one\n# two\n# three\n",
		},
		{
			name: "comments before, trailing, between and after",
			src: "# first\n" +
				"<http://e/a> <http://e/p> _:x . # trailing\n" +
				"# between\n" +
				"<http://e/b> <http://e/p> _:y .\n" +
				"# last",
			want: "# first\n" +
				"<http://e/a> <http://e/p> _:x . # trailing\n" +
				"# between\n" +
				"<http://e/b> <http://e/p> _:y .\n" +
				"# last\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := ntriples.Parse(strings.NewReader(tc.src))
			if err != nil {
				t.Fatalf("Parse() = %v, want nil", err)
			}

			if got := printToString(t, doc); got != tc.want {
				t.Errorf("Print() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestPrintKeepsVersionDirectives covers the other half of what separates
// Print from Encode: canonical form has no VERSION directive, and a document
// being rewritten keeps every one it had, in its place among the triples it
// announces the version of.
func TestPrintKeepsVersionDirectives(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "a directive alone",
			src:  `VERSION "1.2"`,
			want: "VERSION \"1.2\"\n",
		},
		{
			name: "a directive before a triple",
			src:  "VERSION \"1.2\"\n<http://e/s> <http://e/p> <http://e/o> .",
			want: "VERSION \"1.2\"\n<http://e/s> <http://e/p> <http://e/o> .\n",
		},
		{
			name: "a directive keeps its place between two triples",
			src: "<http://e/a> <http://e/p> <http://e/o> .\n" +
				"VERSION \"1.2\"\n" +
				"<http://e/b> <http://e/p> <http://e/o> .",
			want: "<http://e/a> <http://e/p> <http://e/o> .\n" +
				"VERSION \"1.2\"\n" +
				"<http://e/b> <http://e/p> <http://e/o> .\n",
		},
		{
			name: "several directives are all kept",
			src:  "VERSION \"1.1\"\nVERSION \"1.2\"",
			want: "VERSION \"1.1\"\nVERSION \"1.2\"\n",
		},
		{
			name: "a specifier is escaped like any other quoted string",
			src:  `VERSION "1.2\t\"x\""`,
			want: `VERSION "1.2\t\"x\""` + "\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := ntriples.Parse(strings.NewReader(tc.src))
			if err != nil {
				t.Fatalf("Parse() = %v, want nil", err)
			}

			if got := printToString(t, doc); got != tc.want {
				t.Errorf("Print() = %q, want %q", got, tc.want)
			}
		})
	}
}

// withoutPositions returns a copy of doc with every position cleared, so that
// two trees can be compared on what they say rather than on where it was
// written. Printing does not preserve columns, and is not meant to.
func withoutPositions(doc *ntriples.Document) *ntriples.Document {
	if doc == nil {
		return nil
	}

	out := &ntriples.Document{}
	for _, statement := range doc.Statements {
		switch s := statement.(type) {
		case *ntriples.Triple:
			out.Statements = append(out.Statements, &ntriples.Triple{
				Subject:   termWithoutPosition(s.Subject).(ntriples.SubjectTerm),
				Predicate: termWithoutPosition(s.Predicate).(*ntriples.IRIRef),
				Object:    termWithoutPosition(s.Object),
			})
		case *ntriples.VersionDirective:
			out.Statements = append(out.Statements, &ntriples.VersionDirective{Version: s.Version})
		default:
			panic("unknown statement node")
		}
	}
	for _, c := range doc.Comments {
		out.Comments = append(out.Comments, &ntriples.Comment{Text: c.Text})
	}
	return out
}

func termWithoutPosition(node ntriples.Term) ntriples.Term {
	switch n := node.(type) {
	case *ntriples.IRIRef:
		return &ntriples.IRIRef{Value: n.Value}
	case *ntriples.BlankNode:
		return &ntriples.BlankNode{Label: n.Label}
	case *ntriples.Literal:
		out := &ntriples.Literal{Value: n.Value, Language: n.Language, Direction: n.Direction}
		if n.Datatype != nil {
			out.Datatype = &ntriples.IRIRef{Value: n.Datatype.Value}
		}
		return out
	case *ntriples.TripleTerm:
		return &ntriples.TripleTerm{
			Subject:   termWithoutPosition(n.Subject).(ntriples.SubjectTerm),
			Predicate: termWithoutPosition(n.Predicate).(*ntriples.IRIRef),
			Object:    termWithoutPosition(n.Object),
		}
	default:
		panic("unknown term node")
	}
}

// TestPrintRoundTrip covers the criterion that says printing loses nothing:
// parsing what was printed gives back the tree that was printed, once the
// positions — which moved, and were meant to — are set aside.
func TestPrintRoundTrip(t *testing.T) {
	sources := []string{
		"",
		"<http://example.com/s> <http://example.com/p> <http://example.com/o> .",
		"_:a <http://example.com/p> _:b .\n_:b <http://example.com/p> _:a .",
		`<http://e/s> <http://e/p> "text"@en-GB .`,
		`<http://e/s> <http://e/p> "1"^^<http://www.w3.org/2001/XMLSchema#integer> .`,
		`<http://e/s> <http://e/p> "a\"b\\c\nd\re\tf\bg\fh" .`,
		`<http://e/s> <http://e/p> "\u0000\u000B\u001F\u007F\uFFFF" .`,
		`<http://e/s> <http://e/p> "héllo"@fr .`,
		`<http://e/s> <http://e/p> "text"@en--ltr .`,
		`<http://e/s> <http://e/p> "مرحبا"@ar--rtl .`,
		`<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> <http://e/c> )>> .`,
		`<http://e/s> <http://e/p> <<( _:x <http://e/b> "v"@en--rtl )>> .`,
		`<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> <<( <http://e/x> <http://e/y> "z" )>> )>> .`,
		"VERSION \"1.2\"\n<http://e/s> <http://e/p> <http://e/o> .",
		"<http://e/a> <http://e/p> _:x .\nVERSION \"1.2\"\n<http://e/b> <http://e/p> _:y .",
		"# a note\n<http://e/s> <http://e/p> _:o . # trailing\n# last",
		"# only comments\n# and more",
		"<http://e/a> <http://e/p> _:x .\n\n<http://e/b> <http://e/p> _:y .\n# end",
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			first, err := ntriples.Parse(strings.NewReader(src))
			if err != nil {
				t.Fatalf("Parse() = %v, want nil", err)
			}

			printed := printToString(t, first)

			second, err := ntriples.Parse(strings.NewReader(printed))
			if err != nil {
				t.Fatalf("Parse(printed) = %v, want nil; printed = %q", err, printed)
			}

			if !reflect.DeepEqual(withoutPositions(first), withoutPositions(second)) {
				t.Errorf("round trip changed the tree; printed = %q", printed)
			}
		})
	}
}

// TestPrintIsStable checks that printing what was printed changes nothing, so
// that running a document through twice is the same as running it through
// once.
func TestPrintIsStable(t *testing.T) {
	src := "# first\n" +
		"VERSION \"1.2\"\n" +
		"<http://e/a> <http://e/p> \"a\tb\" . # trailing\n" +
		"# between\n" +
		"_:x <http://e/p> <<( _:y <http://e/q> \"v\"@en--ltr )>> .\n" +
		"# last"

	doc, err := ntriples.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}

	once := printToString(t, doc)

	reparsed, err := ntriples.Parse(strings.NewReader(once))
	if err != nil {
		t.Fatalf("Parse(printed) = %v, want nil", err)
	}

	if twice := printToString(t, reparsed); twice != once {
		t.Errorf("printing twice gave %q, want %q", twice, once)
	}
}

func TestEncode(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "no triples produce no output",
			src:  "",
			want: "",
		},
		{
			name: "a statement ends with a line feed",
			src:  `<http://example.com/s> <http://example.com/p> <http://example.com/o> .`,
			want: "<http://example.com/s> <http://example.com/p> <http://example.com/o> .\n",
		},
		{
			name: "comments are dropped, canonical form having none",
			src:  "# a note\n<http://e/s> <http://e/p> <http://e/o> . # trailing",
			want: "<http://e/s> <http://e/p> <http://e/o> .\n",
		},
		{
			name: "a plain literal is written without its datatype",
			src:  `<http://e/s> <http://e/p> "text" .`,
			want: "<http://e/s> <http://e/p> \"text\" .\n",
		},
		{
			name: "an explicitly written xsd:string is written without it too",
			src:  `<http://e/s> <http://e/p> "text"^^<http://www.w3.org/2001/XMLSchema#string> .`,
			want: "<http://e/s> <http://e/p> \"text\" .\n",
		},
		{
			name: "a language tag is kept",
			src:  `<http://e/s> <http://e/p> "text"@en .`,
			want: "<http://e/s> <http://e/p> \"text\"@en .\n",
		},
		{
			name: "the seven characters with an ECHAR are escaped",
			src:  `<http://e/s> <http://e/p> "q\" b\\ n\n r\r t\t f\f s\b" .`,
			want: `<http://e/s> <http://e/p> "q\" b\\ n\n r\r t\t f\f s\b" .` + "\n",
		},
		{
			name: "a character with no printable form takes a UCHAR",
			src:  `<http://e/s> <http://e/p> "\u000B\u007F" .`,
			want: `<http://e/s> <http://e/p> "\u000B\u007F" .` + "\n",
		},
		{
			name: "a directional literal is written with its base direction",
			src:  `<http://e/s> <http://e/p> "text"@en--ltr .`,
			want: "<http://e/s> <http://e/p> \"text\"@en--ltr .\n",
		},
		{
			name: "the datatype a base direction implies is not written",
			src:  `<http://e/s> <http://e/p> "text"@ar--rtl .`,
			want: "<http://e/s> <http://e/p> \"text\"@ar--rtl .\n",
		},
		{
			name: "a triple term",
			src:  `<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> <http://e/c> )>> .`,
			want: "<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> <http://e/c> )>> .\n",
		},
		{
			name: "a nested triple term",
			src:  `<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> <<( <http://e/x> <http://e/y> "v"@en--rtl )>> )>> .`,
			want: "<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> <<( <http://e/x> <http://e/y> \"v\"@en--rtl )>> )>> .\n",
		},
		{
			name: "a version directive is dropped, canonical form forbidding one",
			src:  "VERSION \"1.2\"\n<http://e/s> <http://e/p> <http://e/o> .",
			want: "<http://e/s> <http://e/p> <http://e/o> .\n",
		},
		{
			name: "a document of nothing but a version directive encodes to nothing",
			src:  `VERSION "1.2"`,
			want: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			triples, err := collect2(ntriples.Decode(strings.NewReader(tc.src)))
			if err != nil {
				t.Fatalf("Decode() = %v, want nil", err)
			}

			if got := encodeToString(t, triples); got != tc.want {
				t.Errorf("Encode() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEncodeLowercasesLanguageTags covers §3's rule that the alphabetic
// characters of a LANG_DIR are lowercase. RDF 1.2 has a parser keep the case a
// document wrote — so [ntriples.Parse] and [ntriples.Decode] both do — which
// leaves the mapping to canonical form, and to nothing else.
func TestEncodeLowercasesLanguageTags(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "a tag written in uppercase",
			src:  `<http://e/s> <http://e/p> "text"@EN .`,
			want: "<http://e/s> <http://e/p> \"text\"@en .\n",
		},
		{
			name: "a subtag written in uppercase",
			src:  `<http://e/s> <http://e/p> "text"@en-GB .`,
			want: "<http://e/s> <http://e/p> \"text\"@en-gb .\n",
		},
		{
			name: "the tag of a directional literal",
			src:  `<http://e/s> <http://e/p> "text"@AR--rtl .`,
			want: "<http://e/s> <http://e/p> \"text\"@ar--rtl .\n",
		},
		{
			name: "inside a triple term",
			src:  `<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> "v"@En-Gb )>> .`,
			want: "<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> \"v\"@en-gb )>> .\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			triples, err := collect2(ntriples.Decode(strings.NewReader(tc.src)))
			if err != nil {
				t.Fatalf("Decode() = %v, want nil", err)
			}

			if got := encodeToString(t, triples); got != tc.want {
				t.Errorf("Encode() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEncodeRoundTrip covers the criterion that says encoding loses no
// meaning: decoding what was encoded gives back the same graph. The blank node
// labels differ, each read having minted its own, so the graphs are isomorphic
// rather than equal — which is exactly the comparison the evaluation tests use.
//
// A blank node nested inside a triple term is not among the sources, and the
// encoder is not the reason: [rdf.Isomorphic] does not map one, comparing it
// by its label instead, and the label a document is read with is minted afresh
// each time. Such a document round-trips all the same, which
// TestPrintRoundTrip shows on the tree, where labels are the document's own.
func TestEncodeRoundTrip(t *testing.T) {
	sources := []string{
		"",
		"<http://example.com/s> <http://example.com/p> <http://example.com/o> .",
		"_:a <http://example.com/p> _:b .\n_:b <http://example.com/p> _:a .",
		"_:a <http://e/p> _:b .\n_:b <http://e/p> _:c .\n_:c <http://e/p> _:a .",
		`<http://e/s> <http://e/p> "text"@en-gb .`,
		`<http://e/s> <http://e/p> "1"^^<http://www.w3.org/2001/XMLSchema#integer> .`,
		`<http://e/s> <http://e/p> "a\"b\\c\nd\re\tf\bg\fh" .`,
		`<http://e/s> <http://e/p> "\u0000\u000B\u001F\u007F\uFFFF" .`,
		`<http://e/s> <http://e/p> "héllo wörld" .`,
		`<http://e/s> <http://e/p> "text"@en--ltr .`,
		`<http://e/s> <http://e/p> "مرحبا"@ar--rtl .`,
		`<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> <http://e/c> )>> .`,
		`_:s <http://e/p> <<( <http://e/a> <http://e/b> "v"@he--rtl )>> .`,
		`<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> <<( <http://e/x> <http://e/y> "z" )>> )>> .`,
		"VERSION \"1.2\"\n<http://e/s> <http://e/p> <http://e/o> .",
		"# a comment, which encoding drops\n<http://e/s> <http://e/p> _:o .",
		"<http://e/a> <http://e/p> \"1\" .\n<http://e/b> <http://e/p> \"2\" .\n<http://e/c> <http://e/p> \"3\" .",
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			first, err := collect2(ntriples.Decode(strings.NewReader(src)))
			if err != nil {
				t.Fatalf("Decode() = %v, want nil", err)
			}

			encoded := encodeToString(t, first)

			second, err := collect2(ntriples.Decode(strings.NewReader(encoded)))
			if err != nil {
				t.Fatalf("Decode(encoded) = %v, want nil; encoded = %q", err, encoded)
			}

			if !rdf.Isomorphic(graphOfTriples(t, first), graphOfTriples(t, second)) {
				t.Errorf("round trip changed the graph; encoded = %q", encoded)
			}
		})
	}
}

// TestEncodeIsStable checks that canonical form is a fixed point: encoding
// what was encoded produces the same bytes, which is what makes the output
// comparable across runs. The uppercase tag is here because it is the one
// thing encoding rewrites, so it is the one thing that could fail to settle
// after a single pass.
func TestEncodeIsStable(t *testing.T) {
	src := "<http://e/a> <http://e/p> \"a\tb\"@EN-gb .\n" +
		"<http://e/b> <http://e/p> \"1\"^^<http://e/dt> .\n" +
		"<http://e/c> <http://e/p> <<( <http://e/x> <http://e/y> \"v\"@AR--rtl )>> .\n"

	first, err := collect2(ntriples.Decode(strings.NewReader(src)))
	if err != nil {
		t.Fatalf("Decode() = %v, want nil", err)
	}
	once := encodeToString(t, first)

	second, err := collect2(ntriples.Decode(strings.NewReader(once)))
	if err != nil {
		t.Fatalf("Decode(encoded) = %v, want nil", err)
	}
	if twice := encodeToString(t, second); twice != once {
		t.Errorf("encoding twice gave %q, want %q", twice, once)
	}
}

// TestEncodeIsByteStableOnlyForGroundGraphs pins the limit of what canonical
// form settles, so that the byte-stability claim is not read wider than it
// holds.
//
// §3 fixes the writing — spacing, escaping, tag case, no comments and no
// version directive — so a ground graph encoded twice is the same bytes
// whoever encodes it. It says nothing about which labels blank nodes carry,
// and a label minted by Decode is unique rather than reproducible. Two reads
// of one document therefore agree on the graph and need not agree on the
// bytes.
func TestEncodeIsByteStableOnlyForGroundGraphs(t *testing.T) {
	t.Run("a ground graph encodes to the same bytes every time", func(t *testing.T) {
		src := "<http://e/a> <http://e/p> \"1\" .\n" +
			"<http://e/b> <http://e/q> <<( <http://e/c> <http://e/d> \"v\"@en--ltr )>> .\n"

		first, err := collect2(ntriples.Decode(strings.NewReader(src)))
		if err != nil {
			t.Fatalf("Decode() = %v, want nil", err)
		}
		second, err := collect2(ntriples.Decode(strings.NewReader(src)))
		if err != nil {
			t.Fatalf("Decode() = %v, want nil", err)
		}

		if a, b := encodeToString(t, first), encodeToString(t, second); a != b {
			t.Errorf("two reads encoded to %q and %q, want the same bytes", a, b)
		}
	})

	t.Run("a graph with blank nodes need not, but is the same graph", func(t *testing.T) {
		src := "_:a <http://e/p> _:b .\n"

		first, err := collect2(ntriples.Decode(strings.NewReader(src)))
		if err != nil {
			t.Fatalf("Decode() = %v, want nil", err)
		}
		second, err := collect2(ntriples.Decode(strings.NewReader(src)))
		if err != nil {
			t.Fatalf("Decode() = %v, want nil", err)
		}

		// The bytes differ, each read having minted its own labels...
		if a, b := encodeToString(t, first), encodeToString(t, second); a == b {
			t.Errorf("two reads both encoded to %q; the labels were expected to differ", a)
		}
		// ...and the graph does not.
		if !rdf.Isomorphic(graphOfTriples(t, first), graphOfTriples(t, second)) {
			t.Error("two reads of one document gave graphs that are not isomorphic")
		}
	})
}

// TestPrintMatchesEncodeWithoutComments checks the claim the doc makes: a
// document that carries no comments, no version directive and no uppercase
// language tag prints in canonical form.
func TestPrintMatchesEncodeWithoutComments(t *testing.T) {
	sources := []string{
		"<http://example.com/s> <http://example.com/p> <http://example.com/o> .",
		`<http://e/s> <http://e/p> "text"@en-gb .`,
		`<http://e/s> <http://e/p> "text"@ar--rtl .`,
		`<http://e/s> <http://e/p> "a\"b\\c\nd\re\tf\bg\fh" .`,
		`<http://e/s> <http://e/p> "\u000B\u007F\uFFFE" .`,
		`<http://e/s> <http://e/p> "1"^^<http://e/dt> .`,
		`<http://e/s> <http://e/p> <<( <http://e/a> <http://e/b> <<( <http://e/x> <http://e/y> "z"@en--ltr )>> )>> .`,
		"<http://e/a> <http://e/p> \"1\" .\n<http://e/b> <http://e/p> \"2\" .",
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			doc, err := ntriples.Parse(strings.NewReader(src))
			if err != nil {
				t.Fatalf("Parse() = %v, want nil", err)
			}
			triples, err := ntriples.Triples(doc)
			if err != nil {
				t.Fatalf("Triples() = %v, want nil", err)
			}

			printed := printToString(t, doc)
			if encoded := encodeToString(t, triples); printed != encoded {
				t.Errorf("Print() = %q, Encode() = %q, want them equal", printed, encoded)
			}
		})
	}
}

// failingWriter fails after letting a fixed number of bytes through, standing
// in for a full disk or a closed connection.
type failingWriter struct {
	allow int
	err   error
}

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.allow <= 0 {
		return 0, w.err
	}
	if len(p) > w.allow {
		n := w.allow
		w.allow = 0
		return n, w.err
	}
	w.allow -= len(p)
	return len(p), nil
}

func TestPrintErrors(t *testing.T) {
	t.Run("a nil document", func(t *testing.T) {
		var b strings.Builder
		if err := ntriples.Print(&b, nil); !errors.Is(err, ntriples.ErrNilDocument) {
			t.Errorf("Print() = %v, want %v", err, ntriples.ErrNilDocument)
		}
	})

	t.Run("a writer that fails", func(t *testing.T) {
		errBoom := errors.New("boom")

		doc, err := ntriples.Parse(strings.NewReader("# note\nVERSION \"1.2\"\n<http://e/s> <http://e/p> _:o .\n"))
		if err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}

		if err := ntriples.Print(&failingWriter{err: errBoom}, doc); !errors.Is(err, errBoom) {
			t.Errorf("Print() = %v, want %v", err, errBoom)
		}
	})
}

// manyStatements builds a document long enough that writing it fills the
// output buffer several times over, so that a failing writer fails part way
// through rather than only at the end.
func manyStatements(count int) string {
	var b strings.Builder
	for i := range count {
		fmt.Fprintf(&b, "<http://example.com/subject%d> <http://example.com/predicate> \"value %d\" .\n", i, i)
	}
	return b.String()
}

// TestWriteFailsPartWayThrough covers the failure a small document cannot
// reach. Output is buffered, so a writer that refuses everything is not
// noticed until the flush at the end; only a document that overruns the buffer
// makes a write fail while there is still more to write.
func TestWriteFailsPartWayThrough(t *testing.T) {
	const (
		statements = 500
		// Enough to fill the buffer at least once and fail on a later flush.
		allow = 5000
	)
	errBoom := errors.New("boom")

	src := manyStatements(statements)

	t.Run("Print", func(t *testing.T) {
		doc, err := ntriples.Parse(strings.NewReader(src))
		if err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}

		w := &failingWriter{allow: allow, err: errBoom}
		if err := ntriples.Print(w, doc); !errors.Is(err, errBoom) {
			t.Errorf("Print() = %v, want %v", err, errBoom)
		}
	})

	t.Run("Encode", func(t *testing.T) {
		triples, err := collect2(ntriples.Decode(strings.NewReader(src)))
		if err != nil {
			t.Fatalf("Decode() = %v, want nil", err)
		}

		w := &failingWriter{allow: allow, err: errBoom}
		if err := ntriples.Encode(w, slices.Values(triples)); !errors.Is(err, errBoom) {
			t.Errorf("Encode() = %v, want %v", err, errBoom)
		}
	})
}

func TestEncodeErrors(t *testing.T) {
	t.Run("a writer that fails", func(t *testing.T) {
		errBoom := errors.New("boom")

		triples, err := collect2(ntriples.Decode(strings.NewReader("<http://e/s> <http://e/p> _:o .")))
		if err != nil {
			t.Fatalf("Decode() = %v, want nil", err)
		}

		err = ntriples.Encode(&failingWriter{err: errBoom}, slices.Values(triples))
		if !errors.Is(err, errBoom) {
			t.Errorf("Encode() = %v, want %v", err, errBoom)
		}
	})

	t.Run("a triple that cannot be written as N-Triples", func(t *testing.T) {
		// A literal subject is expressible in the data model's zero value but
		// has no N-Triples form, so writing it would produce a document this
		// package could not read back.
		invalid := rdf.Triple{
			Subject:   rdf.NewLiteral("not a subject"),
			Predicate: "http://example.com/p",
			Object:    rdf.IRI("http://example.com/o"),
		}

		var b strings.Builder
		err := ntriples.Encode(&b, slices.Values([]rdf.Triple{invalid}))
		if !errors.Is(err, rdf.ErrInvalidSubject) {
			t.Errorf("Encode() = %v, want %v", err, rdf.ErrInvalidSubject)
		}
		if b.String() != "" {
			t.Errorf("Encode() wrote %q, want nothing", b.String())
		}
	})
}

// TestEncodeRefusesTripleTermSubjects covers the criterion that says a triple
// term in subject position is an error rather than silent corruption. RDF 1.2
// admits one in the object position alone, and the grammar has no spelling for
// a subject that is a triple term — so writing the brackets anyway would
// produce a document no parser reads back.
func TestEncodeRefusesTripleTermSubjects(t *testing.T) {
	term := rdf.TripleTerm{
		Subject:   rdf.IRI("http://example.com/a"),
		Predicate: "http://example.com/b",
		Object:    rdf.IRI("http://example.com/c"),
	}

	testCases := []struct {
		name   string
		triple rdf.Triple
	}{
		{
			name: "as the subject of the statement",
			triple: rdf.Triple{
				Subject:   term,
				Predicate: "http://example.com/p",
				Object:    rdf.IRI("http://example.com/o"),
			},
		},
		{
			name: "as the subject of a triple term",
			triple: rdf.Triple{
				Subject:   rdf.IRI("http://example.com/s"),
				Predicate: "http://example.com/p",
				Object: rdf.TripleTerm{
					Subject:   term,
					Predicate: "http://example.com/q",
					Object:    rdf.IRI("http://example.com/o"),
				},
			},
		},
		{
			name: "as the subject of a triple term nested two deep",
			triple: rdf.Triple{
				Subject:   rdf.IRI("http://example.com/s"),
				Predicate: "http://example.com/p",
				Object: rdf.TripleTerm{
					Subject:   rdf.IRI("http://example.com/s2"),
					Predicate: "http://example.com/q",
					Object: rdf.TripleTerm{
						Subject:   term,
						Predicate: "http://example.com/r",
						Object:    rdf.IRI("http://example.com/o"),
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var b strings.Builder
			err := ntriples.Encode(&b, slices.Values([]rdf.Triple{tc.triple}))
			if !errors.Is(err, rdf.ErrInvalidSubject) {
				t.Errorf("Encode() = %v, want %v", err, rdf.ErrInvalidSubject)
			}
			if b.String() != "" {
				t.Errorf("Encode() wrote %q, want nothing", b.String())
			}
		})
	}
}

// TestEncodeWritesWhatCameBeforeAnInvalidStatement checks that stopping early
// does not discard what was already written.
//
// Output is buffered, so what a caller receives when encoding stops would
// otherwise depend on where the buffer happened to fill: everything up to the
// last flush for a long document, nothing at all for a short one. Both lengths
// are checked here for that reason.
func TestEncodeWritesWhatCameBeforeAnInvalidStatement(t *testing.T) {
	invalid := rdf.Triple{
		Subject:   rdf.IRI("http://example.com/s"),
		Predicate: "",
		Object:    rdf.IRI("http://example.com/o"),
	}

	valid := func(i int) rdf.Triple {
		return rdf.Triple{
			Subject:   rdf.IRI(fmt.Sprintf("http://example.com/s%d", i)),
			Predicate: "http://example.com/p",
			Object:    rdf.IRI("http://example.com/o"),
		}
	}

	testCases := []struct {
		name   string
		before int
	}{
		{name: "too few to fill the buffer", before: 1},
		{name: "more than fills the buffer", before: 500},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			triples := make([]rdf.Triple, 0, tc.before+1)
			var want strings.Builder
			for i := range tc.before {
				triples = append(triples, valid(i))
				want.WriteString(valid(i).String())
				want.WriteByte('\n')
			}
			triples = append(triples, invalid)

			var got strings.Builder
			err := ntriples.Encode(&got, slices.Values(triples))
			if !errors.Is(err, rdf.ErrInvalidPredicate) {
				t.Errorf("Encode() = %v, want %v", err, rdf.ErrInvalidPredicate)
			}
			if got.String() != want.String() {
				t.Errorf("Encode() wrote %d bytes, want the %d written before the invalid statement",
					got.Len(), want.Len())
			}
		})
	}
}

// TestEncodeAgreesWithRDF11 checks that the two encoders write one graph the
// same way wherever the two specifications agree on what canonical form is.
// Handing a graph read by one package to the printer of the other is the point
// of the shared term types, and this is the half of it the printers answer
// for.
//
// The sources avoid what §3 changed, since agreeing there would be the bug:
// none of them carries a character whose escaping moved between the versions,
// and none carries an uppercase language tag.
func TestEncodeAgreesWithRDF11(t *testing.T) {
	sources := []string{
		"<http://example.com/s> <http://example.com/p> <http://example.com/o> .",
		"_:a <http://e/p> _:b .",
		`<http://e/s> <http://e/p> "text" .`,
		`<http://e/s> <http://e/p> "text"@en-gb .`,
		`<http://e/s> <http://e/p> "1"^^<http://e/dt> .`,
		`<http://e/s> <http://e/p> "a\"b\\c\nd\re" .`,
		`<http://e/s> <http://e/p> "héllo wörld" .`,
		`<http://example.com/ünïcødé> <http://e/p> "v" .`,
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			// One read, two writes: the graph is identical by construction, so
			// any difference in the bytes is the printers'.
			triples, err := collect2(ntriples.Decode(strings.NewReader(src)))
			if err != nil {
				t.Fatalf("Decode() = %v, want nil", err)
			}

			var want strings.Builder
			if err := ntriples11.Encode(&want, slices.Values(triples)); err != nil {
				t.Fatalf("rdf11 Encode() = %v, want nil", err)
			}

			if got := encodeToString(t, triples); got != want.String() {
				t.Errorf("Encode() = %q, want %q", got, want.String())
			}
		})
	}
}

// TestEncodeDepartsFromRDF11 pins the places the two encoders must not agree,
// so that a change bringing them back together is caught rather than welcomed.
// Both are canonical form; the specifications simply changed what that is.
func TestEncodeDepartsFromRDF11(t *testing.T) {
	testCases := []struct {
		name   string
		src    string
		want12 string
		want11 string
	}{
		{
			name:   "a tab is escaped where RDF 1.1 wrote it as itself",
			src:    "<http://e/s> <http://e/p> \"a\tb\" .",
			want12: `<http://e/s> <http://e/p> "a\tb" .` + "\n",
			want11: "<http://e/s> <http://e/p> \"a\tb\" .\n",
		},
		{
			name:   "a language tag is lowercased where RDF 1.1 kept it",
			src:    `<http://e/s> <http://e/p> "text"@en-GB .`,
			want12: "<http://e/s> <http://e/p> \"text\"@en-gb .\n",
			want11: "<http://e/s> <http://e/p> \"text\"@en-GB .\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			triples, err := collect2(ntriples.Decode(strings.NewReader(tc.src)))
			if err != nil {
				t.Fatalf("Decode() = %v, want nil", err)
			}

			if got := encodeToString(t, triples); got != tc.want12 {
				t.Errorf("Encode() = %q, want %q", got, tc.want12)
			}

			var b strings.Builder
			if err := ntriples11.Encode(&b, slices.Values(triples)); err != nil {
				t.Fatalf("rdf11 Encode() = %v, want nil", err)
			}
			if b.String() != tc.want11 {
				t.Errorf("rdf11 Encode() = %q, want %q", b.String(), tc.want11)
			}
		})
	}
}
