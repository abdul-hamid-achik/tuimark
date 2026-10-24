package host

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/dump"
)

// The host decodes an <img> through the app's img registry: the same src
// in two imgs decodes once, and the dump carries the img node with its
// halfblock cells (v0.4d phase 2b).
// solidImg builds a w×h image of one color.
func solidImg(w, h int, c color.RGBA) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			im.SetRGBA(x, y, c)
		}
	}
	return im
}

func TestImgDecodesOncePerSrc(t *testing.T) {
	dir := t.TempDir()
	var b bytes.Buffer
	if err := png.Encode(&b, solidImg(8, 8, color.RGBA{R: 255, A: 255})); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dot.png"), b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "doc.tui"), []byte(`<tui><screen>
  <col>
    <img id="a" src="dot.png" width="4" height="2"/>
    <img id="b" src="dot.png" width="4" height="2"/>
  </col>
</screen></tui>`), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Load(filepath.Join(dir, "doc.tui"))
	if err != nil {
		t.Fatal(err)
	}
	d := a.Dump(40, 24, true)
	if len(d.Errors) != 0 {
		t.Fatalf("errors: %v", d.Errors)
	}
	if a.images == nil || a.images.Decodes() != 1 {
		t.Fatalf("decodes = %v, want 1 (both imgs share the src's decode)", a.images)
	}
	var imgs int
	for _, n := range d.Nodes {
		if n.Tag == "img" {
			imgs++
			if n.W == 0 || n.H == 0 {
				t.Errorf("img %q %dx%d, want a natural size", n.ID, n.W, n.H)
			}
		}
	}
	if imgs != 2 {
		t.Fatalf("dump holds %d img nodes, want 2", imgs)
	}
	var halves int
	for _, c := range d.Cells {
		if c.Ch == "▀" {
			halves++
		}
	}
	if halves == 0 {
		t.Error("no ▀ cell in the dump")
	}
	// A second frame decodes nothing new.
	a.Dump(40, 24, false)
	if a.images.Decodes() != 1 {
		t.Errorf("decodes after a second frame = %d, want 1", a.images.Decodes())
	}
}

// A data: URI src works end to end, and a src that does not resolve
// leaves an empty box (no diagnostic, nothing painted).
func TestImgDataURIAndMissingSrc(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, solidImg(4, 4, color.RGBA{G: 255, A: 255})); err != nil {
		t.Fatal(err)
	}
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.tui")
	src := `<tui><screen>
  <col>
    <img id="inline" src="` + uri + `"/>
    <img id="gone" src="missing.png"/>
  </col>
</screen></tui>`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	d := a.Dump(20, 24, false)
	if len(d.Errors) != 0 {
		t.Fatalf("errors: %v", d.Errors)
	}
	byID := map[string]dump.Node{}
	for _, n := range d.Nodes {
		byID[n.ID] = n
	}
	if n := byID["inline"]; n.W == 0 || n.H == 0 {
		t.Errorf("inline img %dx%d, want a natural size", n.W, n.H)
	}
	// A src that does not resolve has no natural height (its width may
	// still stretch to the column's, like an empty text's).
	if n := byID["gone"]; n.H != 0 {
		t.Errorf("missing src img %dx%d, want height 0", n.W, n.H)
	}
}

// Data-bound src and href (0.5.1): {path} interpolates before the image
// resolves and before the link is registered, sprite-style.
func TestImgAndHrefDataBound(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, solidImg(2, 2, color.RGBA{R: 255, A: 255})); err != nil {
		t.Fatal(err)
	}
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.tui")
	src := `<tui><screen>
  <col>
    <text href="{link.url}">read</text>
    <img id="bound" src="{img.src}"/>
  </col>
</screen></tui>`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(dir, "data.json")
	payload, _ := json.Marshal(map[string]any{
		"link": map[string]any{"url": "https://tuimark.dev/x"},
		"img":  map[string]any{"src": uri},
	})
	if err := os.WriteFile(data, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	store, err := os.ReadFile(data)
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	if err := json.Unmarshal(store, &values); err != nil {
		t.Fatal(err)
	}
	if err := a.Batch(func(b *Batch) error {
		for k, v := range values {
			if err := b.Set(k, v); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The 2x2 image's natural height exceeds the 10-row screen: the clip
	// warning is expected, nothing else.
	d := a.Dump(40, 10, false)
	for _, e := range d.Errors {
		if e.Code != "L009" {
			t.Fatalf("errors: %v", d.Errors)
		}
	}
	var n dump.Node
	for _, node := range d.Nodes {
		if node.ID == "bound" {
			n = node
		}
	}
	if n.H == 0 {
		t.Errorf("bound img %dx%d, want a natural size from the data URI", n.W, n.H)
	}
	// The link cells carry an index: the href interpolated and registered.
	g := a.Dump(40, 10, true)
	linked := 0
	for _, c := range g.Cells {
		if c.Link != 0 {
			linked++
		}
	}
	if linked == 0 {
		t.Error("no cell carries a link index for the data-bound href")
	}
}
