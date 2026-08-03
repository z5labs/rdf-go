package turtle_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/z5labs/rdf-go/rdf12/turtle"
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

// name builds the node a prefixed name with a local part parses to, which the
// expectations below are thick with.
func name(line, column int, prefix, local string) *turtle.PrefixedName {
	return &turtle.PrefixedName{
		Pos: pos(line, column), Prefix: prefix, Local: local, HasLocal: true,
	}
}

// TestParseTripleTerm covers the criterion that a triple term parses in the
// object position and nowhere else.
//
//	tripleTerm ::= '<<(' ttSubject verb ttObject ')>>'
func TestParseTripleTerm(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expected turtle.Term
	}{
		{
			name: "a triple term of prefixed names",
			src:  `:s :p <<( :a :b :c )>> .`,
			expected: &turtle.TripleTerm{
				Pos:     pos(1, 7),
				Subject: name(1, 11, "", "a"),
				Verb:    name(1, 14, "", "b"),
				Object:  name(1, 17, "", "c"),
			},
		},
		{
			name: "the a keyword is a verb inside the brackets too",
			src:  `:s :p <<( :a a :C )>> .`,
			expected: &turtle.TripleTerm{
				Pos:     pos(1, 7),
				Subject: name(1, 11, "", "a"),
				Verb:    &turtle.A{Pos: pos(1, 14)},
				Object:  name(1, 16, "", "C"),
			},
		},
		{
			name: "a triple term with a blank node subject and a literal object",
			src:  `:s :p <<( _:b :q "o" )>> .`,
			expected: &turtle.TripleTerm{
				Pos:     pos(1, 7),
				Subject: &turtle.BlankNode{Pos: pos(1, 11), Label: "b"},
				Verb:    name(1, 15, "", "q"),
				Object: &turtle.Literal{
					Pos: pos(1, 18), Kind: turtle.LiteralString, Value: "o",
				},
			},
		},
		{
			name: "a triple term holding a number, which needs no quotes",
			src:  `:s :p <<( :a :b 42 )>> .`,
			expected: &turtle.TripleTerm{
				Pos:     pos(1, 7),
				Subject: name(1, 11, "", "a"),
				Verb:    name(1, 14, "", "b"),
				Object: &turtle.Literal{
					Pos: pos(1, 17), Kind: turtle.LiteralInteger, Value: "42",
				},
			},
		},
		{
			name: "a triple term holding an anonymous blank node",
			src:  `:s :p <<( [] :b :c )>> .`,
			expected: &turtle.TripleTerm{
				Pos:     pos(1, 7),
				Subject: &turtle.Anon{Pos: pos(1, 11)},
				Verb:    name(1, 14, "", "b"),
				Object:  name(1, 17, "", "c"),
			},
		},
		{
			name: "triple terms nest on the object side",
			src:  `:s :p <<( :a :b <<( :c :d :e )>> )>> .`,
			expected: &turtle.TripleTerm{
				Pos:     pos(1, 7),
				Subject: name(1, 11, "", "a"),
				Verb:    name(1, 14, "", "b"),
				Object: &turtle.TripleTerm{
					Pos:     pos(1, 17),
					Subject: name(1, 21, "", "c"),
					Verb:    name(1, 24, "", "d"),
					Object:  name(1, 27, "", "e"),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := firstObject(t, only(t, parse(t, tc.src)))
			if !reflect.DeepEqual(got, tc.expected) {
				t.Errorf("Parse() = %#v, want %#v", got, tc.expected)
			}
		})
	}
}

// TestParseReifiedTriple covers the reified-triple sugar in every shape the
// production allows, in both the positions it may stand in.
//
//	reifiedTriple ::= '<<' rtSubject verb rtObject reifier? '>>'
//	reifier       ::= '~' (iri | BlankNode)?
func TestParseReifiedTriple(t *testing.T) {
	t.Run("with no reifier at all", func(t *testing.T) {
		got := firstObject(t, only(t, parse(t, `:s :p << :a :b :c >> .`)))

		want := &turtle.ReifiedTriple{
			Pos:     pos(1, 7),
			Subject: name(1, 10, "", "a"),
			Verb:    name(1, 13, "", "b"),
			Object:  name(1, 16, "", "c"),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Parse() = %#v, want %#v", got, want)
		}
	})

	t.Run("with a bare reifier, which is not the same as none", func(t *testing.T) {
		got := firstObject(t, only(t, parse(t, `:s :p << :a :b :c ~ >> .`)))

		want := &turtle.ReifiedTriple{
			Pos:     pos(1, 7),
			Subject: name(1, 10, "", "a"),
			Verb:    name(1, 13, "", "b"),
			Object:  name(1, 16, "", "c"),
			Reifier: &turtle.Reifier{Pos: pos(1, 19)},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Parse() = %#v, want %#v", got, want)
		}
	})

	t.Run("with an iri reifier", func(t *testing.T) {
		got := firstObject(t, only(t, parse(t, `:s :p << :a :b :c ~ :r >> .`)))

		want := &turtle.ReifiedTriple{
			Pos:     pos(1, 7),
			Subject: name(1, 10, "", "a"),
			Verb:    name(1, 13, "", "b"),
			Object:  name(1, 16, "", "c"),
			Reifier: &turtle.Reifier{Pos: pos(1, 19), ID: name(1, 21, "", "r")},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Parse() = %#v, want %#v", got, want)
		}
	})

	t.Run("with a blank node reifier", func(t *testing.T) {
		got := firstObject(t, only(t, parse(t, `:s :p << :a :b :c ~ _:r >> .`)))

		reified, ok := got.(*turtle.ReifiedTriple)
		if !ok {
			t.Fatalf("object is %T, want *turtle.ReifiedTriple", got)
		}
		want := &turtle.BlankNode{Pos: pos(1, 21), Label: "r"}
		if !reflect.DeepEqual(reified.Reifier.ID, want) {
			t.Errorf("Reifier.ID = %#v, want %#v", reified.Reifier.ID, want)
		}
	})

	t.Run("holding the literals the object production admits", func(t *testing.T) {
		testCases := []struct {
			src  string
			want *turtle.Literal
		}{
			{
				src:  `:s :p << :a :b "o" >> .`,
				want: &turtle.Literal{Pos: pos(1, 16), Kind: turtle.LiteralString, Value: "o"},
			},
			{
				src:  `:s :p << :a :b 42 >> .`,
				want: &turtle.Literal{Pos: pos(1, 16), Kind: turtle.LiteralInteger, Value: "42"},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.src, func(t *testing.T) {
				got := firstObject(t, only(t, parse(t, tc.src)))

				reified, ok := got.(*turtle.ReifiedTriple)
				if !ok {
					t.Fatalf("object is %T, want *turtle.ReifiedTriple", got)
				}
				if !reflect.DeepEqual(reified.Object, tc.want) {
					t.Errorf("Object = %#v, want %#v", reified.Object, tc.want)
				}
			})
		}
	})

	t.Run("in the subject position", func(t *testing.T) {
		triples := only(t, parse(t, `<< :s :p :o >> :q :r .`))

		want := &turtle.ReifiedTriple{
			Pos:     pos(1, 1),
			Subject: name(1, 4, "", "s"),
			Verb:    name(1, 7, "", "p"),
			Object:  name(1, 10, "", "o"),
		}
		if !reflect.DeepEqual(triples.Subject, want) {
			t.Errorf("Subject = %#v, want %#v", triples.Subject, want)
		}
		if len(triples.Predicates) != 1 {
			t.Fatalf("parsed %d predicates, want 1", len(triples.Predicates))
		}
	})

	t.Run("standing alone as a whole statement", func(t *testing.T) {
		triples := only(t, parse(t, `<< :s :p :o >> .`))

		if _, ok := triples.Subject.(*turtle.ReifiedTriple); !ok {
			t.Fatalf("subject is %T, want *turtle.ReifiedTriple", triples.Subject)
		}
		if len(triples.Predicates) != 0 {
			t.Errorf("parsed %d predicates, want 0", len(triples.Predicates))
		}
	})

	t.Run("nested in the subject of another", func(t *testing.T) {
		triples := only(t, parse(t, `<< << :s :p :o >> :q :r >> :x :y .`))

		outer, ok := triples.Subject.(*turtle.ReifiedTriple)
		if !ok {
			t.Fatalf("subject is %T, want *turtle.ReifiedTriple", triples.Subject)
		}
		if _, ok := outer.Subject.(*turtle.ReifiedTriple); !ok {
			t.Errorf("nested subject is %T, want *turtle.ReifiedTriple", outer.Subject)
		}
	})

	t.Run("nested in the object of another", func(t *testing.T) {
		got := firstObject(t, only(t, parse(t, `:s :p << :a :b << :c :d :e >> >> .`)))

		outer, ok := got.(*turtle.ReifiedTriple)
		if !ok {
			t.Fatalf("object is %T, want *turtle.ReifiedTriple", got)
		}
		if _, ok := outer.Object.(*turtle.ReifiedTriple); !ok {
			t.Errorf("nested object is %T, want *turtle.ReifiedTriple", outer.Object)
		}
	})

	t.Run("holding a triple term as its object", func(t *testing.T) {
		got := firstObject(t, only(t, parse(t, `:s :p << :a :b <<( :c :d :e )>> >> .`)))

		outer, ok := got.(*turtle.ReifiedTriple)
		if !ok {
			t.Fatalf("object is %T, want *turtle.ReifiedTriple", got)
		}
		if _, ok := outer.Object.(*turtle.TripleTerm); !ok {
			t.Errorf("nested object is %T, want *turtle.TripleTerm", outer.Object)
		}
	})
}

// TestParseDistinguishesTheBracketedForms covers the criterion that the tree
// tells a bare triple term from the reified-triple sugar, which is what lets a
// printer write back whichever the author wrote.
func TestParseDistinguishesTheBracketedForms(t *testing.T) {
	triples := only(t, parse(t, `:s :p <<( :a :b :c )>> , << :a :b :c >> .`))

	objects := triples.Predicates[0].Objects
	if len(objects) != 2 {
		t.Fatalf("parsed %d objects, want 2", len(objects))
	}
	if _, ok := objects[0].(*turtle.TripleTerm); !ok {
		t.Errorf("first object is %T, want *turtle.TripleTerm", objects[0])
	}
	if _, ok := objects[1].(*turtle.ReifiedTriple); !ok {
		t.Errorf("second object is %T, want *turtle.ReifiedTriple", objects[1])
	}
}

// TestParsePositionRestrictions covers the productions that say where the two
// bracketed forms may not stand. Each is a place the grammar names a narrower
// set of terms than the position around it does.
func TestParsePositionRestrictions(t *testing.T) {
	testCases := []struct {
		name string
		src  string
	}{
		{
			name: "a triple term may not be a statement's subject",
			src:  `<<( :a :b :c )>> :p :o .`,
		},
		{
			name: "a triple term may not be a triple term's subject",
			src:  `:s :p <<( <<( :a :b :c )>> :q :o )>> .`,
		},
		{
			name: "a reified triple may not be a triple term's subject",
			src:  `:s :p <<( << :a :b :c >> :q :o )>> .`,
		},
		{
			name: "a reified triple may not be a triple term's object",
			src:  `:s :p <<( :a :b << :c :d :e >> )>> .`,
		},
		{
			name: "a collection may not be a reified triple's subject",
			src:  `:s :p << ( :a ) :b :c >> .`,
		},
		{
			name: "a collection may not be a reified triple's object",
			src:  `:s :p << :a :b ( :c ) >> .`,
		},
		{
			name: "a blank node property list may not be a reified triple's subject",
			src:  `:s :p << [ :a :b ] :c :d >> .`,
		},
		{
			name: "a literal may not be a reified triple's subject",
			src:  `:s :p << "a" :b :c >> .`,
		},
		{
			name: "a literal may not be a triple term's subject",
			src:  `:s :p <<( "a" :b :c )>> .`,
		},
		{
			name: "a triple term takes no reifier",
			src:  `:s :p <<( :a :b :c ~ :r )>> .`,
		},
		{
			name: "a triple term is not closed by the reified triple's bracket",
			src:  `:s :p <<( :a :b :c >> .`,
		},
		{
			name: "a reified triple is not closed by the triple term's bracket",
			src:  `:s :p << :a :b :c )>> .`,
		},
		{
			name: "a reifier takes a term or nothing, not a literal",
			src:  `:s :p << :a :b :c ~ "r" >> .`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := turtle.Parse(strings.NewReader(tc.src))
			if err == nil {
				t.Fatalf("Parse() = %#v, want an error", doc)
			}

			var unexpected turtle.UnexpectedTokenError
			if !errors.As(err, &unexpected) {
				t.Errorf("Parse() error = %#v, want an UnexpectedTokenError", err)
			}
		})
	}
}

// TestParseTruncatedBrackets covers input that ends while a bracketed form is
// still open, which is a document cut off rather than one that ended.
func TestParseTruncatedBrackets(t *testing.T) {
	sources := []string{
		`:s :p <<(`,
		`:s :p <<( :a`,
		`:s :p <<( :a :b`,
		`:s :p <<( :a :b :c`,
		`:s :p <<`,
		`:s :p << :a :b :c`,
		`:s :p << :a :b :c ~`,
		`:s :p << :a :b :c ~ :r`,
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			doc, err := turtle.Parse(strings.NewReader(src))
			if err == nil {
				t.Fatalf("Parse() = %#v, want an error", doc)
			}

			var truncated turtle.UnexpectedEndOfTokensError
			if !errors.As(err, &truncated) {
				t.Errorf("Parse() error = %#v, want an UnexpectedEndOfTokensError", err)
			}
		})
	}
}

// TestParseRDF11Constructs covers the promise the package doc comment makes:
// everything RDF 1.1 Turtle accepts is accepted here unchanged, and parses to
// the same shapes.
func TestParseRDF11Constructs(t *testing.T) {
	t.Run("directives keep the keyword as written", func(t *testing.T) {
		doc := parse(t, "@prefix ex: <http://e/> .\nPREFIX ex2: <http://e2/>\n@base <http://b/> .\nBASE <http://b2/>\n")

		want := []turtle.Statement{
			&turtle.PrefixDirective{
				Pos: pos(1, 1), Keyword: "@prefix", Prefix: "ex",
				IRI: &turtle.IRIRef{Pos: pos(1, 13), Value: "http://e/"},
			},
			&turtle.PrefixDirective{
				Pos: pos(2, 1), Keyword: "PREFIX", Prefix: "ex2",
				IRI: &turtle.IRIRef{Pos: pos(2, 13), Value: "http://e2/"},
			},
			&turtle.BaseDirective{
				Pos: pos(3, 1), Keyword: "@base",
				IRI: &turtle.IRIRef{Pos: pos(3, 7), Value: "http://b/"},
			},
			&turtle.BaseDirective{
				Pos: pos(4, 1), Keyword: "BASE",
				IRI: &turtle.IRIRef{Pos: pos(4, 6), Value: "http://b2/"},
			},
		}
		if !reflect.DeepEqual(doc.Statements, want) {
			t.Errorf("Parse() = %#v, want %#v", doc.Statements, want)
		}
	})

	t.Run("abbreviations are kept rather than expanded", func(t *testing.T) {
		triples := only(t, parse(t, `:s a :C .`))

		if _, ok := triples.Predicates[0].Verb.(*turtle.A); !ok {
			t.Errorf("verb is %T, want *turtle.A", triples.Predicates[0].Verb)
		}
	})

	t.Run("a collection is kept as a list", func(t *testing.T) {
		got := firstObject(t, only(t, parse(t, `:s :p ( :a :b ) .`)))

		collection, ok := got.(*turtle.Collection)
		if !ok {
			t.Fatalf("object is %T, want *turtle.Collection", got)
		}
		if len(collection.Objects) != 2 {
			t.Errorf("collection holds %d objects, want 2", len(collection.Objects))
		}
	})

	t.Run("an empty collection is a collection with nothing in it", func(t *testing.T) {
		got := firstObject(t, only(t, parse(t, `:s :p () .`)))

		collection, ok := got.(*turtle.Collection)
		if !ok {
			t.Fatalf("object is %T, want *turtle.Collection", got)
		}
		if len(collection.Objects) != 0 {
			t.Errorf("collection holds %d objects, want 0", len(collection.Objects))
		}
	})

	t.Run("a blank node property list is kept as a list of predicates", func(t *testing.T) {
		triples := only(t, parse(t, `[ :p :o ] .`))

		list, ok := triples.Subject.(*turtle.BlankNodePropertyList)
		if !ok {
			t.Fatalf("subject is %T, want *turtle.BlankNodePropertyList", triples.Subject)
		}
		if len(list.Predicates) != 1 {
			t.Errorf("list holds %d predicates, want 1", len(list.Predicates))
		}
		if len(triples.Predicates) != 0 {
			t.Errorf("parsed %d predicates, want 0", len(triples.Predicates))
		}
	})

	t.Run("an empty pair of brackets is an anonymous blank node", func(t *testing.T) {
		got := firstObject(t, only(t, parse(t, `:s :p [] .`)))

		if _, ok := got.(*turtle.Anon); !ok {
			t.Errorf("object is %T, want *turtle.Anon", got)
		}
	})

	t.Run("a predicate object list may end in a semicolon", func(t *testing.T) {
		triples := only(t, parse(t, `:s :p :o ; :q :r ; .`))

		if len(triples.Predicates) != 2 {
			t.Errorf("parsed %d predicates, want 2", len(triples.Predicates))
		}
	})

	t.Run("comments are kept with their positions", func(t *testing.T) {
		doc := parse(t, "# one\n:s :p :o . # two\n")

		want := []*turtle.Comment{
			{Pos: pos(1, 1), Text: "# one"},
			{Pos: pos(2, 12), Text: "# two"},
		}
		if !reflect.DeepEqual(doc.Comments, want) {
			t.Errorf("Comments = %#v, want %#v", doc.Comments, want)
		}
	})

	t.Run("every kind of literal is recorded as written", func(t *testing.T) {
		testCases := []struct {
			src  string
			want *turtle.Literal
		}{
			{
				src:  `:s :p "o" .`,
				want: &turtle.Literal{Pos: pos(1, 7), Kind: turtle.LiteralString, Value: "o"},
			},
			{
				src: `:s :p "o"@en .`,
				want: &turtle.Literal{
					Pos: pos(1, 7), Kind: turtle.LiteralString, Value: "o", Language: "en",
				},
			},
			{
				src: `:s :p "o"@en--ltr .`,
				want: &turtle.Literal{
					Pos: pos(1, 7), Kind: turtle.LiteralString, Value: "o",
					Language: "en", Direction: "ltr",
				},
			},
			{
				src: `:s :p "o"^^:dt .`,
				want: &turtle.Literal{
					Pos: pos(1, 7), Kind: turtle.LiteralString, Value: "o",
					Datatype: name(1, 12, "", "dt"),
				},
			},
			{
				src:  `:s :p 42 .`,
				want: &turtle.Literal{Pos: pos(1, 7), Kind: turtle.LiteralInteger, Value: "42"},
			},
			{
				src:  `:s :p 3.14 .`,
				want: &turtle.Literal{Pos: pos(1, 7), Kind: turtle.LiteralDecimal, Value: "3.14"},
			},
			{
				src:  `:s :p 1e6 .`,
				want: &turtle.Literal{Pos: pos(1, 7), Kind: turtle.LiteralDouble, Value: "1e6"},
			},
			{
				src:  `:s :p true .`,
				want: &turtle.Literal{Pos: pos(1, 7), Kind: turtle.LiteralBoolean, Value: "true"},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.src, func(t *testing.T) {
				got := firstObject(t, only(t, parse(t, tc.src)))
				if !reflect.DeepEqual(got, tc.want) {
					t.Errorf("Parse() = %#v, want %#v", got, tc.want)
				}
			})
		}
	})
}

// TestParseRejects covers the tokens the grammar has no place for where they
// stand, in the positions RDF 1.1 already had.
func TestParseRejects(t *testing.T) {
	sources := []string{
		`"o" :p :o .`,
		`:s "p" :o .`,
		`:s :p :o`,
		`@prefix ex: <http://e/>`,
		`@prefix ex: :notaniri .`,
		`@base :notaniri .`,
		`:s :p "o"^^"dt" .`,
		`( :a :b`,
		`[ :p :o`,
		`.`,
	}

	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			if doc, err := turtle.Parse(strings.NewReader(src)); err == nil {
				t.Errorf("Parse() = %#v, want an error", doc)
			}
		})
	}
}

// TestParseReportsTokenizerErrors covers the parser handing back the error that
// stopped it reading rather than one of its own.
func TestParseReportsTokenizerErrors(t *testing.T) {
	doc, err := turtle.Parse(strings.NewReader(`:s :p <http://e/a b> .`))
	if err == nil {
		t.Fatalf("Parse() = %#v, want an error", doc)
	}

	var unexpected turtle.UnexpectedCharacterError
	if !errors.As(err, &unexpected) {
		t.Errorf("Parse() error = %#v, want an UnexpectedCharacterError", err)
	}
}

func TestParseErrorMessages(t *testing.T) {
	testCases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "an unexpected token",
			err: turtle.UnexpectedTokenError{
				Expected: []turtle.TokenType{turtle.TokenIRIRef, turtle.TokenTripleTermOpen},
				Actual:   tok(3, 12, turtle.TokenReifier, "~"),
			},
			want: "unexpected token at line 3, column 12: Reifier(~), expected one of: IRIRef, TripleTermOpen",
		},
		{
			name: "the end of the tokens",
			err: turtle.UnexpectedEndOfTokensError{
				Expected: []turtle.TokenType{turtle.TokenReifiedTripleClose},
				Pos:      pos(3, 12),
			},
			want: "unexpected end of tokens at line 3, column 12, expected one of: ReifiedTripleClose",
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

// TestNodePositions covers Position on the nodes RDF 1.2 adds, which nothing
// else reaches: a printer asks a node where it was written, and a Reifier is
// not a Term and so is never asked through the interface.
func TestNodePositions(t *testing.T) {
	triples := only(t, parse(t, `:s :p << :a :b <<( :c :d :e )>> ~ :r >> .`))

	reified, ok := firstObject(t, triples).(*turtle.ReifiedTriple)
	if !ok {
		t.Fatalf("object is %T, want *turtle.ReifiedTriple", firstObject(t, triples))
	}

	if got, want := reified.Position(), pos(1, 7); got != want {
		t.Errorf("ReifiedTriple.Position() = %s, want %s", got, want)
	}
	if got, want := reified.Object.Position(), pos(1, 16); got != want {
		t.Errorf("TripleTerm.Position() = %s, want %s", got, want)
	}
	if got, want := reified.Reifier.Position(), pos(1, 33); got != want {
		t.Errorf("Reifier.Position() = %s, want %s", got, want)
	}
	if got, want := triples.Position(), pos(1, 1); got != want {
		t.Errorf("Triples.Position() = %s, want %s", got, want)
	}
}

func TestLiteralKindString(t *testing.T) {
	kinds := map[turtle.LiteralKind]string{
		turtle.LiteralString:  "String",
		turtle.LiteralInteger: "Integer",
		turtle.LiteralDecimal: "Decimal",
		turtle.LiteralDouble:  "Double",
		turtle.LiteralBoolean: "Boolean",
		turtle.LiteralKind(9): "LiteralKind(9)",
	}

	for kind, want := range kinds {
		if got := kind.String(); got != want {
			t.Errorf("LiteralKind.String() = %q, want %q", got, want)
		}
	}
}
