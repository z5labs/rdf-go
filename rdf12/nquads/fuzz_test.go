package nquads_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/z5labs/rdf-go/internal/fuzzcorpus"
	"github.com/z5labs/rdf-go/rdf12/nquads"
)

// FuzzParse checks what a parser owes a document it did not ask for: whatever
// bytes it is handed, it returns — with a tree or with an error, but it
// returns. A panic, a loop that never ends, or a document a megabyte long
// costing a gigabyte to read are all the same failure to a caller parsing
// input from somewhere it does not control.
//
// What it asserts beyond that is the round trip of TestPrintRoundTrip, over
// documents nobody wrote by hand: a document that parses prints, and what was
// printed parses back to the tree that was printed. A parser and a printer that
// disagree about a document neither of them rejects lose it silently, which is
// worse than either of them failing.
//
// [nquads.Decode] is drained as well because it is the other entry point a
// document arrives through, and it runs the lowering to data model terms that
// [nquads.Parse] does not.
//
// The seed corpus is both N-Quads suites, whose bad-syntax documents are the
// negative cases. An RDF 1.2 parser reads RDF 1.1, so the older suite is as
// much this parser's input as the newer one.
func FuzzParse(f *testing.F) {
	seeds, err := fuzzcorpus.Seeds(".nq", "rdf12/rdf-n-quads", "rdf11/rdf-n-quads")
	if err != nil {
		f.Fatalf("Seeds() = %v, want nil", err)
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src string) {
		for _, err := range nquads.Decode(strings.NewReader(src)) {
			if err != nil {
				break
			}
		}

		doc, err := nquads.Parse(strings.NewReader(src))
		if err != nil {
			return
		}

		printed := printToString(t, doc)

		reparsed, err := nquads.Parse(strings.NewReader(printed))
		if err != nil {
			t.Fatalf("Parse(Print(doc)) = %v, want nil; printed = %q", err, printed)
		}

		if !reflect.DeepEqual(withoutPositions(doc), withoutPositions(reparsed)) {
			t.Errorf("round trip changed the tree; src = %q, printed = %q", src, printed)
		}
	})
}
