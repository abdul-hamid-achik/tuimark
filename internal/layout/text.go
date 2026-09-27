package layout

import (
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/uniwidth"
)

// Text is measured, cut, and wrapped by extended grapheme cluster, each 0,
// 1, or 2 columns wide (SPEC v0.2 §11.5). A cluster is never split. For
// ASCII and box-drawing text every rune is one cluster of width 1, so these
// functions behave exactly as the v0.1 rune-counting ones.

// Width is the display width of s in columns: the sum of the widths of its
// grapheme clusters (§11.5.2).
func Width(s string) int { return uniwidth.Width(s) }

// InputShown is the text an input paints for its value (SPEC v0.2 §11.5.2,
// §12.1): the value itself, or for a secret input one • (width 1) per
// cluster of the value, so a secret's geometry depends only on its cluster
// count and never on the display width of what was typed.
func InputShown(b *Box) string {
	if b.Secret {
		return strings.Repeat("•", uniwidth.Count(b.Text))
	}
	return b.Text
}

// Cut returns the longest prefix of whole clusters of s that is at most n
// columns wide (empty when n <= 0). A width-2 cluster that would straddle
// column n is left out, so the result may be n-1 columns wide.
func Cut(s string, n int) string {
	b, _ := uniwidth.Prefix(s, n)
	return s[:b]
}

// Truncate returns s when it fits in n columns, else cuts it to n-1 columns
// and marks the cut with an ellipsis (§11.5.2, wrap: truncate). The
// ellipsis follows the cut prefix directly, so the result may be one column
// narrower than n.
func Truncate(s string, n int) string {
	if Width(s) <= n {
		return s
	}
	if n <= 0 {
		return ""
	}
	if n == 1 {
		return "…"
	}
	return Cut(s, n-1) + "…"
}

// Lines splits text into display lines for a content width and wrap mode
// ("wrap", "truncate", or "nowrap"/""). Lines are not padded.
func Lines(text string, width int, mode string) []string {
	if text == "" {
		return nil
	}
	paras := strings.Split(text, "\n")
	switch mode {
	case "wrap":
		if width <= 0 {
			return paras
		}
		var out []string
		for _, p := range paras {
			out = append(out, Wrap(p, width)...)
		}
		return out
	case "truncate":
		out := make([]string, len(paras))
		for i, p := range paras {
			if width > 0 {
				out[i] = Truncate(p, width)
			} else {
				out[i] = p
			}
		}
		return out
	}
	return paras
}

// Wrap greedily word-wraps one paragraph to width columns (§11.5.2). Words
// are the runs strings.Fields finds; a line is words joined by one space.
// A word wider than the width is broken hard at cluster boundaries into
// pieces of at most width columns, and a single cluster wider than the
// width (a CJK character at width 1) is a piece by itself, left to the
// paint clip rule. There is no UAX #14 line breaking: a run of CJK without
// spaces is one word. An empty paragraph yields one empty line.
func Wrap(p string, width int) []string {
	words := strings.Fields(p)
	if len(words) == 0 {
		return []string{""}
	}
	var out []string
	line, lineW := "", 0
	for _, w := range words {
		ww := Width(w)
		for ww > width {
			if line != "" {
				out = append(out, line)
				line, lineW = "", 0
			}
			b, pw := uniwidth.Prefix(w, width)
			if b == 0 {
				// Not even the first cluster fits: it is a piece by itself.
				c, _, cw, _ := uniwidth.Next(w, -1)
				b, pw = len(c), cw
			}
			out = append(out, w[:b])
			w, ww = w[b:], ww-pw
		}
		if w == "" {
			continue
		}
		switch {
		case line == "":
			line, lineW = w, ww
		case lineW+1+ww <= width:
			line += " " + w
			lineW += 1 + ww
		default:
			out = append(out, line)
			line, lineW = w, ww
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

// MaxWidth returns the width of the widest paragraph of text.
func MaxWidth(text string) int {
	m := 0
	for _, p := range strings.Split(text, "\n") {
		if w := Width(p); w > m {
			m = w
		}
	}
	return m
}
