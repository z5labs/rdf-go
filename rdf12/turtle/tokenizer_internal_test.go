package turtle

import (
	"bufio"
	"strings"
	"testing"
)

func newTestTokenizer(src string) *tokenizer {
	return &tokenizer{
		pos: Pos{Line: 1, Column: 1},
		buf: bufio.NewReader(strings.NewReader(src)),
	}
}

// TestBackupUndoesTheWholeRead pins what backup owes its caller: after it, the
// tokenizer must be in exactly the state it was in before the read, not merely
// at the same position.
//
// The carriage return flag is the part that is easy to leave out and hard to
// notice. Reading the line feed of a CRLF pair clears it, so a backup that
// restored only the position would leave the tokenizer thinking the next line
// feed starts a line of its own — and the same line would be counted twice.
// The lookahead that tells "<<" from "<<(" backs up over whatever followed the
// second '<', a line ending included, so this is the invariant that keeps the
// line count right afterwards.
func TestBackupUndoesTheWholeRead(t *testing.T) {
	tk := newTestTokenizer("\r\na")

	r, err := tk.next()
	if err != nil {
		t.Fatalf("next() = %v, want nil", err)
	}
	if r != '\r' {
		t.Fatalf("next() = %q, want %q", r, '\r')
	}

	// Read the line feed of the pair and put it back.
	before := tk.mark()
	if r, err = tk.next(); err != nil {
		t.Fatalf("next() = %v, want nil", err)
	}
	if r != '\n' {
		t.Fatalf("next() = %q, want %q", r, '\n')
	}
	if err := tk.backup(before); err != nil {
		t.Fatalf("backup() = %v, want nil", err)
	}

	// Reading it again must complete the same line ending, not begin another.
	if r, err = tk.next(); err != nil {
		t.Fatalf("next() = %v, want nil", err)
	}
	if r != '\n' {
		t.Fatalf("next() = %q, want %q", r, '\n')
	}
	if r, err = tk.next(); err != nil {
		t.Fatalf("next() = %v, want nil", err)
	}
	if r != 'a' {
		t.Fatalf("next() = %q, want %q", r, 'a')
	}

	if got, want := tk.pos, (Pos{Line: 2, Column: 2}); got != want {
		t.Errorf("pos = %s, want %s: the CRLF pair was counted as two line endings", got, want)
	}
}

// TestPeekLeavesThePositionAlone covers the primitive every ambiguous prefix
// here is settled with. A peek that moved the position would put every token
// after a "<<", a ")" or a "{|" one character out.
func TestPeekLeavesThePositionAlone(t *testing.T) {
	tk := newTestTokenizer("<(")

	if _, err := tk.next(); err != nil {
		t.Fatalf("next() = %v, want nil", err)
	}
	before := tk.pos

	r, ok, err := tk.peek()
	if err != nil {
		t.Fatalf("peek() = %v, want nil", err)
	}
	if !ok || r != '(' {
		t.Fatalf("peek() = %q, %t, want %q, true", r, ok, '(')
	}
	if tk.pos != before {
		t.Errorf("pos = %s, want %s", tk.pos, before)
	}

	// The peeked character is still there to be read.
	if r, err = tk.next(); err != nil {
		t.Fatalf("next() = %v, want nil", err)
	}
	if r != '(' {
		t.Errorf("next() = %q, want %q", r, '(')
	}
}

// TestExpectDistinguishesRunningOutFromGoingWrong covers the two errors a
// half-written delimiter can produce, which the rules for ")>>", ">>" and "|}"
// all report through expect.
func TestExpectDistinguishesRunningOutFromGoingWrong(t *testing.T) {
	t.Run("a character that is not the expected one", func(t *testing.T) {
		tk := newTestTokenizer("x")

		err := tk.expect(">")
		want := UnexpectedCharacterError{Pos: Pos{Line: 1, Column: 1}, R: 'x'}
		if err != want {
			t.Errorf("expect() = %v, want %v", err, want)
		}
	})

	t.Run("no character at all", func(t *testing.T) {
		tk := newTestTokenizer("")

		err := tk.expect(">")
		want := UnexpectedEndOfInputError{Pos: Pos{Line: 1, Column: 1}}
		if err != want {
			t.Errorf("expect() = %v, want %v", err, want)
		}
	})
}

// TestAdvanceCountsLineEndings covers the three ways a document ends its
// lines, each of which has to advance the line exactly once.
func TestAdvanceCountsLineEndings(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want Pos
	}{
		{
			name: "line feeds alone",
			src:  "a\nb\nc",
			want: Pos{Line: 3, Column: 2},
		},
		{
			name: "carriage returns and line feeds",
			src:  "a\r\nb\r\nc",
			want: Pos{Line: 3, Column: 2},
		},
		{
			name: "carriage returns alone",
			src:  "a\rb\rc",
			want: Pos{Line: 3, Column: 2},
		},
		{
			name: "a line feed before a carriage return is not a pair",
			src:  "a\n\rb",
			want: Pos{Line: 3, Column: 2},
		},
		{
			name: "a blank line between two others",
			src:  "a\n\nb",
			want: Pos{Line: 3, Column: 2},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tk := newTestTokenizer(test.src)

			for {
				if _, err := tk.next(); err != nil {
					break
				}
			}

			if tk.pos != test.want {
				t.Errorf("pos = %s, want %s", tk.pos, test.want)
			}
		})
	}
}
