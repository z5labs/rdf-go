package testsuite_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/rdf11/nquads"
	"github.com/z5labs/rdf-go/rdf11/ntriples"
	"github.com/z5labs/rdf-go/rdf11/trig"
	"github.com/z5labs/rdf-go/rdf11/turtle"
)

// pinnedCommit is the w3c/rdf-tests commit the vendored suites were taken
// from. TestVendoredSuitesRecordTheirCommit keeps it and PROVENANCE.md
// agreeing, so that neither can be updated while the other is forgotten.
const pinnedCommit = "767554e135eb6665949d870e6fa7bbc813837293"

const testdata = "testdata/rdf-tests"

// publishedAt is where the vendored suites are published, and so the base
// their manifests and documents resolve relative IRIs against. The suites say
// so themselves: the Turtle and TriG manifests declare it as
// mf:assumedTestBase, which TestManifestsAgreeWithTheirAssumedBase checks
// against this.
const publishedAt = "https://w3c.github.io/rdf-tests/rdf/rdf11/"

// skipped lists the tests this harness does not run, and why, keyed by
// "<suite>/<mf:name>".
//
// A test that cannot be run belongs here with a reason rather than being
// quietly passed over, and the manifest is what decides which tests there are,
// so a test left out of this map is a test that runs.
//
// The six below are all one disagreement. Each writes a character the IRIREF
// production excludes — a space, '<', '>' — as a UCHAR, and expects the
// document to be rejected: an escape is a way of spelling a character, not a
// way of getting one past the grammar. This module's tokenizers accept them,
// and cannot simply be taught otherwise, because [rdf.IRI]'s own N-Triples
// rendering writes exactly those characters as UCHARs (see escapeIRI in the
// root package) so that an IRI holding one can be written down at all. Making
// the tokenizers refuse what the printers emit would break that round trip,
// and deciding what a printer should do instead with an IRI that no syntax can
// write is a change to the printers' contract rather than to this harness.
//
// Nothing else in either suite is affected: only an IRI that is already
// outside the data model, RFC 3987 admitting none of these characters, can
// reach the escaping at all.
var skipped = map[string]string{
	"rdf-turtle/turtle-syntax-bad-uri-escape-01": escapedIRICharacter,
	"rdf-turtle/turtle-syntax-bad-uri-escape-02": escapedIRICharacter,
	"rdf-turtle/turtle-syntax-bad-uri-escape-03": escapedIRICharacter,
	"rdf-trig/trig-syntax-bad-uri-escape-01":     escapedIRICharacter,
	"rdf-trig/trig-syntax-bad-uri-escape-02":     escapedIRICharacter,
	"rdf-trig/trig-syntax-bad-uri-escape-03":     escapedIRICharacter,
}

const escapedIRICharacter = "the tokenizers accept a UCHAR naming a character IRIREF excludes, " +
	"because the printers write one to render an IRI that holds it"

// The RDF test vocabulary, whose classes say what a test is and what a
// conforming parser must do with it (https://www.w3.org/ns/rdftest#).
const namespaceRDFT = "http://www.w3.org/ns/rdftest#"

// kind is what a test asks of the parser.
type kind int

const (
	// positiveSyntax: the action must be read without error.
	positiveSyntax kind = iota

	// negativeSyntax: the action must be rejected.
	negativeSyntax

	// eval: the action must be read, and must mean the same as the result.
	eval

	// negativeEval: the action is within the grammar but does not denote an
	// RDF dataset — an IRI that is not an IRI, say — so reading it must still
	// fail.
	negativeEval
)

// format is a concrete syntax: how to turn a document into the quads it
// states.
type format struct {
	// name is the syntax's name, for error messages.
	name string

	// decode reads a document and returns the dataset it states. Relative
	// IRIs resolve against base, the IRI the document is published under.
	//
	// A syntax with no graph names puts everything in the default graph,
	// which is what a graph is: a dataset with one unnamed graph.
	decode func(r io.Reader, base string) (*rdf.Dataset, error)
}

var (
	ntriplesFormat = format{name: "N-Triples", decode: decodeNTriples}
	nquadsFormat   = format{name: "N-Quads", decode: decodeNQuads}
	turtleFormat   = format{name: "Turtle", decode: decodeTurtle}
	trigFormat     = format{name: "TriG", decode: decodeTriG}
)

// testType is what one rdft: class asks for.
type testType struct {
	kind kind

	// action is the syntax the document under test is written in.
	action format

	// result is the syntax the expected result is written in, used only by an
	// evaluation test. The W3C suites state a graph's result in N-Triples and
	// a dataset's in N-Quads.
	result format
}

// testTypes dispatches on the rdf:type a manifest gives a test.
//
// A type absent from this map is one the harness cannot run, and finding one
// fails the suite rather than passing over it — the suites grow, and a test
// that is never run is worse than one that fails.
var testTypes = map[rdf.IRI]testType{
	rdft("TestNTriplesPositiveSyntax"): {kind: positiveSyntax, action: ntriplesFormat},
	rdft("TestNTriplesNegativeSyntax"): {kind: negativeSyntax, action: ntriplesFormat},
	rdft("TestNTriplesEval"):           {kind: eval, action: ntriplesFormat, result: ntriplesFormat},
	rdft("TestNTriplesNegativeEval"):   {kind: negativeEval, action: ntriplesFormat},

	rdft("TestNQuadsPositiveSyntax"): {kind: positiveSyntax, action: nquadsFormat},
	rdft("TestNQuadsNegativeSyntax"): {kind: negativeSyntax, action: nquadsFormat},
	rdft("TestNQuadsEval"):           {kind: eval, action: nquadsFormat, result: nquadsFormat},
	rdft("TestNQuadsNegativeEval"):   {kind: negativeEval, action: nquadsFormat},

	rdft("TestTurtlePositiveSyntax"): {kind: positiveSyntax, action: turtleFormat},
	rdft("TestTurtleNegativeSyntax"): {kind: negativeSyntax, action: turtleFormat},
	rdft("TestTurtleEval"):           {kind: eval, action: turtleFormat, result: ntriplesFormat},
	rdft("TestTurtleNegativeEval"):   {kind: negativeEval, action: turtleFormat},

	// TriG's classes spell the syntax "Trig", as the vocabulary does.
	rdft("TestTrigPositiveSyntax"): {kind: positiveSyntax, action: trigFormat},
	rdft("TestTrigNegativeSyntax"): {kind: negativeSyntax, action: trigFormat},
	rdft("TestTrigEval"):           {kind: eval, action: trigFormat, result: nquadsFormat},
	rdft("TestTrigNegativeEval"):   {kind: negativeEval, action: trigFormat},
}

func rdft(class string) rdf.IRI { return namespaceRDFT + rdf.IRI(class) }

// suites are the vendored W3C suites, named by the directory each sits in.
var suites = []string{"rdf-n-triples", "rdf-n-quads", "rdf-turtle", "rdf-trig"}

// TestConformance runs every vendored suite from its manifest, one Go subtest
// per manifest entry, named as the manifest names the test so that a failure
// can be looked up directly in the W3C suite.
func TestConformance(t *testing.T) {
	for _, suite := range suites {
		t.Run(suite, func(t *testing.T) {
			m := loadManifest(t, suite)

			counts := make(map[kind]int)
			var skips int

			for _, entry := range m.entries {
				if reason, ok := skipped[suite+"/"+entry.name]; ok {
					skips++
					t.Run(entry.name, func(t *testing.T) {
						t.Skipf("skipped: %s", reason)
					})
					continue
				}

				typ, ok := testTypes[entry.typ]
				if !ok {
					t.Errorf("%s: the manifest declares %s, which this harness cannot run; "+
						"teach it the type or list the test in skipped with a reason", entry.name, entry.typ)
					continue
				}
				counts[typ.kind]++

				t.Run(entry.name, func(t *testing.T) {
					runEntry(t, suite, typ, entry)
				})
			}

			t.Logf("%s: %d positive syntax, %d negative syntax, %d eval, %d negative eval, %d skipped, %d total",
				suite, counts[positiveSyntax], counts[negativeSyntax], counts[eval], counts[negativeEval],
				skips, len(m.entries))
		})
	}
}

// runEntry runs one manifest entry.
func runEntry(t *testing.T, suite string, typ testType, entry manifestEntry) {
	if entry.comment != "" {
		t.Log(entry.comment)
	}

	action, base := localFile(t, suite, entry.action)

	switch typ.kind {
	case positiveSyntax:
		if _, err := decodeFile(action, base, typ.action); err != nil {
			t.Errorf("reading as %s failed, want a positive syntax test to be accepted: %v", typ.action.name, err)
		}

	case negativeSyntax, negativeEval:
		if _, err := decodeFile(action, base, typ.action); err == nil {
			t.Errorf("read as %s without error, want it to be rejected", typ.action.name)
		}

	case eval:
		if entry.result == "" {
			t.Fatal("the manifest gives no mf:result for an evaluation test")
		}
		resultFile, resultBase := localFile(t, suite, entry.result)

		got, err := decodeFile(action, base, typ.action)
		if err != nil {
			t.Fatalf("reading as %s failed: %v", typ.action.name, err)
		}
		want, err := decodeFile(resultFile, resultBase, typ.result)
		if err != nil {
			t.Fatalf("reading the expected result as %s failed: %v", typ.result.name, err)
		}

		if !isomorphic(got, want) {
			t.Errorf("the document does not state the expected dataset\ngot:\n%s\nwant:\n%s",
				render(got), render(want))
		}

	default:
		t.Fatalf("the harness has no way to run a %s test, though it dispatches one", entry.typ)
	}
}

// localFile maps the IRI a manifest names a document by to the vendored copy
// of it, and returns that path along with the IRI itself, which is the base
// the document's own relative IRIs resolve against.
func localFile(t *testing.T, suite string, iri rdf.IRI) (path, base string) {
	t.Helper()

	prefix := publishedAt + suite + "/"
	name, found := strings.CutPrefix(string(iri), prefix)
	if !found {
		t.Fatalf("the manifest names %s, which is not published under %s, so nothing vendored is it", iri, prefix)
	}
	if strings.ContainsRune(name, '/') {
		t.Fatalf("the manifest names %s, which is not a file of the suite", iri)
	}
	return filepath.Join(testdata, "rdf", "rdf11", suite, name), string(iri)
}

func decodeFile(path, base string, f format) (*rdf.Dataset, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return f.decode(file, base)
}

func decodeNTriples(r io.Reader, _ string) (*rdf.Dataset, error) {
	d := rdf.NewDataset()
	for triple, err := range ntriples.Decode(r) {
		if err != nil {
			return nil, err
		}
		if err := d.Add(rdf.Quad{Triple: triple}); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func decodeNQuads(r io.Reader, _ string) (*rdf.Dataset, error) {
	d := rdf.NewDataset()
	for quad, err := range nquads.Decode(r) {
		if err != nil {
			return nil, err
		}
		if err := d.Add(quad); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func decodeTurtle(r io.Reader, base string) (*rdf.Dataset, error) {
	d := rdf.NewDataset()
	for triple, err := range turtle.Decode(withBase(r, base)) {
		if err != nil {
			return nil, err
		}
		if err := d.Add(rdf.Quad{Triple: triple}); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func decodeTriG(r io.Reader, base string) (*rdf.Dataset, error) {
	d := rdf.NewDataset()
	for quad, err := range trig.Decode(withBase(r, base)) {
		if err != nil {
			return nil, err
		}
		if err := d.Add(quad); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// isomorphic reports whether two datasets state the same thing, which for one
// holding blank nodes means the same up to a renaming of them (RDF 1.1
// Concepts §3.5, and §4.3 for datasets).
//
// [rdf.Isomorphic] answers this for a graph, and a dataset reduces to one:
// each quad becomes a node carrying its four positions as properties, so a
// renaming of blank nodes taking one encoding to the other is exactly one
// taking the dataset to the other. The encoding's own nodes are the only blank
// nodes in it with outgoing edges, so no isomorphism can confuse one with a
// blank node of the dataset.
func isomorphic(a, b *rdf.Dataset) bool {
	return rdf.Isomorphic(encodeDataset(a), encodeDataset(b))
}

// The properties the quad encoding uses, in a namespace of this harness's own
// so that nothing a test states can collide with them.
const (
	encodedSubject   rdf.IRI = "urn:rdf-go:testsuite:subject"
	encodedPredicate rdf.IRI = "urn:rdf-go:testsuite:predicate"
	encodedObject    rdf.IRI = "urn:rdf-go:testsuite:object"
	encodedGraph     rdf.IRI = "urn:rdf-go:testsuite:graph"

	// encodedDefaultGraph stands for the default graph, which has no name of
	// its own to encode.
	encodedDefaultGraph rdf.IRI = "urn:rdf-go:testsuite:defaultGraph"
)

func encodeDataset(d *rdf.Dataset) *rdf.Graph {
	g := rdf.NewGraph()

	var i int
	for quad := range d.All() {
		cell := rdf.NewBlankNode(fmt.Sprintf("q%d", i))
		i++

		name := quad.Graph
		if name == nil {
			name = encodedDefaultGraph
		}

		for _, t := range []rdf.Triple{
			{Subject: cell, Predicate: encodedSubject, Object: quad.Subject},
			{Subject: cell, Predicate: encodedPredicate, Object: quad.Predicate},
			{Subject: cell, Predicate: encodedObject, Object: quad.Object},
			{Subject: cell, Predicate: encodedGraph, Object: name},
		} {
			if err := g.Add(t); err != nil {
				panic(fmt.Sprintf("encoding %s: %v", quad, err))
			}
		}
	}
	return g
}

// render writes a dataset as sorted N-Quads, so that two of them can be read
// against each other when a test fails.
func render(d *rdf.Dataset) string {
	var lines []string
	for quad := range d.All() {
		lines = append(lines, "\t"+quad.String())
	}
	slices.Sort(lines)
	return strings.Join(lines, "\n")
}

// loadManifest reads a suite's manifest, which every test of that suite comes
// from.
func loadManifest(t *testing.T, suite string) *manifest {
	t.Helper()

	base := publishedAt + suite + "/manifest.ttl"
	path := filepath.Join(testdata, "rdf", "rdf11", suite, "manifest.ttl")

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer file.Close()

	m, err := readManifest(file, base)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	if len(m.entries) == 0 {
		t.Fatalf("%s lists no tests", path)
	}
	return m
}

// TestManifestsAgreeWithTheirAssumedBase checks the IRI this harness resolves
// a suite's documents against against the one the suite itself says to use.
//
// The mapping from a manifest's IRIs back to vendored files rests on it: get
// the base wrong and every mf:action names a document that is nowhere, or —
// worse — a relative IRI in an evaluation test resolves to something other
// than what the test expects, and the comparison quietly tests something else.
func TestManifestsAgreeWithTheirAssumedBase(t *testing.T) {
	for _, suite := range suites {
		t.Run(suite, func(t *testing.T) {
			m := loadManifest(t, suite)

			if want := rdf.IRI(publishedAt + suite + "/manifest.ttl"); m.iri != want {
				t.Errorf("the manifest describes %s, want %s", m.iri, want)
			}
			if m.assumedTestBase == "" {
				t.Skip("the manifest declares no mf:assumedTestBase")
			}
			if want := rdf.IRI(publishedAt + suite + "/"); m.assumedTestBase != want {
				t.Errorf("mf:assumedTestBase is %s, want %s", m.assumedTestBase, want)
			}
		})
	}
}

// unreferenced lists the files a suite ships that its own manifest names no
// test for, keyed by "<suite>/<file>".
//
// These are not skips: no test was left unrun, because the manifest declares
// none. They are files the W3C suite carries which nothing in it refers to,
// and they are enumerated for the same reason a skip is — so that a file
// arriving unreferenced is noticed rather than assumed to be one of these.
var unreferenced = map[string]string{
	// Positive syntax tests are checked by parsing alone, so the expected
	// result vendored beside each of these is never read.
	"rdf-turtle/turtle-syntax-blank-label.nt": "result of a positive syntax test, which has no result to compare",
	"rdf-turtle/turtle-syntax-ln-colons.nt":   "result of a positive syntax test, which has no result to compare",
	"rdf-turtle/turtle-syntax-ln-dots.nt":     "result of a positive syntax test, which has no result to compare",

	// The manifest mentions none of these at all, under any name.
	"rdf-n-triples/literal_true.nt":                                    "the manifest declares no test for it",
	"rdf-n-triples/literal_false.nt":                                   "the manifest declares no test for it",
	"rdf-n-quads/literal_true.nq":                                      "the manifest declares no test for it",
	"rdf-n-quads/literal_false.nq":                                     "the manifest declares no test for it",
	"rdf-turtle/test-38.ttl":                                           "the manifest declares no test for it",
	"rdf-turtle/test-38.nt":                                            "the manifest declares no test for it",
	"rdf-turtle/turtle-syntax-pname-dots.ttl":                          "the manifest declares no test for it",
	"rdf-turtle/localName_with_PN_CHARS_BASE_character_boundaries.ttl": "the manifest declares no test for it",
	"rdf-turtle/localName_with_PN_CHARS_BASE_character_boundaries.nt":  "the manifest declares no test for it",
	"rdf-trig/localName_with_PN_CHARS_BASE_character_boundaries.trig":  "the manifest declares no test for it",
	"rdf-trig/localName_with_PN_CHARS_BASE_character_boundaries.nq":    "the manifest declares no test for it",
}

// TestManifestsAccountForEveryVendoredFile fails if a suite holds a file no
// test names and [unreferenced] does not account for.
//
// The harness this replaced walked the directory, so an unrecognised file was
// in front of it. This one walks the manifest and would never see one: a test
// file left out of the manifest, or vendored under a name nothing refers to,
// would simply never run. This is what notices.
func TestManifestsAccountForEveryVendoredFile(t *testing.T) {
	// What describes the suite rather than being part of it.
	describes := []string{"manifest.ttl", "README", "LICENSE"}

	for _, suite := range suites {
		t.Run(suite, func(t *testing.T) {
			m := loadManifest(t, suite)

			named := make(map[string]bool)
			for _, entry := range m.entries {
				for _, iri := range []rdf.IRI{entry.action, entry.result} {
					if iri == "" {
						continue
					}
					path, _ := localFile(t, suite, iri)
					named[filepath.Base(path)] = true
				}
			}

			dir := filepath.Join(testdata, "rdf", "rdf11", suite)
			files, err := os.ReadDir(dir)
			if err != nil {
				t.Fatalf("reading %s: %v", dir, err)
			}
			for _, file := range files {
				name := file.Name()
				if named[name] || slices.Contains(describes, name) {
					continue
				}
				if reason, ok := unreferenced[suite+"/"+name]; ok {
					t.Logf("%s is vendored but unreferenced: %s", name, reason)
					continue
				}
				t.Errorf("%s is vendored but no test names it; add the test that uses it, "+
					"list it in unreferenced with a reason, or stop vendoring it", name)
			}
		})
	}
}

// TestSkippedTestsExist keeps [skipped] honest: an entry naming a test no
// manifest declares is a skip that stopped being one, and it is removed rather
// than left standing as an excuse nothing needs.
func TestSkippedTestsExist(t *testing.T) {
	declared := make(map[string]bool)
	for _, suite := range suites {
		for _, entry := range loadManifest(t, suite).entries {
			declared[suite+"/"+entry.name] = true
		}
	}

	for key := range skipped {
		if !declared[key] {
			t.Errorf("%s is skipped but no manifest declares it", key)
		}
	}
}

// TestUnreferencedFilesAreStillThere keeps [unreferenced] honest in the other
// direction: an entry naming a file that is no longer vendored, or that a test
// has since been written for, is a stale excuse and is removed rather than
// left to cover for the next file that goes unrun.
func TestUnreferencedFilesAreStillThere(t *testing.T) {
	for key := range unreferenced {
		suite, name, _ := strings.Cut(key, "/")
		if !slices.Contains(suites, suite) {
			t.Errorf("%s names %s, which is not a vendored suite", key, suite)
			continue
		}
		if _, err := os.Stat(filepath.Join(testdata, "rdf", "rdf11", suite, name)); err != nil {
			t.Errorf("%s is listed as unreferenced but is not vendored: %v", key, err)
		}
	}
}

// TestVendoredSuitesRecordTheirCommit keeps the pinned SHA in one place in
// effect if not in fact, so that re-vendoring at a new commit cannot leave the
// recorded one behind.
func TestVendoredSuitesRecordTheirCommit(t *testing.T) {
	path := filepath.Join(testdata, "PROVENANCE.md")

	provenance, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	if !strings.Contains(string(provenance), pinnedCommit) {
		t.Errorf("%s does not record the commit %s that this harness is pinned to", path, pinnedCommit)
	}
}
