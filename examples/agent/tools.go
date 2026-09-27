// Tools implements the small tool set the agent may call: list_files,
// read_file, search, edit_file, and run_command.
//
// list_files, read_file, search, and edit_file are confined to a workspace
// directory: paths that try to leave it (absolute paths, "..", or a symlink
// that resolves outside) are rejected.
//
// run_command is NOT path-confined the same way. It only runs with its cwd
// set to the workspace root, with no shell, a timeout, and bounded output;
// argv is otherwise executed as given, so a command can still read or write
// anything the OS permissions allow. As a best-effort guard, an argv element
// that is an absolute path or that contains a ".." path segment is rejected
// before the command runs, but that only catches the obvious cases — it does
// not stop e.g. `sh -c ...`, `find /`, or a program that reads paths from
// stdin or an environment variable. run_command's real safety net is that it
// always requires explicit human approval and is refused outright in plan
// mode.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	maxReadBytes   = 64 * 1024
	maxSearchHits  = 200
	maxSearchBytes = 512 * 1024 // per-file cap while grepping
	maxRunOutput   = 16 * 1024
	defaultTimeout = 5 * time.Second
)

// Tools confines file and command operations to Root.
type Tools struct {
	Root string // absolute, symlinks resolved
}

// NewTools resolves dir (creating it if needed) and returns a Tools rooted
// there.
func NewTools(dir string) (*Tools, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("workspace %q: %w", dir, err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	return &Tools{Root: resolved}, nil
}

// resolve validates rel against the workspace root and returns the absolute
// path. It rejects absolute input, ".." segments, and symlinks that escape.
func (t *Tools) resolve(rel string) (string, error) {
	if rel == "" {
		return "", errors.New("path is required")
	}
	if filepath.IsAbs(rel) || (len(rel) > 0 && rel[0] == '/') {
		return "", fmt.Errorf("path %q must be relative to the workspace", rel)
	}
	clean := filepath.Clean(rel)
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		if part == ".." {
			return "", fmt.Errorf("path %q escapes the workspace", rel)
		}
	}
	joined := filepath.Join(t.Root, clean)
	if joined != t.Root && !within(joined, t.Root) {
		return "", fmt.Errorf("path %q escapes the workspace", rel)
	}
	// Resolve symlinks on the nearest existing ancestor, walking up from
	// the leaf, so a symlink anywhere on the path that points outside the
	// workspace is rejected even when the leaf itself does not exist yet
	// (e.g. a new file edit_file is about to create).
	check := joined
	for {
		if resolved, err := filepath.EvalSymlinks(check); err == nil {
			if !within(resolved, t.Root) {
				return "", fmt.Errorf("path %q escapes the workspace", rel)
			}
			return joined, nil
		}
		if check == t.Root {
			return joined, nil
		}
		parent := filepath.Dir(check)
		if parent == check {
			return joined, nil
		}
		check = parent
	}
}

// rejectEscapingArg is RunCommand's best-effort guard against argv elements
// that are obviously reaching outside the workspace: an absolute path, or a
// relative path containing a ".." segment. It is not a sandbox (see
// RunCommand's doc comment) — it only catches the literal case, not e.g. a
// path assembled by a shell or read from a file.
func rejectEscapingArg(a string) error {
	if filepath.IsAbs(a) || strings.HasPrefix(a, "/") {
		return fmt.Errorf("argv element %q is an absolute path, which run_command rejects", a)
	}
	for _, part := range strings.Split(a, string(filepath.Separator)) {
		if part == ".." {
			return fmt.Errorf("argv element %q contains a %q path segment, which run_command rejects", a, "..")
		}
	}
	return nil
}

func within(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == root {
		return true
	}
	return strings.HasPrefix(path, root+string(filepath.Separator))
}

// ListFiles lists paths under rel (default ".") relative to the workspace,
// sorted, skipping dotdirs.
func (t *Tools) ListFiles(rel string) ([]string, error) {
	if rel == "" {
		rel = "."
	}
	root, err := t.resolve(rel)
	if err != nil {
		return nil, err
	}
	var out []string
	err = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if p == root {
			return nil
		}
		name := info.Name()
		if info.IsDir() {
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		r, err := filepath.Rel(t.Root, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(r))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// ReadFile returns the file content, bounded to maxReadBytes.
func (t *Tools) ReadFile(rel string) (string, error) {
	p, err := t.resolve(rel)
	if err != nil {
		return "", err
	}
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%q is a directory", rel)
	}
	buf := make([]byte, min(int(info.Size()), maxReadBytes)+1)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return "", err
	}
	out := buf[:n]
	truncated := info.Size() > int64(maxReadBytes)
	if len(out) > maxReadBytes {
		out = out[:maxReadBytes]
		truncated = true
	}
	if truncated {
		return string(out) + "\n… (truncated)", nil
	}
	return string(out), nil
}

// SearchHit is one matching line from Search.
type SearchHit struct {
	Path string
	Line int
	Text string
}

// Search greps for substr across files under the workspace, bounded to
// maxSearchHits matches and maxSearchBytes read per file.
func (t *Tools) Search(substr string) ([]SearchHit, error) {
	if substr == "" {
		return nil, errors.New("search text is required")
	}
	files, err := t.ListFiles(".")
	if err != nil {
		return nil, err
	}
	var hits []SearchHit
	for _, rel := range files {
		if len(hits) >= maxSearchHits {
			break
		}
		p := filepath.Join(t.Root, rel)
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if len(data) > maxSearchBytes {
			data = data[:maxSearchBytes]
		}
		if !isProbablyText(data) {
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, substr) {
				hits = append(hits, SearchHit{Path: rel, Line: i + 1, Text: line})
				if len(hits) >= maxSearchHits {
					break
				}
			}
		}
	}
	return hits, nil
}

func isProbablyText(b []byte) bool {
	for _, c := range b {
		if c == 0 {
			return false
		}
	}
	return true
}

// EditFile replaces the single, unique occurrence of oldStr with newStr in
// rel. It fails if oldStr is absent or appears more than once.
func (t *Tools) EditFile(rel, oldStr, newStr string) error {
	p, err := t.resolve(rel)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return fmt.Errorf("read %q: %w", rel, err)
	}
	content := string(data)
	n := strings.Count(content, oldStr)
	switch n {
	case 0:
		return fmt.Errorf("old_string not found in %q", rel)
	case 1:
	default:
		return fmt.Errorf("old_string is ambiguous in %q: %d matches", rel, n)
	}
	updated := strings.Replace(content, oldStr, newStr, 1)
	return os.WriteFile(p, []byte(updated), 0o644)
}

// CommandResult is the bounded outcome of RunCommand.
type CommandResult struct {
	ExitCode int
	Output   string // combined stdout+stderr, bounded to maxRunOutput
	TimedOut bool
}

// RunCommand runs argv (no shell) with cwd at the workspace root, bounded by
// timeout and by maxRunOutput of combined output.
//
// This is not a sandbox: run_command is not path-confined the way the other
// tools are (see the package doc). As a best-effort guard against the most
// obvious escapes, every argv element is rejected if it is an absolute path
// or contains a ".." path segment; this alone does not stop a command that
// reaches outside the workspace some other way (a shell, a path baked into a
// script, a path read from stdin or an env var, ...). Approval and plan-mode
// refusal, enforced by the caller, are what actually keep this tool safe.
func (t *Tools) RunCommand(ctx context.Context, argv []string, timeout time.Duration) (CommandResult, error) {
	if len(argv) == 0 {
		return CommandResult{}, errors.New("argv is required")
	}
	for _, a := range argv {
		if err := rejectEscapingArg(a); err != nil {
			return CommandResult{}, err
		}
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, argv[0], argv[1:]...)
	cmd.Dir = t.Root
	var buf boundedBuffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	res := CommandResult{Output: buf.String()}
	if cctx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
		res.ExitCode = -1
		return res, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	if err != nil {
		return res, err
	}
	res.ExitCode = 0
	return res, nil
}

// boundedBuffer caps how much output RunCommand keeps.
type boundedBuffer struct {
	buf bytes.Buffer
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	room := maxRunOutput - b.buf.Len()
	if room <= 0 {
		return len(p), nil // drop, but report the full length written
	}
	if len(p) > room {
		b.buf.Write(p[:room])
		b.buf.WriteString("\n… (truncated)")
		return len(p), nil
	}
	b.buf.Write(p)
	return len(p), nil
}

func (b *boundedBuffer) String() string { return b.buf.String() }
