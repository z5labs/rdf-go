// Package testsuite runs the W3C RDF test suites against this module.
//
// It has no API. Everything here is a test, and the package exists so that
// `go test ./...` runs the suites alongside the unit tests.
//
// # What a suite says about itself
//
// Each W3C suite describes itself in a manifest.ttl: one node typed
// mf:Manifest, whose mf:entries is a collection of the suite's tests in order,
// each of which says what it is (rdf:type), what it is called (mf:name), what
// document to read (mf:action) and, for an evaluation or canonicalization
// test, what that document must produce (mf:result). A manifest may instead —
// or as well — be made of others, its mf:include naming them, which is what
// the RDF 1.2 suites are.
//
// The harness reads that, and only that. A test runs because the manifest
// declares it, is named as the manifest names it — so a failure can be looked
// up directly in the W3C suite — and is dispatched on the rdft: class the
// manifest gives it. A class the harness does not know fails the suite rather
// than being passed over, because the suites grow, and a test that is never
// run is worse than one that fails.
//
// An earlier harness worked out what a test was from its filename, which was
// all that was possible before this module could parse Turtle. It could run
// syntax tests and nothing else. This one runs evaluation tests too, and so
// checks what a document means rather than only that it parses.
//
// # One suite, one version of the specifications
//
// A suite is run by the parsers of the RDF version it is a suite for: the
// rdf11 packages read the RDF 1.1 suites and the rdf12 packages the RDF 1.2
// ones. That is what the versioned package split is for, and the harness
// checks it holds in both directions.
//
// Outwards, an RDF 1.2 parser must go on reading RDF 1.1: the RDF 1.2
// manifests include the RDF 1.1 suite for the same syntax, so running an RDF
// 1.2 suite runs the RDF 1.1 one again through the newer parsers.
//
// Inwards, an RDF 1.1 parser must refuse RDF 1.2, and no W3C suite checks that
// — a suite tests one version at a time. TestRDF11ParsersRefuseRDF12Syntax
// does: every document an RDF 1.2 suite expects to be read is handed to the
// RDF 1.1 parser for its syntax, and must be refused where it is written with
// something RDF 1.2 added and read where it is not. Which of the two a
// document is is written out in rdf12Only rather than worked out, so that a
// document changing under the harness fails rather than quietly changing what
// is asserted about it.
//
// # Canonicalization tests
//
// RDF 1.2 settles how N-Triples and N-Quads are written, and its suites test
// it: a canonicalization test reads a document and compares what the encoder
// writes back with the expected result as text, not as a graph. What that
// leaves open is blank node labels, which canonical form does not fix and the
// encoders do not either, so the two tests whose documents name a blank node
// are skipped with that as the reason.
//
// # Reading a manifest with the parser it tests
//
// A manifest is Turtle, and the parser that reads it is this module's own.
// That is a circle: a parser that misreads a manifest can make a suite look
// like it passes, and the suite is what would otherwise have caught the
// parser. Nothing inside the module can break the circle — a second parser
// would only move the question to which of the two is right.
//
// What the circle is anchored by is a set of manifests written out by hand,
// small enough to check by eye, with what each one means written beside it as
// a Go value. They are in manifest_reader_test.go, and they cover what the
// vendored manifests rest on: the order a collection gives its items, a
// relative IRI resolved against the manifest's own IRI, a manifest made of
// others, a property left out, a property given twice, a list that never ends.
// A parser wrong enough to misread a vendored suite fails one of those first.
//
// # Base IRIs
//
// The suites are published under a base IRI and lean on it: an evaluation test
// whose document holds a relative IRI expects it resolved against the test's
// own IRI, and several tests are about nothing else. This module's parsers
// take a document and no base, so the harness writes the base into the
// document as the @base directive that says the same thing — see withBase in
// manifest_test.go.
//
// # What is not run
//
// Skips are enumerated, with a reason each, in the skipped map in
// conformance_test.go, and a skip naming a test no manifest declares is a
// failure of its own. Files a suite ships that its manifest names no test for
// are enumerated the same way, in unreferenced.
//
// A test is keyed there by its own IRI rather than by its mf:name, which is
// not unique: the vendored suites hold three pairs of tests sharing a name,
// and a key naming one of them would silently mean the other as well.
package testsuite
