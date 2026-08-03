package testsuite_test

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
)

// The manifest reader is this module reading its own test suite with its own
// Turtle parser, and that is a circle: a parser that reads a manifest wrongly
// can make a suite look like it passes, and the suite is what would otherwise
// have caught the parser. Nothing inside the module can break the circle.
//
// What these tests do instead is anchor it. Every fixture below is small
// enough to check by hand, and each says what a manifest means in a form that
// does not depend on the parser being right — a Go value written out in full.
// If the parser starts reading collections backwards, or losing a property, or
// resolving a relative IRI against the wrong base, one of these fails, and it
// fails whatever the vendored suites go on to say.

// fixtureBase is the IRI the fixtures below are read as being published under.
// Their relative IRIs resolve against it, which is most of what there is to
// check: <> is the manifest, <#a> a test in it, <a.ttl> a document beside it.
const fixtureBase = "https://example.test/suite/manifest.ttl"

// prefixes are the declarations every fixture opens with, so that each fixture
// is only what it is about.
const prefixes = `@prefix rdf: <http://www.w3.org/1999/02/22-rdf-syntax-ns#> .
@prefix rdfs: <http://www.w3.org/2000/01/rdf-schema#> .
@prefix mf: <http://www.w3.org/2001/sw/DataAccess/tests/test-manifest#> .
@prefix rdft: <http://www.w3.org/ns/rdftest#> .
`

func TestReadManifest(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want *manifest
	}{
		{
			name: "a syntax test and an evaluation test",
			src: `
<> rdf:type mf:Manifest ;
   rdfs:label "Example tests" ;
   mf:assumedTestBase <https://example.test/suite/> ;
   mf:entries ( <#syntax> <#eval> ) .

<#syntax> rdf:type rdft:TestTurtlePositiveSyntax ;
   mf:name "syntax" ;
   rdfs:comment "a document that parses" ;
   mf:action <syntax.ttl> .

<#eval> rdf:type rdft:TestTurtleEval ;
   mf:name "eval" ;
   mf:action <eval.ttl> ;
   mf:result <eval.nt> .
`,
			want: &manifest{
				iri:             "https://example.test/suite/manifest.ttl",
				label:           "Example tests",
				assumedTestBase: "https://example.test/suite/",
				entries: []manifestEntry{
					{
						iri:     "https://example.test/suite/manifest.ttl#syntax",
						typ:     "http://www.w3.org/ns/rdftest#TestTurtlePositiveSyntax",
						name:    "syntax",
						comment: "a document that parses",
						action:  "https://example.test/suite/syntax.ttl",
					},
					{
						iri:    "https://example.test/suite/manifest.ttl#eval",
						typ:    "http://www.w3.org/ns/rdftest#TestTurtleEval",
						name:   "eval",
						action: "https://example.test/suite/eval.ttl",
						result: "https://example.test/suite/eval.nt",
					},
				},
			},
		},
		{
			// A graph is a set, so the order tests are written in says
			// nothing. The mf:entries collection is what orders them, and it
			// here disagrees with the document on purpose.
			name: "entries are ordered by the collection, not the document",
			src: `
<> rdf:type mf:Manifest ; mf:entries ( <#third> <#first> <#second> ) .

<#first> rdf:type rdft:TestTurtlePositiveSyntax ; mf:name "first" ; mf:action <first.ttl> .
<#second> rdf:type rdft:TestTurtlePositiveSyntax ; mf:name "second" ; mf:action <second.ttl> .
<#third> rdf:type rdft:TestTurtlePositiveSyntax ; mf:name "third" ; mf:action <third.ttl> .
`,
			want: &manifest{
				iri: "https://example.test/suite/manifest.ttl",
				entries: []manifestEntry{
					{
						iri:    "https://example.test/suite/manifest.ttl#third",
						typ:    "http://www.w3.org/ns/rdftest#TestTurtlePositiveSyntax",
						name:   "third",
						action: "https://example.test/suite/third.ttl",
					},
					{
						iri:    "https://example.test/suite/manifest.ttl#first",
						typ:    "http://www.w3.org/ns/rdftest#TestTurtlePositiveSyntax",
						name:   "first",
						action: "https://example.test/suite/first.ttl",
					},
					{
						iri:    "https://example.test/suite/manifest.ttl#second",
						typ:    "http://www.w3.org/ns/rdftest#TestTurtlePositiveSyntax",
						name:   "second",
						action: "https://example.test/suite/second.ttl",
					},
				},
			},
		},
		{
			// The vendored manifests write their lists with the collection
			// shorthand. What that shorthand stands for is a chain of cells,
			// and a manifest is free to write the chain out instead.
			name: "a list written out as rdf:first and rdf:rest cells",
			src: `
<> rdf:type mf:Manifest ; mf:entries _:cell .
_:cell rdf:first <#only> ; rdf:rest rdf:nil .

<#only> rdf:type rdft:TestNTriplesNegativeSyntax ; mf:name "only" ; mf:action <only.nt> .
`,
			want: &manifest{
				iri: "https://example.test/suite/manifest.ttl",
				entries: []manifestEntry{
					{
						iri:    "https://example.test/suite/manifest.ttl#only",
						typ:    "http://www.w3.org/ns/rdftest#TestNTriplesNegativeSyntax",
						name:   "only",
						action: "https://example.test/suite/only.nt",
					},
				},
			},
		},
		{
			// The base is the manifest's own IRI, and a relative reference
			// resolves against it by RFC 3986 §5.2 — which takes the manifest
			// name off the path before joining, so a sibling of the manifest
			// is named by its own name alone.
			name: "an action in a directory of its own",
			src: `
<> rdf:type mf:Manifest ; mf:entries ( <#nested> ) .

<#nested> rdf:type rdft:TestTurtlePositiveSyntax ;
   mf:name "nested" ;
   mf:action <sub/nested.ttl> .
`,
			want: &manifest{
				iri: "https://example.test/suite/manifest.ttl",
				entries: []manifestEntry{
					{
						iri:    "https://example.test/suite/manifest.ttl#nested",
						typ:    "http://www.w3.org/ns/rdftest#TestTurtlePositiveSyntax",
						name:   "nested",
						action: "https://example.test/suite/sub/nested.ttl",
					},
				},
			},
		},
		{
			// The RDF 1.2 manifests are made of others and declare no test of
			// their own, so a manifest with mf:include and no mf:entries is
			// read rather than refused, and the includes keep their order.
			name: "a manifest made only of other manifests",
			src: `
<> rdf:type mf:Manifest ;
   rdfs:label "Included tests" ;
   mf:include ( <c14n/manifest.ttl> <syntax/manifest.ttl> <../other/manifest.ttl> ) .
`,
			want: &manifest{
				iri:   "https://example.test/suite/manifest.ttl",
				label: "Included tests",
				includes: []rdf.IRI{
					"https://example.test/suite/c14n/manifest.ttl",
					"https://example.test/suite/syntax/manifest.ttl",
					"https://example.test/other/manifest.ttl",
				},
			},
		},
		{
			// Nothing stops a manifest from doing both, and one that does is
			// read as saying both.
			name: "a manifest with both includes and entries",
			src: `
<> rdf:type mf:Manifest ;
   mf:include ( <sub/manifest.ttl> ) ;
   mf:entries ( <#a> ) .

<#a> rdf:type rdft:TestNQuadsPositiveSyntax ; mf:name "a" ; mf:action <a.nq> .
`,
			want: &manifest{
				iri:      "https://example.test/suite/manifest.ttl",
				includes: []rdf.IRI{"https://example.test/suite/sub/manifest.ttl"},
				entries: []manifestEntry{
					{
						iri:    "https://example.test/suite/manifest.ttl#a",
						typ:    "http://www.w3.org/ns/rdftest#TestNQuadsPositiveSyntax",
						name:   "a",
						action: "https://example.test/suite/a.nq",
					},
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := readManifest(strings.NewReader(prefixes+testCase.src), fixtureBase)
			if err != nil {
				t.Fatalf("readManifest() = %v, want nil", err)
			}
			if !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("readManifest() =\n%s\nwant\n%s", renderManifest(got), renderManifest(testCase.want))
			}
		})
	}
}

// TestReadManifestRejects covers the ways a manifest can fail to describe a
// test, each of which is reported rather than guessed at: a test the harness
// half understands is a test it would run wrongly.
func TestReadManifestRejects(t *testing.T) {
	testCases := []struct {
		name string
		src  string

		// wantErr is a fragment of the message the failure must carry, so that
		// whoever hits it is told which manifest is wrong and how.
		wantErr string
	}{
		{
			name:    "no manifest node",
			src:     `<#a> rdf:type rdft:TestTurtlePositiveSyntax ; mf:name "a" ; mf:action <a.ttl> .`,
			wantErr: "no node typed",
		},
		{
			name: "two manifest nodes",
			src: `
<> rdf:type mf:Manifest ; mf:entries rdf:nil .
<#other> rdf:type mf:Manifest ; mf:entries rdf:nil .
`,
			wantErr: "2 nodes typed",
		},
		{
			name:    "neither entries nor includes",
			src:     `<> rdf:type mf:Manifest ; rdfs:label "no entries" .`,
			wantErr: "has neither mf:entries nor mf:include",
		},
		{
			name:    "an include that is not an IRI",
			src:     `<> rdf:type mf:Manifest ; mf:include ( "sub/manifest.ttl" ) .`,
			wantErr: "want an IRI",
		},
		{
			name: "entries is not a list",
			src: `
<> rdf:type mf:Manifest ; mf:entries <#a> .
<#a> rdf:type rdft:TestTurtlePositiveSyntax ; mf:name "a" ; mf:action <a.ttl> .
`,
			wantErr: "has no rdf:first",
		},
		{
			name: "a list that never reaches rdf:nil",
			src: `
<> rdf:type mf:Manifest ; mf:entries _:cell .
_:cell rdf:first <#a> .
<#a> rdf:type rdft:TestTurtlePositiveSyntax ; mf:name "a" ; mf:action <a.ttl> .
`,
			wantErr: "has no rdf:rest",
		},
		{
			name: "a list that loops",
			src: `
<> rdf:type mf:Manifest ; mf:entries _:cell .
_:cell rdf:first <#a> ; rdf:rest _:cell .
<#a> rdf:type rdft:TestTurtlePositiveSyntax ; mf:name "a" ; mf:action <a.ttl> .
`,
			wantErr: "cycle",
		},
		{
			name: "an entry that is not an IRI",
			src: `
<> rdf:type mf:Manifest ; mf:entries ( [ mf:name "anonymous" ] ) .
`,
			wantErr: "is not an IRI",
		},
		{
			name: "an entry with no type",
			src: `
<> rdf:type mf:Manifest ; mf:entries ( <#a> ) .
<#a> mf:name "a" ; mf:action <a.ttl> .
`,
			wantErr: "has no rdf:type",
		},
		{
			name: "an entry with no name",
			src: `
<> rdf:type mf:Manifest ; mf:entries ( <#a> ) .
<#a> rdf:type rdft:TestTurtlePositiveSyntax ; mf:action <a.ttl> .
`,
			wantErr: "has no mf:name",
		},
		{
			name: "an entry with no action",
			src: `
<> rdf:type mf:Manifest ; mf:entries ( <#a> ) .
<#a> rdf:type rdft:TestTurtlePositiveSyntax ; mf:name "a" .
`,
			wantErr: "has no mf:action",
		},
		{
			name: "a name that is not a literal",
			src: `
<> rdf:type mf:Manifest ; mf:entries ( <#a> ) .
<#a> rdf:type rdft:TestTurtlePositiveSyntax ; mf:name <a> ; mf:action <a.ttl> .
`,
			wantErr: "want a literal",
		},
		{
			name: "an action that is not an IRI",
			src: `
<> rdf:type mf:Manifest ; mf:entries ( <#a> ) .
<#a> rdf:type rdft:TestTurtlePositiveSyntax ; mf:name "a" ; mf:action "a.ttl" .
`,
			wantErr: "want an IRI",
		},
		{
			// Two names is as unusable as none: nothing says which the test is
			// reported under.
			name: "an entry named twice",
			src: `
<> rdf:type mf:Manifest ; mf:entries ( <#a> ) .
<#a> rdf:type rdft:TestTurtlePositiveSyntax ; mf:name "a", "also a" ; mf:action <a.ttl> .
`,
			wantErr: "has no mf:name",
		},
		{
			// A manifest that will not parse is reported as the parser
			// reported it, rather than being read as far as it goes.
			name:    "a document that stops mid-statement",
			src:     `<> rdf:type mf:Manifest`,
			wantErr: "unexpected end of tokens",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := readManifest(strings.NewReader(prefixes+testCase.src), fixtureBase)
			if err == nil {
				t.Fatalf("readManifest() = %s, want an error", renderManifest(got))
			}
			if !strings.Contains(err.Error(), testCase.wantErr) {
				t.Errorf("readManifest() = %q, want it to mention %q", err, testCase.wantErr)
			}
		})
	}
}

// TestReadManifestReportsAMissingManifestNode checks the one failure the
// caller may want to tell apart: a document that is Turtle but is not a
// manifest.
func TestReadManifestReportsAMissingManifestNode(t *testing.T) {
	src := prefixes + `<#a> rdfs:label "not a manifest" .`

	_, err := readManifest(strings.NewReader(src), fixtureBase)
	if !errors.Is(err, errNoManifestNode) {
		t.Errorf("readManifest() = %v, want %v", err, errNoManifestNode)
	}
}

// TestWithBaseLeavesTheDocumentAlone checks that the base is added ahead of the
// document rather than woven into it, since everything read through it depends
// on the document arriving unchanged.
func TestWithBaseLeavesTheDocumentAlone(t *testing.T) {
	const document = "<a> <b> <c> .\n"

	got, err := io.ReadAll(withBase(strings.NewReader(document), "https://example.test/"))
	if err != nil {
		t.Fatalf("ReadAll() = %v, want nil", err)
	}

	want := "@base <https://example.test/> .\n" + document
	if string(got) != want {
		t.Errorf("withBase() = %q, want %q", got, want)
	}
}

// TestWithBaseResolvesAgainstTheBase checks the whole point of it: a document
// read through withBase says what it says relative to that base.
func TestWithBaseResolvesAgainstTheBase(t *testing.T) {
	index, err := readTurtleIndex(strings.NewReader("<s> <p> <o> .\n"), "https://example.test/dir/doc.ttl")
	if err != nil {
		t.Fatalf("readTurtleIndex() = %v, want nil", err)
	}

	object, ok := index.one(rdf.IRI("https://example.test/dir/s"), "https://example.test/dir/p")
	if !ok {
		t.Fatalf("the document does not state a triple about https://example.test/dir/s")
	}
	if want := rdf.IRI("https://example.test/dir/o"); object != rdf.Term(want) {
		t.Errorf("object = %s, want %s", object, want)
	}
}

// renderManifest writes a manifest out for a failure message, since a
// reflect.DeepEqual mismatch says nothing about where the two differ.
func renderManifest(m *manifest) string {
	if m == nil {
		return "<nil>"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "manifest %s label=%q assumedTestBase=%s", m.iri, m.label, m.assumedTestBase)
	for _, include := range m.includes {
		fmt.Fprintf(&b, "\n\tincludes %s", include)
	}
	for _, e := range m.entries {
		fmt.Fprintf(&b, "\n\t%s type=%s name=%q comment=%q action=%s result=%s",
			e.iri, e.typ, e.name, e.comment, e.action, e.result)
	}
	return b.String()
}
