package trig_test

import (
	"errors"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/rdf11/trig"
	"github.com/z5labs/rdf-go/vocab"
)

// The rules covered here came from the Turtle tokenizer, parser and printer
// along with the rest of the files they sit in. They are exercised again
// because the copies are still separate code — internal/lex holds the
// characters they read, not the rules that read them — and a fix to one that
// missed the other would show up nowhere else.

func TestTokenizeReaderError(t *testing.T) {
	errBoom := errors.New("boom")

	sources := []string{
		"   ", "<http://e/a", `"hello`, `"""hello`, "# a comment",
		"_:abc", `"o"@en`, "ex:name", "42", "1e", "[", `"o"^`, "{",
		"GRAPH", "graph:name",
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			_, err := collect(trig.Tokenize(&failingReader{data: []byte(src), err: errBoom}))
			if !errors.Is(err, errBoom) {
				t.Errorf("Tokenize() error = %v, want %v", err, errBoom)
			}
		})
	}
}

func TestParseReaderError(t *testing.T) {
	errBoom := errors.New("boom")

	sources := []string{
		"<http://e/g> {", "<http://e/g> { <http://e/s", "GRAPH", "@prefix ex:",
		"@base", "<http://e/s> <http://e/p> (", "<http://e/s> <http://e/p> [",
		`<http://e/s> <http://e/p> "v"`,
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			_, err := trig.Parse(&failingReader{data: []byte(src), err: errBoom})
			if !errors.Is(err, errBoom) {
				t.Errorf("Parse() error = %v, want %v", err, errBoom)
			}
		})
	}
}

// TestParseTurtleErrors covers the parse errors TriG takes from Turtle, which
// a graph block does not change.
func TestParseTurtleErrors(t *testing.T) {
	sources := []string{
		"@prefix ex <http://e/> .",
		"@prefix ex: <http://e/>",
		"@base .",
		"<http://e/s> .",
		"<http://e/s> <http://e/p> .",
		"<http://e/s> <http://e/p> <http://e/o>",
		"<http://e/g> { <http://e/s> <http://e/p> ( <http://e/o> }",
		"<http://e/g> { <http://e/s> <http://e/p> [ <http://e/q> <http://e/r> . }",
		`<http://e/s> <http://e/p> "v"^^ .`,
		"<http://e/g> { <http://e/s> <http://e/p> (",
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			if _, err := trig.Parse(strings.NewReader(src)); err == nil {
				t.Error("Parse() = nil, want an error")
			}
		})
	}
}

func TestLoweringErrorMessages(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "invalid term",
			err:  trig.InvalidTermError{Pos: pos(3, 12), Err: trig.ErrNoBase},
			want: "invalid term at line 3, column 12: trig: relative IRI with no base in scope",
		},
		{
			name: "undefined prefix",
			err:  trig.UndefinedPrefixError{Pos: pos(3, 12), Prefix: "ex"},
			want: `undefined prefix "ex" at line 3, column 12`,
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

func TestInvalidTermErrorUnwraps(t *testing.T) {
	err := trig.InvalidTermError{Pos: pos(1, 1), Err: trig.ErrNoBase}
	if !errors.Is(err, trig.ErrNoBase) {
		t.Errorf("errors.Is(%v, ErrNoBase) = false, want true", err)
	}
}

func TestLiteralKindString(t *testing.T) {
	kinds := map[trig.LiteralKind]string{
		trig.LiteralString:  "String",
		trig.LiteralInteger: "Integer",
		trig.LiteralDecimal: "Decimal",
		trig.LiteralDouble:  "Double",
		trig.LiteralBoolean: "Boolean",
		trig.LiteralKind(9): "LiteralKind(9)",
	}
	for kind, want := range kinds {
		if got := kind.String(); got != want {
			t.Errorf("LiteralKind.String() = %q, want %q", got, want)
		}
	}
}

// TestDecodeTurtleTerms covers the terms TriG takes from Turtle, lowered
// inside a graph block so that the graph label rides along with each of them.
func TestDecodeTurtleTerms(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "a datatype written as a prefixed name",
			src:  "@prefix ex: <http://e/> .\nex:g { ex:s ex:p \"v\"^^ex:dt . }",
			want: `<http://e/s> <http://e/p> "v"^^<http://e/dt> <http://e/g> .`,
		},
		{
			name: "the empty prefix",
			src:  "@prefix : <http://e/> .\n:g { :s :p :o . }",
			want: "<http://e/s> <http://e/p> <http://e/o> <http://e/g> .",
		},
		{
			name: "a namespace named on its own",
			src:  "@prefix ex: <http://e/> .\nex:g { ex: ex:p ex: . }",
			want: "<http://e/> <http://e/p> <http://e/> <http://e/g> .",
		},
		{
			name: "a base resolved against the base it replaces",
			src:  "@base <http://e/> .\n@base <sub/> .\n<g> { <s> <p> <o> . }",
			want: "<http://e/sub/s> <http://e/sub/p> <http://e/sub/o> <http://e/sub/g> .",
		},
		{
			name: "the long string forms",
			src:  "<http://e/g> { <http://e/s> <http://e/p> \"\"\"a\nb\"\"\", '''c''' . }",
			want: "<http://e/s> <http://e/p> \"a\\nb\" <http://e/g> .\n" +
				`<http://e/s> <http://e/p> "c" <http://e/g> .`,
		},
		{
			name: "an empty collection is rdf:nil",
			src:  "<http://e/g> { <http://e/s> <http://e/p> () . }",
			want: "<http://e/s> <http://e/p> <" + string(vocab.RDFNil) + "> <http://e/g> .",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := datasetOf(t, decodeTrig(t, tc.src))
			want := datasetOf(t, decodeNQuads(t, tc.want))

			if !isomorphicDatasets(t, got, want) {
				t.Errorf("Decode() =\n%swant\n%s", formatDataset(got), formatDataset(want))
			}
		})
	}
}

func TestDecodeUndefinedPrefixesEverywhere(t *testing.T) {
	sources := []string{
		"<http://e/g> { <http://e/s> ex:p <http://e/o> . }",
		"<http://e/g> { <http://e/s> <http://e/p> ex:o . }",
		`<http://e/g> { <http://e/s> <http://e/p> "v"^^ex:dt . }`,
		"<http://e/g> { <http://e/s> <http://e/p> ( ex:a ) . }",
		"<http://e/g> { <http://e/s> <http://e/p> [ ex:q <http://e/r> ] . }",
		"@prefix ex: ex:relative .",
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			if _, err := collect2(trig.Decode(strings.NewReader(src))); err == nil {
				t.Error("Decode() = nil, want an error")
			}
		})
	}
}

// TestCommentsAreDropped covers the two sinks that read what a document means
// rather than what it wrote: a comment costs them nothing and reaches neither
// the quads nor the dataset.
func TestCommentsAreDropped(t *testing.T) {
	src := "# one\n" +
		"<http://e/g> { # two\n" +
		"  <http://e/s> <http://e/p> <http://e/o> . # three\n" +
		"} # four\n"

	quads := decodeTrig(t, src)
	if len(quads) != 1 {
		t.Errorf("Decode() yielded %d quads, want 1:\n%s", len(quads), formatQuads(quads))
	}

	d, err := trig.Dataset(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Dataset() = %v, want nil", err)
	}
	if got := d.Len(); got != 1 {
		t.Errorf("Dataset() holds %d triples, want 1", got)
	}
}

// TestEncodeShorthandLexicalForms covers the productions a literal has to
// match before it may be written without quotes. The datatype is not enough:
// what is written has to read back as the literal it was written for.
func TestEncodeShorthandLexicalForms(t *testing.T) {
	testCases := []struct {
		datatype rdf.IRI
		value    string
		want     string
	}{
		{datatype: vocab.XSDInteger, value: "42", want: "42"},
		{datatype: vocab.XSDInteger, value: "-0042", want: "-0042"},
		{datatype: vocab.XSDInteger, value: "4.2", want: `"4.2"^^<http://www.w3.org/2001/XMLSchema#integer>`},
		{datatype: vocab.XSDDecimal, value: "-.5", want: "-.5"},
		{datatype: vocab.XSDDecimal, value: "5", want: `"5"^^<http://www.w3.org/2001/XMLSchema#decimal>`},
		{datatype: vocab.XSDDouble, value: "1.0E-6", want: "1.0E-6"},
		{datatype: vocab.XSDDouble, value: ".5e6", want: ".5e6"},
		{datatype: vocab.XSDDouble, value: "1.0", want: `"1.0"^^<http://www.w3.org/2001/XMLSchema#double>`},
		{datatype: vocab.XSDDouble, value: "e6", want: `"e6"^^<http://www.w3.org/2001/XMLSchema#double>`},
		{datatype: vocab.XSDBoolean, value: "true", want: "true"},
		{datatype: vocab.XSDBoolean, value: "1", want: `"1"^^<http://www.w3.org/2001/XMLSchema#boolean>`},
		{datatype: rdf.XSDString, value: "text", want: `"text"`},
	}

	for _, tc := range testCases {
		t.Run(string(tc.datatype)+" "+tc.value, func(t *testing.T) {
			literal, err := rdf.NewTypedLiteral(tc.value, tc.datatype)
			if err != nil {
				t.Fatalf("NewTypedLiteral() = %v, want nil", err)
			}

			d := datasetOf(t, []rdf.Quad{{
				Triple: rdf.Triple{
					Subject:   rdf.IRI("http://e/s"),
					Predicate: "http://e/p",
					Object:    literal,
				},
				Graph: rdf.IRI("http://e/g"),
			}})

			want := "<http://e/g> {\n    <http://e/s> <http://e/p> " + tc.want + " .\n}\n"
			if got := encodeToString(t, d); got != want {
				t.Errorf("Encode() = %q, want %q", got, want)
			}

			// Whichever form was chosen, it has to read back as the literal it
			// was chosen for.
			read := decodeTrig(t, want)
			if len(read) != 1 || !read[0].Object.Equal(literal) {
				t.Errorf("Decode(Encode(%s)) = %s, want the literal back", literal, formatQuads(read))
			}
		})
	}
}

// TestEncodeLocalNames covers the local parts PN_LOCAL can and cannot hold: a
// name it has no room for has to fall back to the IRI written out in full.
func TestEncodeLocalNames(t *testing.T) {
	testCases := []struct {
		name  string
		value rdf.IRI
		want  string
	}{
		{name: "a combining mark may not begin a local name", value: "http://e/̀x", want: "<http://e/̀x>"},
		{name: "a character PN_CHARS has no room for", value: "http://e/a b", want: `<http://e/a\u0020b>`},
		{name: "a digit may begin one", value: "http://e/0a", want: "ex:0a"},
		{name: "a dot inside one", value: "http://e/a.b", want: "ex:a.b"},
		{name: "a dot at the end of one", value: "http://e/a.", want: `ex:a\.`},
		{name: "a hyphen first", value: "http://e/-a", want: `ex:\-a`},
		{name: "a percent escape", value: "http://e/a%20b", want: "ex:a%20b"},
		{name: "a lone percent", value: "http://e/a%zz", want: `ex:a\%zz`},
		{name: "an underscore", value: "http://e/_a_", want: "ex:_a_"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := datasetOf(t, []rdf.Quad{{
				Triple: rdf.Triple{
					Subject:   rdf.IRI("http://e/s"),
					Predicate: "http://e/p",
					Object:    tc.value,
				},
			}})

			got := encodeToString(t, d, trig.WithPrefixes(map[string]string{"ex": "http://e/"}))
			want := "@prefix ex: <http://e/> .\nex:s ex:p " + tc.want + " .\n"
			if got != want {
				t.Errorf("Encode() = %q, want %q", got, want)
			}
		})
	}
}

// TestEncodeChoosesTheLongestPrefix covers what happens when two namespaces
// both match: the more specific one is the one worth having.
func TestEncodeChoosesTheLongestPrefix(t *testing.T) {
	d := datasetOf(t, decodeNQuads(t, "<http://e/vocab/s> <http://e/p> <http://e/vocab/o> ."))

	want := "@prefix e: <http://e/> .\n@prefix ev: <http://e/vocab/> .\nev:s e:p ev:o .\n"
	got := encodeToString(t, d, trig.WithPrefixes(map[string]string{
		"e":  "http://e/",
		"ev": "http://e/vocab/",
	}))
	if got != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

// TestEncodeRelabelsBlankNodes covers the two labels TriG cannot be given: one
// its grammar has no room for, and one already spoken for.
func TestEncodeRelabelsBlankNodes(t *testing.T) {
	testCases := []struct {
		name   string
		labels []string
		want   string
	}{
		{
			name:   "a label the grammar admits is kept",
			labels: []string{"abc"},
			want:   "_:abc <http://e/p> _:abc .\n",
		},
		{
			name:   "a label ending in a dot is replaced",
			labels: []string{"a."},
			want:   "_:b0 <http://e/p> _:b0 .\n",
		},
		{
			name:   "the empty label is replaced",
			labels: []string{""},
			want:   "_:b0 <http://e/p> _:b0 .\n",
		},
		{
			name:   "a minted label does not collide with one still to come",
			labels: []string{"a.", "b0"},
			want:   "_:b0 <http://e/p> _:b0 .\n_:b1 <http://e/p> _:b1 .\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var quads []rdf.Quad
			for _, label := range tc.labels {
				node := rdf.NewBlankNode(label)
				quads = append(quads, rdf.Quad{Triple: rdf.Triple{
					Subject:   node,
					Predicate: "http://e/p",
					Object:    node,
				}})
			}

			if got := encodeToString(t, datasetOf(t, quads)); got != tc.want {
				t.Errorf("Encode() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEncodeAcceptsTheEmptyPrefix covers the one label PN_PREFIX does not
// admit and PNAME_NS does.
func TestEncodeAcceptsTheEmptyPrefix(t *testing.T) {
	d := datasetOf(t, decodeNQuads(t, "<http://e/s> <http://e/p> <http://e/o> <http://e/g> ."))

	want := "@prefix : <http://e/> .\n:g {\n    :s :p :o .\n}\n"
	got := encodeToString(t, d, trig.WithPrefixes(map[string]string{"": "http://e/"}))
	if got != want {
		t.Errorf("Encode() = %q, want %q", got, want)
	}
}

// TestPrintWraps covers the line width, which a graph block indents from
// rather than resets.
func TestPrintWraps(t *testing.T) {
	src := "@prefix ex: <http://e/> .\n" +
		"ex:g { ex:s ex:p ex:objectOne, ex:objectTwo, ex:objectThree . }"

	want := "@prefix ex: <http://e/> .\n" +
		"ex:g {\n" +
		"    ex:s ex:p ex:objectOne,\n" +
		"            ex:objectTwo,\n" +
		"            ex:objectThree .\n" +
		"}\n"

	if got := printToString(t, parse(t, src), trig.WithLineWidth(30)); got != want {
		t.Errorf("Print() = %q, want %q", got, want)
	}
}
