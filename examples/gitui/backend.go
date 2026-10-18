package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// backend is where the repository's data comes from and where changes go.
// sampleRepo is the deterministic default; gitRepo runs git.
type backend interface {
	// Writable says whether Stage, Unstage, and Checkout may be called.
	Writable() bool
	Snapshot() (snapshot, error)
	// FileDiff is the diff of one entry: its staged side, or its working
	// tree side (an untracked file shows as all additions).
	FileDiff(f file, staged bool) (string, error)
	Commit(hash string) (rawCommit, error)
	Stage(f file) error
	Unstage(f file) error
	StageAll() error
	UnstageAll() error
	Checkout(name string) error
}

// gitRepo runs the git binary in a repository. Reads are always allowed;
// writes only when write is set.
type gitRepo struct {
	dir     string
	write   bool
	timeout time.Duration
}

func newGitRepo(dir string, write bool) *gitRepo {
	return &gitRepo{dir: dir, write: write, timeout: 10 * time.Second}
}

func (g *gitRepo) Writable() bool { return g.write }

// run runs git with args in g.dir and returns its standard output. A
// failure carries the first line of git's standard error.
func (g *gitRepo) run(args ...string) (string, error) {
	out, err := g.runOK(nil, args...)
	return out, err
}

// runOK is run, but an exit status in ok is not a failure (git diff
// --no-index exits 1 when the files differ).
func (g *gitRepo) runOK(ok []int, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), g.timeout)
	defer cancel()
	full := append([]string{"-C", g.dir, "--no-pager", "-c", "core.quotepath=false", "-c", "color.ui=never"}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = gitEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			for _, c := range ok {
				if ee.ExitCode() == c {
					return stdout.String(), nil
				}
			}
		}
		msg := strings.TrimSpace(strings.SplitN(strings.TrimSpace(stderr.String()), "\n", 2)[0])
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String(), fmt.Errorf("git %s: %s", args[0], msg)
	}
	return stdout.String(), nil
}

// gitEnv is the environment git runs in: the caller's, minus the variables
// that would override -C, plus a fixed locale and no prompts or optional
// locks.
func gitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		switch {
		case strings.HasPrefix(kv, "GIT_DIR="), strings.HasPrefix(kv, "GIT_WORK_TREE="), strings.HasPrefix(kv, "GIT_INDEX_FILE="):
		default:
			env = append(env, kv)
		}
	}
	return append(env, "LC_ALL=C", "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
}

func (g *gitRepo) Snapshot() (snapshot, error) {
	var s snapshot
	status, err := g.run("status", "--porcelain=v1", "-b")
	if err != nil {
		return s, err
	}
	s.Repo, s.Files = parseStatus(status)
	if top, err := g.run("rev-parse", "--show-toplevel"); err == nil {
		s.Repo.Name = filepath.Base(strings.TrimSpace(top))
	}
	if s.Repo.Name == "" || s.Repo.Name == "." {
		s.Repo.Name = filepath.Base(g.dir)
	}
	logOut, err := g.run("log", "-n", "200", "--format="+logFormat)
	if err != nil && !strings.Contains(err.Error(), "does not have any commits") {
		return s, err
	}
	s.Log = parseLog(logOut)
	branches, err := g.run("branch", "-vv", "--format="+branchFormat)
	if err != nil {
		return s, err
	}
	s.Branches = parseBranches(branches)
	return s, nil
}

func (g *gitRepo) FileDiff(f file, staged bool) (string, error) {
	switch {
	case staged && f.Orig != "":
		return g.run("diff", "--cached", "--no-ext-diff", "-M", "--", f.Orig, f.Path)
	case staged:
		return g.run("diff", "--cached", "--no-ext-diff", "--", f.Path)
	case f.Y == "?":
		return g.runOK([]int{1}, "diff", "--no-index", "--no-ext-diff", "--", "/dev/null", f.Path)
	}
	return g.run("diff", "--no-ext-diff", "--", f.Path)
}

func (g *gitRepo) Commit(hash string) (rawCommit, error) {
	var c rawCommit
	meta, err := g.run("show", "-s", "--date=format:%Y-%m-%d %H:%M", "--format=%an%x1f%ae%x1f%ad%x1f%B", hash)
	if err != nil {
		return c, err
	}
	f := strings.SplitN(strings.TrimRight(meta, "\n"), "\x1f", 4)
	for len(f) < 4 {
		f = append(f, "")
	}
	c.Author, c.Email, c.Date, c.Message = f[0], f[1], f[2], f[3]
	c.Diff, err = g.run("show", "--no-ext-diff", "--format=", "-p", hash)
	return c, err
}

func (g *gitRepo) Stage(f file) error {
	_, err := g.run("add", "--", f.Path)
	return err
}

func (g *gitRepo) Unstage(f file) error {
	paths := []string{f.Path}
	if f.Orig != "" {
		paths = append(paths, f.Orig)
	}
	_, err := g.run(append([]string{"reset", "-q", "--"}, paths...)...)
	return err
}

func (g *gitRepo) StageAll() error {
	_, err := g.run("add", "-A")
	return err
}

func (g *gitRepo) UnstageAll() error {
	_, err := g.run("reset", "-q")
	return err
}

func (g *gitRepo) Checkout(name string) error {
	_, err := g.run("checkout", name, "--")
	return err
}
