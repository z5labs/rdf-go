# Vendored W3C RDF test suites

These files come from [w3c/rdf-tests](https://github.com/w3c/rdf-tests), pinned at:

```
767554e135eb6665949d870e6fa7bbc813837293
```

Update by re-extracting that repository at a new commit and changing the SHA
above and in `testsuite/conformance_test.go`, which asserts the two agree.

RDF 1.2 is a Candidate Recommendation and its suites still move, so the pin is
re-checked whenever they are extended. It was last re-synced on 2026-08-03,
which found `main` still at the commit above — every suite here, RDF 1.1 and
RDF 1.2 alike, is vendored from that one commit.

## What is vendored

The eight suites this module can run, one for each of its format packages:

| Path | Contents |
| --- | --- |
| `rdf/rdf11/rdf-n-triples` | 72 `.nt` files, `manifest.ttl`, `README` |
| `rdf/rdf11/rdf-n-quads` | 89 `.nq` files, `manifest.ttl`, `README` |
| `rdf/rdf11/rdf-turtle` | 316 `.ttl` files, 114 `.nt` results, `manifest.ttl`, `README`, `LICENSE` |
| `rdf/rdf11/rdf-trig` | 357 `.trig` files, 110 `.nq` results, `manifest.ttl`, `README`, `LICENSE` |
| `rdf/rdf12/rdf-n-triples` | `manifest.ttl`, `README.md`, `syntax/` (29 `.nt` files), `c14n/` (81 `.nt` files) |
| `rdf/rdf12/rdf-n-quads` | `manifest.ttl`, `README.md`, `syntax/` (27 `.nq` files), `c14n/` (81 `.nq` files) |
| `rdf/rdf12/rdf-turtle` | `manifest.ttl`, `README.md`, `syntax/` (75 `.ttl` files), `eval/` (29 `.ttl` files, 29 `.nt` results) |
| `rdf/rdf12/rdf-trig` | `manifest.ttl`, `README.md`, `syntax/` (36 `.trig` files), `eval/` (25 `.trig` files, 25 `.nq` results) |

Each directory is a faithful copy of the upstream one but for what is listed
below, so re-vendoring is an extraction and not an edit. A few of the files a
suite ships are named by no test in its own manifest; they are kept, and
enumerated in `unreferenced` in `testsuite/conformance_test.go`, so that a file
arriving unreferenced is noticed rather than assumed to be one of them.

An RDF 1.2 suite is three manifests rather than one: two beside each other in
subdirectories — `syntax/manifest.ttl` in all four, and then `c14n/manifest.ttl`
for N-Triples and N-Quads, whose canonical form RDF 1.2 settles, or
`eval/manifest.ttl` for Turtle and TriG, which it does not — and the
`manifest.ttl` above them, which declares no test itself and is an `mf:include`
of those two and of the RDF 1.1 suite for the same syntax. Following that last
include is why the RDF 1.1 suites are run twice: once by the RDF 1.1 parsers,
and once by the RDF 1.2 ones, which must go on reading what RDF 1.1 wrote.

## What is not, and why

- **The RDF 1.2 semantics suite, and RDF/XML.** Nothing here can read them yet.
  They arrive with the stories that can.
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
