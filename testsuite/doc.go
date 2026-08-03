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
// document to read (mf:action) and, for an evaluation test, what that document
// must mean (mf:result).
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
// relative IRI resolved against the manifest's own IRI, a property left out, a
// property given twice, a list that never ends. A parser wrong enough to
// misread a vendored suite fails one of those first.
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
package testsuite
