package host

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// SPEC v0.2b §17.4 (examples/monitor) through the host: §21 tests 57 (the
// hints property over every settled frame of the monitor play goldens),
// 71 (the events of those goldens equal what Run's loop dispatches for the
// same bytes), and 72 (Set from goroutines while the loop renders).

const (
	monitorDoc  = "../../examples/monitor/studio.tui"
	monitorData = "../../examples/monitor/sample.json"
	goldenDir   = "../../testdata/golden"
)

// monitorEntry is a manifest entry of the monitor fixture with play input.
type monitorEntry struct {
	Name  string   `json:"name"`
	File  string   `json:"file"`
	Input string   `json:"input"`
	Sizes []string `json:"sizes"`
}

func monitorEntries(t *testing.T) []monitorEntry {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(goldenDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var all []monitorEntry
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Fatal(err)
	}
	var out []monitorEntry
	for _, e := range all {
		if e.File == "examples/monitor/studio.tui" && e.Input != "" {
			out = append(out, e)
		}
	}
	if len(out) < 10 {
		t.Fatalf("only %d monitor play entries in the manifest", len(out))
	}
	return out
}

func monitorApp(t testing.TB) *App {
	t.Helper()
	a, err := Load(monitorDoc)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(monitorData)
	if err != nil {
		t.Fatal(err)
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if err := a.Bind("", v); err != nil {
		t.Fatal(err)
	}
	return a
}

func parseSizeT(t *testing.T, s string) (int, int) {
	t.Helper()
	c, r, ok := strings.Cut(s, "x")
	cols, err1 := strconv.Atoi(c)
	rows, err2 := strconv.Atoi(r)
	if !ok || err1 != nil || err2 != nil {
		t.Fatalf("bad size %q", s)
	}
	return cols, rows
}

// replay applies play steps (SPEC §15.4) to a with no handlers, as `tuimark
// play` does, calling each after every settled frame (step 0 included)
// with the step number. It returns the events in order.
type replay struct {
	a          *App
	cols, rows int
	events     []Event
	steps      []int
}

func (r *replay) record(step int, evs []Event) {
	for round := 0; round < 8; round++ {
		for _, ev := range evs {
			r.events = append(r.events, ev)
			r.steps = append(r.steps, step)
		}
		evs = r.a.TakePending()
		if len(evs) == 0 {
			return
		}
		r.a.Frame(r.cols, r.rows)
	}
}

func (r *replay) dispatch(step int, evs []Event, focus string) {
	r.record(step, evs)
	if changed := r.a.TakeDirty(); len(evs) > 0 || r.a.Focus() != focus || changed {
		r.a.Frame(r.cols, r.rows)
	}
}

func (r *replay) settle(step int) {
	r.a.Frame(r.cols, r.rows)
	r.record(step, nil)
}

// inputs handles decoded input in order, as Run's loop does: consecutive
// keys go through HandleKeyRun together (printable keys for an input are
// one edit).
func (r *replay) inputs(step int, ins []Input) {
	for len(ins) > 0 {
		focus := r.a.Focus()
		switch in := ins[0]; {
		case in.IsPaste:
			r.dispatch(step, r.a.HandlePaste(in.Paste), focus)
			ins = ins[1:]
		case in.IsMouse:
			evs, _ := r.a.HandleMouse(in.Mouse)
			r.dispatch(step, evs, focus)
			ins = ins[1:]
		default:
			var keys []Key
			keys, ins = r.a.KeyRun(ins)
			for len(keys) > 0 {
				focus := r.a.Focus()
				evs, used := r.a.HandleKeyRun(keys)
				keys = keys[used:]
				r.dispatch(step, evs, focus)
			}
		}
	}
}

// run replays the steps of input, calling each(step) on every settled
// frame.
func (r *replay) run(t *testing.T, input string, each func(step int)) {
	t.Helper()
	r.settle(0)
	if each != nil {
		each(0)
	}
	for i, tok := range strings.Fields(input) {
		step := i + 1
		switch {
		case strings.HasPrefix(tok, "text:"):
			r.inputs(step, DecodeInput([]byte(tok[5:])))
		case strings.HasPrefix(tok, "set:"):
			path, js, _ := strings.Cut(tok[4:], "=")
			var v any
			if err := json.Unmarshal([]byte(js), &v); err != nil {
				t.Fatal(err)
			}
			if err := r.a.Set(path, v); err != nil {
				t.Fatal(err)
			}
		case strings.HasPrefix(tok, "focus:"):
			_ = r.a.Set("@focus", tok[6:])
		case strings.HasPrefix(tok, "click:"), strings.HasPrefix(tok, "wheel-"):
			name, cell, _ := strings.Cut(tok, ":")
			xs, ys, _ := strings.Cut(cell, ",")
			x, _ := strconv.Atoi(xs)
			y, _ := strconv.Atoi(ys)
			switch name {
			case "click":
				r.inputs(step, []Input{{IsMouse: true, Mouse: Mouse{MousePress, x, y}}, {IsMouse: true, Mouse: Mouse{MouseRelease, x, y}}})
			case "wheel-up":
				r.inputs(step, []Input{{IsMouse: true, Mouse: Mouse{MouseWheelUp, x, y}}})
			default:
				r.inputs(step, []Input{{IsMouse: true, Mouse: Mouse{MouseWheelDown, x, y}}})
			}
		default:
			r.inputs(step, []Input{{Key: keyOf(tok)}})
		}
		r.settle(step)
		if each != nil {
			each(step)
		}
	}
}

// eventLine formats an event as play's text output does (SPEC §15.4).
func eventLine(step int, ev Event) string {
	src := ev.Source
	if src == "" {
		src = "-"
	}
	keys := ev.Keys
	if keys == nil {
		keys = map[string]any{}
	}
	k, _ := json.Marshal(keys)
	v, _ := json.Marshal(ev.Value)
	return fmt.Sprintf("%d %s %s %s %s", step, ev.Action, src, k, v)
}

// goldenEvents reads the events section of a play text golden.
func goldenEvents(t *testing.T, name, size string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(goldenDir, name, size+".txt"))
	if err != nil {
		t.Fatal(err)
	}
	_, evs, ok := strings.Cut(string(raw), "=== events ===\n")
	if !ok {
		t.Fatalf("%s/%s has no events section", name, size)
	}
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(evs), "\n") {
		if l != "none" && l != "" {
			out = append(out, l)
		}
	}
	return out
}

// 57 (property). For every settled frame of the monitor play goldens, the
// hints bar shows exactly the labeled rows that one of their keys would
// fire on the live frame, each with its keycap or the first such key;
// pressing that key fires that row (for a host action, the event is the
// row's action), and no key of a hidden labeled row fires it.
func TestMonitorHintsMatchDispatch(t *testing.T) {
	for _, e := range monitorEntries(t) {
		cols, rows := parseSizeT(t, e.Sizes[0])
		r := &replay{a: monitorApp(t), cols: cols, rows: rows}
		r.run(t, e.Input, func(step int) {
			a := r.a
			f := live(a)
			var shown []string
			for _, it := range f.ByID["keys"].Children {
				shown = append(shown, it.Children[0].Text+" "+it.Children[1].Text)
			}
			var firing []string
			type press struct {
				row int
				tok string
			}
			var presses []press
			a.mu.Lock()
			v := a.liveView()
			for i, kb := range a.doc.Keymap {
				if kb.Label == "" {
					continue
				}
				for _, tok := range kb.Keys {
					if neverArrives(tok) {
						continue
					}
					if p := a.planKey(v, keyOf(tok)); p.step == stepKeymap && p.row == i {
						keycap := tok
						if kb.HasKeycap {
							keycap = kb.Keycap
						}
						firing = append(firing, keycap+" "+kb.Label)
						presses = append(presses, press{i, tok})
						break
					}
				}
			}
			a.mu.Unlock()
			if strings.Join(shown, " | ") != strings.Join(firing, " | ") {
				t.Errorf("%s step %d: the bar shows %q, the dispatch fires %q", e.Name, step, shown, firing)
			}
			// Pressing the key on the same state fires the row.
			for _, p := range presses {
				kb := a.doc.Keymap[p.row]
				if isBuiltinName(kb.Action) {
					continue
				}
				b := &replay{a: monitorApp(t), cols: cols, rows: rows}
				b.run(t, strings.Join(strings.Fields(e.Input)[:step], " "), nil)
				evs := b.a.HandleKey(keyOf(p.tok))
				if len(evs) != 1 || evs[0].Action != kb.Action {
					t.Errorf("%s step %d: %s fires %v, want %s", e.Name, step, p.tok, evs, kb.Action)
				}
			}
		})
	}
}

func isBuiltinName(action string) bool {
	return action == "focus" || strings.Contains(action, "-")
}

// The replay reproduces every monitor play golden's events (the same
// dispatch as `tuimark play`).
func TestMonitorReplayMatchesGoldens(t *testing.T) {
	for _, e := range monitorEntries(t) {
		for _, size := range e.Sizes {
			cols, rows := parseSizeT(t, size)
			r := &replay{a: monitorApp(t), cols: cols, rows: rows}
			r.run(t, e.Input, nil)
			var got []string
			for i, ev := range r.events {
				got = append(got, eventLine(r.steps[i], ev))
			}
			if want := goldenEvents(t, e.Name, size); strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("%s %s:\n got %q\nwant %q", e.Name, size, got, want)
			}
		}
	}
}

// countWriter counts the writes of the loop (each frame is one or two,
// even an empty diff).
type countWriter struct {
	mu sync.Mutex
	n  int
}

func (w *countWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.n++
	w.mu.Unlock()
	return len(p), nil
}

func (w *countWriter) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.n
}

// quiet waits until the loop wrote at least once after since and then
// nothing for a while: every event of the input it handled has been
// dispatched.
func (w *countWriter) quiet(t *testing.T, since int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for w.count() == since {
		if time.Now().After(deadline) {
			t.Fatal("the loop drew nothing")
		}
		time.Sleep(time.Millisecond)
	}
	for {
		n := w.count()
		time.Sleep(60 * time.Millisecond)
		if w.count() == n {
			return
		}
	}
}

// keyBytes are the bytes a terminal sends for a key token (SPEC §15.4).
func keyBytes(tok string) string {
	named := map[string]string{
		"enter": "\r", "esc": "\x1b", "tab": "\t", "backspace": "\x7f", "space": " ",
		"up": "\x1b[A", "down": "\x1b[B", "right": "\x1b[C", "left": "\x1b[D",
		"home": "\x1b[H", "end": "\x1b[F", "pgup": "\x1b[5~", "pgdn": "\x1b[6~",
		"shift+tab": "\x1b[Z", "delete": "\x1b[3~",
	}
	if s, ok := named[tok]; ok {
		return s
	}
	if strings.HasPrefix(tok, "ctrl+") {
		return string(rune(tok[5] - 'a' + 1))
	}
	return tok
}

// 71. For every step of the monitor play goldens, the events equal what
// Run's loop dispatches for the same bytes: keys and mouse reports as a
// terminal sends them, set: and focus: steps as the host's Set.
func TestMonitorLoopParity(t *testing.T) {
	for _, e := range monitorEntries(t) {
		for _, size := range e.Sizes {
			e, size := e, size
			t.Run(e.Name+"/"+size, func(t *testing.T) {
				t.Parallel()
				cols, rows := parseSizeT(t, size)
				a := monitorApp(t)
				var mu sync.Mutex
				step := 0
				var got []string
				for _, spec := range a.Catalog() {
					if spec.Builtin {
						continue
					}
					a.On(spec.Name, func(ev Event) error {
						mu.Lock()
						got = append(got, eventLine(step, ev))
						mu.Unlock()
						return nil
					})
				}
				pr, pw := io.Pipe()
				out := &countWriter{}
				done := make(chan error, 1)
				go func() { done <- a.Loop(pr, out, func() (int, int) { return cols, rows }, nil) }()
				out.quiet(t, 0)
				for i, tok := range strings.Fields(e.Input) {
					mu.Lock()
					step = i + 1
					mu.Unlock()
					before := out.count()
					switch {
					case strings.HasPrefix(tok, "text:"):
						_, _ = io.WriteString(pw, tok[5:])
					case strings.HasPrefix(tok, "set:"):
						path, js, _ := strings.Cut(tok[4:], "=")
						var v any
						_ = json.Unmarshal([]byte(js), &v)
						_ = a.Set(path, v)
					case strings.HasPrefix(tok, "focus:"):
						_ = a.Set("@focus", tok[6:])
					case strings.HasPrefix(tok, "click:"), strings.HasPrefix(tok, "wheel-"):
						name, cell, _ := strings.Cut(tok, ":")
						xs, ys, _ := strings.Cut(cell, ",")
						x, _ := strconv.Atoi(xs)
						y, _ := strconv.Atoi(ys)
						switch name {
						case "click":
							_, _ = fmt.Fprintf(pw, "\x1b[<0;%d;%dM\x1b[<0;%d;%dm", x+1, y+1, x+1, y+1)
						case "wheel-up":
							_, _ = fmt.Fprintf(pw, "\x1b[<64;%d;%dM", x+1, y+1)
						default:
							_, _ = fmt.Fprintf(pw, "\x1b[<65;%d;%dM", x+1, y+1)
						}
					default:
						_, _ = io.WriteString(pw, keyBytes(tok))
					}
					out.quiet(t, before)
				}
				pw.Close()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Fatal("the loop did not return at EOF")
				}
				mu.Lock()
				defer mu.Unlock()
				if want := goldenEvents(t, e.Name, size); strings.Join(got, "\n") != strings.Join(want, "\n") {
					t.Errorf("the loop dispatched\n%q\nthe golden has\n%q", got, want)
				}
			})
		}
	}
}

// 72. Set("@theme"), Set("mouse_enabled"), and Set("procs") from
// goroutines while the loop renders and handles keys and mouse reports
// (run with -race).
func TestMonitorSetWhileRunning(t *testing.T) {
	a := monitorApp(t)
	pr, pw := io.Pipe()
	out := &countWriter{}
	done := make(chan error, 1)
	go func() { done <- a.Loop(pr, out, func() (int, int) { return 100, 30 }, nil) }()
	procs, _ := a.Get("procs")
	arr := procs.([]any)
	var wg sync.WaitGroup
	for g := 0; g < 3; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 40; i++ {
				switch g {
				case 0:
					_ = a.Set("@theme", []string{"dark", "light", "auto"}[i%3])
				case 1:
					_ = a.Set("mouse_enabled", i%2 == 0)
				case 2:
					_ = a.Set("procs", arr[:len(arr)-i%10])
				}
				time.Sleep(time.Millisecond)
			}
		}(g)
	}
	for i := 0; i < 20; i++ {
		_, _ = io.WriteString(pw, "\x1b[B\x1b[<65;20;6M\x1b[<0;70;2M\x1b[<0;70;2m ")
		time.Sleep(2 * time.Millisecond)
	}
	wg.Wait()
	pw.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the loop did not return")
	}
	if out.count() == 0 {
		t.Fatal("nothing drawn")
	}
}
