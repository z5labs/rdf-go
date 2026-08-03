package trig_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/z5labs/rdf-go/rdf11/trig"
)

func parse(t *testing.T, src string) *trig.Document {
	t.Helper()

	doc, err := trig.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}
	return doc
}

func pos(line, column int) trig.Pos {
	return trig.Pos{Line: line, Column: column}
}

// block returns the single graph block a source was expected to hold.
func block(t *testing.T, doc *trig.Document) *trig.GraphBlock {
	t.Helper()

	if len(doc.Statements) != 1 {
		t.Fatalf("parsed %d statements, want 1", len(doc.Statements))
	}
	b, ok := doc.Statements[0].(*trig.GraphBlock)
	if !ok {
		t.Fatalf("parsed a %T, want a *trig.GraphBlock", doc.Statements[0])
	}
	return b
}

// TestParseGraphBlocks covers the three ways a document may write a block, and
// what tells them apart in the tree.
//
//	block          ::= triplesOrGraph | wrappedGraph | triples2
//	                 | "GRAPH" labelOrSubject wrappedGraph
//	triplesOrGraph ::= labelOrSubject (wrappedGraph | predicateObjectList '.')
func TestParseGraphBlocks(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		wantPos     trig.Pos
		wantKeyword string
		wantLabel   trig.Term
		wantTriples int
	}{
		{
			name:        "a block with no label at all",
			src:         "{ <http://e/s> <http://e/p> <http://e/o> . }",
			wantPos:     pos(1, 1),
			wantKeyword: "",
			wantLabel:   nil,
			wantTriples: 1,
		},
		{
			name:        "an empty block with no label",
			src:         "{}",
			wantPos:     pos(1, 1),
			wantKeyword: "",
			wantLabel:   nil,
			wantTriples: 0,
		},
		{
			name:        "a block labelled by an iri",
			src:         "<http://e/g> { }",
			wantPos:     pos(1, 1),
			wantKeyword: "",
			wantLabel:   &trig.IRIRef{Pos: pos(1, 1), Value: "http://e/g"},
			wantTriples: 0,
		},
		{
			name:        "a block labelled by a prefixed name",
			src:         "@prefix ex: <http://e/> .\nex:g { }",
			wantKeyword: "",
			wantLabel:   &trig.PrefixedName{Pos: pos(2, 1), Prefix: "ex", Local: "g", HasLocal: true},
			wantPos:     pos(2, 1),
			wantTriples: 0,
		},
		{
			name:        "a block labelled by a blank node",
			src:         "_:g { }",
			wantPos:     pos(1, 1),
			wantKeyword: "",
			wantLabel:   &trig.BlankNode{Pos: pos(1, 1), Label: "g"},
			wantTriples: 0,
		},
		{
			name:        "a block labelled by an anonymous blank node",
			src:         "[] { }",
			wantPos:     pos(1, 1),
			wantKeyword: "",
			wantLabel:   &trig.Anon{Pos: pos(1, 1)},
			wantTriples: 0,
		},
		{
			name:        "the keyword in upper case",
			src:         "GRAPH <http://e/g> { }",
			wantPos:     pos(1, 1),
			wantKeyword: "GRAPH",
			wantLabel:   &trig.IRIRef{Pos: pos(1, 7), Value: "http://e/g"},
			wantTriples: 0,
		},
		{
			name:        "the keyword in lower case",
			src:         "graph <http://e/g> { }",
			wantPos:     pos(1, 1),
			wantKeyword: "graph",
			wantLabel:   &trig.IRIRef{Pos: pos(1, 7), Value: "http://e/g"},
			wantTriples: 0,
		},
		{
			name:        "the keyword before a blank node label",
			src:         "GRAPH _:g { }",
			wantPos:     pos(1, 1),
			wantKeyword: "GRAPH",
			wantLabel:   &trig.BlankNode{Pos: pos(1, 7), Label: "g"},
			wantTriples: 0,
		},
		{
			name: "several sets of triples, the last without its dot",
			src: "<http://e/g> {\n" +
				"  <http://e/s> <http://e/p> <http://e/o> .\n" +
				"  <http://e/t> <http://e/p> <http://e/o>\n" +
				"}",
			wantPos:     pos(1, 1),
			wantLabel:   &trig.IRIRef{Pos: pos(1, 1), Value: "http://e/g"},
			wantTriples: 2,
		},
		{
			name:        "a trailing dot before the closing brace",
			src:         "<http://e/g> { <http://e/s> <http://e/p> <http://e/o> . }",
			wantPos:     pos(1, 1),
			wantLabel:   &trig.IRIRef{Pos: pos(1, 1), Value: "http://e/g"},
			wantTriples: 1,
		},
		{
			name:        "a blank node property list needs no predicates of its own",
			src:         "<http://e/g> { [ <http://e/p> <http://e/o> ] }",
			wantPos:     pos(1, 1),
			wantLabel:   &trig.IRIRef{Pos: pos(1, 1), Value: "http://e/g"},
			wantTriples: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc := parse(t, tc.src)

			// The directive a case may have written first is not the block.
			statements := doc.Statements
			if len(statements) == 2 {
				doc = &trig.Document{Statements: statements[1:]}
			}

			b := block(t, doc)
			if b.Pos != tc.wantPos {
				t.Errorf("Pos = %v, want %v", b.Pos, tc.wantPos)
			}
			if b.Keyword != tc.wantKeyword {
				t.Errorf("Keyword = %q, want %q", b.Keyword, tc.wantKeyword)
			}
			if !reflect.DeepEqual(b.Label, tc.wantLabel) {
				t.Errorf("Label = %#v, want %#v", b.Label, tc.wantLabel)
			}
			if len(b.Triples) != tc.wantTriples {
				t.Errorf("held %d sets of triples, want %d", len(b.Triples), tc.wantTriples)
			}
		})
	}
}

// TestParseDefaultGraphStatements covers the statements written outside every
// block, which the grammar reaches through triplesOrGraph and triples2.
func TestParseDefaultGraphStatements(t *testing.T) {
	testCases := []struct {
		name string
		src  string
	}{
		{name: "an iri subject", src: "<http://e/s> <http://e/p> <http://e/o> ."},
		{name: "a blank node subject", src: "_:s <http://e/p> <http://e/o> ."},
		{name: "an anonymous subject", src: "[] <http://e/p> <http://e/o> ."},
		{name: "a collection subject", src: "( <http://e/a> ) <http://e/p> <http://e/o> ."},
		{name: "a property list subject", src: "[ <http://e/p> <http://e/o> ] ."},
		{name: "a property list subject with more to say", src: "[ <http://e/p> <http://e/o> ] <http://e/q> <http://e/r> ."},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc := parse(t, tc.src)
			if len(doc.Statements) != 1 {
				t.Fatalf("parsed %d statements, want 1", len(doc.Statements))
			}
			if _, ok := doc.Statements[0].(*trig.Triples); !ok {
				t.Errorf("parsed a %T, want a *trig.Triples", doc.Statements[0])
			}
		})
	}
}

// TestParseMixedDocument covers a document that uses every form at once, which
// is what a parser has to keep straight.
func TestParseMixedDocument(t *testing.T) {
	src := "@prefix ex: <http://e/> .\n" +
		"ex:s ex:p ex:o .\n" +
		"{ ex:s ex:q ex:r . }\n" +
		"ex:g { ex:s ex:p ex:o . }\n" +
		"GRAPH ex:h { ex:s ex:p ex:o . }\n" +
		"PREFIX f: <http://f/>\n" +
		"f:s f:p f:o .\n"

	doc := parse(t, src)

	want := []string{
		"*trig.PrefixDirective", "*trig.Triples", "*trig.GraphBlock",
		"*trig.GraphBlock", "*trig.GraphBlock", "*trig.PrefixDirective",
		"*trig.Triples",
	}
	if len(doc.Statements) != len(want) {
		t.Fatalf("parsed %d statements, want %d", len(doc.Statements), len(want))
	}
	for i, statement := range doc.Statements {
		if got := reflect.TypeOf(statement).String(); got != want[i] {
			t.Errorf("statement %d is a %s, want a %s", i, got, want[i])
		}
	}
}

func TestParseComments(t *testing.T) {
	src := "# one\n" +
		"<http://e/g> { # two\n" +
		"  <http://e/s> <http://e/p> <http://e/o> . # three\n" +
		"} # four\n" +
		"# five"

	doc := parse(t, src)

	want := []*trig.Comment{
		{Pos: pos(1, 1), Text: "# one"},
		{Pos: pos(2, 16), Text: "# two"},
		{Pos: pos(3, 44), Text: "# three"},
		{Pos: pos(4, 3), Text: "# four"},
		{Pos: pos(5, 1), Text: "# five"},
	}
	if !reflect.DeepEqual(doc.Comments, want) {
		t.Errorf("Comments = %v, want %v", doc.Comments, want)
	}
}

func TestParseErrors(t *testing.T) {
	testCases := []struct {
		name string
		src  string
		want error
	}{
		{
			name: "a block that is never closed",
			src:  "<http://e/g> { <http://e/s> <http://e/p> <http://e/o> .",
			want: trig.UnexpectedEndOfTokensError{
				Expected: []trig.TokenType{
					trig.TokenIRIRef, trig.TokenPNameNS, trig.TokenPNameLN,
					trig.TokenBlankNodeLabel, trig.TokenAnon,
					trig.TokenOpenParen, trig.TokenOpenBracket, trig.TokenCloseBrace,
				},
				Pos: pos(1, 55),
			},
		},
		{
			name: "a missing dot between two sets of triples",
			src:  "{ <http://e/s> <http://e/p> <http://e/o> <http://e/t> <http://e/p> <http://e/o> }",
			want: trig.UnexpectedTokenError{
				Expected: []trig.TokenType{trig.TokenDot, trig.TokenCloseBrace},
				Actual:   tok(1, 42, trig.TokenIRIRef, "http://e/t"),
			},
		},
		{
			name: "the keyword with no label",
			src:  "GRAPH { }",
			want: trig.UnexpectedTokenError{
				Expected: []trig.TokenType{
					trig.TokenIRIRef, trig.TokenPNameNS, trig.TokenPNameLN,
					trig.TokenBlankNodeLabel, trig.TokenAnon,
				},
				Actual: tok(1, 7, trig.TokenOpenBrace, "{"),
			},
		},
		{
			name: "the keyword with no block",
			src:  "GRAPH <http://e/g> .",
			want: trig.UnexpectedTokenError{
				Expected: []trig.TokenType{trig.TokenOpenBrace},
				Actual:   tok(1, 20, trig.TokenDot, "."),
			},
		},
		{
			// labelOrSubject admits neither, so the '{' cannot open a graph and
			// a verb was required instead.
			name: "a collection cannot label a graph",
			src:  "( <http://e/a> ) { }",
			want: trig.UnexpectedTokenError{
				Expected: []trig.TokenType{
					trig.TokenIRIRef, trig.TokenPNameNS, trig.TokenPNameLN, trig.TokenA,
				},
				Actual: tok(1, 18, trig.TokenOpenBrace, "{"),
			},
		},
		{
			name: "a property list cannot label a graph",
			src:  "[ <http://e/p> <http://e/o> ] { }",
			want: trig.UnexpectedTokenError{
				Expected: []trig.TokenType{
					trig.TokenIRIRef, trig.TokenPNameNS, trig.TokenPNameLN, trig.TokenA,
				},
				Actual: tok(1, 31, trig.TokenOpenBrace, "{"),
			},
		},
		{
			name: "a literal cannot label a graph",
			src:  `"g" { }`,
			want: trig.UnexpectedTokenError{
				Expected: []trig.TokenType{
					trig.TokenIRIRef, trig.TokenPNameNS, trig.TokenPNameLN,
					trig.TokenBlankNodeLabel, trig.TokenAnon,
					trig.TokenOpenParen, trig.TokenOpenBracket,
				},
				Actual: tok(1, 1, trig.TokenString, "g"),
			},
		},
		{
			name: "a stray closing brace",
			src:  "}",
			want: trig.UnexpectedTokenError{
				Expected: []trig.TokenType{
					trig.TokenIRIRef, trig.TokenPNameNS, trig.TokenPNameLN,
					trig.TokenBlankNodeLabel, trig.TokenAnon,
					trig.TokenOpenParen, trig.TokenOpenBracket,
				},
				Actual: tok(1, 1, trig.TokenCloseBrace, "}"),
			},
		},
		{
			name: "a block may not nest",
			src:  "<http://e/g> { <http://e/h> { } }",
			want: trig.UnexpectedTokenError{
				Expected: []trig.TokenType{
					trig.TokenIRIRef, trig.TokenPNameNS, trig.TokenPNameLN, trig.TokenA,
				},
				Actual: tok(1, 29, trig.TokenOpenBrace, "{"),
			},
		},
		{
			name: "a statement outside a block still needs its dot",
			src:  "<http://e/s> <http://e/p> <http://e/o> }",
			want: trig.UnexpectedTokenError{
				Expected: []trig.TokenType{trig.TokenDot},
				Actual:   tok(1, 40, trig.TokenCloseBrace, "}"),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := trig.Parse(strings.NewReader(tc.src))
			if !reflect.DeepEqual(err, tc.want) {
				t.Errorf("Parse() = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestParseReportsTokenizerErrors covers the errors that come from below the
// parser, which it passes on rather than replacing.
func TestParseReportsTokenizerErrors(t *testing.T) {
	_, err := trig.Parse(strings.NewReader("<http://e/g> { $ }"))
	want := trig.UnexpectedCharacterError{Pos: pos(1, 16), R: '$'}
	if err != want {
		t.Errorf("Parse() = %v, want %v", err, want)
	}
}
