package iri_test

import (
	"errors"
	"testing"

	"github.com/z5labs/rdf-go/iri"
)

// rfcBase is the base URI RFC 3986 §5.4 uses for both of its example tables.
const rfcBase = "http://a/b/c/d;p?q"

// TestResolveNormalExamples is the table of RFC 3986 §5.4.1, transcribed
// verbatim and in the order the RFC gives it.
func TestResolveNormalExamples(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{ref: "g:h", want: "g:h"},
		{ref: "g", want: "http://a/b/c/g"},
		{ref: "./g", want: "http://a/b/c/g"},
		{ref: "g/", want: "http://a/b/c/g/"},
		{ref: "/g", want: "http://a/g"},
		{ref: "//g", want: "http://g"},
		{ref: "?y", want: "http://a/b/c/d;p?y"},
		{ref: "g?y", want: "http://a/b/c/g?y"},
		{ref: "#s", want: "http://a/b/c/d;p?q#s"},
		{ref: "g#s", want: "http://a/b/c/g#s"},
		{ref: "g?y#s", want: "http://a/b/c/g?y#s"},
		{ref: ";x", want: "http://a/b/c/;x"},
		{ref: "g;x", want: "http://a/b/c/g;x"},
		{ref: "g;x?y#s", want: "http://a/b/c/g;x?y#s"},
		{ref: "", want: "http://a/b/c/d;p?q"},
		{ref: ".", want: "http://a/b/c/"},
		{ref: "./", want: "http://a/b/c/"},
		{ref: "..", want: "http://a/b/"},
		{ref: "../", want: "http://a/b/"},
		{ref: "../g", want: "http://a/b/g"},
		{ref: "../..", want: "http://a/"},
		{ref: "../../", want: "http://a/"},
		{ref: "../../g", want: "http://a/g"},
	}

	for _, test := range tests {
		t.Run(quoted(test.ref), func(t *testing.T) {
			got, err := iri.Resolve(rfcBase, test.ref)
			if err != nil {
				t.Fatalf("Resolve() = %v, want nil", err)
			}
			if got != test.want {
				t.Errorf("Resolve() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestResolveAbnormalExamples is the table of RFC 3986 §5.4.2, transcribed
// verbatim. These are the cases a hand-rolled resolver gets wrong: traversal
// past the root, names that merely look like dot segments, and dot segments in
// a query or fragment, where they are ordinary characters.
func TestResolveAbnormalExamples(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{ref: "../../../g", want: "http://a/g"},
		{ref: "../../../../g", want: "http://a/g"},

		{ref: "/./g", want: "http://a/g"},
		{ref: "/../g", want: "http://a/g"},
		{ref: "g.", want: "http://a/b/c/g."},
		{ref: ".g", want: "http://a/b/c/.g"},
		{ref: "g..", want: "http://a/b/c/g.."},
		{ref: "..g", want: "http://a/b/c/..g"},

		{ref: "./../g", want: "http://a/b/g"},
		{ref: "./g/.", want: "http://a/b/c/g/"},
		{ref: "g/./h", want: "http://a/b/c/g/h"},
		{ref: "g/../h", want: "http://a/b/c/h"},
		{ref: "g;x=1/./y", want: "http://a/b/c/g;x=1/y"},
		{ref: "g;x=1/../y", want: "http://a/b/c/y"},

		{ref: "g?y/./x", want: "http://a/b/c/g?y/./x"},
		{ref: "g?y/../x", want: "http://a/b/c/g?y/../x"},
		{ref: "g#s/./x", want: "http://a/b/c/g#s/./x"},
		{ref: "g#s/../x", want: "http://a/b/c/g#s/../x"},

		// §5.4.2 gives both readings of this one. The strict result is the
		// one below; a parser in backward-compatible mode would instead say
		// "http://a/b/c/g".
		{ref: "http:g", want: "http:g"},
	}

	for _, test := range tests {
		t.Run(quoted(test.ref), func(t *testing.T) {
			got, err := iri.Resolve(rfcBase, test.ref)
			if err != nil {
				t.Fatalf("Resolve() = %v, want nil", err)
			}
			if got != test.want {
				t.Errorf("Resolve() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestResolveReferenceForms walks the forms of reference a Turtle document may
// write, named for what they are rather than for the RFC line they come from.
func TestResolveReferenceForms(t *testing.T) {
	tests := []struct {
		name string
		base string
		ref  string
		want string
	}{
		{
			name: "the empty reference names the base document",
			base: "http://example.com/dir/doc.ttl",
			ref:  "",
			want: "http://example.com/dir/doc.ttl",
		},
		{
			name: "the empty reference keeps the base query",
			base: "http://example.com/doc?a=1",
			ref:  "",
			want: "http://example.com/doc?a=1",
		},
		{
			name: "a fragment only reference hangs off the base document",
			base: "http://example.com/dir/doc.ttl",
			ref:  "#frag",
			want: "http://example.com/dir/doc.ttl#frag",
		},
		{
			name: "a fragment only reference replaces a base fragment",
			base: "http://example.com/doc#old",
			ref:  "#new",
			want: "http://example.com/doc#new",
		},
		{
			name: "an empty fragment is kept, being distinct from none",
			base: "http://example.com/doc",
			ref:  "#",
			want: "http://example.com/doc#",
		},
		{
			name: "a query only reference replaces the base query",
			base: "http://example.com/doc?old=1",
			ref:  "?new=2",
			want: "http://example.com/doc?new=2",
		},
		{
			name: "an empty query is kept, being distinct from none",
			base: "http://example.com/doc?old=1",
			ref:  "?",
			want: "http://example.com/doc?",
		},
		{
			name: "a scheme relative reference takes only the scheme",
			base: "https://example.com/dir/doc",
			ref:  "//other.example/path",
			want: "https://other.example/path",
		},
		{
			name: "a scheme relative reference with no path",
			base: "https://example.com/dir/doc",
			ref:  "//other.example",
			want: "https://other.example",
		},
		{
			name: "traversal past the root comes to rest there",
			base: "http://example.com/a/b",
			ref:  "../../../../x",
			want: "http://example.com/x",
		},
		{
			name: "traversal past the root of an absolute reference path",
			base: "http://example.com/a/b",
			ref:  "/../../x",
			want: "http://example.com/x",
		},
		{
			name: "a base with an authority and no path merges as though rooted",
			base: "http://example.com",
			ref:  "g",
			want: "http://example.com/g",
		},
		{
			name: "a base with no authority keeps having none",
			base: "urn:example:resource",
			ref:  "other",
			want: "urn:other",
		},
		{
			name: "a base fragment cannot reach the result",
			base: "http://example.com/dir/doc#ignored",
			ref:  "other",
			want: "http://example.com/dir/other",
		},
		{
			name: "a colon later in a path does not make a scheme",
			base: "http://example.com/dir/doc",
			ref:  "a/b:c",
			want: "http://example.com/dir/a/b:c",
		},
		{
			name: "a leading segment that cannot be a scheme is a path",
			base: "http://example.com/dir/doc",
			ref:  "2011:a",
			want: "http://example.com/dir/2011:a",
		},
		{
			name: "non ascii characters pass through untouched",
			base: "http://example.com/ünïcødé/doc",
			ref:  "../ändere/ißue",
			want: "http://example.com/ändere/ißue",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := iri.Resolve(test.base, test.ref)
			if err != nil {
				t.Fatalf("Resolve() = %v, want nil", err)
			}
			if got != test.want {
				t.Errorf("Resolve() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestResolveAbsoluteReferences covers a reference that brings its own scheme:
// it is returned as it stands, except that §5.2.2 removes dot segments from
// its path along the way.
func TestResolveAbsoluteReferences(t *testing.T) {
	unchanged := []string{
		"http://example.com/a/b?q=1#f",
		"https://other.example",
		"urn:isbn:0451450523",
		"file:///etc/hosts",
		"mailto:someone@example.com",
		"tag:example.com,2026:thing",
		"http://example.com/a%20b",
	}

	for _, ref := range unchanged {
		t.Run(quoted(ref), func(t *testing.T) {
			got, err := iri.Resolve(rfcBase, ref)
			if err != nil {
				t.Fatalf("Resolve() = %v, want nil", err)
			}
			if got != ref {
				t.Errorf("Resolve() = %q, want it returned unchanged", got)
			}
		})
	}

	// The one way an absolute reference does not come back as written.
	dotted := []struct {
		ref  string
		want string
	}{
		{ref: "http://example.com/a/../b", want: "http://example.com/b"},
		{ref: "http://example.com/a/./b", want: "http://example.com/a/b"},
	}

	for _, test := range dotted {
		t.Run(quoted(test.ref), func(t *testing.T) {
			got, err := iri.Resolve(rfcBase, test.ref)
			if err != nil {
				t.Fatalf("Resolve() = %v, want nil", err)
			}
			if got != test.want {
				t.Errorf("Resolve() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestResolveDotSegmentsWithoutARoot covers the corners of RFC 3986 §5.2.4
// that a path beginning with "/" never reaches. A reference carrying its own
// scheme takes its path through dot segment removal without the base's path
// being merged in first, so that path may be rootless — and then a leading
// ".." has nothing to go back through, and a path of "." is the whole of it.
func TestResolveDotSegmentsWithoutARoot(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{
			name: "a leading ./ on a rootless path is dropped",
			ref:  "http:./g",
			want: "http:g",
		},
		{
			name: "a leading ../ on a rootless path has nothing to undo",
			ref:  "http:../g",
			want: "http:g",
		},
		{
			name: "several leading ../ are all dropped",
			ref:  "http:../../../g",
			want: "http:g",
		},
		{
			name: "a rootless path of exactly . leaves nothing",
			ref:  "http:.",
			want: "http:",
		},
		{
			name: "a rootless path of exactly .. leaves nothing",
			ref:  "http:..",
			want: "http:",
		},
		{
			name: "a rooted path of exactly /. is the root",
			ref:  "/.",
			want: "http://a/",
		},
		{
			name: "a rooted path of exactly /.. is the root",
			ref:  "/..",
			want: "http://a/",
		},
		{
			name: "a trailing /.. after a segment goes back through it",
			ref:  "/a/b/..",
			want: "http://a/a/",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := iri.Resolve(rfcBase, test.ref)
			if err != nil {
				t.Fatalf("Resolve() = %v, want nil", err)
			}
			if got != test.want {
				t.Errorf("Resolve() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestResolveRelativeBase(t *testing.T) {
	tests := []struct {
		name string
		base string
	}{
		{
			name: "an empty base",
			base: "",
		},
		{
			name: "a bare path",
			base: "/a/b/c",
		},
		{
			name: "a relative path",
			base: "a/b/c",
		},
		{
			name: "a scheme relative base",
			base: "//example.com/a/b",
		},
		{
			name: "a fragment alone",
			base: "#frag",
		},
		{
			name: "a leading segment that cannot be a scheme",
			base: "2011:a/b",
		},
		{
			name: "a colon that a slash puts out of reach",
			base: "a/b:c",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := iri.Resolve(test.base, "g")
			if !errors.Is(err, iri.ErrRelativeBase) {
				t.Errorf("Resolve() error = %v, want %v", err, iri.ErrRelativeBase)
			}
			if got != "" {
				t.Errorf("Resolve() = %q, want empty on error", got)
			}
		})
	}
}

// TestResolveIsIdempotent checks that resolving a result against the same base
// again changes nothing. A resolved reference is absolute, so it has to come
// back through the absolute branch untouched — which would not hold if
// resolution left dot segments behind.
func TestResolveIsIdempotent(t *testing.T) {
	refs := []string{"g", "./g", "../g", "../../../g", "g/./h", "g/../h", ".", "..", "", "#s", "?y", "//g"}

	for _, ref := range refs {
		t.Run(quoted(ref), func(t *testing.T) {
			once, err := iri.Resolve(rfcBase, ref)
			if err != nil {
				t.Fatalf("Resolve() = %v, want nil", err)
			}

			twice, err := iri.Resolve(rfcBase, once)
			if err != nil {
				t.Fatalf("Resolve() = %v, want nil", err)
			}
			if twice != once {
				t.Errorf("resolving %q again gave %q", once, twice)
			}
		})
	}
}

// quoted names a subtest after a reference, giving the empty one something
// visible to be called.
func quoted(ref string) string {
	if ref == "" {
		return `""`
	}
	return ref
}
