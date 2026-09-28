package tuimark_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// SPEC v0.2b §21 test 70 (acceptance): the goldens of §17.4.5 pin what
// the table "What they pin" asks of each one. `tuimark test` compares the
// goldens byte for byte (TestTestCommandPasses); this test reads them and
// checks each promise, so a regenerated golden that lost one fails here.
// The mouse, the dispatch parity (71), and the hints property (57) over
// the same goldens are in internal/host.

type goldenDump struct {
	Focus  *string   `json:"focus"`
	Wide   bool      `json:"wide"`
	Theme  string    `json:"theme"`
	Nodes  []monNode `json:"nodes"`
	Grid   []string
	Events []struct {
		Step   int    `json:"step"`
		Action string `json:"action"`
		Source string `json:"source"`
		Value  any    `json:"value"`
	} `json:"events"`
	Styles [][]struct {
		X  int      `json:"x"`
		W  int      `json:"w"`
		Fg string   `json:"fg"`
		Bg string   `json:"bg"`
		A  []string `json:"a"`
	} `json:"styles"`
}

type monNode struct {
	ID     string `json:"id"`
	Tag    string `json:"tag"`
	X, Y   int
	W, H   int
	Text   string `json:"text"`
	Scroll *struct {
		Y, H int
	} `json:"scroll"`
}

func readGolden(t *testing.T, name, file string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata/golden", name, file))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func readGoldenJSON(t *testing.T, name, size string) goldenDump {
	t.Helper()
	var d goldenDump
	if err := json.Unmarshal([]byte(readGolden(t, name, size+".json")), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// gridOf is the grid section of a text golden.
func gridOf(t *testing.T, name, size string) []string {
	t.Helper()
	txt := readGolden(t, name, size+".txt")
	_, rest, _ := strings.Cut(txt, "\n")
	grid, _, _ := strings.Cut(rest, "=== nodes ===")
	return strings.Split(strings.TrimRight(grid, "\n"), "\n")
}

// eventsOf is the events section of a play text golden.
func eventsOf(t *testing.T, name, size string) []string {
	t.Helper()
	_, evs, _ := strings.Cut(readGolden(t, name, size+".txt"), "=== events ===\n")
	return strings.Split(strings.TrimSpace(evs), "\n")
}

// nodeLines are the node lines of a text golden.
func nodeLines(t *testing.T, name, size string) []string {
	t.Helper()
	_, rest, _ := strings.Cut(readGolden(t, name, size+".txt"), "=== nodes ===\n")
	nodes, _, _ := strings.Cut(rest, "=== ")
	return strings.Split(strings.TrimSpace(nodes), "\n")
}

func countTag(lines []string, tag string) int {
	n := 0
	for _, l := range lines {
		if f := strings.Fields(l); len(f) > 1 && f[1] == tag {
			n++
		}
	}
	return n
}

func TestMonitorAcceptance(t *testing.T) {
	t.Run("processes columns and name bounds", func(t *testing.T) {
		// 0.3b (SPEC v0.3b §17.4, test 107): the columns hide by
		// priority, no longer by @media: 6, 6, 7, and 7 at 57, 58, 78, and
		// 100 columns, and 7 and 4 in monitor-tabs' 80 and 40.
		for size, want := range map[string]int{"57x24": 6, "58x24": 6, "78x24": 7, "100x30": 7} {
			if got := countTag(nodeLines(t, "monitor-processes", size), "column"); got != want {
				t.Errorf("%s: %d columns, want %d", size, got, want)
			}
		}
		for size, want := range map[string]int{"80x24": 7, "40x24": 4} {
			if got := countTag(nodeLines(t, "monitor-tabs", size), "column"); got != want {
				t.Errorf("monitor-tabs %s: %d columns, want %d", size, got, want)
			}
		}
		d := readGoldenJSON(t, "monitor-processes", "100x30")
		if !d.Wide {
			t.Error("100x30 is not \"wide\"")
		}
		var table, pid monNode
		for _, n := range d.Nodes {
			switch n.ID {
			case "procs":
				table = n
			case "c-pid":
				pid = n
			case "c-name":
				if n.W < 10 || n.W > 44 {
					t.Errorf("c-name is %d wide", n.W)
				}
			}
		}
		// The mark channel (▸ plus a space) sits inside the border.
		if pid.X != table.X+1+2 {
			t.Errorf("c-pid at x %d, table at %d: no mark channel", pid.X, table.X)
		}
		for _, l := range gridOf(t, "monitor-processes", "58x24") {
			if strings.Contains(l, "微信") && !strings.Contains(l, "4242 微信") {
				t.Errorf("微信 row misaligned: %q", l)
			}
		}
	})
	t.Run("processes styles", func(t *testing.T) {
		d := readGoldenJSON(t, "monitor-processes-styles", "80x24")
		has := func(y int, pred func(fg string, a []string) bool) bool {
			for _, s := range d.Styles[y] {
				if pred(s.Fg, s.A) {
					return true
				}
			}
			return false
		}
		reverse := func(_ string, a []string) bool { return strings.Join(a, ",") == "reverse" }
		warn := func(fg string, _ []string) bool { return fg == "#e0a23e" }
		if !has(4, reverse) || has(5, reverse) {
			t.Error("the cursor row (y 4) is not the only reverse row")
		}
		for y := 4; y <= 6; y++ {
			if !has(y, warn) {
				t.Errorf("row %d has no .hot cell in $warn", y)
			}
		}
		if has(7, warn) {
			t.Error("row 3 of the data is .hot")
		}
		for y, l := range d.Grid {
			if strings.Contains(l, "launchd") {
				danger := 0
				for _, s := range d.Styles[y] {
					if s.Fg == "#e0443e" && s.X > 0 && s.X+s.W < 79 {
						danger += s.W
					}
				}
				if danger < len("launchd")+1 {
					t.Errorf("the .blocked row is not in $danger: %v", d.Styles[y])
				}
			}
		}
	})
	t.Run("tab tiers", func(t *testing.T) {
		for size, want := range map[string]string{"120x24": " 1 overview   2 cpu", "80x24": " 1 ovr   2 cpu", "40x24": "▸‹ 7 processes ›"} {
			if got := gridOf(t, "monitor-tabs", size)[1]; !strings.HasPrefix(got, want) {
				t.Errorf("%s strip %q, want prefix %q", size, got, want)
			}
		}
	})
	t.Run("overview grid and light palette", func(t *testing.T) {
		for size, want := range map[string]int{"58x24": 1, "80x24": 2, "120x30": 2} {
			grid := gridOf(t, "monitor-overview", size)
			if got := strings.Count(grid[2], "╭"); got != want {
				t.Errorf("%s: %d panels on the first grid row, want %d: %q", size, got, want, grid[2])
			}
		}
		// Two sparklines, and gauges drawn with eighths.
		if countTag(nodeLines(t, "monitor-overview", "80x24"), "sparkline") != 2 || !strings.ContainsAny(strings.Join(gridOf(t, "monitor-overview", "80x24"), "\n"), "▏▎▍▌▋▊▉") {
			t.Error("no sparklines or no eighths")
		}
		d := readGoldenJSON(t, "monitor-overview-light", "120x30")
		if d.Theme != "light" || d.Styles[0][0].Bg != "#f3efeb" || d.Styles[3][1].Bg != "#fbfaf8" {
			t.Errorf("light palette: theme %q, %v / %v", d.Theme, d.Styles[0][0], d.Styles[3][1])
		}
	})
	t.Run("cpu grid and scrollbar", func(t *testing.T) {
		// 0.3b (test 107): with row-gap: 0 on #cores, 12 cores in one
		// column are 12 rows tall: #cores-view scrolls at 40x12 (9 rows,
		// scroll {0, 12}, the thumb on rows 1-4) and fits at 40x24 (14
		// rows, the same scroll, no thumb).
		d := readGoldenJSON(t, "monitor-cpu", "40x12")
		for _, n := range d.Nodes {
			if n.ID == "cores-view" && (n.H != 9 || n.Scroll == nil || n.Scroll.Y != 0 || n.Scroll.H != 12) {
				t.Errorf("cores-view %+v scroll %+v", n, n.Scroll)
			}
		}
		for y, l := range gridOf(t, "monitor-cpu", "40x12")[3:10] {
			if want := fmt.Sprintf("│cpu%d ", y); !strings.HasPrefix(l, want) {
				t.Errorf("40x12 content row %d is %q, want core %d", y, l, y)
			}
		}
		thumb := func(size string) []int {
			var rows []int
			for y, l := range gridOf(t, "monitor-cpu", size) {
				if strings.HasSuffix(l, "┃") {
					rows = append(rows, y-2)
				}
			}
			return rows
		}
		if got := thumb("40x12"); fmt.Sprint(got) != "[1 2 3 4]" {
			t.Errorf("40x12 thumb rows %v, want [1 2 3 4]", got)
		}
		fits := false
		for _, l := range nodeLines(t, "monitor-cpu", "40x24") {
			if f := strings.Fields(l); len(f) > 3 && f[0] == "cores-view" {
				fits = f[2] == "40x14"
			}
		}
		if !fits {
			t.Error("40x24: #cores-view is not 40x14")
		}
		for size, cols := range map[string]int{"40x24": 1, "80x24": 3, "120x30": 4} {
			if len(thumb(size)) > 0 {
				t.Errorf("%s has a thumb", size)
			}
			if got := strings.Count(gridOf(t, "monitor-cpu", size)[3], "cpu"); got != cols {
				t.Errorf("%s: %d cores on the first row, want %d", size, got, cols)
			}
		}
	})
	t.Run("settings", func(t *testing.T) {
		d := readGoldenJSON(t, "monitor-settings", "80x24")
		if d.Focus == nil || *d.Focus != "settings-list" {
			t.Errorf("focus %v", d.Focus)
		}
		var items []string
		for _, l := range nodeLines(t, "monitor-settings", "80x24") {
			if f := strings.Fields(l); len(f) > 1 && f[1] == "item" {
				items = append(items, l)
			}
		}
		if len(items) != 6 || !strings.HasSuffix(items[1], "selected") || strings.HasSuffix(items[0], "selected") {
			t.Errorf("the cursor is not on the second setting: %q", items)
		}
	})
	t.Run("play select", func(t *testing.T) {
		want := []string{
			`1 cursor_moved procs {"p":0} null`,
			`2 cursor_moved procs {"p":8810} null`,
			`3 marks_changed procs {"p":8810} [8810]`,
			`5 marks_changed procs {"p":8810} []`,
			`6 marks_changed procs {"p":8810} [8810]`,
		}
		evs := eventsOf(t, "monitor-play-select", "80x24")
		if len(evs) != 6 || evs[0] != want[0] || evs[1] != want[1] || evs[2] != want[2] || evs[4] != want[3] || evs[5] != want[4] || !strings.HasPrefix(evs[3], "4 marks_changed procs") {
			t.Errorf("events %q", evs)
		}
		if !strings.Contains(gridOf(t, "monitor-play-select", "80x24")[6], "│▸    8810") {
			t.Error("no ▸ in the mark channel of the checked row")
		}
	})
	t.Run("play tabs", func(t *testing.T) {
		evs := eventsOf(t, "monitor-play-tabs", "120x24")
		want := `1 view_changed nav {} "settings"|2 view_changed nav {} "trends"|3 view_changed nav {} "settings"|4 view_changed nav {} "trends"|5 view_changed nav {} "overview"|6 view_changed nav {} "processes"`
		if strings.Join(evs, "|") != want {
			t.Errorf("events %q", evs)
		}
		if d := readGoldenJSON(t, "monitor-play-tabs", "120x24"); d.Focus == nil || *d.Focus != "procs" {
			t.Errorf("focus %v", d.Focus)
		}
	})
	t.Run("play hints and filter", func(t *testing.T) {
		g := gridOf(t, "monitor-play-hints", "120x24")
		if strings.TrimSpace(g[len(g)-1]) != "esc cancel" {
			t.Errorf("hints bar %q", g[len(g)-1])
		}
		evs := eventsOf(t, "monitor-play-filter", "120x24")
		if strings.Join(evs, "|") != `1 filter_open procs {"p":412} null|4 filter_apply filter {} ""|5 filter filter {} "ch"` {
			t.Errorf("filter events %q", evs)
		}
		if !strings.Contains(gridOf(t, "monitor-play-filter", "120x24")[1], "▸7 processes") {
			t.Error("tab in the filter switched tabs")
		}
	})
	t.Run("play kill", func(t *testing.T) {
		g := gridOf(t, "monitor-play-kill", "80x24")
		if strings.TrimSpace(g[len(g)-1]) != "y confirm  n cancel" {
			t.Errorf("kill bar %q", g[len(g)-1])
		}
		if !strings.Contains(strings.Join(g, "\n"), "[BLOCKED ]       1 launchd protected") {
			t.Error("the blocked row is not laid out by CSS")
		}
		fg := gridOf(t, "monitor-play-kill-force", "80x24")
		if !strings.Contains(strings.Join(fg, "\n"), "SIGKILL cannot be caught") {
			t.Error("no SIGKILL warning")
		}
		if evs := eventsOf(t, "monitor-play-kill-force", "80x24"); len(evs) != 1 || !strings.HasPrefix(evs[0], "1 kill_force_ask procs") {
			t.Errorf("force events %q", evs)
		}
	})
	t.Run("play detail", func(t *testing.T) {
		for size, two := range map[string]bool{"77x24": false, "78x24": true} {
			g := gridOf(t, "monitor-play-detail", size)
			if strings.Contains(g[3], "name") != two {
				t.Errorf("%s: two detail columns %v: %q", size, !two, g[3])
			}
			if strings.TrimSpace(g[len(g)-1]) != "enter close  r refresh" {
				t.Errorf("%s bar %q", size, g[len(g)-1])
			}
			if evs := eventsOf(t, "monitor-play-detail", size); len(evs) != 1 || !strings.HasPrefix(evs[0], "1 diagnose procs") {
				t.Errorf("%s events %q", size, evs)
			}
		}
	})
	t.Run("play mouse off and wheel", func(t *testing.T) {
		if evs := eventsOf(t, "monitor-play-mouse-off", "120x24"); len(evs) != 1 || evs[0] != "none" {
			t.Errorf("mouse off events %q", evs)
		}
		if !strings.Contains(gridOf(t, "monitor-play-mouse-off", "120x24")[1], "▸7 processes") {
			t.Error("the click with the mouse off switched tabs")
		}
		evs := eventsOf(t, "monitor-play-wheel", "80x24")
		if strings.Join(evs, "|") != `1 cursor_moved procs {"p":0} null|2 cursor_moved procs {"p":8810} null` {
			t.Errorf("wheel events %q", evs)
		}
		if d := readGoldenJSON(t, "monitor-play-wheel", "80x24"); d.Focus == nil || *d.Focus != "procs" {
			t.Errorf("focus %v", d.Focus)
		}
	})
	t.Run("play empty", func(t *testing.T) {
		// An empty procs (a filter that matches nothing): the auto-fit
		// #procs keeps one body row, and "no results" is painted on it
		// (SPEC §6.9.3, §6.9.4).
		g := gridOf(t, "monitor-play-empty", "100x30")
		if !strings.HasPrefix(g[3], "│      PID NAME") || strings.TrimSpace(strings.Trim(g[4], "│")) != "no results" || !strings.HasPrefix(g[5], "╰") {
			t.Errorf("no placeholder row: %q", g[2:6])
		}
		var procs string
		for _, l := range nodeLines(t, "monitor-play-empty", "100x30") {
			if strings.HasPrefix(l, "procs ") {
				procs = strings.Join(strings.Fields(l)[:3], " ")
			}
		}
		if procs != "procs table 100x4" {
			t.Errorf("procs node %q, want procs table 100x4", procs)
		}
	})
}

// §24: validate reports no error on examples/monitor under both themes
// (Validate renders a theme="auto" document under dark and light), and
// the host's --dump is the same frame as the document's own dump.
func TestMonitorValidates(t *testing.T) {
	app, err := tuimark.Load("examples/monitor/studio.tui")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("examples/monitor/sample.json")
	if err != nil {
		t.Fatal(err)
	}
	var data any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	if err := app.Bind("", data); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"auto", "dark", "light"} {
		if err := app.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		for _, d := range app.Validate() {
			if d.Severity == "error" {
				t.Errorf("%s: %s", theme, d)
			}
		}
	}
}
