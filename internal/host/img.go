package host

import (
	"image"

	"github.com/abdul-hamid-achik/tuimark/internal/img"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/layout"
)

// imageOf returns the decoded image an <img> names (v0.4d phase 2b):
// its src, resolved against the document's directory (a.path) when it is
// a relative path, or the bytes of its data:image/…;base64,… URI, decoded
// once per app through the img registry (cached by src, a file re-decoded
// when its mtime or size changed). A src that is missing, unreadable, or
// not a png/jpeg/gif/webp image gives nil: the box collapses to nothing
// and paint skips it — the frame's diagnostics stay as they are (an
// <img> without src is already a parse-time V003).
func (a *App) imageOf(src string) image.Image {
	if src == "" {
		return nil
	}
	if a.images == nil {
		a.images = &img.Registry{}
	}
	im, err := a.images.Image(src, a.dir)
	if err != nil {
		return nil
	}
	return im
}

// inflateImg sets an img box's decoded image and its pixel size, the
// inputs its layout (natural size) and paint (halfblocks) read, and the
// photo-mode flags of phase 3: the src as written (the identity of the
// transmitted kitty image) and whether this session renders photo
// quality — the session's probe found the kitty graphics protocol on a
// truecolor terminal outside tmux, and the document did not force
// quality="blocks" (quality="photo" is the same as the default "auto"
// here: without the protocol there is nothing to force).
func (fb *builder) inflateImg(n *ir.Node, b *layout.Box, sc *scope) {
	src := fb.interp(n, "src", n.Attrs["src"], sc)
	b.Img = fb.a.imageOf(src)
	b.ImgSrc = src
	b.ImgPhoto = fb.a.imgPhoto && n.Attrs["quality"] != "blocks"
	if b.Img == nil {
		return
	}
	sz := b.Img.Bounds()
	b.ImgW, b.ImgH = sz.Dx(), sz.Dy()
}
