package ntriples_test

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/rdf11/ntriples"
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

// TestPrint pins the bytes a document prints as. Every case but the commented
// ones is canonical form, which is the point of pinning it exactly.
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
			name: "a language tagged literal",
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

// TestPrintEscapesCanonically covers §4's escaping rule, which is as much
// about what must not be escaped as about what must. A tab written as \t
// parses to the same literal but is not canonical.
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
			name: "a tab is written as itself, not as an escape",
			src:  `<http://e/s> <http://e/p> "a\tb" .`,
			want: "<http://e/s> <http://e/p> \"a\tb\" .\n",
		},
		{
			name: "a backspace and a form feed are written as themselves",
			src:  `<http://e/s> <http://e/p> "a\bb\fc" .`,
			want: "<http://e/s> <http://e/p> \"a\bb\fc\" .\n",
		},
		{
			name: "an escaped apostrophe needs no escape at all",
			src:  `<http://e/s> <http://e/p> "it\'s" .`,
			want: "<http://e/s> <http://e/p> \"it's\" .\n",
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
			name: "a non ascii iri is not escaped",
			src:  `<http://example.com/ünïcødé> <http://e/p> "v" .`,
			want: "<http://example.com/ünïcødé> <http://e/p> \"v\" .\n",
		},
		{
			name: "an iri written with a UCHAR prints as the character",
			src:  "<http://example.com/\\u0061> <http://e/p> \"v\" .",
			want: "<http://example.com/a> <http://e/p> \"v\" .\n",
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

// withoutPositions returns a copy of doc with every position cleared, so that
// two trees can be compared on what they say rather than on where it was
// written. Printing does not preserve columns, and is not meant to.
func withoutPositions(doc *ntriples.Document) *ntriples.Document {
	if doc == nil {
		return nil
	}

	out := &ntriples.Document{}
	for _, t := range doc.Triples {
		out.Triples = append(out.Triples, &ntriples.Triple{
			Subject:   termWithoutPosition(t.Subject).(ntriples.SubjectTerm),
			Predicate: termWithoutPosition(t.Predicate).(*ntriples.IRIRef),
			Object:    termWithoutPosition(t.Object),
		})
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
		out := &ntriples.Literal{Value: n.Value, Language: n.Language}
		if n.Datatype != nil {
			out.Datatype = &ntriples.IRIRef{Value: n.Datatype.Value}
		}
		return out
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
		`<http://e/s> <http://e/p> "a\"b\\c\nd\re\tf" .`,
		`<http://e/s> <http://e/p> "héllo"@fr .`,
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
	src := "# first\n<http://e/a> <http://e/p> \"a\tb\" . # trailing\n# between\n_:x <http://e/p> _:y .\n# last"

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
			src:  `<http://e/s> <http://e/p> "text"@en-GB .`,
			want: "<http://e/s> <http://e/p> \"text\"@en-GB .\n",
		},
		{
			name: "only the four characters that must be escaped are",
			src:  `<http://e/s> <http://e/p> "q\" b\\ n\n r\r t\t" .`,
			want: "<http://e/s> <http://e/p> \"q\\\" b\\\\ n\\n r\\r t\t\" .\n",
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
func TestEncodeRoundTrip(t *testing.T) {
	sources := []string{
		"",
		"<http://example.com/s> <http://example.com/p> <http://example.com/o> .",
		"_:a <http://example.com/p> _:b .\n_:b <http://example.com/p> _:a .",
		"_:a <http://e/p> _:b .\n_:b <http://e/p> _:c .\n_:c <http://e/p> _:a .",
		`<http://e/s> <http://e/p> "text"@en-GB .`,
		`<http://e/s> <http://e/p> "1"^^<http://www.w3.org/2001/XMLSchema#integer> .`,
		`<http://e/s> <http://e/p> "a\"b\\c\nd\re\tf" .`,
		`<http://e/s> <http://e/p> "héllo wörld" .`,
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
// comparable across runs.
func TestEncodeIsStable(t *testing.T) {
	src := "<http://e/a> <http://e/p> \"a\tb\"@en .\n<http://e/b> <http://e/p> \"1\"^^<http://e/dt> .\n"

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
// §4 fixes the writing — spacing, escaping, no comments — so a ground graph
// encoded twice is the same bytes whoever encodes it. It says nothing about
// which labels blank nodes carry, and a label minted by Decode is unique
// rather than reproducible. Two reads of one document therefore agree on the
// graph and need not agree on the bytes.
func TestEncodeIsByteStableOnlyForGroundGraphs(t *testing.T) {
	t.Run("a ground graph encodes to the same bytes every time", func(t *testing.T) {
		src := "<http://e/a> <http://e/p> \"1\" .\n<http://e/b> <http://e/q> <http://e/c> .\n"

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
// document carrying no comments prints in canonical form.
func TestPrintMatchesEncodeWithoutComments(t *testing.T) {
	sources := []string{
		"<http://example.com/s> <http://example.com/p> <http://example.com/o> .",
		`<http://e/s> <http://e/p> "text"@en-GB .`,
		`<http://e/s> <http://e/p> "a\"b\\c\nd\re\tf" .`,
		`<http://e/s> <http://e/p> "1"^^<http://e/dt> .`,
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

		doc, err := ntriples.Parse(strings.NewReader("# note\n<http://e/s> <http://e/p> _:o .\n"))
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

	t.Run("the statements before an invalid one are still written", func(t *testing.T) {
		valid := rdf.Triple{
			Subject:   rdf.IRI("http://example.com/s"),
			Predicate: "http://example.com/p",
			Object:    rdf.IRI("http://example.com/o"),
		}
		invalid := rdf.Triple{
			Subject:   rdf.IRI("http://example.com/s"),
			Predicate: "",
			Object:    rdf.IRI("http://example.com/o"),
		}

		var b strings.Builder
		err := ntriples.Encode(&b, slices.Values([]rdf.Triple{valid, invalid}))
		if !errors.Is(err, rdf.ErrInvalidPredicate) {
			t.Errorf("Encode() = %v, want %v", err, rdf.ErrInvalidPredicate)
		}
	})
}
