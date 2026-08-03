package testsuite_test

import (
	"errors"
	"fmt"
	"io"
	"strings"

	rdf "github.com/z5labs/rdf-go"
	turtle11 "github.com/z5labs/rdf-go/rdf11/turtle"
	"github.com/z5labs/rdf-go/vocab"
)

// The test manifest vocabulary, from the DAWG test manifest namespace that
// every W3C RDF suite describes itself with. Only the terms this harness reads
// are named here; a manifest says more than a test runner needs.
const namespaceMF = "http://www.w3.org/2001/sw/DataAccess/tests/test-manifest#"

const (
	// mfManifest is mf:Manifest, the class of the one node a manifest
	// describes itself with.
	mfManifest rdf.IRI = namespaceMF + "Manifest"

	// mfEntries is mf:entries, whose object is an RDF collection of the
	// manifest's tests in the order they are meant to be reported.
	mfEntries rdf.IRI = namespaceMF + "entries"

	// mfInclude is mf:include, whose object is an RDF collection of other
	// manifests this one is made of. The RDF 1.2 suites declare no test of
	// their own: their manifest is nothing but an mf:include naming the
	// syntax and canonicalization manifests beside it, and the RDF 1.1
	// manifest for the same syntax.
	mfInclude rdf.IRI = namespaceMF + "include"

	// mfName is mf:name, the test's short name. It is what the W3C reports
	// call a test, and so what a Go subtest is named after.
	mfName rdf.IRI = namespaceMF + "name"

	// mfAction is mf:action, the document under test.
	mfAction rdf.IRI = namespaceMF + "action"

	// mfResult is mf:result, the document an evaluation test must produce.
	// Syntax tests have none.
	mfResult rdf.IRI = namespaceMF + "result"

	// mfAssumedTestBase is mf:assumedTestBase, the IRI the suite's documents
	// are published under, which relative IRIs inside them resolve against.
	mfAssumedTestBase rdf.IRI = namespaceMF + "assumedTestBase"
)

// manifest is a W3C test manifest: the node typed mf:Manifest, and the tests
// its mf:entries collection lists, in order.
type manifest struct {
	// iri is the manifest node's own IRI, which is the document's base when
	// the manifest writes it as the empty relative IRI <>.
	iri rdf.IRI

	// label is the manifest's rdfs:label, empty if it carries none.
	label string

	// assumedTestBase is the manifest's mf:assumedTestBase, empty if it
	// declares none.
	assumedTestBase rdf.IRI

	// includes are the manifests this one is made of, in mf:include order,
	// as the IRIs of the documents holding them.
	includes []rdf.IRI

	// entries are the tests, in mf:entries order.
	entries []manifestEntry
}

// manifestEntry is one test in a manifest.
type manifestEntry struct {
	// iri is the test's own IRI, usually a fragment of the manifest's.
	iri rdf.IRI

	// typ is the test's rdf:type, one of the rdft: test classes. It is what
	// the harness dispatches on.
	typ rdf.IRI

	// name is the test's mf:name, which the W3C suite reports it under.
	name string

	// comment is the test's rdfs:comment, empty if it carries none.
	comment string

	// action is the document under test.
	action rdf.IRI

	// result is the document an evaluation test must produce, empty for a
	// syntax test.
	result rdf.IRI
}

// readManifest reads a manifest document, resolving its relative IRIs against
// base, which is the IRI the manifest itself is published under.
//
// The manifest is Turtle, and this module's own Turtle parser reads it. That
// is a bootstrap risk — a parser bug could make a manifest say something other
// than what it says, and so make a suite look like it passes — which is why
// this function is exercised against hand-checked fixtures in
// TestReadManifest before any vendored manifest is trusted.
func readManifest(r io.Reader, base string) (*manifest, error) {
	index, err := readTurtleIndex(r, base)
	if err != nil {
		return nil, err
	}

	node, err := index.soleSubjectOfType(mfManifest)
	if err != nil {
		return nil, err
	}
	iri, ok := node.(rdf.IRI)
	if !ok {
		return nil, fmt.Errorf("manifest: mf:Manifest node is %s, want an IRI", node)
	}

	m := &manifest{iri: iri}

	if label, ok, err := index.optionalLiteral(node, vocab.RDFSLabel); err != nil {
		return nil, err
	} else if ok {
		m.label = label
	}

	if assumed, ok, err := index.optionalIRI(node, mfAssumedTestBase); err != nil {
		return nil, err
	} else if ok {
		m.assumedTestBase = assumed
	}

	includes, hasIncludes := index.one(node, mfInclude)
	if hasIncludes {
		list, err := index.collection(includes)
		if err != nil {
			return nil, err
		}
		for _, item := range list {
			included, ok := item.(rdf.IRI)
			if !ok {
				return nil, fmt.Errorf("manifest: %s includes %s, want an IRI", iri, item)
			}
			m.includes = append(m.includes, included)
		}
	}

	entries, hasEntries := index.one(node, mfEntries)
	if !hasEntries {
		// A manifest made only of others declares no test itself, which is
		// what the RDF 1.2 manifests are. One declaring neither is not a
		// manifest of anything, and is reported rather than run as empty.
		if hasIncludes {
			return m, nil
		}
		return nil, fmt.Errorf("manifest: %s has neither mf:entries nor mf:include, so it declares no tests", iri)
	}
	list, err := index.collection(entries)
	if err != nil {
		return nil, err
	}

	for _, item := range list {
		entry, err := index.entry(item)
		if err != nil {
			return nil, err
		}
		m.entries = append(m.entries, entry)
	}
	return m, nil
}

// entry reads the test node, which every field of is required but rdfs:comment
// and mf:result. A manifest that leaves one out is reported rather than
// guessed at: a test that cannot be described cannot be run.
func (ix turtleIndex) entry(node rdf.Term) (manifestEntry, error) {
	iri, ok := node.(rdf.IRI)
	if !ok {
		return manifestEntry{}, fmt.Errorf("manifest: entry %s is not an IRI", node)
	}
	e := manifestEntry{iri: iri}

	typ, ok := ix.one(node, vocab.RDFType)
	if !ok {
		return e, fmt.Errorf("manifest: %s has no rdf:type", iri)
	}
	if e.typ, ok = typ.(rdf.IRI); !ok {
		return e, fmt.Errorf("manifest: %s has rdf:type %s, want an IRI", iri, typ)
	}

	name, ok, err := ix.optionalLiteral(node, mfName)
	if err != nil {
		return e, err
	}
	if !ok {
		return e, fmt.Errorf("manifest: %s has no mf:name", iri)
	}
	e.name = name

	if e.comment, _, err = ix.optionalLiteral(node, vocab.RDFSComment); err != nil {
		return e, err
	}

	action, ok, err := ix.optionalIRI(node, mfAction)
	if err != nil {
		return e, err
	}
	if !ok {
		return e, fmt.Errorf("manifest: %s has no mf:action", iri)
	}
	e.action = action

	if e.result, _, err = ix.optionalIRI(node, mfResult); err != nil {
		return e, err
	}
	return e, nil
}

// turtleIndex is a parsed Turtle document arranged for lookup by subject and
// predicate, which is what reading a manifest is: following named properties
// out from one node.
//
// A graph is a set, so the objects of a subject and predicate are unordered.
// Order is recovered only where RDF gives it: an rdf:first/rdf:rest chain,
// which [turtleIndex.collection] walks.
type turtleIndex map[rdf.Term]map[rdf.IRI][]rdf.Term

// readTurtleIndex decodes a Turtle document and indexes its triples.
func readTurtleIndex(r io.Reader, base string) (turtleIndex, error) {
	index := make(turtleIndex)
	for triple, err := range turtle11.Decode(withBase(r, base)) {
		if err != nil {
			return nil, err
		}
		byPredicate, ok := index[triple.Subject]
		if !ok {
			byPredicate = make(map[rdf.IRI][]rdf.Term)
			index[triple.Subject] = byPredicate
		}
		byPredicate[triple.Predicate] = append(byPredicate[triple.Predicate], triple.Object)
	}
	return index, nil
}

// one returns the single object of subject and predicate, and whether there
// was exactly one. Every property this harness reads is single-valued, so more
// than one object is as unusable as none.
func (ix turtleIndex) one(subject rdf.Term, predicate rdf.IRI) (rdf.Term, bool) {
	objects := ix[subject][predicate]
	if len(objects) != 1 {
		return nil, false
	}
	return objects[0], true
}

// optionalLiteral returns the lexical form of a single literal object, and
// whether the property is present. A present property whose object is not a
// literal is an error rather than an absence.
func (ix turtleIndex) optionalLiteral(subject rdf.Term, predicate rdf.IRI) (string, bool, error) {
	object, ok := ix.one(subject, predicate)
	if !ok {
		return "", false, nil
	}
	literal, ok := object.(rdf.Literal)
	if !ok {
		return "", false, fmt.Errorf("manifest: %s %s %s, want a literal", subject, predicate, object)
	}
	return literal.Value(), true, nil
}

// optionalIRI returns a single IRI object, and whether the property is
// present. A present property whose object is not an IRI is an error.
func (ix turtleIndex) optionalIRI(subject rdf.Term, predicate rdf.IRI) (rdf.IRI, bool, error) {
	object, ok := ix.one(subject, predicate)
	if !ok {
		return "", false, nil
	}
	iri, ok := object.(rdf.IRI)
	if !ok {
		return "", false, fmt.Errorf("manifest: %s %s %s, want an IRI", subject, predicate, object)
	}
	return iri, true, nil
}

// errNoManifestNode is reported when a document holds no mf:Manifest node, and
// so is not a manifest at all.
var errNoManifestNode = errors.New("manifest: no node typed mf:Manifest")

// soleSubjectOfType returns the one node the document types class.
func (ix turtleIndex) soleSubjectOfType(class rdf.IRI) (rdf.Term, error) {
	var found []rdf.Term
	for subject, byPredicate := range ix {
		for _, object := range byPredicate[vocab.RDFType] {
			if iri, ok := object.(rdf.IRI); ok && iri == class {
				found = append(found, subject)
			}
		}
	}
	switch len(found) {
	case 0:
		return nil, errNoManifestNode
	case 1:
		return found[0], nil
	default:
		return nil, fmt.Errorf("manifest: %d nodes typed %s, want one", len(found), class)
	}
}

// collection walks the rdf:first/rdf:rest chain starting at head and returns
// the items it holds, in order (RDF Schema 1.1 §5.2).
//
// A chain that loops back on itself is reported rather than followed forever:
// nothing stops a graph from describing one, and a manifest that does is
// broken rather than infinite.
func (ix turtleIndex) collection(head rdf.Term) ([]rdf.Term, error) {
	var items []rdf.Term
	seen := make(map[rdf.Term]bool)

	for {
		if iri, ok := head.(rdf.IRI); ok && iri == vocab.RDFNil {
			return items, nil
		}
		if seen[head] {
			return nil, fmt.Errorf("manifest: collection cell %s repeats, so the list has a cycle", head)
		}
		seen[head] = true

		first, ok := ix.one(head, vocab.RDFFirst)
		if !ok {
			return nil, fmt.Errorf("manifest: collection cell %s has no rdf:first", head)
		}
		items = append(items, first)

		if head, ok = ix.one(head, vocab.RDFRest); !ok {
			return nil, fmt.Errorf("manifest: collection cell %s has no rdf:rest, so the list never reaches rdf:nil", head)
		}
	}
}

// withBase gives a Turtle or TriG document the base IRI it is published under,
// by prepending the @base directive that says so.
//
// A document fetched from a URL is parsed with that URL as its base (RFC 3986
// §5.1.3), and the W3C suites lean on it: an evaluation test whose action
// holds a relative IRI expects it resolved against the test's own IRI. This
// module's parsers take a document and nothing else, so the base is written
// into the document instead. The two are equivalent — @base sets the base in
// scope for everything after it, and a document's own @base or BASE overrides
// it exactly as one written later overrides one written earlier (Turtle §6.1).
//
// N-Triples and N-Quads need none of this: their grammars have no base
// directive and no relative IRIs, so a base has nothing to resolve.
func withBase(r io.Reader, base string) io.Reader {
	return io.MultiReader(strings.NewReader("@base <"+base+"> .\n"), r)
}
