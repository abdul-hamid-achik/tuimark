package layout

import (
	"math/big"

	"github.com/abdul-hamid-achik/tuimark/internal/ir"
)

// maxCells bounds every resolved size, margin, and padding so coordinate
// sums (x + w, pos + size) cannot overflow an int, whatever the markup says.
// Paint only walks the visible cells, so a huge box costs nothing extra.
const maxCells = 1 << 40

// clampCells bounds an integer cell count to [0, maxCells].
func clampCells(v int) int { return min(max(v, 0), maxCells) }

// cells converts a parsed cell count to an int in [0, maxCells]. Cell
// literals are whole numbers (ir.ParseScalar), and their float64 is exact
// up to 2^53, far past maxCells, so the float64 is enough here.
func cells(n float64) int {
	if !(n > 0) { // also NaN
		return 0
	}
	if n >= maxCells {
		return maxCells
	}
	return int(n)
}

// smallInt reports whether s is a whole number small enough for the
// integer fast paths below (products stay under 2^60). Wholeness is decided
// on the literal: 2.0000000000000001 is not whole even though its float64
// is exactly 2.
func smallInt(s ir.Scalar) bool {
	return s.N >= 0 && s.N < 1<<20 && s.IsWhole()
}

// floorRat returns floor(r) as a cell count in [0, maxCells].
func floorRat(r *big.Rat) int {
	if r.Sign() <= 0 {
		return 0
	}
	q := new(big.Int).Quo(r.Num(), r.Denom())
	if !q.IsInt64() || q.Int64() >= maxCells {
		return maxCells
	}
	return int(q.Int64())
}

// percent returns floor(base * s / 100) for a percent s, computed exactly
// on its decimal literal (SPEC §11.4).
func percent(base int, s ir.Scalar) int {
	if base <= 0 || !(s.N > 0) { // N > 0 exactly when the literal is
		return 0
	}
	base = min(base, maxCells)
	if smallInt(s) {
		return min(base*int(s.N)/100, maxCells)
	}
	r := new(big.Rat).Mul(big.NewRat(int64(base), 1), s.Rat())
	return floorRat(r.Quo(r, big.NewRat(100, 1)))
}

// frSplit shares remain between fr weights per SPEC §11.4: every weight but
// the last takes floor(remain * w / sum), computed exactly on the decimal
// literals; the last takes what is left.
func frSplit(remain int, weights []ir.Scalar) []int {
	out := make([]int, len(weights))
	n := len(weights)
	if n == 0 {
		return out
	}
	remain = clampCells(remain)
	used := 0
	ints, isum := true, 0
	for _, w := range weights {
		if !smallInt(w) {
			ints = false
			break
		}
		isum += int(w.N)
	}
	switch {
	case ints && isum > 0:
		for i := 0; i < n-1; i++ {
			out[i] = remain * int(weights[i].N) / isum
			used += out[i]
		}
	default:
		ws := make([]*big.Rat, n)
		sum := new(big.Rat)
		for i, w := range weights {
			ws[i] = w.Rat()
			sum.Add(sum, ws[i])
		}
		if sum.Sign() > 0 {
			rem := big.NewRat(int64(remain), 1)
			for i := 0; i < n-1; i++ {
				t := new(big.Rat).Mul(rem, ws[i])
				out[i] = floorRat(t.Quo(t, sum))
				used += out[i]
			}
		}
	}
	out[n-1] = remain - used
	return out
}
