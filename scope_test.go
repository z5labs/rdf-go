package rdf_test

import (
	"regexp"
	"slices"
	"strconv"
	"sync"
	"testing"

	rdf "github.com/z5labs/rdf-go"
)

func TestBlankNodeScopeNode(t *testing.T) {
	t.Run("the same source label maps to the same node", func(t *testing.T) {
		s := rdf.NewBlankNodeScope()

		first, second := s.Node("b0"), s.Node("b0")
		if !first.Equal(second) {
			t.Errorf("Node(%q) = %s then %s, want the same node", "b0", first, second)
		}
		if got, want := s.Len(), 1; got != want {
			t.Errorf("Len() = %d, want %d", got, want)
		}
	})

	t.Run("different source labels map to different nodes", func(t *testing.T) {
		s := rdf.NewBlankNodeScope()

		if first, second := s.Node("b0"), s.Node("b1"); first.Equal(second) {
			t.Errorf("Node(%q) and Node(%q) both = %s, want distinct nodes", "b0", "b1", first)
		}
		if got, want := s.Len(), 2; got != want {
			t.Errorf("Len() = %d, want %d", got, want)
		}
	})

	t.Run("the minted label is not the source label", func(t *testing.T) {
		s := rdf.NewBlankNodeScope()

		if got := s.Node("b0").Label(); got == "b0" {
			t.Errorf("Label() = %q, want a minted label rather than the source one", got)
		}
	})

	t.Run("the zero scope is ready to use", func(t *testing.T) {
		var s rdf.BlankNodeScope

		if first, second := s.Node("b0"), s.Node("b0"); !first.Equal(second) {
			t.Errorf("Node(%q) = %s then %s, want the same node", "b0", first, second)
		}
		if got, want := s.Len(), 1; got != want {
			t.Errorf("Len() = %d, want %d", got, want)
		}
	})
}

// TestBlankNodeScopeSeparatesDocuments is the scenario the whole type exists
// for: two documents that each write _:b0 and _:b1 describe four unrelated
// resources, so merging their graphs must leave four blank nodes standing.
func TestBlankNodeScopeSeparatesDocuments(t *testing.T) {
	const predicate = rdf.IRI("http://example.com/p")

	// Both documents write the same two labels, as unrelated files freely may.
	document := []string{"b0", "b1"}

	merged := rdf.NewGraph()
	for _, scope := range []*rdf.BlankNodeScope{rdf.NewBlankNodeScope(), rdf.NewBlankNodeScope()} {
		for _, label := range document {
			node := scope.Node(label)
			if err := merged.Add(rdf.Triple{Subject: node, Predicate: predicate, Object: node}); err != nil {
				t.Fatalf("Add() = %v, want nil", err)
			}
		}
	}

	if got, want := merged.Len(), 4; got != want {
		t.Fatalf("merged graph holds %d triples, want %d", got, want)
	}

	subjects := make(map[rdf.Term]struct{})
	for triple := range merged.All() {
		subjects[triple.Subject] = struct{}{}
	}
	if got, want := len(subjects), 4; got != want {
		t.Errorf("merged graph holds %d distinct blank nodes, want %d", got, want)
	}
}

func TestBlankNodeScopeNew(t *testing.T) {
	t.Run("every fresh node is distinct", func(t *testing.T) {
		s := rdf.NewBlankNodeScope()

		seen := make(map[rdf.BlankNode]struct{})
		for range 100 {
			seen[s.New()] = struct{}{}
		}
		if got, want := len(seen), 100; got != want {
			t.Errorf("minted %d distinct nodes, want %d", got, want)
		}
	})

	t.Run("a fresh node never collides with a mapped one", func(t *testing.T) {
		s := rdf.NewBlankNodeScope()

		// Interleaved so that a fresh node cannot simply be numbered past the
		// mapped ones by luck of ordering.
		seen := make(map[rdf.BlankNode]struct{})
		for _, label := range []string{"b0", "b1", "b2"} {
			seen[s.Node(label)] = struct{}{}
			seen[s.New()] = struct{}{}
		}
		if got, want := len(seen), 6; got != want {
			t.Errorf("minted %d distinct nodes, want %d", got, want)
		}
	})

	t.Run("fresh nodes are not counted as source labels", func(t *testing.T) {
		s := rdf.NewBlankNodeScope()

		s.New()
		s.Node("b0")
		s.New()

		if got, want := s.Len(), 1; got != want {
			t.Errorf("Len() = %d, want %d", got, want)
		}
	})

	t.Run("two scopes never mint the same node", func(t *testing.T) {
		first, second := rdf.NewBlankNodeScope(), rdf.NewBlankNodeScope()

		seen := make(map[rdf.BlankNode]struct{})
		for range 50 {
			seen[first.New()] = struct{}{}
			seen[second.New()] = struct{}{}
		}
		if got, want := len(seen), 100; got != want {
			t.Errorf("minted %d distinct nodes across two scopes, want %d", got, want)
		}
	})
}

func TestBlankNodeScopeSourceLabel(t *testing.T) {
	s := rdf.NewBlankNodeScope()
	mapped := s.Node("theLabelTheAuthorWrote")
	fresh := s.New()

	other := rdf.NewBlankNodeScope()
	foreign := other.Node("b0")

	tests := []struct {
		name  string
		node  rdf.BlankNode
		want  string
		found bool
	}{
		{
			name:  "a mapped node cites the label the document wrote",
			node:  mapped,
			want:  "theLabelTheAuthorWrote",
			found: true,
		},
		{
			name: "a fresh node stands for no source label",
			node: fresh,
		},
		{
			name: "a node from another scope is unknown",
			node: foreign,
		},
		{
			name: "a node this package did not mint is unknown",
			node: rdf.NewBlankNode("b0"),
		},
		{
			name: "the zero blank node is unknown",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, found := s.SourceLabel(test.node)
			if found != test.found {
				t.Fatalf("SourceLabel() found = %t, want %t", found, test.found)
			}
			if got != test.want {
				t.Errorf("SourceLabel() = %q, want %q", got, test.want)
			}
		})
	}
}

// TestBlankNodeScopeAll pins the documented iteration order of the mapping:
// the order in which the source labels were first seen, not the order a map
// happens to range in.
func TestBlankNodeScopeAll(t *testing.T) {
	s := rdf.NewBlankNodeScope()

	want := []string{"zebra", "apple", "b0", "mango"}
	for _, label := range want {
		s.Node(label)
	}
	// Neither a repeat of an earlier label nor a fresh node disturbs the order.
	s.Node(want[0])
	s.New()

	for i := range 2 {
		var got []string
		for label, node := range s.All() {
			got = append(got, label)

			if mapped := s.Node(label); !mapped.Equal(node) {
				t.Errorf("All() yielded %s for %q, want %s", node, label, mapped)
			}
		}
		if !slices.Equal(got, want) {
			t.Errorf("All() pass %d = %v, want %v", i, got, want)
		}
	}
}

func TestBlankNodeScopeAllStopsEarly(t *testing.T) {
	s := rdf.NewBlankNodeScope()
	for _, label := range []string{"b0", "b1", "b2"} {
		s.Node(label)
	}

	var seen int
	for range s.All() {
		seen++
		break
	}
	if want := 1; seen != want {
		t.Errorf("visited %d labels after break, want %d", seen, want)
	}
}

// blankNodeLabel matches the ASCII subset of the N-Triples production
//
//	BLANK_NODE_LABEL ::= '_:' (PN_CHARS_U | [0-9]) ((PN_CHARS | '.')* PN_CHARS)?
//
// which is all a minted label ever uses. A label that failed it would be one no
// printer in this module could write out.
var blankNodeLabel = regexp.MustCompile(`^[A-Za-z_0-9]([A-Za-z_0-9.\-]*[A-Za-z_0-9\-])?$`)

func TestBlankNodeScopeMintsWritableLabels(t *testing.T) {
	s := rdf.NewBlankNodeScope()

	nodes := []rdf.BlankNode{s.Node("b0"), s.New()}
	for range 20 {
		nodes = append(nodes, s.New())
	}

	for _, node := range nodes {
		if label := node.Label(); !blankNodeLabel.MatchString(label) {
			t.Errorf("minted label %q is not a BLANK_NODE_LABEL", label)
		}
	}
}

// TestBlankNodeScopesAreConcurrencySafeToCreate covers the half of the
// concurrency contract that is a guarantee rather than a caveat: independent
// parsers may run in parallel, each with its own scope, and their labels still
// never collide. Under -race it also asserts the shared numbering is not a data
// race.
func TestBlankNodeScopesAreConcurrencySafeToCreate(t *testing.T) {
	const (
		scopes         = 16
		nodesPerScope  = 32
		wantTotalNodes = scopes * nodesPerScope
	)

	var (
		mu   sync.Mutex
		seen = make(map[rdf.BlankNode]struct{}, wantTotalNodes)
		wg   sync.WaitGroup
	)

	wg.Add(scopes)
	for range scopes {
		go func() {
			defer wg.Done()

			// Each goroutine mints into its own scope, as a parser would, and
			// only the collection of results is shared.
			s := rdf.NewBlankNodeScope()
			minted := make([]rdf.BlankNode, 0, nodesPerScope)
			for i := range nodesPerScope {
				minted = append(minted, s.Node(strconv.Itoa(i)))
			}

			mu.Lock()
			defer mu.Unlock()
			for _, node := range minted {
				seen[node] = struct{}{}
			}
		}()
	}
	wg.Wait()

	if got := len(seen); got != wantTotalNodes {
		t.Errorf("%d scopes minted %d distinct nodes, want %d", scopes, got, wantTotalNodes)
	}
}
