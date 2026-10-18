package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abdul-hamid-achik/tuimark"
)

// tempRepo builds a small repository in t.TempDir(): two commits on main,
// then a staged rename (b.txt to d.txt), an unstaged edit (a.txt), and an
// untracked file (c.txt). It skips the test when git is not installed.
// Nothing here touches the repository the tests run in: every git command
// runs with -C on the temp directory and an isolated configuration.
func tempRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	git := func(args ...string) string {
		t.Helper()
		full := append([]string{"-C", dir, "-c", "user.name=Test Author", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)
		cmd := exec.Command("git", full...)
		cmd.Env = gitEnv()
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	write := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	git("symbolic-ref", "HEAD", "refs/heads/main")
	write("a.txt", "one\ntwo\nthree\n")
	write("b.txt", "bee\nbee\nbee\nbee\n")
	git("add", ".")
	git("commit", "-q", "-m", "first commit")
	write("a.txt", "one\ntwo\nthree\nfour\n")
	git("commit", "-q", "-am", "second commit\n\nWith a body paragraph.")
	git("branch", "topic")
	git("mv", "b.txt", "d.txt")
	write("a.txt", "ONE\ntwo\nthree\nfour\n")
	write("c.txt", "new\n")
	return dir
}

func TestGitRepoSnapshot(t *testing.T) {
	g := newGitRepo(tempRepo(t), false)
	snap, err := g.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Repo.Branch != "main" || snap.Repo.Name == "" || snap.Repo.Upstream != "" {
		t.Errorf("repo = %+v", snap.Repo)
	}
	byPath := map[string]file{}
	for _, f := range snap.Files {
		byPath[f.Path] = f
	}
	if f := byPath["a.txt"]; f.X != " " || f.Y != "M" {
		t.Errorf("a.txt = %+v", f)
	}
	if f := byPath["c.txt"]; f.X != "?" || f.Y != "?" {
		t.Errorf("c.txt = %+v", f)
	}
	if f := byPath["d.txt"]; f.X != "R" || f.Orig != "b.txt" {
		t.Errorf("d.txt = %+v", f)
	}
	if len(snap.Log) != 2 || snap.Log[0].Subject != "second commit" || snap.Log[1].Subject != "first commit" ||
		snap.Log[0].Author != "Test Author" || !snap.Log[0].Head || !strings.Contains(snap.Log[0].Refs, "main") || snap.Log[0].Date == "" {
		t.Errorf("log = %+v", snap.Log)
	}
	names := []string{}
	for _, b := range snap.Branches {
		names = append(names, b.Name)
		if b.Name == "main" && (!b.Current || b.Mark != "●") {
			t.Errorf("main = %+v", b)
		}
	}
	if strings.Join(names, ",") != "main,topic" {
		t.Errorf("branches = %v", names)
	}
}

func TestGitRepoDiffsAndCommit(t *testing.T) {
	g := newGitRepo(tempRepo(t), false)
	un, err := g.FileDiff(file{Path: "a.txt", X: " ", Y: "M"}, false)
	if err != nil || !strings.Contains(un, "-one") || !strings.Contains(un, "+ONE") {
		t.Errorf("unstaged diff = %q, %v", un, err)
	}
	untracked, err := g.FileDiff(file{Path: "c.txt", X: "?", Y: "?"}, false)
	if err != nil || !strings.Contains(untracked, "+new") {
		t.Errorf("untracked diff = %q, %v", untracked, err)
	}
	staged, err := g.FileDiff(file{Path: "d.txt", Orig: "b.txt", X: "R", Y: " "}, true)
	if err != nil || !strings.Contains(staged, "rename from b.txt") {
		t.Errorf("staged diff = %q, %v", staged, err)
	}
	snap, _ := g.Snapshot()
	raw, err := g.Commit(snap.Log[0].Hash)
	if err != nil {
		t.Fatal(err)
	}
	if raw.Author != "Test Author" || raw.Email != "test@example.com" || !strings.HasPrefix(raw.Message, "second commit") || !strings.Contains(raw.Diff, "+four") {
		t.Errorf("commit = %+v", raw)
	}
	if _, err := g.Commit("nope"); err == nil || !strings.HasPrefix(err.Error(), "git show:") {
		t.Errorf("unknown commit error = %v", err)
	}
	if _, err := newGitRepo(t.TempDir(), false).Snapshot(); err == nil || !strings.HasPrefix(err.Error(), "git status:") {
		t.Errorf("not a repository error = %v", err)
	}
}

func realApp(t *testing.T, dir string, write bool) *app {
	t.Helper()
	a, err := load(".")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.useRepo(dir, write); err != nil {
		t.Fatal(err)
	}
	a.register()
	return a
}

// Without -write, stage, unstage, and checkout touch nothing and say so.
func TestRealRepoIsReadOnlyByDefault(t *testing.T) {
	dir := tempRepo(t)
	a := realApp(t, dir, false)
	if diags := a.ui.Validate(); len(diags) != 0 {
		t.Errorf("Validate = %v", diags)
	}
	before, _ := newGitRepo(dir, false).run("status", "--porcelain=v1")
	res := play(t, a, "space")
	if got := actions(res); len(got) != 2 || got[1] != "stage_toggle" {
		t.Errorf("events = %v", got)
	}
	if st := get[map[string]any](t, a, "status"); st["text"] != readOnlyMsg {
		t.Errorf("status = %v", st)
	}
	play(t, a, "a")
	play(t, a, "3", "down", "enter")
	if get[bool](t, a, "confirm_open") {
		t.Errorf("the checkout confirmation opened in read-only mode")
	}
	if st := get[map[string]any](t, a, "status"); st["text"] != readOnlyMsg {
		t.Errorf("status = %v", st)
	}
	if after, _ := newGitRepo(dir, false).run("status", "--porcelain=v1"); after != before {
		t.Errorf("read-only mode changed the repository:\n%s\nvs\n%s", before, after)
	}
	if get[string](t, a, "mode") != "read-only" {
		t.Errorf("mode = %q", get[string](t, a, "mode"))
	}
	// reads still work: the first unstaged file's diff is on screen, and
	// the log tab shows the real commits
	if s := screenOf(t, a, 100, 30, "1"); !strings.Contains(s, "a.txt (unstaged)") || !strings.Contains(s, "+ONE") {
		t.Errorf("no diff of a.txt:\n%s", s)
	}
	if s := screenOf(t, a, 100, 30, "2"); !strings.Contains(s, "second commit") || !strings.Contains(s, "Test Author") {
		t.Errorf("no log:\n%s", s)
	}
}

func TestRealRepoWriteStagesAndChecksOut(t *testing.T) {
	dir := tempRepo(t)
	a := realApp(t, dir, true)
	res := play(t, a, "space")
	if e := res.Events[len(res.Events)-1]; e.Action != "stage_toggle" || e.Keys["f"] != "a.txt" {
		t.Errorf("event = %+v", e)
	}
	out, _ := newGitRepo(dir, false).run("status", "--porcelain=v1")
	if !strings.Contains(out, "M  a.txt") {
		t.Errorf("a.txt is not staged:\n%s", out)
	}
	if got := paths(get[[]fileRow](t, a, "staged")); !contains(got, "a.txt") {
		t.Errorf("staged = %v", got)
	}
	// stage everything, then unstage everything
	play(t, a, "a")
	if c := get[counts](t, a, "counts"); c.Unstaged != 0 || c.Staged != 3 {
		t.Errorf("counts after stage all = %+v", c)
	}
	play(t, a, "A")
	if c := get[counts](t, a, "counts"); c.Staged != 0 {
		t.Errorf("counts after unstage all = %+v", c)
	}
	// checkout topic (the working tree has changes that do not conflict)
	play(t, a, "3", "down", "enter", "y")
	if r := get[repoInfo](t, a, "repo"); r.Branch != "topic" {
		t.Errorf("repo = %+v", r)
	}
	if head, _ := newGitRepo(dir, false).run("rev-parse", "--abbrev-ref", "HEAD"); strings.TrimSpace(head) != "topic" {
		t.Errorf("HEAD = %q", head)
	}
	// commit detail from real git
	res = play(t, a, "2", "enter")
	s := strings.Join(res.Dump.Grid, "\n")
	if !strings.Contains(s, "second commit") || !strings.Contains(s, "With a body paragraph.") || !strings.Contains(s, "Test Author <test@example.com>") {
		t.Errorf("commit detail:\n%s", s)
	}
}

// A failing git command surfaces as the handler's error, which RunWith
// hands to onError and the status line shows.
func TestRealRepoErrorsReachTheStatusLine(t *testing.T) {
	dir := tempRepo(t)
	a := realApp(t, dir, true)
	err := a.be.Checkout("no-such-branch")
	if err == nil || !strings.HasPrefix(err.Error(), "git checkout:") {
		t.Fatalf("Checkout error = %v", err)
	}
	_ = a.onError(nil, err)
	if st := get[map[string]any](t, a, "status"); st["err"] != true || !strings.Contains(st["text"].(string), "git checkout") {
		t.Errorf("status = %v", st)
	}
}

func TestNewAppRejectsWriteWithoutRepo(t *testing.T) {
	if err := run(options{dir: ".", write: true, dump: "80x24"}); err == nil || !strings.Contains(err.Error(), "-write needs -repo") {
		t.Errorf("run error = %v", err)
	}
}

// ---- the sample backend

func TestSampleRepoBehavesLikeGit(t *testing.T) {
	a, err := load(".")
	if err != nil {
		t.Fatal(err)
	}
	be := a.be.(*sampleRepo)
	find := func(path string) file {
		snap, _ := be.Snapshot()
		f, ok := findFile(snap.Files, path)
		if !ok {
			t.Fatalf("%s is gone", path)
		}
		return f
	}
	// stage a modification, then an untracked file
	if err := be.Stage(find("README.md")); err != nil {
		t.Fatal(err)
	}
	if f := find("README.md"); f.X != "M" || f.Y != " " {
		t.Errorf("README.md = %+v", f)
	}
	staged, _ := be.FileDiff(find("README.md"), true)
	if !strings.Contains(staged, "Quick start") {
		t.Errorf("staged diff = %q", staged)
	}
	if un, _ := be.FileDiff(find("README.md"), false); un != "" {
		t.Errorf("unstaged diff should be empty, got %q", un)
	}
	be.Stage(find("internal/queue/backoff_test.go"))
	if f := find("internal/queue/backoff_test.go"); f.X != "A" || f.Y != " " {
		t.Errorf("backoff_test.go = %+v", f)
	}
	// unstage them back
	be.Unstage(find("README.md"))
	be.Unstage(find("internal/queue/backoff_test.go"))
	if f := find("README.md"); f.X != " " || f.Y != "M" {
		t.Errorf("README.md = %+v", f)
	}
	if f := find("internal/queue/backoff_test.go"); f.X != "?" || f.Y != "?" {
		t.Errorf("backoff_test.go = %+v", f)
	}
	// unstaging a rename gives a deletion plus an untracked file, in order
	be.Unstage(find("internal/cfg/load.go"))
	snap, _ := be.Snapshot()
	var order []string
	for _, f := range snap.Files {
		order = append(order, f.Path)
	}
	if f := find("internal/cfg/config.go"); f.X != " " || f.Y != "D" {
		t.Errorf("config.go = %+v", f)
	}
	for i := 1; i < len(order); i++ {
		if order[i-1] > order[i] {
			t.Errorf("files are not sorted: %v", order)
		}
	}
	// staging an unknown path or an already staged file changes nothing
	n := len(snap.Files)
	be.Stage(file{Path: "nope"})
	be.Unstage(find("scripts/release-old.sh"))
	be.Stage(find("internal/queue/worker.go"))
	if snap2, _ := be.Snapshot(); len(snap2.Files) != n {
		t.Errorf("files changed: %d vs %d", len(snap2.Files), n)
	}
	// checkout
	if err := be.Checkout("release/0.4"); err != nil {
		t.Fatal(err)
	}
	snap, _ = be.Snapshot()
	if snap.Repo.Branch != "release/0.4" || snap.Repo.Behind != 3 || snap.Repo.Ahead != 0 {
		t.Errorf("repo = %+v", snap.Repo)
	}
	if err := be.Checkout("nope"); err == nil {
		t.Errorf("checkout of an unknown branch should fail")
	}
	if _, err := be.Commit("nope"); err == nil {
		t.Errorf("unknown commit should fail")
	}
}

// Dump and Play never tick, so Every needs no test of its own, but the
// reload that Every runs does: it re-reads the backend and refreshes the
// diff being shown.
func TestReloadRefreshesTheShownDiff(t *testing.T) {
	a := newApp(t)
	play(t, a) // the first file_select shows README.md
	be := a.be.(*sampleRepo)
	be.mu.Lock()
	d := be.diffs["README.md"]
	d.Unstaged += "+extra line\n"
	be.diffs["README.md"] = d
	be.mu.Unlock()
	err := a.ui.Batch(func(b *tuimark.Batch) error { return a.reload(b) })
	if err != nil {
		t.Fatal(err)
	}
	lines := get[diffView](t, a, "diff").Lines
	if last := lines[len(lines)-1]; last.Text != "+extra line" || !last.Add {
		t.Errorf("last line = %+v", last)
	}
}
