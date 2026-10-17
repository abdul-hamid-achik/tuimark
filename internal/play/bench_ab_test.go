package play

// TEMPORARY: same-runner A/B of listRows.reuse for the CI benchmark job,
// interleaved, removed before the branch is done.

import (
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
)

func abDown(b *testing.B, off bool) {
	host.NoReuseForBench = off
	defer func() { host.NoReuseForBench = false }()
	benchDown(b, benchList)
}

func abTick(b *testing.B, off bool) {
	host.NoReuseForBench = off
	defer func() { host.NoReuseForBench = false }()
	BenchmarkBatchTick20k(b)
}

func BenchmarkZZ1DownReuse(b *testing.B)   { abDown(b, false) }
func BenchmarkZZ1DownNoReuse(b *testing.B) { abDown(b, true) }
func BenchmarkZZ2DownReuse(b *testing.B)   { abDown(b, false) }
func BenchmarkZZ2DownNoReuse(b *testing.B) { abDown(b, true) }
func BenchmarkZZ3DownReuse(b *testing.B)   { abDown(b, false) }
func BenchmarkZZ3DownNoReuse(b *testing.B) { abDown(b, true) }
func BenchmarkZZ4TickReuse(b *testing.B)   { abTick(b, false) }
func BenchmarkZZ4TickNoReuse(b *testing.B) { abTick(b, true) }
func BenchmarkZZ5TickReuse(b *testing.B)   { abTick(b, false) }
func BenchmarkZZ5TickNoReuse(b *testing.B) { abTick(b, true) }
func BenchmarkZZ6TickReuse(b *testing.B)   { abTick(b, false) }
func BenchmarkZZ6TickNoReuse(b *testing.B) { abTick(b, true) }
