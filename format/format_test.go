package format_test

import (
	"errors"
	"iter"
	"slices"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/format"
)

// collect drains a decoded sequence, returning the quads yielded before any
// error and the error that stopped it.
func collect(seq iter.Seq2[rdf.Quad, error]) ([]rdf.Quad, error) {
	var quads []rdf.Quad
	for quad, err := range seq {
		if err != nil {
			return quads, err
		}
		quads = append(quads, quad)
	}
	return quads, nil
}

func TestFromMediaType(t *testing.T) {
	testCases := []struct {
		name      string
		mediaType string
		syntax    format.Syntax
		version   format.Version
	}{
		{
			name:      "n-triples",
			mediaType: "application/n-triples",
			syntax:    format.NTriples,
			version:   format.RDF11,
		},
		{
			name:      "n-quads",
			mediaType: "application/n-quads",
			syntax:    format.NQuads,
			version:   format.RDF11,
		},
		{
			name:      "turtle",
			mediaType: "text/turtle",
			syntax:    format.Turtle,
			version:   format.RDF11,
		},
		{
			name:      "trig",
			mediaType: "application/trig",
			syntax:    format.TriG,
			version:   format.RDF11,
		},
		{
			name:      "the version parameter selects rdf 1.2",
			mediaType: "text/turtle;version=1.2",
			syntax:    format.Turtle,
			version:   format.RDF12,
		},
		{
			name:      "an explicit version=1.1 selects rdf 1.1",
			mediaType: "application/trig;version=1.1",
			syntax:    format.TriG,
			version:   format.RDF11,
		},
		{
			name:      "every syntax has an rdf 1.2 decoder",
			mediaType: "application/n-quads; version=1.2",
			syntax:    format.NQuads,
			version:   format.RDF12,
		},
		{
			name:      "a quoted version parameter",
			mediaType: `application/n-triples; version="1.2"`,
			syntax:    format.NTriples,
			version:   format.RDF12,
		},
		{
			name:      "an unrelated parameter is ignored",
			mediaType: "text/turtle; charset=utf-8",
			syntax:    format.Turtle,
			version:   format.RDF11,
		},
		{
			name:      "parameters in any order",
			mediaType: "text/turtle; version=1.2; charset=utf-8",
			syntax:    format.Turtle,
			version:   format.RDF12,
		},
		{
			name:      "the type and subtype are matched without regard to case",
			mediaType: "TEXT/Turtle",
			syntax:    format.Turtle,
			version:   format.RDF11,
		},
		{
			name:      "a parameter name is matched without regard to case",
			mediaType: "text/turtle; VERSION=1.2",
			syntax:    format.Turtle,
			version:   format.RDF12,
		},
		{
			name:      "surrounding white space is ignored",
			mediaType: "  application/trig  ",
			syntax:    format.TriG,
			version:   format.RDF11,
		},
		{
			name:      "white space around a parameter is ignored",
			mediaType: "text/turtle ;  version=1.2",
			syntax:    format.Turtle,
			version:   format.RDF12,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			f, err := format.FromMediaType(testCase.mediaType)
			if err != nil {
				t.Fatalf("FromMediaType(%q) = %v", testCase.mediaType, err)
			}
			if f.Syntax() != testCase.syntax {
				t.Errorf("syntax = %q, want %q", f.Syntax(), testCase.syntax)
			}
			if f.Version() != testCase.version {
				t.Errorf("version = %s, want %s", f.Version(), testCase.version)
			}
		})
	}
}

func TestFromMediaTypeErrors(t *testing.T) {
	testCases := []struct {
		name      string
		mediaType string
		expected  error
	}{
		{
			name:      "a media type no format claims",
			mediaType: "application/rdf+xml",
			expected:  format.ErrUnknownMediaType,
		},
		{
			name:      "a media type with the right subtype under the wrong type",
			mediaType: "text/n-triples",
			expected:  format.ErrUnknownMediaType,
		},
		{
			name:      "a version no decoder implements",
			mediaType: "text/turtle;version=1.3",
			expected:  format.ErrUnsupportedVersion,
		},
		{
			name:      "a version parameter that is not a version",
			mediaType: `text/turtle;version="rdf 1.2"`,
			expected:  format.ErrUnsupportedVersion,
		},
		{
			name:      "an empty media type",
			mediaType: "",
			expected:  nil,
		},
		{
			name:      "a media type that does not parse",
			mediaType: "text/turtle; charset",
			expected:  nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			f, err := format.FromMediaType(testCase.mediaType)
			if err == nil {
				t.Fatalf("FromMediaType(%q) = %s, want an error", testCase.mediaType, f)
			}
			if testCase.expected != nil && !errors.Is(err, testCase.expected) {
				t.Errorf("error = %v, want one wrapping %v", err, testCase.expected)
			}

			// The zero Format is returned beside the error, and decoding with
			// it says so rather than panicking on a nil decoder.
			assertNoDecoder(t, f)
		})
	}
}

func TestFromExtension(t *testing.T) {
	testCases := []struct {
		name      string
		extension string
		syntax    format.Syntax
	}{
		{
			name:      "n-triples",
			extension: ".nt",
			syntax:    format.NTriples,
		},
		{
			name:      "n-quads",
			extension: ".nq",
			syntax:    format.NQuads,
		},
		{
			name:      "turtle",
			extension: ".ttl",
			syntax:    format.Turtle,
		},
		{
			name:      "trig",
			extension: ".trig",
			syntax:    format.TriG,
		},
		{
			name:      "the leading dot is optional",
			extension: "ttl",
			syntax:    format.Turtle,
		},
		{
			name:      "an extension is matched without regard to case",
			extension: ".TriG",
			syntax:    format.TriG,
		},
		{
			name:      "surrounding white space is ignored",
			extension: "  .nq ",
			syntax:    format.NQuads,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			f, err := format.FromExtension(testCase.extension)
			if err != nil {
				t.Fatalf("FromExtension(%q) = %v", testCase.extension, err)
			}
			if f.Syntax() != testCase.syntax {
				t.Errorf("syntax = %q, want %q", f.Syntax(), testCase.syntax)
			}

			// A file name has nowhere to say which version of the
			// specification it holds, so an extension is always RDF 1.1.
			if f.Version() != format.RDF11 {
				t.Errorf("version = %s, want %s", f.Version(), format.RDF11)
			}
		})
	}
}

func TestFromExtensionErrors(t *testing.T) {
	testCases := []struct {
		name      string
		extension string
	}{
		{
			name:      "an extension no format claims",
			extension: ".rdf",
		},
		{
			name:      "an empty extension",
			extension: "",
		},
		{
			name:      "a bare dot",
			extension: ".",
		},
		{
			name:      "a file name rather than an extension",
			extension: "graph.ttl",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			f, err := format.FromExtension(testCase.extension)
			if err == nil {
				t.Fatalf("FromExtension(%q) = %s, want an error", testCase.extension, f)
			}
			if !errors.Is(err, format.ErrUnknownExtension) {
				t.Errorf("error = %v, want one wrapping %v", err, format.ErrUnknownExtension)
			}
			assertNoDecoder(t, f)
		})
	}
}

// TestMediaTypeAndExtensionAgree checks the acceptance criterion that the
// extensions map to the same decoders the media types do, by looking the same
// format up both ways.
func TestMediaTypeAndExtensionAgree(t *testing.T) {
	testCases := []struct {
		name      string
		mediaType string
		extension string
	}{
		{
			name:      "n-triples",
			mediaType: "application/n-triples",
			extension: ".nt",
		},
		{
			name:      "n-quads",
			mediaType: "application/n-quads",
			extension: ".nq",
		},
		{
			name:      "turtle",
			mediaType: "text/turtle",
			extension: ".ttl",
		},
		{
			name:      "trig",
			mediaType: "application/trig",
			extension: ".trig",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			byType, err := format.FromMediaType(testCase.mediaType)
			if err != nil {
				t.Fatalf("FromMediaType(%q) = %v", testCase.mediaType, err)
			}
			byExtension, err := format.FromExtension(testCase.extension)
			if err != nil {
				t.Fatalf("FromExtension(%q) = %v", testCase.extension, err)
			}

			if byType.Syntax() != byExtension.Syntax() || byType.Version() != byExtension.Version() {
				t.Errorf("FromMediaType = %s, FromExtension = %s", byType, byExtension)
			}
			if !slices.Contains(byType.Extensions(), testCase.extension) {
				t.Errorf("Extensions() = %q, want one containing %q", byType.Extensions(), testCase.extension)
			}
			if byExtension.MediaType() != testCase.mediaType {
				t.Errorf("MediaType() = %q, want %q", byExtension.MediaType(), testCase.mediaType)
			}
		})
	}
}

func TestLookup(t *testing.T) {
	syntaxes := []format.Syntax{format.NTriples, format.NQuads, format.Turtle, format.TriG}
	versions := []format.Version{format.RDF11, format.RDF12}

	for _, syntax := range syntaxes {
		for _, version := range versions {
			t.Run(string(syntax)+" "+version.String(), func(t *testing.T) {
				f, err := format.Lookup(syntax, version)
				if err != nil {
					t.Fatalf("Lookup(%q, %s) = %v", syntax, version, err)
				}
				if f.Syntax() != syntax || f.Version() != version {
					t.Errorf("Lookup(%q, %s) = %s", syntax, version, f)
				}
			})
		}
	}

	t.Run("a syntax this module does not implement", func(t *testing.T) {
		f, err := format.Lookup("rdf/xml", format.RDF11)
		if !errors.Is(err, format.ErrUnknownSyntax) {
			t.Fatalf("error = %v, want one wrapping %v", err, format.ErrUnknownSyntax)
		}
		assertNoDecoder(t, f)
	})

	t.Run("a version this module does not implement", func(t *testing.T) {
		f, err := format.Lookup(format.Turtle, format.Version(7))
		if !errors.Is(err, format.ErrUnsupportedVersion) {
			t.Fatalf("error = %v, want one wrapping %v", err, format.ErrUnsupportedVersion)
		}
		assertNoDecoder(t, f)
	})
}

// TestDecode reads one document per registered format, checking that the
// decoder a lookup hands back is the one for that syntax and that a graph
// syntax yields quads in the default graph.
func TestDecode(t *testing.T) {
	triple := rdf.Triple{
		Subject:   rdf.IRI("http://example.com/s"),
		Predicate: "http://example.com/p",
		Object:    rdf.IRI("http://example.com/o"),
	}

	testCases := []struct {
		name      string
		mediaType string
		src       string
		expected  []rdf.Quad
	}{
		{
			name:      "n-triples yields quads in the default graph",
			mediaType: "application/n-triples",
			src:       "<http://example.com/s> <http://example.com/p> <http://example.com/o> .\n",
			expected:  []rdf.Quad{{Triple: triple}},
		},
		{
			name:      "turtle yields quads in the default graph",
			mediaType: "text/turtle",
			src:       "@prefix ex: <http://example.com/> .\nex:s ex:p ex:o .\n",
			expected:  []rdf.Quad{{Triple: triple}},
		},
		{
			name:      "n-quads keeps the graph name",
			mediaType: "application/n-quads",
			src:       "<http://example.com/s> <http://example.com/p> <http://example.com/o> <http://example.com/g> .\n",
			expected:  []rdf.Quad{{Triple: triple, Graph: rdf.IRI("http://example.com/g")}},
		},
		{
			name:      "trig keeps the graph name",
			mediaType: "application/trig",
			src:       "@prefix ex: <http://example.com/> .\nex:g { ex:s ex:p ex:o }\n",
			expected:  []rdf.Quad{{Triple: triple, Graph: rdf.IRI("http://example.com/g")}},
		},
		{
			name:      "an rdf 1.2 n-triples document",
			mediaType: "application/n-triples;version=1.2",
			src: "VERSION \"1.2\"\n" +
				"<http://example.com/s> <http://example.com/p> <http://example.com/o> .\n",
			expected: []rdf.Quad{{Triple: triple}},
		},
		{
			name:      "an rdf 1.2 turtle document",
			mediaType: "text/turtle;version=1.2",
			src:       "VERSION \"1.2\"\n@prefix ex: <http://example.com/> .\nex:s ex:p ex:o .\n",
			expected:  []rdf.Quad{{Triple: triple}},
		},
		{
			name:      "an rdf 1.2 n-quads document",
			mediaType: "application/n-quads;version=1.2",
			src: "VERSION \"1.2\"\n" +
				"<http://example.com/s> <http://example.com/p> <http://example.com/o> <http://example.com/g> .\n",
			expected: []rdf.Quad{{Triple: triple, Graph: rdf.IRI("http://example.com/g")}},
		},
		{
			name:      "an rdf 1.2 trig document",
			mediaType: "application/trig;version=1.2",
			src:       "VERSION \"1.2\"\n@prefix ex: <http://example.com/> .\nex:g { ex:s ex:p ex:o }\n",
			expected:  []rdf.Quad{{Triple: triple, Graph: rdf.IRI("http://example.com/g")}},
		},
		{
			name:      "an empty document",
			mediaType: "text/turtle",
			src:       "",
			expected:  nil,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			f, err := format.FromMediaType(testCase.mediaType)
			if err != nil {
				t.Fatalf("FromMediaType(%q) = %v", testCase.mediaType, err)
			}

			quads, err := collect(f.Decode(strings.NewReader(testCase.src)))
			if err != nil {
				t.Fatalf("Decode = %v", err)
			}
			if len(quads) != len(testCase.expected) {
				t.Fatalf("decoded %d quads, want %d: %v", len(quads), len(testCase.expected), quads)
			}
			for i, quad := range quads {
				if !quad.Equal(testCase.expected[i]) {
					t.Errorf("quad %d = %s, want %s", i, quad, testCase.expected[i])
				}
			}
		})
	}
}

// TestDecodeIsVersioned checks that the version parameter picks a different
// decoder rather than the same one twice, by handing each version a document
// only the other reads: the RDF 1.2 triple term is a syntax error in RDF 1.1,
// and RDF 1.2 refuses a version directive naming a version it is not.
func TestDecodeIsVersioned(t *testing.T) {
	const tripleTerm = "<http://example.com/s> <http://example.com/p> " +
		"<<( <http://example.com/a> <http://example.com/b> \"v\" )>> .\n"

	t.Run("rdf 1.1 refuses a triple term", func(t *testing.T) {
		f, err := format.FromMediaType("application/n-triples")
		if err != nil {
			t.Fatalf("FromMediaType = %v", err)
		}
		if _, err := collect(f.Decode(strings.NewReader(tripleTerm))); err == nil {
			t.Fatal("decoded an RDF 1.2 triple term with the RDF 1.1 decoder")
		}
	})

	t.Run("rdf 1.2 reads a triple term", func(t *testing.T) {
		f, err := format.FromMediaType("application/n-triples;version=1.2")
		if err != nil {
			t.Fatalf("FromMediaType = %v", err)
		}
		quads, err := collect(f.Decode(strings.NewReader(tripleTerm)))
		if err != nil {
			t.Fatalf("Decode = %v", err)
		}
		if len(quads) != 1 {
			t.Fatalf("decoded %d quads, want 1", len(quads))
		}
		if _, ok := quads[0].Object.(rdf.TripleTerm); !ok {
			t.Errorf("object = %v, want an rdf.TripleTerm", quads[0].Object)
		}
	})
}

// TestDecodeStopsEarly checks that a caller who breaks out of the range over a
// graph syntax stops the underlying decoder rather than being handed the rest
// of the document. An adapter that kept ranging over the triples after its own
// consumer left would yield again after the loop exited, which the runtime
// turns into a panic.
func TestDecodeStopsEarly(t *testing.T) {
	f, err := format.FromExtension(".ttl")
	if err != nil {
		t.Fatalf("FromExtension = %v", err)
	}

	src := "@prefix ex: <http://example.com/> .\n" +
		"ex:s ex:p ex:a, ex:b, ex:c .\n"

	var count int
	for _, err := range f.Decode(strings.NewReader(src)) {
		if err != nil {
			t.Fatalf("Decode = %v", err)
		}
		count++
		break
	}
	if count != 1 {
		t.Errorf("yielded %d quads after break, want 1", count)
	}
}

func TestZeroFormat(t *testing.T) {
	var f format.Format

	t.Run("decoding reports there is no decoder", func(t *testing.T) {
		assertNoDecoder(t, f)
	})

	t.Run("it describes itself as the zero value", func(t *testing.T) {
		if got := f.String(); !strings.Contains(got, "zero") {
			t.Errorf("String() = %q, want it to name the zero Format", got)
		}
	})

	t.Run("it names no syntax, media type or extension", func(t *testing.T) {
		if f.Syntax() != "" {
			t.Errorf("Syntax() = %q, want empty", f.Syntax())
		}
		if f.MediaType() != "" {
			t.Errorf("MediaType() = %q, want empty", f.MediaType())
		}
		if got := f.Extensions(); len(got) != 0 {
			t.Errorf("Extensions() = %q, want none", got)
		}
	})
}

func TestFormats(t *testing.T) {
	formats := format.Formats()

	t.Run("every syntax is registered at every version", func(t *testing.T) {
		if len(formats) != 8 {
			t.Fatalf("Formats() returned %d formats, want 8", len(formats))
		}
		for _, syntax := range []format.Syntax{format.NTriples, format.NQuads, format.Turtle, format.TriG} {
			for _, version := range []format.Version{format.RDF11, format.RDF12} {
				found := slices.ContainsFunc(formats, func(f format.Format) bool {
					return f.Syntax() == syntax && f.Version() == version
				})
				if !found {
					t.Errorf("no %s %s in Formats()", syntax, version)
				}
			}
		}
	})

	t.Run("every format has a media type, an extension and a decoder", func(t *testing.T) {
		for _, f := range formats {
			if f.MediaType() == "" {
				t.Errorf("%s has no media type", f)
			}
			if len(f.Extensions()) == 0 {
				t.Errorf("%s has no extensions", f)
			}
			if _, err := collect(f.Decode(strings.NewReader(""))); err != nil {
				t.Errorf("%s decoding an empty document = %v", f, err)
			}
		}
	})

	t.Run("the registry is not shared with the caller", func(t *testing.T) {
		formats[0] = format.Format{}
		if again := format.Formats(); again[0].Syntax() == "" {
			t.Error("mutating the result of Formats() changed the registry")
		}
	})

	t.Run("extensions are not shared with the caller", func(t *testing.T) {
		f, err := format.FromExtension(".ttl")
		if err != nil {
			t.Fatalf("FromExtension = %v", err)
		}
		f.Extensions()[0] = ".nope"
		if got := f.Extensions()[0]; got != ".ttl" {
			t.Errorf("Extensions()[0] = %q after mutation, want %q", got, ".ttl")
		}
	})
}

func TestVersionString(t *testing.T) {
	testCases := []struct {
		name     string
		version  format.Version
		expected string
	}{
		{
			name:     "rdf 1.1",
			version:  format.RDF11,
			expected: "1.1",
		},
		{
			name:     "rdf 1.2",
			version:  format.RDF12,
			expected: "1.2",
		},
		{
			name:     "the zero version is rdf 1.1",
			version:  format.Version(0),
			expected: "1.1",
		},
		{
			name:     "a version this module does not implement",
			version:  format.Version(7),
			expected: "Version(7)",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.version.String(); got != testCase.expected {
				t.Errorf("String() = %q, want %q", got, testCase.expected)
			}
		})
	}
}

// assertNoDecoder checks that decoding with f reports [format.ErrNoDecoder]
// rather than panicking, which is what the zero Format returned beside every
// lookup error has to do.
func assertNoDecoder(t *testing.T, f format.Format) {
	t.Helper()

	_, err := collect(f.Decode(strings.NewReader("")))
	if !errors.Is(err, format.ErrNoDecoder) {
		t.Errorf("Decode = %v, want one wrapping %v", err, format.ErrNoDecoder)
	}
}
