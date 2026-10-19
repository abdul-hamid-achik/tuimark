package main

import "fmt"

// The Derive functions below are pure functions of the raw store (dims,
// level, state, secs, mines_left, flag_mode, best). They call no App
// method: Derive functions run under the app's lock.

type getter func(path string) (any, bool)

func num(get getter, path string) int {
	v, _ := get(path)
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func str(get getter, path string) string {
	v, _ := get(path)
	s, _ := v.(string)
	return s
}

func boolAt(get getter, path string) bool {
	v, _ := get(path)
	b, _ := v.(bool)
	return b
}

// clock is a time as m:ss.
func clock(secs int) string { return fmt.Sprintf("%d:%02d", secs/60, secs%60) }

// minesLabel is the mine counter: three digits, a minus sign if negative.
func minesLabel(n int) string {
	if n < 0 {
		if n < -99 {
			n = -99
		}
		return fmt.Sprintf("-%02d", -n)
	}
	if n > 999 {
		n = 999
	}
	return fmt.Sprintf("%03d", n)
}

// faceOf is the status face: two ASCII characters, one column each. The
// text faces (U+263A and friends) can be drawn as two-column emoji by a
// terminal font, which would shift the line.
func faceOf(state string) string {
	switch state {
	case "lost":
		return ":("
	case "won":
		return "B)"
	case "paused":
		return ":|"
	}
	return ":)"
}

var stateLabels = map[string]string{
	"ready": "ready", "playing": "playing", "paused": "paused", "lost": "boom", "won": "cleared",
}

// needCols and needRows are the terminal a cols x rows board needs: the
// board in its frame, plus the status line and the key hints; at least
// minTermCols so the status line fits.
const minTermCols = 36

func needCols(cols int) int {
	if w := 2*cols + 2; w > minTermCols {
		return w
	}
	return minTermCols
}

func needRows(rows int) int { return rows + 4 }

// levelHides is the widest terminal at which the status line, with the
// counters in it, has no room for the level label: the label (up to 28
// columns), the counters with the best time (up to 42), and the gaps.
const levelHides = 75

// sideRoom is the room the side panel takes beside the board: its width
// and the gap before it. Below needCols+sideRoom columns the panel gives
// way to the counters in the status line.
const sideRoom = 30

// derive installs the derived values, in dependency order.
func (a *app) derive() error {
	type d struct {
		path string
		fn   func(getter) (any, error)
	}
	ds := []d{
		{"mines_label", func(get getter) (any, error) { return minesLabel(num(get, "mines_left")), nil }},
		{"time_label", func(get getter) (any, error) { return clock(num(get, "secs")), nil }},
		{"face", func(get getter) (any, error) { return faceOf(str(get, "state")), nil }},
		{"state_label", func(get getter) (any, error) { return stateLabels[str(get, "state")], nil }},
		{"mode_label", func(get getter) (any, error) {
			if boolAt(get, "flag_mode") {
				return "flag", nil
			}
			return "reveal", nil
		}},
		{"level_label", func(get getter) (any, error) {
			return fmt.Sprintf("%s %dx%d, %d mines", str(get, "level"),
				num(get, "dims.cols"), num(get, "dims.rows"), num(get, "dims.mines")), nil
		}},
		{"need_label", func(get getter) (any, error) {
			return fmt.Sprintf("terminal too small: this board needs %d columns by %d rows",
				needCols(num(get, "dims.cols")), needRows(num(get, "dims.rows"))), nil
		}},
		{"is", func(get getter) (any, error) {
			s := str(get, "state")
			return map[string]any{
				"paused": s == "paused", "lost": s == "lost", "won": s == "won",
			}, nil
		}},
		{"lvl", func(get getter) (any, error) {
			l := str(get, "level")
			m := map[string]any{}
			for _, p := range presets {
				m[p.name] = l == p.name
			}
			return m, nil
		}},
		{"best_now", func(get getter) (any, error) {
			if s := num(get, "best."+str(get, "level")); s >= 0 && str(get, "level") != "custom" {
				return clock(s), nil
			}
			return "--", nil
		}},
		{"best_label", func(get getter) (any, error) {
			m := map[string]any{}
			for _, p := range presets {
				m[p.name] = "--"
				if s := num(get, "best."+p.name); s >= 0 {
					m[p.name] = clock(s)
				}
			}
			return m, nil
		}},
		{"fit", func(get getter) (any, error) {
			nc, nr := needCols(num(get, "dims.cols")), needRows(num(get, "dims.rows"))
			m := map[string]any{}
			for _, v := range fitCols() {
				m[fmt.Sprintf("nc%d", v)] = v == nc
			}
			for _, v := range fitRows() {
				m[fmt.Sprintf("nh%d", v)] = v == nr
			}
			return m, nil
		}},
	}
	for _, x := range ds {
		x := x
		if err := a.ui.Derive(x.path, func(get func(string) (any, bool)) (any, error) { return x.fn(get) }); err != nil {
			return err
		}
	}
	return nil
}

// fitCols and fitRows are every terminal width and height a board can
// need: the document has a class for each, and sizes.tcss a rule.
func fitCols() []int {
	var out []int
	seen := map[int]bool{}
	for c := minCols; c <= maxCols; c++ {
		if v := needCols(c); !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func fitRows() []int {
	var out []int
	for r := minRows; r <= maxRows; r++ {
		out = append(out, needRows(r))
	}
	return out
}
