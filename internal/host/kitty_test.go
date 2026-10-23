package host

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/abdul-hamid-achik/tuimark/internal/paint"
)

// The kitty graphics probe (v0.4d phase 3): the query goes out before
// the DA1 sentinel, and an APC _G answer naming i=31 that arrives before
// the sentinel records kittyGfx. A decoder-level check: canned bytes in,
// termCaps out.
func TestKittyProbeReply(t *testing.T) {
	var d decoder
	_, reps := d.feed([]byte("\x1b_Gi=31;OK\x1b\\\x1b[?62;1;2;22c"), true)
	var c termCaps
	for _, r := range reps {
		c.record(r)
	}
	if !c.done || !c.kittyGfx {
		t.Fatalf("caps = %+v, want done and kittyGfx", c)
	}
	// Without the graphics answer, or with an answer to another query id,
	// the capability stays off.
	for _, in := range []string{
		"\x1b[?62c",
		"\x1b_Gi=7;OK\x1b\\\x1b[?62c",
		"\x1b_Gi=31;OK\x1b\\", // no DA1 yet: not done, but recorded
	} {
		var d decoder
		_, reps := d.feed([]byte(in), true)
		var c termCaps
		for _, r := range reps {
			c.record(r)
		}
		if c.kittyGfx != strings.Contains(in, "i=31") {
			t.Errorf("%q: kittyGfx = %v", in, c.kittyGfx)
		}
		if c.done != strings.Contains(in, "\x1b[?62") {
			t.Errorf("%q: done = %v", in, c.done)
		}
	}
	// The query is written before everything else the probe writes.
	s := &termSession{}
	if q := s.queries(); !strings.HasPrefix(q, queryKittyGfx) || !strings.HasSuffix(q, queryDA1) {
		t.Errorf("queries = %q, want the graphics query first and DA1 last", q)
	}
	s = &termSession{themeQuery: true}
	if q := s.queries(); !strings.HasPrefix(q, queryBackground+queryKittyGfx) {
		t.Errorf("queries with OSC 11 = %q", q)
	}
}

// kittyUpload transmits an image once: the id is deleted first (Ghostty
// bug #13719), the transmission is a virtual placement (U=1) of c×r
// cells, quiet, PNG, base64 in m=1 chunks of at most 4096 bytes with one
// final m=0 chunk.
func TestKittyUploadFormat(t *testing.T) {
	im := solidImg(6, 6, color.RGBA{R: 255, A: 255})
	s := kittyUpload(42, 4, 2, im)
	if !strings.HasPrefix(s, "\x1b_Ga=d,d=i,i=42\x1b\\") {
		t.Errorf("upload does not delete the id first: %q", s[:40])
	}
	if !strings.Contains(s, "a=t,i=42,U=1,c=4,r=2,q=2,f=100,t=d,m=0;") {
		t.Errorf("upload head wrong: %q", s)
	}
	// Every APC payload is at most kittyChunk base64 bytes, the last (and
	// here only) chunk carries m=0, and it decodes to a PNG.
	n := 0
	for _, part := range strings.Split(s, "\x1b_G") {
		if part == "" || strings.HasPrefix(part, "a=d") {
			continue
		}
		n++
		i := strings.IndexByte(part, ';')
		if i < 0 || !strings.Contains(part[:i], "m=1") || len(part)-i-1 > kittyChunk+2 { // payload + ST
			if !strings.Contains(part[:i], "m=0") {
				t.Errorf("chunk %q is not m=0 or its payload exceeds %d bytes", part[:i], kittyChunk)
			}
		}
	}
	if n < 1 {
		t.Fatalf("no transmission chunk in %q", s)
	}
	// An incompressible (noise) image really is split into m=1 chunks.
	noise := noiseImg(300, 300)
	if n := strings.Count(kittyUpload(7, 100, 100, noise), "m=1;"); n < 1 {
		t.Errorf("no m=1 chunk for a noise image")
	}
}

// kittyDoc builds a document whose <img> is a 6×6 red PNG as a data URI.
// noiseImg builds an incompressible w×h image (random pixels), so its
// PNG does not shrink below one transmission chunk.
func noiseImg(w, h int) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	rnd := rand.New(rand.NewSource(1))
	for y := range h {
		for x := range w {
			var b [4]byte
			rnd.Read(b[:])
			b[3] = 255
			im.SetRGBA(x, y, color.RGBA{R: b[0], G: b[1], B: b[2], A: b[3]})
		}
	}
	return im
}

// ph is the placeholder code point as the strings of the output carry it.
const ph = string(rune(paint.Placeholder))

// kittyDoc builds the document of the img tests.
func kittyDoc(t *testing.T, imgAttrs string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, solidImg(6, 6, color.RGBA{R: 255, A: 255})); err != nil {
		t.Fatal(err)
	}
	return `<tui><screen id="s"><img src="data:image/png;base64,` +
		base64.StdEncoding.EncodeToString(buf.Bytes()) + `" ` + imgAttrs + `/></screen></tui>`
}

// A probed session transmits each photo-mode image once, before the
// frame diff that first shows its placeholders, and deletes every image
// when it leaves; a session inside tmux, or a document that forces
// quality="blocks", keeps the halfblocks of phase 2b (v0.4d phase 3).
func TestKittyUploadOnceAndDeleteOnExit(t *testing.T) {
	for _, c := range []struct {
		name    string
		attrs   string
		tmux    bool
		photo   bool
		profile paint.Profile
	}{
		{"photo", `width="4" height="2"`, false, true, paint.TrueColor},
		{"tmux degrades", `width="4" height="2"`, true, false, paint.TrueColor},
		{"blocks forced", `width="4" height="2" quality="blocks"`, false, false, paint.TrueColor},
		{"256 colors", `width="4" height="2"`, false, false, paint.ANSI256},
	} {
		r := startSession(t, kittyDoc(t, c.attrs), &termSession{probe: true, wait: 10 * time.Second, tmux: c.tmux, profile: c.profile})
		r.waitOut(t, "\x1b[c")
		r.send(t, "\x1b_Gi=31;OK\x1b\\\x1b[?62c")
		// The first frame is drawn once the probe ends: its placeholders
		// (or, degraded, its halfblocks) are in the output.
		if c.photo {
			r.waitOut(t, "U=1,c=4,r=2,q=2")
		} else {
			r.waitOut(t, "▀")
		}
		r.in.Close()
		if err := r.wait(t); err != nil {
			t.Fatal(err)
		}
		out := r.out.String()
		if uploads := strings.Count(out, "a=t,i="); c.photo && uploads != 1 {
			t.Errorf("%s: %d transmissions, want 1", c.name, uploads)
		}
		if !c.photo && strings.Contains(out, "\x1b_Ga=") {
			t.Errorf("%s: a graphics sequence was written: %q", c.name, out)
		}
		if c.photo {
			if !strings.Contains(out, "U=1,c=4,r=2,q=2") {
				t.Errorf("%s: no virtual placement of 4x2 cells", c.name)
			}
			if !strings.Contains(out, ph) {
				t.Errorf("%s: no placeholder cell written", c.name)
			}
			if !strings.HasSuffix(out, kittyDeleteAll+leaveScreen) {
				t.Errorf("%s: leave = %q, want the image delete before the leave screen", c.name, out[max(0, len(out)-40):])
			}
		} else if strings.HasSuffix(out, kittyDeleteAll+leaveScreen) {
			t.Errorf("%s: images deleted although none was sent", c.name)
		}
	}
}

// The app-level selection: with the session's photo flag set, a Dump of
// the same document carries placeholder cells; without it (every
// headless command), or with quality="blocks", the halfblocks.
func TestPhotoModeSelection(t *testing.T) {
	for _, c := range []struct {
		attrs string
		photo bool
		want  string
	}{
		{`width="4" height="2"`, true, ph},
		{`width="4" height="2" quality="photo"`, true, ph},
		{`width="4" height="2" quality="blocks"`, true, "▀"},
		{`width="4" height="2"`, false, "▀"},
	} {
		a := doc(t, kittyDoc(t, c.attrs))
		a.mu.Lock()
		a.imgPhoto = c.photo
		a.mu.Unlock()
		d := a.Dump(40, 24, true)
		if len(d.Errors) != 0 {
			t.Fatalf("errors: %v", d.Errors)
		}
		found := false
		for _, cell := range d.Cells {
			if cell.Ch == c.want {
				found = true
			}
			if cell.Ch == ph && c.want != ph {
				t.Errorf("%s: a placeholder cell leaked into a blocks frame", c.attrs)
			}
		}
		if !found {
			t.Errorf("%s (photo %v): no %q cell in the dump", c.attrs, c.photo, c.want)
		}
	}
}
