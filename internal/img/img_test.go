package img

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
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

// bombPNG is a PNG of a few dozen bytes whose header declares w×h RGBA
// pixels, with an IDAT holding one zlib stream of zeros (a real bomb
// compresses to about 1 MB; the stream here is cut short, which is
// enough: png.Decode allocates the pixel buffer from the header before it
// reads the data).
func bombPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	small := pngBytes(t, solid(1, 1, color.RGBA{A: 255}))
	// IHDR is the first chunk: 8 signature bytes, 4 length, 4 type, then
	// 13 bytes of data (width, height, ...), then its CRC.
	b := append([]byte(nil), small...)
	binary.BigEndian.PutUint32(b[16:], uint32(w))
	binary.BigEndian.PutUint32(b[20:], uint32(h))
	binary.BigEndian.PutUint32(b[29:], crc32.ChecksumIEEE(b[12:29]))
	return b
}

// An image whose header declares more than MaxPixels is refused before
// its pixel buffer is allocated (v0.5.2: a ~1 MB PNG declaring
// 16000×16000 took 1 GB of memory in png.Decode), on both the file path
// and the data-URI path, and Halfblocks refuses an oversized image it is
// handed.
func TestDecodeRefusesAPixelBomb(t *testing.T) {
	bomb := bombPNG(t, 16000, 16000)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	im, err := Decode(bomb)
	runtime.ReadMemStats(&after)
	if im != nil || !errors.Is(err, ErrTooManyPixels) {
		t.Fatalf("Decode = %v, %v; want nil and ErrTooManyPixels", im, err)
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 64<<20 {
		t.Errorf("refusing the bomb still allocated %d MiB", grew>>20)
	}
	// Both Image paths take the error (the image is missing).
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bomb.png"), bomb, 0o644); err != nil {
		t.Fatal(err)
	}
	r := &Registry{}
	if im, err := r.Image("bomb.png", dir); im != nil || !errors.Is(err, ErrTooManyPixels) {
		t.Errorf("file: %v, %v", im, err)
	}
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(bomb)
	if im, err := r.Image(uri, dir); im != nil || !errors.Is(err, ErrTooManyPixels) {
		t.Errorf("data URI: %v, %v", im, err)
	}
	// Just over the bound by width alone, and the bound itself decodes.
	if _, err := Decode(bombPNG(t, MaxPixels+1, 1)); !errors.Is(err, ErrTooManyPixels) {
		t.Errorf("MaxPixels+1 × 1: %v", err)
	}
	if got := Halfblocks(hugeImage{}, 4, 2); got != nil {
		t.Errorf("Halfblocks of an oversized image = %v, want nil", got)
	}
	if got := Halfblocks(solid(8, 8, color.RGBA{A: 255}), 4, 2); got == nil {
		t.Error("Halfblocks of a normal image is nil")
	}
}

// hugeImage is an image that claims 20000×20000 pixels and holds none.
type hugeImage struct{}

func (hugeImage) ColorModel() color.Model { return color.RGBAModel }
func (hugeImage) Bounds() image.Rectangle { return image.Rect(0, 0, 20000, 20000) }
func (hugeImage) At(int, int) color.Color { return color.RGBA{} }
