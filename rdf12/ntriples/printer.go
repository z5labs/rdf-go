package ntriples

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"iter"
	"strings"
	"unicode/utf8"

	rdf "github.com/z5labs/rdf-go"
)

// ErrNilDocument is reported when [Print] is given no document to print.
var ErrNilDocument = errors.New("ntriples: cannot print a nil document")

// Print writes doc to w as RDF 1.2 N-Triples.
//
// The document is reproduced rather than normalized: its statements keep the
// order they were written in, its version directives keep their place among
// them — which is what a directive means, it announcing the version of what
// follows it — and its comments are written back where they were, a comment
// that trailed a statement trailing it still. That is what makes [Parse]
// followed by Print a way to rewrite a document rather than to replace it.
//
// The writing itself is canonical (§3): one space between the parts of a
// statement and after the "<<(" of a triple term, one statement to a line, and
// the escaping of §3 within a quoted string.
//
// Two things §3 settles are nonetheless left as the document wrote them, both
// because Print reproduces a document and [Encode] is what canonicalizes one:
//
//   - a VERSION directive, which canonical form omits and which a document
//     rewritten by Print has every reason to keep.
//   - the case of a language tag, which canonical form maps to lowercase. RDF
//     1.2 has parsers preserve the tag as written and compare tags case
//     insensitively, so the case is the document's and not this printer's to
//     change.
//
// A document that carries neither — no comments, no version directive and no
// uppercase language tag — prints in canonical form.
//
// Layout follows the positions on the nodes, which [Parse] fills in. A comment
// is written trailing a statement when it was written on that statement's line
// after it, and on a line of its own otherwise.
//
// It reports [ErrNilDocument] if doc is nil, and otherwise the first error
// from w.
func Print(w io.Writer, doc *Document) error {
	buf := bufio.NewWriter(w)

	pr := &printer{w: buf}
	for action := printDocument; action != nil && pr.err == nil; {
		action = action(pr, doc)
	}
	if pr.err != nil {
		return pr.err
	}
	return buf.Flush()
}

// printer writes to w, remembering the first error rather than returning one
// from every step. Actions can then be written as though writing cannot fail,
// and the loop in [Print] stops at the first that did.
type printer struct {
	w   io.Writer
	err error
}

// write writes s, doing nothing once an earlier write has failed.
func (pr *printer) write(s string) {
	if pr.err != nil {
		return
	}
	_, pr.err = io.WriteString(pr.w, s)
}

// printerAction is one step of printing: it writes some of the document and
// returns the step to run next, or nil when there is nothing left to write.
type printerAction func(pr *printer, doc *Document) printerAction

func printDocument(pr *printer, doc *Document) printerAction {
	if doc == nil {
		pr.err = ErrNilDocument
		return nil
	}
	return printNext(0, 0)
}

// printNext writes whichever of the next statement and the next comment comes
// first, and returns the step that writes the one after.
//
// The two are held in separate lists, each already in the order it was
// written, so putting the document back together is a merge on position.
func printNext(statement, comment int) printerAction {
	return func(pr *printer, doc *Document) printerAction {
		var (
			s Statement
			c *Comment
		)
		if statement < len(doc.Statements) {
			s = doc.Statements[statement]
		}
		if comment < len(doc.Comments) {
			c = doc.Comments[comment]
		}

		switch {
		case s == nil && c == nil:
			return nil

		case s == nil || (c != nil && earlier(c.Pos, s.Position())):
			pr.write(c.Text)
			pr.write("\n")
			return printNext(statement, comment+1)

		default:
			printStatement(pr, s)

			// A comment written after a statement on the same line trailed it,
			// and goes back there. Anything else stands on its own line, which
			// is also what an unpositioned comment gets.
			if pos := s.Position(); c != nil && c.Pos.Line == pos.Line && c.Pos.Column > pos.Column {
				pr.write(" ")
				pr.write(c.Text)
				comment++
			}
			pr.write("\n")
			return printNext(statement+1, comment)
		}
	}
}

// earlier reports whether a comes before b in the document.
func earlier(a, b Pos) bool {
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	return a.Column < b.Column
}

// printStatement writes one statement, without its line ending.
//
//	statement ::= directive | triple
func printStatement(pr *printer, s Statement) {
	switch n := s.(type) {
	case *Triple:
		printTriple(pr, n)
	case *VersionDirective:
		printVersionDirective(pr, n)
	default:
		panic(fmt.Sprintf("unknown statement node: %T", s))
	}
}

// printTriple writes one triple statement.
//
//	triple ::= subject predicate object '.'
//
// The single space between the parts is what §3 requires of canonical
// N-Triples, and there is no reason to write anything else.
func printTriple(pr *printer, t *Triple) {
	pr.write(term(t.Subject))
	pr.write(" ")
	pr.write(term(t.Predicate))
	pr.write(" ")
	pr.write(term(t.Object))
	pr.write(" .")
}

// printVersionDirective writes one version announcement.
//
//	versionDirective ::= 'VERSION' versionSpecifier
//	versionSpecifier ::= STRING_LITERAL_QUOTE
//
// No '.' ends it, the grammar giving a directive none, and the specifier is a
// quoted string like any other and escaped like one.
func printVersionDirective(pr *printer, v *VersionDirective) {
	var b strings.Builder
	b.WriteString("VERSION ")
	writeQuoted(&b, v.Version)
	pr.write(b.String())
}

// term renders a syntax tree term in canonical N-Triples form.
func term(node Term) string {
	var b strings.Builder
	writeNode(&b, node)
	return b.String()
}

// writeNode writes a syntax tree term to b.
//
// An IRI and a blank node are rendered by the rdf package, which is where the
// canonical form of those two is defined and where an IRI's escaping lives —
// unchanged between RDF 1.1 and RDF 1.2, so there is no reason for a second
// copy of it here. A literal is not: §3 changed which characters a quoted
// string escapes, so the rendering of one belongs to the package that
// implements the version.
func writeNode(b *strings.Builder, node Term) {
	switch n := node.(type) {
	case *IRIRef:
		b.WriteString(rdf.IRI(n.Value).String())
	case *BlankNode:
		b.WriteString(rdf.NewBlankNode(n.Label).String())
	case *Literal:
		writeNodeLiteral(b, n)
	case *TripleTerm:
		writeNodeTripleTerm(b, n)
	default:
		panic(fmt.Sprintf("unknown term node: %T", node))
	}
}

// writeNodeLiteral writes a syntax tree literal.
//
//	literal ::= STRING_LITERAL_QUOTE (('^^' IRIREF) | LANG_DIR)?
//
// The tag and the datatype are appended here rather than the literal being
// built as an [rdf.Literal] first: a document may say "v"^^rdf:langString,
// which the data model refuses and this printer still has to be able to write
// back.
//
// A base direction is written only alongside a language tag. LANG_DIR has no
// spelling for one without a tag, and the grammar cannot produce such a node —
// the production demands a letter before the "--" — so there is nothing to
// write and nothing that could have been read.
func writeNodeLiteral(b *strings.Builder, l *Literal) {
	writeQuoted(b, l.Value)

	switch {
	case l.Language != "":
		b.WriteByte('@')
		b.WriteString(l.Language)
		if l.Direction != "" {
			b.WriteString("--")
			b.WriteString(l.Direction)
		}
	case l.Datatype != nil:
		b.WriteString("^^")
		b.WriteString(rdf.IRI(l.Datatype.Value).String())
	}
}

// writeNodeTripleTerm writes a syntax tree triple term.
//
//	tripleTerm ::= '<<(' subject predicate object ')>>'
//
// Its object is written by [writeNode], so a nested triple term comes back
// here and the brackets nest as deeply as the document wrote them.
//
// The space after the "<<(" is §3's: the terminal is one of the four things
// canonical form allows white space after, and the ")>>" is not, so the
// closing bracket is preceded by the space that follows the object and by
// nothing else.
func writeNodeTripleTerm(b *strings.Builder, t *TripleTerm) {
	b.WriteString("<<( ")
	writeNode(b, t.Subject)
	b.WriteByte(' ')
	writeNode(b, t.Predicate)
	b.WriteByte(' ')
	writeNode(b, t.Object)
	b.WriteString(" )>>")
}

// Encode writes triples to w as canonical RDF 1.2 N-Triples (§3).
//
// Canonical form is what makes output comparable: the same graph written by
// two implementations, or by this one twice, is the same bytes. A statement
// occupies one line and its parts are separated by one space, a quoted string
// escapes exactly the characters §3 says it must, a language tag is written in
// lowercase, and every statement ends with a line feed, the last one included.
//
// There is no VERSION directive — §3 forbids one — and there are no comments,
// canonical N-Triples having none. A caller wanting to keep either has a
// [Document] and wants [Print].
//
// A triple term object is written as "<<( s p o )>>", nested as deeply as the
// term is. Everything the RDF 1.2 data model can hold has an N-Triples
// spelling here, so unlike the RDF 1.1 encoder this one refuses no term for
// want of syntax.
//
// # What canonical form does and does not fix
//
// §3 settles the writing — the spacing, the escaping, the case of a language
// tag, the absence of comments and of a version directive — so a ground graph
// encoded twice, here or anywhere, is the same bytes.
//
// It says nothing about which labels the blank nodes carry, and neither does
// this function: a blank node is written with the label it has. Labels minted
// by [Decode] are unique but not reproducible, being drawn from a counter that
// runs for the life of the process, so a graph with blank nodes read and
// written again is the same graph and need not be the same bytes. Comparing
// two such graphs is what [rdf.Isomorphic] is for. Producing labels that are
// themselves canonical is a separate problem, and a much larger one.
//
// Encoding stops at the first error, which is either the one from w or the one
// from [rdf.Triple.Validate] if a triple could not be written as N-Triples at
// all. A literal subject cannot, and neither can a triple term subject — RDF
// 1.2 admits a triple term in the object position alone — so both are reported
// as [rdf.ErrInvalidSubject] rather than written in a spelling no parser would
// read back. The check recurses, so a triple term standing as the subject of
// another triple term is reported however deep it is.
//
// Stopping early does not discard what came before it: the statements written
// before the offending one reach w, and only the offending one and those after
// it do not. A caller that must not act on a partial document should encode
// into a buffer of its own and write that on once Encode returns nil.
func Encode(w io.Writer, triples iter.Seq[rdf.Triple]) error {
	buf := bufio.NewWriter(w)

	var (
		err error
		b   strings.Builder
	)
	for triple := range triples {
		if err = triple.Validate(); err != nil {
			break
		}

		b.Reset()
		writeTerm(&b, triple.Subject)
		b.WriteByte(' ')
		b.WriteString(triple.Predicate.String())
		b.WriteByte(' ')
		writeTerm(&b, triple.Object)
		b.WriteString(" .\n")

		if _, err = io.WriteString(buf, b.String()); err != nil {
			break
		}
	}

	// Flush even when stopping early, so that the statements written before
	// the offending one reach w. Without this what a caller receives would
	// depend on where the buffer happened to fill: everything up to the last
	// flush for a long document, and nothing at all for a short one.
	if flushErr := buf.Flush(); err == nil {
		err = flushErr
	}
	return err
}

// writeTerm writes a data model term to b in canonical form.
//
// It is reached only for a term of a triple that [rdf.Triple.Validate]
// accepted, so the four cases are exhaustive and none of them is nil.
func writeTerm(b *strings.Builder, t rdf.Term) {
	switch v := t.(type) {
	case rdf.IRI:
		b.WriteString(v.String())
	case rdf.BlankNode:
		b.WriteString(v.String())
	case rdf.Literal:
		writeLiteral(b, v)
	case rdf.TripleTerm:
		writeTripleTerm(b, v)
	default:
		panic(fmt.Sprintf("unknown term: %T", t))
	}
}

// writeLiteral writes a data model literal in canonical form.
//
// The datatype is written only when the rest of the syntax does not already
// say it: §3 has xsd:string omitted, and rdf:langString and rdf:dirLangString
// are implied by the tag and the direction that a literal carrying either must
// have.
//
// The tag is lowercased, which is §3's rule and not the data model's: RDF 1.2
// preserves the case a document wrote and folds it only when comparing, so the
// mapping happens here, on the way out, and nowhere else.
func writeLiteral(b *strings.Builder, l rdf.Literal) {
	writeQuoted(b, l.Value())

	switch {
	case l.Language() != "":
		b.WriteByte('@')
		b.WriteString(strings.ToLower(l.Language()))
		if l.Direction() != rdf.DirectionNone {
			b.WriteString("--")
			b.WriteString(string(l.Direction()))
		}
	case l.Datatype() != rdf.XSDString:
		b.WriteString("^^")
		b.WriteString(l.Datatype().String())
	}
}

// writeTripleTerm writes a data model triple term in canonical form.
//
//	tripleTerm ::= '<<(' subject predicate object ')>>'
//
// Its object goes through [writeTerm] and so through here again, which is what
// carries the nesting of the production into the output.
func writeTripleTerm(b *strings.Builder, t rdf.TripleTerm) {
	b.WriteString("<<( ")
	writeTerm(b, t.Subject)
	b.WriteByte(' ')
	b.WriteString(t.Predicate.String())
	b.WriteByte(' ')
	writeTerm(b, t.Object)
	b.WriteString(" )>>")
}

// hexDigits are the digits a canonical UCHAR is written with. §3 requires
// uppercase letters, so a lowercase table would be wrong rather than merely
// unusual.
const hexDigits = "0123456789ABCDEF"

// writeQuoted writes s to b as a canonical STRING_LITERAL_QUOTE, quotes
// included.
//
//	STRING_LITERAL_QUOTE ::= '"' ([^#x22#x5C#xA#xD] | ECHAR | UCHAR)* '"'
//
// §3 leaves a character no choice of spelling. Seven have an ECHAR and must
// use it — BS, HT, LF, FF, CR, '"' and '\' — which is where RDF 1.2 parts from
// RDF 1.1, whose canonical form escaped only the last four of those and wrote
// a tab as itself. The characters with no printable form take a UCHAR, and
// everything else is written as the character it is.
//
// This is why the rendering is not the rdf package's: [rdf.Literal.String]
// writes the RDF 1.1 canonical form that the rdf11 packages need, and the two
// versions no longer agree.
func writeQuoted(b *strings.Builder, s string) {
	b.WriteByte('"')
	escapeString(b, s)
	b.WriteByte('"')
}

// escapeString writes the body of a canonical STRING_LITERAL_QUOTE.
func escapeString(b *strings.Builder, s string) {
	for i := 0; i < len(s); {
		// Every character with an ECHAR, and every one below U+0080 that needs
		// a UCHAR, is a single byte, so the common path never decodes.
		if c := s[i]; c < utf8.RuneSelf {
			escapeASCII(b, c)
			i++
			continue
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			// Not UTF-8 at all, and so not a character the grammar can spell.
			// Writing U+FFFD in its place would change the literal, so the
			// byte goes out as it stands and the document it produces is the
			// caller's to answer for.
			b.WriteByte(s[i])
		case r == 0xFFFE, r == 0xFFFF:
			// The only characters above U+007F that XML 1.1's Char production
			// excludes and that can still reach here: a surrogate has no UTF-8
			// encoding, and every other code point above U+FFFF is a Char.
			writeUCHAR(b, r)
		default:
			b.WriteString(s[i : i+size])
		}
		i += size
	}
}

// escapeASCII writes one character below U+0080.
//
// The three ranges taking a UCHAR are what is left of the C0 controls and DEL
// once the five with an ECHAR are set aside: U+0008 to U+000D are BS, HT, LF,
// VT, FF and CR, and VT is the one of the six that has no escape of its own.
// U+0000 is in the first range already, which is also the only character below
// U+0080 that XML 1.1's Char production excludes.
func escapeASCII(b *strings.Builder, c byte) {
	switch c {
	case '\b':
		b.WriteString(`\b`)
	case '\t':
		b.WriteString(`\t`)
	case '\n':
		b.WriteString(`\n`)
	case '\f':
		b.WriteString(`\f`)
	case '\r':
		b.WriteString(`\r`)
	case '"':
		b.WriteString(`\"`)
	case '\\':
		b.WriteString(`\\`)
	default:
		if c <= 0x07 || c == 0x0B || (c >= 0x0E && c <= 0x1F) || c == 0x7F {
			writeUCHAR(b, rune(c))
			return
		}
		b.WriteByte(c)
	}
}

// writeUCHAR writes r as the four digit UCHAR §3 calls for: a lowercase '\u'
// and uppercase hexadecimal digits.
//
// Four digits are always enough. Every character this is reached for is below
// U+0080 or is U+FFFE or U+FFFF, so none of them needs the eight digit form.
func writeUCHAR(b *strings.Builder, r rune) {
	b.WriteString(`\u`)
	for shift := 12; shift >= 0; shift -= 4 {
		b.WriteByte(hexDigits[(r>>shift)&0xF])
	}
}
