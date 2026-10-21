package paint

import (
	"unicode/utf8"

	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// runs paints a text with runs (SPEC v0.4d §6.20.5, §12.7): its joined
// text is laid out and cut exactly as a body is (layout.RunLines gives the
// lines of layout.Lines with where each piece comes from), and each
// cluster is painted in the look of the span of the run that holds its
// first code point. The joining space of wrap: wrap has the look of the
// first space it stands for, and the ellipsis of a truncate mode, like
// everything the text does not paint, has the text's own look (st).
func (p *painter) runs(b *layout.Box, clip layout.Rect, mode string, st cellStyle) {
	looks := runLooks(b.Runs, st)
	view := clip.Intersect(layout.Rect{W: p.g.W, H: p.g.H})
	b.EachRunCluster(mode, view, func(x, y, w, span int, cl string) bool {
		if w > 0 {
			look := st
			if span >= 0 && span < len(looks) {
				look = looks[span]
			}
			r, n := utf8.DecodeRuneInString(cl)
			p.put(clip, x, y, r, cl[n:], w, look)
		}
		return true
	})
}

// runLooks is the cell style of each span of rs: the span's computed
// style (set by the host), else the text's own (st), with st's owner.
func runLooks(rs *layout.TextRuns, st cellStyle) []cellStyle {
	out := make([]cellStyle, len(rs.Spans))
	for i, sp := range rs.Spans {
		out[i] = st
		if sp.Style != nil {
			out[i].fg, out[i].bg, out[i].attrs = sp.Style.Color, sp.Style.Background, attrsOf(sp.Style)
		}
	}
	return out
}
