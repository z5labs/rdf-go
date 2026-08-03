package vocab_test

import (
	"strings"
	"testing"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/vocab"
)

// TestNamespaces asserts each namespace against the IRI the specification
// publishes, written out in full rather than assembled from anything in the
// package, so that the test is checking the constants rather than agreeing
// with them.
func TestNamespaces(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "rdf:",
			got:  vocab.NamespaceRDF,
			want: "http://www.w3.org/1999/02/22-rdf-syntax-ns#",
		},
		{
			name: "rdfs:",
			got:  vocab.NamespaceRDFS,
			want: "http://www.w3.org/2000/01/rdf-schema#",
		},
		{
			name: "xsd:",
			got:  vocab.NamespaceXSD,
			want: "http://www.w3.org/2001/XMLSchema#",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Errorf("= %q, want %q", test.got, test.want)
			}
		})
	}
}

// vocabulary is every term this package declares, paired with the IRI the
// specification publishes for it. Each want is a whole IRI written out by hand:
// building one from vocab.NamespaceRDF would only assert that the constant
// equals itself.
var vocabulary = []struct {
	name string
	got  rdf.IRI
	want string
}{
	{
		name: "rdf:type",
		got:  vocab.RDFType,
		want: "http://www.w3.org/1999/02/22-rdf-syntax-ns#type",
	},
	{
		name: "rdf:first",
		got:  vocab.RDFFirst,
		want: "http://www.w3.org/1999/02/22-rdf-syntax-ns#first",
	},
	{
		name: "rdf:rest",
		got:  vocab.RDFRest,
		want: "http://www.w3.org/1999/02/22-rdf-syntax-ns#rest",
	},
	{
		name: "rdf:nil",
		got:  vocab.RDFNil,
		want: "http://www.w3.org/1999/02/22-rdf-syntax-ns#nil",
	},
	{
		name: "rdf:langString",
		got:  vocab.RDFLangString,
		want: "http://www.w3.org/1999/02/22-rdf-syntax-ns#langString",
	},
	{
		name: "rdf:dirLangString",
		got:  vocab.RDFDirLangString,
		want: "http://www.w3.org/1999/02/22-rdf-syntax-ns#dirLangString",
	},
	{
		name: "rdf:reifies",
		got:  vocab.RDFReifies,
		want: "http://www.w3.org/1999/02/22-rdf-syntax-ns#reifies",
	},
	{
		name: "rdf:JSON",
		got:  vocab.RDFJSON,
		want: "http://www.w3.org/1999/02/22-rdf-syntax-ns#JSON",
	},
	{
		name: "xsd:string",
		got:  vocab.XSDString,
		want: "http://www.w3.org/2001/XMLSchema#string",
	},
	{
		name: "xsd:boolean",
		got:  vocab.XSDBoolean,
		want: "http://www.w3.org/2001/XMLSchema#boolean",
	},
	{
		name: "xsd:integer",
		got:  vocab.XSDInteger,
		want: "http://www.w3.org/2001/XMLSchema#integer",
	},
	{
		name: "xsd:decimal",
		got:  vocab.XSDDecimal,
		want: "http://www.w3.org/2001/XMLSchema#decimal",
	},
	{
		name: "xsd:double",
		got:  vocab.XSDDouble,
		want: "http://www.w3.org/2001/XMLSchema#double",
	},
	{
		name: "rdfs:Class",
		got:  vocab.RDFSClass,
		want: "http://www.w3.org/2000/01/rdf-schema#Class",
	},
	{
		name: "rdfs:subClassOf",
		got:  vocab.RDFSSubClassOf,
		want: "http://www.w3.org/2000/01/rdf-schema#subClassOf",
	},
	{
		name: "rdfs:domain",
		got:  vocab.RDFSDomain,
		want: "http://www.w3.org/2000/01/rdf-schema#domain",
	},
	{
		name: "rdfs:range",
		got:  vocab.RDFSRange,
		want: "http://www.w3.org/2000/01/rdf-schema#range",
	},
	{
		name: "rdfs:label",
		got:  vocab.RDFSLabel,
		want: "http://www.w3.org/2000/01/rdf-schema#label",
	},
	{
		name: "rdfs:comment",
		got:  vocab.RDFSComment,
		want: "http://www.w3.org/2000/01/rdf-schema#comment",
	},
}

func TestVocabulary(t *testing.T) {
	for _, test := range vocabulary {
		t.Run(test.name, func(t *testing.T) {
			if string(test.got) != test.want {
				t.Errorf("= %q, want %q", test.got, test.want)
			}
		})
	}
}

// TestVocabularyIsDistinct guards against the failure a table of near-identical
// strings invites: two constants copied from one another and one of them left
// unedited. A duplicate would still pass TestVocabulary if the want beside it
// were copied too.
func TestVocabularyIsDistinct(t *testing.T) {
	seen := make(map[rdf.IRI]string, len(vocabulary))
	for _, term := range vocabulary {
		if first, ok := seen[term.got]; ok {
			t.Errorf("%s and %s are both %s", first, term.name, term.got)
			continue
		}
		seen[term.got] = term.name
	}
}

// TestVocabularyUsesItsNamespace checks that each term sits under the namespace
// its name claims, and that what follows is the local name the prefixed form
// gives — so a term filed under the wrong vocabulary is caught even if its own
// IRI is written correctly.
func TestVocabularyUsesItsNamespace(t *testing.T) {
	namespaces := map[string]string{
		"rdf":  vocab.NamespaceRDF,
		"rdfs": vocab.NamespaceRDFS,
		"xsd":  vocab.NamespaceXSD,
	}

	for _, term := range vocabulary {
		t.Run(term.name, func(t *testing.T) {
			prefix, local, found := strings.Cut(term.name, ":")
			if !found {
				t.Fatalf("test name %q is not a prefixed name", term.name)
			}

			namespace, ok := namespaces[prefix]
			if !ok {
				t.Fatalf("no namespace declared for the prefix %q", prefix)
			}
			if want := rdf.IRI(namespace + local); term.got != want {
				t.Errorf("= %s, want %s", term.got, want)
			}
		})
	}
}

// TestReexportsMatchTheRootPackage covers the reason this package aliases the
// constants the rdf package declares instead of restating them: a datatype
// check written against either one has to reach the same answer.
func TestReexportsMatchTheRootPackage(t *testing.T) {
	tests := []struct {
		name     string
		vocab    rdf.IRI
		declared rdf.IRI
	}{
		{
			name:     "rdf:langString",
			vocab:    vocab.RDFLangString,
			declared: rdf.RDFLangString,
		},
		{
			name:     "rdf:dirLangString",
			vocab:    vocab.RDFDirLangString,
			declared: rdf.RDFDirLangString,
		},
		{
			name:     "xsd:string",
			vocab:    vocab.XSDString,
			declared: rdf.XSDString,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.vocab != test.declared {
				t.Errorf("vocab has %s, the rdf package has %s", test.vocab, test.declared)
			}
		})
	}

	t.Run("namespaces", func(t *testing.T) {
		if vocab.NamespaceRDF != rdf.NamespaceRDF {
			t.Errorf("vocab has %s, the rdf package has %s", vocab.NamespaceRDF, rdf.NamespaceRDF)
		}
		if vocab.NamespaceXSD != rdf.NamespaceXSD {
			t.Errorf("vocab has %s, the rdf package has %s", vocab.NamespaceXSD, rdf.NamespaceXSD)
		}
	})
}

// TestVocabularyIsUsableAsTerms checks the constants are typed so that they can
// be dropped straight into a statement, which is the whole point of declaring
// them as [rdf.IRI] rather than as strings.
func TestVocabularyIsUsableAsTerms(t *testing.T) {
	triple := rdf.Triple{
		Subject:   rdf.IRI("http://example.com/thing"),
		Predicate: vocab.RDFType,
		Object:    vocab.RDFSClass,
	}
	if err := triple.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}

	want := "<http://example.com/thing> " +
		"<http://www.w3.org/1999/02/22-rdf-syntax-ns#type> " +
		"<http://www.w3.org/2000/01/rdf-schema#Class> ."
	if got := triple.String(); got != want {
		t.Errorf("String() = %s, want %s", got, want)
	}
}
