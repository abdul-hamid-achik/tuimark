package ir

// EditDistance is the optimal-string-alignment distance between a and b
// (insertions, deletions, substitutions, and adjacent transpositions each
// cost 1), counted in bytes: the vocabularies it is used on (tags,
// attributes, properties, themes) are ASCII.
func EditDistance(a, b string) int {
	if a == b {
		return 0
	}
	// Three rolling rows: two back, one back, current.
	prev2 := make([]int, len(b)+1)
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			d := min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				d = min(d, prev2[j-2]+1)
			}
			cur[j] = d
		}
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(b)]
}

// Suggest returns the candidate closest to name for a "did you mean"
// hint, or "" when none is close enough: the distance must be at most 2
// and below len(name)/2+1, so a very short name gets no suggestion and a
// far one neither. Of equally close candidates the first in candidates'
// order wins, so a caller that passes a catalog in its documented order
// gets a deterministic answer.
func Suggest(name string, candidates []string) string {
	best, bestD := "", len(name)/2+1
	if bestD > 3 {
		bestD = 3 // at most distance 2 is ever suggested
	}
	for _, c := range candidates {
		if c == name {
			continue
		}
		// The distance is at least the difference of the lengths, so a
		// candidate that far off cannot beat the best: skip the work.
		if diff := len(name) - len(c); diff >= bestD || -diff >= bestD {
			continue
		}
		if d := EditDistance(name, c); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}
