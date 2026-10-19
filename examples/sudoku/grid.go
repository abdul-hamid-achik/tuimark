package main

import (
	"fmt"
	"math/bits"
	"strings"
)

// The geometry of a 9x9 grid, with cells numbered 0..80 row by row.

const (
	size  = 81
	units = 27 // 9 rows, 9 columns, 9 boxes
)

var (
	rowOf, colOf, boxOf [size]int
	unitCells           [units][9]int // 0-8 rows, 9-17 columns, 18-26 boxes
	peers               [size][20]int // the other cells sharing a row, column, or box
)

func init() {
	for i := 0; i < size; i++ {
		rowOf[i], colOf[i] = i/9, i%9
		boxOf[i] = i/27*3 + i%9/3
	}
	for i := 0; i < size; i++ {
		unitCells[rowOf[i]][colOf[i]] = i
		unitCells[9+colOf[i]][rowOf[i]] = i
		unitCells[18+boxOf[i]][i/9%3*3+i%3] = i
	}
	for i := 0; i < size; i++ {
		n := 0
		for j := 0; j < size; j++ {
			if j != i && (rowOf[j] == rowOf[i] || colOf[j] == colOf[i] || boxOf[j] == boxOf[i]) {
				peers[i][n] = j
				n++
			}
		}
	}
}

// grid is a board: 0 for an empty cell, else a digit 1..9.
type grid [size]byte

// Digit sets are 9-bit masks: bit d-1 stands for digit d.
const allDigits uint16 = 0x1FF

func bit(d int) uint16         { return 1 << (d - 1) }
func count(m uint16) int       { return bits.OnesCount16(m) }
func lowest(m uint16) int      { return bits.TrailingZeros16(m) + 1 }
func has(m uint16, d int) bool { return m&bit(d) != 0 }

// parseGrid reads 81 characters: digits 1-9, and 0 or . for an empty cell.
func parseGrid(s string) (grid, error) {
	var g grid
	if len(s) != size {
		return g, fmt.Errorf("a puzzle has %d characters, got %d", size, len(s))
	}
	for i := 0; i < size; i++ {
		switch c := s[i]; {
		case c >= '1' && c <= '9':
			g[i] = c - '0'
		case c == '0' || c == '.':
		default:
			return g, fmt.Errorf("character %d is %q: want a digit 0-9 or .", i+1, c)
		}
	}
	return g, nil
}

// String writes the grid as 81 digits, 0 for an empty cell.
func (g grid) String() string {
	var b strings.Builder
	for _, d := range g {
		b.WriteByte('0' + d)
	}
	return b.String()
}

// clues is how many cells hold a digit.
func (g grid) clues() int {
	n := 0
	for _, d := range g {
		if d != 0 {
			n++
		}
	}
	return n
}

// conflicts reports whether a digit repeats in a row, column, or box.
func (g grid) conflicts() bool {
	for _, u := range unitCells {
		var seen uint16
		for _, i := range u {
			if g[i] == 0 {
				continue
			}
			if seen&bit(int(g[i])) != 0 {
				return true
			}
			seen |= bit(int(g[i]))
		}
	}
	return false
}
