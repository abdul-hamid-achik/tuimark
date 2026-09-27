package layout

import (
	"strings"
	"unicode/utf8"
)

// Width is the display width of s. v1 treats every rune as one cell
// (SPEC §11.5); wide-character support is deferred.
func Width(s string) int { return utf8.RuneCountInString(s) }

// Cut returns the first n cells of s.
func Cut(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if Width(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

// Truncate cuts s to n cells, marking the cut with an ellipsis.
func Truncate(s string, n int) string {
	if Width(s) <= n {
		return s
	}
	if n <= 1 {
		return Cut("…", n)
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

// Wrap greedily word-wraps one paragraph to width cells. Words longer than
// the width are broken hard. An empty paragraph yields one empty line.
func Wrap(p string, width int) []string {
	words := strings.Fields(p)
	if len(words) == 0 {
		return []string{""}
	}
	var out []string
	line := ""
	for _, w := range words {
		for Width(w) > width {
			if line != "" {
				out = append(out, line)
				line = ""
			}
			r := []rune(w)
			out = append(out, string(r[:width]))
			w = string(r[width:])
		}
		if w == "" {
			continue
		}
		switch {
		case line == "":
			line = w
		case Width(line)+1+Width(w) <= width:
			line += " " + w
		default:
			out = append(out, line)
			line = w
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

// MaxWidth returns the widest paragraph of text.
func MaxWidth(text string) int {
	m := 0
	for _, p := range strings.Split(text, "\n") {
		if w := Width(p); w > m {
			m = w
		}
	}
	return m
}
