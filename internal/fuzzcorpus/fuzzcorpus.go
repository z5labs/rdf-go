// Package fuzzcorpus builds fuzz seed corpora out of the vendored W3C test
// suites.
//
// A fuzzer is only as good as what it starts from: mutating an empty string
// finds the first byte of a grammar and little else, while mutating a document
// the suite already wrote reaches the corners of it — a triple term, a
// collection, a numeric escape at the end of a long literal. The suites are
// also already both halves of what a parser has to get right, since every one
// of them ships the documents that must be rejected next to the documents that
// must be read, so seeding from all of a suite's files seeds from its negative
// cases as well as its positive ones.
//
// The suites are vendored under testsuite/testdata, which is the fuzz targets'
// only reason to look outside their own package: reproducing several hundred
// documents into eight testdata directories would be the same corpus written
// down eight more times, and drifting from the suites the moment either was
// updated. [Seeds] finds them by walking up to the module root, so it does not
// care which package's directory the test happens to run in.
package fuzzcorpus

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// suiteRoot is where the vendored suites sit, relative to the module root.
// Every suite is a directory under it — "rdf11/rdf-turtle", say.
const suiteRoot = "testsuite/testdata/rdf-tests/rdf"

// ErrNoModuleRoot is reported when no go.mod is found in the working directory
// or any directory above it, and so the vendored suites cannot be located.
var ErrNoModuleRoot = errors.New("fuzzcorpus: no go.mod above the working directory")

// Seeds returns the contents of every file whose name ends in ext under the
// named suites, which are paths relative to the vendored suite root — for
// instance "rdf11/rdf-n-triples". Subdirectories are walked, because the RDF
// 1.2 suites keep their documents in syntax, eval and c14n directories rather
// than beside their manifest.
//
// Files are returned in the order the walk reaches them, which is lexical, and
// a document that appears in more than one suite is returned once. Both are so
// that a target seeds identically from one run to the next: the fuzzing engine
// remembers a corpus by content, and a seed that comes and goes would keep
// re-entering it.
//
// It reports an error rather than an empty corpus if a suite is missing, since
// a target that silently seeds from nothing is a target that fuzzes an empty
// string.
func Seeds(ext string, suites ...string) ([]string, error) {
	root, err := moduleRoot()
	if err != nil {
		return nil, err
	}

	var seeds []string
	seen := make(map[string]bool)
	for _, suite := range suites {
		dir := filepath.Join(root, suiteRoot, filepath.FromSlash(suite))
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(d.Name(), ext) {
				return nil
			}

			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			seed := string(b)
			if seen[seed] {
				return nil
			}
			seen[seed] = true
			seeds = append(seeds, seed)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("fuzzcorpus: reading suite %s: %w", suite, err)
		}
	}
	return seeds, nil
}

// moduleRoot returns the directory of the module's go.mod, found by walking up
// from the working directory.
//
// A test runs in its own package's directory, and the eight format packages
// sit at two different depths' worth of nothing in common, so the walk is what
// keeps the callers from each spelling out a relative path to the same place.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNoModuleRoot
		}
		dir = parent
	}
}
