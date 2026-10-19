package host

import (
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/abdul-hamid-achik/tuimark/internal/paint"
)

// SPEC v0.4b §26.13 tests 194-196 and decisions 75, 85, and 105, on the
// fake clock of runclock_test.go.

// cellWrite is a run of text the frame diff wrote at a cell (0-based).
type cellWrite struct {
	row, col int
	text     string
}

var csiRE = regexp.MustCompile("\x1b\\[[0-9;?]*[A-Za-z]")

// writes parses what the frame diff wrote into runs of text with their
// positions. A unit of test 194 is a cursor move (CSI row ; col H), an
// optional SGR, and one glyph; the diff's fixed suffix (ESC [ 0 m, ESC [
// ? 25 l, or a cursor move and ESC [ ? 25 h) holds no text and so no run.
func writes(s string) []cellWrite {
	var out []cellWrite
	row, col, pos := 0, 0, 0
	text := func(end int) {
		if end > pos {
			t := s[pos:end]
			out = append(out, cellWrite{row, col, t})
			col += utf8.RuneCountInString(t)
		}
	}
	for _, m := range csiRE.FindAllStringIndex(s, -1) {
		text(m[0])
		seq := s[m[0]:m[1]]
		if strings.HasSuffix(seq, "H") {
			parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(seq, "\x1b["), "H"), ";")
			if len(parts) == 2 {
				r, _ := strconv.Atoi(parts[0])
				c, _ := strconv.Atoi(parts[1])
				row, col = r-1, c-1
			}
		}
		pos = m[1]
	}
	text(len(s))
	return out
}

// spinDoc is a current-language document with S1, the spinner of test
// 194 (or s1 when given), and body after it.
func spinDoc(s1, body string) string {
	if s1 == "" {
		s1 = `<progress id="s1" indeterminate="true"/>`
	}
	return `<tui><screen id="main">` + s1 + body + `</screen></tui>`
}

// advance100 advances the clock n times by 100 ms and returns what each
// advance made the loop write, as runs of text, with how many timers each
// fired.
func (r *clockRig) advance100(n int) (runs [][]cellWrite, fired []int) {
	r.t.Helper()
	for i := 0; i < n; i++ {
		fired = append(fired, r.clk.Advance(r.t, 100*time.Millisecond))
		runs = append(runs, writes(r.since()))
	}
	return runs, fired
}

// wantGlyphs checks that each advance gave exactly one redraw at (row,
// col) holding the next glyph, and nothing else.
func (r *clockRig) wantGlyphs(row, col int, glyphs []string) {
	r.t.Helper()
	runs, fired := r.advance100(len(glyphs))
	for i, g := range glyphs {
		want := []cellWrite{{row, col, g}}
		if fired[i] != 1 || !reflect.DeepEqual(runs[i], want) {
			r.t.Errorf("advance %d: %d timers fired, wrote %v; want 1 and %v", i+1, fired[i], runs[i], want)
		}
	}
}

var braille = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// next10 are the glyphs of the ten phases after phase ph.
func next10(set []string, ph, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = set[(ph+1+i)%len(set)]
	}
	return out
}

// 194. When Run() redraws (SPEC v0.4b §26.13): a visible spinner turns
// once per 100 ms of the session's clock, one redraw at its cell per
// advance, with the glyphs in order and nothing else written.
func TestRunClock194VisibleTurns(t *testing.T) {
	r := startClockRig(t, spinDoc("", ""), rigOpts{})
	if got := r.since(); !strings.Contains(got, "⠋") {
		t.Fatalf("the first frame = %q, want phase 0", got)
	}
	if n := r.clk.active(); n != 1 {
		t.Errorf("%d timers armed after the first frame, want the spinner's", n)
	}
	r.wantGlyphs(0, 0, next10(braille, 0, 10)) // ⠙⠹⠸⠼⠴⠦⠧⠇⠏⠋
	r.wantGlyphs(0, 0, next10(braille, 0, 10)) // a second turn of the cycle
	if err := r.quit(); err != nil {
		t.Fatal(err)
	}
}

// 194. A spinner nobody can see arms no timer: advancing the clock by 1 s
// in steps of 100 ms fires nothing and writes nothing.
func TestRunClock194NotVisible(t *testing.T) {
	scroll := `<scroll id="sc" focusable="true" style="height: 3">` + strings.Repeat(`<text>t</text>`, 9) + `<progress id="s1" indeterminate="true"/></scroll>`
	cases := []struct{ name, src string }{
		{"visibility hidden", spinDoc(`<progress id="s1" indeterminate="true" style="visibility: hidden"/>`, "")},
		{"inactive tab", spinDoc(`<tabs id="t"><tab id="a" label="a"><text>a</text></tab><tab id="b" label="b"><progress id="s1" indeterminate="true"/></tab></tabs>`, "")},
		{"scrolled out", spinDoc(scroll, "")},
		{"under a modal", spinDoc("", `<modal id="m" open="true" style="width: 100%; height: 100%"><text>m</text></modal>`)},
		{"zero width", spinDoc(`<progress id="s1" indeterminate="true" style="width: 0"/>`, "")},
		{"indeterminate false", spinDoc(`<progress id="s1" indeterminate="false"/>`, "")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := startClockRig(t, c.src, rigOpts{})
			r.since()
			if n := r.clk.active(); n != 0 {
				t.Errorf("%d timers armed after the first frame, want none", n)
			}
			runs, fired := r.advance100(10)
			for i := range runs {
				if fired[i] != 0 || len(runs[i]) != 0 {
					t.Errorf("advance %d: %d timers fired, wrote %v; want nothing", i+1, fired[i], runs[i])
				}
			}
			if err := r.quit(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// 194. Brought into view: a key scrolls the spinner into the clip and the
// next ten advances give ten redraws; another takes it out again and the
// advances give none.
func TestRunClock194BroughtIntoView(t *testing.T) {
	scroll := `<scroll id="sc" focusable="true" style="height: 3">` + strings.Repeat(`<text>t</text>`, 9) + `<progress id="s1" indeterminate="true"/></scroll>`
	src := `<tui><screen id="main" focus="#sc">` + scroll + `</screen></tui>`
	r := startClockRig(t, src, rigOpts{})
	r.since()
	runs, fired := r.advance100(3)
	for i := range runs {
		if fired[i] != 0 || len(runs[i]) != 0 {
			t.Fatalf("a scrolled-out spinner turned: advance %d fired %d, wrote %v", i+1, fired[i], runs[i])
		}
	}
	r.key("\x1b[F") // end: offset 7, the spinner is the last row of the clip
	var cell cellWrite
	for _, w := range writes(r.since()) {
		if strings.ContainsAny(w.text, strings.Join(braille, "")) {
			cell = w
		}
	}
	if cell.text == "" {
		t.Fatal("the key did not bring the spinner into view")
	}
	if cell.row != 2 || cell.col != 0 {
		t.Errorf("the spinner is at (%d,%d), want (2,0)", cell.row, cell.col)
	}
	ph := PhaseOf(r.clk.elapsed())
	r.wantGlyphs(cell.row, cell.col, next10(braille, ph, 10))
	r.key("\x1b[H") // home: out of the clip again
	r.since()
	runs, fired = r.advance100(10)
	for i := range runs {
		if fired[i] != 0 || len(runs[i]) != 0 {
			t.Errorf("after home, advance %d: %d timers fired, wrote %v; want none", i+1, fired[i], runs[i])
		}
	}
	if err := r.quit(); err != nil {
		t.Fatal(err)
	}
}

// 194. The glyph set follows the color profile: ASCII under 16 (the
// first frame included), braille under 256, none, and truecolor.
func TestRunClock194GlyphSets(t *testing.T) {
	ascii := []string{"|", "/", "-", "\\"}
	for _, c := range []struct {
		profile paint.Profile
		set     []string
	}{
		{paint.ANSI16, ascii}, {paint.ANSI256, braille}, {paint.NoColor, braille}, {paint.TrueColor, braille},
	} {
		t.Run(c.profile.String(), func(t *testing.T) {
			r := startClockRig(t, spinDoc("", ""), rigOpts{profile: c.profile})
			first := writes(r.since())
			if len(first) == 0 || !strings.Contains(first[0].text, c.set[0]) {
				t.Fatalf("the first frame wrote %v, want it to hold %q", first, c.set[0])
			}
			r.wantGlyphs(0, 0, next10(c.set, 0, 6))
			if err := r.quit(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// 194. The log: the spinner's redraws write no record, a frame drawn for
// any other reason is recorded as always, and nothing in the log names a
// phase. The log's t counts on the session's clock.
func TestRunClock194Log(t *testing.T) {
	r := startClockRig(t, spinDoc("", ""), rigOpts{log: true})
	r.advance100(10)
	r.key("x") // unbound: the key's settle draws one frame
	if err := r.quit(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(r.log)
	if err != nil {
		t.Fatal(err)
	}
	recs := readLog(t, r.log)
	var evs []string
	for _, rec := range recs {
		evs = append(evs, rec["ev"].(string))
	}
	want := []string{"start", "caps", "frame", "key", "frame", "end"}
	if !reflect.DeepEqual(evs, want) {
		t.Fatalf("records = %v, want %v", evs, want)
	}
	if k := recs[3]["t"]; k != float64(1000) {
		t.Errorf("the key's t = %v, want 1000 (ten advances of 100 ms)", k)
	}
	for _, bad := range []string{"phase", "spin", "⠋", "⠙", "⠏"} {
		if strings.Contains(string(raw), bad) {
			t.Errorf("the log mentions %q:\n%s", bad, raw)
		}
	}
}

// 195. A redraw is paint only: the spinner neither fires an Every
// function nor delays it, and dispatches no event.
func TestRunClock195PaintOnlyAndEvery(t *testing.T) {
	src := spinDoc("", `<input id="q" on:change="ping"/><text>n={n}</text>`)
	var mu sync.Mutex
	pings := 0
	r := startClockRig(t, src, rigOpts{setup: func(a *App) {
		_ = a.Bind("", map[string]any{"n": 0})
		a.On("ping", func(Event) error { mu.Lock(); pings++; mu.Unlock(); return nil })
		a.Every(300*time.Millisecond, incr(a))
	}})
	_, fired := r.advance100(12)
	// The spinner's timer fires at each advance; the Every timer at every
	// third one.
	for i, f := range fired {
		want := 1
		if (i+1)%3 == 0 {
			want = 2
		}
		if f != want {
			t.Errorf("advance %d: %d timers fired, want %d", i+1, f, want)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if pings != 0 {
		t.Errorf("ping was called %d times by redraws", pings)
	}
	if v, _ := r.app.Get("n"); v != float64(4) {
		t.Errorf("n = %v after 1.2 s of 300 ms ticks, want 4", v)
	}
	if g := screenOf(r.app); !strings.Contains(g, "n=4") {
		t.Errorf("the screen = %q, want n=4", g)
	}
}

// 195. The events a settle kept at its 8-round cap (the fixture of test
// 115) are not dispatched by a spinner's redraw; the next key's settle
// dispatches them first.
func TestRunClock195CarryUntouched(t *testing.T) {
	src := `<tui>
<style>button:focus { display: none; }</style>
<keymap><bind keys="k" action="kick"/></keymap>
<screen id="s">
  <progress id="s1" indeterminate="true"/>
  <button id="a" on:focus="fa">A</button>
  <button id="b" on:focus="fb">B</button>
</screen>
</tui>`
	var mu sync.Mutex
	var seen []string
	rec := func(name string) func(Event) error {
		return func(Event) error { mu.Lock(); seen = append(seen, name); mu.Unlock(); return nil }
	}
	r := startClockRig(t, src, rigOpts{setup: func(a *App) {
		a.On("fa", rec("fa"))
		a.On("fb", rec("fb"))
		a.On("kick", rec("kick"))
	}})
	mu.Lock()
	before := append([]string(nil), seen...)
	mu.Unlock()
	kept := r.app.carryLen()
	if kept == 0 {
		t.Fatal("the fixture did not reach the 8-round cap")
	}
	runs, _ := r.advance100(10)
	if len(runs[0]) == 0 {
		t.Error("the spinner did not turn")
	}
	mu.Lock()
	after := append([]string(nil), seen...)
	mu.Unlock()
	if !reflect.DeepEqual(before, after) || r.app.carryLen() != kept {
		t.Errorf("redraws dispatched %v (kept %d -> %d)", after[len(before):], kept, r.app.carryLen())
	}
	r.key("k")
	mu.Lock()
	rest := append([]string(nil), seen[len(before):]...)
	mu.Unlock()
	if len(rest) <= kept || rest[kept] != "kick" {
		t.Errorf("the key dispatched %v, want %d kept events and then kick", rest, kept)
	}
	if err := r.quit(); err != nil {
		t.Fatal(err)
	}
}

// carryLen is the number of events a settle kept at its cap.
func (a *App) carryLen() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.carry)
}

// 195. A Run started on an app that Play ticked draws phase 0 in its
// first frame: Run reads neither the virtual clock nor changes it.
func TestRunClock195RunIgnoresVirtualClock(t *testing.T) {
	a := doc(t, spinDoc("", ""))
	if err := a.EnterPlay(); err != nil {
		t.Fatal(err)
	}
	a.VirtualTick(300*time.Millisecond, true, func() bool { return false })
	a.ExitPlay()
	if a.VClock() != 300*time.Millisecond {
		t.Fatalf("VClock = %v", a.VClock())
	}
	r := startClockRigApp(t, a, rigOpts{})
	if first := writes(r.since()); len(first) == 0 || !strings.Contains(first[0].text, "⠋") {
		t.Errorf("the first frame wrote %v, want ⠋", first)
	}
	r.advance100(2)
	if err := r.quit(); err != nil {
		t.Fatal(err)
	}
	if a.VClock() != 300*time.Millisecond {
		t.Errorf("Run moved the virtual clock to %v", a.VClock())
	}
}

// 196. Dump never turns: called after every advance while Run is active
// with a visible spinner, it always paints phase 0, whatever the session's
// clock says, and the live frame is not touched by it.
func TestRunClock196DumpStaysAtPhase0(t *testing.T) {
	r := startClockRig(t, spinDoc("", ""), rigOpts{})
	// Another goroutine Dumps all the while (the race detector's food).
	stop := make(chan struct{})
	bad := make(chan string, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if got := r.app.Dump(20, 5, false).Grid[0]; !strings.HasPrefix(got, "⠋") {
				select {
				case bad <- got:
				default:
				}
				return
			}
		}
	}()
	for i := 0; i < 10; i++ {
		r.clk.Advance(t, 100*time.Millisecond)
		d := r.app.Dump(20, 5, false)
		if got := d.Grid[0]; !strings.HasPrefix(got, "⠋") {
			t.Fatalf("Dump after advance %d = %q, want phase 0", i+1, got)
		}
	}
	close(stop)
	wg.Wait()
	select {
	case got := <-bad:
		t.Fatalf("a concurrent Dump painted %q, want phase 0", got)
	default:
	}
	// The next redraw is still the next phase of the session's clock.
	r.since()
	runs, _ := r.advance100(1)
	if len(runs[0]) != 1 || runs[0][0].text != braille[1] {
		t.Errorf("the redraw after the Dumps wrote %v, want %s", runs[0], braille[1])
	}
	if err := r.quit(); err != nil {
		t.Fatal(err)
	}
}

// Every reads the same clock: its deadlines, counted from the session's
// start on the fake clock, give exact counts with no wall time.
func TestRunClockEveryDeadlines(t *testing.T) {
	r := startClockRig(t, v1(`<text>n={n}</text>`), rigOpts{setup: func(a *App) {
		_ = a.Bind("", map[string]any{"n": 0})
		a.Every(250*time.Millisecond, incr(a))
	}})
	for _, want := range []float64{0, 0, 0, 1} { // 83 ms three times, then past 250 ms
		r.clk.Advance(t, 83*time.Millisecond)
		if v, _ := r.app.Get("n"); v != want {
			t.Fatalf("n = %v, want %v", v, want)
		}
	}
	// Ticks are never queued: a jump over several periods is one call.
	for _, want := range []float64{2, 3, 4} {
		r.clk.Advance(t, 250*time.Millisecond)
		if v, _ := r.app.Get("n"); v != want {
			t.Fatalf("n = %v, want %v", v, want)
		}
	}
	r.clk.Advance(t, 750*time.Millisecond)
	if v, _ := r.app.Get("n"); v != float64(5) {
		t.Errorf("n = %v after a 750 ms jump, want 5 (the missed ticks are dropped)", v)
	}
	if err := r.quit(); err != nil {
		t.Fatal(err)
	}
}

// A registration made during the session counts its first tick from the
// session's clock.
func TestRunClockEveryRegisteredDuringSession(t *testing.T) {
	r := startClockRig(t, v1(`<text>n={n}</text>`), rigOpts{setup: func(a *App) {
		_ = a.Bind("", map[string]any{"n": 0})
	}})
	r.clk.Advance(t, 40*time.Millisecond)
	seq := r.clk.quietSeq()
	r.app.Every(100*time.Millisecond, incr(r.app))
	r.clk.waitQuiet(t, seq) // kickEvery: the loop arms its timer again
	if n := r.clk.Advance(t, 99*time.Millisecond); n != 0 {
		t.Errorf("the tick fired %d timers 99 ms after registration", n)
	}
	if n := r.clk.Advance(t, time.Millisecond); n != 1 {
		t.Errorf("the tick fired %d timers at 100 ms after registration, want 1", n)
	}
	if v, _ := r.app.Get("n"); v != float64(1) {
		t.Errorf("n = %v, want 1", v)
	}
	if err := r.quit(); err != nil {
		t.Fatal(err)
	}
}

// Decision 85: a reload of tuimark run does not restart the clock. The
// session hands its clock to the new app, whose first frame is drawn at
// the phase the session had reached, and whose Every registrations count
// from the swap.
func TestRunClock85ReloadKeepsClock(t *testing.T) {
	swaps := make(chan HostSwap, 1)
	r := startClockRig(t, spinDoc("", ""), rigOpts{hooks: HostHooks{Swap: swaps}})
	r.advance100(3) // phase 3
	next := doc(t, spinDoc("", `<text>n={n}</text>`))
	_ = next.Bind("", map[string]any{"n": 0})
	next.Every(300*time.Millisecond, incr(next))
	applied := make(chan struct{})
	seq := r.clk.quietSeq()
	swaps <- HostSwap{App: next, Applied: func() { close(applied) }}
	<-applied
	r.clk.waitQuiet(t, seq)
	r.since()
	if g := screenOf(next); !strings.HasPrefix(g, braille[3]) {
		t.Errorf("the new app's first frame = %q, want phase 3 (%s)", g, braille[3])
	}
	// The spinner keeps turning from there.
	runs, _ := r.advance100(1)
	if len(runs[0]) != 1 || runs[0][0].text != braille[4] {
		t.Errorf("after the swap the next redraw wrote %v, want %s", runs[0], braille[4])
	}
	// The Every counts its 300 ms from the swap, on the same clock.
	r.advance100(1)
	if v, _ := next.Get("n"); v != float64(0) {
		t.Errorf("n = %v 200 ms after the swap, want 0", v)
	}
	r.advance100(1)
	if v, _ := next.Get("n"); v != float64(1) {
		t.Errorf("n = %v 300 ms after the swap, want 1", v)
	}
	next.Quit()
	select {
	case err := <-r.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the loop did not return")
	}
}

// Decision 105 / MUST 13: a document without a spinner keeps the frame
// skip as the session's clock moves (the phase is an input of the frame
// only when there is a spinner), and one with a spinner does not.
func TestRunClockPhaseOnlyForSpinnerDocuments(t *testing.T) {
	plain := doc(t, v1(`<text>x</text>`))
	if plain.HasSpinner() {
		t.Error("a document without a spinner reports one")
	}
	if !doc(t, spinDoc("", "")).HasSpinner() {
		t.Error("a document with a spinner reports none")
	}
	if !doc(t, `<tui><screen id="a"><text>x</text><modal id="m" open="true"><list id="l" each="rows as r" key="r"><item><progress indeterminate="true"/></item></list></modal></screen></tui>`).HasSpinner() {
		t.Error("a spinner in a modal's list item is not found")
	}
}
