package ntriples_test

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/rdf11/ntriples"
)

// collect2 drains a decoded sequence, returning the triples yielded before any
// error and the error that stopped it.
func collect2(seq iter.Seq2[rdf.Triple, error]) ([]rdf.Triple, error) {
	var triples []rdf.Triple
	for triple, err := range seq {
		if err != nil {
			return triples, err
		}
		triples = append(triples, triple)
	}
	return triples, nil
}

func mustLiteral(t *testing.T, value, language string, datatype rdf.IRI) rdf.Literal {
	t.Helper()

	switch {
	case language != "":
		literal, err := rdf.NewLanguageLiteral(value, language)
		if err != nil {
			t.Fatalf("NewLanguageLiteral(%q, %q) = %v", value, language, err)
		}
		return literal
	case datatype != "":
		literal, err := rdf.NewTypedLiteral(value, datatype)
		if err != nil {
			t.Fatalf("NewTypedLiteral(%q, %q) = %v", value, datatype, err)
		}
		return literal
	default:
		return rdf.NewLiteral(value)
	}
}

func TestDecode(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected func(t *testing.T) []rdf.Triple
	}{
		{
			name: "an empty document",
			src:  "",
			expected: func(*testing.T) []rdf.Triple {
				return nil
			},
		},
		{
			name: "a triple of iris",
			src:  `<http://example.com/s> <http://example.com/p> <http://example.com/o> .`,
			expected: func(*testing.T) []rdf.Triple {
				return []rdf.Triple{
					{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    rdf.IRI("http://example.com/o"),
					},
				}
			},
		},
		{
			name: "a plain literal takes the xsd:string datatype",
			src:  `<http://example.com/s> <http://example.com/p> "text" .`,
			expected: func(t *testing.T) []rdf.Triple {
				return []rdf.Triple{
					{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    mustLiteral(t, "text", "", ""),
					},
				}
			},
		},
		{
			name: "a tagged literal takes the rdf:langString datatype",
			src:  `<http://example.com/s> <http://example.com/p> "text"@en-GB .`,
			expected: func(t *testing.T) []rdf.Triple {
				return []rdf.Triple{
					{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    mustLiteral(t, "text", "en-GB", ""),
					},
				}
			},
		},
		{
			name: "a typed literal keeps the datatype it was written with",
			src:  `<http://example.com/s> <http://example.com/p> "1"^^<http://www.w3.org/2001/XMLSchema#integer> .`,
			expected: func(t *testing.T) []rdf.Triple {
				return []rdf.Triple{
					{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    mustLiteral(t, "1", "", "http://www.w3.org/2001/XMLSchema#integer"),
					},
				}
			},
		},
		{
			name: "an explicit xsd:string is the same literal as a plain one",
			src:  `<http://example.com/s> <http://example.com/p> "text"^^<http://www.w3.org/2001/XMLSchema#string> .`,
			expected: func(t *testing.T) []rdf.Triple {
				return []rdf.Triple{
					{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    mustLiteral(t, "text", "", ""),
					},
				}
			},
		},
		{
			name: "escapes are decoded on the way through",
			src:  `<http://example.com/s> <http://example.com/p> "a\nbé" .`,
			expected: func(t *testing.T) []rdf.Triple {
				return []rdf.Triple{
					{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    mustLiteral(t, "a\nbé", "", ""),
					},
				}
			},
		},
		{
			name: "several statements arrive in order",
			src: "<http://example.com/a> <http://example.com/p> \"1\" .\n" +
				"<http://example.com/b> <http://example.com/p> \"2\" .\n" +
				"<http://example.com/c> <http://example.com/p> \"3\" .\n",
			expected: func(t *testing.T) []rdf.Triple {
				return []rdf.Triple{
					{Subject: rdf.IRI("http://example.com/a"), Predicate: "http://example.com/p", Object: mustLiteral(t, "1", "", "")},
					{Subject: rdf.IRI("http://example.com/b"), Predicate: "http://example.com/p", Object: mustLiteral(t, "2", "", "")},
					{Subject: rdf.IRI("http://example.com/c"), Predicate: "http://example.com/p", Object: mustLiteral(t, "3", "", "")},
				}
			},
		},
		{
			name: "comments and blank lines are passed over",
			src:  "# a note\n\n<http://example.com/s> <http://example.com/p> \"v\" . # trailing\n\n# last\n",
			expected: func(t *testing.T) []rdf.Triple {
				return []rdf.Triple{
					{
						Subject:   rdf.IRI("http://example.com/s"),
						Predicate: "http://example.com/p",
						Object:    mustLiteral(t, "v", "", ""),
					},
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := collect2(ntriples.Decode(strings.NewReader(tc.src)))
			if err != nil {
				t.Fatalf("Decode() = %v, want nil", err)
			}

			want := tc.expected(t)
			if !slices.EqualFunc(got, want, rdf.Triple.Equal) {
				t.Errorf("Decode() = %v, want %v", got, want)
			}
		})
	}
}

// TestDecodeScopesBlankNodes covers the criterion that keeps two documents
// from being merged by accident: a label means the same node throughout one
// document and nothing at all outside it.
func TestDecodeScopesBlankNodes(t *testing.T) {
	src := "_:a <http://example.com/p> _:b .\n_:b <http://example.com/p> _:a .\n"

	t.Run("one label is one node throughout a document", func(t *testing.T) {
		triples, err := collect2(ntriples.Decode(strings.NewReader(src)))
		if err != nil {
			t.Fatalf("Decode() = %v, want nil", err)
		}
		if len(triples) != 2 {
			t.Fatalf("decoded %d triples, want 2", len(triples))
		}

		// _:a is the subject of the first and the object of the second.
		if !triples[0].Subject.Equal(triples[1].Object) {
			t.Errorf("_:a decoded as %s then %s, want one node", triples[0].Subject, triples[1].Object)
		}
		if !triples[0].Object.Equal(triples[1].Subject) {
			t.Errorf("_:b decoded as %s then %s, want one node", triples[0].Object, triples[1].Subject)
		}
		if triples[0].Subject.Equal(triples[0].Object) {
			t.Error("_:a and _:b decoded as the same node, want distinct ones")
		}
	})

	t.Run("the same label in two documents is two nodes", func(t *testing.T) {
		first, err := collect2(ntriples.Decode(strings.NewReader(src)))
		if err != nil {
			t.Fatalf("Decode() = %v, want nil", err)
		}
		second, err := collect2(ntriples.Decode(strings.NewReader(src)))
		if err != nil {
			t.Fatalf("Decode() = %v, want nil", err)
		}

		if first[0].Subject.Equal(second[0].Subject) {
			t.Errorf("both documents decoded _:a as %s, want distinct nodes", first[0].Subject)
		}

		// Distinct nodes, but the same shape — which is exactly the difference
		// isomorphism exists to see past.
		if !rdf.Isomorphic(graphOfTriples(t, first), graphOfTriples(t, second)) {
			t.Error("the same document decoded twice gave graphs that are not isomorphic")
		}
	})
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

func TestDecodeErrors(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name:        "a tokenizer error carries its character position",
			src:         `$ <http://example.com/p> _:o .`,
			expectedErr: ntriples.UnexpectedCharacterError{Pos: pos(1, 1), R: '$'},
		},
		{
			name: "a parse error carries its token position",
			src:  `"nope" <http://example.com/p> _:o .`,
			expectedErr: ntriples.UnexpectedTokenError{
				Expected: []ntriples.TokenType{ntriples.TokenIRIRef, ntriples.TokenBlankNodeLabel},
				Actual: ntriples.Token{
					Pos:   pos(1, 1),
					Type:  ntriples.TokenStringLiteral,
					Value: []byte("nope"),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect2(ntriples.Decode(strings.NewReader(tc.src)))
			if !reflect.DeepEqual(err, tc.expectedErr) {
				t.Errorf("Decode() error = %#v, want %#v", err, tc.expectedErr)
			}
		})
	}
}

// TestDecodeInvalidTerms covers what the grammar accepts but the data model
// refuses. Each has to name the position of the term at fault, and to keep the
// rdf package's error reachable underneath.
func TestDecodeInvalidTerms(t *testing.T) {
	testCases := []struct {
		name    string
		src     string
		wantPos ntriples.Pos
		wantErr error
	}{
		{
			// RDF 1.1 asks for a well-formed language tag as RDF 1.2 does, and
			// LANGTAG admits any run of letters just as LANG_DIR does, so the
			// rule the rdf package states reaches this syntax too.
			name:    "a language tag that is not well-formed by RFC 5646",
			src:     `<http://example.com/s> <http://example.com/p> "v"@cantbethislong .`,
			wantPos: pos(1, 50),
			wantErr: rdf.ErrInvalidLanguage,
		},
		{
			name:    "rdf:langString without a language tag",
			src:     `<http://example.com/s> <http://example.com/p> "v"^^<http://www.w3.org/1999/02/22-rdf-syntax-ns#langString> .`,
			wantPos: pos(1, 52),
			wantErr: rdf.ErrReservedDatatype,
		},
		{
			name:    "rdf:dirLangString, which rdf 1.2 added, without a language tag",
			src:     `<http://example.com/s> <http://example.com/p> "v"^^<http://www.w3.org/1999/02/22-rdf-syntax-ns#dirLangString> .`,
			wantPos: pos(1, 52),
			wantErr: rdf.ErrReservedDatatype,
		},
		{
			// The empty IRI is refused as the relative IRI it is, before the
			// data model gets to object to it being empty.
			name:    "an empty datatype iri",
			src:     `<http://example.com/s> <http://example.com/p> "v"^^<> .`,
			wantPos: pos(1, 52),
			wantErr: ntriples.ErrRelativeIRI,
		},
		{
			name:    "an empty predicate iri",
			src:     `<http://example.com/s> <> "v" .`,
			wantPos: pos(1, 24),
			wantErr: ntriples.ErrRelativeIRI,
		},
		{
			name:    "a relative iri",
			src:     `<http://example.com/s> <http://example.com/p> <o> .`,
			wantPos: pos(1, 47),
			wantErr: ntriples.ErrRelativeIRI,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := collect2(ntriples.Decode(strings.NewReader(tc.src)))
			if err == nil {
				t.Fatal("Decode() = nil, want an error")
			}

			var invalid ntriples.InvalidTermError
			if !errors.As(err, &invalid) {
				t.Fatalf("Decode() error = %#v, want an InvalidTermError", err)
			}
			if invalid.Pos != tc.wantPos {
				t.Errorf("Pos = %s, want %s", invalid.Pos, tc.wantPos)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Decode() error = %v, want it to wrap %v", err, tc.wantErr)
			}
		})
	}
}

// TestDecodeYieldsNothingAfterAnError checks the contract the doc states: the
// statements before the bad one are handed over, and nothing after it is.
func TestDecodeYieldsNothingAfterAnError(t *testing.T) {
	src := "<http://example.com/a> <http://example.com/p> \"1\" .\n" +
		"$ <http://example.com/p> \"2\" .\n" +
		"<http://example.com/c> <http://example.com/p> \"3\" .\n"

	var (
		triples []rdf.Triple
		errs    []error
	)
	for triple, err := range ntriples.Decode(strings.NewReader(src)) {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		triples = append(triples, triple)
	}

	if len(errs) != 1 {
		t.Fatalf("yielded %d errors, want 1", len(errs))
	}
	if len(triples) != 1 {
		t.Fatalf("yielded %d triples, want 1", len(triples))
	}
	if got, want := triples[0].Subject, rdf.IRI("http://example.com/a"); !got.Equal(want) {
		t.Errorf("yielded %s, want %s", got, want)
	}
}

func TestDecodeStopsEarly(t *testing.T) {
	src := "<http://example.com/a> <http://example.com/p> \"1\" .\n" +
		"<http://example.com/b> <http://example.com/p> \"2\" .\n" +
		"<http://example.com/c> <http://example.com/p> \"3\" .\n"

	var seen int
	for range ntriples.Decode(strings.NewReader(src)) {
		seen++
		break
	}
	if want := 1; seen != want {
		t.Errorf("yielded %d triples after break, want %d", seen, want)
	}
}

func TestDecodeReaderError(t *testing.T) {
	errBoom := errors.New("boom")

	src := "<http://example.com/a> <http://example.com/p> \"1\" .\n<http://example.com/b>"
	_, err := collect2(ntriples.Decode(&failingReader{data: []byte(src), err: errBoom}))
	if !errors.Is(err, errBoom) {
		t.Errorf("Decode() error = %v, want %v", err, errBoom)
	}
}

// TestTriples covers lowering a document already parsed, which has to agree
// with what Decode would have produced from the same source.
func TestTriples(t *testing.T) {
	src := "_:a <http://example.com/p> \"text\"@en .\n" +
		"<http://example.com/s> <http://example.com/q> _:a .\n" +
		"<http://example.com/s> <http://example.com/r> \"1\"^^<http://www.w3.org/2001/XMLSchema#integer> .\n"

	doc, err := ntriples.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}

	lowered, err := ntriples.Triples(doc)
	if err != nil {
		t.Fatalf("Triples() = %v, want nil", err)
	}
	if got, want := len(lowered), 3; got != want {
		t.Fatalf("Triples() returned %d triples, want %d", got, want)
	}

	t.Run("one scope covers the whole document", func(t *testing.T) {
		if !lowered[0].Subject.Equal(lowered[1].Object) {
			t.Errorf("_:a lowered as %s then %s, want one node", lowered[0].Subject, lowered[1].Object)
		}
	})

	t.Run("the graph matches what Decode produces", func(t *testing.T) {
		streamed, err := collect2(ntriples.Decode(strings.NewReader(src)))
		if err != nil {
			t.Fatalf("Decode() = %v, want nil", err)
		}

		// The blank node labels differ, each scope having minted its own, so
		// the graphs are isomorphic rather than equal.
		if !rdf.Isomorphic(graphOfTriples(t, lowered), graphOfTriples(t, streamed)) {
			t.Error("Triples() and Decode() produced graphs that are not isomorphic")
		}
	})
}

func TestTriplesErrors(t *testing.T) {
	t.Run("a nil document has no triples", func(t *testing.T) {
		triples, err := ntriples.Triples(nil)
		if err != nil {
			t.Fatalf("Triples() = %v, want nil", err)
		}
		if triples != nil {
			t.Errorf("Triples() = %v, want nil", triples)
		}
	})

	t.Run("an invalid term is reported with its position", func(t *testing.T) {
		src := `<http://example.com/s> <http://example.com/p> "v"^^<> .`

		doc, err := ntriples.Parse(strings.NewReader(src))
		if err != nil {
			t.Fatalf("Parse() = %v, want nil", err)
		}

		_, err = ntriples.Triples(doc)
		var invalid ntriples.InvalidTermError
		if !errors.As(err, &invalid) {
			t.Fatalf("Triples() error = %#v, want an InvalidTermError", err)
		}
		if got, want := invalid.Pos, pos(1, 52); got != want {
			t.Errorf("Pos = %s, want %s", got, want)
		}
	})
}

func TestInvalidTermErrorMessage(t *testing.T) {
	err := ntriples.InvalidTermError{Pos: pos(3, 12), Err: rdf.ErrEmptyDatatype}

	want := "invalid term at line 3, column 12: rdf: datatype must not be empty"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// tripleGenerator writes N statements on demand, so that a test can read a
// document far larger than it could hold.
//
// The line buffer is reused rather than allocated per statement, so that what
// the generator costs stays out of what the decoder is measured on.
type tripleGenerator struct {
	remaining int
	scratch   []byte
	pending   []byte
}

func (g *tripleGenerator) Read(p []byte) (int, error) {
	if len(g.pending) == 0 {
		if g.remaining == 0 {
			return 0, io.EOF
		}
		g.remaining--
		g.scratch = fmt.Appendf(g.scratch[:0], "<http://e/s%d> <http://e/p> \"v%d\" .\n", g.remaining, g.remaining)
		g.pending = g.scratch
	}

	n := copy(p, g.pending)
	g.pending = g.pending[n:]
	return n, nil
}

// heapInUse returns the live heap after a collection, so that garbage the
// decoder has already let go of is not counted against it.
func heapInUse() uint64 {
	runtime.GC()

	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

// TestDecodeMemoryDoesNotGrowWithTripleCount is the criterion that makes
// Decode worth having over Parse. It reads a million statements, sampling the
// live heap a tenth of the way in and again at the end: a decoder holding on
// to what it has read would show ten times the heap at the second sample, and
// one that streams shows about the same.
//
// The document is written on demand rather than built up front, since a test
// that had to hold the input in memory would prove nothing about the decoder.
// It names no blank nodes, because remembering those is the one thing Decode
// genuinely cannot do in constant space — see the note on [ntriples.Decode].
func TestDecodeMemoryDoesNotGrowWithTripleCount(t *testing.T) {
	const (
		total = 1_000_000
		early = total / 10
	)

	var (
		count            int
		earlyHeap, final uint64
		lastSubject      rdf.Term
	)

	for triple, err := range ntriples.Decode(&tripleGenerator{remaining: total}) {
		if err != nil {
			t.Fatalf("Decode() = %v, want nil", err)
		}

		count++
		// Keep the most recent triple reachable so the loop cannot be
		// optimized into doing nothing at all.
		lastSubject = triple.Subject

		if count == early {
			earlyHeap = heapInUse()
		}
	}
	final = heapInUse()

	if count != total {
		t.Fatalf("decoded %d triples, want %d", count, total)
	}
	if lastSubject == nil {
		t.Fatal("decoded no subject")
	}

	// Ten times the statements read, so anything retained per statement would
	// be plain at this margin. The allowance is for the heap wandering, not
	// for growth.
	if final > earlyHeap*2 {
		t.Errorf(
			"live heap grew from %d bytes after %d triples to %d bytes after %d",
			earlyHeap, early, final, total,
		)
	}
	t.Logf("live heap: %d bytes after %d triples, %d bytes after %d", earlyHeap, early, final, total)
}
