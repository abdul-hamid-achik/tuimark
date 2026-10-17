package lsp

import "strings"

// Position is an LSP position: a 0-based line and a UTF-16 offset in it.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is an LSP range, end exclusive.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// splitLines splits text at LF. A CR stays on its line, which is how the
// parsers count a column (every code point, a CR included), so a position
// validate prints lands on the same character here.
func splitLines(text string) []string {
	return strings.Split(text, "\n")
}

// runeUnits is the number of UTF-16 code units of r: 2 outside the BMP.
func runeUnits(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

// utf16Len is the number of UTF-16 code units of s.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += runeUnits(r)
	}
	return n
}

// fromDiag converts a diagnostic's position, as validate prints it (a
// 1-based line and a 1-based column counted in code points), to an LSP
// one (SPEC §15.12 "Positions"). A line of zero (no position) is the start
// of the file; a position past the end of its line or of the text is
// clamped to it.
func fromDiag(lines []string, line, col int) Position {
	if line < 1 {
		return Position{}
	}
	if line > len(lines) {
		line = len(lines)
		return Position{Line: line - 1, Character: utf16Len(lines[line-1])}
	}
	n := 0
	for _, r := range lines[line-1] {
		if col <= 1 {
			break
		}
		col--
		n += runeUnits(r)
	}
	return Position{Line: line - 1, Character: n}
}

// byteOffset is the byte offset of an LSP position in text, clamped to its
// line (a character inside a surrogate pair rounds down to the pair's
// start) and to the text.
func byteOffset(text string, p Position) int {
	off := 0
	for i := 0; i < p.Line; i++ {
		nl := strings.IndexByte(text[off:], '\n')
		if nl < 0 {
			return len(text)
		}
		off += nl + 1
	}
	line := text[off:]
	if nl := strings.IndexByte(line, '\n'); nl >= 0 {
		line = line[:nl]
	}
	units := 0
	for i, r := range line {
		u := runeUnits(r)
		if units+u > p.Character {
			return off + i
		}
		units += u
	}
	return off + len(line)
}

// positionAt is the LSP position of a byte offset in text.
func positionAt(text string, off int) Position {
	if off > len(text) {
		off = len(text)
	}
	head := text[:off]
	start := strings.LastIndexByte(head, '\n') + 1
	return Position{Line: strings.Count(head, "\n"), Character: utf16Len(head[start:])}
}

// endPosition is the position just past the last character of text.
func endPosition(text string) Position { return positionAt(text, len(text)) }
