package format

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"mime"
	"slices"
	"strings"

	rdf "github.com/z5labs/rdf-go"
	nquads11 "github.com/z5labs/rdf-go/rdf11/nquads"
	ntriples11 "github.com/z5labs/rdf-go/rdf11/ntriples"
	trig11 "github.com/z5labs/rdf-go/rdf11/trig"
	turtle11 "github.com/z5labs/rdf-go/rdf11/turtle"
	nquads12 "github.com/z5labs/rdf-go/rdf12/nquads"
	ntriples12 "github.com/z5labs/rdf-go/rdf12/ntriples"
	trig12 "github.com/z5labs/rdf-go/rdf12/trig"
	turtle12 "github.com/z5labs/rdf-go/rdf12/turtle"
)

// Errors reported when nothing in the registry answers to what a caller asked
// for. Each is wrapped with the media type, extension, syntax or version that
// was asked for, so test with [errors.Is] rather than ==.
var (
	// ErrUnknownMediaType is reported when no registered format claims a media
	// type. The media type is well formed; nothing here reads it.
	ErrUnknownMediaType = errors.New("format: unknown media type")

	// ErrUnknownExtension is reported when no registered format claims a file
	// extension.
	ErrUnknownExtension = errors.New("format: unknown file extension")

	// ErrUnknownSyntax is reported by [Lookup] for a syntax outside the four
	// this module implements.
	ErrUnknownSyntax = errors.New("format: unknown syntax")

	// ErrUnsupportedVersion is reported for a version this module does not
	// implement — a media type carrying version=1.3, say. RDF 1.1 and RDF 1.2
	// are the versions there are.
	ErrUnsupportedVersion = errors.New("format: unsupported RDF version")

	// ErrNoDecoder is reported by [Format.Decode] on the zero [Format], which
	// names no syntax and so has nothing to decode with. It is what a caller
	// sees for ignoring the error from a lookup: a description of what went
	// wrong rather than a panic on a nil decoder.
	ErrNoDecoder = errors.New("format: zero Format has no decoder")
)

// Syntax is one of the four concrete syntaxes of the Turtle family, named as
// its specification names it.
type Syntax string

// The syntaxes this module implements.
const (
	// NTriples is N-Triples, a line-based syntax for a graph.
	NTriples Syntax = "n-triples"

	// NQuads is N-Quads, N-Triples with a graph name on each line.
	NQuads Syntax = "n-quads"

	// Turtle is Turtle, the readable syntax for a graph.
	Turtle Syntax = "turtle"

	// TriG is TriG, Turtle with named graphs.
	TriG Syntax = "trig"
)

// Version is the version of the RDF specification a syntax is read against.
//
// The zero Version is [RDF11], which is what a document says by saying nothing:
// a media type with no version parameter, or a file named only by its
// extension.
type Version int

const (
	// RDF11 is RDF 1.1, https://www.w3.org/TR/rdf11-concepts/.
	RDF11 Version = iota

	// RDF12 is RDF 1.2, https://www.w3.org/TR/rdf12-concepts/, which a media
	// type selects with the parameter version=1.2.
	RDF12
)

// String returns the version as a media-type parameter writes it: "1.1" or
// "1.2".
func (v Version) String() string {
	switch v {
	case RDF11:
		return "1.1"
	case RDF12:
		return "1.2"
	default:
		return fmt.Sprintf("Version(%d)", int(v))
	}
}

// Format is one concrete syntax read at one version of the RDF specification:
// the decoder [Format.Decode] runs, and the media type and file extensions that
// name it.
//
// The zero Format is not a format. It is what a lookup returns beside an error,
// and its Decode yields [ErrNoDecoder] rather than panicking.
type Format struct {
	syntax     Syntax
	version    Version
	mediaType  string
	extensions []string
	decode     func(io.Reader) iter.Seq2[rdf.Quad, error]
}

// Syntax returns the concrete syntax the format reads.
func (f Format) Syntax() Syntax { return f.syntax }

// Version returns the version of the RDF specification the format reads its
// syntax against.
func (f Format) Version() Version { return f.version }

// MediaType returns the canonical media type of the syntax, without parameters
// — "text/turtle", never "text/turtle;version=1.2". The version a Format reads
// is [Format.Version]; a receiver told only the media type reads RDF 1.1.
func (f Format) MediaType() string { return f.mediaType }

// Extensions returns the file extensions that name the syntax, leading dot
// included. The result is a copy, so a caller may sort or truncate it without
// disturbing the registry.
//
// Both versions of a syntax answer to the same extensions, a file name having
// nowhere to say which it holds.
func (f Format) Extensions() []string { return slices.Clone(f.extensions) }

// String returns the syntax and version, as in "turtle 1.2".
func (f Format) String() string {
	if f.syntax == "" {
		return "format.Format(zero)"
	}
	return fmt.Sprintf("%s %s", f.syntax, f.version)
}

// Decode reads the document in r and yields its statements as data model quads,
// one at a time, stopping at the first error.
//
// It is the Decode of the underlying syntax package, widened to quads so that
// the four syntaxes share a signature. N-Triples and Turtle describe a graph
// rather than a dataset, and every statement they yield has a nil
// [rdf.Quad.Graph] — the default graph, not a missing value. Errors are the
// syntax package's own, and carry the position they came from.
//
// The zero Format yields [ErrNoDecoder] and nothing else.
func (f Format) Decode(r io.Reader) iter.Seq2[rdf.Quad, error] {
	if f.decode == nil {
		return func(yield func(rdf.Quad, error) bool) {
			yield(rdf.Quad{}, ErrNoDecoder)
		}
	}
	return f.decode(r)
}

// formats is the registry: each of the four syntaxes at each of the two
// versions. Eight entries are scanned rather than indexed — a map keyed by
// media type and version would have to be built at init, and a linear walk of
// eight costs less than the map does to build.
var formats = []Format{
	{
		syntax:     NTriples,
		version:    RDF11,
		mediaType:  "application/n-triples",
		extensions: []string{".nt"},
		decode:     fromTriples(ntriples11.Decode),
	},
	{
		syntax:     NTriples,
		version:    RDF12,
		mediaType:  "application/n-triples",
		extensions: []string{".nt"},
		decode:     fromTriples(ntriples12.Decode),
	},
	{
		syntax:     NQuads,
		version:    RDF11,
		mediaType:  "application/n-quads",
		extensions: []string{".nq"},
		decode:     nquads11.Decode,
	},
	{
		syntax:     NQuads,
		version:    RDF12,
		mediaType:  "application/n-quads",
		extensions: []string{".nq"},
		decode:     nquads12.Decode,
	},
	{
		syntax:     Turtle,
		version:    RDF11,
		mediaType:  "text/turtle",
		extensions: []string{".ttl"},
		decode:     fromTriples(turtle11.Decode),
	},
	{
		syntax:     Turtle,
		version:    RDF12,
		mediaType:  "text/turtle",
		extensions: []string{".ttl"},
		decode:     fromTriples(turtle12.Decode),
	},
	{
		syntax:     TriG,
		version:    RDF11,
		mediaType:  "application/trig",
		extensions: []string{".trig"},
		decode:     trig11.Decode,
	},
	{
		syntax:     TriG,
		version:    RDF12,
		mediaType:  "application/trig",
		extensions: []string{".trig"},
		decode:     trig12.Decode,
	},
}

// fromTriples widens the decoder of a graph syntax to the quad-yielding shape
// every [Format] exposes. A triple of a syntax with no graph names belongs to
// the dataset's default graph, which [rdf.Quad] writes as a nil Graph.
func fromTriples(decode func(io.Reader) iter.Seq2[rdf.Triple, error]) func(io.Reader) iter.Seq2[rdf.Quad, error] {
	return func(r io.Reader) iter.Seq2[rdf.Quad, error] {
		return func(yield func(rdf.Quad, error) bool) {
			for triple, err := range decode(r) {
				if !yield(rdf.Quad{Triple: triple}, err) {
					return
				}
			}
		}
	}
}

// Formats returns every registered format, in no particular order.
//
// The result is a copy of the registry, so a caller building an Accept header
// or a table of what it reads may sort and filter it freely.
func Formats() []Format { return slices.Clone(formats) }

// Lookup returns the format for a syntax at a version of the RDF
// specification.
//
// It is the lookup for a caller who already knows what a document holds —
// a ".ttl" written against RDF 1.2, say, which [FromExtension] cannot tell from
// any other ".ttl". It reports [ErrUnknownSyntax] or [ErrUnsupportedVersion]
// for anything outside the four syntaxes and two versions this module
// implements.
func Lookup(syntax Syntax, version Version) (Format, error) {
	for _, f := range formats {
		if f.syntax == syntax && f.version == version {
			return f, nil
		}
	}
	if version != RDF11 && version != RDF12 {
		return Format{}, fmt.Errorf("%w: %s", ErrUnsupportedVersion, version)
	}
	return Format{}, fmt.Errorf("%w: %q", ErrUnknownSyntax, string(syntax))
}

// FromMediaType returns the format for a media type, as an HTTP Content-Type
// header carries it.
//
// The media type is parsed per RFC 9110 §8.3: the type and subtype are matched
// without regard to case, surrounding white space is ignored, and parameters
// are allowed and ignored — a charset, say, this module reading UTF-8 and
// nothing else. So all of these name the same format:
//
//	text/turtle
//	 text/turtle
//	TEXT/Turtle; charset=utf-8
//
// The one parameter that is read is version, which RDF 1.2 Concepts §7 defines
// to select the version of the specification a document is written against:
// version=1.2 selects the RDF 1.2 decoder, version=1.1 the RDF 1.1 one, and no
// version parameter at all is RDF 1.1.
//
// A media type no registered format claims is [ErrUnknownMediaType], a version
// parameter naming no known version is [ErrUnsupportedVersion], and a media
// type that does not parse is the error from [mime.ParseMediaType]. The Format
// returned beside any of them is the zero Format, whose [Format.Decode] reports
// [ErrNoDecoder] rather than panicking.
func FromMediaType(mediaType string) (Format, error) {
	name, params, err := mime.ParseMediaType(mediaType)
	if err != nil {
		return Format{}, fmt.Errorf("format: parsing media type %q: %w", mediaType, err)
	}

	// ParseMediaType lower cases the type, the subtype and every parameter
	// name, so the case folding RFC 9110 §8.3.1 requires is already done.
	version := RDF11
	if s, ok := params["version"]; ok {
		version, err = parseVersion(s)
		if err != nil {
			return Format{}, err
		}
	}

	for _, f := range formats {
		if f.mediaType == name && f.version == version {
			return f, nil
		}
	}
	return Format{}, fmt.Errorf("%w: %q", ErrUnknownMediaType, name)
}

// parseVersion reads the value of a version media-type parameter. Parameter
// values are case sensitive and these have no letters in them, so the two
// spellings that mean anything are matched outright.
func parseVersion(s string) (Version, error) {
	switch s {
	case "1.1":
		return RDF11, nil
	case "1.2":
		return RDF12, nil
	default:
		return RDF11, fmt.Errorf("%w: %q", ErrUnsupportedVersion, s)
	}
}

// FromExtension returns the format for a file extension, with or without its
// leading dot and without regard to case: ".ttl", "ttl" and ".TTL" all name
// Turtle.
//
// A file name says nothing about which version of the specification its
// contents are written against, so this returns the RDF 1.1 format, the same
// default [FromMediaType] applies to a media type with no version parameter.
// A caller who knows better names the version itself with [Lookup]:
//
//	f, err := format.Lookup(format.Turtle, format.RDF12)
//
// An extension no registered format claims is [ErrUnknownExtension], beside the
// zero Format, whose [Format.Decode] reports [ErrNoDecoder] rather than
// panicking.
func FromExtension(extension string) (Format, error) {
	ext := strings.ToLower(strings.TrimSpace(extension))
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	for _, f := range formats {
		if f.version == RDF11 && slices.Contains(f.extensions, ext) {
			return f, nil
		}
	}
	return Format{}, fmt.Errorf("%w: %q", ErrUnknownExtension, extension)
}
