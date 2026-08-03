// Package testsuite runs the W3C RDF test suites against this module.
//
// It has no API. Everything here is a test, and the package exists so that
// `go test ./...` runs the suites alongside the unit tests.
//
// # Why this harness walks directories
//
// Each W3C suite describes itself in a manifest.ttl, which says what every
// test file is and what a conforming parser should do with it. Reading that
// would be the right way to run a suite — and it is Turtle, which this module
// cannot parse yet.
//
// So this harness works out what a test is from its name instead, which the
// two RDF 1.1 suites it runs make possible: they hold syntax tests only, and
// a negative one is named for it. The manifest-driven harness replaces this
// one once Turtle 1.1 lands, and can then run the suites that need more than
// a filename to describe them.
package testsuite
