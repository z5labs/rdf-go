package rdf

import (
	"iter"
	"strconv"
	"sync/atomic"
)

// scopeCounter numbers the blank node scopes created in this process. It is
// the one piece of scope state shared between goroutines, and the only reason
// two scopes created concurrently cannot mint the same label.
var scopeCounter atomic.Uint64

// newScopePrefix returns a label prefix no other scope in this process will
// use.
//
// The result must leave every label it prefixes valid under the N-Triples
// production
//
//	BLANK_NODE_LABEL ::= '_:' (PN_CHARS_U | [0-9]) ((PN_CHARS | '.')* PN_CHARS)?
//
// so it starts with a letter, which PN_CHARS_U admits, and continues with
// digits and an underscore, both of which PN_CHARS admits. The counter is
// appended in decimal, and the labels that follow it are decimal too, so the
// underscore is what keeps scope 1 node 10 from reading like scope 11 node 0.
func newScopePrefix() string {
	return "b" + strconv.FormatUint(scopeCounter.Add(1), 10) + "_"
}

// BlankNodeScope mints blank nodes whose labels are unique across every scope
// in the process, and remembers the source label each one stands for.
//
// A blank node label means nothing outside the document that wrote it (RDF 1.1
// Concepts §3.4): two files may both write _:b0 for entirely unrelated
// resources. A parser therefore gives each document its own scope and looks
// every label up through it, so that merging the graphs of two documents keeps
// their blank nodes apart instead of unifying them by accident.
//
// [BlankNodeScope.New] mints a blank node no source label maps to, which is
// what a parser needs for the nodes a document implies rather than names: the
// anonymous node of [], the cells of a collection, and, in RDF 1.2, reifiers.
//
// The zero BlankNodeScope is an empty scope, ready to use, and mints labels as
// distinct from every other scope's as one from [NewBlankNodeScope] does.
//
// # Concurrency
//
// A BlankNodeScope is not safe for concurrent use. One parser reading one
// document owns its scope for that document's lifetime, and nothing else
// touches it; callers that do share a scope between goroutines must serialize
// access themselves.
//
// Creating scopes concurrently is safe. The numbering that keeps two scopes'
// labels apart is the only synchronized state, so independent parsers may run
// in parallel as long as each has its own scope.
type BlankNodeScope struct {
	prefix string
	next   uint64
	nodes  map[string]BlankNode
	labels map[BlankNode]string
	order  []string
}

// NewBlankNodeScope returns an empty scope. The zero [BlankNodeScope] is
// equally usable; this is for the common case of needing a pointer.
func NewBlankNodeScope() *BlankNodeScope {
	return new(BlankNodeScope)
}

// Node returns the blank node this scope uses for the source label, minting
// one the first time the label is seen and returning that same node every time
// after.
//
// The label is the one the document wrote, without the leading "_:". It is
// stored as given: deciding whether a document may write it at all is the
// parser's job, not the scope's.
func (s *BlankNodeScope) Node(label string) BlankNode {
	if b, ok := s.nodes[label]; ok {
		return b
	}

	b := s.New()
	if s.nodes == nil {
		s.nodes = make(map[string]BlankNode)
		s.labels = make(map[BlankNode]string)
	}
	s.nodes[label] = b
	s.labels[b] = label
	s.order = append(s.order, label)
	return b
}

// New returns a blank node that no source label maps to and that no other
// scope will mint.
//
// It is what a parser uses for a node the document implies without naming: the
// anonymous node written [], each cell of a collection, and the reifier of an
// RDF 1.2 reified triple.
func (s *BlankNodeScope) New() BlankNode {
	if s.prefix == "" {
		s.prefix = newScopePrefix()
	}

	label := s.prefix + strconv.FormatUint(s.next, 10)
	s.next++
	return BlankNode{label: label}
}

// SourceLabel returns the label the document wrote for b, so that an error
// message can cite what its author typed rather than the label this scope
// minted.
//
// The lookup is by label, which is all a blank node carries. It reports false
// for any label this scope has not mapped: one minted by
// [BlankNodeScope.New], which stands for no source label at all, and one
// belonging to another scope, whose prefix this scope never mints. A node the
// caller assembled by hand is not treated differently — if its label is one
// this scope mapped, it is that node.
func (s *BlankNodeScope) SourceLabel(b BlankNode) (string, bool) {
	label, ok := s.labels[b]
	return label, ok
}

// Len returns the number of distinct source labels the scope has seen. Nodes
// minted by [BlankNodeScope.New] have no source label and are not counted.
func (s *BlankNodeScope) Len() int {
	return len(s.order)
}

// All returns an iterator over the scope's mapping, from each source label to
// the blank node it stands for, in the order the labels were first seen.
//
// The iterator observes the scope as it was when All was called, so labels
// first seen while ranging over it are not visited.
func (s *BlankNodeScope) All() iter.Seq2[string, BlankNode] {
	order := s.order

	return func(yield func(string, BlankNode) bool) {
		for _, label := range order {
			if !yield(label, s.nodes[label]) {
				return
			}
		}
	}
}
