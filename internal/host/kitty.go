package host

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"

	"github.com/abdul-hamid-achik/tuimark/internal/paint"
)

// The transmission side of the kitty graphics protocol (v0.4d phase 3):
// what the session writes before the frame diff that first shows a
// photo-mode <img> (paint's placeholder cells name the image by id, and
// the id exists only once the terminal holds the image). See
// sw.kovidgoyal.net/kitty/graphics-protocol (transmission and Unicode
// placeholders).

// kittyChunk is the largest base64 payload one APC transmission carries
// (the protocol's limit).
const kittyChunk = 4096

// kittyUpload builds the one-time transmission of one photo-mode image:
// a delete of the id first (a terminal that still holds a placement of
// it must not blend the new one into it — Ghostty bug #13719), then the
// image as PNG (f=100), in a virtual placement (U=1) of c columns and
// r rows, quiet (q=2, no reply can confuse the host application),
// base64-chunked with m=1 except the last chunk's m=0. im is the decoded
// source; it is re-encoded, so every format internal/img reads
// (png/jpeg/gif/webp) transmits.
func kittyUpload(id int32, c, r int, im image.Image) string {
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		return "" // an image that decoded cannot fail to re-encode; guard anyway
	}
	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())
	head := fmt.Sprintf("a=t,i=%d,U=1,c=%d,r=%d,q=2,f=100,t=d", id, c, r)
	out := fmt.Sprintf("\x1b_Ga=d,d=i,i=%d\x1b\\", id)
	for len(b64) > kittyChunk {
		out += "\x1b_G" + head + ",m=1;" + b64[:kittyChunk] + "\x1b\\"
		b64 = b64[kittyChunk:]
	}
	return out + "\x1b_G" + head + ",m=0;" + b64 + "\x1b\\"
}

// kittyUploads builds the transmissions of the images of frame g that
// were not sent yet, marking each sent; "" when nothing is new. The
// session writes them before the frame's diff bytes, outside the
// synchronized-output wrapper, and marks the session so leaving deletes
// every image (termSession.setKittyImgs). Caller holds a.mu.
func (a *App) kittyUploads(g *paint.Grid, sent map[int32]bool) string {
	var out bytes.Buffer
	for _, k := range g.Imgs {
		if sent[k.ID] || a.images == nil {
			continue
		}
		im, err := a.images.Image(k.Src, a.dir)
		if err != nil || im == nil {
			continue
		}
		sent[k.ID] = true
		out.WriteString(kittyUpload(k.ID, k.W, k.H, im))
	}
	return out.String()
}
