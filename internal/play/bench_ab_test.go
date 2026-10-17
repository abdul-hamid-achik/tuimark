package play

// TEMPORARY: same-runner A/B of huge pages for the slabs (adviseHuge) in
// the CI benchmark job, interleaved, removed before the branch is done.

import (
	"testing"

	"github.com/abdul-hamid-achik/tuimark/internal/host"
)

func abRun(b *testing.B, off bool, f func(*testing.B)) {
	host.NoHugeForBench = off
	defer func() { host.NoHugeForBench = false }()
	f(b)
}

func down(b *testing.B) { benchDown(b, benchList) }

func BenchmarkZZ1DownHuge(b *testing.B)     { abRun(b, false, down) }
func BenchmarkZZ1DownNoHuge(b *testing.B)   { abRun(b, true, down) }
func BenchmarkZZ2DownNoHuge(b *testing.B)   { abRun(b, true, down) }
func BenchmarkZZ2DownHuge(b *testing.B)     { abRun(b, false, down) }
func BenchmarkZZ3DownHuge(b *testing.B)     { abRun(b, false, down) }
func BenchmarkZZ3DownNoHuge(b *testing.B)   { abRun(b, true, down) }
func BenchmarkZZ4TickHuge(b *testing.B)     { abRun(b, false, BenchmarkBatchTick20k) }
func BenchmarkZZ4TickNoHuge(b *testing.B)   { abRun(b, true, BenchmarkBatchTick20k) }
func BenchmarkZZ5TickNoHuge(b *testing.B)   { abRun(b, true, BenchmarkBatchTick20k) }
func BenchmarkZZ5TickHuge(b *testing.B)     { abRun(b, false, BenchmarkBatchTick20k) }
func BenchmarkZZ6DeriveHuge(b *testing.B)   { abRun(b, false, BenchmarkDeriveFilterText20k) }
func BenchmarkZZ6DeriveNoHuge(b *testing.B) { abRun(b, true, BenchmarkDeriveFilterText20k) }
func BenchmarkZZ7DeriveNoHuge(b *testing.B) { abRun(b, true, BenchmarkDeriveFilterText20k) }
func BenchmarkZZ7DeriveHuge(b *testing.B)   { abRun(b, false, BenchmarkDeriveFilterText20k) }
