package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseQuery(t *testing.T) {
	inc, exc := parseQuery("  Billing -RETRY !card - ! timeout ")
	if !reflect.DeepEqual(inc, []string{"billing", "timeout"}) || !reflect.DeepEqual(exc, []string{"retry", "card"}) {
		t.Errorf("include=%v exclude=%v", inc, exc)
	}
	if inc, exc := parseQuery(""); inc != nil || exc != nil {
		t.Errorf("empty query gave %v %v", inc, exc)
	}
}

func testLines() []any {
	es := []entry{
		{TS: "09:00:00.000", Level: lvlInfo, Svc: "gateway", Msg: "GET /api/orders 200 12ms"},
		{TS: "09:00:00.100", Level: lvlError, Svc: "orders", Msg: "panic: nil pointer"},
		{Level: lvlError, Msg: "↳ handler.go:142", Cont: true},
		{TS: "09:00:00.200", Level: lvlDebug, Svc: "cache", Msg: "miss key=user:1"},
		{TS: "09:00:00.300", Level: lvlWarn, Svc: "billing", Msg: "retry 1/3"},
	}
	a := &app{cap: 100}
	a.append(es)
	return a.items()
}

func ids(lines []any) []float64 {
	var out []float64
	for _, l := range lines {
		out = append(out, float64(l.(map[string]any)["id"].(int)))
	}
	return out
}

func allOn() map[string]any {
	return map[string]any{lvlError: true, lvlWarn: true, lvlInfo: true, lvlDebug: true}
}

func TestVisibleLines(t *testing.T) {
	lines := testLines()
	tests := []struct {
		name   string
		levels map[string]any
		query  string
		want   []int
	}{
		{"everything", allOn(), "", []int{1, 2, 3, 4, 5}},
		{"nil levels show all", nil, "", []int{1, 2, 3, 4, 5}},
		{"a level off", map[string]any{lvlDebug: false}, "", []int{1, 2, 3, 5}},
		{"a stack goes with its error", map[string]any{lvlError: false}, "", []int{1, 4, 5}},
		{"substring, any case", allOn(), "ORDERS", []int{1, 2, 3}},
		{"a frame's text finds its error", allOn(), "handler.go", []int{2, 3}},
		{"all words must match", allOn(), "orders panic", []int{2, 3}},
		{"exclude", allOn(), "-orders", []int{4, 5}},
		{"include and exclude", allOn(), "e -panic", []int{1, 4, 5}},
		{"no match", allOn(), "zzz", nil},
		{"levels and query together", map[string]any{lvlError: false}, "e -cache", []int{1, 5}},
	}
	for _, tc := range tests {
		got := visibleLines(lines, tc.levels, tc.query)
		var gotIDs []int
		for _, id := range ids(got) {
			gotIDs = append(gotIDs, int(id))
		}
		if !reflect.DeepEqual(gotIDs, tc.want) {
			t.Errorf("%s: ids %v, want %v", tc.name, gotIDs, tc.want)
		}
		for _, l := range got {
			if _, has := l.(map[string]any)["hay"]; has {
				t.Errorf("%s: a visible line still carries its search text", tc.name)
			}
		}
	}
}

func TestComputeStats(t *testing.T) {
	lines := testLines()
	vis := visibleLines(lines, map[string]any{lvlDebug: false}, "")
	s := computeStats(lines, vis)
	want := map[string]any{"error": 1, "warn": 1, "info": 1, "debug": 1, "total": 5, "shown": 4, "none": false}
	if !reflect.DeepEqual(s, want) {
		t.Errorf("stats = %v, want %v", s, want)
	}
	if s := computeStats(nil, nil); s["none"] != true || s["total"] != 0 {
		t.Errorf("empty stats = %v", s)
	}
}

func TestGroupHayKeepsAStackWhole(t *testing.T) {
	es := []entry{
		{Level: lvlError, Svc: "Orders", Msg: "Boom"},
		{Level: lvlError, Msg: "frame ONE", Cont: true},
		{Level: lvlError, Msg: "frame two", Cont: true},
		{Level: lvlInfo, Svc: "x", Msg: "after"},
	}
	want := "orders error boom frame one frame two"
	for i := 0; i < 3; i++ {
		if got := groupHay(es, i); got != want {
			t.Errorf("groupHay(%d) = %q, want %q", i, got, want)
		}
	}
	if got := groupHay(es, 3); got != "x info after" {
		t.Errorf("groupHay(3) = %q", got)
	}
}

// ---- the generator ----

func take(seed int64, ticks int) []entry {
	g := newFake(seed)
	var out []entry
	for i := 0; i < ticks; i++ {
		es, err := g.Next()
		if err != nil {
			panic(err)
		}
		out = append(out, es...)
	}
	return out
}

func TestGeneratorIsDeterministic(t *testing.T) {
	a, b := take(7, 200), take(7, 200)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("the same seed gave different streams")
	}
	if reflect.DeepEqual(a, take(8, 200)) {
		t.Fatal("different seeds gave the same stream")
	}
	// The simulated clock, not the wall clock, stamps the lines.
	if first := a[0].TS; !strings.HasPrefix(first, "09:41:00") {
		t.Errorf("first timestamp %q does not start at the simulated epoch", first)
	}
}

func TestGeneratorMixAndShape(t *testing.T) {
	es := take(3, 2000)
	counts := map[string]int{}
	var stacks int
	for i, e := range es {
		counts[e.Level]++
		if e.Cont {
			if i == 0 || es[i-1].Svc == "" && !es[i-1].Cont {
				t.Fatalf("a stack frame without a parent at %d", i)
			}
			if e.TS != "" || e.Svc != "" {
				t.Errorf("a stack frame has a timestamp or service: %+v", e)
			}
			continue
		}
		if len(e.TS) != len(tsLayout) || e.Svc == "" || e.Msg == "" {
			t.Fatalf("malformed entry %+v", e)
		}
		if e.Level == lvlError && i+1 < len(es) && es[i+1].Cont {
			stacks++
		}
	}
	for _, l := range levelNames {
		if counts[l] == 0 {
			t.Errorf("no %s lines in %d entries", l, len(es))
		}
	}
	if counts[lvlInfo] <= counts[lvlError] || counts[lvlDebug] <= counts[lvlError] {
		t.Errorf("errors should be the rarest level: %v", counts)
	}
	if stacks == 0 {
		t.Error("no error came with a stack")
	}
}

func TestBacklogStartsOnAParentAndFillsTheHistory(t *testing.T) {
	es, hist := backlog(newFake(defaultSeed))
	if len(es) < backlogLines || es[0].Cont {
		t.Errorf("backlog: %d entries, first is a frame: %v", len(es), es[0].Cont)
	}
	if len(hist) != histLen {
		t.Errorf("history has %d seconds, want %d", len(hist), histLen)
	}
}

func TestRingBufferKeepsTheNewestAndDropsOrphanFrames(t *testing.T) {
	a := &app{cap: 2}
	a.append([]entry{
		{Level: lvlInfo, Svc: "a", Msg: "1"},
		{Level: lvlError, Svc: "a", Msg: "2"},
		{Level: lvlError, Msg: "frame", Cont: true},
		{Level: lvlInfo, Svc: "a", Msg: "4"},
	})
	// cap 2 would start on the frame; it must start on "4" instead.
	if len(a.buf) != 1 || a.buf[0].m["msg"] != "4" || a.buf[0].id != 4 {
		t.Errorf("buffer = %+v", a.buf)
	}
	a.append([]entry{{Level: lvlInfo, Svc: "a", Msg: "5"}, {Level: lvlInfo, Svc: "a", Msg: "6"}})
	if len(a.buf) != 2 || a.buf[0].id != 5 || a.buf[1].id != 6 {
		t.Errorf("buffer = %+v", a.buf)
	}
}

// ---- -follow ----

func TestParseLevel(t *testing.T) {
	tests := map[string]string{
		"2026-10-17T09:41:07Z ERROR db: connection refused":    lvlError,
		"[warn] disk 91% full":                                 lvlWarn,
		"level=debug msg=\"cache miss\"":                       lvlDebug,
		"INFO  server listening on :8080":                      lvlInfo,
		"Oct 17 kernel: WARNING: CPU soft lockup":              lvlWarn,
		"FATAL: out of memory":                                 lvlError,
		"trace id=ab12 span started":                           lvlDebug,
		"E ERR timeout talking to upstream":                    lvlError,
		"the err variable is unset":                            lvlInfo, // lowercase err is not a level
		"informational note, no level word":                    lvlInfo,
		"INFO request failed with error 500":                   lvlInfo, // the first level word wins
		"plain line":                                           lvlInfo,
		"":                                                     lvlInfo,
		"worker-3 DBG tick":                                    lvlDebug,
		"Error: ENOENT":                                        lvlError,
		"warnings=0 errors=0 (counters, not a level) all fine": lvlInfo,
	}
	for in, want := range tests {
		if got := parseLevel(in); got != want {
			t.Errorf("parseLevel(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestSanitize(t *testing.T) {
	in := "\x1b[31mred\x1b[0m\ttab\r\x00 bell\a done  "
	if got, want := sanitize(in), "red tab bell done"; got != want {
		t.Errorf("sanitize = %q, want %q", got, want)
	}
}

func TestFileSourceFollowsAppendedLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, []byte("INFO boot one\nWARN boot two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	src, backlog, err := newFileSource(path, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	if src.Name() != "tail -f app.log" {
		t.Errorf("name = %q", src.Name())
	}
	if len(backlog) != 2 || backlog[0].Level != lvlInfo || backlog[1].Level != lvlWarn || backlog[0].Svc != "app.log" {
		t.Fatalf("backlog = %+v", backlog)
	}
	if es, err := src.Next(); err != nil || len(es) != 0 {
		t.Fatalf("nothing was appended, got %+v %v", es, err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	// A line cut in the middle waits for its newline.
	f.WriteString("ERROR disk fa")
	if es, _ := src.Next(); len(es) != 0 {
		t.Fatalf("a partial line was reported: %+v", es)
	}
	f.WriteString("iled\n\n   \ndebug tick\nlast without newline")
	clock = clock.Add(1500 * time.Millisecond)
	es, err := src.Next()
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 2 || es[0].Msg != "ERROR disk failed" || es[0].Level != lvlError || es[1].Level != lvlDebug {
		t.Fatalf("entries = %+v", es)
	}
	if es[0].TS != "12:00:01.500" {
		t.Errorf("ts = %q, want the reading time", es[0].TS)
	}
	f.WriteString("\n")
	if es, _ := src.Next(); len(es) != 1 || es[0].Msg != "last without newline" {
		t.Fatalf("the carried line was lost: %+v", es)
	}

	// Truncation (log rotation) starts the file over.
	if err := os.WriteFile(path, []byte("INFO fresh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	es, err = src.Next()
	if err != nil || len(es) != 2 || !strings.Contains(es[0].Msg, "truncated") || es[1].Msg != "INFO fresh" {
		t.Fatalf("after truncation: %+v %v", es, err)
	}

	// A file that vanishes is an error for the tick, not a crash.
	os.Remove(path)
	if _, err := src.Next(); err == nil || !strings.HasPrefix(err.Error(), "follow:") {
		t.Errorf("err = %v, want a follow error", err)
	}
}

func TestFileSourceStartsAtTheTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.log")
	var b strings.Builder
	for i := 0; i < 20000; i++ {
		b.WriteString("INFO line number padded to be long enough to overflow the tail window\n")
	}
	b.WriteString("ERROR the very last line\n")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	_, es, err := newFileSource(path, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) == 0 || len(es) >= 20000 {
		t.Fatalf("backlog has %d lines, want a bounded tail", len(es))
	}
	if last := es[len(es)-1]; last.Msg != "ERROR the very last line" || last.Level != lvlError {
		t.Errorf("last = %+v", last)
	}
	if !strings.HasPrefix(es[0].Msg, "INFO line number") {
		t.Errorf("the first backlog line was cut: %q", es[0].Msg)
	}
	if _, _, err := newFileSource(filepath.Join(t.TempDir(), "missing.log"), time.Now); err == nil {
		t.Error("a missing file must fail the start")
	}
	if _, _, err := newFileSource(t.TempDir(), time.Now); err == nil {
		t.Error("a directory must fail the start")
	}
}

func TestUseFileDrivesTheView(t *testing.T) {
	path := filepath.Join(t.TempDir(), "svc.log")
	if err := os.WriteFile(path, []byte("INFO up\nERROR down\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := loadApp(t)
	if err := a.useFile(path); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.ui.Get("source"); v != "tail -f svc.log" {
		t.Errorf("source = %v", v)
	}
	g := grid(t, a, 100, 24)
	if !strings.Contains(g, "tail -f svc.log") || !strings.Contains(g, "2 of 2 lines") || !strings.Contains(g, "ERROR svc.log") {
		t.Errorf("the file's lines are not shown:\n%s", g)
	}
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("WARN flaky\n")
	f.Close()
	if err := a.ui.Batch(a.tick); err != nil {
		t.Fatal(err)
	}
	if g := grid(t, a, 100, 24); !strings.Contains(g, "3 of 3 lines") || !strings.Contains(g, "WARN  svc.log  WARN flaky") {
		t.Errorf("the appended line is not shown:\n%s", g)
	}
	// A paused stream does not read; resuming catches up.
	a.paused = true
	f, _ = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("INFO while paused\n")
	f.Close()
	_ = a.ui.Batch(a.tick)
	if len(a.buf) != 3 {
		t.Fatalf("a paused tick read the file: %d lines", len(a.buf))
	}
	a.paused = false
	_ = a.ui.Batch(a.tick)
	if len(a.buf) != 4 {
		t.Errorf("resuming did not catch up: %d lines", len(a.buf))
	}
}

func TestAFailingTickReachesOnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gone.log")
	os.WriteFile(path, []byte("INFO x\n"), 0o644)
	a := loadApp(t)
	if err := a.useFile(path); err != nil {
		t.Fatal(err)
	}
	os.Remove(path)
	err := a.ui.Batch(a.tick)
	if err == nil {
		t.Fatal("tick must return the source's error")
	}
	// RunWith hands that error to onError, which keeps the session going.
	if err := a.onError(nil, err); err != nil {
		t.Fatal(err)
	}
	if v, _ := a.ui.Get("notice"); !strings.HasPrefix(v.(string), "follow:") {
		t.Errorf("notice = %v", v)
	}
}

func TestUseFakeWithAnotherSeedChangesTheStream(t *testing.T) {
	a := loadApp(t)
	before, _ := a.ui.Get("lines")
	if err := a.useFake(99); err != nil {
		t.Fatal(err)
	}
	after, _ := a.ui.Get("lines")
	if reflect.DeepEqual(before, after) {
		t.Error("another seed gave the same lines")
	}
	if err := a.useFake(defaultSeed); err != nil {
		t.Fatal(err)
	}
	again, _ := a.ui.Get("lines")
	if !reflect.DeepEqual(before, again) {
		t.Error("the default seed does not reproduce sample.json's lines")
	}
}
