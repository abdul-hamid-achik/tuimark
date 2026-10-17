//go:build linux

package host

import (
	"unsafe"

	"github.com/abdul-hamid-achik/tuimark/internal/layout"
	"golang.org/x/sys/unix"
)

// hugePage is the size of a transparent huge page on Linux (x86-64 and
// arm64 with 4 KiB base pages).
const hugePage = 2 << 20

// adviseHuge asks the kernel to back the whole 2 MiB pages inside b's
// memory with transparent huge pages (madvise MADV_HUGEPAGE), before
// anything is written there: every pass of a frame walks a long list's
// slab from end to end, and one TLB entry then covers 2 MiB of it
// instead of 4 KiB. It changes nothing a frame holds; when the kernel
// refuses, or has no transparent huge pages, the memory stays as it was.
func adviseHuge(b []layout.Box) {
	if NoHugeForBench || cap(b) == 0 {
		return
	}
	p := unsafe.Pointer(unsafe.SliceData(b[:cap(b)]))
	size := uintptr(cap(b)) * unsafe.Sizeof(layout.Box{})
	if size < 2*hugePage {
		return
	}
	start := uintptr(p)
	skip := (hugePage - start%hugePage) % hugePage
	n := (size - skip) &^ (hugePage - 1)
	if n == 0 {
		return
	}
	_ = unix.Madvise(unsafe.Slice((*byte)(unsafe.Add(p, skip)), n), unix.MADV_HUGEPAGE)
}
