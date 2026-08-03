# Vendored W3C RDF test suites

These files come from [w3c/rdf-tests](https://github.com/w3c/rdf-tests), pinned at:

```
767554e135eb6665949d870e6fa7bbc813837293
```

Update by re-extracting that repository at a new commit and changing the SHA
above and in `testsuite/conformance_test.go`, which asserts the two agree.

## What is vendored

Only the two suites this module can run today:

| Path | Contents |
| --- | --- |
| `rdf/rdf11/rdf-n-triples` | 72 `.nt` files, `manifest.ttl`, `README` |
| `rdf/rdf11/rdf-n-quads` | 89 `.nq` files, `manifest.ttl`, `README` |

## What is not, and why

- **Every other suite.** Turtle, TriG, RDF/XML and the RDF 1.2 suites are not
  vendored, because nothing here can read them yet. They arrive with the
  stories that can.
- **`TESTS.tar.gz` and `TESTS.zip`.** Archives of the same files that sit
  beside them, unpacked.
- **`reports/`.** Conformance reports submitted by other implementations —
  2.4 MB of HTML and JSON-LD saying how other parsers did, which is no part of
  how this one does.

## `manifest.ttl` is vendored but not read

The manifests are Turtle, which this module cannot parse yet — that is the
bootstrap problem this harness exists to work around, and the reason it walks
the directories instead. They are kept so that the manifest-driven harness can
use them without re-vendoring anything, and so that a reader can check what the
suite says a test is meant to do.
