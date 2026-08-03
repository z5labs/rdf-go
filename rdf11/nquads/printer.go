package nquads

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"iter"

	rdf "github.com/z5labs/rdf-go"
)

// ErrNilDocument is reported when [Print] is given no document to print.
var ErrNilDocument = errors.New("nquads: cannot print a nil document")

// Print writes doc to w as N-Quads.
//
// The document is reproduced rather than normalized: its statements keep the
// order they were written in, and its comments are written back where they
// were, a comment that trailed a statement trailing it still. That is what
// makes [Parse] followed by Print a way to rewrite a document rather than to
// replace it.
//
// Everything else is canonical — one space between the parts of a statement,
// one statement to a line, and the escaping canonical form calls for — so a
// document that carries no comments prints in canonical form. A document that
// does carry them cannot be canonical, canonical form having none.
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
			t *Quad
			c *Comment
		)
		if statement < len(doc.Quads) {
			t = doc.Quads[statement]
		}
		if comment < len(doc.Comments) {
			c = doc.Comments[comment]
		}

		switch {
		case t == nil && c == nil:
			return nil

		case t == nil || (c != nil && earlier(c.Pos, t.Pos)):
			pr.write(c.Text)
			pr.write("\n")
			return printNext(statement, comment+1)

		default:
			printStatement(pr, t)

			// A comment written after a statement on the same line trailed it,
			// and goes back there. Anything else stands on its own line, which
			// is also what an unpositioned comment gets.
			if c != nil && c.Pos.Line == t.Pos.Line && c.Pos.Column > t.Pos.Column {
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
//	statement ::= subject predicate object graphLabel? '.'
//
// The single space between the parts is what canonical form requires, and
// there is no reason to write anything else. A statement with no label is
// written without one, which is how the default graph is spelled.
func printStatement(pr *printer, t *Quad) {
	pr.write(term(t.Subject))
	pr.write(" ")
	pr.write(term(t.Predicate))
	pr.write(" ")
	pr.write(term(t.Object))
	if t.Graph != nil {
		pr.write(" ")
		pr.write(term(t.Graph))
	}
	pr.write(" .")
}

// term renders a syntax tree term in canonical N-Quads form.
//
// The rendering is the rdf package's, which is where the canonical form of a
// term is defined and escaped. Writing it a second time here would be a second
// place for the escaping rules to be got wrong.
func term(node Term) string {
	switch n := node.(type) {
	case *IRIRef:
		return rdf.IRI(n.Value).String()
	case *BlankNode:
		return rdf.NewBlankNode(n.Label).String()
	case *Literal:
		return literal(n)
	default:
		panic(fmt.Sprintf("unknown term node: %T", node))
	}
}

// literal renders a syntax tree literal.
//
// The quoted form comes from the rdf package, but the tag and the datatype are
// appended here rather than built into an [rdf.Literal] first: a document may
// say "v"^^rdf:langString, which the data model refuses and this printer still
// has to be able to write back.
func literal(l *Literal) string {
	quoted := rdf.NewLiteral(l.Value).String()

	switch {
	case l.Language != "":
		return quoted + "@" + l.Language
	case l.Datatype != nil:
		return quoted + "^^" + rdf.IRI(l.Datatype.Value).String()
	default:
		return quoted
	}
}

// Encode writes quads to w as canonical N-Quads.
//
// # Where the canonical form comes from
//
// The N-Quads specification defines no canonical form of its own. N-Triples
// §4 does, and N-Quads differs from N-Triples by one optional graph label, so
// the rules carried over here are §4's with the label written after the object
// and before the '.', separated by the same single space as everything else.
// That is the form RDF Dataset Canonicalization means when it speaks of
// canonical N-Quads.
//
// Canonical form is what makes output comparable: the same graph written by
// two implementations, or by this one twice, is the same bytes. A statement
// occupies one line, its parts separated by one space, and only the four
// characters that must be escaped inside a literal are — a tab is written as
// itself, not as \t, both being legal and only one canonical. Every statement
// ends with a line feed, the last one included.
//
// There are no comments, canonical N-Quads having none. A caller wanting to
// keep the comments of a document has a [Document] and wants [Print].
//
// # What canonical form does and does not fix
//
// Canonical form settles the writing — the spacing, the escaping, the absence
// of comments — so a ground dataset encoded twice, here or anywhere, is the
// same bytes.
//
// It says nothing about which labels the blank nodes carry, and neither does
// this function: a blank node is written with the label it has, whether it
// stands as a term or as a graph label. Labels minted by [Decode] are unique
// but not reproducible, being drawn from a counter that runs for the life of
// the process, so a dataset with blank nodes read and written again is the
// same dataset and need not be the same bytes. Producing labels that are
// themselves canonical is a separate problem, and a much larger one.
//
// Encoding stops at the first error, which is the one from w, or the one from
// [rdf.Quad.Validate] if a quad could not be written as N-Quads at all — a
// literal subject or graph label cannot, and writing one anyway would produce
// a document this package could not read back.
//
// Stopping early does not discard what came before it: the statements written
// before the offending one reach w, and only the offending one and those after
// it do not. A caller that must not act on a partial document should encode
// into a buffer of its own and write that on once Encode returns nil.
func Encode(w io.Writer, quads iter.Seq[rdf.Quad]) error {
	buf := bufio.NewWriter(w)

	var err error
	for quad := range quads {
		if err = quad.Validate(); err != nil {
			break
		}
		if _, err = io.WriteString(buf, quad.String()); err != nil {
			break
		}
		if err = buf.WriteByte('\n'); err != nil {
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
