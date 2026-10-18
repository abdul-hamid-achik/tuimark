package main

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// newApp loads the sample app with its handlers and Derives registered,
// the way run does.
func newApp(t *testing.T) *app {
	t.Helper()
	a, err := load(".")
	if err != nil {
		t.Fatal(err)
	}
	a.register()
	return a
}

func play(t *testing.T, a *app, steps ...string) *tuimark.PlayResult {
	t.Helper()
	res, err := a.ui.Play(tuimark.PlayOptions{Cols: 100, Rows: 30}, steps...)
	if err != nil {
		t.Fatalf("play %v: %v", steps, err)
	}
	if len(res.Dump.Errors) != 0 {
		t.Fatalf("play %v: the frame has diagnostics: %v", steps, res.Dump.Errors)
	}
	return res
}

// actions lists the actions of a result's events, in order.
func actions(res *tuimark.PlayResult) []string {
	var out []string
	for _, e := range res.Events {
		out = append(out, e.Action)
	}
	return out
}

func get[T any](t *testing.T, a *app, path string) T {
	t.Helper()
	v, ok := read[T](a.ui.Get, path)
	if !ok {
		t.Fatalf("store path %q missing or of another shape", path)
	}
	return v
}

func paths(rows []fileRow) []string {
	out := []string{}
	for _, r := range rows {
		out = append(out, r.Path)
	}
	return out
}

func screenOf(t *testing.T, a *app, cols, rows int, steps ...string) string {
	t.Helper()
	res, err := a.ui.Play(tuimark.PlayOptions{Cols: cols, Rows: rows}, steps...)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Dump.Errors) != 0 {
		t.Fatalf("dump errors: %v", res.Dump.Errors)
	}
	return strings.Join(res.Dump.Grid, "\n")
}

// ---- document, data, and handlers

func TestValidateIsClean(t *testing.T) {
	a := newApp(t)
	for _, theme := range []string{"dark", "light"} {
		if err := a.ui.Set("@theme", theme); err != nil {
			t.Fatal(err)
		}
		if diags := a.ui.Validate(); len(diags) != 0 {
			t.Errorf("%s: Validate gave %v", theme, diags)
		}
	}
}

// Dump is the handler-free snapshot: no focus event has run, so everything
// on screen comes from sample.json and the Derives.
func TestDumpSnapshot(t *testing.T) {
	a := newApp(t)
	for _, c := range []struct {
		cols, rows int
		want       []string
	}{
		{80, 24, []string{"◆ gitui", "tidepool", "4 staged · 5 unstaged", "staged · 4", "unstaged · 5", "README.md (unstaged)", "+q, err := tidepool.Open", "q quit  ? help"}},
		{120, 30, []string{"1 status (9)", "internal/queue/backoff_test.go", "testdata/legacy.json", "-q := tidepool.Open"}},
	} {
		d, err := a.ui.Dump(c.cols, c.rows)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Errors) != 0 || !d.OK {
			t.Errorf("%dx%d: dump diagnostics %v", c.cols, c.rows, d.Errors)
		}
		if len(d.Grid) != c.rows {
			t.Errorf("%dx%d: grid has %d rows", c.cols, c.rows, len(d.Grid))
		}
		s := strings.Join(d.Grid, "\n")
		for _, w := range c.want {
			if !strings.Contains(s, w) {
				t.Errorf("%dx%d: want %q in\n%s", c.cols, c.rows, w, s)
			}
		}
	}
}

// The modals are closed in the sample, so Validate does not lay them out:
// open each at the sizes the checks use and expect no warning (a modal cutting
// its content, say).
func TestOpenModalsLayOutWithoutWarnings(t *testing.T) {
	for _, size := range [][2]int{{40, 20}, {80, 24}, {120, 30}} {
		for _, theme := range []string{"dark", "light"} {
			for _, steps := range [][]string{
				{"2", "set:detail_open=true"},
				{"3", "set:confirm_open=true"},
				{"set:help_open=true"},
			} {
				a := newApp(t)
				if err := a.ui.Set("@theme", theme); err != nil {
					t.Fatal(err)
				}
				res, err := a.ui.Play(tuimark.PlayOptions{Cols: size[0], Rows: size[1]}, steps...)
				if err != nil {
					t.Fatal(err)
				}
				if len(res.Dump.Errors) != 0 {
					t.Errorf("%dx%d %s %v: %v", size[0], size[1], theme, steps, res.Dump.Errors)
				}
			}
		}
	}
}

func TestDumpShowsEachTab(t *testing.T) {
	a := newApp(t)
	cases := []struct {
		cols, rows int
		steps      []string
		want       []string
	}{
		{80, 24, nil, []string{"gitui", "tidepool", "↑2 ↓1", "staged · 4", "unstaged · 5", "README.md (unstaged)", "@@ -12,7", "space (un)stage", "q quit"}},
		{120, 30, nil, []string{"4 staged · 5 unstaged", "internal/queue/backoff_test.go", "1 status (9)"}},
		{80, 24, []string{"2"}, []string{"12 commits", "HASH", "queue: retry with jittered backoff", "Priya Raman", "enter open", "/ filter"}},
		{120, 30, []string{"2"}, []string{"REFS", "main", "origin/main", "v0.4.0", "feature/dedupe", "WHEN"}},
		{80, 24, []string{"3"}, []string{"local branches", "● main", "feature/dedupe", "enter checkout"}},
		{120, 30, []string{"3"}, []string{"UPSTREAM", "origin/release/0.4", "LAST COMMIT", "docs: describe the dedupe design"}},
	}
	for _, c := range cases {
		s := screenOf(t, a, c.cols, c.rows, c.steps...)
		for _, w := range c.want {
			if !strings.Contains(s, w) {
				t.Errorf("%dx%d %v: want %q in\n%s", c.cols, c.rows, c.steps, w, s)
			}
		}
	}
}

// Narrow terminals drop log columns by priority: refs first, then author.
func TestLogColumnsDropByPriority(t *testing.T) {
	a := newApp(t)
	wide := screenOf(t, a, 120, 24, "2")
	if !strings.Contains(wide, "REFS") || !strings.Contains(wide, "AUTHOR") {
		t.Fatalf("120 columns should show every column:\n%s", wide)
	}
	narrow := screenOf(t, a, 60, 24, "2")
	if strings.Contains(narrow, "REFS") || !strings.Contains(narrow, "SUBJECT") || !strings.Contains(narrow, "HASH") {
		t.Errorf("60 columns should drop REFS and keep SUBJECT and HASH:\n%s", narrow)
	}
	tiny := screenOf(t, a, 40, 24, "2")
	if strings.Contains(tiny, "AUTHOR") || strings.Contains(tiny, "REFS") {
		t.Errorf("40 columns should drop AUTHOR and REFS:\n%s", tiny)
	}
}

func TestHandlersCoverCatalogAndMock(t *testing.T) {
	a := newApp(t)
	hs := a.handlers()
	known := map[string]bool{}
	for _, spec := range a.ui.Catalog() {
		if spec.Builtin {
			continue
		}
		known[spec.Name] = true
		if _, ok := hs[spec.Name]; !ok {
			t.Errorf("no handler for %s (%v)", spec.Name, spec.Sources)
		}
		if !spec.Registered {
			t.Errorf("%s is not registered with the app", spec.Name)
		}
	}
	for name := range hs {
		if !known[name] && name != "quit" { // quit is a built-in action
			t.Errorf("handler %s is never fired by app.tui", name)
		}
	}
	raw, err := os.ReadFile("mock.json")
	if err != nil {
		t.Fatal(err)
	}
	var mock map[string][]struct {
		Path  string `json:"path"`
		Value any    `json:"value"`
	}
	if err := json.Unmarshal(raw, &mock); err != nil {
		t.Fatal(err)
	}
	for name, writes := range mock {
		if !known[name] {
			t.Errorf("mock.json answers %s, which app.tui does not fire", name)
		}
		if len(writes) == 0 {
			t.Errorf("mock.json: %s has no writes", name)
		}
	}
	if err := hs["quit"](tuimark.Event{}); !errors.Is(err, tuimark.ErrQuit) {
		t.Errorf("quit returned %v", err)
	}
}

// sample.json carries the Derive outputs and the first preview so that
// `tuimark dump --data sample.json` shows the whole app with no Go: this
// pins them to what the host computes.
func TestSampleIsAFixedPointOfTheHost(t *testing.T) {
	a, err := load(".")
	if err != nil {
		t.Fatal(err)
	}
	derived := []string{"sync_label", "staged", "unstaged", "counts", "log_view", "log_label"}
	before := map[string]any{}
	for _, p := range derived {
		v, ok := a.ui.Get(p)
		if !ok {
			t.Fatalf("sample.json lacks %s", p)
		}
		before[p] = v
	}
	a.register()
	for _, p := range derived {
		after, _ := a.ui.Get(p)
		if !reflect.DeepEqual(before[p], after) {
			t.Errorf("sample.json's %s differs from the Derive's:\n sample %v\n derive %v", p, before[p], after)
		}
	}
	// the first preview and the first commit detail
	f, _ := a.fileAt("README.md")
	want, err := a.diffFor("unstaged", f)
	if err != nil {
		t.Fatal(err)
	}
	if got := get[diffView](t, a, "diff"); !reflect.DeepEqual(got, want) {
		t.Errorf("sample.json's diff differs from the host's:\n got  %+v\n want %+v", got, want)
	}
	log := get[[]commit](t, a, "log")
	raw, err := a.be.Commit(log[0].Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := get[detailView](t, a, "detail"), buildDetail(log[0].Hash, raw, maxDiffLines); !reflect.DeepEqual(got, want) {
		t.Errorf("sample.json's detail differs from the host's:\n got  %+v\n want %+v", got, want)
	}
}

// ---- Play flows

func TestPlayStageAndUnstage(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "space")
	if got := actions(res); !reflect.DeepEqual(got, []string{"file_select", "stage_toggle"}) {
		t.Fatalf("events = %v", got)
	}
	if e := res.Events[1]; e.Source != "unstaged" || e.Keys["f"] != "README.md" {
		t.Errorf("stage_toggle event = %+v", e)
	}
	if c := get[counts](t, a, "counts"); c.Staged != 5 || c.Unstaged != 4 || c.Label != "5 staged · 4 unstaged" {
		t.Errorf("counts = %+v", c)
	}
	if got := paths(get[[]fileRow](t, a, "staged")); !contains(got, "README.md") {
		t.Errorf("README.md did not reach the staged list: %v", got)
	}
	// the cursor moved to the row that took README.md's place, and the
	// preview follows it
	if got := get[string](t, a, "cur.unstaged"); got != "cmd/tidepool/main.go" {
		t.Errorf("cur.unstaged = %q", got)
	}
	if d := get[diffView](t, a, "diff"); d.Title != "cmd/tidepool/main.go (unstaged)" {
		t.Errorf("diff title = %q", d.Title)
	}
	if st := get[map[string]any](t, a, "status"); st["text"] != "staged README.md" || st["err"] != false {
		t.Errorf("status = %v", st)
	}
	if s := strings.Join(res.Dump.Grid, "\n"); !strings.Contains(s, "5 staged · 4 unstaged") || !strings.Contains(s, "staged README.md") {
		t.Errorf("the screen does not show the new counts and status:\n%s", s)
	}

	// shift+tab moves to the staged list, whose cursor is on the file just
	// staged; space there unstages it again
	res = play(t, a, "shift+tab", "space")
	if got := actions(res); !reflect.DeepEqual(got, []string{"file_select", "stage_toggle"}) {
		t.Fatalf("events = %v", got)
	}
	if e := res.Events[1]; e.Source != "staged" {
		t.Errorf("stage_toggle source = %q", e.Source)
	}
	if c := get[counts](t, a, "counts"); c.Staged != 4 || c.Unstaged != 5 {
		t.Errorf("counts after unstage = %+v", c)
	}
}

func TestPlayStageAllMovesFocusToTheOtherList(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "a")
	if c := get[counts](t, a, "counts"); c.Staged != 9 || c.Unstaged != 0 {
		t.Errorf("counts = %+v", c)
	}
	if res.Dump.Focus == nil || *res.Dump.Focus != "staged" {
		t.Errorf("focus = %v, want the staged list (the unstaged one is empty)", res.Dump.Focus)
	}
	// an untracked file became an added one
	rows := get[[]fileRow](t, a, "staged")
	for _, r := range rows {
		if r.Path == "internal/queue/backoff_test.go" && r.Code != "A" {
			t.Errorf("backoff_test.go is %q, want A", r.Code)
		}
	}
	// unstage everything: the added file is untracked again and the rename
	// is a deletion plus an untracked file
	play(t, a, "A")
	c := get[counts](t, a, "counts")
	if c.Staged != 0 || c.Unstaged != 10 || c.Total != 10 {
		t.Errorf("counts after unstage all = %+v", c)
	}
	un := get[[]fileRow](t, a, "unstaged")
	codes := map[string]string{}
	for _, r := range un {
		codes[r.Path] = r.Code
	}
	if codes["internal/cfg/load.go"] != "?" || codes["internal/cfg/config.go"] != "D" || codes["internal/queue/dedupe.go"] != "?" {
		t.Errorf("unstaged codes = %v", codes)
	}
}

func TestPlayOpenCommitDetail(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "2", "down", "enter")
	last := res.Events[len(res.Events)-1]
	hash := get[[]commit](t, a, "log")[1].Hash
	if last.Action != "commit_open" || last.Keys["c"] != hash {
		t.Fatalf("last event = %+v, want commit_open for %s", last, hash)
	}
	d := get[detailView](t, a, "detail")
	if d.Title != "commit 65a3e23" || d.Subject != "queue: fix lost wakeup in worker shutdown" || len(d.Lines) == 0 {
		t.Errorf("detail = %+v", d)
	}
	if !get[bool](t, a, "detail_open") || res.Dump.Focus == nil || *res.Dump.Focus != "commit-diff" {
		f := "<nil>"
		if res.Dump.Focus != nil {
			f = *res.Dump.Focus
		}
		t.Errorf("the modal is not open and focused: focus=%s", f)
	}
	s := strings.Join(res.Dump.Grid, "\n")
	for _, w := range []string{"commit 65a3e23", "Marcus Webb <marcus@tidepool.dev>", "1 file changed, +", "internal/queue/worker.go", "q close commit"} {
		if !strings.Contains(s, w) {
			t.Errorf("want %q on screen:\n%s", w, s)
		}
	}
	// esc closes it through on:escape
	play(t, a, "esc")
	if get[bool](t, a, "detail_open") {
		t.Errorf("esc did not close the detail")
	}
}

func TestPlayFilterTheLog(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "2", "/", "text:Lena")
	if got := actions(res); !reflect.DeepEqual(got, []string{"file_select", "filter_open"}) {
		t.Errorf("events = %v", got)
	}
	if q := get[string](t, a, "log_query"); q != "Lena" {
		t.Fatalf("log_query = %q", q)
	}
	view := get[[]commit](t, a, "log_view")
	if len(view) != 3 {
		t.Fatalf("log_view has %d rows, want the 3 by Lena", len(view))
	}
	if label := get[string](t, a, "log_label"); label != `3 of 12 commits · filter "Lena"` {
		t.Errorf("log_label = %q", label)
	}
	s := strings.Join(res.Dump.Grid, "\n")
	if strings.Contains(s, "Marcus Webb") || !strings.Contains(s, "cfg: load config from XDG paths") || !strings.Contains(s, "filter by subject") && !strings.Contains(s, "Lena") {
		t.Errorf("the table is not filtered:\n%s", s)
	}
	// enter keeps the filter and returns to the table; esc clears it
	play(t, a, "enter")
	if !get[bool](t, a, "filter_open") || get[string](t, a, "log_query") != "Lena" {
		t.Errorf("enter should keep the filter")
	}
	res = play(t, a, "esc")
	if get[bool](t, a, "filter_open") || get[string](t, a, "log_query") != "" || len(get[[]commit](t, a, "log_view")) != 12 {
		t.Errorf("esc did not clear the filter")
	}
	if res.Dump.Focus == nil || *res.Dump.Focus != "commits" {
		t.Errorf("focus = %v, want the table", res.Dump.Focus)
	}
	// a filter that matches nothing shows the placeholder
	s = screenOf(t, a, 100, 30, "/", "text:zzz")
	if !strings.Contains(s, "no commits match") || !strings.Contains(s, "0 of 12 commits") {
		t.Errorf("empty filter result:\n%s", s)
	}
}

func TestPlayCheckoutConfirm(t *testing.T) {
	a := newApp(t)
	res := play(t, a, "3", "down", "enter")
	if last := res.Events[len(res.Events)-1]; last.Action != "checkout_ask" || last.Keys["b"] != "feature/dedupe" {
		t.Fatalf("last event = %+v", last)
	}
	if res.Dump.Focus == nil || *res.Dump.Focus != "checkout" || !strings.Contains(strings.Join(res.Dump.Grid, "\n"), "Check out feature/dedupe?") {
		t.Fatalf("the confirmation is not open: focus=%v", res.Dump.Focus)
	}
	// n leaves everything as it was
	play(t, a, "n")
	if get[bool](t, a, "confirm_open") || get[repoInfo](t, a, "repo").Branch != "main" {
		t.Errorf("n should only close the confirmation")
	}
	// y switches branch: the header, the marks, and the sync label follow
	play(t, a, "enter", "y")
	if r := get[repoInfo](t, a, "repo"); r.Branch != "feature/dedupe" || r.Upstream != "origin/feature/dedupe" || r.Ahead != 0 {
		t.Errorf("repo = %+v", r)
	}
	if label := get[string](t, a, "sync_label"); label != "in sync" {
		t.Errorf("sync_label = %q", label)
	}
	marks := map[string]string{}
	for _, b := range get[[]branch](t, a, "branches") {
		marks[b.Name] = b.Mark
	}
	if marks["feature/dedupe"] != "●" || marks["main"] != "" {
		t.Errorf("marks = %v", marks)
	}
	if st := get[map[string]any](t, a, "status"); st["text"] != "switched to feature/dedupe" {
		t.Errorf("status = %v", st)
	}
	s := screenOf(t, a, 100, 30)
	if !strings.Contains(s, "feature/dedupe in sync") {
		t.Errorf("the header does not show the new branch:\n%s", s)
	}
	// enter on the current branch asks nothing
	res = play(t, a, "3", "enter")
	if get[bool](t, a, "confirm_open") {
		t.Errorf("the current branch should not ask")
	}
	if st := get[map[string]any](t, a, "status"); st["text"] != "already on feature/dedupe" {
		t.Errorf("status = %v", st)
	}
}

func TestPlayTabsAndKeys(t *testing.T) {
	a := newApp(t)
	for _, c := range []struct {
		keys  []string
		view  string
		focus string
	}{
		{[]string{"2"}, "log", "commits"},
		{[]string{"3"}, "branches", "branch-table"},
		{[]string{"1"}, "status", "unstaged"},
		{[]string{"]"}, "log", "commits"},
		{[]string{"]", "["}, "log", "commits"},
	} {
		res := play(t, a, c.keys...)
		if v := get[string](t, a, "view"); v != c.view {
			t.Errorf("%v: view = %q, want %q", c.keys, v, c.view)
		}
		if res.Dump.Focus == nil || *res.Dump.Focus != c.focus {
			t.Errorf("%v: focus = %v, want %q", c.keys, res.Dump.Focus, c.focus)
		}
	}
	// help opens a modal listing every key row, then closes
	res := play(t, a, "?")
	s := strings.Join(res.Dump.Grid, "\n")
	for _, w := range []string{"keys", "stage all", "close help", "cancel checkout"} {
		if !strings.Contains(s, w) {
			t.Errorf("help lacks %q:\n%s", w, s)
		}
	}
	play(t, a, "q")
	if get[bool](t, a, "help_open") {
		t.Errorf("q should close the help")
	}
	if res := play(t, a, "q"); !res.Quit {
		t.Errorf("q did not quit")
	}
}

func TestPlayRefreshAndErrors(t *testing.T) {
	a := newApp(t)
	play(t, a, "r")
	if st := get[map[string]any](t, a, "status"); st["text"] != "refreshed" {
		t.Errorf("status = %v", st)
	}
	// an action that fails is reported in the status line by RunWith's
	// OnError, which is a.onError
	if err := a.onError(nil, errors.New("git status: not a git repository")); err != nil {
		t.Fatal(err)
	}
	if st := get[map[string]any](t, a, "status"); st["text"] != "git status: not a git repository" || st["err"] != true {
		t.Errorf("status = %v", st)
	}
	if s := screenOf(t, a, 100, 30); !strings.Contains(s, "git status: not a git repository") {
		t.Errorf("the error is not on screen:\n%s", s)
	}
	// a failing handler's error reaches the caller of Play
	boom := errors.New("boom")
	a.ui.On("stage_all", func(tuimark.Event) error { return boom })
	if _, err := a.ui.Play(tuimark.PlayOptions{Cols: 100, Rows: 30}, "a"); !errors.Is(err, boom) {
		t.Errorf("Play error = %v", err)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// j and k scroll the diff pane when it has focus (move-next and move-prev
// act on a viewport's offset).
func TestPlayScrollTheDiff(t *testing.T) {
	a := newApp(t)
	before := screenOf(t, a, 100, 12)
	if !strings.Contains(before, "diff --git a/README.md") {
		t.Fatalf("the diff does not start at its first line:\n%s", before)
	}
	res, err := a.ui.Play(tuimark.PlayOptions{Cols: 100, Rows: 12}, "tab", "j", "j", "j")
	if err != nil {
		t.Fatal(err)
	}
	if res.Dump.Focus == nil || *res.Dump.Focus != "diff" {
		t.Fatalf("focus = %v, want the diff pane", res.Dump.Focus)
	}
	if s := strings.Join(res.Dump.Grid, "\n"); strings.Contains(s, "diff --git a/README.md") || !strings.Contains(s, "@@ -12,7") {
		t.Errorf("j did not scroll the diff:\n%s", s)
	}
}
