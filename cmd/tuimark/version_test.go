package main

import "testing"

// withBuildMetadata sets the link-time build variables for one test and
// restores them afterwards, as `go build -ldflags "-X main.version=..."`
// would have set them.
func withBuildMetadata(t *testing.T, v, c, d string) {
	t.Helper()
	oldV, oldC, oldD := version, commit, date
	version, commit, date = v, c, d
	t.Cleanup(func() { version, commit, date = oldV, oldC, oldD })
}

// A build without -ldflags (go build, go install, go test) reports the
// source tree's release, 0.3.0, with no build metadata. All three
// spellings of the command print the same line.
func TestVersionDefault(t *testing.T) {
	if version != "0.3.0" {
		t.Fatalf("default version = %q, want 0.3.0", version)
	}
	withBuildMetadata(t, version, "", "")
	for _, arg := range []string{"version", "--version", "-v"} {
		code, out, errw := runCLI(arg)
		if code != 0 {
			t.Fatalf("%s: exit %d, want 0; stderr=%s", arg, code, errw)
		}
		if out != "tuimark 0.3.0\n" {
			t.Errorf("%s: got %q, want %q", arg, out, "tuimark 0.3.0\n")
		}
		if errw != "" {
			t.Errorf("%s: unexpected stderr %q", arg, errw)
		}
	}
}

// Release builds inject version, commit, and date; `tuimark version` shows
// whichever of commit and date are set, after the version.
func TestVersionShowsBuildMetadata(t *testing.T) {
	cases := []struct {
		name, version, commit, date, want string
	}{
		{"all", "0.2.1", "abc1234", "2026-09-27T12:00:00Z", "tuimark 0.2.1 (commit abc1234, built 2026-09-27T12:00:00Z)\n"},
		{"commit only", "0.2.1", "abc1234", "", "tuimark 0.2.1 (commit abc1234)\n"},
		{"date only", "0.2.1", "", "2026-09-27T12:00:00Z", "tuimark 0.2.1 (built 2026-09-27T12:00:00Z)\n"},
		{"version only", "0.3.0-rc.1", "", "", "tuimark 0.3.0-rc.1\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withBuildMetadata(t, tc.version, tc.commit, tc.date)
			code, out, errw := runCLI("version")
			if code != 0 {
				t.Fatalf("exit %d, want 0; stderr=%s", code, errw)
			}
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
