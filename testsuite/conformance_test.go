package testsuite_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z5labs/rdf-go/rdf11/nquads"
	"github.com/z5labs/rdf-go/rdf11/ntriples"
)

// pinnedCommit is the w3c/rdf-tests commit the vendored suites were taken
// from. TestVendoredSuitesRecordTheirCommit keeps it and PROVENANCE.md
// agreeing, so that neither can be updated while the other is forgotten.
const pinnedCommit = "767554e135eb6665949d870e6fa7bbc813837293"

const testdata = "testdata/rdf-tests"

// skipped lists the tests this harness does not run, and why.
//
// It is empty, and a test that cannot be run belongs here rather than being
// quietly passed over. Every file in a vendored suite is otherwise accounted
// for: the harness fails on one it cannot classify rather than ignoring it.
var skipped = map[string]string{}

// suite is one vendored W3C test suite.
type suite struct {
	// name is the suite's directory, and the name of the subtest that runs it.
	name string

	// ext is the extension its test files carry.
	ext string

	// read consumes a document, reporting whether it could be read. A positive
	// test must be readable and a negative one must not.
	read func(io.Reader) error
}

func readNTriples(r io.Reader) error {
	for _, err := range ntriples.Decode(r) {
		if err != nil {
			return err
		}
	}
	return nil
}

func readNQuads(r io.Reader) error {
	for _, err := range nquads.Decode(r) {
		if err != nil {
			return err
		}
	}
	return nil
}

// TestConformance runs every vendored suite, one Go subtest per W3C test file,
// so a failure names the file that caused it.
func TestConformance(t *testing.T) {
	suites := []suite{
		{name: "rdf-n-triples", ext: ".nt", read: readNTriples},
		{name: "rdf-n-quads", ext: ".nq", read: readNQuads},
	}

	for _, s := range suites {
		t.Run(s.name, func(t *testing.T) {
			runSuite(t, s)
		})
	}
}

func runSuite(t *testing.T, s suite) {
	dir := filepath.Join(testdata, "rdf", "rdf11", s.name)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}

	var positive, negative, skips int
	for _, entry := range entries {
		name := entry.Name()

		// The manifest and the README describe the suite rather than being
		// part of it. Anything else that is not a test file is a surprise,
		// and is reported rather than passed over.
		if entry.IsDir() || name == "manifest.ttl" || name == "README" {
			continue
		}
		if filepath.Ext(name) != s.ext {
			t.Errorf("%s: not a %s file and not one of the suite's own descriptions; "+
				"teach the harness what it is rather than leaving it unrun", name, s.ext)
			continue
		}

		if reason, ok := skipped[name]; ok {
			skips++
			t.Run(name, func(t *testing.T) {
				t.Skipf("skipped: %s", reason)
			})
			continue
		}

		// A negative syntax test is named for it. The two suites hold nothing
		// else, which TestSuitesHoldOnlySyntaxTests keeps true.
		wantError := strings.Contains(name, "-bad-")
		if wantError {
			negative++
		} else {
			positive++
		}

		t.Run(name, func(t *testing.T) {
			f, err := os.Open(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("opening: %v", err)
			}
			defer f.Close()

			err = s.read(f)
			switch {
			case wantError && err == nil:
				t.Error("read without error, want a negative syntax test to be rejected")
			case !wantError && err != nil:
				t.Errorf("read failed, want a positive syntax test to be accepted: %v", err)
			}
		})
	}

	t.Logf("%s: %d positive, %d negative, %d skipped, %d total",
		s.name, positive, negative, skips, positive+negative+skips)
}

// TestSuitesHoldOnlySyntaxTests guards the assumption that lets this harness
// work from filenames.
//
// The two RDF 1.1 suites declare positive and negative syntax tests and
// nothing else — no evaluation tests, which would name an expected result that
// a filename cannot carry and that this harness would have no way to check.
// Rather than trust that, it reads the manifests as text and fails if a test
// type it cannot run ever appears, so that the tests are not silently left
// unrun the day the suites grow one.
//
// Grepping a manifest is not parsing it. The parsing is what waits for Turtle.
func TestSuitesHoldOnlySyntaxTests(t *testing.T) {
	runnable := []string{
		"rdft:TestNTriplesPositiveSyntax",
		"rdft:TestNTriplesNegativeSyntax",
		"rdft:TestNQuadsPositiveSyntax",
		"rdft:TestNQuadsNegativeSyntax",
	}

	for _, name := range []string{"rdf-n-triples", "rdf-n-quads"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(testdata, "rdf", "rdf11", name, "manifest.ttl")

			manifest, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading %s: %v", path, err)
			}

			for line := range strings.Lines(string(manifest)) {
				_, rest, found := strings.Cut(line, "rdft:Test")
				if !found {
					continue
				}
				declared := "rdft:Test" + strings.FieldsFunc(rest, func(r rune) bool {
					return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'))
				})[0]

				if !slicesContains(runnable, declared) {
					t.Errorf("the suite declares %s, which this harness cannot run; "+
						"teach it or list the tests in skipped with a reason", declared)
				}
			}
		})
	}
}

func slicesContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
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
