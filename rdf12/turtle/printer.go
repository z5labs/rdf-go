// This file began as a copy of rdf11/turtle/printer.go and keeps its shape: the
// same options, the same two-pass layout — a flat rendering that decides whether
// a term fits and is written when it does, and a broken one for when it does not
// — and the same encoder that builds a syntax tree rather than writing bytes, so
// that [Print] is the only thing that decides where anything goes.
//
// What RDF 1.2 adds is four more things to write back: a version directive, a
// triple term, a reified triple, and an annotation. The first three are terms or
// statements and go where their RDF 1.1 equivalents go. The fourth does not: an
// annotation is written after one object of an object list and belongs to that
// object, so the printer walks [Object] where the RDF 1.1 one walked [Term].
//
// The two errors the RDF 1.1 printer has for a term it cannot spell —
// ErrBaseDirection and ErrTripleTerm — are absent here, RDF 1.2 Turtle having a
// syntax for both.

package turtle

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
	"github.com/z5labs/rdf-go/internal/lex"
	"github.com/z5labs/rdf-go/iri"
	"github.com/z5labs/rdf-go/vocab"
)

// ErrNilDocument is reported when [Print] is given no document to print.
var ErrNilDocument = errors.New("turtle: cannot print a nil document")

// ErrInvalidPrefix is reported when [WithPrefixes] is given a binding that
// cannot be written as a prefix directive: a label that is not a PN_PREFIX, or
// a namespace that is not an absolute IRI.
//
// Both are refused rather than silently dropped. A prefix that cannot be
// written is a prefix the caller expected to see in the output, and a
// namespace that is not absolute would expand every name using it into a
// relative IRI, which the data model has no room for.
var ErrInvalidPrefix = errors.New("turtle: invalid prefix binding")

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

// Print writes doc to w as RDF 1.2 Turtle.
//
// The document is reproduced rather than translated: a prefixed name is written
// as the prefixed name it was, the "a" keyword as "a", a collection as
// "( ... )" and a blank node described in place as "[ ... ]". What the document
// meant by any of that is [Decode]'s business, not this function's.
//
// The RDF 1.2 constructs are reproduced on the same terms, which is the whole
// point of keeping them apart in the syntax tree:
//
//   - a triple term is written "<<( s p o )>>" and a reified triple
//     "<< s p o >>", each with the reifier the author wrote it with, so the two
//     do not collapse into one another. They mean different things — a triple
//     term is a term of the data model, a reified triple is sugar for an
//     rdf:reifies triple relating its reifier to one — and a document that
//     wrote one gets it back.
//   - an annotation is written after the object it was written after, in the
//     order its reifiers and blocks were written in, since "~ :r {| ... |}" and
//     "{| ... |} ~ :r" do not say the same thing.
//   - a version directive keeps its place among the statements, a directive
//     announcing the version of what follows it rather than of the document.
//   - a directional literal keeps its "--ltr" or "--rtl", which is what
//     separates it from a literal carrying only a language tag.
//
// The layout is this package's rather than the author's, white space between
// terminals meaning nothing in Turtle. A statement begins in the first column;
// the verbs after the first are written one to a line, indented; and an object
// list is kept on one line while it fits within the line width, wrapping to a
// further level of indentation when it does not. Both are configurable, with
// [WithIndent] and [WithLineWidth].
//
// A collection, a blank node property list and an annotation block are broken
// across lines when they do not fit. The two bracketed 1.2 forms are not: a
// triple term and a reified triple are one statement written where a term is
// wanted, and breaking them would spread three terms over three lines to no
// one's benefit. Either is written whole however long it is, as a long IRI is.
//
// Comments are kept, each on a line of its own, in the order they were written
// and between the same statements they were written between. A comment written
// inside a statement follows it rather than interrupting it: the layout it was
// written against is gone, and there is no longer anywhere inside a statement
// for it to stand.
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

// statement writes one directive or one set of triples.
//
//	statement ::= directive | triples '.'
//	directive ::= prefixID | base | version | sparqlPrefix | sparqlBase | sparqlVersion
func (pr *printer) statement(s Statement) {
	switch st := s.(type) {
	case *PrefixDirective:
		pr.prefixDirective(st)
	case *BaseDirective:
		pr.baseDirective(st)
	case *VersionDirective:
		pr.versionDirective(st)
	case *Triples:
		pr.triples(st)
	default:
		panic(fmt.Sprintf("unknown statement node: %T", s))
	}
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

// versionDirective writes a version announcement in the form it was written in.
//
//	version          ::= '@version' VersionSpecifier '.'
//	sparqlVersion    ::= "VERSION" VersionSpecifier
//	VersionSpecifier ::= STRING_LITERAL_QUOTE | STRING_LITERAL_SINGLE_QUOTE
//
// The specifier is written in the double-quoted form whichever of the two it
// arrived in, and escaped as any short string is. The two differ in which quote
// they have to escape, not in what they can say.
func (pr *printer) versionDirective(d *VersionDirective) {
	keyword := d.Keyword
	if keyword == "" {
		keyword = "@version"
	}

	pr.write(keyword)
	pr.write(" ")
	pr.write(quoted(d.Version))
	if isAtKeyword(keyword) {
		pr.write(" .")
	}
}

// isAtKeyword reports whether the keyword was written with an '@', which is
// the form the grammar ends with a '.'.
func isAtKeyword(keyword string) bool {
	return strings.HasPrefix(keyword, "@")
}

// triples writes a subject and everything said about it.
//
//	triples ::= subject predicateObjectList
//	          | blankNodePropertyList predicateObjectList?
//	          | reifiedTriple predicateObjectList?
//
// The second and third forms are why the predicate list may be empty:
// "[ :p :o ] ." and "<< :s :p :o >> ." have each already said everything they
// have to say.
func (pr *printer) triples(t *Triples) {
	pr.term(t.Subject, flat(t.Subject), 0)
	if len(t.Predicates) > 0 {
		pr.write(" ")
		pr.predicateObjectList(t.Predicates, 0)
	}
	pr.write(" .")
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
//	objectList ::= object annotation (',' object annotation)*
//
// The list stays on one line while it fits and wraps a level further in when
// it does not, which keeps a short list short and a long one readable. What is
// measured is the object together with its annotation, the two being written as
// one run of text.
func (pr *printer) objectList(objects []*Object, depth int) {
	for i, node := range objects {
		single := flatObject(node)

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
		pr.object(node, single, at)
	}
}

// object writes one object of an object list and whatever was written after it.
//
//	objectList ::= object annotation (',' object annotation)*
//	annotation ::= (reifier | annotationBlock)*
//
// single is the one-line rendering of both together, used whenever it fits. It
// is only when it does not that the two are written separately, so that the
// object may break where it can and each annotation block after it may break on
// its own.
func (pr *printer) object(node *Object, single string, depth int) {
	if pr.fits(runeLen(single)) {
		pr.write(single)
		return
	}

	pr.term(node.Term, flat(node.Term), depth)
	for _, item := range node.Annotation {
		pr.annotation(item, depth)
	}
}

// annotation writes one reifier or annotation block written after an object.
//
//	annotation ::= (reifier | annotationBlock)*
//
// A reifier is two tokens at most and is always written whole. A block holds a
// predicate object list and is broken like one when it does not fit.
func (pr *printer) annotation(item Annotation, depth int) {
	pr.write(" ")

	switch n := item.(type) {
	case *Reifier:
		pr.write(flat(n))
	case *AnnotationBlock:
		single := flat(n)
		if pr.fits(runeLen(single)) {
			pr.write(single)
			return
		}
		pr.annotationBlock(n, depth)
	default:
		panic(fmt.Sprintf("unknown annotation node: %T", item))
	}
}

// annotationBlock writes a block too long for its line.
//
//	annotationBlock ::= '{|' predicateObjectList '|}'
func (pr *printer) annotationBlock(b *AnnotationBlock, depth int) {
	pr.write("{|")
	pr.newline(depth + 1)
	pr.predicateObjectList(b.Predicates, depth)
	pr.newline(depth)
	pr.write("|}")
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
		// Breaking an IRI or a literal across lines would change it. A triple
		// term and a reified triple are longer than one token and are written
		// whole too: they are one statement standing where a term is wanted,
		// and there is nothing to be gained by spreading three terms over three
		// lines.
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
//
// It takes a [Term] and an [Annotation] alike, the two having no type in common
// and every caller wanting the same thing of them.
func flat(node any) string {
	var b strings.Builder
	flatNode(&b, node)
	return b.String()
}

// flatObject renders an object together with the annotation written after it.
func flatObject(o *Object) string {
	var b strings.Builder
	flatNode(&b, o.Term)
	for _, item := range o.Annotation {
		b.WriteString(" ")
		flatNode(&b, item)
	}
	return b.String()
}

// flatNode writes one node of the syntax tree on a single line.
func flatNode(b *strings.Builder, node any) {
	switch n := node.(type) {
	case *IRIRef:
		// The escaping is the rdf package's, which is where the canonical form
		// of an IRI is defined. Writing it a second time here would be a second
		// place for the rules to be got wrong, and RDF 1.2 did not change it.
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
	case *TripleTerm:
		flatTripleTerm(b, n)
	case *ReifiedTriple:
		flatReifiedTriple(b, n)
	case *Reifier:
		flatReifier(b, n)
	case *AnnotationBlock:
		b.WriteString("{| ")
		flatPredicateObjectList(b, n.Predicates)
		b.WriteString(" |}")
	default:
		panic(fmt.Sprintf("unknown node: %T", node))
	}
}

// flatTripleTerm writes a triple standing as a term.
//
//	tripleTerm ::= '<<(' ttSubject verb ttObject ')>>'
//
// Its object goes back through [flatNode], so a nested triple term nests as
// deeply as the document wrote it.
func flatTripleTerm(b *strings.Builder, t *TripleTerm) {
	b.WriteString("<<( ")
	flatNode(b, t.Subject)
	b.WriteString(" ")
	flatNode(b, t.Verb)
	b.WriteString(" ")
	flatNode(b, t.Object)
	b.WriteString(" )>>")
}

// flatReifiedTriple writes a triple written where a term is wanted.
//
//	reifiedTriple ::= '<<' rtSubject verb rtObject reifier? '>>'
//
// The reifier is written only where the document wrote one. A reifier left out
// and a reifier written bare mean the same thing — both mint a blank node — and
// a printer that supplied the missing '~' would be writing something the author
// did not.
func flatReifiedTriple(b *strings.Builder, r *ReifiedTriple) {
	b.WriteString("<< ")
	flatNode(b, r.Subject)
	b.WriteString(" ")
	flatNode(b, r.Verb)
	b.WriteString(" ")
	flatNode(b, r.Object)
	if r.Reifier != nil {
		b.WriteString(" ")
		flatReifier(b, r.Reifier)
	}
	b.WriteString(" >>")
}

// flatReifier writes the identifier a reified triple or an annotation is given.
//
//	reifier ::= '~' (iri | BlankNode)?
//
// A '~' with nothing after it is written as itself: it is what the document
// wrote, and the blank node it stands for has no name to write.
func flatReifier(b *strings.Builder, r *Reifier) {
	b.WriteString("~")
	if r.ID == nil {
		return
	}
	b.WriteString(" ")
	flatNode(b, r.ID)
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
		flatNode(b, node)
	}
	b.WriteString(" )")
}

func flatPredicateObjectList(b *strings.Builder, list []*PredicateObject) {
	for i, po := range list {
		if i > 0 {
			b.WriteString(" ; ")
		}

		flatNode(b, po.Verb)
		for j, object := range po.Objects {
			if j > 0 {
				b.WriteString(",")
			}
			b.WriteString(" ")
			b.WriteString(flatObject(object))
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
//	RDFLiteral ::= String (LANG_DIR | '^^' iri)?
//	LANG_DIR   ::= '@' [a-zA-Z]+ ('-' [a-zA-Z0-9]+)* ('--' [a-zA-Z]+)?
//
// A number or a boolean written as itself is written back that way, and every
// quoted string is written in the short double-quoted form whichever of the
// four it arrived in: the four differ in what they can hold, not in what they
// mean, and this one can hold anything once it is escaped.
//
// A base direction is written only alongside a language tag. LANG_DIR has no
// spelling for one without a tag — the production demands a letter before the
// "--" — so there is nothing to write and nothing that could have been read.
func flatLiteral(b *strings.Builder, l *Literal) {
	switch l.Kind {
	case LiteralInteger, LiteralDecimal, LiteralDouble, LiteralBoolean:
		b.WriteString(l.Value)
		return
	}

	b.WriteString(quoted(l.Value))

	switch {
	case l.Language != "":
		b.WriteString("@")
		b.WriteString(l.Language)
		if l.Direction != "" {
			b.WriteString("--")
			b.WriteString(l.Direction)
		}
	case l.Datatype != nil:
		b.WriteString("^^")
		flatNode(b, l.Datatype)
	}
}

// quoted renders s as a STRING_LITERAL_QUOTE, quotes included.
//
//	STRING_LITERAL_QUOTE ::= '"' ([^#x22#x5C#xA#xD] | ECHAR | UCHAR)* '"'
//
// The rendering is the rdf package's, which escapes exactly the four characters
// the production excludes — '"', '\', line feed and carriage return — so what is
// written re-reads as the value it was made from. The production is unchanged
// between RDF 1.1 and RDF 1.2 Turtle, and unlike N-Triples §3 Turtle defines no
// canonical form to prefer a longer escape over a shorter one.
func quoted(s string) string {
	return rdf.NewLiteral(s).String()
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
			if i+2 < len(s) && lex.IsHex(rune(s[i+1])) && lex.IsHex(rune(s[i+2])) {
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

		case lex.IsPNLocalEsc(r):
			b.WriteByte('\\')
			b.WriteRune(r)

		case r == ':' || lex.IsPNChars(r):
			if first && !(lex.IsPNCharsU(r) || r == ':' || lex.IsDigit(r)) {
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

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// Encode writes triples to w as RDF 1.2 Turtle.
//
// This is the other direction of [Decode]: it takes what a graph means and
// chooses a way of writing it. Statements about one subject are gathered into
// one, its verbs separated by ';' and the objects of a verb by ',', rdf:type
// is written "a", and an IRI is abbreviated against the prefixes given by
// [WithPrefixes] — the three things that make a Turtle document readable
// rather than a list of triples in longer clothes.
//
// Subjects are written in the order they first arrive and the verbs of a
// subject in the order they first arrive under it, so encoding the same
// sequence twice writes the same bytes. Encoding two sequences that differ
// only in their order does not: the order of triples in a graph means nothing,
// and preserving the caller's is more use than inventing one of this package's.
//
// A number, a boolean and a plain string are written in their short forms
// wherever the lexical form allows — 42 rather than "42"^^xsd:integer — and in
// full wherever it does not, so that reading the output back yields the
// literal that was written. A blank node keeps the label it carries when
// Turtle can spell it and is given one it can otherwise; either way two
// mentions of one node stay one node.
//
// Everything the RDF 1.2 data model can hold has a Turtle spelling here, so
// unlike the RDF 1.1 encoder this one refuses no term for want of syntax: a
// literal carrying a base direction is written "v"@en--ltr, and a triple term
// object is written "<<( s p o )>>", nested as deeply as the term is.
//
// # Which form a reification is written in
//
// RDF 1.2 gives one set of triples more than one spelling, and Encode always
// picks the plainest. A triple term is written "<<( s p o )>>" and a triple
// whose predicate is rdf:reifies is written as the ordinary triple it is:
//
//	:r rdf:reifies <<( :s :p :o )>> .
//
// The two sugars are never produced. The same graph could be written
// "<< :s :p :o ~ :r >> ." instead; and if ":s :p :o" is itself asserted, as
//
//	:s :p :o ~ :r .
//
// and if everything else said about ":r" may be moved inside the annotation, as
//
//	:s :p :o ~ :r {| :q :v |} .
//
// Choosing between those is not a local decision. It turns on whether the
// reified triple is asserted elsewhere in the graph, whether the reifier is the
// subject of anything else, and whether the triple term is referred to more than
// once — three questions about the whole graph, asked of a function that groups
// triples by subject and otherwise writes them as they arrive. Getting any of
// them wrong writes a different graph, since the sugar states triples of its
// own: "<< :s :p :o ~ :r >>" and ":s :p :o ~ :r" differ in exactly the triple
// ":s :p :o".
//
// So the sugar is [Print]'s to write and not this function's. A caller who wants
// it builds a [Document] — by [Parse], or by hand — and prints that; a caller
// who wants a faithful graph and does not mind how it reads uses Encode.
//
// # Streaming
//
// Unlike [Decode] this does not stream. Grouping statements by subject means
// knowing every statement about that subject, and a subject may be mentioned
// again at the end of the sequence, so the whole of it is held. A caller
// encoding more than fits in memory wants N-Triples, which needs no grouping.
//
// It reports [ErrInvalidPrefix] for a prefix binding that cannot be written,
// the error from [rdf.Triple.Validate] for a triple that is not one — which is
// what reports a triple term standing as a subject, however deeply nested —
// and otherwise the first error from w.
//
// Nothing is written until every triple has been read, so both of those leave w
// untouched: a sequence this package refuses is refused before any of it has
// been written. An error from w itself is a different matter, and leaves behind
// whatever reached w before it — a caller that must not act on a partial
// document should encode into a buffer of its own and write that on once Encode
// returns nil.
func Encode(w io.Writer, triples iter.Seq[rdf.Triple], opts ...Option) error {
	cfg := newConfig(opts)

	e, err := newEncoder(cfg)
	if err != nil {
		return err
	}

	doc, err := e.document(triples)
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

// document builds the tree for the triples, gathering the statements about one
// subject into one.
func (e *encoder) document(triples iter.Seq[rdf.Triple]) (*Document, error) {
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
			group = &subjectGroup{
				triples: &Triples{Subject: e.term(triple.Subject)},
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

		po := group.triples.Predicates[at]
		po.Objects = append(po.Objects, &Object{Term: e.term(triple.Object)})
	}

	doc := &Document{Pos: Pos{Line: 1, Column: 1}}
	for _, prefix := range e.declared() {
		doc.Statements = append(doc.Statements, &PrefixDirective{
			Keyword: "@prefix",
			Prefix:  prefix,
			IRI:     &IRIRef{Value: e.cfg.prefixes[prefix]},
		})
	}
	for _, key := range order {
		doc.Statements = append(doc.Statements, groups[key].triples)
	}
	return doc, nil
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
//
// It cannot fail, which is where this parts from the RDF 1.1 encoder: every
// term RDF 1.2 admits has an RDF 1.2 Turtle spelling. The one term that cannot
// be written is a triple term standing as a subject, and that is not a term the
// data model admits there — [rdf.Triple.Validate] has already refused it by the
// time this is reached.
func (e *encoder) term(t rdf.Term) Term {
	switch n := t.(type) {
	case rdf.IRI:
		return e.iri(n)
	case rdf.BlankNode:
		return e.blankNode(n)
	case rdf.Literal:
		return e.literal(n)
	case rdf.TripleTerm:
		return e.tripleTerm(n)
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

// tripleTerm builds the node for a triple standing as a term.
//
//	tripleTerm ::= '<<(' ttSubject verb ttObject ')>>'
//
// Its object goes back through [encoder.term], so a nested triple term nests as
// deeply as the data model term does. Its verb goes through [encoder.verb], so
// rdf:type is written "a" inside a triple term as it is outside one — the
// production names verb and not predicate.
//
// A reified triple is never built. Which of the RDF 1.2 spellings a reification
// is written in is settled in [Encode]'s documentation; this is the one that
// says only what the data model says.
func (e *encoder) tripleTerm(t rdf.TripleTerm) *TripleTerm {
	return &TripleTerm{
		Subject: e.term(t.Subject),
		Verb:    e.verb(t.Predicate),
		Object:  e.term(t.Object),
	}
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

// blankNode builds the node for a blank node, giving it a label Turtle can
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
//
// A base direction is written after the language tag, which is where RDF 1.2
// puts it and where the RDF 1.1 encoder has to refuse it. The datatype either
// implies — rdf:langString for a tag alone, rdf:dirLangString for a tag and a
// direction — is left unwritten, LANG_DIR saying it already.
func (e *encoder) literal(l rdf.Literal) *Literal {
	if language := l.Language(); language != "" {
		return &Literal{
			Kind:      LiteralString,
			Value:     l.Value(),
			Language:  language,
			Direction: string(l.Direction()),
		}
	}

	datatype := l.Datatype()
	if kind, ok := shorthandKind(datatype, l.Value()); ok {
		return &Literal{Kind: kind, Value: l.Value()}
	}
	if datatype == rdf.XSDString {
		// The datatype every string carries, and the one Turtle leaves unsaid.
		return &Literal{Kind: LiteralString, Value: l.Value()}
	}
	return &Literal{
		Kind:     LiteralString,
		Value:    l.Value(),
		Datatype: e.iri(datatype),
	}
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
			if !lex.IsPNCharsBase(r) {
				return false
			}
		case r == '.':
		default:
			if !lex.IsPNChars(r) {
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
			if !lex.IsPNCharsU(r) && !lex.IsDigit(r) {
				return false
			}
		case r == '.':
		default:
			if !lex.IsPNChars(r) {
				return false
			}
		}
		last = r
	}
	return last != '.'
}
