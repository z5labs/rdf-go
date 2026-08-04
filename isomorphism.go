package rdf

import (
	"cmp"
	"encoding/binary"
	"hash/fnv"
	"slices"
	"strconv"
	"strings"
)

// maxIsomorphismSteps bounds the backtracking search in [Isomorphic]: the
// number of candidate blank node assignments it will try before giving up.
//
// Colour refinement decides almost every real graph outright, so the search
// exists for the graphs it cannot separate. Those are either highly symmetric,
// where the first bijection tried usually works, or adversarial, where nothing
// short of an exponential search would settle the question. The budget is what
// keeps the second kind from running forever; it is far beyond what any graph
// of conformance-suite size reaches.
const maxIsomorphismSteps = 100_000

// Isomorphic reports whether a and b are the same graph up to blank node
// relabeling — whether some bijection between their blank nodes turns one into
// the other (RDF 1.1 Concepts §3.6). Ground terms have to match exactly; only
// blank nodes are free to be renamed.
//
// This is the comparison the W3C evaluation tests call for. A parser is only
// required to produce a graph isomorphic to the expected one, because the
// labels it invents for blank nodes are its own business and no two
// implementations need agree on them.
//
// A blank node written inside a [TripleTerm] counts as one of the graph's, and
// is renamed by the same bijection: RDF 1.2 Concepts §3.3 gives a graph's blank
// nodes as those occurring in its triples, and a triple term's positions are
// occurrences. So the two graphs
//
//	<s> <p> <<( _:a <q> <o> )>> .
//	<s> <p> <<( _:b <q> <o> )>> .
//
// are isomorphic, as the same pair without the brackets would be.
//
// A nil *Graph is treated as an empty graph.
//
// # Complexity and cutoff
//
// Blank nodes are first partitioned by iterated colour refinement: every node
// is repeatedly recoloured by the multiset of its incident triples, with the
// other blank nodes in them standing in as their current colours, until the
// partition stops getting finer. That costs O(n·m·log m) for n blank nodes and
// m triples, and for almost every graph met in practice it ends with each
// blank node alone in its class, which pins the bijection outright.
//
// What refinement cannot separate is graphs whose blank nodes really are
// interchangeable — n mutually indistinguishable nodes admit n! bijections —
// or the pairs it is simply too weak to tell apart, of which a six-cycle and
// two three-cycles are the standard example. Those fall through to a
// backtracking search that assigns blank nodes in an order that walks outward
// from ones already assigned, so a wrong guess is normally refuted by the very
// next triple rather than at the end of a long chain.
//
// That search is bounded. After maxIsomorphismSteps candidate assignments
// Isomorphic stops and reports false, so a false means "not isomorphic, or not
// decided within the budget". The distinction is unobservable through this
// signature, which is the price of always terminating; reaching the budget at
// all takes a graph built to defeat the refinement, not merely a large one.
func Isomorphic(a, b *Graph) bool {
	if a == nil || b == nil {
		return graphLen(a) == 0 && graphLen(b) == 0
	}
	if a.Len() != b.Len() {
		return false
	}

	x, y := newIsoGraph(a), newIsoGraph(b)
	if len(x.blanks) != len(y.blanks) {
		return false
	}

	// Every bijection maps a ground triple to itself, so the ground triples
	// have to match already. For a pair of ground graphs that settles it:
	// equal size plus containment is set equality.
	for i, t := range x.triples {
		if !x.ground[i] {
			continue
		}
		if _, ok := y.set[t]; !ok {
			return false
		}
	}
	if len(x.blanks) == 0 {
		return true
	}

	xColours, yColours := x.refine(), y.refine()
	xClasses, yClasses := byColour(xColours), byColour(yColours)
	if len(xClasses) != len(yClasses) {
		return false
	}
	for colour, nodes := range xClasses {
		if len(yClasses[colour]) != len(nodes) {
			return false
		}
	}

	m := &matcher{
		a:          x,
		b:          y,
		colours:    xColours,
		candidates: yClasses,
		forward:    make(map[BlankNode]BlankNode, len(x.blanks)),
		used:       make(map[BlankNode]bool, len(y.blanks)),
		budget:     maxIsomorphismSteps,
	}
	return m.match(m.order(xClasses), 0)
}

// graphLen returns the number of triples in g, treating a nil graph as empty.
func graphLen(g *Graph) int {
	if g == nil {
		return 0
	}
	return g.Len()
}

// isoGraph is a graph arranged for isomorphism testing: its triples in folded
// form, indexed both as a set and by the blank nodes that occur in them.
//
// Triples are folded with [keyOf] so that membership here means exactly what
// it means in [Graph] — two literals differing only in the case of a language
// tag are one term, not two.
type isoGraph struct {
	triples  []tripleKey
	set      map[tripleKey]struct{}
	ground   []bool
	blanks   []BlankNode
	incident map[BlankNode][]int
}

// newIsoGraph arranges g for isomorphism testing.
func newIsoGraph(g *Graph) *isoGraph {
	iso := &isoGraph{
		triples:  make([]tripleKey, 0, g.Len()),
		set:      make(map[tripleKey]struct{}, g.Len()),
		ground:   make([]bool, 0, g.Len()),
		incident: make(map[BlankNode][]int),
	}

	for triple := range g.All() {
		t := keyOf(triple)
		i := len(iso.triples)
		iso.triples = append(iso.triples, t)
		iso.set[t] = struct{}{}

		ground := true
		// A predicate is an IRI by construction, so only the subject and the
		// object can hold a blank node.
		for _, term := range [2]Term{t.subject, t.object} {
			eachBlankNode(term, func(node BlankNode) {
				ground = false

				incident := iso.incident[node]
				if incident == nil {
					iso.blanks = append(iso.blanks, node)
				}
				// A triple naming the same blank node twice — as its own
				// subject and object, or twice inside a triple term — is
				// incident to it once, not twice.
				if len(incident) == 0 || incident[len(incident)-1] != i {
					iso.incident[node] = append(incident, i)
				}
			})
		}
		iso.ground = append(iso.ground, ground)
	}

	slices.SortFunc(iso.blanks, compareBlankNodes)
	return iso
}

// eachBlankNode calls yield once for every occurrence of a blank node in term,
// descending into a triple term for the ones nested inside it.
//
// A triple term is a term, so a blank node written in one is a blank node of
// the graph like any other: RDF 1.2 Concepts §3.3 gives the blank nodes of a
// graph as those occurring in its triples, and a triple term's positions are
// occurrences. Nothing here treats one differently for being nested, which is
// what lets a bijection rename it.
func eachBlankNode(term Term, yield func(BlankNode)) {
	switch t := term.(type) {
	case BlankNode:
		yield(t)
	case TripleTerm:
		// A triple term's predicate is an IRI, as a triple's is.
		eachBlankNode(t.Subject, yield)
		eachBlankNode(t.Object, yield)
	}
}

// neighbours returns the blank nodes sharing a triple with b, excluding b
// itself, in a deterministic order.
func (g *isoGraph) neighbours(b BlankNode) []BlankNode {
	var out []BlankNode

	seen := map[BlankNode]bool{b: true}
	for _, i := range g.incident[b] {
		t := g.triples[i]
		for _, term := range [2]Term{t.subject, t.object} {
			eachBlankNode(term, func(node BlankNode) {
				if seen[node] {
					return
				}
				seen[node] = true
				out = append(out, node)
			})
		}
	}

	slices.SortFunc(out, compareBlankNodes)
	return out
}

// refine colours the graph's blank nodes so that two nodes share a colour only
// if no amount of looking at their surroundings tells them apart.
//
// Every node starts the same colour and is then recoloured from its own
// current colour together with the multiset of its incident triples, with each
// other blank node in them standing in as its current colour. Carrying the old
// colour forward means recolouring can only split a class, never merge two, so
// the partition gets strictly finer every round until it stops changing —
// which it must do within one round per blank node, there being no more splits
// to make than nodes to separate. A round that leaves the number of classes
// unchanged has therefore left the partition itself unchanged, which is what
// makes counting them a sound test for having converged.
//
// The colouring is an isomorphism invariant: corresponding nodes of isomorphic
// graphs are given equal colours, because the hash sees only ground terms and
// colours already agreed on. A hash collision can leave the partition coarser
// than it should be, which costs search time but never a wrong answer — the
// bijection is verified against the triples themselves.
func (g *isoGraph) refine() map[BlankNode]uint64 {
	colours := make(map[BlankNode]uint64, len(g.blanks))
	for _, b := range g.blanks {
		colours[b] = 0
	}

	classes := 1
	for range len(g.blanks) {
		next := make(map[BlankNode]uint64, len(g.blanks))
		for _, b := range g.blanks {
			next[b] = g.signature(b, colours)
		}

		refined := countDistinct(next)
		colours = next
		if refined == classes {
			break
		}
		classes = refined
	}
	return colours
}

// signature hashes b's current colour together with the multiset of triples
// incident to it, describing every other blank node in them by its current
// colour and b itself by a marker, so that a node pointing at itself is never
// confused with one pointing at a like-coloured neighbour.
//
// Carrying b's own colour in is what makes this 1-WL rather than a bare
// recolouring by neighbourhood: two nodes already told apart differ here
// whatever their surroundings have since done, so a round can only ever split
// a class.
//
// Recolouring by neighbourhood alone would in fact come to the same thing from
// the seeding [isoGraph.refine] uses — merging two classes at one round takes
// neighbours that merged at the round before, and that regress ends at the
// first round, where every node shares one colour and a lone class has nothing
// to merge with. Carrying the colour makes it a property of the step instead of
// a fact about how the colouring was started, which is the assumption refine
// would otherwise be resting its convergence test on from a distance.
func (g *isoGraph) signature(b BlankNode, colours map[BlankNode]uint64) uint64 {
	incident := g.incident[b]

	parts := make([]string, 0, len(incident))
	for _, i := range incident {
		t := g.triples[i]

		var part strings.Builder
		writeTermSignature(&part, t.subject, b, colours)
		part.WriteByte(' ')
		part.WriteString(t.predicate.String())
		part.WriteByte(' ')
		writeTermSignature(&part, t.object, b, colours)
		parts = append(parts, part.String())
	}
	// Sorting is what makes this a multiset: the order a node's triples happen
	// to sit in the graph is not part of its structure.
	slices.Sort(parts)

	h := fnv.New64a()
	var previous [8]byte
	binary.LittleEndian.PutUint64(previous[:], colours[b])
	h.Write(previous[:])
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return h.Sum64()
}

// writeTermSignature describes term as it appears from self's point of view: a
// ground term by its canonical form, self by a marker, and any other blank
// node by its colour alone, which is all that is known about it so far.
//
// A triple term is written out position by position rather than by its
// canonical form, so that the blank nodes in it are described the same way as
// the ones outside — by colour. Rendering it whole would put their labels in
// the signature, and a label is exactly what a bijection is free to change.
func writeTermSignature(b *strings.Builder, term Term, self BlankNode, colours map[BlankNode]uint64) {
	switch t := term.(type) {
	case BlankNode:
		if t == self {
			b.WriteString("_:self")
			return
		}
		b.WriteString("_:#")
		b.WriteString(strconv.FormatUint(colours[t], 10))
	case TripleTerm:
		b.WriteString("<<( ")
		writeTermSignature(b, t.Subject, self, colours)
		b.WriteByte(' ')
		b.WriteString(t.Predicate.String())
		b.WriteByte(' ')
		writeTermSignature(b, t.Object, self, colours)
		b.WriteString(" )>>")
	default:
		b.WriteString(term.String())
	}
}

// countDistinct returns the number of colours in use.
func countDistinct(colours map[BlankNode]uint64) int {
	seen := make(map[uint64]struct{}, len(colours))
	for _, colour := range colours {
		seen[colour] = struct{}{}
	}
	return len(seen)
}

// byColour groups blank nodes into their colour classes, each in a
// deterministic order so that the search explores candidates the same way on
// every run.
func byColour(colours map[BlankNode]uint64) map[uint64][]BlankNode {
	classes := make(map[uint64][]BlankNode)
	for node, colour := range colours {
		classes[colour] = append(classes[colour], node)
	}
	for _, nodes := range classes {
		slices.SortFunc(nodes, compareBlankNodes)
	}
	return classes
}

// compareBlankNodes orders blank nodes by label.
func compareBlankNodes(a, b BlankNode) int {
	return cmp.Compare(a.label, b.label)
}

// matcher searches for a bijection from the blank nodes of a to those of b.
type matcher struct {
	a, b       *isoGraph
	colours    map[BlankNode]uint64
	candidates map[uint64][]BlankNode
	forward    map[BlankNode]BlankNode
	used       map[BlankNode]bool

	// budget caps steps, the number of candidate assignments tried. Running
	// out sets exhausted, which unwinds the whole search rather than merely
	// failing the branch that hit it.
	budget    int
	steps     int
	exhausted bool
}

// order returns a's blank nodes in the order the search should assign them.
//
// Each connected group of blank nodes is walked breadth-first from the node
// with the fewest candidates, so every node after the first shares a triple
// with one already assigned. That is what makes the consistency check bite: a
// wrong guess is refuted by the next triple instead of surviving until the
// whole bijection is built.
func (m *matcher) order(classes map[uint64][]BlankNode) []BlankNode {
	roots := slices.Clone(m.a.blanks)
	slices.SortFunc(roots, func(p, q BlankNode) int {
		if c := cmp.Compare(len(classes[m.colours[p]]), len(classes[m.colours[q]])); c != 0 {
			return c
		}
		return compareBlankNodes(p, q)
	})

	seen := make(map[BlankNode]bool, len(roots))
	out := make([]BlankNode, 0, len(roots))
	for _, root := range roots {
		if seen[root] {
			continue
		}
		seen[root] = true

		for queue := []BlankNode{root}; len(queue) > 0; {
			node := queue[0]
			queue = queue[1:]
			out = append(out, node)

			for _, neighbour := range m.a.neighbours(node) {
				if seen[neighbour] {
					continue
				}
				seen[neighbour] = true
				queue = append(queue, neighbour)
			}
		}
	}
	return out
}

// match tries to extend the bijection to order[depth:], reporting whether it
// succeeded.
func (m *matcher) match(order []BlankNode, depth int) bool {
	if depth == len(order) {
		return m.verify()
	}

	node := order[depth]
	for _, candidate := range m.candidates[m.colours[node]] {
		if m.exhausted {
			return false
		}
		if m.used[candidate] {
			continue
		}

		m.steps++
		if m.steps > m.budget {
			m.exhausted = true
			return false
		}

		m.forward[node] = candidate
		m.used[candidate] = true
		if m.consistent(node) && m.match(order, depth+1) {
			return true
		}
		delete(m.forward, node)
		delete(m.used, candidate)
	}
	return false
}

// consistent reports whether every triple incident to node that is now fully
// assigned maps to a triple b actually holds.
func (m *matcher) consistent(node BlankNode) bool {
	for _, i := range m.a.incident[node] {
		mapped, complete := m.mapTriple(m.a.triples[i])
		if !complete {
			continue
		}
		if _, ok := m.b.set[mapped]; !ok {
			return false
		}
	}
	return true
}

// verify reports whether the completed bijection carries every triple of a to
// a triple of b. The two graphs hold the same number of triples and a
// bijection on blank nodes maps distinct triples to distinct triples, so
// containment here is equality.
//
// This is where correctness rests. [matcher.consistent] rejects a doomed
// bijection earlier, which is the whole reason the search finishes in
// reasonable time, but it is an optimization: a bijection that reached this
// point is accepted on the strength of this check alone. Nothing about the
// colouring is trusted here.
func (m *matcher) verify() bool {
	for _, t := range m.a.triples {
		// The mapping is total here: order covers every blank node of a, and
		// verify runs only once all of them are assigned. Were that ever to
		// stop holding, an unassigned term would map to nothing and the triple
		// would simply fail to be found, which is the safe way to be wrong.
		mapped, _ := m.mapTriple(t)
		if _, ok := m.b.set[mapped]; !ok {
			return false
		}
	}
	return true
}

// mapTriple carries t across the bijection, reporting false if a blank node in
// it is still unassigned.
func (m *matcher) mapTriple(t tripleKey) (tripleKey, bool) {
	subject, ok := m.mapTerm(t.subject)
	if !ok {
		return tripleKey{}, false
	}
	object, ok := m.mapTerm(t.object)
	if !ok {
		return tripleKey{}, false
	}
	return tripleKey{subject: subject, predicate: t.predicate, object: object}, true
}

// mapTerm carries term across the bijection. A ground term maps to itself, and
// a triple term to one with its own positions carried across, so that a blank
// node nested in it is renamed like any other.
func (m *matcher) mapTerm(term Term) (Term, bool) {
	switch t := term.(type) {
	case BlankNode:
		mapped, ok := m.forward[t]
		return mapped, ok
	case TripleTerm:
		subject, ok := m.mapTerm(t.Subject)
		if !ok {
			return nil, false
		}
		object, ok := m.mapTerm(t.Object)
		if !ok {
			return nil, false
		}
		return TripleTerm{Subject: subject, Predicate: t.Predicate, Object: object}, true
	default:
		return term, true
	}
}
