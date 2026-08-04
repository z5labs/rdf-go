package turtle_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/z5labs/rdf-go/internal/fuzzcorpus"
	"github.com/z5labs/rdf-go/rdf12/turtle"
)

// FuzzParse checks what a parser owes a document it did not ask for: whatever
// bytes it is handed, it returns — with a tree or with an error, but it
// returns. A panic, a loop that never ends, or a document a megabyte long
// costing a gigabyte to read are all the same failure to a caller parsing
// input from somewhere it does not control.
//
// Turtle gives a fuzzer more to work with than the line-based syntaxes do:
// collections, blank node property lists, triple terms and annotations all
// nest, so a mutation that adds a bracket adds a level of recursion, and the
// parser has to end rather than descend for as long as the brackets keep
// coming.
//
// What it asserts beyond that is the round trip of TestPrintRoundTrip, over
// documents nobody wrote by hand: a document that parses prints, and what was
// printed parses back to the tree that was printed. A parser and a printer that
// disagree about a document neither of them rejects lose it silently, which is
// worse than either of them failing.
//
// [turtle.Decode] is drained as well because it is the other entry point a
// document arrives through, and it runs the prefix and base resolution and the
// lowering to data model terms that [turtle.Parse] does not.
//
// The seed corpus is both Turtle suites, whose bad-syntax documents are the
// negative cases. An RDF 1.2 parser reads RDF 1.1, so the older suite is as
// much this parser's input as the newer one.
func FuzzParse(f *testing.F) {
	seeds, err := fuzzcorpus.Seeds(".ttl", "rdf12/rdf-turtle", "rdf11/rdf-turtle")
	if err != nil {
		f.Fatalf("Seeds() = %v, want nil", err)
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src string) {
		for _, err := range turtle.Decode(strings.NewReader(src)) {
			if err != nil {
				break
			}
		}

		doc, err := turtle.Parse(strings.NewReader(src))
		if err != nil {
			return
		}

		printed := printToString(t, doc)

		reparsed, err := turtle.Parse(strings.NewReader(printed))
		if err != nil {
			t.Fatalf("Parse(Print(doc)) = %v, want nil; printed = %q", err, printed)
		}

		zeroDocument(doc)
		zeroDocument(reparsed)
		if !reflect.DeepEqual(doc, reparsed) {
			t.Errorf(
				"round trip changed the tree; src = %q, printed = %q\nfirst  %s\nsecond %s",
				src, printed, formatDocument(doc), formatDocument(reparsed),
			)
		}
	})
}
