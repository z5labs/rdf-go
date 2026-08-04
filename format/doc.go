// Package format picks a decoder for an RDF document from how the document
// arrived — the media type an HTTP response labelled it with, or the extension
// on the file it was read from — so that a caller handling documents it did not
// write does not have to keep a switch over the four syntaxes of its own.
//
// [FromMediaType] and [FromExtension] each return a [Format]: one concrete
// syntax at one version of the RDF specification, with a [Format.Decode] that
// reads a document of that syntax. Every Format decodes to [rdf.Quad], whatever
// its syntax says about graphs, so that the four are interchangeable at the
// call site:
//
//	f, err := format.FromMediaType(resp.Header.Get("Content-Type"))
//	if err != nil {
//		return err
//	}
//	for quad, err := range f.Decode(resp.Body) {
//		// ...
//	}
//
// N-Triples and Turtle describe a graph rather than a dataset, and their
// statements arrive as quads in the default graph — a nil [rdf.Quad.Graph],
// which is the default graph rather than a missing value.
//
// # Versions
//
// The registry holds every syntax twice, once for RDF 1.1 and once for RDF 1.2,
// because the two are separate grammars in separate packages. A media type
// chooses between them with the version parameter RDF 1.2 Concepts §7 defines:
//
//	application/n-triples;version=1.2
//
// A media type carrying no version parameter is RDF 1.1, and so is a file
// extension, which has no room to say otherwise. That default is the
// conservative reading rather than an arbitrary one: RDF 1.2 adds triple terms,
// reifiers and base directions to the grammars, so a 1.1 decoder refuses a 1.2
// document with a parse error rather than reading it wrongly, while a 1.2
// decoder would silently accept syntax the sender never claimed to be writing.
//
// [Lookup] names a syntax and a version outright, for a caller who knows what a
// file holds despite what it is called.
package format
