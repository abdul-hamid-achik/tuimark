package main

// The seven tetrominoes with the Super Rotation System: each piece lives in a
// square box (3x3, or 4x4 for I and O), its four rotation states are cell
// lists inside that box, and a rotation tries five kick offsets in order.
// Coordinates grow right and down.

type kind uint8

const (
	kNone kind = iota
	kI
	kO
	kT
	kS
	kZ
	kJ
	kL
)

// kinds is the bag, in the order a fresh bag is laid out before shuffling.
var kinds = [7]kind{kI, kO, kT, kS, kZ, kJ, kL}

const letters = ".IOTSZJL"

func (k kind) letter() string { return string(letters[k]) }

// kindOf is the kind a letter names, kNone for "." or anything else.
func kindOf(s string) kind {
	if len(s) != 1 {
		return kNone
	}
	for i := 1; i < len(letters); i++ {
		if letters[i] == s[0] {
			return kind(i)
		}
	}
	return kNone
}

type point struct{ x, y int }

// shapes[k][r] are the four cells of piece k in rotation r (0 spawn, 1 right,
// 2 reversed, 3 left), inside the piece's box.
var shapes [8][4][4]point

func init() {
	spawn := map[kind][4]point{
		kI: {{0, 1}, {1, 1}, {2, 1}, {3, 1}},
		kO: {{1, 0}, {2, 0}, {1, 1}, {2, 1}},
		kT: {{1, 0}, {0, 1}, {1, 1}, {2, 1}},
		kS: {{1, 0}, {2, 0}, {0, 1}, {1, 1}},
		kZ: {{0, 0}, {1, 0}, {1, 1}, {2, 1}},
		kJ: {{0, 0}, {0, 1}, {1, 1}, {2, 1}},
		kL: {{2, 0}, {0, 1}, {1, 1}, {2, 1}},
	}
	for k, cells := range spawn {
		n := 3
		if k == kI {
			n = 4
		}
		shapes[k][0] = cells
		for r := 1; r < 4; r++ {
			if k == kO {
				shapes[k][r] = cells // a square looks the same from every side
				continue
			}
			for i, c := range shapes[k][r-1] {
				shapes[k][r][i] = point{n - 1 - c.y, c.x} // a quarter turn clockwise
			}
		}
	}
}

// turn is a rotation from one state to another.
type turn struct{ from, to int }

// The SRS wall-kick offsets, in the guideline's own convention (x to the
// right, y UP), keyed by the rotation state before and after. The first
// offset is always (0, 0): the plain rotation.
var kicksJLSTZ = map[turn][5]point{
	{0, 1}: {{0, 0}, {-1, 0}, {-1, 1}, {0, -2}, {-1, -2}},
	{1, 0}: {{0, 0}, {1, 0}, {1, -1}, {0, 2}, {1, 2}},
	{1, 2}: {{0, 0}, {1, 0}, {1, -1}, {0, 2}, {1, 2}},
	{2, 1}: {{0, 0}, {-1, 0}, {-1, 1}, {0, -2}, {-1, -2}},
	{2, 3}: {{0, 0}, {1, 0}, {1, 1}, {0, -2}, {1, -2}},
	{3, 2}: {{0, 0}, {-1, 0}, {-1, -1}, {0, 2}, {-1, 2}},
	{3, 0}: {{0, 0}, {-1, 0}, {-1, -1}, {0, 2}, {-1, 2}},
	{0, 3}: {{0, 0}, {1, 0}, {1, 1}, {0, -2}, {1, -2}},
}

var kicksI = map[turn][5]point{
	{0, 1}: {{0, 0}, {-2, 0}, {1, 0}, {-2, -1}, {1, 2}},
	{1, 0}: {{0, 0}, {2, 0}, {-1, 0}, {2, 1}, {-1, -2}},
	{1, 2}: {{0, 0}, {-1, 0}, {2, 0}, {-1, 2}, {2, -1}},
	{2, 1}: {{0, 0}, {1, 0}, {-2, 0}, {1, -2}, {-2, 1}},
	{2, 3}: {{0, 0}, {2, 0}, {-1, 0}, {2, 1}, {-1, -2}},
	{3, 2}: {{0, 0}, {-2, 0}, {1, 0}, {-2, -1}, {1, 2}},
	{3, 0}: {{0, 0}, {1, 0}, {-2, 0}, {1, -2}, {-2, 1}},
	{0, 3}: {{0, 0}, {-1, 0}, {2, 0}, {-1, 2}, {2, -1}},
}

// kickOffsets returns the five offsets for turning piece k from one rotation
// to another, with y DOWN (the guideline's y is flipped). O never moves when
// it turns, so its offsets are all (0, 0).
func kickOffsets(k kind, from, to int) [5]point {
	if k == kO {
		return [5]point{}
	}
	table := kicksJLSTZ
	if k == kI {
		table = kicksI
	}
	t := table[turn{from, to}]
	for i := range t {
		t[i].y = -t[i].y
	}
	return t
}

// piece is a tetromino on the field: its box's top left corner and rotation.
type piece struct {
	k    kind
	rot  int
	x, y int
}

func (p piece) cells() [4]point {
	var out [4]point
	for i, c := range shapes[p.k][p.rot] {
		out[i] = point{p.x + c.x, p.y + c.y}
	}
	return out
}
