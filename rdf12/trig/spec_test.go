package trig_test

import (
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
)

// The documents the RDF 1.2 TriG specification writes out for itself, which are
// the nearest thing to a conformance suite this package has until the
// manifest-driven harness reaches RDF 1.2 TriG (#34). Each is copied as
// written, indentation and comments included, so what is exercised is a
// document someone would actually write rather than one shaped to suit the
// parser.

// specOneNamedGraph is RDF 1.2 TriG §2.1.
const specOneNamedGraph = `# This document encodes one graph.
PREFIX ex: <http://www.example.org/vocabulary#>
PREFIX : <http://www.example.org/exampleDocument#>

:G1 { :Monica a ex:Person ;
    ex:name "Monica Murphy" ;
    ex:homepage <http://www.monicamurphy.org> ;
    ex:email <mailto:monica@monicamurphy.org> ;
    ex:hasSkill ex:Management ,
                ex:Programming .
}
`

// specDefaultAndNamedGraphs is RDF 1.2 TriG §2.2, where the default graph is
// written as a block with no label alongside two named graphs.
const specDefaultAndNamedGraphs = `# This document contains a default graph and two named graphs.

PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>
PREFIX dc: <http://purl.org/dc/terms/>
PREFIX foaf: <http://xmlns.com/foaf/0.1/>

# default graph
{
  <http://example.org/bob> dc:publisher "Bob" .
  <http://example.org/alice> dc:publisher "Alice" .
}

<http://example.org/bob> {
   _:a foaf:name "Bob" .
   _:a foaf:mbox <mailto:bob@oldcorp.example.org> .
   _:a foaf:knows _:b .
}

<http://example.org/alice> {
   _:b foaf:name "Alice" .
   _:b foaf:mbox <mailto:alice@work.example.org> .
}
`

// specGraphKeyword is RDF 1.2 TriG §2.3, which writes the same dataset with the
// "GRAPH" keyword and with statements standing outside every block. Its last
// block leaves off the '.' the grammar lets a block's last statement omit.
const specGraphKeyword = `# This document contains a same data as the previous example.

PREFIX rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#>
PREFIX dc: <http://purl.org/dc/terms/>
PREFIX foaf: <http://xmlns.com/foaf/0.1/>

# default graph - no {} used.
<http://example.org/bob> dc:publisher "Bob" .
<http://example.org/alice> dc:publisher "Alice" .

# GRAPH keyword to highlight a named graph
# Abbreviation of triples using ;
GRAPH <http://example.org/bob>
{
   [] foaf:name "Bob" ;
      foaf:mbox <mailto:bob@oldcorp.example.org> ;
      foaf:knows _:b .
}

GRAPH <http://example.org/alice>
{
    _:b foaf:name "Alice" ;
        foaf:mbox <mailto:alice@work.example.org>
}
`

// specRDF12Features is not the specification's own, there being no example in
// it that uses the RDF 1.2 constructs inside a graph block. It is written here
// so that the acceptance the issue asks for — every RDF 1.2 form working inside
// a block — is exercised as one document rather than one construct at a time.
const specRDF12Features = `VERSION "1.2"
PREFIX ex: <http://www.example.org/>

# The default graph, outside every block.
ex:s ex:p ex:o ~ ex:r {| ex:certainty 0.8 |} .

GRAPH ex:g {
    ex:s ex:p "نص عربي"@ar--rtl , "English"@en--ltr .
    ex:claim ex:says <<( ex:s ex:p ex:o )>> .
    << ex:s ex:p ex:o ~ ex:r2 >> ex:source ex:doc .
}

ex:h {
    ex:s ex:p ex:o {| ex:certainty 0.9 |} .
}

{ ex:s ex:q ex:r . }
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
			wantLen:     7,
		},
		{
			// The default graph holds the annotated triple, the rdf:reifies
			// naming its reifier, what the annotation block says about that
			// reifier, and the statement written in the block with no label.
			// ex:g holds five: two directional literals, the triple term's
			// enclosing triple, and the reified triple's rdf:reifies and the
			// triple written about its reifier. ex:h holds three, an annotation
			// being one triple asserted and two more describing it.
			name:        "the RDF 1.2 constructs inside and outside blocks",
			src:         specRDF12Features,
			wantDefault: 4,
			wantNamed:   2,
			wantLen:     12,
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
				t.Errorf("the dataset holds %d triples, want %d:\n%s", got, tc.wantLen, formatDataset(d))
			}

			// What the specification wrote has to survive being written back.
			encoded := encodeToString(t, d)
			if !isomorphicDatasets(t, d, datasetFromTriG(t, encoded)) {
				t.Errorf("Dataset(Encode(d)) is not isomorphic to d\nencoded %q", encoded)
			}

			// And so does the document itself, read and written as a tree.
			printed := printToString(t, parse(t, tc.src))
			if !isomorphicDatasets(t, d, datasetFromTriG(t, printed)) {
				t.Errorf("Dataset(Print(Parse(src))) is not isomorphic to d\nprinted %q", printed)
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
