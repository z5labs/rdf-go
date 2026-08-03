package nquads

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
// No rule reaches this today, because a blank node label backs up over the
// carriage return before any line feed is read and a language tag only peeks
// after alphanumerics; the invariant is pinned here so that the next rule to
// want lookahead inherits a backup that works rather than one that happens to
// be called in the only places it is safe.
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
