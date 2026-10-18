package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// This file holds the store's data shapes and the pure functions that turn
// git's text output (or the sample's) into them. Nothing here touches the
// terminal, the clock, or git itself, so every function is tested with
// canned strings.

// repoInfo is the header: the repository name, the current branch, and how
// it relates to its upstream.
type repoInfo struct {
	Name     string `json:"name"`
	Branch   string `json:"branch"`
	Upstream string `json:"upstream"`
	Ahead    int    `json:"ahead"`
	Behind   int    `json:"behind"`
}

// file is one entry of `git status --porcelain=v1`: X is the index column,
// Y the working tree column, Orig the old path of a rename.
type file struct {
	Path string `json:"path"`
	Orig string `json:"orig"`
	X    string `json:"x"`
	Y    string `json:"y"`
}

// commit is one row of the log table. Short, Date, and Refs are already
// formatted for display.
type commit struct {
	Hash    string `json:"hash"`
	Short   string `json:"short"`
	Subject string `json:"subject"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Refs    string `json:"refs"`
	Head    bool   `json:"head"`
}

// branch is one row of the branches table.
type branch struct {
	Name     string `json:"name"`
	Mark     string `json:"mark"`
	Current  bool   `json:"current"`
	Upstream string `json:"upstream"`
	Track    string `json:"track"`
	Hash     string `json:"hash"`
	Subject  string `json:"subject"`
	Date     string `json:"date"`
}

// snapshot is everything a refresh reads from a repository.
type snapshot struct {
	Repo     repoInfo
	Files    []file
	Log      []commit
	Branches []branch
}

// diffLine is one line of a diff, with the booleans the stylesheet's
// class:NAME guards read.
type diffLine struct {
	N    int    `json:"n"`
	Text string `json:"text"`
	Add  bool   `json:"add"`
	Del  bool   `json:"del"`
	Hunk bool   `json:"hunk"`
	Meta bool   `json:"meta"`
}

// diffView is the store's `diff`: the file preview of the Status tab.
type diffView struct {
	Title string     `json:"title"`
	Lines []diffLine `json:"lines"`
}

// para is one paragraph of a commit message.
type para struct {
	N    int    `json:"n"`
	Text string `json:"text"`
}

// detailView is the store's `detail`: the commit modal.
type detailView struct {
	Title   string     `json:"title"`
	Hash    string     `json:"hash"`
	Meta    string     `json:"meta"`
	Subject string     `json:"subject"`
	Body    []para     `json:"body"`
	Stats   string     `json:"stats"`
	Lines   []diffLine `json:"lines"`
}

// rawCommit is what a backend knows about one commit before it is shaped
// for the view.
type rawCommit struct {
	Author  string
	Email   string
	Date    string
	Message string
	Diff    string
}

// fileRow is a row of the staged or unstaged list. The is_* booleans pick
// the letter's color through class:NAME guards.
type fileRow struct {
	Path string `json:"path"`
	Orig string `json:"orig"`
	Code string `json:"code"`
	IsM  bool   `json:"is_m"`
	IsA  bool   `json:"is_a"`
	IsD  bool   `json:"is_d"`
	IsR  bool   `json:"is_r"`
	IsU  bool   `json:"is_u"`
}

func newFileRow(f file, code string) fileRow {
	return fileRow{
		Path: f.Path, Orig: f.Orig, Code: code,
		IsM: code == "M", IsA: code == "A", IsD: code == "D", IsR: code == "R" || code == "C", IsU: code == "?",
	}
}

// stagedRows are the files with a change in the index.
func stagedRows(files []file) []fileRow {
	rows := []fileRow{}
	for _, f := range files {
		if f.X == " " || f.X == "?" || f.X == "" {
			continue
		}
		rows = append(rows, newFileRow(f, f.X))
	}
	return rows
}

// unstagedRows are the files with a change in the working tree, untracked
// files included (shown with "?").
func unstagedRows(files []file) []fileRow {
	rows := []fileRow{}
	for _, f := range files {
		if f.Y == " " || f.Y == "" {
			continue
		}
		rows = append(rows, newFileRow(f, f.Y))
	}
	return rows
}

// counts is the store's derived `counts`.
type counts struct {
	Staged   int    `json:"staged"`
	Unstaged int    `json:"unstaged"`
	Total    int    `json:"total"`
	Label    string `json:"label"`
}

func countFiles(files []file) counts {
	s, u := len(stagedRows(files)), len(unstagedRows(files))
	return counts{Staged: s, Unstaged: u, Total: len(files), Label: fmt.Sprintf("%d staged · %d unstaged", s, u)}
}

// syncLabel says how the branch relates to its upstream.
func syncLabel(r repoInfo) string {
	switch {
	case r.Upstream == "":
		return "no upstream"
	case r.Ahead == 0 && r.Behind == 0:
		return "in sync"
	case r.Behind == 0:
		return fmt.Sprintf("↑%d", r.Ahead)
	case r.Ahead == 0:
		return fmt.Sprintf("↓%d", r.Behind)
	}
	return fmt.Sprintf("↑%d ↓%d", r.Ahead, r.Behind)
}

// filterLog keeps the commits whose subject or author contains q, ignoring
// case. An empty query keeps them all.
func filterLog(log []commit, q string) []commit {
	q = strings.ToLower(strings.TrimSpace(q))
	out := []commit{}
	for _, c := range log {
		if q == "" || strings.Contains(strings.ToLower(c.Subject), q) || strings.Contains(strings.ToLower(c.Author), q) {
			out = append(out, c)
		}
	}
	return out
}

// logLabel is the log table's title.
func logLabel(total, shown int, q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return plural(total, "commit")
	}
	return fmt.Sprintf("%d of %s · filter %q", shown, plural(total, "commit"), q)
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// ---- git text parsers

// parseTrack reads git's tracking text: "ahead 2, behind 1", "behind 3",
// "gone", or "".
func parseTrack(s string) (ahead, behind int, gone bool) {
	for _, part := range strings.Split(s, ",") {
		f := strings.Fields(part)
		switch {
		case len(f) == 2 && f[0] == "ahead":
			ahead, _ = strconv.Atoi(f[1])
		case len(f) == 2 && f[0] == "behind":
			behind, _ = strconv.Atoi(f[1])
		case len(f) == 1 && f[0] == "gone":
			gone = true
		}
	}
	return
}

// trackArrows is parseTrack's display form: "↑2 ↓1", "↓3", "gone", or "".
func trackArrows(ahead, behind int, gone bool) string {
	switch {
	case gone:
		return "gone"
	case ahead > 0 && behind > 0:
		return fmt.Sprintf("↑%d ↓%d", ahead, behind)
	case ahead > 0:
		return fmt.Sprintf("↑%d", ahead)
	case behind > 0:
		return fmt.Sprintf("↓%d", behind)
	}
	return ""
}

var arrowRe = regexp.MustCompile(`([↑↓])(\d+)`)

// arrowCounts reads trackArrows' output back.
func arrowCounts(s string) (ahead, behind int) {
	for _, m := range arrowRe.FindAllStringSubmatch(s, -1) {
		n, _ := strconv.Atoi(m[2])
		if m[1] == "↑" {
			ahead = n
		} else {
			behind = n
		}
	}
	return
}

// parseStatus reads `git status --porcelain=v1 -b`: the "## " header line
// and one "XY path" line per entry (renames are "XY old -> new").
func parseStatus(out string) (repoInfo, []file) {
	var r repoInfo
	files := []file{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		switch {
		case line == "":
		case strings.HasPrefix(line, "## "):
			r.Branch, r.Upstream, r.Ahead, r.Behind = parseBranchHeader(line[3:])
		case len(line) >= 4 && line[2] == ' ':
			x, y := line[0:1], line[1:2]
			if x == "!" && y == "!" {
				continue
			}
			f := file{X: x, Y: y}
			rest := line[3:]
			if x == "R" || y == "R" || x == "C" || y == "C" {
				if i := strings.Index(rest, " -> "); i >= 0 {
					f.Orig, rest = unquoteGit(rest[:i]), rest[i+4:]
				}
			}
			f.Path = unquoteGit(rest)
			files = append(files, f)
		}
	}
	return r, files
}

// parseBranchHeader reads the text after "## " in the status header.
func parseBranchHeader(h string) (name, upstream string, ahead, behind int) {
	switch {
	case strings.HasPrefix(h, "No commits yet on "):
		return strings.TrimPrefix(h, "No commits yet on "), "", 0, 0
	case strings.HasPrefix(h, "Initial commit on "):
		return strings.TrimPrefix(h, "Initial commit on "), "", 0, 0
	case strings.HasPrefix(h, "HEAD (no branch)"):
		return "HEAD (detached)", "", 0, 0
	}
	track := ""
	if i := strings.Index(h, " ["); i >= 0 && strings.HasSuffix(h, "]") {
		track = h[i+2 : len(h)-1]
		h = h[:i]
	}
	name = h
	if i := strings.Index(h, "..."); i >= 0 {
		name, upstream = h[:i], h[i+3:]
	}
	ahead, behind, _ = parseTrack(track)
	return
}

// unquoteGit undoes git's C-style path quoting ("a b\tc" in quotes).
func unquoteGit(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
	}
	return s
}

// logFormat and branchFormat are the --format strings of the commands that
// parseLog and parseBranches read. The unit separator \x1f separates
// fields: git log writes it as %x1f, git branch (ref-filter) as %1f.
const (
	logFormat    = "%H%x1f%h%x1f%an%x1f%ar%x1f%D%x1f%s"
	branchFormat = "%(HEAD)%1f%(refname:short)%1f%(upstream:short)%1f%(upstream:track,nobracket)%1f%(objectname:short)%1f%(contents:subject)%1f%(committerdate:relative)"
)

// parseLog reads `git log --format=logFormat`.
func parseLog(out string) []commit {
	log := []commit{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\x1f")
		if len(f) < 6 {
			continue
		}
		refs, head := formatRefs(f[4])
		log = append(log, commit{
			Hash: f[0], Short: f[1], Author: clean(f[2]), Date: shortAgo(f[3]),
			Refs: clean(refs), Head: head, Subject: clean(strings.Join(f[5:], " ")),
		})
	}
	return log
}

// parseBranches reads `git branch -vv --format=branchFormat`.
func parseBranches(out string) []branch {
	bs := []branch{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\x1f")
		if len(f) < 7 {
			continue
		}
		b := branch{
			Name: clean(f[1]), Upstream: clean(f[2]), Hash: f[4], Subject: clean(f[5]),
			Date: shortAgo(f[6]), Current: f[0] == "*",
		}
		if b.Current {
			b.Mark = "●"
		}
		ahead, behind, gone := parseTrack(f[3])
		b.Track = trackArrows(ahead, behind, gone)
		bs = append(bs, b)
	}
	return bs
}

// formatRefs turns git's %D ("HEAD -> main, origin/main, tag: v1") into
// "main, origin/main, v1" and says whether HEAD is among them.
func formatRefs(d string) (string, bool) {
	var out []string
	head := false
	for _, r := range strings.Split(d, ", ") {
		r = strings.TrimSpace(r)
		switch {
		case r == "":
		case strings.HasPrefix(r, "HEAD -> "):
			head = true
			out = append(out, strings.TrimPrefix(r, "HEAD -> "))
		case r == "HEAD":
			head = true
			out = append(out, "HEAD")
		case strings.HasSuffix(r, "/HEAD"):
		default:
			out = append(out, strings.TrimPrefix(r, "tag: "))
		}
	}
	return strings.Join(out, ", "), head
}

var agoRe = regexp.MustCompile(`^(\d+) (second|minute|hour|day|week|month|year)s?\b`)

// shortAgo shortens git's relative dates: "3 days ago" is "3d", "2 months
// ago" is "2mo". Anything else comes back unchanged.
func shortAgo(s string) string {
	m := agoRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return s
	}
	unit := map[string]string{"second": "s", "minute": "m", "hour": "h", "day": "d", "week": "w", "month": "mo", "year": "y"}[m[2]]
	return m[1] + unit
}

// ---- diffs

var ansiRe = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]")

// clean makes s safe for a text node, which rejects control characters and
// ANSI escapes (V007): it strips both and replaces invalid UTF-8.
func clean(s string) string {
	s = strings.ToValidUTF8(s, "�")
	s = ansiRe.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// expandTabs replaces each tab with four spaces: a text node trims tabs.
func expandTabs(s string) string { return strings.ReplaceAll(s, "\t", "    ") }

// diffLines classifies a unified diff line by line: lines before a file's
// first hunk are headers (meta), "@@" starts a hunk, and inside a hunk
// "+" and "-" lines are additions and deletions. At most max lines are
// returned, the last saying how many were dropped.
func diffLines(text string, max int) []diffLine {
	text = strings.TrimRight(text, "\r\n")
	lines := []diffLine{}
	if text == "" {
		return lines
	}
	inHunk := false
	for i, l := range strings.Split(text, "\n") {
		if i >= max {
			rest := strings.Count(text, "\n") + 1 - max
			lines = append(lines, diffLine{N: i, Text: fmt.Sprintf("… %d more lines not shown", rest), Meta: true})
			break
		}
		l = strings.TrimRight(l, "\r")
		if strings.HasPrefix(l, "diff --git") || strings.HasPrefix(l, "diff --cc") {
			inHunk = false
		}
		hunk := strings.HasPrefix(l, "@@")
		if hunk {
			inHunk = true
		}
		lines = append(lines, diffLine{
			N:    i,
			Text: clean(expandTabs(l)),
			Add:  inHunk && strings.HasPrefix(l, "+"),
			Del:  inHunk && strings.HasPrefix(l, "-"),
			Hunk: hunk,
			Meta: (!inHunk && !hunk) || strings.HasPrefix(l, "\\"),
		})
	}
	return lines
}

// diffStats counts the files, added lines, and removed lines of a diff.
func diffStats(text string) (files, adds, dels int) {
	for _, l := range diffLines(text, 1<<30) {
		switch {
		case l.Meta && strings.HasPrefix(l.Text, "diff --"):
			files++
		case l.Add:
			adds++
		case l.Del:
			dels++
		}
	}
	return
}

// statsLine is the commit modal's summary.
func statsLine(files, adds, dels int) string {
	return fmt.Sprintf("%s changed, +%d −%d", plural(files, "file"), adds, dels)
}

// splitMessage splits a commit message into its subject and paragraphs
// (lines of a paragraph are joined by a space).
func splitMessage(msg string) (string, []string) {
	msg = strings.Trim(strings.ReplaceAll(msg, "\r", ""), "\n")
	subject, rest, _ := strings.Cut(msg, "\n")
	var paras []string
	for _, p := range strings.Split(strings.Trim(rest, "\n"), "\n\n") {
		p = strings.Join(strings.Fields(p), " ")
		if p != "" {
			paras = append(paras, p)
		}
	}
	return clean(subject), paras
}

// buildDetail shapes a commit for the modal.
func buildDetail(hash string, raw rawCommit, maxLines int) detailView {
	subject, paras := splitMessage(raw.Message)
	body := []para{}
	for i, p := range paras {
		body = append(body, para{N: i, Text: clean(p)})
	}
	files, adds, dels := diffStats(raw.Diff)
	meta := raw.Author
	if raw.Email != "" {
		meta += " <" + raw.Email + ">"
	}
	meta = clean(meta + " · " + raw.Date)
	return detailView{
		Title: "commit " + short(hash), Hash: hash, Meta: meta, Subject: subject, Body: body,
		Stats: statsLine(files, adds, dels), Lines: diffLines(raw.Diff, maxLines),
	}
}

func short(hash string) string {
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

// mergeDiff appends the hunks of b to the diff a (both of one file), for a
// file staged while it already had staged changes.
func mergeDiff(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	if i := strings.Index(b, "\n@@"); i >= 0 {
		b = b[i+1:]
	}
	return strings.TrimRight(a, "\n") + "\n" + b
}
