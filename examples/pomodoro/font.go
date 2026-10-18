package main

import "strings"

// The big-digit font: every glyph is fontRows rows tall and fontWidth cells
// wide (the colon is colonWidth wide), drawn with block characters. A
// glyph's rows are padded with spaces to its width, so a line of glyphs is
// as wide as it is, whichever digits it shows, and a blinking colon (drawn
// blank) never moves the digits.
const (
	fontRows   = 5
	fontWidth  = 6
	colonWidth = 2
	glyphGap   = " " // between two glyphs
)

var digitGlyphs = [10][fontRows]string{
	{"▄████▄", "██  ██", "██  ██", "██  ██", "▀████▀"}, // 0
	{" ▄██  ", "▀███  ", "  ██  ", "  ██  ", "▄████▄"}, // 1
	{"▄████▄", "▀▀  ██", " ▄██▀ ", "██▀   ", "██████"}, // 2
	{"██████", "   ▄█▀", "   ▀█▄", "██  ██", "▀████▀"}, // 3
	{"██  ██", "██  ██", "▀█████", "    ██", "    ██"}, // 4
	{"██████", "██    ", "▀████▄", "    ██", "▀████▀"}, // 5
	{"▄████▄", "██    ", "█████▄", "██  ██", "▀████▀"}, // 6
	{"██████", "▀▀  ██", "   ██▀", "  ██  ", "  ██  "}, // 7
	{"▄████▄", "██  ██", "▀████▀", "██  ██", "▀████▀"}, // 8
	{"▄████▄", "██  ██", "▀█████", "    ██", "▀████▀"}, // 9
}

var colonGlyph = [fontRows]string{"  ", "██", "  ", "██", "  "}

// blankColon is the colon while it blinks off: the same width, no ink.
var blankColon = [fontRows]string{"  ", "  ", "  ", "  ", "  "}

// bigText renders s in the big font and returns its fontRows lines joined
// by "\n", which a <text> shows as fontRows rows. s holds digits and ':' (a
// colon, drawn when colon is true and blank otherwise); any other rune is
// skipped. Every line has the same width, bigWidth(s).
func bigText(s string, colon bool) string {
	rows := make([]strings.Builder, fontRows)
	first := true
	for _, r := range s {
		var g [fontRows]string
		switch {
		case r >= '0' && r <= '9':
			g = digitGlyphs[r-'0']
		case r == ':':
			g = blankColon
			if colon {
				g = colonGlyph
			}
		default:
			continue
		}
		for i := range rows {
			if !first {
				rows[i].WriteString(glyphGap)
			}
			rows[i].WriteString(g[i])
		}
		first = false
	}
	lines := make([]string, fontRows)
	for i := range rows {
		lines[i] = rows[i].String()
	}
	return strings.Join(lines, "\n")
}

// bigWidth is the width in cells of bigText(s, …).
func bigWidth(s string) int {
	w, n := 0, 0
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			w += fontWidth
		case r == ':':
			w += colonWidth
		default:
			continue
		}
		n++
	}
	if n > 1 {
		w += (n - 1) * len(glyphGap)
	}
	return w
}
