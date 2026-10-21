package host

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// vt is a small terminal emulator for the tests of inline mode (SPEC
// v0.4d §26.15): it follows what Run writes the way a terminal would,
// with a screen of rows × cols cells, a cursor, and a scrollback that
// takes the rows an LF pushes off the top. It knows CR, LF, BS, and the
// CSI sequences CUU, CUD, CUF, CUB, CHA, CUP, ED, EL, ECH, SGR, and the
// private modes; it records the sequences an inline session must never
// write (CSI H, CSI 2 J, an absolute row, CSI ? 1049).
type vt struct {
	mu         sync.Mutex
	cols, rows int
	cells      [][]rune
	cx, cy     int
	scrollback []string
	cursorOn   bool
	// forbidden holds every sequence of the kind an inline session never
	// writes: CUP/HVP/VPA (an absolute row), ED 2/3, and mode 1049.
	forbidden []string
	// raw is every byte written.
	raw []byte
	// sgr is the text of the SGR sequences seen, in order.
	pend []byte
}

func newVT(cols, rows int) *vt {
	v := &vt{cols: cols, rows: rows, cursorOn: true}
	v.cells = make([][]rune, rows)
	for i := range v.cells {
		v.cells[i] = v.blankRow()
	}
	return v
}

func (v *vt) blankRow() []rune {
	r := make([]rune, v.cols)
	for i := range r {
		r[i] = ' '
	}
	return r
}

// Write implements io.Writer.
func (v *vt) Write(p []byte) (int, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.raw = append(v.raw, p...)
	b := append(v.pend, p...)
	v.pend = nil
	for len(b) > 0 {
		switch {
		case b[0] == 0x1b:
			if len(b) < 2 {
				v.pend = append([]byte(nil), b...)
				return len(p), nil
			}
			if b[1] != '[' {
				b = b[2:]
				continue
			}
			end := -1
			for i := 2; i < len(b); i++ {
				if b[i] >= 0x40 && b[i] <= 0x7e {
					end = i
					break
				}
			}
			if end < 0 {
				v.pend = append([]byte(nil), b...)
				return len(p), nil
			}
			v.csi(string(b[2:end]), b[end])
			b = b[end+1:]
		case b[0] == '\r':
			v.cx = 0
			b = b[1:]
		case b[0] == '\n':
			v.lf()
			b = b[1:]
		case b[0] == '\b':
			v.cx = max(0, v.cx-1)
			b = b[1:]
		case b[0] < 0x20:
			b = b[1:]
		default:
			if !utf8.FullRune(b) {
				v.pend = append([]byte(nil), b...)
				return len(p), nil
			}
			r, n := utf8.DecodeRune(b)
			b = b[n:]
			if v.cx < v.cols { // autowrap is off: a write past the last column is lost
				v.cells[v.cy][v.cx] = r
				v.cx++
			}
		}
	}
	return len(p), nil
}

func (v *vt) lf() {
	if v.cy == v.rows-1 {
		v.scrollback = append(v.scrollback, strings.TrimRight(string(v.cells[0]), " "))
		copy(v.cells, v.cells[1:])
		v.cells[v.rows-1] = v.blankRow()
		return
	}
	v.cy++
}

func (v *vt) csi(params string, final byte) {
	private := strings.HasPrefix(params, "?")
	params = strings.TrimPrefix(params, "?")
	nums := []int{}
	for _, f := range strings.Split(params, ";") {
		n, _ := strconv.Atoi(f)
		nums = append(nums, n)
	}
	n1 := func() int {
		if nums[0] == 0 {
			return 1
		}
		return nums[0]
	}
	switch final {
	case 'A':
		v.cy = max(0, v.cy-n1())
	case 'B':
		v.cy = min(v.rows-1, v.cy+n1())
	case 'C':
		v.cx = min(v.cols-1, v.cx+n1())
	case 'D':
		v.cx = max(0, v.cx-n1())
	case 'G':
		v.cx = min(v.cols-1, max(0, n1()-1))
	case 'H', 'f', 'd':
		v.forbidden = append(v.forbidden, fmt.Sprintf("CSI %s%c", params, final))
	case 'J':
		switch nums[0] {
		case 0:
			for x := v.cx; x < v.cols; x++ {
				v.cells[v.cy][x] = ' '
			}
			for y := v.cy + 1; y < v.rows; y++ {
				v.cells[y] = v.blankRow()
			}
		default:
			v.forbidden = append(v.forbidden, fmt.Sprintf("CSI %dJ", nums[0]))
		}
	case 'K':
		for x := v.cx; x < v.cols; x++ {
			v.cells[v.cy][x] = ' '
		}
	case 'X':
		for x := v.cx; x < min(v.cols, v.cx+n1()); x++ {
			v.cells[v.cy][x] = ' '
		}
	case 'h', 'l':
		if private {
			for _, n := range nums {
				switch n {
				case 25:
					v.cursorOn = final == 'h'
				case 1049:
					v.forbidden = append(v.forbidden, fmt.Sprintf("CSI ?%d%c", n, final))
				}
			}
		}
	}
}

// screenRows returns the screen's rows with trailing spaces trimmed.
func (v *vt) screenRows() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	out := make([]string, v.rows)
	for i, r := range v.cells {
		out[i] = strings.TrimRight(string(r), " ")
	}
	return out
}

// text is the scrollback followed by the screen, trailing empty rows
// dropped, one row per line.
func (v *vt) text() string {
	v.mu.Lock()
	all := append(append([]string(nil), v.scrollback...))
	for _, r := range v.cells {
		all = append(all, strings.TrimRight(string(r), " "))
	}
	v.mu.Unlock()
	for len(all) > 0 && all[len(all)-1] == "" {
		all = all[:len(all)-1]
	}
	return strings.Join(all, "\n")
}

func (v *vt) cursor() (x, y int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.cx, v.cy
}

func (v *vt) bytes() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return string(v.raw)
}

func (v *vt) forbiddenSeen() []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]string(nil), v.forbidden...)
}

// write feeds s to the terminal as if the shell had written it.
func (v *vt) shell(s string) { _, _ = v.Write([]byte(s)) }
