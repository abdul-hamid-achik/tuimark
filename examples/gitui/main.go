// Command gitui runs examples/gitui/app.tui, a small git client in the
// spirit of lazygit and tig: Status (staged and unstaged files with a diff
// preview), Log (a filterable commit table with a commit detail), and
// Branches (with a checkout confirmation).
//
//	go build -o bin/gitui ./examples/gitui
//	./bin/gitui                       # the fake repository of sample.json
//	./bin/gitui -repo .               # a real repository, read-only
//	./bin/gitui -repo . -write        # ... that may also stage and check out
//	./bin/gitui -dump 120x30          # print Dump() as JSON and exit
//	./bin/gitui -theme light          # Set("@theme", "light") before Run
//
// The view lives entirely in app.tui and theme.tcss. This host holds no
// layout, width, padding, column, or truncation code: it keeps the
// repository's data in the store, derives the staged and unstaged lists,
// the counts, and the filtered log with Derive, and implements the named
// actions. By default everything comes from sample.json, so the app is
// deterministic; -repo PATH reads a repository by running git (read-only
// unless -write is also given) and refreshes it every few seconds.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/abdul-hamid-achik/tuimark"
)

const (
	maxDiffLines = 2000
	readOnlyMsg  = "read-only (pass -write)"
)

func main() {
	dir := flag.String("dir", "examples/gitui", "directory with app.tui, theme.tcss, sample.json")
	dumpSize := flag.String("dump", "", "print Dump() as JSON at COLSxROWS and exit")
	theme := flag.String("theme", "", "dark or light: Set(\"@theme\", …) before Run")
	repo := flag.String("repo", "", "read this git repository instead of sample.json (read-only unless -write)")
	write := flag.Bool("write", false, "with -repo: let stage, unstage, and checkout run git")
	refresh := flag.Duration("refresh", 3*time.Second, "with -repo: how often to re-read git (0 disables)")
	flag.Parse()
	if err := run(options{dir: *dir, dump: *dumpSize, theme: *theme, repo: *repo, write: *write, refresh: *refresh}); err != nil {
		fmt.Fprintln(os.Stderr, "gitui:", err)
		os.Exit(1)
	}
}

type options struct {
	dir, dump, theme, repo string
	write                  bool
	refresh                time.Duration
}

// setter is what App and Batch share: the write half of the store.
type setter interface {
	Set(path string, v any) error
}

// app is the host's state: the app, where its data comes from, and the file
// the diff pane shows.
type app struct {
	ui   *tuimark.App
	be   backend
	mode string

	mu   sync.Mutex
	last struct{ list, path string }
}

func run(o options) error {
	a, err := load(o.dir)
	if err != nil {
		return err
	}
	if o.repo != "" {
		if err := a.useRepo(o.repo, o.write); err != nil {
			return err
		}
	} else if o.write {
		return fmt.Errorf("-write needs -repo")
	}
	a.register()
	if o.theme != "" {
		if o.theme != "dark" && o.theme != "light" {
			return fmt.Errorf("-theme wants dark or light, got %q", o.theme)
		}
		if err := a.ui.Set("@theme", o.theme); err != nil {
			return err
		}
	}
	if o.dump != "" {
		var cols, rows int
		if _, err := fmt.Sscanf(o.dump, "%dx%d", &cols, &rows); err != nil {
			return fmt.Errorf("-dump wants COLSxROWS, got %q", o.dump)
		}
		d, err := a.ui.Dump(cols, rows)
		if err != nil {
			return err
		}
		b, _ := json.MarshalIndent(d, "", "  ")
		fmt.Println(string(b))
		return nil
	}
	if o.repo != "" && o.refresh > 0 {
		stop := a.ui.Every(o.refresh, func(b *tuimark.Batch) error { return a.reload(b) })
		defer stop()
	}
	return a.ui.RunWith(context.Background(), os.Stdout, tuimark.RunOptions{OnError: a.onError})
}

// onError shows a failed action (a git error, say) in the status line and
// keeps the session going.
func (a *app) onError(ev *tuimark.Event, err error) error {
	_ = a.setStatus(a.ui, err.Error(), true)
	return nil
}

// load loads app.tui from dir, binds sample.json, and builds the sample
// backend from the same file.
func load(dir string) (*app, error) {
	ui, err := tuimark.Load(filepath.Join(dir, "app.tui"))
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
	delete(data, "sim") // read by the sample backend only
	if err := ui.Bind("", data); err != nil {
		return nil, err
	}
	be, err := newSampleRepo(raw)
	if err != nil {
		return nil, err
	}
	a := &app{ui: ui, be: be, mode: "sample"}
	a.last.list, a.last.path = "unstaged", "README.md"
	return a, nil
}

// newSampleRepo builds the sample backend from sample.json's raw state and
// its `sim` member.
func newSampleRepo(raw []byte) (*sampleRepo, error) {
	var s struct {
		Repo     repoInfo  `json:"repo"`
		Files    []file    `json:"files"`
		Log      []commit  `json:"log"`
		Branches []branch  `json:"branches"`
		Sim      sampleSim `json:"sim"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &sampleRepo{
		repo: s.Repo, files: s.Files, log: s.Log, branches: s.Branches,
		diffs: s.Sim.Diffs, commits: s.Sim.Commits,
	}, nil
}

// useRepo switches the app from the sample to the git repository at path.
func (a *app) useRepo(path string, write bool) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	g := newGitRepo(abs, write)
	snap, err := g.Snapshot()
	if err != nil {
		return err
	}
	a.be = g
	a.mode = "read-only"
	if write {
		a.mode = "write"
	}
	cur := ""
	for _, b := range snap.Branches {
		if b.Current {
			cur = b.Name
		}
	}
	// show the first changed file, unstaged ones first, as the first focus
	// event would
	list, first := "unstaged", unstagedRows(snap.Files)
	if len(first) == 0 {
		list, first = "staged", stagedRows(snap.Files)
	}
	cursors := map[string]any{"staged": "", "unstaged": "", "log": "", "branch": cur}
	view := diffView{Title: "no file selected", Lines: []diffLine{}}
	if len(first) > 0 {
		f, _ := findFile(snap.Files, first[0].Path)
		if view, err = a.diffFor(list, f); err != nil {
			return err
		}
		cursors[list] = f.Path
		a.last.list, a.last.path = list, f.Path
	}
	return a.ui.Batch(func(b *tuimark.Batch) error {
		publish(b, snap)
		return firstErr(
			b.Set("mode", a.mode),
			b.Set("cur", cursors),
			b.Set("diff", view),
			b.Set("detail", detailView{Body: []para{}, Lines: []diffLine{}}),
			b.Set("log_query", ""),
			b.Set("status", map[string]any{"text": fmt.Sprintf("%s · %s", abs, a.mode), "err": false}),
		)
	})
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

// publish writes a snapshot's raw data. The staged and unstaged lists, the
// counts, the sync label, and the filtered log follow by Derive.
func publish(s setter, snap snapshot) {
	_ = s.Set("repo", snap.Repo)
	_ = s.Set("files", snap.Files)
	_ = s.Set("log", snap.Log)
	_ = s.Set("branches", snap.Branches)
}

func (a *app) setStatus(s setter, text string, isErr bool) error {
	return s.Set("status", map[string]any{"text": clean(text), "err": isErr})
}

// register installs the Derives and the handlers.
func (a *app) register() {
	for name, h := range a.handlers() {
		a.ui.On(name, h)
	}
	a.derive()
}

// read decodes the store value at path into T through JSON.
func read[T any](get func(string) (any, bool), path string) (T, bool) {
	var v T
	raw, ok := get(path)
	if !ok {
		return v, false
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return v, false
	}
	return v, json.Unmarshal(b, &v) == nil
}

// derive declares the computed values the markup reads, so it needs no
// expressions: the staged and unstaged lists, their counts, the branch's
// sync label, and the log filtered by whatever the filter input holds.
func (a *app) derive() {
	type getter = func(string) (any, bool)
	must := func(path string, fn func(get getter) (any, error)) {
		if err := a.ui.Derive(path, fn); err != nil {
			panic(err) // a programming error: the paths are fixed
		}
	}
	must("sync_label", func(get getter) (any, error) {
		r, _ := read[repoInfo](get, "repo")
		return syncLabel(r), nil
	})
	must("staged", func(get getter) (any, error) {
		files, _ := read[[]file](get, "files")
		return stagedRows(files), nil
	})
	must("unstaged", func(get getter) (any, error) {
		files, _ := read[[]file](get, "files")
		return unstagedRows(files), nil
	})
	must("counts", func(get getter) (any, error) {
		files, _ := read[[]file](get, "files")
		return countFiles(files), nil
	})
	must("log_view", func(get getter) (any, error) {
		log, _ := read[[]commit](get, "log")
		q, _ := read[string](get, "log_query")
		return filterLog(log, q), nil
	})
	must("log_label", func(get getter) (any, error) {
		log, _ := read[[]commit](get, "log")
		view, _ := read[[]commit](get, "log_view")
		q, _ := read[string](get, "log_query")
		return logLabel(len(log), len(view), q), nil
	})
}

// handlers are the named actions of app.tui.
func (a *app) handlers() map[string]tuimark.Handler {
	ui := a.ui
	hs := map[string]tuimark.Handler{}
	on := func(name string, h tuimark.Handler) { hs[name] = h }
	on("quit", func(tuimark.Event) error { return tuimark.ErrQuit })
	on("help_open", func(tuimark.Event) error { return ui.Set("help_open", true) })
	on("help_close", func(tuimark.Event) error { return ui.Set("help_open", false) })
	on("refresh", func(tuimark.Event) error {
		if err := a.reload(ui); err != nil {
			return err
		}
		return a.setStatus(ui, "refreshed", false)
	})

	// Status tab
	on("file_select", func(ev tuimark.Event) error {
		list := ev.Source
		if list != "staged" && list != "unstaged" {
			return nil
		}
		path, _ := ev.Keys["f"].(string)
		if path == "" {
			path = a.cursorPath(list)
		}
		return a.showFile(ui, list, path)
	})
	on("stage_toggle", func(ev tuimark.Event) error {
		list := ev.Source
		if list != "staged" && list != "unstaged" {
			return nil
		}
		if !a.be.Writable() {
			return a.setStatus(ui, readOnlyMsg, false)
		}
		path, _ := ev.Keys["f"].(string)
		if path == "" {
			path = a.cursorPath(list)
		}
		f, ok := a.fileAt(path)
		if !ok {
			return nil
		}
		idx := indexOf(a.rowsOf(list), path)
		verb := "staged"
		var err error
		if list == "staged" {
			verb, err = "unstaged", a.be.Unstage(f)
		} else {
			err = a.be.Stage(f)
		}
		if err != nil {
			return err
		}
		return a.afterChange(list, idx, path, verb+" "+path)
	})
	on("stage_all", func(tuimark.Event) error {
		if !a.be.Writable() {
			return a.setStatus(ui, readOnlyMsg, false)
		}
		if err := a.be.StageAll(); err != nil {
			return err
		}
		return a.afterChange("unstaged", 0, "", "staged everything")
	})
	on("unstage_all", func(tuimark.Event) error {
		if !a.be.Writable() {
			return a.setStatus(ui, readOnlyMsg, false)
		}
		if err := a.be.UnstageAll(); err != nil {
			return err
		}
		return a.afterChange("staged", 0, "", "unstaged everything")
	})

	// Log tab
	on("commit_open", func(ev tuimark.Event) error {
		hash, _ := ev.Keys["c"].(string)
		if hash == "" {
			hash, _ = read[string](ui.Get, "cur.log")
		}
		if hash == "" {
			return nil
		}
		raw, err := a.be.Commit(hash)
		if err != nil {
			return err
		}
		return ui.Batch(func(b *tuimark.Batch) error {
			return firstErr(b.Set("detail", buildDetail(hash, raw, maxDiffLines)), b.Set("detail_open", true))
		})
	})
	on("commit_close", func(tuimark.Event) error { return ui.Set("detail_open", false) })
	on("filter_open", func(tuimark.Event) error {
		return firstErr(ui.Set("filter_open", true), ui.Set("@focus", "#filter"))
	})
	on("filter_apply", func(tuimark.Event) error { return ui.Set("@focus", "#commits") })
	on("filter_cancel", func(tuimark.Event) error {
		open, _ := read[bool](ui.Get, "filter_open")
		if !open {
			return nil
		}
		return firstErr(ui.Set("log_query", ""), ui.Set("filter_open", false), ui.Set("@focus", "#commits"))
	})

	// Branches tab
	on("checkout_ask", func(ev tuimark.Event) error {
		name, _ := ev.Keys["b"].(string)
		if name == "" {
			name, _ = read[string](ui.Get, "cur.branch")
		}
		branches, _ := read[[]branch](ui.Get, "branches")
		var target *branch
		for i := range branches {
			if branches[i].Name == name {
				target = &branches[i]
			}
		}
		switch {
		case target == nil:
			return nil
		case target.Current:
			return a.setStatus(ui, "already on "+name, false)
		case !a.be.Writable():
			return a.setStatus(ui, readOnlyMsg, false)
		}
		c, _ := read[counts](ui.Get, "counts")
		note := "The working tree is clean."
		if c.Total > 0 {
			note = fmt.Sprintf("%s changed in the working tree; git refuses the switch if they conflict.", plural(c.Total, "file"))
		}
		return ui.Batch(func(b *tuimark.Batch) error {
			return firstErr(b.Set("confirm", map[string]any{"branch": name, "note": note}), b.Set("confirm_open", true))
		})
	})
	on("checkout_cancel", func(tuimark.Event) error { return ui.Set("confirm_open", false) })
	on("checkout_confirm", func(tuimark.Event) error {
		name, _ := read[string](ui.Get, "confirm.branch")
		if err := ui.Set("confirm_open", false); err != nil {
			return err
		}
		if err := a.be.Checkout(name); err != nil {
			return err
		}
		if err := a.reload(ui); err != nil {
			return err
		}
		return a.setStatus(ui, "switched to "+name, false)
	})
	return hs
}

func hasRow(rows []fileRow, path string) bool {
	for _, r := range rows {
		if r.Path == path {
			return true
		}
	}
	return false
}

func indexOf(rows []fileRow, path string) int {
	for i, r := range rows {
		if r.Path == path {
			return i
		}
	}
	return 0
}

// rowsOf are the rows of the staged or unstaged list, as the store holds
// them (derived).
func (a *app) rowsOf(list string) []fileRow {
	rows, _ := read[[]fileRow](a.ui.Get, list)
	return rows
}

// cursorPath is the path under the cursor of a files list: the bound
// cursor if the list still has it, else the first row.
func (a *app) cursorPath(list string) string {
	rows := a.rowsOf(list)
	p, _ := read[string](a.ui.Get, "cur."+list)
	for _, r := range rows {
		if r.Path == p {
			return p
		}
	}
	if len(rows) > 0 {
		return rows[0].Path
	}
	return ""
}

func (a *app) fileAt(path string) (file, bool) {
	files, _ := read[[]file](a.ui.Get, "files")
	return findFile(files, path)
}

func findFile(files []file, path string) (file, bool) {
	for _, f := range files {
		if f.Path == path {
			return f, true
		}
	}
	return file{}, false
}

// diffFor builds the preview of one entry of a files list.
func (a *app) diffFor(list string, f file) (diffView, error) {
	title := f.Path + " (" + list + ")"
	if list == "staged" && f.Orig != "" {
		title = f.Orig + " → " + f.Path + " (staged)"
	}
	if strings.HasSuffix(f.Path, "/") {
		return diffView{Title: clean(title), Lines: []diffLine{{Text: "untracked directory", Meta: true}}}, nil
	}
	text, err := a.be.FileDiff(f, list == "staged")
	if err != nil {
		return diffView{}, err
	}
	return diffView{Title: clean(title), Lines: diffLines(text, maxDiffLines)}, nil
}

// showFile puts the diff of path in the preview pane.
func (a *app) showFile(s setter, list, path string) error {
	f, ok := a.fileAt(path)
	if !ok {
		a.mu.Lock()
		a.last.list, a.last.path = "", ""
		a.mu.Unlock()
		return s.Set("diff", diffView{Title: "no file selected", Lines: []diffLine{}})
	}
	v, err := a.diffFor(list, f)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.last.list, a.last.path = list, path
	a.mu.Unlock()
	return s.Set("diff", v)
}

// afterChange re-reads the repository after a stage or unstage, moves the
// cursor of the list the user acted in (list, at row idx) to the row that
// took the place of the one that left, focusing the other list when this
// one is now empty, and reports what was done. moved, if set, is the path
// that changed sides: the other list's cursor goes to it, so tab lands on
// the file just staged or unstaged.
func (a *app) afterChange(list string, idx int, moved, msg string) error {
	ui := a.ui
	if err := a.reload(ui); err != nil {
		return err
	}
	if moved != "" {
		other := "staged"
		if list == "staged" {
			other = "unstaged"
		}
		if hasRow(a.rowsOf(other), moved) {
			if err := ui.Set("cur."+other, moved); err != nil {
				return err
			}
		}
	}
	rows := a.rowsOf(list)
	if len(rows) == 0 {
		other := "staged"
		if list == "staged" {
			other = "unstaged"
		}
		if len(a.rowsOf(other)) > 0 {
			list, idx = other, 0
			rows = a.rowsOf(list)
			if err := ui.Set("@focus", "#"+list); err != nil {
				return err
			}
		}
	}
	if len(rows) == 0 {
		if err := a.showFile(ui, list, ""); err != nil {
			return err
		}
		return a.setStatus(ui, msg, false)
	}
	if idx >= len(rows) {
		idx = len(rows) - 1
	}
	if err := ui.Set("cur."+list, rows[idx].Path); err != nil {
		return err
	}
	if err := a.showFile(ui, list, rows[idx].Path); err != nil {
		return err
	}
	return a.setStatus(ui, msg, false)
}

// reload re-reads the repository into s (the app, or an Every batch) and
// refreshes the diff of the file being shown, if it is still listed.
func (a *app) reload(s setter) error {
	snap, err := a.be.Snapshot()
	if err != nil {
		return err
	}
	publish(s, snap)
	a.mu.Lock()
	list, path := a.last.list, a.last.path
	a.mu.Unlock()
	if path == "" {
		return nil
	}
	var rows []fileRow
	if list == "staged" {
		rows = stagedRows(snap.Files)
	} else {
		rows = unstagedRows(snap.Files)
	}
	f, ok := findFile(snap.Files, path)
	if !ok || !hasRow(rows, path) {
		a.mu.Lock()
		a.last.list, a.last.path = "", ""
		a.mu.Unlock()
		return s.Set("diff", diffView{Title: "no file selected", Lines: []diffLine{}})
	}
	v, err := a.diffFor(list, f)
	if err != nil {
		return err
	}
	return s.Set("diff", v)
}
