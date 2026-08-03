// Package iri resolves relative IRI references against a base IRI, as
// RFC 3986 §5 specifies for URIs and RFC 3987 §6.5 carries over to IRIs.
//
// Turtle and TriG cannot be parsed without this: both let a document set a
// base and write every IRI after it relative to that base. N-Triples has no
// such directive and needs none of this.
//
// # IRIs rather than URIs
//
// The algorithm is RFC 3986's, applied to the IRI itself rather than to a
// percent-encoded form of it. That is what RFC 3987 §6.5 calls for, and it is
// safe here because every character the algorithm looks at — ':', '/', '?',
// '#' and '.' — is ASCII, and no byte of a multi-byte UTF-8 sequence is ever
// ASCII. Characters outside the ASCII range are carried through untouched.
//
// # What is not checked
//
// Neither argument is validated beyond the one thing resolution cannot do
// without: a base with a scheme. Whether the result is a well-formed IRI is
// the caller's business, because a parser rejecting a malformed IRI wants to
// do so at its own boundary, where it can report a position.
package iri

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
)

// ErrRelativeBase is reported when the base has no scheme. Resolution is
// defined only against an absolute base (RFC 3986 §5.1), since a reference
// that supplies no scheme of its own has nowhere else to take one from.
var ErrRelativeBase = errors.New("iri: base must be absolute")

// Resolve resolves the reference ref against the base IRI base, returning the
// target IRI. It implements the reference transformation of RFC 3986 §5.2.2,
// with the merge of §5.2.3 and the dot segment removal of §5.2.4.
//
// The reference may be of any form the grammar allows, and each is handled as
// the RFC prescribes:
//
//	iri.Resolve("http://a/b/c/d;p?q", "g")     // "http://a/b/c/g"
//	iri.Resolve("http://a/b/c/d;p?q", "")      // "http://a/b/c/d;p?q"
//	iri.Resolve("http://a/b/c/d;p?q", "#s")    // "http://a/b/c/d;p?q#s"
//	iri.Resolve("http://a/b/c/d;p?q", "?y")    // "http://a/b/c/d;p?y"
//	iri.Resolve("http://a/b/c/d;p?q", "//g")   // "http://g"
//	iri.Resolve("http://a/b/c/d;p?q", "../..") // "http://a/"
//
// A reference carrying its own scheme is returned as it stands, save that dot
// segments are removed from its path — §5.2.2 applies remove_dot_segments in
// that branch too, so "http://a/b/../c" resolves to "http://a/c" however
// absolute it already was.
//
// It reports [ErrRelativeBase] if base has no scheme. A fragment on base is
// ignored rather than rejected: the target always takes its fragment from the
// reference, so a base fragment could not contribute to the result anyway.
//
// # Strict resolution
//
// This is the strict algorithm. §5.2.2 notes that some parsers, for the sake
// of "erroneous references relying on a parser's habit of ignoring a scheme
// equal to the base's", first discard a scheme that matches the base's. That
// backward-compatible reading turns "http:g" against a base of "http://a/b/c/"
// into "http://a/b/c/g"; the strict one leaves it as "http:g". RFC 3986 gives
// the strict result in its own examples, and it is the one the W3C Turtle
// evaluation tests expect.
func Resolve(base, ref string) (string, error) {
	b := parse(base)
	if !b.hasScheme {
		return "", fmt.Errorf("%w: %q has no scheme", ErrRelativeBase, base)
	}
	return transform(b, parse(ref)).String(), nil
}

// reference is a IRI reference split into the five components of RFC 3986 §3.
//
// Each of the components that the grammar lets be absent as well as empty
// carries a flag saying which it is, because the algorithm turns on the
// difference: a reference of "//host" has an authority and an empty path,
// while "path" has no authority at all, and "g?" carries an empty query where
// "g" carries none. The path alone needs no flag, being always present and
// possibly empty.
type reference struct {
	scheme    string
	authority string
	path      string
	query     string
	fragment  string

	hasScheme    bool
	hasAuthority bool
	hasQuery     bool
	hasFragment  bool
}

// parse splits an IRI reference into its components, following the order the
// grammar of RFC 3986 §3 nests them: the fragment ends the reference, the
// query ends what precedes it, a scheme begins it, and an authority follows
// the "//" that can only start the remainder.
//
// It cannot fail. Anything that is not a scheme, an authority or a query is a
// path, which admits nearly everything, so an input that is no IRI at all
// still parses — into components that are simply not worth much.
func parse(s string) reference {
	var r reference

	if before, after, found := strings.Cut(s, "#"); found {
		s, r.fragment, r.hasFragment = before, after, true
	}
	if before, after, found := strings.Cut(s, "?"); found {
		s, r.query, r.hasQuery = before, after, true
	}
	if scheme, after, found := cutScheme(s); found {
		s, r.scheme, r.hasScheme = after, scheme, true
	}
	if rest, found := strings.CutPrefix(s, "//"); found {
		authority, path, _ := cutAuthority(rest)
		s, r.authority, r.hasAuthority = path, authority, true
	}
	r.path = s

	return r
}

// cutScheme splits a leading scheme from s, reporting whether there was one.
//
// A scheme is what stands before the first colon, but only if it is one:
//
//	scheme = ALPHA *( ALPHA / DIGIT / "+" / "-" / "." )
//
// A colon appearing anywhere else belongs to a path segment. "2011:a" has no
// scheme because a scheme may not begin with a digit, and neither has
// "a/b:c", because a scheme may not contain a slash and so cannot reach that
// colon.
func cutScheme(s string) (scheme, rest string, found bool) {
	colon := strings.IndexByte(s, ':')
	if colon <= 0 {
		return "", s, false
	}

	for i := range colon {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case i == 0:
			// The first character must be a letter; a digit, '+', '-' or '.'
			// is only allowed after one.
			return "", s, false
		case c >= '0' && c <= '9', c == '+', c == '-', c == '.':
		default:
			return "", s, false
		}
	}
	return s[:colon], s[colon+1:], true
}

// cutAuthority splits the authority from the path that follows it. The
// authority runs to the first "/", which begins the path, or to the end of the
// input, leaving the path empty (RFC 3986 §3.2).
func cutAuthority(s string) (authority, path string, found bool) {
	if i := strings.IndexByte(s, '/'); i >= 0 {
		return s[:i], s[i:], true
	}
	return s, "", false
}

// String recomposes the components into an IRI, following RFC 3986 §5.3.
func (r reference) String() string {
	var b strings.Builder
	b.Grow(len(r.scheme) + len(r.authority) + len(r.path) + len(r.query) + len(r.fragment) + 4)

	if r.hasScheme {
		b.WriteString(r.scheme)
		b.WriteByte(':')
	}
	if r.hasAuthority {
		b.WriteString("//")
		b.WriteString(r.authority)
	}
	b.WriteString(r.path)
	if r.hasQuery {
		b.WriteByte('?')
		b.WriteString(r.query)
	}
	if r.hasFragment {
		b.WriteByte('#')
		b.WriteString(r.fragment)
	}
	return b.String()
}

// transform resolves ref against base, following the strict form of the
// pseudocode in RFC 3986 §5.2.2.
//
// The cases run from the most information the reference supplies to the least.
// Whatever it does not supply is taken from the base, and the target's
// fragment always comes from the reference — which is why a base fragment can
// never reach the result.
func transform(base, ref reference) reference {
	var t reference

	switch {
	case ref.hasScheme:
		t.scheme, t.hasScheme = ref.scheme, true
		t.authority, t.hasAuthority = ref.authority, ref.hasAuthority
		t.path = removeDotSegments(ref.path)
		t.query, t.hasQuery = ref.query, ref.hasQuery

	case ref.hasAuthority:
		t.scheme, t.hasScheme = base.scheme, base.hasScheme
		t.authority, t.hasAuthority = ref.authority, true
		t.path = removeDotSegments(ref.path)
		t.query, t.hasQuery = ref.query, ref.hasQuery

	case ref.path == "":
		// A reference with no path at all — "", "?y" or "#s" — points at the
		// base document itself, so it keeps the base's path, and its query too
		// unless it brought one.
		t.scheme, t.hasScheme = base.scheme, base.hasScheme
		t.authority, t.hasAuthority = base.authority, base.hasAuthority
		t.path = base.path
		if ref.hasQuery {
			t.query, t.hasQuery = ref.query, true
		} else {
			t.query, t.hasQuery = base.query, base.hasQuery
		}

	default:
		t.scheme, t.hasScheme = base.scheme, base.hasScheme
		t.authority, t.hasAuthority = base.authority, base.hasAuthority
		if strings.HasPrefix(ref.path, "/") {
			t.path = removeDotSegments(ref.path)
		} else {
			t.path = removeDotSegments(merge(base, ref.path))
		}
		t.query, t.hasQuery = ref.query, ref.hasQuery
	}

	t.fragment, t.hasFragment = ref.fragment, ref.hasFragment
	return t
}

// merge joins a relative path onto the base's, following RFC 3986 §5.2.3: the
// reference replaces everything after the base's last "/", which is the sense
// in which a relative reference is relative to the document rather than to the
// directory it appears to name.
//
// A base that has an authority but no path is treated as though its path were
// "/", since "http://a" and "http://a/" name the same thing and only the
// latter can have anything merged onto it.
func merge(base reference, path string) string {
	if base.hasAuthority && base.path == "" {
		return "/" + path
	}
	if i := strings.LastIndexByte(base.path, '/'); i >= 0 {
		return base.path[:i+1] + path
	}
	return path
}

// removeDotSegments interprets the "." and ".." segments of a path, following
// the loop of RFC 3986 §5.2.4 step by step.
//
// Segments are moved one at a time from the front of the input to the end of
// the output, where ".." undoes the last move. Traversing past the root is not
// an error: the RFC has the output simply stay empty, so "/../g" against any
// base is "/g" rather than a failure. Only whole segments count — "..g" and
// "g." are ordinary names, not traversals.
func removeDotSegments(path string) string {
	var out []byte

	for path != "" {
		switch {
		// A: a leading "../" or "./" cannot go anywhere, there being no output
		// yet to go back through, so it is simply dropped.
		case strings.HasPrefix(path, "../"):
			path = path[len("../"):]
		case strings.HasPrefix(path, "./"):
			path = path[len("./"):]

		// B: "/./" and a trailing "/." both mean "stay here".
		case strings.HasPrefix(path, "/./"):
			path = path[len("/."):]
		case path == "/.":
			path = "/"

		// C: "/../" and a trailing "/.." go back through the last segment
		// moved to the output.
		case strings.HasPrefix(path, "/../"):
			path = path[len("/.."):]
			out = truncateLastSegment(out)
		case path == "/..":
			path = "/"
			out = truncateLastSegment(out)

		// D: a path that is nothing but "." or ".." is dropped whole.
		case path == "." || path == "..":
			path = ""

		// E: anything else begins with a real segment, which moves across
		// along with its leading "/" if it has one.
		default:
			end := 0
			if path[0] == '/' {
				end = 1
			}
			if i := strings.IndexByte(path[end:], '/'); i >= 0 {
				end += i
			} else {
				end = len(path)
			}
			out = append(out, path[:end]...)
			path = path[end:]
		}
	}

	return string(out)
}

// truncateLastSegment removes the last segment from the output buffer, along
// with the "/" that introduced it. An output with no "/" left is emptied,
// which is how traversal past the root comes to rest rather than fail.
func truncateLastSegment(out []byte) []byte {
	if i := bytes.LastIndexByte(out, '/'); i >= 0 {
		return out[:i]
	}
	return out[:0]
}
