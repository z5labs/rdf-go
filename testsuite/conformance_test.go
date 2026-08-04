package testsuite_test

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"text/tabwriter"

	rdf "github.com/z5labs/rdf-go"
	nquads11 "github.com/z5labs/rdf-go/rdf11/nquads"
	ntriples11 "github.com/z5labs/rdf-go/rdf11/ntriples"
	trig11 "github.com/z5labs/rdf-go/rdf11/trig"
	turtle11 "github.com/z5labs/rdf-go/rdf11/turtle"
	nquads12 "github.com/z5labs/rdf-go/rdf12/nquads"
	ntriples12 "github.com/z5labs/rdf-go/rdf12/ntriples"
	trig12 "github.com/z5labs/rdf-go/rdf12/trig"
	turtle12 "github.com/z5labs/rdf-go/rdf12/turtle"
)

// pinnedCommit is the w3c/rdf-tests commit the vendored suites were taken
// from. TestVendoredSuitesRecordTheirCommit keeps it and PROVENANCE.md
// agreeing, so that neither can be updated while the other is forgotten.
const pinnedCommit = "767554e135eb6665949d870e6fa7bbc813837293"

const testdata = "testdata/rdf-tests"

// publishedAt is where the vendored suites are published, and so the base
// their manifests and documents resolve relative IRIs against. Every suite
// sits in a directory under it, and every document a manifest names is a file
// in one of those directories, which is what lets an IRI be turned back into
// the vendored copy of what it names — see vendoredPath.
//
// The suites say where they are themselves: each manifest declares its own
// directory as mf:assumedTestBase, which
// TestManifestsAgreeWithTheirAssumedBase checks against this.
const publishedAt = "https://w3c.github.io/rdf-tests/rdf/"

// skipped lists the tests this harness does not run, and why, keyed as
// [testKey] keys a test.
//
// A test that cannot be run belongs here with a reason rather than being
// quietly passed over, and the manifest is what decides which tests there are,
// so a test left out of this map is a test that runs.
//
// The two mintedBlankNodeLabels entries are the one place a canonicalization
// test asks for something the RDF 1.2 encoders do not promise. Canonical form
// settles the writing and says nothing about which labels the blank nodes
// carry, and [ntriples12.Encode] says the same: it writes a blank node with
// the label it has, and the labels a document's blank nodes have after
// decoding are minted rather than the ones it wrote. Comparing the two as text
// therefore compares labels neither specification fixes.
var skipped = map[string]string{
	"rdf12/rdf-n-triples/c14n#triple-term-02": mintedBlankNodeLabels,
	"rdf12/rdf-n-quads/c14n#triple-term-02":   mintedBlankNodeLabels,
}

const mintedBlankNodeLabels = "the document names a blank node, and canonical form does not fix " +
	"blank node labels: Decode mints them, so the text differs where nothing about the graph does"

// testKey names a test by its own IRI, with the IRI the suites are published
// under taken off the front — "rdf12/rdf-n-triples/syntax#ntriples12-01".
//
// It is the test's IRI and not its mf:name because a name is not unique: the
// vendored suites hold three pairs of tests that share one, both RDF 1.1
// suites calling two of their own tests "…-syntax-bad-num-05" and the RDF 1.2
// N-Quads canonicalization suite calling two of its own "C14N triple-term-03".
// A key naming one of those would silently mean the other as well. An entry's
// IRI is the node the manifest describes it as, so no two tests can share one.
//
// A Go subtest is still named by mf:name, which is what the W3C reports call a
// test and so what a failure is looked up under. Where two share one, Go tells
// them apart with a "#01" suffix that says nothing about which is which — so
// every subtest logs its key, and a failing or skipped test names itself
// unambiguously without the other thousand-odd having longer names for it.
func testKey(entry manifestEntry) string {
	return strings.TrimPrefix(string(entry.iri), publishedAt)
}

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

	// canonical: the action must be read, and written back in the canonical
	// form of its syntax as the same bytes as the result. Unlike an evaluation
	// test this is a comparison of text, so it checks the writing and not only
	// the meaning (RDF 1.2 N-Triples §3).
	canonical
)

// expectsTheDocumentRead reports whether a test of this kind requires its
// mf:action to be read without error, which is what makes the document a
// document of the syntax the suite is for and so what
// TestRDF11ParsersRefuseRDF12Syntax has something to say about.
//
// The three kinds that do are the ones that go on to check something about
// what was read; the two negative kinds require the opposite, and a document
// an RDF 1.2 parser must refuse says nothing about which version wrote it.
func expectsTheDocumentRead(k kind) bool {
	switch k {
	case positiveSyntax, eval, canonical:
		return true
	default:
		return false
	}
}

// syntax names a concrete syntax. Which parser reads one depends on the
// specification version the suite is for, so a test says only which syntax its
// documents are written in and the suite's [spec] supplies the parser.
type syntax int

const (
	nTriples syntax = iota
	nQuads
	turtle
	triG
)

func (s syntax) String() string {
	switch s {
	case nTriples:
		return "N-Triples"
	case nQuads:
		return "N-Quads"
	case turtle:
		return "Turtle"
	case triG:
		return "TriG"
	default:
		return fmt.Sprintf("syntax(%d)", int(s))
	}
}

// format is one syntax as one specification version defines it: how to turn a
// document into the quads it states, and where the version settles it, how to
// write those quads back canonically.
type format struct {
	// name is the parser's name, for error messages.
	name string

	// decode reads a document and returns the dataset it states. Relative
	// IRIs resolve against base, the IRI the document is published under.
	//
	// A syntax with no graph names puts everything in the default graph,
	// which is what a graph is: a dataset with one unnamed graph.
	decode func(r io.Reader, base string) (*rdf.Dataset, error)

	// canonicalize reads a document and writes what it states back in the
	// canonical form of this syntax. It is nil where the specification
	// version defines no canonical form, which is every RDF 1.1 syntax:
	// canonical N-Triples and N-Quads arrive with RDF 1.2.
	canonicalize func(r io.Reader) ([]byte, error)
}

// spec is one version of the RDF specifications, and the parsers this module
// provides for it. A suite runs against exactly one of them, and that a
// document of the other version is read — or refused — by these parsers is
// what the versioned package split is for: see
// TestRDF11ParsersRefuseRDF12Syntax.
type spec struct {
	// name is the version's name, for error messages.
	name string

	// formats are the syntaxes this version has a parser for here.
	formats map[syntax]format
}

var rdf11 = &spec{
	name: "RDF 1.1",
	formats: map[syntax]format{
		nTriples: {name: "RDF 1.1 N-Triples", decode: decodeNTriples11},
		nQuads:   {name: "RDF 1.1 N-Quads", decode: decodeNQuads11},
		turtle:   {name: "RDF 1.1 Turtle", decode: decodeTurtle11},
		triG:     {name: "RDF 1.1 TriG", decode: decodeTriG11},
	},
}

var rdf12 = &spec{
	name: "RDF 1.2",
	formats: map[syntax]format{
		nTriples: {name: "RDF 1.2 N-Triples", decode: decodeNTriples12, canonicalize: canonicalNTriples12},
		nQuads:   {name: "RDF 1.2 N-Quads", decode: decodeNQuads12, canonicalize: canonicalNQuads12},
		turtle:   {name: "RDF 1.2 Turtle", decode: decodeTurtle12},
		triG:     {name: "RDF 1.2 TriG", decode: decodeTriG12},
	},
}

// testType is what one rdft: class asks for.
type testType struct {
	kind kind

	// action is the syntax the document under test is written in.
	action syntax

	// result is the syntax the expected result is written in, used only by an
	// evaluation or canonicalization test. The W3C suites state a graph's
	// result in N-Triples and a dataset's in N-Quads.
	result syntax
}

// testTypes dispatches on the rdf:type a manifest gives a test.
//
// A type absent from this map is one the harness cannot run, and finding one
// fails the suite rather than passing over it — the suites grow, and a test
// that is never run is worse than one that fails.
var testTypes = map[rdf.IRI]testType{
	rdft("TestNTriplesPositiveSyntax"): {kind: positiveSyntax, action: nTriples},
	rdft("TestNTriplesNegativeSyntax"): {kind: negativeSyntax, action: nTriples},
	rdft("TestNTriplesEval"):           {kind: eval, action: nTriples, result: nTriples},
	rdft("TestNTriplesNegativeEval"):   {kind: negativeEval, action: nTriples},
	rdft("TestNTriplesPositiveC14N"):   {kind: canonical, action: nTriples, result: nTriples},

	rdft("TestNQuadsPositiveSyntax"): {kind: positiveSyntax, action: nQuads},
	rdft("TestNQuadsNegativeSyntax"): {kind: negativeSyntax, action: nQuads},
	rdft("TestNQuadsEval"):           {kind: eval, action: nQuads, result: nQuads},
	rdft("TestNQuadsNegativeEval"):   {kind: negativeEval, action: nQuads},
	rdft("TestNQuadsPositiveC14N"):   {kind: canonical, action: nQuads, result: nQuads},

	rdft("TestTurtlePositiveSyntax"): {kind: positiveSyntax, action: turtle},
	rdft("TestTurtleNegativeSyntax"): {kind: negativeSyntax, action: turtle},
	rdft("TestTurtleEval"):           {kind: eval, action: turtle, result: nTriples},
	rdft("TestTurtleNegativeEval"):   {kind: negativeEval, action: turtle},

	// TriG's classes spell the syntax "Trig", as the vocabulary does.
	rdft("TestTrigPositiveSyntax"): {kind: positiveSyntax, action: triG},
	rdft("TestTrigNegativeSyntax"): {kind: negativeSyntax, action: triG},
	rdft("TestTrigEval"):           {kind: eval, action: triG, result: nQuads},
	rdft("TestTrigNegativeEval"):   {kind: negativeEval, action: triG},
}

func rdft(class string) rdf.IRI { return namespaceRDFT + rdf.IRI(class) }

// suite is one vendored W3C suite: the directory it sits in under rdf/, which
// names it, and the version of the specifications whose parsers run it.
type suite struct {
	dir string

	// pkg is the package of this module the suite is a suite for, relative to
	// the module root. It is what the conformance summary reports a row of the
	// table under, there being one suite for each of the eight.
	//
	// A suite exercises more than its own package — an evaluation test's
	// expected result is read by the package for the result's syntax — but it
	// is the parser for the action's syntax that a failure is about, and the
	// suite is published as that parser's suite.
	pkg string

	spec *spec
}

// manifestIRI is the IRI of the manifest the suite describes itself in, which
// is where running it starts.
func (s suite) manifestIRI() rdf.IRI { return rdf.IRI(publishedAt + s.dir + "/manifest.ttl") }

// declares reports whether iri names a document of this suite rather than one
// of a suite it includes. The RDF 1.2 manifests include their RDF 1.1
// counterparts, so a suite's tests are not all its own.
func (s suite) declares(iri rdf.IRI) bool {
	return strings.HasPrefix(string(iri), publishedAt+s.dir+"/")
}

// suites are the vendored W3C suites, each run by the parsers of the
// specification version it is for.
//
// There is one for each of the eight format packages this module provides, so
// running them is running the whole of it.
var suites = []suite{
	{dir: "rdf11/rdf-n-triples", pkg: "rdf11/ntriples", spec: rdf11},
	{dir: "rdf11/rdf-n-quads", pkg: "rdf11/nquads", spec: rdf11},
	{dir: "rdf11/rdf-turtle", pkg: "rdf11/turtle", spec: rdf11},
	{dir: "rdf11/rdf-trig", pkg: "rdf11/trig", spec: rdf11},
	{dir: "rdf12/rdf-n-triples", pkg: "rdf12/ntriples", spec: rdf12},
	{dir: "rdf12/rdf-n-quads", pkg: "rdf12/nquads", spec: rdf12},
	{dir: "rdf12/rdf-turtle", pkg: "rdf12/turtle", spec: rdf12},
	{dir: "rdf12/rdf-trig", pkg: "rdf12/trig", spec: rdf12},
}

// result is how one suite did: how many of its tests are of each kind, and how
// many passed, failed or were skipped.
type result struct {
	suite   suite
	counts  map[kind]int
	passed  int
	failed  int
	skipped int
	total   int
}

// TestConformance runs every vendored suite from its manifest, one Go subtest
// per manifest entry, named as the manifest names the test so that a failure
// can be looked up directly in the W3C suite.
//
// It finishes by logging what every suite did as one table, which is the
// conformance report for the module: there is one suite per format package, so
// the eight rows are the whole of what this module claims to read. A run with
// -v prints it whether or not anything failed.
func TestConformance(t *testing.T) {
	results := make([]result, 0, len(suites))

	for _, s := range suites {
		r := result{suite: s, counts: make(map[kind]int)}

		t.Run(s.dir, func(t *testing.T) {
			entries := entriesOf(loadSuite(t, s))
			r.total = len(entries)

			for _, entry := range entries {
				if reason, ok := skipped[testKey(entry)]; ok {
					r.skipped++
					t.Run(entry.name, func(t *testing.T) {
						t.Skipf("%s skipped: %s", testKey(entry), reason)
					})
					continue
				}

				typ, ok := testTypes[entry.typ]
				if !ok {
					r.failed++
					t.Errorf("%s: the manifest declares %s, which this harness cannot run; "+
						"teach it the type or list the test in skipped with a reason", entry.name, entry.typ)
					continue
				}
				r.counts[typ.kind]++

				if t.Run(entry.name, func(t *testing.T) {
					runEntry(t, s, typ, entry)
				}) {
					r.passed++
				} else {
					r.failed++
				}
			}
		})

		results = append(results, r)
	}

	t.Log("conformance, one row per format package:\n" + report(results))
}

// report renders the conformance numbers as a table, a row per format package
// and a row totalling them.
//
// The counts by kind are there because the totals alone do not say what was
// checked: a suite of syntax tests says a parser accepts and rejects the right
// documents, and only an evaluation or canonicalization test says the parser
// agrees about what a document means or how it is written.
func report(results []result) string {
	var b strings.Builder

	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "package\tsuite\tpass\tfail\tskip\ttotal\tsyntax+\tsyntax-\teval\teval-\tc14n")

	var all result
	all.counts = make(map[kind]int)

	for _, r := range results {
		writeReportRow(w, r.suite.pkg, r.suite.dir, r)

		all.passed += r.passed
		all.failed += r.failed
		all.skipped += r.skipped
		all.total += r.total
		for k, n := range r.counts {
			all.counts[k] += n
		}
	}
	writeReportRow(w, "all", "", all)

	if err := w.Flush(); err != nil {
		// A [strings.Builder] never fails to be written to, so this cannot
		// happen; reporting it beats returning a table with a hole in it.
		return fmt.Sprintf("rendering the conformance table: %v", err)
	}
	return b.String()
}

func writeReportRow(w io.Writer, pkg, dir string, r result) {
	fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n",
		pkg, dir, r.passed, r.failed, r.skipped, r.total,
		r.counts[positiveSyntax], r.counts[negativeSyntax],
		r.counts[eval], r.counts[negativeEval], r.counts[canonical])
}

// runEntry runs one manifest entry against the parsers of the suite's
// specification version.
func runEntry(t *testing.T, s suite, typ testType, entry manifestEntry) {
	t.Log(testKey(entry))
	if entry.comment != "" {
		t.Log(entry.comment)
	}

	f := formatOf(t, s.spec, typ.action)
	action, base := localFile(t, entry.action)

	switch typ.kind {
	case positiveSyntax:
		if _, err := decodeFile(action, base, f); err != nil {
			t.Errorf("reading as %s failed, want a positive syntax test to be accepted: %v", f.name, err)
		}

	case negativeSyntax, negativeEval:
		if _, err := decodeFile(action, base, f); err == nil {
			t.Errorf("read as %s without error, want it to be rejected", f.name)
		}

	case eval:
		if entry.result == "" {
			t.Fatal("the manifest gives no mf:result for an evaluation test")
		}
		resultFile, resultBase := localFile(t, entry.result)
		resultFormat := formatOf(t, s.spec, typ.result)

		got, err := decodeFile(action, base, f)
		if err != nil {
			t.Fatalf("reading as %s failed: %v", f.name, err)
		}
		want, err := decodeFile(resultFile, resultBase, resultFormat)
		if err != nil {
			t.Fatalf("reading the expected result as %s failed: %v", resultFormat.name, err)
		}

		if !isomorphic(got, want) {
			t.Errorf("the document does not state the expected dataset\ngot:\n%s\nwant:\n%s",
				render(got), render(want))
		}

	case canonical:
		if entry.result == "" {
			t.Fatal("the manifest gives no mf:result for a canonicalization test")
		}
		if f.canonicalize == nil {
			t.Fatalf("%s defines no canonical %s, so this test cannot be run", s.spec.name, typ.action)
		}
		resultFile, _ := localFile(t, entry.result)

		got, err := canonicalizeFile(action, f)
		if err != nil {
			t.Fatalf("reading as %s failed: %v", f.name, err)
		}
		want, err := os.ReadFile(resultFile)
		if err != nil {
			t.Fatalf("reading the expected result: %v", err)
		}

		if !bytes.Equal(got, want) {
			t.Errorf("the document does not write back in canonical form\ngot:\n%s\nwant:\n%s", got, want)
		}

	default:
		t.Fatalf("the harness has no way to run a %s test, though it dispatches one", entry.typ)
	}
}

// formatOf returns the parser a specification version provides for a syntax.
//
// A suite naming a syntax its own version has no parser for here is a gap in
// this module rather than in the suite, and is reported as one.
func formatOf(t *testing.T, s *spec, syn syntax) format {
	t.Helper()

	f, ok := s.formats[syn]
	if !ok {
		t.Fatalf("this module has no %s %s parser", s.name, syn)
	}
	return f
}

// localFile maps the IRI a manifest names a document by to the vendored copy
// of it, and returns that path along with the IRI itself, which is the base
// the document's own relative IRIs resolve against.
func localFile(t *testing.T, iri rdf.IRI) (path, base string) {
	t.Helper()

	p, err := vendoredPath(iri)
	if err != nil {
		t.Fatal(err)
	}
	return p, string(iri)
}

// vendoredPath is the file in testdata that iri names, or an error saying why
// nothing vendored is it.
//
// A manifest names its documents by IRI, and the suites are vendored as the
// directory tree those IRIs describe, so the mapping is the prefix removed and
// the rest read as a path. Both halves are checked: an IRI published somewhere
// else names nothing here, and one whose path climbs out of the tree names
// something that is not part of the suites at all.
func vendoredPath(iri rdf.IRI) (string, error) {
	rest, found := strings.CutPrefix(string(iri), publishedAt)
	if !found {
		return "", fmt.Errorf("the manifest names %s, which is not published under %s, so nothing vendored is it", iri, publishedAt)
	}
	if rest == "" || strings.HasSuffix(rest, "/") {
		return "", fmt.Errorf("the manifest names %s, which is a directory rather than a document", iri)
	}
	if path.Clean(rest) != rest {
		return "", fmt.Errorf("the manifest names %s, which does not name a file under %s", iri, publishedAt)
	}
	return filepath.Join(testdata, "rdf", filepath.FromSlash(rest)), nil
}

func decodeFile(path, base string, f format) (*rdf.Dataset, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return f.decode(file, base)
}

// canonicalizeFile reads a document and returns it written back in the
// canonical form of its syntax.
func canonicalizeFile(path string, f format) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return f.canonicalize(file)
}

func decodeNTriples11(r io.Reader, _ string) (*rdf.Dataset, error) {
	d := rdf.NewDataset()
	for triple, err := range ntriples11.Decode(r) {
		if err != nil {
			return nil, err
		}
		if err := d.Add(rdf.Quad{Triple: triple}); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func decodeNQuads11(r io.Reader, _ string) (*rdf.Dataset, error) {
	d := rdf.NewDataset()
	for quad, err := range nquads11.Decode(r) {
		if err != nil {
			return nil, err
		}
		if err := d.Add(quad); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func decodeTurtle11(r io.Reader, base string) (*rdf.Dataset, error) {
	d := rdf.NewDataset()
	for triple, err := range turtle11.Decode(withBase(r, base)) {
		if err != nil {
			return nil, err
		}
		if err := d.Add(rdf.Quad{Triple: triple}); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func decodeTriG11(r io.Reader, base string) (*rdf.Dataset, error) {
	d := rdf.NewDataset()
	for quad, err := range trig11.Decode(withBase(r, base)) {
		if err != nil {
			return nil, err
		}
		if err := d.Add(quad); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func decodeNTriples12(r io.Reader, _ string) (*rdf.Dataset, error) {
	d := rdf.NewDataset()
	for triple, err := range ntriples12.Decode(r) {
		if err != nil {
			return nil, err
		}
		if err := d.Add(rdf.Quad{Triple: triple}); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func decodeNQuads12(r io.Reader, _ string) (*rdf.Dataset, error) {
	d := rdf.NewDataset()
	for quad, err := range nquads12.Decode(r) {
		if err != nil {
			return nil, err
		}
		if err := d.Add(quad); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func decodeTurtle12(r io.Reader, base string) (*rdf.Dataset, error) {
	d := rdf.NewDataset()
	for triple, err := range turtle12.Decode(withBase(r, base)) {
		if err != nil {
			return nil, err
		}
		if err := d.Add(rdf.Quad{Triple: triple}); err != nil {
			return nil, err
		}
	}
	return d, nil
}

func decodeTriG12(r io.Reader, base string) (*rdf.Dataset, error) {
	d := rdf.NewDataset()
	for quad, err := range trig12.Decode(withBase(r, base)) {
		if err != nil {
			return nil, err
		}
		if err := d.Add(quad); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// canonicalNTriples12 reads a document and writes its triples back as
// canonical N-Triples (RDF 1.2 N-Triples §3).
//
// The triples are collected before being written because a canonicalization
// test compares text: what is under test is the bytes the whole document
// becomes, and nothing can be compared until the document has been read
// through. The suites' documents are a handful of statements each.
func canonicalNTriples12(r io.Reader) ([]byte, error) {
	var triples []rdf.Triple
	for triple, err := range ntriples12.Decode(r) {
		if err != nil {
			return nil, err
		}
		triples = append(triples, triple)
	}

	var b bytes.Buffer
	if err := ntriples12.Encode(&b, slices.Values(triples)); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// canonicalNQuads12 reads a document and writes its quads back as canonical
// N-Quads (RDF 1.2 N-Quads §3).
func canonicalNQuads12(r io.Reader) ([]byte, error) {
	var quads []rdf.Quad
	for quad, err := range nquads12.Decode(r) {
		if err != nil {
			return nil, err
		}
		quads = append(quads, quad)
	}

	var b bytes.Buffer
	if err := nquads12.Encode(&b, slices.Values(quads)); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
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

// loadedManifest is one manifest and the IRI of the document it was read from,
// which is not always the IRI of the node it describes: the RDF 1.1 manifests
// describe <>, the document itself, while the RDF 1.2 ones describe a fragment
// of the directory they sit in.
type loadedManifest struct {
	docIRI rdf.IRI
	*manifest
}

// loadSuite reads the manifest a suite describes itself in, and every manifest
// that one includes, in the order a report would list their tests: a manifest
// made of others contributes what it includes before whatever it declares
// itself.
//
// The RDF 1.2 suites are nothing but an mf:include of three manifests — the
// canonicalization tests, the syntax tests, and the whole RDF 1.1 suite for
// the same syntax, which an RDF 1.2 parser must go on reading. Following the
// includes is therefore what running an RDF 1.2 suite at all consists of.
func loadSuite(t *testing.T, s suite) []loadedManifest {
	t.Helper()

	var loaded []loadedManifest
	seen := make(map[rdf.IRI]bool)

	var load func(docIRI rdf.IRI)
	load = func(docIRI rdf.IRI) {
		if seen[docIRI] {
			t.Fatalf("%s is included more than once, so the suite's manifests include each other in a cycle", docIRI)
		}
		seen[docIRI] = true

		m := loadManifest(t, docIRI)
		for _, include := range m.includes {
			load(include)
		}
		loaded = append(loaded, loadedManifest{docIRI: docIRI, manifest: m})
	}
	load(s.manifestIRI())

	if len(entriesOf(loaded)) == 0 {
		t.Fatalf("%s declares no tests", s.manifestIRI())
	}
	return loaded
}

// entriesOf flattens the manifests of a suite into the tests they declare.
func entriesOf(loaded []loadedManifest) []manifestEntry {
	var entries []manifestEntry
	for _, m := range loaded {
		entries = append(entries, m.entries...)
	}
	return entries
}

// loadManifest reads one manifest document, named by the IRI it is published
// under.
func loadManifest(t *testing.T, docIRI rdf.IRI) *manifest {
	t.Helper()

	path, base := localFile(t, docIRI)

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer file.Close()

	m, err := readManifest(file, base)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
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
//
// What is checked is mf:assumedTestBase and not the IRI of the manifest node,
// which nothing here reads and which the suites do not agree on: the RDF 1.1
// manifests describe <>, the document itself, the RDF 1.2 ones a fragment of
// the directory they sit in, and the RDF 1.2 N-Quads manifest a fragment of
// the N-Triples directory, having been written by copying that one.
func TestManifestsAgreeWithTheirAssumedBase(t *testing.T) {
	for _, s := range suites {
		t.Run(s.dir, func(t *testing.T) {
			for _, m := range loadSuite(t, s) {
				dir, _, _ := strings.Cut(strings.TrimPrefix(string(m.docIRI), publishedAt), "manifest.ttl")

				t.Run(dir, func(t *testing.T) {
					if m.assumedTestBase == "" {
						t.Skip("the manifest declares no mf:assumedTestBase")
					}
					if want := rdf.IRI(publishedAt + dir); m.assumedTestBase != want {
						t.Errorf("mf:assumedTestBase is %s, want %s", m.assumedTestBase, want)
					}
				})
			}
		})
	}
}

// unreferenced lists the files the vendored suites ship that no manifest names
// a test for, keyed by the path each sits at under rdf/.
//
// These are not skips: no test was left unrun, because the manifests declare
// none. They are files the W3C suites carry which nothing in them refers to,
// and they are enumerated for the same reason a skip is — so that a file
// arriving unreferenced is noticed rather than assumed to be one of these.
var unreferenced = map[string]string{
	// Positive syntax tests are checked by parsing alone, so the expected
	// result vendored beside each of these is never read.
	"rdf11/rdf-turtle/turtle-syntax-blank-label.nt": "result of a positive syntax test, which has no result to compare",
	"rdf11/rdf-turtle/turtle-syntax-ln-colons.nt":   "result of a positive syntax test, which has no result to compare",
	"rdf11/rdf-turtle/turtle-syntax-ln-dots.nt":     "result of a positive syntax test, which has no result to compare",

	// The manifests mention none of these at all, under any name.
	"rdf11/rdf-n-triples/literal_true.nt":                                    "the manifest declares no test for it",
	"rdf11/rdf-n-triples/literal_false.nt":                                   "the manifest declares no test for it",
	"rdf11/rdf-n-quads/literal_true.nq":                                      "the manifest declares no test for it",
	"rdf11/rdf-n-quads/literal_false.nq":                                     "the manifest declares no test for it",
	"rdf11/rdf-turtle/test-38.ttl":                                           "the manifest declares no test for it",
	"rdf11/rdf-turtle/test-38.nt":                                            "the manifest declares no test for it",
	"rdf11/rdf-turtle/turtle-syntax-pname-dots.ttl":                          "the manifest declares no test for it",
	"rdf11/rdf-turtle/localName_with_PN_CHARS_BASE_character_boundaries.ttl": "the manifest declares no test for it",
	"rdf11/rdf-turtle/localName_with_PN_CHARS_BASE_character_boundaries.nt":  "the manifest declares no test for it",
	"rdf11/rdf-trig/localName_with_PN_CHARS_BASE_character_boundaries.trig":  "the manifest declares no test for it",
	"rdf11/rdf-trig/localName_with_PN_CHARS_BASE_character_boundaries.nq":    "the manifest declares no test for it",

	// Each is the next in a run of negative syntax documents whose manifest
	// stops one short: the RDF 1.2 Turtle suite declares nt-ttl12-bad-01
	// through -09 and the TriG suite trig12-syntax-bad-01 through -07.
	"rdf12/rdf-turtle/syntax/nt-ttl12-bad-10.ttl":     "the manifest declares no test for it",
	"rdf12/rdf-trig/syntax/trig12-syntax-bad-08.trig": "the manifest declares no test for it",
}

// describes are the files a suite is described by rather than made of, which
// no test names because they are not tests.
var describes = []string{"manifest.ttl", "README", "README.md", "LICENSE"}

// TestManifestsAccountForEveryVendoredFile fails if the vendored tree holds a
// file no test names and [unreferenced] does not account for.
//
// The harness this replaced walked the directory, so an unrecognised file was
// in front of it. This one walks the manifests and would never see one: a test
// file left out of a manifest, or vendored under a name nothing refers to,
// would simply never run. This is what notices.
func TestManifestsAccountForEveryVendoredFile(t *testing.T) {
	named := make(map[string]bool)
	for _, s := range suites {
		for _, entry := range entriesOf(loadSuite(t, s)) {
			for _, iri := range []rdf.IRI{entry.action, entry.result} {
				if iri == "" {
					continue
				}
				path, _ := localFile(t, iri)
				named[path] = true
			}
		}
	}

	root := filepath.Join(testdata, "rdf")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if named[path] || slices.Contains(describes, d.Name()) {
			return nil
		}

		key := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
		if reason, ok := unreferenced[key]; ok {
			t.Logf("%s is vendored but unreferenced: %s", key, reason)
			return nil
		}
		t.Errorf("%s is vendored but no test names it; add the test that uses it, "+
			"list it in unreferenced with a reason, or stop vendoring it", key)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}

// TestSkippedTestsExist keeps [skipped] honest: an entry naming a test no
// manifest declares is a skip that stopped being one, and it is removed rather
// than left standing as an excuse nothing needs.
func TestSkippedTestsExist(t *testing.T) {
	declared := make(map[string]bool)
	for _, s := range suites {
		for _, entry := range entriesOf(loadSuite(t, s)) {
			declared[testKey(entry)] = true
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
		if _, err := os.Stat(filepath.Join(testdata, "rdf", filepath.FromSlash(key))); err != nil {
			t.Errorf("%s is listed as unreferenced but is not vendored: %v", key, err)
		}
	}
}

// rdf12OnlyManifests are the manifests every one of whose documents is written
// with something RDF 1.2 added, keyed by the manifest's directory under the IRI
// the suites are published at, with what makes the whole of it RDF 1.2 as the
// value.
//
// A document written with something RDF 1.2 added — a triple term, a reifier,
// an annotation, a version directive, or a literal carrying a base direction —
// is not an RDF 1.1 document at all, and the RDF 1.1 parser for its syntax must
// refuse it. Which of the RDF 1.2 suites' documents those are is written out
// here and in [rdf12Only] rather than worked out from the documents, because it
// is a judgement about the grammar and belongs somewhere it can be read.
//
// Every test an RDF 1.2 suite declares whose document must be read is looked up
// in the two, both ways round: one they say is RDF 1.2 must be refused by the
// RDF 1.1 parser, and one they do not must be read by it. A test whose document
// changes under them therefore fails rather than quietly deciding it was RDF 1.1
// all along — which is why they say which documents are RDF 1.2 and never which
// the parsers happen to refuse.
//
// A manifest belongs here only when the claim holds of every test in it; a
// manifest holding a mix is left out and its RDF 1.2 tests named one by one in
// [rdf12Only].
var rdf12OnlyManifests = map[string]string{
	"rdf12/rdf-n-triples/syntax": "the syntax suites are about what RDF 1.2 added and nothing else",
	"rdf12/rdf-n-quads/syntax":   "the syntax suites are about what RDF 1.2 added and nothing else",
	"rdf12/rdf-turtle/syntax":    "the syntax suites are about what RDF 1.2 added and nothing else",
	"rdf12/rdf-trig/syntax":      "the syntax suites are about what RDF 1.2 added and nothing else",

	"rdf12/rdf-turtle/eval": "the Turtle and TriG evaluation suites are new in RDF 1.2, and every " +
		"document in them states a reified triple, a triple term or an annotation",
	"rdf12/rdf-trig/eval": "the Turtle and TriG evaluation suites are new in RDF 1.2, and every " +
		"document in them states a reified triple, a triple term or an annotation",
}

// rdf12Only names the RDF 1.2 tests whose document is RDF 1.2 in a manifest
// where not every document is, keyed as [testKey] keys a test.
var rdf12Only = map[string]bool{
	// The canonicalization suites are mostly the RDF 1.1 documents again, with
	// these written in what RDF 1.2 added.
	"rdf12/rdf-n-triples/c14n#dirlangtagged_string": true,
	"rdf12/rdf-n-triples/c14n#triple-term-01":       true,
	"rdf12/rdf-n-triples/c14n#triple-term-02":       true,
	"rdf12/rdf-n-triples/c14n#triple-term-03":       true,
	"rdf12/rdf-n-triples/c14n#triple-term-04":       true,
	"rdf12/rdf-n-quads/c14n#dirlangtagged_string":   true,
	"rdf12/rdf-n-quads/c14n#triple-term-01":         true,
	"rdf12/rdf-n-quads/c14n#triple-term-02":         true,
	"rdf12/rdf-n-quads/c14n#triple-term-03":         true,
	"rdf12/rdf-n-quads/c14n#triple-term-04":         true,
}

// isRDF12Only reports whether the document of the test keyed by key is written
// with something RDF 1.2 added, by either the manifest it is declared in or the
// test itself being listed above.
func isRDF12Only(key string) bool {
	manifest, _, _ := strings.Cut(key, "#")
	if _, ok := rdf12OnlyManifests[manifest]; ok {
		return true
	}
	return rdf12Only[key]
}

// TestRDF11ParsersRefuseRDF12Syntax feeds every document an RDF 1.2 suite
// expects to be read to the RDF 1.1 parser for the same syntax, and checks it
// is refused where the document uses something RDF 1.2 added and read where it
// does not.
//
// This is what the versioned package split is for. The rdf11 and rdf12
// packages exist separately so that a caller can say which specification it
// means, and that only holds while an rdf11 parser really does refuse RDF 1.2:
// a tokenizer that quietly grew a triple term would leave the two packages
// differing in name alone. Nothing in the W3C suites checks it, because a
// suite tests one version at a time.
//
// The other direction — an RDF 1.2 parser reading RDF 1.1 — is checked by
// TestConformance itself, the RDF 1.2 manifests including the RDF 1.1 suite
// for the same syntax.
func TestRDF11ParsersRefuseRDF12Syntax(t *testing.T) {
	for _, s := range suites {
		if s.spec != rdf12 {
			continue
		}

		t.Run(s.dir, func(t *testing.T) {
			for _, entry := range entriesOf(loadSuite(t, s)) {
				// Only what the RDF 1.2 suite declares itself. The rest is the
				// RDF 1.1 suite it includes, which the RDF 1.1 parsers are run
				// against by TestConformance already.
				if !s.declares(entry.action) {
					continue
				}
				typ, ok := testTypes[entry.typ]
				if !ok || !expectsTheDocumentRead(typ.kind) {
					continue
				}

				t.Run(entry.name, func(t *testing.T) {
					t.Log(testKey(entry))

					f := formatOf(t, rdf11, typ.action)
					action, base := localFile(t, entry.action)
					_, err := decodeFile(action, base, f)

					if isRDF12Only(testKey(entry)) {
						if err == nil {
							t.Errorf("%s read it without error, want an RDF 1.1 parser to refuse a document written in what RDF 1.2 added", f.name)
						}
						return
					}
					if err != nil {
						t.Errorf("%s refused it: %v\nneither rdf12Only nor rdf12OnlyManifests lists it, so its document is expected to be RDF 1.1 as well", f.name, err)
					}
				})
			}
		})
	}
}

// TestRDF12OnlyTestsExist keeps [rdf12Only] and [rdf12OnlyManifests] honest,
// the way TestSkippedTestsExist keeps [skipped] honest: an entry naming a test
// or a manifest no RDF 1.2 suite declares is one whose assertion nothing makes.
//
// A test named in rdf12Only that its manifest already covers is reported too.
// The two would agree, so nothing would fail — but the entry would be read as
// singling the test out, when what makes it RDF 1.2 is a claim about the whole
// manifest.
func TestRDF12OnlyTestsExist(t *testing.T) {
	declared := make(map[string]bool)
	manifests := make(map[string]bool)
	for _, s := range suites {
		if s.spec != rdf12 {
			continue
		}
		for _, entry := range entriesOf(loadSuite(t, s)) {
			if !s.declares(entry.action) {
				continue
			}
			typ, ok := testTypes[entry.typ]
			if !ok || !expectsTheDocumentRead(typ.kind) {
				continue
			}

			key := testKey(entry)
			declared[key] = true

			manifest, _, _ := strings.Cut(key, "#")
			manifests[manifest] = true
		}
	}

	for key := range rdf12Only {
		if !declared[key] {
			t.Errorf("%s is listed as RDF 1.2 only but no RDF 1.2 suite declares it as a test whose document must be read", key)
			continue
		}
		manifest, _, _ := strings.Cut(key, "#")
		if _, ok := rdf12OnlyManifests[manifest]; ok {
			t.Errorf("%s is listed as RDF 1.2 only, but the whole of %s already is; remove the entry", key, manifest)
		}
	}

	for manifest := range rdf12OnlyManifests {
		if !manifests[manifest] {
			t.Errorf("%s is listed as wholly RDF 1.2 but no RDF 1.2 suite declares a test in it", manifest)
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
