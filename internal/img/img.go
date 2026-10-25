// Package img decodes the images an <img src="…"> names and caches them,
// and resizes them to the pixel grid a halfblock paint reads.
//
// A src is either a data:image/png;base64,… / data:image/jpeg;base64,…
// URI, decoded inline, or a filesystem path, resolved against the
// document's directory. Decoding follows the magic bytes, never the
// suffix: png, jpeg, and gif from the standard library, webp from
// golang.org/x/image/webp (webp is everywhere on the web).
package img

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

// Registry is the per-app cache of decoded images, keyed by src (a data
// URI as written, a path as resolved). A path entry is re-decoded when
// the file's mtime or size changed since it was read. It mirrors the
// layout engine's memo pattern: one registry per running app, consulted
// on every frame, so a source painted twice decodes once.
type Registry struct {
	mu     sync.Mutex
	images map[string]cachedImage
	// decodes counts the actual decodes (data URIs and files), the test
	// hook for "the same src decodes once".
	decodes int
}

type cachedImage struct {
	im        image.Image
	mtime     time.Time // the file's mtime when read (zero for a data URI)
	size      int64     // the file's size when read (0 for a data URI)
	isDataURI bool
}

// Decodes returns how many decodes this registry performed.
func (r *Registry) Decodes() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.decodes
}

const dataURIPrefix = "data:image/"

// Image returns the decoded image src names. dir is the directory a
// relative path resolves against (the document's own directory); a data
// URI resolves nowhere.
func (r *Registry) Image(src, dir string) (image.Image, error) {
	if src == "" {
		return nil, errors.New("img: empty src")
	}
	if strings.HasPrefix(src, dataURIPrefix) {
		return r.dataURI(src)
	}
	path := src
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("img %s: %w", src, err)
	}
	r.mu.Lock()
	if r.images == nil {
		r.images = map[string]cachedImage{}
	} else if c, ok := r.images[path]; ok && c.mtime.Equal(st.ModTime()) && c.size == st.Size() && !c.isDataURI {
		r.mu.Unlock()
		return c.im, nil
	}
	r.mu.Unlock()
	// A data-bound src can name any path: a FIFO or /dev/zero must be an
	// error (the image's nil fallback), not a hang or an unbounded read.
	data, err := ReadRegular(path, 0)
	if err != nil {
		return nil, fmt.Errorf("img %s: %w", src, err)
	}
	im, err := Decode(data)
	if err != nil {
		return nil, fmt.Errorf("img %s: %w", src, err)
	}
	r.mu.Lock()
	r.images[path] = cachedImage{im: im, mtime: st.ModTime(), size: st.Size()}
	r.decodes++
	r.mu.Unlock()
	return im, nil
}

func (r *Registry) dataURI(src string) (image.Image, error) {
	// data:image/png;base64,XXXX — the media type names what the magic
	// bytes confirm. The cache is checked before the base64 is decoded:
	// a URI never changes, so one decode serves every frame.
	r.mu.Lock()
	if r.images == nil {
		r.images = map[string]cachedImage{}
	} else if c, ok := r.images[src]; ok {
		r.mu.Unlock()
		return c.im, nil
	}
	r.mu.Unlock()
	rest := src[len(dataURIPrefix):]
	semi := strings.IndexByte(rest, ';')
	if semi < 0 || !strings.HasPrefix(rest[semi:], ";base64,") {
		return nil, fmt.Errorf("img data URI %q: want data:image/png|jpeg;base64,…", clip(src))
	}
	data, err := base64.StdEncoding.DecodeString(rest[semi+len(";base64,"):])
	if err != nil {
		return nil, fmt.Errorf("img data URI %q: %w", clip(src), err)
	}
	im, err := Decode(data)
	if err != nil {
		return nil, fmt.Errorf("img data URI %q: %w", clip(src), err)
	}
	r.mu.Lock()
	if r.images == nil {
		r.images = map[string]cachedImage{}
	}
	r.images[src] = cachedImage{im: im, isDataURI: true}
	r.decodes++
	r.mu.Unlock()
	return im, nil
}

// clip shortens a src for a diagnostic.
func clip(src string) string {
	if len(src) > 40 {
		return src[:37] + "…"
	}
	return src
}

// MaxPixels bounds the pixels of an image Decode accepts (32 Mi): the
// decoders allocate width×height×4 bytes (or more) from a header a few
// bytes long, so the byte cap on the file does not bound the memory a
// small file can ask for (a 1 MB PNG can declare 16000×16000).
const MaxPixels = 32 << 20

// ErrTooManyPixels is Decode's refusal of an image that declares more
// than MaxPixels; the image is then missing like any unreadable one.
var ErrTooManyPixels = errors.New("image has too many pixels")

// Decode decodes encoded image bytes by their magic bytes: png, jpeg,
// gif (the standard library) and webp (golang.org/x/image/webp). The
// declared size is read first (DecodeConfig, header only) and an image of
// more than MaxPixels is refused before any pixel buffer is allocated.
func Decode(data []byte) (image.Image, error) {
	var (
		cfg    func(io.Reader) (image.Config, error)
		decode func(io.Reader) (image.Image, error)
	)
	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		cfg, decode = png.DecodeConfig, png.Decode
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		cfg, decode = jpeg.DecodeConfig, jpeg.Decode
	case bytes.HasPrefix(data, []byte("GIF8")):
		cfg, decode = gif.DecodeConfig, gif.Decode
	case len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		cfg, decode = webp.DecodeConfig, webp.Decode
	default:
		return nil, errors.New("not a png, jpeg, gif, or webp image")
	}
	c, err := cfg(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if c.Width <= 0 || c.Height <= 0 || int64(c.Width)*int64(c.Height) > MaxPixels {
		return nil, fmt.Errorf("%w (%d×%d, over %d)", ErrTooManyPixels, c.Width, c.Height, MaxPixels)
	}
	return decode(bytes.NewReader(data))
}

// halfKey identifies one resized buffer: the source image (decoded
// images are pointer values, so this is the identity of the decode) and
// the cell size it was resized to.
type halfKey struct {
	im   image.Image
	w, h int
}

var (
	halfMu    sync.Mutex
	halfCache = map[halfKey]*image.RGBA{}
)

// halfCacheMax bounds the resize cache (each entry is a small W×2H RGBA
// buffer); when full it is dropped whole, a one-off re-resize of what is
// painted next.
const halfCacheMax = 64

// Halfblocks resizes im to the pixel grid of a w×h cell halfblock paint:
// w pixels wide, 2h tall (a cell is about twice as tall as wide), with
// x/image/draw's ApproxBiLinear filter. The buffer is cached per (image,
// w, h), so a frame that paints the same source at the same size twice,
// or every frame of a running app, resizes once.
func Halfblocks(im image.Image, w, h int) *image.RGBA {
	if im == nil || w <= 0 || h <= 0 {
		return nil
	}
	// An image that did not come through Decode may be any size; the
	// resize reads every source pixel it scales from.
	if b := im.Bounds(); int64(b.Dx())*int64(b.Dy()) > MaxPixels {
		return nil
	}
	k := halfKey{im: im, w: w, h: h}
	halfMu.Lock()
	if buf, ok := halfCache[k]; ok {
		halfMu.Unlock()
		return buf
	}
	halfMu.Unlock()
	buf := image.NewRGBA(image.Rect(0, 0, w, 2*h))
	xd := draw.ApproxBiLinear
	xd.Scale(buf, buf.Bounds(), im, im.Bounds(), draw.Src, nil)
	halfMu.Lock()
	if len(halfCache) >= halfCacheMax {
		halfCache = map[halfKey]*image.RGBA{}
	}
	halfCache[k] = buf
	halfMu.Unlock()
	return buf
}
