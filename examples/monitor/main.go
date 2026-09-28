// Command monitor runs examples/monitor/studio.tui, the Tuimark 0.2b
// acceptance fixture: three tabs of monitor studio (Overview, Processes
// with the kill confirmation and the process detail, Settings), plus CPU
// for the core grid. Since 0.3a it is a version="3" document, so it also
// shows that no tab cuts content at the sizes Validate checks. Since 0.3b
// it groups its keymap rows by context (<keymap when>), stacks the cores
// with no row gap, and hides Processes columns by priority, not @media.
//
//	go run ./examples/monitor                  # from the repo root
//	go run ./examples/monitor --dump 120x30    # print Dump() as JSON and exit
//	go run ./examples/monitor --theme light    # Set("@theme", "light") before Run
//
// The view lives entirely in studio.tui and studio.tcss. This host holds
// no layout, width, padding, truncation, or column code: it loads
// sample.json, runs a seeded fake collector, and implements the named
// actions with derived data only (booleans, labels, pre-formatted numbers,
// sort arrows), through the public API (Load, Bind, Set, On, Dump, Run).
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

func main() {
	dir := flag.String("dir", "examples/monitor", "directory with studio.tui, studio.tcss, sample.json")
	dumpSize := flag.String("dump", "", "print Dump() as JSON at COLSxROWS and exit")
	theme := flag.String("theme", "", "dark or light: Set(\"@theme\", …) before Run")
	flag.Parse()
	if err := run(*dir, *dumpSize, *theme); err != nil {
		fmt.Fprintln(os.Stderr, "monitor:", err)
		os.Exit(1)
	}
}

// proc is one process of the fake snapshot. cpu0 is its base load, which
// the collector scales by one factor per sample, so the CPU order holds.
type proc struct {
	PID       float64 `json:"pid"`
	Name      string  `json:"name"`
	CPU       string  `json:"cpu"`
	Mem       string  `json:"mem"`
	IO        string  `json:"io"`
	Threads   float64 `json:"threads"`
	User      string  `json:"user"`
	Protected bool    `json:"protected"`
	CPUHot    bool    `json:"cpu_hot"`

	cpu0   float64
	memMiB float64
}

// setting is one row of the Settings tab.
type setting struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value string `json:"value"`
	Dirty bool   `json:"dirty"`
	Error bool   `json:"error"`
}

// options are the values each setting cycles through.
var options = map[string][]string{
	"theme":   {"auto", "dark", "light"},
	"refresh": {"1s", "2s", "5s"},
	"mouse":   {"on", "off"},
	"units":   {"binary", "decimal"},
	"sort":    {"cpu", "mem"},
	"confirm": {"on", "off"},
}

// monitor is the host's state. ui is safe from any goroutine; mu guards
// the rest.
type monitor struct {
	ui *tuimark.App
	mu sync.Mutex

	procs    []*proc // every live process, in sample order
	sortKey  string  // cpu or mem
	sortAsc  bool
	lastSort string // the sort key pressed last: pressing it again reverses
	query    string
	saved    string // the query when the filter opened, for filter_cancel
	cursor   float64
	marked   []any
	paused   bool
	tick     int
	interval time.Duration
	rng      *rand.Rand

	cpuHist, memHist []any
	cores            []map[string]any
	coreBase         []float64

	killForce bool
	detailPID float64 // the pid the detail is pinned to; -1 for none
	settings  []*setting
}

func run(dir, dumpSize, theme string) error {
	m, err := load(dir)
	if err != nil {
		return err
	}
	ui := m.ui
	m.register()
	if theme != "" {
		if theme != "dark" && theme != "light" {
			return fmt.Errorf("--theme wants dark or light, got %q", theme)
		}
		if err := ui.Set("@theme", theme); err != nil {
			return err
		}
	}
	if dumpSize != "" {
		var cols, rows int
		if _, err := fmt.Sscanf(dumpSize, "%dx%d", &cols, &rows); err != nil {
			return fmt.Errorf("--dump wants COLSxROWS, got %q", dumpSize)
		}
		d, err := ui.Dump(cols, rows)
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(d, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	stop := make(chan struct{})
	defer close(stop)
	go m.collect(stop)
	return ui.Run(os.Stdout)
}

// load loads studio.tui from dir, binds sample.json, and builds the host
// state from the same sample.
func load(dir string) (*monitor, error) {
	ui, err := tuimark.Load(filepath.Join(dir, "studio.tui"))
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "sample.json"))
	if err != nil {
		return nil, err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	if err := ui.Bind("", data); err != nil {
		return nil, err
	}
	return newMonitor(ui, raw)
}

// newMonitor reads the host's own copy of the sample.
func newMonitor(ui *tuimark.App, raw []byte) (*monitor, error) {
	var s struct {
		Procs    []*proc    `json:"procs"`
		Settings []*setting `json:"settings"`
		Cursor   float64    `json:"cursor_pid"`
		CPU      struct {
			Hist []any `json:"hist"`
		} `json:"cpu"`
		Mem struct {
			Hist []any `json:"hist"`
		} `json:"mem"`
		Cores []map[string]any `json:"cores"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	m := &monitor{
		ui: ui, procs: s.Procs, settings: s.Settings, cursor: s.Cursor, marked: []any{},
		sortKey: "cpu", interval: time.Second, rng: rand.New(rand.NewSource(7)),
		cpuHist: s.CPU.Hist, memHist: s.Mem.Hist, cores: s.Cores, detailPID: -1,
	}
	for _, p := range m.procs {
		fmt.Sscanf(p.CPU, "%g", &p.cpu0)
		p.memMiB = parseMem(p.Mem)
	}
	for _, c := range m.cores {
		pct, _ := c["pct"].(float64)
		m.coreBase = append(m.coreBase, pct)
	}
	return m, nil
}

// parseMem reads a pre-formatted size such as 612M or 1.4G, in MiB.
func parseMem(s string) float64 {
	var v float64
	var unit string
	fmt.Sscanf(s, "%g%s", &v, &unit)
	if unit == "G" {
		return v * 1024
	}
	return v
}

// register registers every handler with the app.
func (m *monitor) register() {
	for name, h := range m.handlers() {
		m.ui.On(name, h)
	}
}

// handlers are the named actions of studio.tui.
func (m *monitor) handlers() map[string]tuimark.Handler {
	ui := m.ui
	hs := map[string]tuimark.Handler{}
	on := func(name string, h tuimark.Handler) { hs[name] = h }
	on("quit", func(tuimark.Event) error { return tuimark.ErrQuit })
	on("pause", func(tuimark.Event) error {
		m.mu.Lock()
		m.paused = !m.paused
		paused := m.paused
		m.mu.Unlock()
		label := "● LIVE"
		if paused {
			label = "‖ PAUSED"
		}
		return firstErr(ui.Set("paused", paused), ui.Set("live", !paused), ui.Set("status_label", label))
	})
	on("refresh", func(tuimark.Event) error {
		m.sample()
		return nil
	})
	on("view_changed", func(tuimark.Event) error { return nil })
	on("cursor_moved", func(ev tuimark.Event) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		if pid, ok := ev.Keys["p"].(float64); ok {
			m.cursor = pid
		}
		return nil
	})
	on("marks_changed", func(ev tuimark.Event) error {
		m.mu.Lock()
		defer m.mu.Unlock()
		if arr, ok := ev.Value.([]any); ok {
			m.marked = arr
		}
		return nil
	})
	on("sort_cpu", func(tuimark.Event) error { return m.sortBy("cpu") })
	on("sort_mem", func(tuimark.Event) error { return m.sortBy("mem") })
	on("filter_open", func(tuimark.Event) error {
		m.mu.Lock()
		m.saved = m.query
		m.mu.Unlock()
		return firstErr(ui.Set("filter_open", true), ui.Set("@focus", "#filter"))
	})
	on("filter", func(ev tuimark.Event) error {
		m.mu.Lock()
		m.query, _ = ev.Value.(string)
		m.mu.Unlock()
		return m.publishProcs()
	})
	on("filter_apply", func(tuimark.Event) error { return ui.Set("@focus", "#procs") })
	on("filter_cancel", func(tuimark.Event) error {
		m.mu.Lock()
		m.query = m.saved
		q := m.query
		m.mu.Unlock()
		return firstErr(ui.Set("query", q), m.publishProcs(), ui.Set("filter_open", false), ui.Set("@focus", "#procs"))
	})
	on("kill_ask", func(ev tuimark.Event) error { return m.killAsk(ev, false) })
	on("kill_force_ask", func(ev tuimark.Event) error { return m.killAsk(ev, true) })
	on("kill_confirm", func(tuimark.Event) error { return m.killConfirm() })
	on("kill_cancel", func(tuimark.Event) error { return ui.Set("kill_open", false) })
	on("diagnose", func(ev tuimark.Event) error {
		m.mu.Lock()
		m.track(ev)
		m.detailPID = m.cursor
		m.mu.Unlock()
		return firstErr(m.publishDetail(), ui.Set("detail_open", true))
	})
	on("detail_refresh", func(tuimark.Event) error { return m.publishDetail() })
	on("detail_close", func(tuimark.Event) error { return ui.Set("detail_open", false) })
	on("setting_next", func(ev tuimark.Event) error { return m.cycleSetting(ev, 1) })
	on("setting_prev", func(ev tuimark.Event) error { return m.cycleSetting(ev, -1) })
	on("settings_save", func(tuimark.Event) error { return m.saveSettings() })
	on("help_open", func(tuimark.Event) error { return ui.Set("help_open", true) })
	on("help_close", func(tuimark.Event) error { return ui.Set("help_open", false) })
	return hs
}

func firstErr(errs ...error) error { return errors.Join(errs...) }

// sortBy re-sorts the processes by key; pressing the same sort key again
// reverses the order. The arrow in the header is data (h).
func (m *monitor) sortBy(key string) error {
	m.mu.Lock()
	if m.lastSort == key {
		m.sortAsc = !m.sortAsc
	} else {
		m.sortKey, m.sortAsc = key, false
	}
	m.lastSort = key
	arrow := "▼"
	if m.sortAsc {
		arrow = "▲"
	}
	h := map[string]any{"name": "NAME", "cpu": "CPU%", "mem": "MEM"}
	h[m.sortKey] = h[m.sortKey].(string) + arrow
	m.mu.Unlock()
	return firstErr(m.ui.Set("h", h), m.publishProcs())
}

// view is the process list as the table shows it: filtered by the query
// (a case-insensitive substring of the name) and sorted. The caller holds
// m.mu.
func (m *monitor) view() []any {
	q := strings.ToLower(m.query)
	var ps []*proc
	for _, p := range m.procs {
		if q == "" || strings.Contains(strings.ToLower(p.Name), q) {
			ps = append(ps, p)
		}
	}
	less := func(a, b *proc) bool {
		if m.sortKey == "mem" {
			return a.memMiB > b.memMiB
		}
		return a.cpu0 > b.cpu0
	}
	sort.SliceStable(ps, func(i, j int) bool {
		if m.sortAsc {
			return less(ps[j], ps[i])
		}
		return less(ps[i], ps[j])
	})
	out := make([]any, 0, len(ps))
	for _, p := range ps {
		out = append(out, p)
	}
	return out
}

func (m *monitor) publishProcs() error {
	m.mu.Lock()
	v := m.view()
	m.mu.Unlock()
	return m.ui.Set("procs", v)
}

// find returns the live process with pid, or nil. The caller holds m.mu.
func (m *monitor) find(pid float64) *proc {
	for _, p := range m.procs {
		if p.PID == pid {
			return p
		}
	}
	return nil
}

// track takes the cursor row from a keymap event of the focused table,
// whose keys carry the cursor row's key (SPEC §8.2): the table may have
// moved its cursor without an on:select (its row left the snapshot). The
// caller holds m.mu.
func (m *monitor) track(ev tuimark.Event) {
	if pid, ok := ev.Keys["p"].(float64); ok {
		m.cursor = pid
	}
}

// killAsk fills the confirmation from the marked processes, else the
// cursor row, and opens it; force asks for SIGKILL.
func (m *monitor) killAsk(ev tuimark.Event, force bool) error {
	m.mu.Lock()
	m.track(ev)
	pids := append([]any(nil), m.marked...)
	if len(pids) == 0 {
		pids = []any{m.cursor}
	}
	var rows []any
	for _, k := range pids {
		pid, _ := k.(float64)
		p := m.find(pid)
		if p == nil {
			continue
		}
		state, note := "[ELIGIBLE]", ""
		if p.Protected {
			state, note = "[BLOCKED ]", "protected"
		}
		rows = append(rows, map[string]any{"pid": p.PID, "state": state, "name": p.Name, "note": note, "blocked": p.Protected})
	}
	m.killForce = force
	m.mu.Unlock()
	if len(rows) == 0 {
		return nil
	}
	return firstErr(m.ui.Set("kill_rows", rows), m.ui.Set("kill_force", force), m.ui.Set("kill_open", true))
}

// killConfirm drops the eligible processes of the confirmation from the
// snapshot, unmarks them, and closes the confirmation.
func (m *monitor) killConfirm() error {
	m.mu.Lock()
	rows, _ := m.currentKillRows()
	gone := map[float64]bool{}
	for _, r := range rows {
		if blocked, _ := r["blocked"].(bool); !blocked {
			pid, _ := r["pid"].(float64)
			gone[pid] = true
		}
	}
	kept := m.procs[:0]
	for _, p := range m.procs {
		if !gone[p.PID] {
			kept = append(kept, p)
		}
	}
	m.procs = kept
	marked := []any{}
	for _, k := range m.marked {
		if pid, _ := k.(float64); !gone[pid] {
			marked = append(marked, k)
		}
	}
	m.marked = marked
	m.mu.Unlock()
	return firstErr(m.ui.Set("marked_pids", marked), m.publishProcs(), m.publishDetail(), m.ui.Set("kill_open", false))
}

// currentKillRows rebuilds the rows killAsk showed. The caller holds m.mu.
func (m *monitor) currentKillRows() ([]map[string]any, bool) {
	pids := m.marked
	if len(pids) == 0 {
		pids = []any{m.cursor}
	}
	var rows []map[string]any
	for _, k := range pids {
		pid, _ := k.(float64)
		if p := m.find(pid); p != nil {
			rows = append(rows, map[string]any{"pid": p.PID, "blocked": p.Protected})
		}
	}
	return rows, m.killForce
}

// publishDetail writes the detail of the pinned pid; present is false
// once the pid is gone from the snapshot.
func (m *monitor) publishDetail() error {
	m.mu.Lock()
	pid := m.detailPID
	if pid < 0 {
		m.mu.Unlock()
		return nil
	}
	p := m.find(pid)
	d := map[string]any{"title": fmt.Sprintf("process detail · pid %.0f", pid), "present": p != nil, "rows": []any{}}
	if p != nil {
		d["title"] = fmt.Sprintf("process detail · %s · pid %.0f", p.Name, pid)
		rows := [][2]string{
			{"pid", fmt.Sprintf("%.0f", p.PID)}, {"name", p.Name}, {"user", p.User}, {"cpu", p.CPU + "%"},
			{"memory", p.Mem}, {"threads", fmt.Sprintf("%.0f", p.Threads)}, {"i/o", p.IO}, {"protected", yesNo(p.Protected)},
		}
		var out []any
		for _, r := range rows {
			out = append(out, map[string]any{"k": r[0], "label": r[0], "value": r[1]})
		}
		d["rows"] = out
	}
	m.mu.Unlock()
	return m.ui.Set("detail", d)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// cycleSetting moves the cursor row's setting (the event's keys carry its
// key) to its next or previous value and marks it dirty. The mouse
// setting also switches the document's mouse; the theme setting, the
// host's theme.
func (m *monitor) cycleSetting(ev tuimark.Event, dir int) error {
	key, _ := ev.Keys["s"].(string)
	m.mu.Lock()
	var s *setting
	idx := -1
	for i, x := range m.settings {
		if x.Key == key {
			s, idx = x, i
		}
	}
	if s == nil {
		m.mu.Unlock()
		return nil
	}
	opts := options[key]
	cur := 0
	for i, o := range opts {
		if o == s.Value {
			cur = i
		}
	}
	s.Value = opts[(cur+dir+len(opts))%len(opts)]
	s.Dirty = true
	value, snap := s.Value, *s
	switch key {
	case "refresh":
		d, _ := time.ParseDuration(value)
		m.interval = d
	}
	m.mu.Unlock()
	errs := []error{m.ui.Set(fmt.Sprintf("settings.%d", idx), snap), m.ui.Set("settings_status", ""), m.ui.Set("settings_saved", false)}
	switch key {
	case "mouse":
		errs = append(errs, m.ui.Set("mouse_enabled", value == "on"))
	case "theme":
		errs = append(errs, m.ui.Set("@theme", value))
	}
	return firstErr(errs...)
}

// saveSettings clears the dirty marks and reports the result.
func (m *monitor) saveSettings() error {
	m.mu.Lock()
	n := 0
	var all []any
	for _, s := range m.settings {
		if s.Dirty {
			n++
		}
		s.Dirty = false
		all = append(all, *s)
	}
	m.mu.Unlock()
	status := fmt.Sprintf("saved: %d changed", n)
	return firstErr(m.ui.Set("settings", all), m.ui.Set("settings_status", status), m.ui.Set("settings_saved", true), m.ui.Set("settings_failed", false))
}

// collect is the fake collector: one sample per interval unless paused.
func (m *monitor) collect(stop <-chan struct{}) {
	for {
		m.mu.Lock()
		d := m.interval
		m.mu.Unlock()
		select {
		case <-stop:
			return
		case <-time.After(d):
		}
		m.mu.Lock()
		paused := m.paused
		m.mu.Unlock()
		if !paused {
			m.sample()
		}
	}
}

// sample takes one seeded fake sample and Sets cpu, mem, cores, top,
// procs, and sampled_at. Every process's load scales by one factor, so the
// CPU order holds; the short-lived "sleep" exits at the third sample, and a
// detail pinned to it then says it is no longer present.
func (m *monitor) sample() {
	m.mu.Lock()
	m.tick++
	f := 0.85 + 0.3*m.rng.Float64()
	kept := m.procs[:0]
	for _, p := range m.procs {
		if p.Name == "sleep" && m.tick >= 3 {
			continue
		}
		p.CPU = fmt.Sprintf("%.1f", p.cpu0*f)
		kept = append(kept, p)
	}
	m.procs = kept
	cpu := math.Round(math.Min(100, 23*f))
	mem := math.Round(math.Min(100, 61+4*(m.rng.Float64()-0.5)))
	m.cpuHist = append(append([]any(nil), m.cpuHist[1:]...), cpu)
	m.memHist = append(append([]any(nil), m.memHist[1:]...), mem)
	cores := make([]any, len(m.cores))
	for i, c := range m.cores {
		pct := math.Round(math.Max(0, math.Min(100, m.coreBase[i]*(0.7+0.6*m.rng.Float64()))))
		cores[i] = map[string]any{"id": c["id"], "label": fmt.Sprintf("cpu%d %.0f%%", i, pct), "pct": pct}
	}
	procs := m.view()
	var top []any
	byCPU := append([]*proc(nil), m.procs...)
	sort.SliceStable(byCPU, func(i, j int) bool { return byCPU[i].cpu0 > byCPU[j].cpu0 })
	for _, p := range byCPU[:min(5, len(byCPU))] {
		top = append(top, map[string]any{"pid": p.PID, "name": p.Name, "cpu": p.CPU + "%"})
	}
	cpuHist, memHist := m.cpuHist, m.memHist
	m.mu.Unlock()
	ui := m.ui
	_ = ui.Set("cpu", map[string]any{"label": fmt.Sprintf("cpu %.0f%%", cpu), "pct": cpu, "hist": cpuHist})
	_ = ui.Set("mem", map[string]any{"label": fmt.Sprintf("mem %.0f%%", mem), "pct": mem, "hist": memHist})
	_ = ui.Set("cores", cores)
	_ = ui.Set("top", top)
	_ = ui.Set("procs", procs)
	_ = m.publishDetail()
	_ = ui.Set("sampled_at", time.Now().Format("15:04:05"))
}
