package layout

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/abdul-hamid-achik/tuimark/internal/uniwidth"
)

// ModePreWrap is the computed wrap value `pre-wrap` (SPEC v0.4d §10.3,
// §11.5.2): every space and line break of a text is kept, and the text is
// wrapped at the content width. Only the current language accepts it.
const ModePreWrap = "pre-wrap"

// PreWrap wraps text at n columns keeping every space (SPEC v0.4d §11.5.2,
// decision 213), as CSS's white-space: pre-wrap does. Each hard line (split
// at LF) is wrapped on its own, and an empty hard line is one empty line.
// A hard line is split into segments: maximal runs of space characters
// (unicode.IsSpace) and maximal runs of other characters, the words. The
// segments are placed in order on the current line:
//
//   - a segment of spaces always joins the line, even past n: the spaces
//     that do not fit hang past the width (the paint clips them);
//   - a word joins the line when the line's width plus the word's is at
//     most n;
//   - otherwise, when the line is not empty, the line ends there and the
//     word starts the next one, so a line may hold only spaces;
//   - a word wider than n on an empty line is broken hard at cluster
//     boundaries into pieces of at most n columns, as Wrap breaks it, and
//     the last piece stays on the line for what follows.
//
// An empty text has no hard line and gives no line: PreWrap("", n) is [].
// The lines are substrings of text, in order. n must be positive; for
// n <= 0 there is nothing to wrap at and each hard line is one line.
func PreWrap(text string, n int) []string {
	if text == "" {
		return nil
	}
	var out []string
	preWrapRanges(text, n, func(start, end int) {
		out = append(out, text[start:end])
	})
	return out
}

// preWrapLineCount is len(PreWrap(text, n)) without building the lines.
func preWrapLineCount(text string, n int) int {
	if text == "" {
		return 0
	}
	c := 0
	preWrapRanges(text, n, func(int, int) { c++ })
	return c
}

// preWrapRanges calls emit(start, end) for each line PreWrap gives, as the
// byte range of text it is. text is not empty.
func preWrapRanges(text string, n int, emit func(start, end int)) {
	for ls := 0; ; {
		le := len(text)
		more := false
		if k := strings.IndexByte(text[ls:], '\n'); k >= 0 {
			le, more = ls+k, true
		}
		if n <= 0 || ls == le {
			emit(ls, le)
		} else {
			preWrapLine(text, ls, le, n, emit)
		}
		if !more {
			return
		}
		ls = le + 1
	}
}

// preWrapLine wraps the hard line text[ls:le] (not empty, n > 0).
func preWrapLine(text string, ls, le, n int, emit func(start, end int)) {
	start, lineW := ls, 0
	for i := ls; i < le; {
		r, _ := utf8.DecodeRuneInString(text[i:le])
		space := unicode.IsSpace(r)
		j := i
		for j < le {
			r, size := utf8.DecodeRuneInString(text[j:le])
			if unicode.IsSpace(r) != space {
				break
			}
			j += size
		}
		seg := text[i:j]
		ww := Width(seg)
		switch {
		case space || lineW+ww <= n:
			lineW += ww
		default:
			if i > start {
				emit(start, i)
				start, lineW = i, 0
			}
			// The word is on an empty line. A word wider than n is broken
			// hard into pieces of at most n columns; the last piece stays.
			for ww > n {
				b, pw := uniwidth.Prefix(seg, n)
				if b == 0 {
					// Not even the first cluster fits: it is a piece by itself.
					c, _, cw, _ := uniwidth.Next(seg, -1)
					b, pw = len(c), cw
				}
				emit(start, start+b)
				start += b
				seg, ww = seg[b:], ww-pw
			}
			lineW = ww
		}
		i = j
	}
	if start < le {
		emit(start, le)
	}
}

// LineWidth is the width of a line for content-align (SPEC v0.4d
// §11.5.2, decision 213): its trailing spaces hang and are not painted, so
// a pre-wrap line is aligned without them. Any other line is its Width.
func LineWidth(line, mode string) int {
	if mode == ModePreWrap {
		return Width(strings.TrimRightFunc(line, unicode.IsSpace))
	}
	return Width(line)
}
