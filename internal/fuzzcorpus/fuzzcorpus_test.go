package fuzzcorpus_test

import (
	"errors"
	"io/fs"
	"slices"
	"testing"

	"github.com/z5labs/rdf-go/internal/fuzzcorpus"
)

// TestSeeds covers what a fuzz target relies on: that a suite it names is
// found from wherever its own package sits, that it gets the documents of that
// suite and no others, and that a suite it names wrongly is an error rather
// than an empty corpus.
func TestSeeds(t *testing.T) {
	t.Run("returns the documents of a suite", func(t *testing.T) {
		seeds, err := fuzzcorpus.Seeds(".nt", "rdf11/rdf-n-triples")
		if err != nil {
			t.Fatalf("Seeds() = %v, want nil", err)
		}
		if len(seeds) == 0 {
			t.Fatal("Seeds() returned no seeds, want the RDF 1.1 N-Triples suite")
		}

		// literal_true.nt, whole. The walk is checked against a document it
		// should have found rather than only against having found something.
		want := "<http://a.example/s> <http://a.example/p> \"true\"^^<http://www.w3.org/2001/XMLSchema#boolean> .\n"
		if !slices.Contains(seeds, want) {
			t.Errorf("Seeds() does not hold %q", want)
		}
	})

	t.Run("takes only files with the extension", func(t *testing.T) {
		// The suite's manifest is the one .ttl in a directory of .nt files,
		// which makes it what a filter on the extension has to leave out.
		manifests, err := fuzzcorpus.Seeds(".ttl", "rdf11/rdf-n-triples")
		if err != nil {
			t.Fatalf("Seeds() = %v, want nil", err)
		}
		if len(manifests) != 1 {
			t.Fatalf("Seeds(\".ttl\") = %d seeds, want 1 — the manifest", len(manifests))
		}

		seeds, err := fuzzcorpus.Seeds(".nt", "rdf11/rdf-n-triples")
		if err != nil {
			t.Fatalf("Seeds() = %v, want nil", err)
		}
		if slices.Contains(seeds, manifests[0]) {
			t.Error("Seeds(\".nt\") holds the manifest, which is a .ttl")
		}
	})

	t.Run("walks subdirectories", func(t *testing.T) {
		// The RDF 1.2 suites keep their documents in syntax, eval and c14n
		// directories, so a walk that stopped at the top would find nothing
		// but the manifest.
		seeds, err := fuzzcorpus.Seeds(".nt", "rdf12/rdf-n-triples")
		if err != nil {
			t.Fatalf("Seeds() = %v, want nil", err)
		}
		if len(seeds) == 0 {
			t.Fatal("Seeds() returned no seeds, want the RDF 1.2 N-Triples suite")
		}
	})

	t.Run("returns a document held by two suites once", func(t *testing.T) {
		once, err := fuzzcorpus.Seeds(".nt", "rdf11/rdf-n-triples")
		if err != nil {
			t.Fatalf("Seeds() = %v, want nil", err)
		}

		twice, err := fuzzcorpus.Seeds(".nt", "rdf11/rdf-n-triples", "rdf11/rdf-n-triples")
		if err != nil {
			t.Fatalf("Seeds() = %v, want nil", err)
		}

		if len(twice) != len(once) {
			t.Errorf("Seeds() over one suite twice = %d seeds, want %d", len(twice), len(once))
		}
	})

	t.Run("reports a suite that is not there", func(t *testing.T) {
		seeds, err := fuzzcorpus.Seeds(".nt", "rdf11/rdf-no-such-suite")
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Seeds() = %v, want fs.ErrNotExist", err)
		}
		if seeds != nil {
			t.Errorf("Seeds() = %d seeds, want none", len(seeds))
		}
	})
}
