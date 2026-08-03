// Package vocab provides the IRIs of the standard RDF vocabularies as
// constants, so that a namespace is never hand-typed and every datatype check
// across the parsers and printers in this module compares against one source
// of truth.
//
// Names carry the prefix of the vocabulary they come from, because the same
// local name means different things in different ones: [RDFSLabel] is
// rdfs:label and would collide with an rdf:label were one ever defined.
//
// The [github.com/z5labs/rdf-go] package declares a handful of these itself —
// the ones it needs to enforce the invariants of [rdf.Literal] — because this
// package depends on [rdf.IRI] and so cannot be imported from there. Those are
// re-exported here rather than written out twice, so there is still only one
// place each string is spelled.
package vocab

import (
	rdf "github.com/z5labs/rdf-go"
)

// The namespace IRIs of the standard vocabularies.
//
// These are untyped string constants rather than [rdf.IRI] so that they can be
// concatenated with a local name to form one, as the constants below do and as
// a caller may for a term this package does not declare.
const (
	// NamespaceRDF is the RDF namespace, whose conventional prefix is rdf:
	// (RDF 1.1 Concepts §1.4).
	NamespaceRDF = rdf.NamespaceRDF

	// NamespaceRDFS is the RDF Schema namespace, whose conventional prefix is
	// rdfs: (RDF Schema 1.1 §1.2).
	NamespaceRDFS = "http://www.w3.org/2000/01/rdf-schema#"

	// NamespaceXSD is the XML Schema datatypes namespace, whose conventional
	// prefix is xsd: (RDF 1.1 Concepts §5.1).
	NamespaceXSD = rdf.NamespaceXSD
)

// Terms of the RDF vocabulary (RDF Schema 1.1 §5, RDF 1.2 Concepts §1.9).
const (
	// RDFType is rdf:type, which states that its subject is an instance of its
	// object. It is the predicate the "a" keyword abbreviates in Turtle and
	// TriG.
	RDFType rdf.IRI = NamespaceRDF + "type"

	// RDFFirst is rdf:first, the predicate holding the head of a collection
	// cell.
	RDFFirst rdf.IRI = NamespaceRDF + "first"

	// RDFRest is rdf:rest, the predicate holding the tail of a collection
	// cell: either another cell or [RDFNil].
	RDFRest rdf.IRI = NamespaceRDF + "rest"

	// RDFNil is rdf:nil, the empty collection, which terminates the chain of
	// cells a Turtle collection expands to.
	RDFNil rdf.IRI = NamespaceRDF + "nil"

	// RDFLangString is rdf:langString, the datatype of every literal carrying
	// a language tag but no base direction.
	RDFLangString rdf.IRI = rdf.RDFLangString

	// RDFDirLangString is rdf:dirLangString, the datatype of every literal
	// carrying both a language tag and a base direction.
	//
	// It is an RDF 1.2 addition: RDF 1.1 has no way to record the base
	// direction of text, so no 1.1 document can mention this datatype.
	RDFDirLangString rdf.IRI = rdf.RDFDirLangString

	// RDFReifies is rdf:reifies, the predicate relating a reifier to the
	// triple term it reifies.
	//
	// It is an RDF 1.2 addition, introduced with triple terms; RDF 1.1
	// reification instead spelled a statement out with rdf:subject,
	// rdf:predicate and rdf:object.
	RDFReifies rdf.IRI = NamespaceRDF + "reifies"

	// RDFJSON is rdf:JSON, the datatype of a literal whose lexical form is a
	// JSON document. The local name is capitalised, as a datatype's is.
	RDFJSON rdf.IRI = NamespaceRDF + "JSON"
)

// Datatypes of the XML Schema vocabulary that RDF adopts (RDF 1.1 Concepts
// §5.1, XML Schema Part 2 §3).
const (
	// XSDString is xsd:string, the datatype of a literal written with neither
	// a language tag nor an explicit datatype.
	XSDString rdf.IRI = rdf.XSDString

	// XSDBoolean is xsd:boolean, whose lexical forms are "true" and "false".
	XSDBoolean rdf.IRI = NamespaceXSD + "boolean"

	// XSDInteger is xsd:integer, the datatype Turtle gives an unquoted
	// integer.
	XSDInteger rdf.IRI = NamespaceXSD + "integer"

	// XSDDecimal is xsd:decimal, the datatype Turtle gives an unquoted number
	// written with a decimal point but no exponent.
	XSDDecimal rdf.IRI = NamespaceXSD + "decimal"

	// XSDDouble is xsd:double, the datatype Turtle gives an unquoted number
	// written with an exponent.
	XSDDouble rdf.IRI = NamespaceXSD + "double"
)

// Terms of the RDF Schema vocabulary (RDF Schema 1.1 §2 and §3).
const (
	// RDFSClass is rdfs:Class, the class of classes.
	RDFSClass rdf.IRI = NamespaceRDFS + "Class"

	// RDFSSubClassOf is rdfs:subClassOf, which states that every instance of
	// its subject is an instance of its object.
	RDFSSubClassOf rdf.IRI = NamespaceRDFS + "subClassOf"

	// RDFSDomain is rdfs:domain, which states that anything appearing as the
	// subject of its subject property is an instance of its object.
	RDFSDomain rdf.IRI = NamespaceRDFS + "domain"

	// RDFSRange is rdfs:range, which states that anything appearing as the
	// object of its subject property is an instance of its object.
	RDFSRange rdf.IRI = NamespaceRDFS + "range"

	// RDFSLabel is rdfs:label, a human-readable name for the subject.
	RDFSLabel rdf.IRI = NamespaceRDFS + "label"

	// RDFSComment is rdfs:comment, a human-readable description of the
	// subject.
	RDFSComment rdf.IRI = NamespaceRDFS + "comment"
)
