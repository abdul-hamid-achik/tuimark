package layout

import (
	"strings"
	"unicode/utf8"
)

// The geometry of a <textarea> (SPEC v0.4d §6.19.4, decisions 230-233):
// one function of its value and its content width, read by the engine
// (measure and place), the painter, the dump's caret, and the host's key
// handling, so that all of them agree on where a visual line ends and on
// which line the caret stands.

// TextLine is one visual line of a textarea's value: the byte range
// [Start, End) of the value it shows, and whether a line break (LF) follows
// it, so the next line starts at End+1 instead of End.
type TextLine struct {
	Start, End int
	LF         bool
}

// TextareaWrapWidth is the width a textarea's value wraps at for a content
// width w: max(1, w - 1), the spare column being the cell of a caret after
// the last character of a full line (SPEC v0.4d §6.19.4, decision 230).
func TextareaWrapWidth(w int) int { return max(1, w-1) }

// TextareaLines returns the visual lines of text at content width w: one
// PreWrap of the whole value at TextareaWrapWidth(w) columns, so an empty
// hard line is one empty visual line ("a\n" has two), and an empty value,
// for which PreWrap gives none, has one empty visual line (SPEC v0.4d
// §6.19.4, decision 230).
func TextareaLines(text string, w int) []TextLine {
	if text == "" {
		return []TextLine{{}}
	}
	var out []TextLine
	preWrapRanges(text, TextareaWrapWidth(w), func(start, end int) {
		out = append(out, TextLine{Start: start, End: end, LF: end < len(text) && text[end] == '\n'})
	})
	return out
}

// TextareaLineCount is len(TextareaLines(text, w)) without building the
// lines: the number of visual lines, at least 1.
func TextareaLineCount(text string, w int) int {
	return max(1, preWrapLineCount(text, TextareaWrapWidth(w)))
}

// textareaWidth is the intrinsic content width of a textarea: an input's in
// the current language, max(width(placeholder) + 1, 20), whatever its value
// (SPEC v0.4d §6.19.4, decision 231).
func textareaWidth(b *Box) int { return max(Width(b.Placeholder)+1, 20) }

// LocateCaret returns the visual line the caret at byte offset off of text
// stands on, and its column in that line, in columns from the line's start
// (not clamped to the content box). A caret where a visual line ends
// without a line break, other than the end of the value, stands at the
// start of the next one; at the end of the value it stays on the last
// visual line, after its last cluster (SPEC v0.4d §6.19.4, decision 230).
func LocateCaret(text string, lines []TextLine, off int) (line, col int) {
	off = min(max(off, 0), len(text))
	line = len(lines) - 1
	for i, ln := range lines {
		if off < ln.Start {
			break
		}
		if off < ln.End || (off == ln.End && (ln.LF || i == len(lines)-1)) {
			line = i
			break
		}
	}
	ln := lines[line]
	if off < ln.Start {
		return line, 0
	}
	return line, Width(text[ln.Start:min(off, ln.End)])
}

// LocateHard returns the hard line the caret at byte offset off stands on,
// and its column in that line, in columns (SPEC v0.4d §6.19.8: the dump's
// caret line and col).
func LocateHard(text string, off int) (line, col int) {
	off = min(max(off, 0), len(text))
	before := text[:off]
	line = strings.Count(before, "\n")
	start := strings.LastIndexByte(before, '\n') + 1
	return line, Width(before[start:])
}

// ByteOffset returns the byte offset of the rune index r in text, clamped
// to the text. The caret of a textarea is a rune index (Box.Cursor, as an
// input's cursor is).
func ByteOffset(text string, r int) int {
	if r <= 0 {
		return 0
	}
	n := 0
	for i := range text {
		if n == r {
			return i
		}
		n++
	}
	return len(text)
}

// RuneIndex returns the rune index of the byte offset off in text.
func RuneIndex(text string, off int) int {
	return utf8.RuneCountInString(text[:min(max(off, 0), len(text))])
}

// TextareaCaret is where the caret of a laid-out textarea stands.
type TextareaCaret struct {
	// Hard line and column in it (SPEC v0.4d §6.19.8).
	Line, Col int
	// Visual line (from the start of the value) and column in it.
	VLine, VCol int
	// X and Y are the cell the terminal's cursor goes on, when Shown: the
	// caret's column in its visual line from the content origin, clamped to
	// the last column of the content box, and its row from the offset on.
	X, Y  int
	Shown bool
}

// CaretOf locates the caret of textarea b as laid out: its value (b.Text),
// its caret (b.Cursor, a rune index), its content box, and the offset it
// was laid out with (b.ScrollY). Shown is false when the caret's visual
// line is outside the content box, where only the wheel can leave it (SPEC
// v0.4d §6.19.5, decision 237).
func CaretOf(b *Box) TextareaCaret {
	off := ByteOffset(b.Text, b.Cursor)
	lines := TextareaLines(b.Text, b.Content.W)
	var c TextareaCaret
	c.VLine, c.VCol = LocateCaret(b.Text, lines, off)
	c.Line, c.Col = LocateHard(b.Text, off)
	row := c.VLine - b.ScrollY
	if b.Content.W > 0 && row >= 0 && row < b.Content.H {
		c.X = b.Content.X + min(c.VCol, b.Content.W-1)
		c.Y = b.Content.Y + row
		c.Shown = true
	}
	return c
}

// textareaPlace sets what layout gives a textarea once it is placed (SPEC
// v0.4d §6.19.4): its scroll extent, the larger of its content box's rows
// and its visual lines (decision 232), and its offset. The offset comes in
// as ScrollY; it follows the caret by the least change that puts the
// caret's visual line inside the content box when Follow is not negative
// (an edit, a caret move, or no offset yet: decision 233), and is clamped
// to the extent either way.
func (e *Engine) textareaPlace(b *Box) {
	c := b.Content
	lines := TextareaLines(b.Text, c.W)
	b.ContentW = c.W
	b.ContentH = max(c.H, len(lines))
	off := b.ScrollY
	if b.Follow >= 0 && c.H > 0 {
		v, _ := LocateCaret(b.Text, lines, ByteOffset(b.Text, b.Cursor))
		if v < off {
			off = v
		}
		if v >= off+c.H {
			off = v - c.H + 1
		}
	}
	b.ScrollX = 0
	b.ScrollY = min(max(off, 0), b.ContentH-c.H)
}
