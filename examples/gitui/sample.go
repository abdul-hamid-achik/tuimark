package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// sampleRepo is the in-memory fake repository behind sample.json. It
// behaves like a tiny git for the three things the app changes (staging,
// unstaging, checkout), so the whole app can be tried, screenshotted, and
// tested with no repository and no git binary.
type sampleRepo struct {
	mu       sync.Mutex
	repo     repoInfo
	files    []file
	log      []commit
	branches []branch
	diffs    map[string]sampleDiff
	commits  map[string]sampleCommit
}

// sampleDiff is the staged and working tree diff of one path.
type sampleDiff struct {
	Staged   string `json:"staged"`
	Unstaged string `json:"unstaged"`
}

// sampleCommit is the detail of one commit.
type sampleCommit struct {
	Message string `json:"message"`
	Diff    string `json:"diff"`
	Email   string `json:"email"`
	Date    string `json:"date"`
}

// sampleSim is the `sim` member of sample.json: data only the sample
// backend reads, kept out of the store.
type sampleSim struct {
	Diffs   map[string]sampleDiff   `json:"diffs"`
	Commits map[string]sampleCommit `json:"commits"`
}

func (s *sampleRepo) Writable() bool { return true }

func (s *sampleRepo) Snapshot() (snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return snapshot{
		Repo:     s.repo,
		Files:    append([]file{}, s.files...),
		Log:      append([]commit{}, s.log...),
		Branches: append([]branch{}, s.branches...),
	}, nil
}

func (s *sampleRepo) find(path string) int {
	for i, f := range s.files {
		if f.Path == path {
			return i
		}
	}
	return -1
}

func (s *sampleRepo) FileDiff(f file, staged bool) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.diffs[f.Path]
	if staged {
		return d.Staged, nil
	}
	return d.Unstaged, nil
}

func (s *sampleRepo) Commit(hash string) (rawCommit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.commits[hash]
	if !ok {
		return rawCommit{}, fmt.Errorf("unknown commit %s", short(hash))
	}
	var author string
	for _, l := range s.log {
		if l.Hash == hash {
			author = l.Author
		}
	}
	return rawCommit{Author: author, Email: c.Email, Date: c.Date, Message: c.Message, Diff: c.Diff}, nil
}

func (s *sampleRepo) Stage(f file) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stage(f.Path)
	return nil
}

// stage moves the working tree change of path into the index.
func (s *sampleRepo) stage(path string) {
	i := s.find(path)
	if i < 0 || s.files[i].Y == " " {
		return
	}
	f := &s.files[i]
	x := f.Y
	if x == "?" {
		x = "A"
	}
	d := s.diffs[path]
	d.Staged, d.Unstaged = mergeDiff(d.Staged, d.Unstaged), ""
	s.diffs[path] = d
	f.X, f.Y = x, " "
}

func (s *sampleRepo) Unstage(f file) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unstage(f.Path)
	return nil
}

// unstage moves the index change of path back to the working tree, as git
// reset does: an added file becomes untracked, a rename becomes a deletion
// of the old path plus an untracked new one.
func (s *sampleRepo) unstage(path string) {
	i := s.find(path)
	if i < 0 || s.files[i].X == " " || s.files[i].X == "?" {
		return
	}
	f := &s.files[i]
	d := s.diffs[path]
	d.Unstaged, d.Staged = mergeDiff(d.Unstaged, d.Staged), ""
	s.diffs[path] = d
	switch f.X {
	case "A":
		f.X, f.Y = "?", "?"
	case "R", "C":
		orig := f.Orig
		f.X, f.Y, f.Orig = "?", "?", ""
		if orig != "" && s.find(orig) < 0 {
			s.files = append(s.files, file{Path: orig, X: " ", Y: "D"})
			s.diffs[orig] = sampleDiff{Unstaged: deletedDiff(orig)}
		}
	default:
		f.X, f.Y = " ", f.X
	}
	sort.SliceStable(s.files, func(a, b int) bool { return s.files[a].Path < s.files[b].Path })
}

// deletedDiff is the diff of a removed file whose content the sample does
// not keep.
func deletedDiff(path string) string {
	return strings.Join([]string{
		"diff --git a/" + path + " b/" + path,
		"deleted file mode 100644",
		"--- a/" + path,
		"+++ /dev/null",
		"@@ -1 +0,0 @@",
		"-(removed)",
	}, "\n") + "\n"
}

func (s *sampleRepo) StageAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range append([]file{}, s.files...) {
		s.stage(f.Path)
	}
	return nil
}

func (s *sampleRepo) UnstageAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range append([]file{}, s.files...) {
		s.unstage(f.Path)
	}
	return nil
}

func (s *sampleRepo) Checkout(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	target := -1
	for i, b := range s.branches {
		if b.Name == name {
			target = i
		}
	}
	if target < 0 {
		return fmt.Errorf("git checkout: pathspec %q did not match any branch", name)
	}
	for i := range s.branches {
		s.branches[i].Current = i == target
		s.branches[i].Mark = ""
	}
	b := &s.branches[target]
	b.Mark = "●"
	s.repo.Branch, s.repo.Upstream = b.Name, b.Upstream
	s.repo.Ahead, s.repo.Behind = arrowCounts(b.Track)
	for i := range s.log {
		s.log[i].Head = s.log[i].Short == b.Hash
	}
	return nil
}
