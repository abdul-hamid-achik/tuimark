package img

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// pngBytes encodes im as the png bytes of a test source.
func pngBytes(t *testing.T, im image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// solid is a w×h image of one color.
func solid(w, h int, c color.RGBA) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			im.SetRGBA(x, y, c)
		}
	}
	return im
}

// base64Of encodes b as the tail of a data URI.
func base64Of(t *testing.T, b []byte) string {
	t.Helper()
	return base64.StdEncoding.EncodeToString(b)
}

func TestDecodeSniffsMagicBytes(t *testing.T) {
	png := pngBytes(t, solid(1, 1, color.RGBA{R: 255}))
	if _, err := Decode(png); err != nil {
		t.Errorf("png: %v", err)
	}
	if _, err := Decode(append([]byte{}, png...)); err != nil {
		t.Errorf("png again: %v", err)
	}
	if _, err := Decode([]byte("not an image")); err == nil {
		t.Error("garbage decoded")
	}
	if _, err := Decode(nil); err == nil {
		t.Error("empty decoded")
	}
}

func TestRegistryDataURI(t *testing.T) {
	uri := "data:image/png;base64," + base64Of(t, pngBytes(t, solid(2, 2, color.RGBA{G: 255})))
	r := &Registry{}
	im, err := r.Image(uri, "/nowhere")
	if err != nil {
		t.Fatalf("data URI: %v", err)
	}
	if im.Bounds().Dx() != 2 || im.Bounds().Dy() != 2 {
		t.Fatalf("bounds %v", im.Bounds())
	}
	// The same URI twice decodes once.
	if _, err := r.Image(uri, "/nowhere"); err != nil {
		t.Fatal(err)
	}
	if r.Decodes() != 1 {
		t.Errorf("decodes = %d, want 1", r.Decodes())
	}
	// A URI that is not base64 png is an error, not a panic.
	if _, err := r.Image("data:image/png;base64,!!!", ""); err == nil {
		t.Error("bad base64 decoded")
	}
	if _, err := r.Image("data:image/webp;base64,"+base64Of(t, []byte("x")), ""); err == nil {
		t.Error("wrong media type decoded")
	}
}

func TestRegistryFileCacheAndMtime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.png")
	if err := os.WriteFile(path, pngBytes(t, solid(4, 4, color.RGBA{R: 255})), 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Registry{}
	for range 3 {
		im, err := r.Image("a.png", dir)
		if err != nil {
			t.Fatal(err)
		}
		if im.Bounds().Dx() != 4 {
			t.Fatalf("bounds %v", im.Bounds())
		}
	}
	if r.Decodes() != 1 {
		t.Errorf("decodes = %d, want 1 (cached by src)", r.Decodes())
	}
	// A changed file (new mtime and size) is decoded again.
	if err := os.WriteFile(path, pngBytes(t, solid(8, 8, color.RGBA{B: 255})), 0o644); err != nil {
		t.Fatal(err)
	}
	im, err := r.Image("a.png", dir)
	if err != nil {
		t.Fatal(err)
	}
	if im.Bounds().Dx() != 8 || r.Decodes() != 2 {
		t.Errorf("after rewrite: bounds %v, decodes %d, want 8 wide, 2", im.Bounds(), r.Decodes())
	}
	// A missing src is an error, never a panic.
	if _, err := r.Image("nope.png", dir); err == nil {
		t.Error("missing file decoded")
	}
}

func TestDecodeWebP(t *testing.T) {
	data, err := os.ReadFile("testdata/tiny.webp")
	if err != nil {
		t.Fatal(err)
	}
	im, err := Decode(data)
	if err != nil {
		t.Fatalf("webp: %v", err)
	}
	if im.Bounds().Dx() != 1 || im.Bounds().Dy() != 1 {
		t.Fatalf("bounds %v, want 1x1", im.Bounds())
	}
}

func TestHalfblocksCache(t *testing.T) {
	im := solid(16, 32, color.RGBA{R: 255})
	a := Halfblocks(im, 2, 1)
	if a == nil || a.Bounds().Dx() != 2 || a.Bounds().Dy() != 2 {
		t.Fatalf("buffer bounds %v, want 2x2", a.Bounds())
	}
	if b := Halfblocks(im, 2, 1); b != a {
		t.Error("same (image, w, h) resized again")
	}
	if c := Halfblocks(im, 3, 1); c == a {
		t.Error("another size shares the buffer")
	}
	if Halfblocks(nil, 2, 1) != nil || Halfblocks(im, 0, 1) != nil || Halfblocks(im, 2, 0) != nil {
		t.Error("an empty request returned a buffer")
	}
}

// An image file of more than MaxReadBytes is an error, not a 64 MiB+ read
// into memory (v0.5.2: Image used os.ReadFile). A sparse file of one byte
// over the bound keeps the test cheap on disk.
func TestImageRefusesAnOversizedFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "huge.png")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(pngBytes(t, solid(2, 2, color.RGBA{A: 255}))); err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxReadBytes + 1); err != nil {
		t.Skipf("no sparse files: %v", err)
	}
	f.Close()
	im, err := (&Registry{}).Image("huge.png", dir)
	if im != nil || !errors.Is(err, ErrTooLarge) {
		t.Fatalf("image %v, err %v; want nil and ErrTooLarge", im, err)
	}
	// A regular file still decodes.
	ok := filepath.Join(dir, "ok.png")
	if err := os.WriteFile(ok, pngBytes(t, solid(2, 2, color.RGBA{A: 255})), 0o644); err != nil {
		t.Fatal(err)
	}
	if im, err := (&Registry{}).Image("ok.png", dir); err != nil || im == nil {
		t.Fatalf("a regular file: %v, %v", im, err)
	}
}
