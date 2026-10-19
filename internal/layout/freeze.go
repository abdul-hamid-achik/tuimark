package layout

import "github.com/abdul-hamid-achik/tuimark/internal/ir"

// frResolve is the fr split of the current language (SPEC v0.4b §11.4,
// decision 38): CSS §9.7's resolution of flexible lengths, in integers.
// It shares remain among the fr children frs of kids (indexes into kids,
// in document order), whose weights are weights[i], and writes their
// slots (border-box size plus margins) into sizes.
//
// Each round splits remain among the open children as frSplit does
// (floor, the last takes the rest), clamps each share to its max-* and
// then its min-* (clampAxis, % of main), and sums the violations v
// (clamped minus free). When v is 0 every open child is frozen at its
// clamped size; when v > 0 only those its min-* raised; when v < 0 only
// those its max-* lowered. The frozen slots leave remain, in document
// order and never below 0, and the others are split again. Every round
// freezes at least one child, so there are at most len(frs) rounds, and
// when no min-* or max-* binds the first round freezes them all: the
// result is then the single split of a pinned document (mainSizes).
func frResolve(kids []*Box, frs []int, weights []ir.Scalar, sizes []int, remain int, horizontal bool, main int) {
	open := append([]int(nil), frs...)
	ws := make([]ir.Scalar, 0, len(frs))
	free := make([]int, len(frs))
	inner := make([]int, len(frs))
	for len(open) > 0 {
		ws = ws[:0]
		for _, i := range open {
			ws = append(ws, weights[i])
		}
		takes := frSplit(remain, ws)
		v := 0
		for j, i := range open {
			ms, me := margins(kids[i], horizontal)
			free[j] = max(0, takes[j]-ms-me)
			inner[j] = clampAxis(kids[i], horizontal, free[j], main)
			v += inner[j] - free[j]
		}
		next := open[:0]
		for j, i := range open {
			if v == 0 || (v > 0 && inner[j] > free[j]) || (v < 0 && inner[j] < free[j]) {
				ms, me := margins(kids[i], horizontal)
				sizes[i] = inner[j] + ms + me
				remain = max(0, remain-sizes[i])
				continue
			}
			next = append(next, i)
		}
		// next reuses open's array from its start: it only ever holds
		// elements read before they are overwritten.
		open = next
	}
}
