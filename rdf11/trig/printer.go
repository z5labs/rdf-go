package trig

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"iter"
	"slices"
	"strings"
	"unicode/utf8"

	rdf "github.com/z5labs/rdf-go"
	"github.com/z5labs/rdf-go/iri"
	"github.com/z5labs/rdf-go/vocab"
)

// ErrNilDocument is reported when [Print] is given no document to print.
var ErrNilDocument = errors.New("trig: cannot print a nil document")

// ErrInvalidPrefix is reported when [WithPrefixes] is given a binding that
// cannot be written as a prefix directive: a label that is not a PN_PREFIX, or
// a namespace that is not an absolute IRI.
//
// Both are refused rather than silently dropped. A prefix that cannot be
// written is a prefix the caller expected to see in the output, and a
// namespace that is not absolute would expand every name using it into a
// relative IRI, which the data model has no room for.
var ErrInvalidPrefix = errors.New("trig: invalid prefix binding")

// ErrBaseDirection is reported when [Encode] is given a literal carrying a
// base direction.
//
// The shared term types model RDF 1.2, where a literal may carry one; RDF 1.1
// TriG has no syntax for it. Writing the tag without the direction would
// quietly change what the literal means, so it is refused here and belongs to
// the rdf12/trig package instead.
var ErrBaseDirection = errors.New("trig: base direction has no RDF 1.1 TriG syntax")

// ErrTripleTerm is reported when [Encode] is given a statement whose object is
// a triple term.
//
// The shared term types model RDF 1.2, where a triple used as a term is the
// fourth kind of term; RDF 1.1 TriG has no syntax for one. There is nothing to
// write it as that this package could read back, so it is refused here and
// belongs to the rdf12/trig package instead.
var ErrTripleTerm = errors.New("trig: triple term has no RDF 1.1 TriG syntax")

// Defaults for the layout options.
const (
	// DefaultIndent is one level of indentation for a continuation line.
	DefaultIndent = "    "

	// DefaultLineWidth is the column output is kept within where it can be.
	// It is a target rather than a limit: a single term longer than a line is
	// written whole rather than broken, there being no way to break one.
	DefaultLineWidth = 80
)

// Option configures how [Print] and [Encode] lay a document out.
type Option interface {
	apply(*config)
}

// config is the settled form of the options.
type config struct {
	indent   string
	width    int
	prefixes map[string]string
}

func newConfig(opts []Option) *config {
	cfg := &config{indent: DefaultIndent, width: DefaultLineWidth}
	for _, opt := range opts {
		opt.apply(cfg)
	}
	return cfg
}

// optionFunc adapts a function to the [Option] interface.
type optionFunc func(*config)

func (f optionFunc) apply(cfg *config) { f(cfg) }

// WithIndent sets the string one level of indentation is written as. The
// default is [DefaultIndent].
//
// The string is written as given, so a caller wanting tabs asks for "\t" and
// one wanting everything on as few lines as possible asks for "".
func WithIndent(indent string) Option {
	return optionFunc(func(cfg *config) { cfg.indent = indent })
}

// WithLineWidth sets the column output is kept within. The default is
// [DefaultLineWidth]; a width of zero or less turns wrapping off, so that a
// subject and everything said about it stands on one line.
//
// The width counts characters rather than bytes, so a document written in a
// script outside ASCII wraps where it looks like it should.
func WithLineWidth(width int) Option {
	return optionFunc(func(cfg *config) { cfg.width = width })
}

// WithPrefixes gives [Encode] the prefixes it may abbreviate IRIs with,
// mapping each label to the namespace it stands for.
//
// The label is written without its colon — "ex" and not "ex:" — and the empty
// label is the empty prefix, written ":name". A binding is used only where it
// helps: an IRI is abbreviated against the longest namespace that is a prefix
// of it and whose remainder can be written as a PN_LOCAL, and written out in
// full otherwise. Only the prefixes an abbreviation actually used are declared
// in the output, so an over-large map costs nothing but the lookup.
//
// The map is not copied and must not be modified while encoding.
//
// It has no effect on [Print], which writes back the prefixes the document
// declared.
func WithPrefixes(prefixes map[string]string) Option {
	return optionFunc(func(cfg *config) { cfg.prefixes = prefixes })
}

// Print writes doc to w as TriG.
//
// The document is reproduced rather than translated: a prefixed name is
// written as the prefixed name it was, the "a" keyword as "a", a collection as
// "( ... )", a blank node described in place as "[ ... ]", and a graph block
// with the "GRAPH" keyword it was written with, or without one if it had none.
// What the document meant by any of that is [Decode]'s business, not this
// function's.
//
// The layout is this package's rather than the author's, white space between
// terminals meaning nothing in TriG. A statement begins in the first column;
// the triples of a graph block are indented one level inside its braces; the
// verbs after the first are written one to a line, indented; and an object
// list is kept on one line while it fits within the line width, wrapping to a
// further level of indentation when it does not. All of it is configurable,
// with [WithIndent] and [WithLineWidth].
//
// Comments are kept, each on a line of its own, in the order they were
// written and between the same statements they were written between. A comment
// written inside a statement or inside a graph block follows it rather than
// interrupting it: the layout it was written against is gone, and there is no
// longer anywhere inside a statement for it to stand.
//
// It reports [ErrNilDocument] if doc is nil, and otherwise the first error
// from w.
func Print(w io.Writer, doc *Document, opts ...Option) error {
	if doc == nil {
		return ErrNilDocument
	}

	buf := bufio.NewWriter(w)
	pr := &printer{w: buf, cfg: newConfig(opts)}
	pr.document(doc)
	if pr.err != nil {
		return pr.err
	}
	return buf.Flush()
}

// printer writes to w, remembering the first error rather than returning one
// from every step, and tracking the column so that a line can be broken where
// it grows too long.
type printer struct {
	w   io.Writer
	cfg *config
	err error
	col int
}

// write writes s, doing nothing once an earlier write has failed.
func (pr *printer) write(s string) {
	if pr.err != nil {
		return
	}
	if _, pr.err = io.WriteString(pr.w, s); pr.err != nil {
		return
	}

	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		pr.col = runeLen(s[i+1:])
		return
	}
	pr.col += runeLen(s)
}

// newline ends the line and indents the next one to the given depth.
func (pr *printer) newline(depth int) {
	pr.write("\n")
	if depth > 0 && pr.cfg.indent != "" {
		pr.write(strings.Repeat(pr.cfg.indent, depth))
	}
}

// fits reports whether n more characters stay within the line width. A width
// of zero or less is no width at all, and everything fits.
func (pr *printer) fits(n int) bool {
	return pr.cfg.width <= 0 || pr.col+n <= pr.cfg.width
}

// document writes every statement, putting the comments back between them.
//
// The two are held in separate lists, each already in the order it was
// written, so the document is put back together by merging on position.
func (pr *printer) document(doc *Document) {
	comment := 0
	for _, statement := range doc.Statements {
		comment = pr.commentsBefore(doc.Comments, comment, statement.Position())
		pr.statement(statement)
		pr.write("\n")
	}

	for ; comment < len(doc.Comments); comment++ {
		pr.write(doc.Comments[comment].Text)
		pr.write("\n")
	}
}

// commentsBefore writes the comments from index on that were written before
// pos, returning the index of the first that was not.
func (pr *printer) commentsBefore(comments []*Comment, index int, pos Pos) int {
	for index < len(comments) && earlier(comments[index].Pos, pos) {
		pr.write(comments[index].Text)
		pr.write("\n")
		index++
	}
	return index
}

// earlier reports whether a comes before b in the document.
func earlier(a, b Pos) bool {
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	return a.Column < b.Column
}

// statement writes one directive, one set of triples or one graph block.
//
//	trigDoc ::= (directive | block)*
func (pr *printer) statement(s Statement) {
	switch st := s.(type) {
	case *PrefixDirective:
		pr.prefixDirective(st)
	case *BaseDirective:
		pr.baseDirective(st)
	case *Triples:
		pr.triples(st, 0)
		pr.write(" .")
	case *GraphBlock:
		pr.graphBlock(st)
	default:
		panic(fmt.Sprintf("unknown statement node: %T", s))
	}
}

// graphBlock writes a graph and everything said in it.
//
//	block          ::= ... | wrappedGraph | "GRAPH" labelOrSubject wrappedGraph
//	triplesOrGraph ::= labelOrSubject (wrappedGraph | predicateObjectList '.')
//	wrappedGraph   ::= '{' triplesBlock? '}'
//
// The keyword and the label are written only if the block carried them, so the
// three forms of a block are written back as the form they were read in. Each
// set of triples inside is given a line and its own '.', the one the grammar
// lets the last of them leave off: writing it makes a block of one statement
// read the same as a block of many, and costs a character.
func (pr *printer) graphBlock(b *GraphBlock) {
	if b.Keyword != "" {
		pr.write(b.Keyword)
		pr.write(" ")
	}
	if b.Label != nil {
		pr.write(flat(b.Label))
		pr.write(" ")
	}

	pr.write("{")
	for _, t := range b.Triples {
		pr.newline(1)
		pr.triples(t, 1)
		pr.write(" .")
	}
	pr.newline(0)
	pr.write("}")
}

// prefixDirective writes a prefix binding in the form it was written in.
//
//	prefixID     ::= '@prefix' PNAME_NS IRIREF '.'
//	sparqlPrefix ::= "PREFIX" PNAME_NS IRIREF
//
// Only the '@' form takes a trailing '.', which is why the keyword decides
// what follows.
func (pr *printer) prefixDirective(d *PrefixDirective) {
	keyword := d.Keyword
	if keyword == "" {
		keyword = "@prefix"
	}

	pr.write(keyword)
	pr.write(" ")
	pr.write(d.Prefix)
	pr.write(": ")
	pr.write(flat(d.IRI))
	if isAtKeyword(keyword) {
		pr.write(" .")
	}
}

// baseDirective writes a base declaration in the form it was written in.
//
//	base       ::= '@base' IRIREF '.'
//	sparqlBase ::= "BASE" IRIREF
func (pr *printer) baseDirective(d *BaseDirective) {
	keyword := d.Keyword
	if keyword == "" {
		keyword = "@base"
	}

	pr.write(keyword)
	pr.write(" ")
	pr.write(flat(d.IRI))
	if isAtKeyword(keyword) {
		pr.write(" .")
	}
}

// isAtKeyword reports whether the keyword was written with an '@', which is
// the form the grammar ends with a '.'.
func isAtKeyword(keyword string) bool {
	return strings.HasPrefix(keyword, "@")
}

// triples writes a subject and everything said about it, without the '.' that
// ends it — which is the caller's, a block being free to leave the last one
// off.
//
//	triples ::= subject predicateObjectList
//	          | blankNodePropertyList predicateObjectList?
//
// The second form is why the predicate list may be empty: "[ :p :o ] ." has
// already said everything it has to say.
//
// depth is the indentation of the line the subject stands on, which is one
// level in when the triples are inside a graph block.
func (pr *printer) triples(t *Triples, depth int) {
	pr.term(t.Subject, flat(t.Subject), depth)
	if len(t.Predicates) > 0 {
		pr.write(" ")
		pr.predicateObjectList(t.Predicates, depth)
	}
}

// predicateObjectList writes the verbs given for a subject, one to a line
// after the first.
//
//	predicateObjectList ::= verb objectList (';' (verb objectList)?)*
//
// depth is the indentation of the line the list begins on, so its
// continuations are written one level in from that.
func (pr *printer) predicateObjectList(list []*PredicateObject, depth int) {
	for i, po := range list {
		if i > 0 {
			pr.write(" ;")
			pr.newline(depth + 1)
		}

		pr.write(flat(po.Verb))
		pr.write(" ")
		pr.objectList(po.Objects, depth+1)
	}
}

// objectList writes the objects given for one verb.
//
//	objectList ::= object (',' object)*
//
// The list stays on one line while it fits and wraps a level further in when
// it does not, which keeps a short list short and a long one readable.
func (pr *printer) objectList(objects []Term, depth int) {
	for i, node := range objects {
		single := flat(node)

		// Where the object ends up, which is a level in from the rest of the
		// list when the line had to be broken to fit it. Anything the object
		// itself has to break is indented from there rather than from where
		// the list began.
		at := depth
		if i > 0 {
			pr.write(",")
			if pr.fits(1 + runeLen(single)) {
				pr.write(" ")
			} else {
				at = depth + 1
				pr.newline(at)
			}
		}
		pr.term(node, single, at)
	}
}

// term writes one term. single is its one-line rendering, which is used
// whenever it fits; only a collection and a blank node property list have
// anything else to fall back on.
func (pr *printer) term(node Term, single string, depth int) {
	if pr.fits(runeLen(single)) {
		pr.write(single)
		return
	}

	switch n := node.(type) {
	case *Collection:
		pr.collection(n, depth)
	case *BlankNodePropertyList:
		pr.blankNodePropertyList(n, depth)
	default:
		// A term that is one token long is written whole however long it is.
		// Breaking an IRI or a literal across lines would change it.
		pr.write(single)
	}
}

// collection writes a list too long for its line, one item to a line.
//
//	collection ::= '(' object* ')'
func (pr *printer) collection(c *Collection, depth int) {
	pr.write("(")
	for _, node := range c.Objects {
		pr.newline(depth + 1)
		pr.term(node, flat(node), depth+1)
	}
	pr.newline(depth)
	pr.write(")")
}

// blankNodePropertyList writes a node described in place that is too long for
// its line.
//
//	blankNodePropertyList ::= '[' predicateObjectList ']'
func (pr *printer) blankNodePropertyList(b *BlankNodePropertyList, depth int) {
	pr.write("[")
	pr.newline(depth + 1)
	pr.predicateObjectList(b.Predicates, depth)
	pr.newline(depth)
	pr.write("]")
}

// flat renders a term on a single line, nesting and all.
//
// It is what decides whether a term fits, and what is written when it does, so
// the measurement and the output cannot disagree.
func flat(node Term) string {
	var b strings.Builder
	flatTerm(&b, node)
	return b.String()
}

func flatTerm(b *strings.Builder, node Term) {
	switch n := node.(type) {
	case *IRIRef:
		// The escaping is the rdf package's, which is where the canonical form
		// of an IRI is defined. Writing it a second time here would be a second
		// place for the rules to be got wrong.
		b.WriteString(rdf.IRI(n.Value).String())
	case *PrefixedName:
		flatPrefixedName(b, n)
	case *A:
		b.WriteString("a")
	case *BlankNode:
		b.WriteString("_:")
		b.WriteString(n.Label)
	case *Anon:
		b.WriteString("[]")
	case *Literal:
		flatLiteral(b, n)
	case *Collection:
		flatCollection(b, n)
	case *BlankNodePropertyList:
		b.WriteString("[ ")
		flatPredicateObjectList(b, n.Predicates)
		b.WriteString(" ]")
	default:
		panic(fmt.Sprintf("unknown term node: %T", node))
	}
}

func flatCollection(b *strings.Builder, c *Collection) {
	if len(c.Objects) == 0 {
		b.WriteString("()")
		return
	}

	b.WriteString("( ")
	for i, node := range c.Objects {
		if i > 0 {
			b.WriteString(" ")
		}
		flatTerm(b, node)
	}
	b.WriteString(" )")
}

func flatPredicateObjectList(b *strings.Builder, list []*PredicateObject) {
	for i, po := range list {
		if i > 0 {
			b.WriteString(" ; ")
		}

		flatTerm(b, po.Verb)
		for j, node := range po.Objects {
			if j > 0 {
				b.WriteString(",")
			}
			b.WriteString(" ")
			flatTerm(b, node)
		}
	}
}

// flatPrefixedName writes an abbreviated name, escaping its local part.
//
//	PNAME_NS ::= PN_PREFIX? ':'
//	PNAME_LN ::= PNAME_NS PN_LOCAL
func flatPrefixedName(b *strings.Builder, n *PrefixedName) {
	b.WriteString(n.Prefix)
	b.WriteString(":")
	if !n.HasLocal {
		return
	}

	local, _ := escapeLocalName(n.Local)
	b.WriteString(local)
}

// flatLiteral writes a literal in the form the syntax tree says it was written
// in.
//
//	literal    ::= RDFLiteral | NumericLiteral | BooleanLiteral
//	RDFLiteral ::= String (LANGTAG | '^^' iri)?
//
// A number or a boolean written as itself is written back that way, and every
// quoted string is written in the short double-quoted form whichever of the
// four it arrived in: the four differ in what they can hold, not in what they
// mean, and this one can hold anything once it is escaped.
func flatLiteral(b *strings.Builder, l *Literal) {
	switch l.Kind {
	case LiteralInteger, LiteralDecimal, LiteralDouble, LiteralBoolean:
		b.WriteString(l.Value)
		return
	}

	// The quoted form is the rdf package's, which escapes exactly the four
	// characters STRING_LITERAL_QUOTE excludes — '"', '\', line feed and
	// carriage return — so what is written re-reads as the value it was made
	// from.
	b.WriteString(rdf.NewLiteral(l.Value).String())

	switch {
	case l.Language != "":
		b.WriteString("@")
		b.WriteString(l.Language)
	case l.Datatype != nil:
		b.WriteString("^^")
		flatTerm(b, l.Datatype)
	}
}

// escapeLocalName renders s as a PN_LOCAL, reporting whether it could be
// rendered as one at all.
//
//	PN_LOCAL     ::= (PN_CHARS_U | ':' | [0-9] | PLX)
//	                 ((PN_CHARS | '.' | ':' | PLX)* (PN_CHARS | ':' | PLX))?
//	PLX          ::= PERCENT | PN_LOCAL_ESC
//	PERCENT      ::= '%' HEX HEX
//	PN_LOCAL_ESC ::= '\' ('_' | '~' | '.' | '-' | '!' | '$' | '&' | "'" | '(' |
//	                      ')' | '*' | '+' | ',' | ';' | '=' | '/' | '?' | '#' |
//	                      '@' | '%')
//
// Only what has to be escaped is: a '.' may stand inside a name but not at
// either end of one, a '-' may stand anywhere but first, and a '%' is a
// percent escape when two hexadecimal digits follow it and an escaped
// character when they do not. Escaping the rest as well would be legal and
// unreadable.
//
// A character that PN_LOCAL has no room for at all is written as itself and
// reported false. [Print] writes what it is given, a tree built by hand being
// the caller's business; [Encode] uses the report to write the IRI out in full
// instead.
func escapeLocalName(s string) (string, bool) {
	var b strings.Builder
	b.Grow(len(s))

	ok := true
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		first, last := i == 0, i+size == len(s)

		switch {
		case r == '%':
			if i+2 < len(s) && isHexDigit(s[i+1]) && isHexDigit(s[i+2]) {
				b.WriteString(s[i : i+3])
				i += 3
				continue
			}
			b.WriteString(`\%`)

		case r == '.':
			if first || last {
				b.WriteString(`\.`)
			} else {
				b.WriteByte('.')
			}

		case r == '-':
			if first {
				b.WriteString(`\-`)
			} else {
				b.WriteByte('-')
			}

		case r == '_':
			// PN_CHARS_U, so it needs no escape anywhere.
			b.WriteByte('_')

		case isPNLocalEsc(r):
			b.WriteByte('\\')
			b.WriteRune(r)

		case r == ':' || isPNChars(r):
			if first && !(isPNCharsU(r) || r == ':' || isDigit(r)) {
				// A PN_CHARS that is not a PN_CHARS_U — a combining mark, say —
				// may not begin a local name, and has no escaped form.
				ok = false
			}
			b.WriteRune(r)

		default:
			ok = false
			b.WriteRune(r)
		}
		i += size
	}
	return b.String(), ok
}

func isHexDigit(c byte) bool {
	_, ok := hexValue(rune(c))
	return ok
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// ErrNilDataset is reported when [Encode] is given no dataset to write.
var ErrNilDataset = errors.New("trig: cannot encode a nil dataset")

// Encode writes dataset to w as TriG.
//
// This is the other direction of [Decode]: it takes what a dataset means and
// chooses a way of writing it. The default graph is written as statements
// standing outside every block, and each named graph as a block introduced by
// its label — the form TriG is usually written in, and the shortest, the
// "GRAPH" keyword being optional wherever a label stands. A named graph the
// dataset holds and says nothing in is written as an empty block, which is the
// one thing a dataset can say and a sequence of quads cannot.
//
// Inside a graph the choices are Turtle's. Statements about one subject are
// gathered into one, its verbs separated by ';' and the objects of a verb by
// ',', rdf:type is written "a", and an IRI is abbreviated against the prefixes
// given by [WithPrefixes] — the three things that make a document readable
// rather than a list of quads in longer clothes.
//
// Graphs are written in the order [rdf.Dataset] visits them, and within a graph
// subjects in the order they first arrive and the verbs of a subject in the
// order they first arrive under it, so encoding one dataset twice writes the
// same bytes.
//
// A number, a boolean and a plain string are written in their short forms
// wherever the lexical form allows — 42 rather than "42"^^xsd:integer — and in
// full wherever it does not, so that reading the output back yields the
// literal that was written. A blank node keeps the label it carries when TriG
// can spell it and is given one it can otherwise; either way two mentions of
// one node stay one node, whether they stand in one graph or in several, and
// whether they stand as a term or as a graph label.
//
// Unlike [Decode] this does not stream, a dataset being in memory already.
// A caller encoding more than fits in memory wants N-Quads, which needs no
// grouping.
//
// It reports [ErrNilDataset] for a nil dataset, [ErrInvalidPrefix] for a prefix
// binding that cannot be written, [ErrBaseDirection] for a literal that RDF 1.1
// TriG cannot spell, [ErrTripleTerm] for an object that is a triple term, the
// error from [rdf.Quad.Validate] for a quad that is not one, and otherwise the
// first error from w.
//
// Nothing is written until the whole dataset has been read, so all of those
// leave w untouched: a dataset this package refuses is refused before any of it
// has been written. An error from w itself is a different matter, and leaves
// behind whatever reached w before it — a caller that must not act on a partial
// document should encode into a buffer of its own and write that on once Encode
// returns nil.
func Encode(w io.Writer, dataset *rdf.Dataset, opts ...Option) error {
	if dataset == nil {
		return ErrNilDataset
	}

	cfg := newConfig(opts)

	e, err := newEncoder(cfg)
	if err != nil {
		return err
	}

	doc, err := e.document(dataset)
	if err != nil {
		return err
	}
	return Print(w, doc, opts...)
}

// encoder turns data model triples into the syntax tree that says them.
//
// Going through the tree rather than writing bytes directly is what keeps
// [Encode] and [Print] laying a document out the same way: there is one
// printer, and this decides only what to write, never where.
type encoder struct {
	cfg *config

	// bindings are the prefixes, longest namespace first, so that the first
	// one that matches an IRI is the most specific.
	bindings []binding

	// used names the prefixes an abbreviation has actually used, which are the
	// only ones worth declaring.
	used map[string]bool

	// labels maps a blank node's label to the one it is written with, and
	// taken is every label already written, so that a label this encoder
	// invented cannot collide with one it has yet to read.
	labels map[string]string
	taken  map[string]bool
	next   int
}

// binding is one prefix and the namespace it stands for.
type binding struct {
	prefix    string
	namespace string
}

func newEncoder(cfg *config) (*encoder, error) {
	e := &encoder{
		cfg:    cfg,
		used:   make(map[string]bool),
		labels: make(map[string]string),
		taken:  make(map[string]bool),
	}

	for prefix, namespace := range cfg.prefixes {
		if !isValidPrefix(prefix) {
			return nil, fmt.Errorf("%w: %q is not a PN_PREFIX", ErrInvalidPrefix, prefix)
		}
		if !iri.IsAbsolute(namespace) {
			return nil, fmt.Errorf(
				"%w: namespace %q of prefix %q is not an absolute IRI",
				ErrInvalidPrefix, namespace, prefix,
			)
		}
		e.bindings = append(e.bindings, binding{prefix: prefix, namespace: namespace})
	}

	// Longest namespace first, and by prefix where two are the same length, so
	// that a map iterated in any order still abbreviates the same way.
	slices.SortFunc(e.bindings, func(a, b binding) int {
		if n := len(b.namespace) - len(a.namespace); n != 0 {
			return n
		}
		return strings.Compare(a.prefix, b.prefix)
	})
	return e, nil
}

// subjectGroup is everything said about one subject, and where under it each
// verb was put.
type subjectGroup struct {
	triples *Triples
	verbs   map[string]int
}

// document builds the tree for a dataset: the default graph as statements
// standing on their own, and each named graph as a block.
//
// The tree is built before anything is written, and the blank node labels are
// settled as the terms are read, which is what lets a node mentioned in two
// graphs be written with one label in both.
func (e *encoder) document(dataset *rdf.Dataset) (*Document, error) {
	defaults, err := e.graph(dataset.Default().All())
	if err != nil {
		return nil, err
	}

	// Every named graph, empty ones included: GraphNames visits a graph the
	// dataset holds and says nothing in, which All does not.
	var blocks []*GraphBlock
	for name := range dataset.GraphNames() {
		g, ok := dataset.NamedGraph(name)
		if !ok {
			// Unreachable: GraphNames only names graphs the dataset holds.
			continue
		}

		label, err := e.label(name)
		if err != nil {
			return nil, err
		}

		triples, err := e.graph(g.All())
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, &GraphBlock{Label: label, Triples: triples})
	}

	// The prefix directives come last, being known only once every IRI has
	// been abbreviated, and are written first, a directive applying to what
	// follows it.
	doc := &Document{Pos: Pos{Line: 1, Column: 1}}
	for _, prefix := range e.declared() {
		doc.Statements = append(doc.Statements, &PrefixDirective{
			Keyword: "@prefix",
			Prefix:  prefix,
			IRI:     &IRIRef{Value: e.cfg.prefixes[prefix]},
		})
	}
	for _, t := range defaults {
		doc.Statements = append(doc.Statements, t)
	}
	for _, b := range blocks {
		doc.Statements = append(doc.Statements, b)
	}
	return doc, nil
}

// label builds the node naming a graph.
//
//	labelOrSubject ::= iri | BlankNode
//
// A blank node label goes through the same mapping a term does, so a node used
// both to name a graph and to stand in one is written the same way in both
// places.
func (e *encoder) label(name rdf.Term) (Term, error) {
	// [rdf.Dataset.AddGraph] refuses a name the data model does not allow, so
	// this only fires for a dataset built by some other means.
	if err := validateGraphName(name); err != nil {
		return nil, err
	}

	switch n := name.(type) {
	case rdf.IRI:
		return e.iri(n), nil
	case rdf.BlankNode:
		return e.blankNode(n), nil
	default:
		panic(fmt.Sprintf("unknown graph name: %T", name))
	}
}

// graph builds the tree for one graph's triples, gathering the statements
// about one subject into one.
func (e *encoder) graph(triples iter.Seq[rdf.Triple]) ([]*Triples, error) {
	var order []string
	groups := make(map[string]*subjectGroup)

	for triple := range triples {
		if err := triple.Validate(); err != nil {
			return nil, err
		}

		// The canonical N-Triples form of a term is unique to it, which makes
		// it a key without a type of its own.
		key := triple.Subject.String()

		group, ok := groups[key]
		if !ok {
			subject, err := e.term(triple.Subject)
			if err != nil {
				return nil, err
			}

			group = &subjectGroup{
				triples: &Triples{Subject: subject},
				verbs:   make(map[string]int),
			}
			groups[key] = group
			order = append(order, key)
		}

		at, ok := group.verbs[string(triple.Predicate)]
		if !ok {
			at = len(group.triples.Predicates)
			group.verbs[string(triple.Predicate)] = at
			group.triples.Predicates = append(
				group.triples.Predicates,
				&PredicateObject{Verb: e.verb(triple.Predicate)},
			)
		}

		object, err := e.term(triple.Object)
		if err != nil {
			return nil, err
		}
		po := group.triples.Predicates[at]
		po.Objects = append(po.Objects, object)
	}

	list := make([]*Triples, 0, len(order))
	for _, key := range order {
		list = append(list, groups[key].triples)
	}
	return list, nil
}

// declared returns the prefixes worth a directive, in a fixed order.
func (e *encoder) declared() []string {
	prefixes := make([]string, 0, len(e.used))
	for prefix := range e.used {
		prefixes = append(prefixes, prefix)
	}
	slices.Sort(prefixes)
	return prefixes
}

// term builds the node for one data model term.
func (e *encoder) term(t rdf.Term) (Term, error) {
	switch n := t.(type) {
	case rdf.IRI:
		return e.iri(n), nil
	case rdf.BlankNode:
		return e.blankNode(n), nil
	case rdf.Literal:
		return e.literal(n)
	case rdf.TripleTerm:
		return nil, fmt.Errorf("%w: %s", ErrTripleTerm, n)
	default:
		panic(fmt.Sprintf("unknown term: %T", t))
	}
}

// verb builds the node for a predicate, which is "a" when it is rdf:type.
//
//	verb ::= predicate | 'a'
func (e *encoder) verb(predicate rdf.IRI) Verb {
	if predicate == vocab.RDFType {
		return &A{}
	}
	return e.iri(predicate)
}

// iri builds the node for an IRI, abbreviating it where a prefix allows.
//
// The bindings are tried longest namespace first, so the most specific one
// wins, and a namespace whose remainder cannot be written as a PN_LOCAL is
// passed over rather than written wrongly: what is abbreviated has to expand
// back to the IRI it came from.
func (e *encoder) iri(value rdf.IRI) Verb {
	for _, b := range e.bindings {
		rest, ok := strings.CutPrefix(string(value), b.namespace)
		if !ok {
			continue
		}

		if rest == "" {
			e.used[b.prefix] = true
			return &PrefixedName{Prefix: b.prefix}
		}
		if _, ok := escapeLocalName(rest); ok {
			e.used[b.prefix] = true
			return &PrefixedName{Prefix: b.prefix, Local: rest, HasLocal: true}
		}
	}
	return &IRIRef{Value: string(value)}
}

// blankNode builds the node for a blank node, giving it a label TriG can
// write.
//
//	BLANK_NODE_LABEL ::= '_:' (PN_CHARS_U | [0-9]) ((PN_CHARS | '.')* PN_CHARS)?
//
// A label the production admits is kept, so that output stays recognisable to
// whoever wrote the input. One it does not — or one already given to another
// node — is replaced, and the replacement is remembered, so that two mentions
// of one node stay one node.
func (e *encoder) blankNode(b rdf.BlankNode) *BlankNode {
	if label, ok := e.labels[b.Label()]; ok {
		return &BlankNode{Label: label}
	}

	label := b.Label()
	if !isValidBlankNodeLabel(label) || e.taken[label] {
		label = e.mint()
	}
	e.labels[b.Label()] = label
	e.taken[label] = true
	return &BlankNode{Label: label}
}

// mint returns a label no node has been written with yet.
func (e *encoder) mint() string {
	for {
		label := "b" + itoa(e.next)
		e.next++
		if !e.taken[label] {
			return label
		}
	}
}

// literal builds the node for a literal, using the short form of one written
// as itself wherever the lexical form allows.
//
//	true, false  xsd:boolean
//	42           xsd:integer
//	3.14         xsd:decimal
//	1e6          xsd:double
//
// The lexical form has to match the production exactly, since it is what is
// written: "0042"^^xsd:integer may be shortened to 0042, which reads back as
// the same literal, while "1.0"^^xsd:double may not be shortened to 1.0, which
// reads back as an xsd:decimal.
func (e *encoder) literal(l rdf.Literal) (*Literal, error) {
	if l.Direction() != rdf.DirectionNone {
		return nil, fmt.Errorf("%w: %s", ErrBaseDirection, l.String())
	}

	if language := l.Language(); language != "" {
		return &Literal{Kind: LiteralString, Value: l.Value(), Language: language}, nil
	}

	datatype := l.Datatype()
	if kind, ok := shorthandKind(datatype, l.Value()); ok {
		return &Literal{Kind: kind, Value: l.Value()}, nil
	}
	if datatype == rdf.XSDString {
		// The datatype every string carries, and the one TriG leaves unsaid.
		return &Literal{Kind: LiteralString, Value: l.Value()}, nil
	}
	return &Literal{
		Kind:     LiteralString,
		Value:    l.Value(),
		Datatype: e.iri(datatype),
	}, nil
}

// shorthandKind returns the kind a literal may be written as without quotes,
// reporting false when its datatype has no short form or its lexical form does
// not match the production that would carry it.
func shorthandKind(datatype rdf.IRI, value string) (LiteralKind, bool) {
	switch datatype {
	case vocab.XSDBoolean:
		return LiteralBoolean, value == "true" || value == "false"
	case vocab.XSDInteger:
		return LiteralInteger, matchesInteger(value)
	case vocab.XSDDecimal:
		return LiteralDecimal, matchesDecimal(value)
	case vocab.XSDDouble:
		return LiteralDouble, matchesDouble(value)
	default:
		return LiteralString, false
	}
}

// matchesInteger reports whether s is an INTEGER.
//
//	INTEGER ::= [+-]? [0-9]+
func matchesInteger(s string) bool {
	return someDigits(trimSign(s))
}

// matchesDecimal reports whether s is a DECIMAL.
//
//	DECIMAL ::= [+-]? [0-9]* '.' [0-9]+
func matchesDecimal(s string) bool {
	whole, fraction, ok := strings.Cut(trimSign(s), ".")
	return ok && anyDigits(whole) && someDigits(fraction)
}

// matchesDouble reports whether s is a DOUBLE.
//
//	DOUBLE   ::= [+-]? ([0-9]+ '.' [0-9]* EXPONENT | '.' [0-9]+ EXPONENT
//	                    | [0-9]+ EXPONENT)
//	EXPONENT ::= [eE] [+-]? [0-9]+
func matchesDouble(s string) bool {
	at := strings.IndexAny(s, "eE")
	if at < 0 {
		return false
	}

	mantissa, exponent := trimSign(s[:at]), trimSign(s[at+1:])
	if !someDigits(exponent) {
		return false
	}

	whole, fraction, ok := strings.Cut(mantissa, ".")
	if !ok {
		return someDigits(mantissa)
	}
	if whole == "" {
		return someDigits(fraction)
	}
	return someDigits(whole) && anyDigits(fraction)
}

// trimSign drops the leading sign a numeric literal may carry.
func trimSign(s string) string {
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		return s[1:]
	}
	return s
}

// someDigits reports whether s is one or more digits and nothing else.
func someDigits(s string) bool { return s != "" && anyDigits(s) }

// anyDigits reports whether s is digits and nothing else, the empty string
// included.
func anyDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// isValidPrefix reports whether s may be written as the label of a prefix.
//
//	PN_PREFIX ::= PN_CHARS_BASE ((PN_CHARS | '.')* PN_CHARS)?
//
// The empty label is the empty prefix, written ":name", and is admitted by
// PNAME_NS rather than by this production.
func isValidPrefix(s string) bool {
	if s == "" {
		return true
	}

	var last rune
	for i, r := range s {
		switch {
		case i == 0:
			if !isPNCharsBase(r) {
				return false
			}
		case r == '.':
		default:
			if !isPNChars(r) {
				return false
			}
		}
		last = r
	}
	return last != '.'
}

// isValidBlankNodeLabel reports whether s may be written after a "_:".
//
//	BLANK_NODE_LABEL ::= '_:' (PN_CHARS_U | [0-9]) ((PN_CHARS | '.')* PN_CHARS)?
func isValidBlankNodeLabel(s string) bool {
	if s == "" {
		return false
	}

	var last rune
	for i, r := range s {
		switch {
		case i == 0:
			if !isPNCharsU(r) && !isDigit(r) {
				return false
			}
		case r == '.':
		default:
			if !isPNChars(r) {
				return false
			}
		}
		last = r
	}
	return last != '.'
}
