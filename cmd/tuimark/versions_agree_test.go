package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
)

// versionMentions are the places outside the Go code that write the release
// version, each with a pattern whose first group is that version. A release
// bumps all of them together (AGENTS.md, "Releasing"); this list and that
// section name the same files.
var versionMentions = []struct {
	file string
	re   *regexp.Regexp
}{
	{"docs/.vitepress/config.mts", regexp.MustCompile(`const VERSION = 'v([^']+)'`)},
	{"docs/.vitepress/theme/components/HomeLanding.vue", regexp.MustCompile(`v([0-9][^ ]*) · Go runtime`)},
	{"docs/.vitepress/theme/components/InstallPanel.vue", regexp.MustCompile(`tuimark_([0-9][^_]*)_Darwin_arm64`)},
}

// The release version is written in several files; a release that bumps one
// and forgets another ships a CLI, a runtime, an npm package, or a website
// that disagree about which Tuimark they are.
func TestVersionsAgree(t *testing.T) {
	root := filepath.Join("..", "..")
	if version != host.Version {
		t.Errorf("cmd/tuimark version = %q, internal/host Version = %q", version, host.Version)
	}

	raw, err := os.ReadFile(filepath.Join(root, "sdk", "ts", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Version != version {
		t.Errorf("sdk/ts/package.json version = %q, want %q", pkg.Version, version)
	}

	for _, m := range versionMentions {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(m.file)))
		if err != nil {
			t.Errorf("%s: %v (update versionMentions and AGENTS.md's \"Releasing\" if it moved)", m.file, err)
			continue
		}
		got := m.re.FindSubmatch(raw)
		if got == nil {
			t.Errorf("%s: no version matches %s", m.file, m.re)
			continue
		}
		if string(got[1]) != version {
			t.Errorf("%s says %q, want %q", m.file, got[1], version)
		}
	}
}
