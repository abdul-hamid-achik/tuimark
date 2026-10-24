package host

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"slices"

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

// kittyDelete frees image id: its placements (the virtual one included)
// and its data. The uppercase d=I is what frees the data; the lowercase
// d=i would only remove the placements and leave the pixels in the
// terminal's memory. Quiet, like the transmission.
func kittyDelete(id int32) string {
	return fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id)
}

// kittyUpload builds the one-time transmission of one photo-mode image:
// a delete of the id first (a terminal that still holds the image or a
// placement of it must not blend the new one into it — Ghostty bug
// #13719 — and a retransmission replaces the old data), then the image
// as PNG (f=100), transmitted and displayed in one command (a=T) as a
// virtual placement (U=1) of c columns and r rows. a=T, not a=t: a=t only
// stores the image, and the protocol documents a virtual placement only as
// a separate a=p,U=1 after a transmit or combined with it as a=T,…,U=1, so
// with a=t the placeholder cells would name an image that has no
// placement and stay blank. Quiet (q=2, no reply can confuse the host application),
// base64-chunked with m=1 except the last chunk's m=0. Only the first
// chunk carries the control keys: the protocol allows nothing but m (and
// q) on a continuation chunk. im is the decoded source; it is
// re-encoded, so every format internal/img reads (png/jpeg/gif/webp)
// transmits.
func kittyUpload(id int32, c, r int, im image.Image) string {
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		return "" // an image that decoded cannot fail to re-encode; guard anyway
	}
	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())
	ctl := fmt.Sprintf("a=T,i=%d,U=1,c=%d,r=%d,q=2,f=100,t=d", id, c, r)
	out := kittyDelete(id)
	for len(b64) > kittyChunk {
		out += "\x1b_G" + ctl + ",m=1;" + b64[:kittyChunk] + "\x1b\\"
		b64 = b64[kittyChunk:]
		ctl = "q=2"
	}
	return out + "\x1b_G" + ctl + ",m=0;" + b64 + "\x1b\\"
}

// kittyUploads builds what the terminal needs before frame g's diff:
// the images g no longer shows are freed, and the images g shows that
// the terminal does not hold, or holds in an older version, are
// transmitted. held maps the ids the terminal holds to the decoded image
// that was sent: the registry gives a new decode when a file's content
// changed (mtime or size), so a different image under a held id is a
// retransmission under the same id, which the placeholder cells already
// name. held is updated; added and freed list the ids touched, for the
// session's bookkeeping. The session writes the result before the
// frame's diff bytes (inside the synchronized-output wrapper when the
// frame has one). An image that cannot be read at this moment (the file
// vanished between paint and here) counts as not shown: its id is freed
// and the image is transmitted again on the next frame that reads fine,
// which is intended. Caller holds a.mu.
func (a *App) kittyUploads(g *paint.Grid, held map[int32]image.Image) (out string, added, freed []int32) {
	shown := map[int32]bool{}
	var sends bytes.Buffer
	for _, k := range g.Imgs {
		if a.images == nil {
			continue
		}
		im, err := a.images.Image(k.Src, a.dir)
		if err != nil || im == nil {
			continue
		}
		shown[k.ID] = true
		// Interface equality is pointer identity here: every decoder
		// internal/img uses (png, jpeg, gif, webp) returns a pointer
		// type (*image.RGBA, *image.NRGBA, *image.YCbCr, *image.Paletted,
		// …), and the registry hands back the same pointer until the
		// file changes. A decoder returning a non-comparable image value
		// would panic here.
		if held[k.ID] == im {
			continue
		}
		up := kittyUpload(k.ID, k.W, k.H, im)
		if up == "" {
			continue
		}
		held[k.ID] = im
		added = append(added, k.ID)
		sends.WriteString(up)
	}
	var frees bytes.Buffer
	for id := range held {
		if !shown[id] {
			freed = append(freed, id)
		}
	}
	slices.Sort(freed)
	for _, id := range freed {
		delete(held, id)
		frees.WriteString(kittyDelete(id))
	}
	return frees.String() + sends.String(), added, freed
}
