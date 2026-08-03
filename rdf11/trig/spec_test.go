package trig_test

import (
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
)

// The documents the TriG specification writes out for itself, which are the
// nearest thing to a conformance suite this package has until the
// manifest-driven harness arrives. Each is copied as written, indentation and
// comments included, so what is exercised is a document someone would actually
// write rather than one shaped to suit the parser.

// specOneNamedGraph is TriG §2.4.
const specOneNamedGraph = `# This document encodes one graph.
@prefix ex: <http://www.example.org/vocabulary#> .
@prefix : <http://www.example.org/exampleDocument#> .

:G1 {
  :Monica a ex:Person ;
    ex:name "Monica Murphy" ;
    ex:homepage <http://www.monicamurphy.org> ;
    ex:email <mailto:monica@monicamurphy.org> ;
    ex:hasSkill ex:Management ,
      ex:Programming .
}
`

// specDefaultAndNamedGraphs is TriG §2.2, where the default graph is written
// as a block with no label alongside two named graphs.
const specDefaultAndNamedGraphs = `@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix dc: <http://purl.org/dc/elements/1.1/> .
@prefix foaf: <http://xmlns.com/foaf/0.1/> .

# default graph
    {
      <http://example.org/bob> dc:publisher "Bob" .
      <http://example.org/alice> dc:publisher "Alice" .
    }

<http://example.org/bob>
    {
       _:a foaf:name "Bob" .
       _:a foaf:mbox <mailto:bob@oldcorp.example.org> .
       _:a foaf:knows _:b .
    }

<http://example.org/alice>
    {
       _:b foaf:name "Alice" .
       _:b foaf:mbox <mailto:alice@work.example.org> .
    }
`

// specGraphKeyword is TriG §2.3, which writes the same dataset with the
// "GRAPH" keyword and with statements standing outside every block.
const specGraphKeyword = `@prefix dc: <http://purl.org/dc/elements/1.1/> .
@prefix foaf: <http://xmlns.com/foaf/0.1/> .

<http://example.org/bob> dc:publisher "Bob" .
<http://example.org/alice> dc:publisher "Alice" .

GRAPH <http://example.org/bob>
{
   _:a foaf:name "Bob" .
   _:a foaf:mbox <mailto:bob@oldcorp.example.org> .
}

GRAPH <http://example.org/alice>
{
   _:b foaf:name "Alice" .
   _:b foaf:mbox <mailto:alice@work.example.org> .
}
`

func TestSpecExamples(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		wantDefault int
		wantNamed   int
		wantLen     int
	}{
		{
			name:        "one named graph",
			src:         specOneNamedGraph,
			wantDefault: 0,
			wantNamed:   1,
			wantLen:     6,
		},
		{
			name:        "a default graph block beside two named graphs",
			src:         specDefaultAndNamedGraphs,
			wantDefault: 2,
			wantNamed:   2,
			wantLen:     7,
		},
		{
			name:        "the GRAPH keyword beside statements outside every block",
			src:         specGraphKeyword,
			wantDefault: 2,
			wantNamed:   2,
			wantLen:     6,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			d := datasetFromTriG(t, tc.src)

			if got := d.Default().Len(); got != tc.wantDefault {
				t.Errorf("the default graph holds %d triples, want %d", got, tc.wantDefault)
			}
			if got := len(namesOf(d)); got != tc.wantNamed {
				t.Errorf("named %d graphs, want %d", got, tc.wantNamed)
			}
			if got := d.Len(); got != tc.wantLen {
				t.Errorf("the dataset holds %d triples, want %d", got, tc.wantLen)
			}

			// What the specification wrote has to survive being written back.
			encoded := encodeToString(t, d)
			if !isomorphicDatasets(t, d, datasetFromTriG(t, encoded)) {
				t.Errorf("Dataset(Encode(d)) is not isomorphic to d\nencoded %q", encoded)
			}
		})
	}
}

// TestSpecDefaultGraphIsOneGraph covers the claim TriG §2.2 makes and this
// package has to keep: the statements outside every block and those of a block
// with no label are the same graph.
func TestSpecDefaultGraphIsOneGraph(t *testing.T) {
	outside := datasetFromTriG(t, specGraphKeyword).Default()
	inside := datasetFromTriG(t, specDefaultAndNamedGraphs).Default()

	if !rdf.Isomorphic(outside, inside) {
		t.Errorf(
			"the two ways of writing the default graph differ\noutside\n%sinside\n%s",
			formatTriples(outside), formatTriples(inside),
		)
	}
}

func namesOf(d *rdf.Dataset) []rdf.Term {
	var names []rdf.Term
	for name := range d.GraphNames() {
		names = append(names, name)
	}
	return names
}

func formatTriples(g *rdf.Graph) string {
	var b strings.Builder
	for triple := range g.All() {
		b.WriteString("  ")
		b.WriteString(triple.String())
		b.WriteString("\n")
	}
	return b.String()
}
