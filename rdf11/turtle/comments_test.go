package turtle_test

import (
	"reflect"
	"testing"

	"github.com/z5labs/rdf-go/rdf11/turtle"
)

// TestParseKeepsCommentsInsideStatements covers the other half of a comment
// being white space: it may stand between any two terminals, not only between
// statements.
//
// A parser that allowed them only between statements would refuse documents
// that are perfectly ordinary — a note against a predicate, or against one
// entry of a collection — and would lose the comment even where it did parse.
func TestParseKeepsCommentsInsideStatements(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want []*turtle.Comment
	}{
		{
			name: "between a predicate and its object",
			src:  "<http://e/s> <http://e/p> # a note\n<http://e/o> .",
			want: []*turtle.Comment{{Pos: pos(1, 27), Text: "# a note"}},
		},
		{
			name: "between an object and the dot",
			src:  "<http://e/s> <http://e/p> <http://e/o> # a note\n.",
			want: []*turtle.Comment{{Pos: pos(1, 40), Text: "# a note"}},
		},
		{
			name: "inside a collection",
			src:  "<http://e/s> <http://e/p> (1 # a note\n2) .",
			want: []*turtle.Comment{{Pos: pos(1, 30), Text: "# a note"}},
		},
		{
			name: "inside a blank node property list",
			src:  "<http://e/s> <http://e/p> [ # a note\n<http://e/q> 1 ] .",
			want: []*turtle.Comment{{Pos: pos(1, 29), Text: "# a note"}},
		},
		{
			name: "inside a directive",
			src:  "@prefix # a note\nex: <http://e/> .",
			want: []*turtle.Comment{{Pos: pos(1, 9), Text: "# a note"}},
		},
		{
			name: "after a language tag",
			src:  "<http://e/s> <http://e/p> \"v\"@en # a note\n.",
			want: []*turtle.Comment{{Pos: pos(1, 34), Text: "# a note"}},
		},
		{
			name: "several inside one statement",
			src: "<http://e/s> # one\n" +
				"<http://e/p> # two\n" +
				"<http://e/o> # three\n" +
				".",
			want: []*turtle.Comment{
				{Pos: pos(1, 14), Text: "# one"},
				{Pos: pos(2, 14), Text: "# two"},
				{Pos: pos(3, 14), Text: "# three"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc := parse(t, tc.src)

			if !reflect.DeepEqual(doc.Comments, tc.want) {
				t.Errorf("Comments = %s, want %s", formatComments(doc.Comments), formatComments(tc.want))
			}
			if got, want := len(doc.Statements), 1; got != want {
				t.Errorf("parsed %d statements, want %d", got, want)
			}
		})
	}
}
