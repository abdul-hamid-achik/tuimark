package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseStatus(t *testing.T) {
	out := strings.Join([]string{
		"## main...origin/main [ahead 2, behind 1]",
		" M README.md",
		"M  internal/queue/worker.go",
		"MM cmd/main.go",
		"A  docs/new.md",
		"D  old.sh",
		" D gone.json",
		"R  internal/cfg/config.go -> internal/cfg/load.go",
		"?? notes.txt",
		"!! build/",
		`?? "with space.txt"`,
		`R  "a b.go" -> "c d.go"`,
	}, "\n") + "\n"
	repo, files := parseStatus(out)
	if repo.Branch != "main" || repo.Upstream != "origin/main" || repo.Ahead != 2 || repo.Behind != 1 {
		t.Errorf("repo = %+v", repo)
	}
	want := []file{
		{Path: "README.md", X: " ", Y: "M"},
		{Path: "internal/queue/worker.go", X: "M", Y: " "},
		{Path: "cmd/main.go", X: "M", Y: "M"},
		{Path: "docs/new.md", X: "A", Y: " "},
		{Path: "old.sh", X: "D", Y: " "},
		{Path: "gone.json", X: " ", Y: "D"},
		{Path: "internal/cfg/load.go", Orig: "internal/cfg/config.go", X: "R", Y: " "},
		{Path: "notes.txt", X: "?", Y: "?"},
		{Path: "with space.txt", X: "?", Y: "?"},
		{Path: "c d.go", Orig: "a b.go", X: "R", Y: " "},
	}
	if !reflect.DeepEqual(files, want) {
		t.Errorf("files =\n%+v\nwant\n%+v", files, want)
	}
}

func TestParseBranchHeader(t *testing.T) {
	cases := []struct {
		in       string
		name, up string
		ahead    int
		behind   int
	}{
		{"main...origin/main", "main", "origin/main", 0, 0},
		{"main...origin/main [ahead 3]", "main", "origin/main", 3, 0},
		{"main...origin/main [behind 4]", "main", "origin/main", 0, 4},
		{"feat/x...origin/feat/x [ahead 1, behind 2]", "feat/x", "origin/feat/x", 1, 2},
		{"main...origin/main [gone]", "main", "origin/main", 0, 0},
		{"topic", "topic", "", 0, 0},
		{"No commits yet on main", "main", "", 0, 0},
		{"Initial commit on trunk", "trunk", "", 0, 0},
		{"HEAD (no branch)", "HEAD (detached)", "", 0, 0},
	}
	for _, c := range cases {
		name, up, ahead, behind := parseBranchHeader(c.in)
		if name != c.name || up != c.up || ahead != c.ahead || behind != c.behind {
			t.Errorf("%q = %q %q %d %d, want %q %q %d %d", c.in, name, up, ahead, behind, c.name, c.up, c.ahead, c.behind)
		}
	}
}

func TestParseLog(t *testing.T) {
	out := "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111\x1faaaa111\x1fPriya Raman\x1f2 hours ago\x1fHEAD -> main, origin/main, tag: v1.0\x1fqueue: retry\n" +
		"bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222\x1fbbbb222\x1fMarcus Webb\x1f3 days ago\x1f\x1ffix: a\x1fb\n" +
		"\n" +
		"broken line without separators\n"
	log := parseLog(out)
	if len(log) != 2 {
		t.Fatalf("parsed %d commits: %+v", len(log), log)
	}
	want0 := commit{Hash: "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111", Short: "aaaa111", Subject: "queue: retry", Author: "Priya Raman", Date: "2h", Refs: "main, origin/main, v1.0", Head: true}
	if log[0] != want0 {
		t.Errorf("log[0] = %+v, want %+v", log[0], want0)
	}
	if log[1].Date != "3d" || log[1].Refs != "" || log[1].Head || log[1].Subject != "fix: a b" {
		t.Errorf("log[1] = %+v", log[1])
	}
}

func TestParseBranches(t *testing.T) {
	out := "*\x1fmain\x1forigin/main\x1fahead 2, behind 1\x1f82556a8\x1fqueue: retry\x1f2 hours ago\n" +
		" \x1ffeature/x\x1f\x1f\x1f18d8996\x1fdocs: x\x1f3 weeks ago\n" +
		" \x1fold\x1forigin/old\x1fgone\x1f9fea534\x1finitial import\x1f1 year, 2 months ago\n"
	bs := parseBranches(out)
	want := []branch{
		{Name: "main", Mark: "●", Current: true, Upstream: "origin/main", Track: "↑2 ↓1", Hash: "82556a8", Subject: "queue: retry", Date: "2h"},
		{Name: "feature/x", Hash: "18d8996", Subject: "docs: x", Date: "3w"},
		{Name: "old", Upstream: "origin/old", Track: "gone", Hash: "9fea534", Subject: "initial import", Date: "1y"},
	}
	want[1].Name = "feature/x"
	if !reflect.DeepEqual(bs, want) {
		t.Errorf("branches =\n%+v\nwant\n%+v", bs, want)
	}
}

func TestFormatRefs(t *testing.T) {
	cases := []struct {
		in   string
		out  string
		head bool
	}{
		{"", "", false},
		{"HEAD -> main", "main", true},
		{"HEAD -> main, origin/main, origin/HEAD", "main, origin/main", true},
		{"tag: v0.4.0, origin/release", "v0.4.0, origin/release", false},
		{"HEAD", "HEAD", true},
	}
	for _, c := range cases {
		out, head := formatRefs(c.in)
		if out != c.out || head != c.head {
			t.Errorf("formatRefs(%q) = %q, %v; want %q, %v", c.in, out, head, c.out, c.head)
		}
	}
}

func TestShortAgo(t *testing.T) {
	for in, want := range map[string]string{
		"45 seconds ago":       "45s",
		"5 minutes ago":        "5m",
		"1 hour ago":           "1h",
		"3 days ago":           "3d",
		"2 weeks ago":          "2w",
		"1 month ago":          "1mo",
		"1 year, 2 months ago": "1y",
		"yesterday":            "yesterday",
	} {
		if got := shortAgo(in); got != want {
			t.Errorf("shortAgo(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTrackHelpers(t *testing.T) {
	a, b, g := parseTrack("ahead 2, behind 1")
	if a != 2 || b != 1 || g {
		t.Errorf("parseTrack = %d %d %v", a, b, g)
	}
	if _, _, g := parseTrack("gone"); !g {
		t.Errorf("gone not parsed")
	}
	for _, c := range []struct {
		ahead, behind int
		gone          bool
		want          string
	}{{0, 0, false, ""}, {2, 0, false, "↑2"}, {0, 3, false, "↓3"}, {2, 1, false, "↑2 ↓1"}, {0, 0, true, "gone"}} {
		if got := trackArrows(c.ahead, c.behind, c.gone); got != c.want {
			t.Errorf("trackArrows(%d,%d,%v) = %q", c.ahead, c.behind, c.gone, got)
		}
	}
	if a, b := arrowCounts("↑2 ↓1"); a != 2 || b != 1 {
		t.Errorf("arrowCounts = %d %d", a, b)
	}
	if a, b := arrowCounts("↓3"); a != 0 || b != 3 {
		t.Errorf("arrowCounts = %d %d", a, b)
	}
}

const cannedDiff = `diff --git a/a.go b/a.go
index 111..222 100644
--- a/a.go
+++ b/a.go
@@ -1,4 +1,5 @@
 package a
-var x = 1
+var x = 2
+var y = 3
--- not a header, a removed comment
 func f() {}
\ No newline at end of file
diff --git a/b.txt b/b.txt
new file mode 100644
--- /dev/null
+++ b/b.txt
@@ -0,0 +1 @@
+hello
`

func TestDiffLines(t *testing.T) {
	lines := diffLines(cannedDiff, 100)
	type flags struct{ add, del, hunk, meta bool }
	var got []flags
	for i, l := range lines {
		if l.N != i {
			t.Errorf("line %d has N=%d", i, l.N)
		}
		got = append(got, flags{l.Add, l.Del, l.Hunk, l.Meta})
	}
	meta, add, del, hunk, ctx := flags{meta: true}, flags{add: true}, flags{del: true}, flags{hunk: true}, flags{}
	want := []flags{
		meta, meta, meta, meta, // diff --git, index, ---, +++
		hunk, ctx, del, add, add,
		del, // "--- not a header" inside a hunk is a removal
		ctx,
		meta,                   // "\ No newline at end of file"
		meta, meta, meta, meta, // second file's header
		hunk, add,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("flags =\n%v\nwant\n%v", got, want)
	}
	if files, adds, dels := diffStats(cannedDiff); files != 2 || adds != 3 || dels != 2 {
		t.Errorf("diffStats = %d %d %d", files, adds, dels)
	}
}

func TestDiffLinesTruncatesAndCleans(t *testing.T) {
	text := "@@ -1 +1 @@\n+a\n+b\n+c\n+d"
	lines := diffLines(text, 3)
	if len(lines) != 4 || !lines[3].Meta || lines[3].Text != "… 2 more lines not shown" {
		t.Errorf("lines = %+v", lines)
	}
	l := diffLines("@@ -1 +1 @@\n+\tx\x1b[31mred\x1b[0m\x00y\r\n", 10)
	if l[1].Text != "+    xredy" || !l[1].Add {
		t.Errorf("cleaned line = %+v", l[1])
	}
	if got := diffLines("", 10); got == nil || len(got) != 0 {
		t.Errorf("empty diff = %#v", got)
	}
}

func TestCleanAndTabs(t *testing.T) {
	if got := clean("a\x1b[1;32mb\x1b[0m\x07c\xffd"); got != "abc�d" {
		t.Errorf("clean = %q", got)
	}
	if got := expandTabs("\tx\ty"); got != "    x    y" {
		t.Errorf("expandTabs = %q", got)
	}
	if got := unquoteGit(`"a\tb"`); got != "a\tb" {
		t.Errorf("unquoteGit = %q", got)
	}
	if got := unquoteGit(`"\303\251.txt"`); got != "é.txt" {
		t.Errorf("unquoteGit octal = %q", got)
	}
}

func TestSplitMessageAndDetail(t *testing.T) {
	subject, paras := splitMessage("queue: retry\n\nFirst line\nsecond line.\n\nThird.\n")
	if subject != "queue: retry" || !reflect.DeepEqual(paras, []string{"First line second line.", "Third."}) {
		t.Errorf("split = %q %q", subject, paras)
	}
	if s, p := splitMessage("only a subject\n"); s != "only a subject" || len(p) != 0 {
		t.Errorf("split = %q %q", s, p)
	}
	d := buildDetail("0123456789abcdef", rawCommit{Author: "A B", Email: "a@b.c", Date: "2026-01-02 03:04", Message: "s\n\nbody", Diff: cannedDiff}, 100)
	if d.Title != "commit 0123456" || d.Meta != "A B <a@b.c> · 2026-01-02 03:04" || d.Subject != "s" ||
		len(d.Body) != 1 || d.Body[0].Text != "body" || d.Stats != "2 files changed, +3 −2" || len(d.Lines) == 0 {
		t.Errorf("detail = %+v", d)
	}
}

func TestMergeDiff(t *testing.T) {
	a := "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-1\n+2\n"
	b := "diff --git a/x b/x\nindex 1..2\n--- a/x\n+++ b/x\n@@ -5 +5 @@\n-5\n+6\n"
	want := a + "@@ -5 +5 @@\n-5\n+6\n"
	if got := mergeDiff(a, b); got != want {
		t.Errorf("mergeDiff =\n%q\nwant\n%q", got, want)
	}
	if mergeDiff("", b) != b || mergeDiff(a, "") != a {
		t.Errorf("mergeDiff with an empty side")
	}
}

// ---- the values Derive computes

func TestRowsAndCounts(t *testing.T) {
	files := []file{
		{Path: "a", X: " ", Y: "M"},
		{Path: "b", X: "M", Y: "M"},
		{Path: "c", X: "A", Y: " "},
		{Path: "d", X: "?", Y: "?"},
		{Path: "e", Orig: "old", X: "R", Y: " "},
		{Path: "f", X: "D", Y: " "},
	}
	if got := paths(stagedRows(files)); !reflect.DeepEqual(got, []string{"b", "c", "e", "f"}) {
		t.Errorf("staged = %v", got)
	}
	if got := paths(unstagedRows(files)); !reflect.DeepEqual(got, []string{"a", "b", "d"}) {
		t.Errorf("unstaged = %v", got)
	}
	r := stagedRows(files)[2]
	if r.Code != "R" || !r.IsR || r.IsM || r.Orig != "old" {
		t.Errorf("row = %+v", r)
	}
	if u := unstagedRows(files)[2]; u.Code != "?" || !u.IsU {
		t.Errorf("untracked row = %+v", u)
	}
	if c := countFiles(files); c != (counts{Staged: 4, Unstaged: 3, Total: 6, Label: "4 staged · 3 unstaged"}) {
		t.Errorf("counts = %+v", c)
	}
	if rows := stagedRows(nil); rows == nil {
		t.Errorf("no rows must be an empty list, not null")
	}
}

func TestSyncLabel(t *testing.T) {
	for _, c := range []struct {
		r    repoInfo
		want string
	}{
		{repoInfo{}, "no upstream"},
		{repoInfo{Upstream: "o/m"}, "in sync"},
		{repoInfo{Upstream: "o/m", Ahead: 2}, "↑2"},
		{repoInfo{Upstream: "o/m", Behind: 3}, "↓3"},
		{repoInfo{Upstream: "o/m", Ahead: 2, Behind: 1}, "↑2 ↓1"},
	} {
		if got := syncLabel(c.r); got != c.want {
			t.Errorf("syncLabel(%+v) = %q, want %q", c.r, got, c.want)
		}
	}
}

func TestFilterLogAndLabel(t *testing.T) {
	log := []commit{
		{Hash: "1", Subject: "queue: retry", Author: "Priya Raman"},
		{Hash: "2", Subject: "cfg: XDG", Author: "Lena Fischer"},
		{Hash: "3", Subject: "Fix the QUEUE", Author: "Marcus Webb"},
	}
	for q, want := range map[string][]string{
		"":        {"1", "2", "3"},
		"  ":      {"1", "2", "3"},
		"queue":   {"1", "3"},
		"lena":    {"2"},
		"WEBB":    {"3"},
		"nothing": {},
		" retry ": {"1"},
	} {
		got := []string{}
		for _, c := range filterLog(log, q) {
			got = append(got, c.Hash)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("filterLog(%q) = %v, want %v", q, got, want)
		}
	}
	if got := logLabel(12, 12, ""); got != "12 commits" {
		t.Errorf("label = %q", got)
	}
	if got := logLabel(1, 1, ""); got != "1 commit" {
		t.Errorf("label = %q", got)
	}
	if got := logLabel(12, 3, "fix"); got != `3 of 12 commits · filter "fix"` {
		t.Errorf("label = %q", got)
	}
}
