package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestTools(t *testing.T) *Tools {
	t.Helper()
	dir := t.TempDir()
	if err := SeedDemoWorkspace(dir); err != nil {
		t.Fatalf("seed demo workspace: %v", err)
	}
	tl, err := NewTools(dir)
	if err != nil {
		t.Fatalf("NewTools: %v", err)
	}
	return tl
}

func TestListFiles(t *testing.T) {
	tl := newTestTools(t)
	files, err := tl.ListFiles(".")
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	want := map[string]bool{"main.go": true, "notes.txt": true}
	if len(files) != len(want) {
		t.Fatalf("ListFiles = %v, want %v", files, want)
	}
	for _, f := range files {
		if !want[f] {
			t.Errorf("unexpected file %q", f)
		}
	}
}

func TestReadFile(t *testing.T) {
	tl := newTestTools(t)
	got, err := tl.ReadFile("notes.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got != "before\n" {
		t.Errorf("ReadFile = %q, want %q", got, "before\n")
	}
}

func TestReadFileBounded(t *testing.T) {
	tl := newTestTools(t)
	big := strings.Repeat("x", maxReadBytes+1000)
	if err := os.WriteFile(filepath.Join(tl.Root, "big.txt"), []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := tl.ReadFile("big.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.HasSuffix(got, "(truncated)") {
		t.Errorf("ReadFile of an oversized file should be truncated, got suffix %q", got[max(0, len(got)-30):])
	}
	if len(got) > maxReadBytes+64 {
		t.Errorf("ReadFile returned %d bytes, want roughly <= %d", len(got), maxReadBytes)
	}
}

func TestPathConfinement(t *testing.T) {
	tl := newTestTools(t)
	cases := []string{"../etc/passwd", "/etc/passwd", "a/../../b", ".."}
	for _, c := range cases {
		if _, err := tl.resolve(c); err == nil {
			t.Errorf("resolve(%q) = nil error, want an escape error", c)
		}
	}
}

func TestPathConfinementSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	tl, err := NewTools(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tl.ReadFile("escape/secret.txt"); err == nil {
		t.Error("ReadFile through a symlink that escapes the workspace should fail")
	}
}

func TestEditFileExactUnique(t *testing.T) {
	tl := newTestTools(t)
	if err := tl.EditFile("notes.txt", "before", "after"); err != nil {
		t.Fatalf("EditFile: %v", err)
	}
	got, _ := tl.ReadFile("notes.txt")
	if got != "after\n" {
		t.Errorf("after edit, content = %q, want %q", got, "after\n")
	}
}

func TestEditFileMissingOld(t *testing.T) {
	tl := newTestTools(t)
	if err := tl.EditFile("notes.txt", "nope-not-there", "x"); err == nil {
		t.Error("EditFile with a missing old_string should fail")
	}
}

func TestEditFileAmbiguous(t *testing.T) {
	tl := newTestTools(t)
	if err := os.WriteFile(filepath.Join(tl.Root, "dup.txt"), []byte("aa\naa\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tl.EditFile("dup.txt", "aa", "bb"); err == nil {
		t.Error("EditFile with an ambiguous old_string should fail")
	}
}

func TestSearch(t *testing.T) {
	tl := newTestTools(t)
	hits, err := tl.Search("func main")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || hits[0].Path != "main.go" {
		t.Errorf("Search hits = %+v, want one hit in main.go", hits)
	}
}

func TestRunCommand(t *testing.T) {
	tl := newTestTools(t)
	res, err := tl.RunCommand(context.Background(), []string{"echo", "hello"}, time.Second)
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if res.ExitCode != 0 || !strings.Contains(res.Output, "hello") {
		t.Errorf("RunCommand = %+v, want exit 0 containing %q", res, "hello")
	}
}

func TestRunCommandTimeout(t *testing.T) {
	tl := newTestTools(t)
	res, err := tl.RunCommand(context.Background(), []string{"sleep", "2"}, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if !res.TimedOut {
		t.Errorf("RunCommand result = %+v, want TimedOut", res)
	}
}

// TestRunCommandCwdIsWorkspaceRoot pins down the one guarantee RunCommand's
// doc comment actually makes: it runs with cwd at the workspace root.
func TestRunCommandCwdIsWorkspaceRoot(t *testing.T) {
	tl := newTestTools(t)
	res, err := tl.RunCommand(context.Background(), []string{"pwd"}, time.Second)
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if got := strings.TrimSpace(res.Output); got != tl.Root {
		t.Errorf("RunCommand pwd = %q, want workspace root %q", got, tl.Root)
	}
}

// TestRunCommandRejectsAbsolutePathArg is the regression test for the
// finding that run_command had zero path confinement: an argv element that
// is an absolute path outside the workspace must be rejected before the
// command runs, not silently executed against the real filesystem.
func TestRunCommandRejectsAbsolutePathArg(t *testing.T) {
	tl := newTestTools(t)

	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outside, []byte("OUTSIDE_MARKER_CONTENT\n"), 0o644); err != nil {
		t.Fatalf("seed outside file: %v", err)
	}

	res, err := tl.RunCommand(context.Background(), []string{"cat", outside}, time.Second)
	if err == nil {
		t.Fatalf("RunCommand(cat %s) = %+v, %v, want a rejection error", outside, res, err)
	}
	if !strings.Contains(err.Error(), "absolute path") {
		t.Errorf("RunCommand error = %v, want it to mention the absolute path", err)
	}
	if strings.Contains(res.Output, "OUTSIDE_MARKER_CONTENT") {
		t.Errorf("RunCommand leaked outside file content: %+v", res)
	}

	pwned := filepath.Join(outsideDir, "pwned_via_touch.txt")
	if _, err := tl.RunCommand(context.Background(), []string{"touch", pwned}, time.Second); err == nil {
		t.Fatalf("RunCommand(touch %s) succeeded, want rejection", pwned)
	}
	if _, statErr := os.Stat(pwned); !os.IsNotExist(statErr) {
		t.Errorf("RunCommand created a file outside the workspace: %s", pwned)
	}
}

// TestRunCommandRejectsDotDotArg covers the relative-path escape variant
// (a "run cat ../outside/secret.txt" style prompt), which the resolve()-based
// tools already rejected but run_command did not.
func TestRunCommandRejectsDotDotArg(t *testing.T) {
	tl := newTestTools(t)

	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("OUTSIDE_MARKER_CONTENT\n"), 0o644); err != nil {
		t.Fatalf("seed outside file: %v", err)
	}
	rel, err := filepath.Rel(tl.Root, filepath.Join(outsideDir, "secret.txt"))
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}

	res, err := tl.RunCommand(context.Background(), []string{"cat", rel}, time.Second)
	if err == nil {
		t.Fatalf("RunCommand(cat %s) = %+v, want a rejection error", rel, res)
	}
	if !strings.Contains(err.Error(), "..") {
		t.Errorf("RunCommand error = %v, want it to mention %q", err, "..")
	}
}
