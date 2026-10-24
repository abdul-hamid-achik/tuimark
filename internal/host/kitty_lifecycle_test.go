package host

import (
	"image/color"
	"strings"
	"testing"
)

// apcControls returns the control-data part (before ";") of every kitty
// graphics APC in s, in order.
func apcControls(s string) []string {
	var out []string
	for _, part := range strings.Split(s, "\x1b_G")[1:] {
		end := strings.Index(part, "\x1b\\")
		if end < 0 {
			continue
		}
		body := part[:end]
		if i := strings.IndexByte(body, ';'); i >= 0 {
			body = body[:i]
		}
		out = append(out, body)
	}
	return out
}

// Kitty allows only m (and q) on the continuation chunks of a chunked
// transmission; the first chunk carries the whole control list once
// (v0.5.2: every chunk repeated it).
func TestKittyUploadContinuationChunksCarryOnlyMAndQ(t *testing.T) {
	s := kittyUpload(7, 100, 100, noiseImg(300, 300))
	ctl := apcControls(s)
	var chunks []string
	for _, c := range ctl {
		if !strings.HasPrefix(c, "a=d") {
			chunks = append(chunks, c)
		}
	}
	if len(chunks) < 3 {
		t.Fatalf("a noise image should need 3+ chunks, got %d: %q", len(chunks), chunks)
	}
	if want := "a=t,i=7,U=1,c=100,r=100,q=2,f=100,t=d,m=1"; chunks[0] != want {
		t.Errorf("first chunk = %q, want %q", chunks[0], want)
	}
	for i, c := range chunks[1:] {
		last := i == len(chunks)-2
		for _, kv := range strings.Split(c, ",") {
			if k, _, _ := strings.Cut(kv, "="); k != "m" && k != "q" {
				t.Errorf("continuation chunk %d carries key %q (%q): only m and q are allowed", i+1, k, c)
			}
		}
		wantM := "m=1"
		if last {
			wantM = "m=0"
		}
		if !strings.Contains(c, wantM) {
			t.Errorf("continuation chunk %d = %q, want %s", i+1, c, wantM)
		}
	}
}

// The delete before a (re)transmission frees the old data as well as its
// placements: uppercase d=I (lowercase d=i leaves the image data in the
// terminal's memory).
func TestKittyUploadFreesThePreviousImageWithUppercaseDelete(t *testing.T) {
	s := kittyUpload(42, 4, 2, solidImg(6, 6, color.RGBA{R: 255, A: 255}))
	if !strings.HasPrefix(s, "\x1b_Ga=d,d=I,i=42") {
		t.Errorf("upload starts %q, want a d=I delete of the id", s[:min(len(s), 30)])
	}
	for _, c := range apcControls(s) {
		if strings.Contains(c, "d=i") || strings.Contains(c, "d=a") {
			t.Errorf("lowercase delete %q frees nothing", c)
		}
	}
}

// Leaving frees the images with the uppercase delete (d=A): the
// lowercase d=a only removes the placements and leaves the image data in
// the terminal's memory.
func TestKittyLeaveFreesWithUppercaseDelete(t *testing.T) {
	out := (&termSession{kittyImgs: true}).leave()
	if !strings.Contains(out, "\x1b_Ga=d,d=A") {
		t.Errorf("leave = %q, want a d=A delete", out)
	}
	for _, c := range apcControls(out) {
		if strings.Contains(c, "d=a") || strings.Contains(c, "d=i") {
			t.Errorf("lowercase delete %q frees nothing", c)
		}
	}
}
