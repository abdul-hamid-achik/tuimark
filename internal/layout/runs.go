package layout

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/uniwidth"
)

// Text runs (SPEC v0.4d §6.20): a <text runs="path"> is a text whose
// content is the joined text of its runs, laid out exactly as a body is.
// The runs only decide the look of each cell, so a Box keeps them beside
// its Text: where each run ends in Text, its classes, and (set by the host
// after the cascade) the look of its generated span.

// TextRuns are the runs of a text (Box.Runs), in order, empty runs left
// out. The joined text of the runs is Box.Text.
type TextRuns struct {
	Spans []RunSpan
}

// RunSpan is one run of a text.
type RunSpan struct {
	// End is the byte offset in Box.Text just after the run's text; the
	// run starts where the one before it ends (0 for the first).
	End int
	// Index is the run's index in the array at the runs path (0 for a
	// string value), which `tuimark inspect` reports.
	Index int
	// Classes are the run's class names, in order, each once.
	Classes []string
	// Style is the look the run's span gives its cells (SPEC v0.4d
	// §6.20.4): the text's own style with the properties a rule set on the
	// span replaced. nil until the host computes it, and then the text's.
	Style *css.Style
}

// At returns the index in Spans of the run that holds byte offset off of
// the joined text, or the last run for an offset past the end (-1 when
// there is no run).
func (r *TextRuns) At(off int) int {
	if r == nil || len(r.Spans) == 0 {
		return -1
	}
	i := sort.Search(len(r.Spans), func(i int) bool { return r.Spans[i].End > off })
	return min(i, len(r.Spans)-1)
}

// Piece is a part of a line of a text with runs: Text, painted from the
// byte offset Off of the joined text, where its first cluster starts. A
// cluster takes the look of the run that holds its first code point
// (SPEC v0.4d §6.20.2). Off is negative for text that comes from no run:
// the ellipsis of a truncated line, which takes the text's own look. The
// joining space of wrap: wrap is a piece of one space whose Off is the
// offset of the first space character it stands for (§6.20.5).
type Piece struct {
	Text string
	Off  int
}

// RunLines is Lines for a text with runs: the same lines Lines(text,
// width, mode) gives, each as the pieces it is made of and where each
// comes from in text (SPEC v0.4d §6.20.5: wrapping, truncating, and
// pre-wrap act on the joined text, line by line). The pieces of a line,
// concatenated, are the line Lines gives. A line with no pieces is an
// empty line.
func RunLines(text string, width int, mode string) [][]Piece {
	if text == "" {
		return nil
	}
	var out [][]Piece
	if mode == ModePreWrap && width > 0 {
		preWrapRanges(text, width, func(start, end int) {
			out = append(out, []Piece{{text[start:end], start}})
		})
		return out
	}
	for off := 0; ; {
		para, more := text[off:], false
		if k := strings.IndexByte(para, '\n'); k >= 0 {
			para, more = para[:k], true
		}
		out = appendParaPieces(out, para, off, width, mode)
		if !more {
			return out
		}
		off += len(para) + 1
	}
}

// appendParaPieces adds the lines of the paragraph para, which starts at
// byte offset off of the text, to out.
func appendParaPieces(out [][]Piece, para string, off, width int, mode string) [][]Piece {
	switch mode {
	case "wrap":
		if width > 0 {
			return append(out, wrapPieces(para, off, width)...)
		}
	case "truncate", "truncate-start", "truncate-middle":
		if width > 0 {
			return append(out, truncatePieces(para, off, width, mode))
		}
	}
	// nowrap, a width of 0, and the modes that need a width: the paragraph
	// as it is.
	if para == "" {
		return append(out, nil)
	}
	return append(out, []Piece{{para, off}})
}

// truncatePieces is the line the truncate mode makes of para (Truncate,
// TruncateStart, TruncateMiddle) at width > 0, with its pieces. The
// ellipsis comes from no run.
func truncatePieces(para string, off, width int, mode string) []Piece {
	if Width(para) <= width {
		if para == "" {
			return nil
		}
		return []Piece{{para, off}}
	}
	if width == 1 {
		return []Piece{{"…", -1}}
	}
	var pieces []Piece
	add := func(s string, at int) {
		if s != "" {
			pieces = append(pieces, Piece{s, at})
		}
	}
	switch mode {
	case "truncate-start":
		tail := CutEnd(para, width-1)
		add("…", -1)
		add(tail, off+len(para)-len(tail))
	case "truncate-middle":
		head, tail := Cut(para, width/2), CutEnd(para, (width-1)/2)
		add(head, off)
		add("…", -1)
		add(tail, off+len(para)-len(tail))
	default:
		add(Cut(para, width-1), off)
		add("…", -1)
	}
	return pieces
}

// wrapPieces is Wrap(para, width) with the pieces of each line: the words
// are substrings of para, joined by pieces of one space that stand for the
// first space character between the two words (SPEC v0.4d §6.20.5). An
// empty paragraph, or one of spaces, is one empty line, as Wrap gives.
func wrapPieces(para string, off, width int) [][]Piece {
	type word struct{ s, e int }
	var words []word
	for i := 0; i < len(para); {
		r, size := utf8.DecodeRuneInString(para[i:])
		if unicode.IsSpace(r) {
			i += size
			continue
		}
		j := i + size
		for j < len(para) {
			r, size = utf8.DecodeRuneInString(para[j:])
			if unicode.IsSpace(r) {
				break
			}
			j += size
		}
		words = append(words, word{i, j})
		i = j
	}
	if len(words) == 0 {
		return [][]Piece{nil}
	}
	var out [][]Piece
	var line []Piece
	lineW, prevEnd := 0, 0
	for _, wd := range words {
		w := para[wd.s:wd.e]
		at := wd.s
		ww := Width(w)
		for ww > width {
			if len(line) > 0 {
				out = append(out, line)
				line, lineW = nil, 0
			}
			b, pw := uniwidth.Prefix(w, width)
			if b == 0 {
				// Not even the first cluster fits: it is a piece by itself.
				c, _, cw, _ := uniwidth.Next(w, -1)
				b, pw = len(c), cw
			}
			out = append(out, []Piece{{w[:b], off + at}})
			w, at, ww = w[b:], at+b, ww-pw
		}
		if w == "" {
			prevEnd = wd.e
			continue
		}
		switch {
		case len(line) == 0:
			line, lineW = []Piece{{w, off + at}}, ww
		case lineW+1+ww <= width:
			line = append(line, Piece{" ", off + prevEnd}, Piece{w, off + at})
			lineW += 1 + ww
		default:
			out = append(out, line)
			line, lineW = []Piece{{w, off + at}}, ww
		}
		prevEnd = wd.e
	}
	if len(line) > 0 {
		out = append(out, line)
	}
	return out
}

// AlignOffset is the column offset content-align gives a line of width w
// in a content box room columns wide: 0 for start, the rest of the room
// halved for center, all of it for end.
func AlignOffset(mode string, room, w int) int {
	switch mode {
	case "center":
		return max(0, (room-w)/2)
	case "end":
		return max(0, room-w)
	}
	return 0
}

// EachRunCluster visits the grapheme clusters of the text with runs b as
// they are painted (SPEC v0.4d §6.20.5, §12.7): the lines of RunLines for
// b's content box and the computed wrap mode, each from the column its
// content-align gives, row by row, and cluster by cluster. fn gets the
// column and row of the cluster, its width, the cluster, and span, the
// index in b.Runs.Spans of the run whose look it has, or -1 for the
// ellipsis, which has the text's own look. Clusters of width 0 are visited
// (they occupy no column). Rows and columns outside view are not visited;
// a line ends at view's right edge, and the walk ends when fn returns
// false. Painting and `tuimark inspect` both walk with it, so they cannot
// disagree about which run a cell belongs to.
func (b *Box) EachRunCluster(mode string, view Rect, fn func(x, y, w, span int, cl string) bool) {
	c := b.Content
	right := view.X + view.W
	for i, line := range RunLines(b.Text, c.W, mode) {
		if i >= c.H {
			return
		}
		y := c.Y + i
		if y < view.Y || y >= view.Y+view.H {
			continue
		}
		x := c.X + AlignOffset(b.Style.ContentAlign, c.W, PiecesWidth(line, mode))
		for _, pc := range line {
			off, ri := pc.Off, -1
			if off >= 0 {
				ri = b.Runs.At(off)
			}
			stop := false
			uniwidth.Each(pc.Text, func(cl string, w int) bool {
				if x >= right {
					return false
				}
				span := -1
				if off >= 0 {
					for ri >= 0 && ri < len(b.Runs.Spans)-1 && off >= b.Runs.Spans[ri].End {
						ri++
					}
					span = ri
					off += len(cl)
				}
				if !fn(x, y, w, span, cl) {
					stop = true
					return false
				}
				x += w
				return true
			})
			if stop {
				return
			}
		}
	}
}

// RunAt returns the index in b.Runs.Spans of the run that paints the cell
// (x, y) of the text with runs b under the wrap mode, or -1 when no run
// does: a cell outside its text, an ellipsis (EachRunCluster).
func (b *Box) RunAt(mode string, x, y int) int {
	found := -1
	b.EachRunCluster(mode, Rect{X: x, Y: y, W: 1, H: 1}, func(cx, cy, w, span int, _ string) bool {
		if cy == y && w > 0 && x >= cx && x < cx+w {
			found = span
			return false
		}
		return true
	})
	return found
}

// PiecesWidth is the width of a line made of pieces, as LineWidth gives it
// for the line's text (SPEC v0.4d §11.5.2: a pre-wrap line is aligned
// without its trailing spaces, and it is one piece).
func PiecesWidth(line []Piece, mode string) int {
	if mode == ModePreWrap && len(line) == 1 {
		return LineWidth(line[0].Text, mode)
	}
	w := 0
	for _, p := range line {
		w += Width(p.Text)
	}
	return w
}
