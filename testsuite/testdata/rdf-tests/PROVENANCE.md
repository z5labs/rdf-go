# Vendored W3C RDF test suites

These files come from [w3c/rdf-tests](https://github.com/w3c/rdf-tests), pinned at:

```
767554e135eb6665949d870e6fa7bbc813837293
```

Update by re-extracting that repository at a new commit and changing the SHA
above and in `testsuite/conformance_test.go`, which asserts the two agree.

## What is vendored

The four RDF 1.1 suites this module can run:

| Path | Contents |
| --- | --- |
| `rdf/rdf11/rdf-n-triples` | 72 `.nt` files, `manifest.ttl`, `README` |
| `rdf/rdf11/rdf-n-quads` | 89 `.nq` files, `manifest.ttl`, `README` |
| `rdf/rdf11/rdf-turtle` | 316 `.ttl` files, 114 `.nt` results, `manifest.ttl`, `README`, `LICENSE` |
| `rdf/rdf11/rdf-trig` | 357 `.trig` files, 110 `.nq` results, `manifest.ttl`, `README`, `LICENSE` |

Each directory is a faithful copy of the upstream one but for what is listed
below, so re-vendoring is an extraction and not an edit. A few of the files a
suite ships are named by no test in its own manifest; they are kept, and
enumerated in `unreferenced` in `testsuite/conformance_test.go`, so that a file
arriving unreferenced is noticed rather than assumed to be one of them.

## What is not, and why

- **The RDF 1.2 suites, and RDF/XML.** Nothing here can read them yet. They
  arrive with the stories that can.
- **`TESTS.tar.gz` and `TESTS.zip`.** Archives of the same files that sit
  beside them, unpacked.
- **`reports/`.** Conformance reports submitted by other implementations — 16
  MB of HTML and JSON-LD saying how other parsers did, which is no part of how
  this one does.

## `manifest.ttl` is what runs the suite

Each suite describes itself in its `manifest.ttl`: what tests there are, what
each is called, which document it reads, and what that document must mean. The
harness runs the suites from those manifests, reading them with this module's
own Turtle parser — see the package documentation in `testsuite/doc.go` for
what that circularity costs and what holds it in place.
