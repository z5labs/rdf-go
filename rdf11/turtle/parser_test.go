package turtle_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/z5labs/rdf-go/rdf11/turtle"
)

func parse(t *testing.T, src string) *turtle.Document {
	t.Helper()

	doc, err := turtle.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse() = %v, want nil", err)
	}
	return doc
}

func pos(line, column int) turtle.Pos {
	return turtle.Pos{Line: line, Column: column}
}

// only returns the single set of triples a source was expected to hold.
func only(t *testing.T, doc *turtle.Document) *turtle.Triples {
	t.Helper()

	if len(doc.Statements) != 1 {
		t.Fatalf("parsed %d statements, want 1", len(doc.Statements))
	}
	triples, ok := doc.Statements[0].(*turtle.Triples)
	if !ok {
		t.Fatalf("statement is %T, want *turtle.Triples", doc.Statements[0])
	}
	return triples
}

// firstObject returns the object of the first thing said about a subject.
func firstObject(t *testing.T, triples *turtle.Triples) turtle.Term {
	t.Helper()

	if len(triples.Predicates) == 0 || len(triples.Predicates[0].Objects) == 0 {
		t.Fatal("no objects")
	}
	return triples.Predicates[0].Objects[0]
}

func TestParseDirectives(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected turtle.Statement
	}{
		{
			name: "an at prefix directive",
			src:  `@prefix ex: <http://e/> .`,
			expected: &turtle.PrefixDirective{
				Pos:     pos(1, 1),
				Keyword: "@prefix",
				Prefix:  "ex",
				IRI:     &turtle.IRIRef{Pos: pos(1, 13), Value: "http://e/"},
			},
		},
		{
			name: "a SPARQL prefix directive, which takes no dot",
			src:  `PREFIX ex: <http://e/>`,
			expected: &turtle.PrefixDirective{
				Pos:     pos(1, 1),
				Keyword: "PREFIX",
				Prefix:  "ex",
				IRI:     &turtle.IRIRef{Pos: pos(1, 12), Value: "http://e/"},
			},
		},
		{
			name: "the empty prefix",
			src:  `@prefix : <http://e/> .`,
			expected: &turtle.PrefixDirective{
				Pos:     pos(1, 1),
				Keyword: "@prefix",
				Prefix:  "",
				IRI:     &turtle.IRIRef{Pos: pos(1, 11), Value: "http://e/"},
			},
		},
		{
			name: "an at base directive",
			src:  `@base <http://e/> .`,
			expected: &turtle.BaseDirective{
				Pos:     pos(1, 1),
				Keyword: "@base",
				IRI:     &turtle.IRIRef{Pos: pos(1, 7), Value: "http://e/"},
			},
		},
		{
			name: "a SPARQL base directive",
			src:  `BASE <http://e/>`,
			expected: &turtle.BaseDirective{
				Pos:     pos(1, 1),
				Keyword: "BASE",
				IRI:     &turtle.IRIRef{Pos: pos(1, 6), Value: "http://e/"},
			},
		},
		{
			name: "the SPARQL keyword is kept as written",
			src:  `pReFiX ex: <http://e/>`,
			expected: &turtle.PrefixDirective{
				Pos:     pos(1, 1),
				Keyword: "pReFiX",
				Prefix:  "ex",
				IRI:     &turtle.IRIRef{Pos: pos(1, 12), Value: "http://e/"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc := parse(t, tc.src)
			if len(doc.Statements) != 1 {
				t.Fatalf("parsed %d statements, want 1", len(doc.Statements))
			}
			if !reflect.DeepEqual(doc.Statements[0], tc.expected) {
				t.Errorf("Parse() = %#v, want %#v", doc.Statements[0], tc.expected)
			}
		})
	}
}

// TestParseKeepsAbbreviations covers the criterion that keeps a printer from
// rewriting a document into a different but equivalent one.
func TestParseKeepsAbbreviations(t *testing.T) {
	t.Run("a is not an rdf:type iri", func(t *testing.T) {
		triples := only(t, parse(t, `<http://e/s> a <http://e/C> .`))

		if _, ok := triples.Predicates[0].Verb.(*turtle.A); !ok {
			t.Errorf("verb is %T, want *turtle.A", triples.Predicates[0].Verb)
		}
	})

	t.Run("an explicit rdf:type iri is not the a keyword", func(t *testing.T) {
		src := `<http://e/s> <http://www.w3.org/1999/02/22-rdf-syntax-ns#type> <http://e/C> .`
		triples := only(t, parse(t, src))

		iri, ok := triples.Predicates[0].Verb.(*turtle.IRIRef)
		if !ok {
			t.Fatalf("verb is %T, want *turtle.IRIRef", triples.Predicates[0].Verb)
		}
		if want := "http://www.w3.org/1999/02/22-rdf-syntax-ns#type"; iri.Value != want {
			t.Errorf("Value = %q, want %q", iri.Value, want)
		}
	})

	t.Run("a prefixed name is not an absolute iri", func(t *testing.T) {
		triples := only(t, parse(t, `ex:s ex:p ex:o .`))

		name, ok := triples.Subject.(*turtle.PrefixedName)
		if !ok {
			t.Fatalf("subject is %T, want *turtle.PrefixedName", triples.Subject)
		}
		if name.Prefix != "ex" || name.Local != "s" || !name.HasLocal {
			t.Errorf("subject = %#v, want prefix ex and local s", name)
		}
	})

	t.Run("a namespace with no local name is kept apart from one with an empty local name", func(t *testing.T) {
		triples := only(t, parse(t, `ex: ex:p ex:o .`))

		name, ok := triples.Subject.(*turtle.PrefixedName)
		if !ok {
			t.Fatalf("subject is %T, want *turtle.PrefixedName", triples.Subject)
		}
		if name.HasLocal {
			t.Error("HasLocal = true, want false for a bare namespace")
		}
	})
}

// TestParseCollections covers the criterion that a collection stays a list
// rather than becoming the triples it stands for.
func TestParseCollections(t *testing.T) {
	testCases := []struct {
		name  string
		src   string
		check func(t *testing.T, c *turtle.Collection)
	}{
		{
			name: "an empty collection",
			src:  `<http://e/s> <http://e/p> () .`,
			check: func(t *testing.T, c *turtle.Collection) {
				if len(c.Objects) != 0 {
					t.Errorf("holds %d objects, want 0", len(c.Objects))
				}
			},
		},
		{
			name: "a collection of literals",
			src:  `<http://e/s> <http://e/p> (1 2 3) .`,
			check: func(t *testing.T, c *turtle.Collection) {
				if len(c.Objects) != 3 {
					t.Fatalf("holds %d objects, want 3", len(c.Objects))
				}
				for i, object := range c.Objects {
					literal, ok := object.(*turtle.Literal)
					if !ok {
						t.Fatalf("object %d is %T, want *turtle.Literal", i, object)
					}
					if literal.Kind != turtle.LiteralInteger {
						t.Errorf("object %d is %s, want Integer", i, literal.Kind)
					}
				}
			},
		},
		{
			name: "a nested collection",
			src:  `<http://e/s> <http://e/p> (1 (2 3) 4) .`,
			check: func(t *testing.T, c *turtle.Collection) {
				if len(c.Objects) != 3 {
					t.Fatalf("holds %d objects, want 3", len(c.Objects))
				}
				inner, ok := c.Objects[1].(*turtle.Collection)
				if !ok {
					t.Fatalf("the middle object is %T, want *turtle.Collection", c.Objects[1])
				}
				if len(inner.Objects) != 2 {
					t.Errorf("the inner collection holds %d objects, want 2", len(inner.Objects))
				}
			},
		},
		{
			name: "a collection holding a blank node property list",
			src:  `<http://e/s> <http://e/p> ( [ <http://e/q> 1 ] ) .`,
			check: func(t *testing.T, c *turtle.Collection) {
				if len(c.Objects) != 1 {
					t.Fatalf("holds %d objects, want 1", len(c.Objects))
				}
				if _, ok := c.Objects[0].(*turtle.BlankNodePropertyList); !ok {
					t.Errorf("object is %T, want *turtle.BlankNodePropertyList", c.Objects[0])
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			object := firstObject(t, only(t, parse(t, tc.src)))

			collection, ok := object.(*turtle.Collection)
			if !ok {
				t.Fatalf("object is %T, want *turtle.Collection", object)
			}
			tc.check(t, collection)
		})
	}

	t.Run("a collection in subject position", func(t *testing.T) {
		triples := only(t, parse(t, `(1 2) <http://e/p> <http://e/o> .`))

		collection, ok := triples.Subject.(*turtle.Collection)
		if !ok {
			t.Fatalf("subject is %T, want *turtle.Collection", triples.Subject)
		}
		if len(collection.Objects) != 2 {
			t.Errorf("holds %d objects, want 2", len(collection.Objects))
		}
	})
}

func TestParseBlankNodes(t *testing.T) {
	t.Run("an anon in subject position", func(t *testing.T) {
		triples := only(t, parse(t, `[] <http://e/p> <http://e/o> .`))

		if _, ok := triples.Subject.(*turtle.Anon); !ok {
			t.Errorf("subject is %T, want *turtle.Anon", triples.Subject)
		}
	})

	t.Run("an anon in object position", func(t *testing.T) {
		object := firstObject(t, only(t, parse(t, `<http://e/s> <http://e/p> [] .`)))

		if _, ok := object.(*turtle.Anon); !ok {
			t.Errorf("object is %T, want *turtle.Anon", object)
		}
	})

	t.Run("a property list in subject position says everything itself", func(t *testing.T) {
		triples := only(t, parse(t, `[ <http://e/p> <http://e/o> ] .`))

		list, ok := triples.Subject.(*turtle.BlankNodePropertyList)
		if !ok {
			t.Fatalf("subject is %T, want *turtle.BlankNodePropertyList", triples.Subject)
		}
		if len(list.Predicates) != 1 {
			t.Errorf("holds %d predicates, want 1", len(list.Predicates))
		}
		if len(triples.Predicates) != 0 {
			t.Errorf("the statement has %d predicates of its own, want 0", len(triples.Predicates))
		}
	})

	t.Run("a property list in subject position may still be said more about", func(t *testing.T) {
		triples := only(t, parse(t, `[ <http://e/p> 1 ] <http://e/q> 2 .`))

		if _, ok := triples.Subject.(*turtle.BlankNodePropertyList); !ok {
			t.Fatalf("subject is %T, want *turtle.BlankNodePropertyList", triples.Subject)
		}
		if len(triples.Predicates) != 1 {
			t.Errorf("the statement has %d predicates of its own, want 1", len(triples.Predicates))
		}
	})

	t.Run("a nested property list", func(t *testing.T) {
		src := `<http://e/s> <http://e/p> [ <http://e/q> [ <http://e/r> 1 ] ] .`
		object := firstObject(t, only(t, parse(t, src)))

		outer, ok := object.(*turtle.BlankNodePropertyList)
		if !ok {
			t.Fatalf("object is %T, want *turtle.BlankNodePropertyList", object)
		}
		if _, ok := outer.Predicates[0].Objects[0].(*turtle.BlankNodePropertyList); !ok {
			t.Errorf("the inner object is %T, want *turtle.BlankNodePropertyList",
				outer.Predicates[0].Objects[0])
		}
	})

	t.Run("a labelled blank node", func(t *testing.T) {
		triples := only(t, parse(t, `_:a <http://e/p> _:b .`))

		subject, ok := triples.Subject.(*turtle.BlankNode)
		if !ok {
			t.Fatalf("subject is %T, want *turtle.BlankNode", triples.Subject)
		}
		if subject.Label != "a" {
			t.Errorf("Label = %q, want %q", subject.Label, "a")
		}
	})
}

func TestParseLiterals(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		kind     turtle.LiteralKind
		value    string
		language string
	}{
		{name: "a string", src: `"text"`, kind: turtle.LiteralString, value: "text"},
		{name: "a long string", src: `"""text"""`, kind: turtle.LiteralString, value: "text"},
		{name: "a tagged string", src: `"text"@en-GB`, kind: turtle.LiteralString, value: "text", language: "en-GB"},
		{name: "an integer", src: `42`, kind: turtle.LiteralInteger, value: "42"},
		{name: "a negative integer", src: `-42`, kind: turtle.LiteralInteger, value: "-42"},
		{name: "a decimal", src: `3.14`, kind: turtle.LiteralDecimal, value: "3.14"},
		{name: "a double", src: `1e6`, kind: turtle.LiteralDouble, value: "1e6"},
		{name: "true", src: `true`, kind: turtle.LiteralBoolean, value: "true"},
		{name: "false", src: `false`, kind: turtle.LiteralBoolean, value: "false"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			src := `<http://e/s> <http://e/p> ` + tc.src + ` .`
			object := firstObject(t, only(t, parse(t, src)))

			literal, ok := object.(*turtle.Literal)
			if !ok {
				t.Fatalf("object is %T, want *turtle.Literal", object)
			}
			if literal.Kind != tc.kind {
				t.Errorf("Kind = %s, want %s", literal.Kind, tc.kind)
			}
			if literal.Value != tc.value {
				t.Errorf("Value = %q, want %q", literal.Value, tc.value)
			}
			if literal.Language != tc.language {
				t.Errorf("Language = %q, want %q", literal.Language, tc.language)
			}
			if tc.kind != turtle.LiteralString && literal.Datatype != nil {
				t.Errorf("Datatype = %#v, want nil: the tree records what was written", literal.Datatype)
			}
		})
	}

	t.Run("a datatype written as an iri", func(t *testing.T) {
		src := `<http://e/s> <http://e/p> "1"^^<http://e/dt> .`
		object := firstObject(t, only(t, parse(t, src)))

		literal := object.(*turtle.Literal)
		if _, ok := literal.Datatype.(*turtle.IRIRef); !ok {
			t.Errorf("Datatype is %T, want *turtle.IRIRef", literal.Datatype)
		}
	})

	t.Run("a datatype written as a prefixed name stays one", func(t *testing.T) {
		src := `<http://e/s> <http://e/p> "1"^^xsd:integer .`
		object := firstObject(t, only(t, parse(t, src)))

		literal := object.(*turtle.Literal)
		name, ok := literal.Datatype.(*turtle.PrefixedName)
		if !ok {
			t.Fatalf("Datatype is %T, want *turtle.PrefixedName", literal.Datatype)
		}
		if name.Prefix != "xsd" || name.Local != "integer" {
			t.Errorf("Datatype = %#v, want xsd:integer", name)
		}
	})
}

func TestParsePredicateObjectLists(t *testing.T) {
	t.Run("several verbs separated by semicolons", func(t *testing.T) {
		triples := only(t, parse(t, `<http://e/s> <http://e/p> 1 ; <http://e/q> 2 ; <http://e/r> 3 .`))

		if len(triples.Predicates) != 3 {
			t.Errorf("parsed %d predicates, want 3", len(triples.Predicates))
		}
	})

	t.Run("a trailing semicolon is allowed", func(t *testing.T) {
		triples := only(t, parse(t, `<http://e/s> <http://e/p> 1 ; .`))

		if len(triples.Predicates) != 1 {
			t.Errorf("parsed %d predicates, want 1", len(triples.Predicates))
		}
	})

	t.Run("repeated semicolons are allowed", func(t *testing.T) {
		triples := only(t, parse(t, `<http://e/s> <http://e/p> 1 ;; <http://e/q> 2 .`))

		if len(triples.Predicates) != 2 {
			t.Errorf("parsed %d predicates, want 2", len(triples.Predicates))
		}
	})

	t.Run("several objects separated by commas", func(t *testing.T) {
		triples := only(t, parse(t, `<http://e/s> <http://e/p> 1, 2, 3 .`))

		if len(triples.Predicates) != 1 {
			t.Fatalf("parsed %d predicates, want 1", len(triples.Predicates))
		}
		if len(triples.Predicates[0].Objects) != 3 {
			t.Errorf("parsed %d objects, want 3", len(triples.Predicates[0].Objects))
		}
	})
}

func TestParseKeepsComments(t *testing.T) {
	src := "# first\n" +
		"@prefix ex: <http://e/> . # trailing\n" +
		"# between\n" +
		"ex:s ex:p 1 .\n" +
		"# last"

	doc := parse(t, src)

	want := []*turtle.Comment{
		{Pos: pos(1, 1), Text: "# first"},
		{Pos: pos(2, 27), Text: "# trailing"},
		{Pos: pos(3, 1), Text: "# between"},
		{Pos: pos(5, 1), Text: "# last"},
	}
	if !reflect.DeepEqual(doc.Comments, want) {
		t.Errorf("Comments = %s, want %s", formatComments(doc.Comments), formatComments(want))
	}
}

// formatComments renders comments for a failure message, the default
// rendering of a slice of pointers saying nothing useful.
func formatComments(comments []*turtle.Comment) string {
	var b strings.Builder
	for i, c := range comments {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s@%s", c.Text, c.Pos)
	}
	return "[" + b.String() + "]"
}

func TestParseErrors(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name: "a literal in subject position",
			src:  `"nope" <http://e/p> <http://e/o> .`,
			expectedErr: turtle.UnexpectedTokenError{
				Expected: []turtle.TokenType{
					turtle.TokenIRIRef, turtle.TokenPNameNS, turtle.TokenPNameLN,
					turtle.TokenBlankNodeLabel, turtle.TokenAnon,
					turtle.TokenOpenParen, turtle.TokenOpenBracket,
				},
				Actual: turtle.Token{Pos: pos(1, 1), Type: turtle.TokenString, Value: []byte("nope")},
			},
		},
		{
			name: "a literal in predicate position",
			src:  `<http://e/s> "nope" <http://e/o> .`,
			expectedErr: turtle.UnexpectedTokenError{
				Expected: []turtle.TokenType{
					turtle.TokenIRIRef, turtle.TokenPNameNS,
					turtle.TokenPNameLN, turtle.TokenA,
				},
				Actual: turtle.Token{Pos: pos(1, 14), Type: turtle.TokenString, Value: []byte("nope")},
			},
		},
		{
			name: "a missing trailing dot",
			src:  `<http://e/s> <http://e/p> <http://e/o>`,
			expectedErr: turtle.UnexpectedEndOfTokensError{
				Expected: []turtle.TokenType{turtle.TokenDot},
				Pos:      pos(1, 27),
			},
		},
		{
			name: "an at prefix directive with no dot",
			src:  `@prefix ex: <http://e/>`,
			expectedErr: turtle.UnexpectedEndOfTokensError{
				Expected: []turtle.TokenType{turtle.TokenDot},
				Pos:      pos(1, 13),
			},
		},
		{
			name: "a prefix directive with no namespace",
			src:  `@prefix <http://e/> .`,
			expectedErr: turtle.UnexpectedTokenError{
				Expected: []turtle.TokenType{turtle.TokenPNameNS},
				Actual:   turtle.Token{Pos: pos(1, 9), Type: turtle.TokenIRIRef, Value: []byte("http://e/")},
			},
		},
		{
			name: "an unclosed collection",
			src:  `<http://e/s> <http://e/p> (1 2 .`,
			expectedErr: turtle.UnexpectedTokenError{
				Expected: []turtle.TokenType{
					turtle.TokenIRIRef, turtle.TokenPNameNS, turtle.TokenPNameLN,
					turtle.TokenBlankNodeLabel, turtle.TokenAnon,
					turtle.TokenOpenParen, turtle.TokenOpenBracket, turtle.TokenString,
					turtle.TokenInteger, turtle.TokenDecimal, turtle.TokenDouble, turtle.TokenBoolean,
				},
				Actual: turtle.Token{Pos: pos(1, 32), Type: turtle.TokenDot, Value: []byte(".")},
			},
		},
		{
			name: "an unclosed property list",
			src:  `<http://e/s> <http://e/p> [ <http://e/q> 1 .`,
			expectedErr: turtle.UnexpectedTokenError{
				Expected: []turtle.TokenType{turtle.TokenCloseBracket},
				Actual:   turtle.Token{Pos: pos(1, 44), Type: turtle.TokenDot, Value: []byte(".")},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := turtle.Parse(strings.NewReader(tc.src))
			if !reflect.DeepEqual(err, tc.expectedErr) {
				t.Errorf("Parse() error = %#v,\nwant %#v", err, tc.expectedErr)
			}
		})
	}
}

func TestParseReportsTokenizerErrors(t *testing.T) {
	testCases := []struct {
		name        string
		src         string
		expectedErr error
	}{
		{
			name:        "a character that begins no terminal",
			src:         `$ <http://e/p> <http://e/o> .`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: pos(1, 1), R: '$'},
		},
		{
			name:        "an unterminated string",
			src:         `<http://e/s> <http://e/p> "oops`,
			expectedErr: turtle.UnterminatedStringError{Pos: pos(1, 27)},
		},
		{
			name:        "an rdf 1.2 base direction",
			src:         `<http://e/s> <http://e/p> "v"@en--ltr .`,
			expectedErr: turtle.UnexpectedCharacterError{Pos: pos(1, 34), R: '-'},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := turtle.Parse(strings.NewReader(tc.src))
			if err != tc.expectedErr {
				t.Errorf("Parse() error = %v, want %v", err, tc.expectedErr)
			}
		})
	}
}

func TestParseReaderError(t *testing.T) {
	errBoom := errors.New("boom")

	_, err := turtle.Parse(&failingReader{data: []byte(`<http://e/s> <http://e/p>`), err: errBoom})
	if !errors.Is(err, errBoom) {
		t.Errorf("Parse() error = %v, want %v", err, errBoom)
	}
}

func TestParseErrorMessages(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "unexpected token",
			err: turtle.UnexpectedTokenError{
				Expected: []turtle.TokenType{turtle.TokenIRIRef, turtle.TokenA},
				Actual:   turtle.Token{Pos: pos(2, 7), Type: turtle.TokenString, Value: []byte("v")},
			},
			want: "unexpected token at line 2, column 7: String(v), expected one of: IRIRef, A",
		},
		{
			name: "unexpected end of tokens",
			err: turtle.UnexpectedEndOfTokensError{
				Expected: []turtle.TokenType{turtle.TokenDot},
				Pos:      pos(4, 12),
			},
			want: "unexpected end of tokens at line 4, column 12, expected one of: Dot",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Errorf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestParseDocument reads a document using most of the grammar at once, to
// check the rules hand over to each other correctly.
func TestParseDocument(t *testing.T) {
	src := `@prefix ex: <http://e/> .
@base <http://base/> .
PREFIX foaf: <http://xmlns.com/foaf/0.1/>

ex:alice a foaf:Person ;
	foaf:name "Alice"@en ;
	ex:age 42 ;
	ex:height 1.75 ;
	ex:active true ;
	ex:knows [ foaf:name 'Bob' ], ex:carol ;
	ex:list (1 2 (3 4)) ;
	ex:note """long
text""" .

[] ex:says "anonymous" .
`

	doc := parse(t, src)

	if got, want := len(doc.Statements), 5; got != want {
		t.Fatalf("parsed %d statements, want %d", got, want)
	}

	kinds := []string{"prefix", "base", "prefix", "triples", "triples"}
	for i, statement := range doc.Statements {
		var got string
		switch statement.(type) {
		case *turtle.PrefixDirective:
			got = "prefix"
		case *turtle.BaseDirective:
			got = "base"
		case *turtle.Triples:
			got = "triples"
		}
		if got != kinds[i] {
			t.Errorf("statement %d is %s, want %s", i, got, kinds[i])
		}
	}

	alice, ok := doc.Statements[3].(*turtle.Triples)
	if !ok {
		t.Fatalf("statement 3 is %T, want *turtle.Triples", doc.Statements[3])
	}
	if got, want := len(alice.Predicates), 8; got != want {
		t.Errorf("alice has %d predicates, want %d", got, want)
	}
}

// TestNodesCarryPositions checks that every node reports where it was written.
func TestNodesCarryPositions(t *testing.T) {
	doc := parse(t, `ex:s a ex:C ; ex:p (1) , [ ex:q "v" ] .`)

	triples := only(t, doc)
	positions := []struct {
		name string
		got  turtle.Pos
		want turtle.Pos
	}{
		{name: "document", got: doc.Pos, want: pos(1, 1)},
		{name: "statement", got: triples.Position(), want: pos(1, 1)},
		{name: "subject", got: triples.Subject.Position(), want: pos(1, 1)},
		{name: "the a keyword", got: triples.Predicates[0].Verb.Position(), want: pos(1, 6)},
		{name: "second verb", got: triples.Predicates[1].Verb.Position(), want: pos(1, 15)},
		{name: "collection", got: triples.Predicates[1].Objects[0].Position(), want: pos(1, 20)},
		{name: "property list", got: triples.Predicates[1].Objects[1].Position(), want: pos(1, 26)},
	}

	for _, p := range positions {
		t.Run(p.name, func(t *testing.T) {
			if p.got != p.want {
				t.Errorf("Pos = %s, want %s", p.got, p.want)
			}
		})
	}
}

func TestLiteralKindString(t *testing.T) {
	kinds := map[turtle.LiteralKind]string{
		turtle.LiteralString:  "String",
		turtle.LiteralInteger: "Integer",
		turtle.LiteralDecimal: "Decimal",
		turtle.LiteralDouble:  "Double",
		turtle.LiteralBoolean: "Boolean",
	}
	for kind, want := range kinds {
		if got := kind.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}

	if got, want := turtle.LiteralKind(99).String(), "LiteralKind(99)"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
