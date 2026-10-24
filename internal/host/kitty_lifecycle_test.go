package host

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/paint"
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

// pngFile writes a w×h solid png into dir/name.
func pngFile(t *testing.T, dir, name string, w, h int, c color.RGBA) string {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, solidImg(w, h, c)); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func uploadsOf(out string) int { return strings.Count(out, "a=t,i=") }

// waitUploads waits until the output holds n transmissions.
func (r *sessRig) waitUploads(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for uploadsOf(r.out.String()) < n {
		if time.Now().After(deadline) {
			t.Fatalf("want %d transmissions, got %d: %q", n, uploadsOf(r.out.String()), r.out.String())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// An image whose file content changes is transmitted again, under the
// same id (the placeholder cells do not change), after the delete of the
// old one; a frame that changes nothing about the image transmits
// nothing (v0.5.2: a transmitted image was never refreshed).
func TestKittyRetransmitsWhenTheFileChanges(t *testing.T) {
	dir := t.TempDir()
	src := pngFile(t, dir, "pic.png", 6, 6, color.RGBA{R: 255, A: 255})
	id := paint.KittyID(src, 4, 2)
	r := startSessionWith(t, `<tui><screen><img src="`+src+`" width="4" height="2"/><text>[{text}]</text></screen></tui>`,
		&termSession{probe: true, wait: 10 * time.Second, profile: paint.TrueColor}, func(*App) {})
	r.waitOut(t, "\x1b[c")
	r.send(t, "\x1b_Gi=31;OK\x1b\\\x1b[?62c")
	r.waitUploads(t, 1)

	// A frame that leaves the file alone does not transmit again.
	if err := r.app.Set("text", "one"); err != nil {
		t.Fatal(err)
	}
	r.waitOut(t, "one]")
	if n := uploadsOf(r.out.String()); n != 1 {
		t.Fatalf("an unchanged file was transmitted %d times, want 1", n)
	}

	// New content at the same path: another size and mtime.
	pngFile(t, dir, "pic.png", 8, 8, color.RGBA{B: 255, A: 255})
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(src, later, later); err != nil {
		t.Fatal(err)
	}
	if err := r.app.Set("text", "two"); err != nil {
		t.Fatal(err)
	}
	r.waitUploads(t, 2)
	out := r.out.String()
	second := out[strings.LastIndex(out, "\x1b_Ga=d"):]
	if !strings.HasPrefix(second, fmt.Sprintf("\x1b_Ga=d,d=I,i=%d", id)) || !strings.Contains(second, fmt.Sprintf("a=t,i=%d,", id)) {
		t.Errorf("the retransmission does not delete then send id %d: %q", id, second[:min(len(second), 120)])
	}
	r.in.Close()
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
}

// An image the current frame no longer shows is freed (uppercase delete
// by id); showing it again transmits it again; leaving frees what is
// still held, before the alternate screen is left (v0.5.2: nothing was
// ever freed until exit, and then only by a lowercase delete-all).
func TestKittyFreesUnreferencedImagesAndAllOnLeave(t *testing.T) {
	dir := t.TempDir()
	src := pngFile(t, dir, "pic.png", 6, 6, color.RGBA{R: 255, A: 255})
	id := paint.KittyID(src, 4, 2)
	r := startSessionWith(t, `<tui><screen><img if="show" src="`+src+`" width="4" height="2"/><text>[{text}]</text></screen></tui>`,
		&termSession{probe: true, wait: 10 * time.Second, profile: paint.TrueColor},
		func(a *App) { _ = a.Set("show", true) })
	r.waitOut(t, "\x1b[c")
	r.send(t, "\x1b_Gi=31;OK\x1b\\\x1b[?62c")
	r.waitUploads(t, 1)
	free := fmt.Sprintf("\x1b_Ga=d,d=I,i=%d", id)
	anyDelete := regexp.MustCompile(fmt.Sprintf(`a=d,d=[iI],i=%d[,\x1b]`, id))
	if n := len(anyDelete.FindAllString(r.out.String(), -1)); n != 1 {
		t.Fatalf("before hiding: %d deletes of the id, want 1 (the one before the upload)", n)
	}

	if err := r.app.Set("show", false); err != nil {
		t.Fatal(err)
	}
	// The image left the frame: it is freed, by an uppercase delete.
	deadline := time.Now().Add(2 * time.Second)
	for len(anyDelete.FindAllString(r.out.String(), -1)) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("a hidden image was never freed: %q", r.out.String())
		}
		time.Sleep(2 * time.Millisecond)
	}
	r.waitCount(t, free, 2)
	if err := r.app.Set("show", true); err != nil {
		t.Fatal(err)
	}
	r.waitUploads(t, 2) // shown again: transmitted again
	r.in.Close()
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
	out := r.out.String()
	// Leaving frees the id again: a free for it appears after the last
	// transmission.
	lastSend := strings.LastIndex(out, "a=t,i=")
	if lastSend < 0 || !strings.Contains(out[lastSend:], free) {
		t.Errorf("no free of id %d after the last transmission: %q", id, out[max(0, len(out)-120):])
	}
	if idx := strings.LastIndex(out, free); idx > strings.Index(out, leaveScreen) {
		t.Errorf("the last free comes after the alternate screen was left")
	}
	if regexp.MustCompile(`d=[ai][,\x1b]`).MatchString(out) {
		t.Errorf("a lowercase delete was written: it frees no image data: %q", out)
	}
	if strings.Contains(out, "d=A") {
		t.Errorf("a delete-all was written: only the session's own ids may be freed")
	}
}

// waitCount waits until the output holds want n times.
func (r *sessRig) waitCount(t *testing.T, want string, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for strings.Count(r.out.String(), want) < n {
		if time.Now().After(deadline) {
			t.Fatalf("output has %d of %q, want %d: %q", strings.Count(r.out.String(), want), want, n, r.out.String())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// With synchronized output on, a retransmission (the delete of the id and
// the new data) is written inside the frame's wrapper, in the same write
// as the cells that show it, so the terminal never presents the frame
// between the delete and the new image.
func TestKittyRetransmitIsInsideTheSyncWrapper(t *testing.T) {
	dir := t.TempDir()
	src := pngFile(t, dir, "pic.png", 6, 6, color.RGBA{R: 255, A: 255})
	id := paint.KittyID(src, 4, 2)
	r := startSessionWith(t, `<tui><screen><img src="`+src+`" width="4" height="2"/><text>[{text}]</text></screen></tui>`,
		&termSession{probe: true, wait: 10 * time.Second, profile: paint.TrueColor}, func(*App) {})
	r.waitOut(t, "\x1b[c")
	// The terminal answers: kitty graphics OK, 2026 supported (off), DA1.
	r.send(t, "\x1b_Gi=31;OK\x1b\\\x1b[?2026;2$y\x1b[?62c")
	r.waitUploads(t, 1)
	pngFile(t, dir, "pic.png", 8, 8, color.RGBA{B: 255, A: 255})
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(src, later, later); err != nil {
		t.Fatal(err)
	}
	if err := r.app.Set("text", "two"); err != nil {
		t.Fatal(err)
	}
	r.waitUploads(t, 2)
	r.waitOut(t, "two]")
	out := r.out.String()
	del := fmt.Sprintf("\x1b_Ga=d,d=I,i=%d", id)
	for _, which := range []string{"first", "second"} {
		var i int
		if which == "first" {
			i = strings.Index(out, del)
		} else {
			i = strings.LastIndex(out, del)
		}
		if i < len(syncBegin) || out[i-len(syncBegin):i] != syncBegin {
			t.Errorf("%s transmission is not right after %q: %q", which, syncBegin, out[max(0, i-20):min(len(out), i+30)])
		}
		end := strings.Index(out[i:], syncEnd)
		if end < 0 || !strings.Contains(out[i:i+end], "a=t,i=") {
			t.Errorf("%s transmission is not inside the wrapper", which)
		}
	}
	r.in.Close()
	if err := r.wait(t); err != nil {
		t.Fatal(err)
	}
}
