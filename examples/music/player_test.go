package main

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

// clockAt is the fake clock the player tests use.
func clockAt(ms int) time.Time { return epoch.Add(time.Duration(ms) * time.Millisecond) }

// playing returns a player that is playing the track with this id from the
// start, its clock set at time zero.
func playing(t *testing.T, seed int64, id string) *player {
	t.Helper()
	p := newPlayer(seed)
	if !p.playID(id) {
		t.Fatalf("no track %s", id)
	}
	p.step(clockAt(0))
	return p
}

func maxBand(p *player) int {
	m := 0
	for _, v := range p.bands {
		if v > m {
			m = v
		}
	}
	return m
}

func TestStepAdvancesThePosition(t *testing.T) {
	p := playing(t, 1, "t01")
	p.step(clockAt(1000))
	if p.posMs != 1000 {
		t.Errorf("after 1 s, pos = %d ms, want 1000", p.posMs)
	}
	p.speed = 4
	p.step(clockAt(1500))
	if p.posMs != 1000+500*4 {
		t.Errorf("after 0.5 s at 4x, pos = %d ms, want 3000", p.posMs)
	}
	p.toggle()
	p.step(clockAt(2500))
	if p.posMs != 3000 {
		t.Errorf("paused: pos moved to %d ms", p.posMs)
	}
}

func TestFirstStepOnlySetsTheReference(t *testing.T) {
	p := newPlayer(1)
	p.playID("t01")
	p.step(clockAt(5000))
	if p.posMs != 0 {
		t.Errorf("the first step moved the position to %d ms", p.posMs)
	}
}

// A long gap (a laptop that slept) is capped, not a jump.
func TestStepCapsALongGap(t *testing.T) {
	p := playing(t, 1, "t01")
	p.step(clockAt(3600 * 1000))
	if p.posMs != maxStepMs {
		t.Errorf("after an hour's gap, pos = %d ms, want %d", p.posMs, maxStepMs)
	}
}

// nearEnd is a player 100 ms before the end of the track at index i, in
// the given mode, ready for a step across the end.
func nearEnd(t *testing.T, i int, repeat string) *player {
	t.Helper()
	p := playing(t, 1, catalog[i].ID)
	p.repeat = repeat
	p.posMs = p.durMs() - 100
	return p
}

func TestTrackEndByRepeatMode(t *testing.T) {
	last := len(catalog) - 1
	cases := []struct {
		name    string
		prep    func() *player
		wantCur int
		playing bool
		posMs   int64
	}{
		{"off moves on", func() *player { return nearEnd(t, 0, repeatOff) }, 1, true, 100},
		{"all moves on", func() *player { return nearEnd(t, 0, repeatAll) }, 1, true, 100},
		{"one repeats the track", func() *player { return nearEnd(t, 0, repeatOne) }, 0, true, 100},
		{"off stops after the last track", func() *player { return nearEnd(t, last, repeatOff) }, last, false, 0},
		{"all wraps to the first", func() *player { return nearEnd(t, last, repeatAll) }, 0, true, 100},
		{"one repeats the last track", func() *player { return nearEnd(t, last, repeatOne) }, last, true, 100},
	}
	for _, c := range cases {
		p := c.prep()
		p.step(clockAt(200))
		if p.cur != c.wantCur || p.playing != c.playing || p.posMs != c.posMs {
			t.Errorf("%s: cur=%d playing=%v pos=%d, want cur=%d playing=%v pos=%d",
				c.name, p.cur, p.playing, p.posMs, c.wantCur, c.playing, c.posMs)
		}
	}
}

func TestTrackEndTakesTheQueueFirst(t *testing.T) {
	p := nearEnd(t, 0, repeatOff)
	p.enqueue("t10")
	p.enqueue("t11")
	p.step(clockAt(200))
	if catalog[p.cur].ID != "t10" || len(p.queue) != 1 || p.queue[0].ID != "t11" {
		t.Errorf("cur=%s queue=%v, want t10 playing and t11 left", catalog[p.cur].ID, p.queue)
	}
	// Repeat one beats the queue at a track end.
	q := nearEnd(t, 0, repeatOne)
	q.enqueue("t10")
	q.step(clockAt(200))
	if q.cur != 0 || len(q.queue) != 1 {
		t.Errorf("repeat one: cur=%d queue=%v, want the same track and an untouched queue", q.cur, q.queue)
	}
}

func TestManualNextIgnoresRepeatOneAndWraps(t *testing.T) {
	p := playing(t, 1, "t05")
	p.repeat = repeatOne
	p.next()
	if p.cur != 5 {
		t.Errorf("n with repeat one went to %d, want 5", p.cur)
	}
	p.playID("t25")
	p.repeat = repeatOff
	p.next()
	if p.cur != 0 {
		t.Errorf("n on the last track went to %d, want 0", p.cur)
	}
}

func TestShuffleIsSeededAndNeverRepeatsTheTrack(t *testing.T) {
	run := func(seed int64) []int {
		p := playing(t, seed, "t01")
		p.shuffle = true
		var seq []int
		for i := 0; i < 40; i++ {
			before := p.cur
			p.next()
			if p.cur == before {
				t.Fatalf("shuffle picked the track that was playing (%d)", before)
			}
			seq = append(seq, p.cur)
		}
		return seq
	}
	a, b, c := run(3), run(3), run(4)
	if !reflect.DeepEqual(a, b) {
		t.Error("the same seed gave two different shuffles")
	}
	if reflect.DeepEqual(a, c) {
		t.Error("two seeds gave the same shuffle")
	}
}

func TestPrevious(t *testing.T) {
	p := playing(t, 1, "t03")
	p.posMs = 10_000
	p.prev()
	if p.cur != 2 || p.posMs != 0 {
		t.Errorf("p after 10 s: cur=%d pos=%d, want the same track from 0", p.cur, p.posMs)
	}
	p.playID("t08") // history: t03
	p.posMs = 1000
	p.prev()
	if catalog[p.cur].ID != "t03" {
		t.Errorf("p within 3 s went to %s, want the track played before (t03)", catalog[p.cur].ID)
	}
	// With nothing played before, it goes back in library order, wrapping.
	q := newPlayer(1)
	q.playID("t01")
	q.history = nil
	q.posMs = 0
	q.prev()
	if q.cur != len(catalog)-1 {
		t.Errorf("p on the first track went to %d, want the last", q.cur)
	}
}

func TestSeekStaysInsideTheTrack(t *testing.T) {
	p := playing(t, 1, "t01")
	p.seek(-seekMs)
	if p.posMs != 0 {
		t.Errorf("seek back at the start gave %d", p.posMs)
	}
	p.seek(seekMs)
	if p.posMs != seekMs {
		t.Errorf("seek forward gave %d", p.posMs)
	}
	p.seek(10_000_000)
	if p.posMs != p.durMs()-1000 {
		t.Errorf("seek past the end gave %d, want %d", p.posMs, p.durMs()-1000)
	}
}

func TestVolumeIsClamped(t *testing.T) {
	p := newPlayer(1)
	p.setVolume(1000)
	if p.volume != 100 {
		t.Errorf("volume = %d, want 100", p.volume)
	}
	p.setVolume(-5)
	if p.volume != 0 {
		t.Errorf("volume = %d, want 0", p.volume)
	}
}

func TestQueueKeepsDuplicatesApart(t *testing.T) {
	p := newPlayer(1)
	p.enqueue("t05")
	p.enqueue("t05")
	if len(p.queue) != 2 || p.queue[0].QID == p.queue[1].QID {
		t.Fatalf("queue = %v, want two entries with their own ids", p.queue)
	}
	p.dequeue(p.queue[0].QID)
	if len(p.queue) != 1 || p.queue[0].QID != "q2" {
		t.Errorf("queue = %v after removing q1", p.queue)
	}
	if p.enqueue("nope") {
		t.Error("enqueued a track that does not exist")
	}
}

// A fast clock crosses several tracks in one step and lands inside one.
func TestStepCrossesSeveralTracks(t *testing.T) {
	p := playing(t, 1, "t01")
	p.speed = 1000
	p.step(clockAt(1000)) // 1000 s of music
	if p.cur < 3 || p.posMs >= p.durMs() || !p.playing {
		t.Errorf("cur=%d pos=%d playing=%v, want a later track mid-way", p.cur, p.posMs, p.playing)
	}
}

func TestSpectrumBouncesWhilePlaying(t *testing.T) {
	p := playing(t, 1, "t02")
	seen := map[string]bool{}
	peak := 0
	for i := 1; i <= 60; i++ {
		p.step(clockAt(i * 100))
		for b, v := range p.bands {
			if v < 0 || v > 100 {
				t.Fatalf("band %d = %d, outside 0 to 100", b, v)
			}
		}
		seen[fmt.Sprint(p.bands)] = true
		if m := maxBand(p); m > peak {
			peak = m
		}
	}
	if len(seen) < 50 {
		t.Errorf("only %d distinct frames in 60 ticks: the bars are not moving", len(seen))
	}
	if peak < 60 {
		t.Errorf("peak = %d, want the bars to reach high", peak)
	}
}

func TestSpectrumFallsToRestWhenPaused(t *testing.T) {
	p := playing(t, 1, "t02")
	for i := 1; i <= 40; i++ {
		p.step(clockAt(i * 100))
	}
	if maxBand(p) == 0 {
		t.Fatal("the bars never rose")
	}
	p.toggle()
	prev := maxBand(p)
	for i := 41; i <= 41+30; i++ {
		p.step(clockAt(i * 100))
		if m := maxBand(p); m > prev {
			t.Fatalf("tick %d: the peak rose from %d to %d while paused", i, prev, m)
		} else {
			prev = m
		}
	}
	if maxBand(p) != 0 {
		t.Errorf("after 3 s paused the bars are at %v, want rest", p.bands)
	}
	// The loudness history flushes to zero too.
	for i := 0; i < 80; i++ {
		p.step(clockAt((100 + i) * 100))
	}
	for _, v := range p.loud {
		if v != 0 {
			t.Fatalf("loudness history = %v, want all zero", p.loud)
		}
	}
	// And they rise again on resume.
	p.toggle()
	for i := 0; i < 10; i++ {
		p.step(clockAt((200 + i) * 100))
	}
	if maxBand(p) == 0 {
		t.Error("the bars did not come back after resume")
	}
}

func TestVolumeScalesTheBars(t *testing.T) {
	peak := func(vol int) int {
		p := playing(t, 1, "t02")
		p.setVolume(vol)
		sum := 0
		for i := 1; i <= 60; i++ {
			p.step(clockAt(i * 100))
			for _, v := range p.bands {
				sum += v
			}
		}
		return sum
	}
	if lo, hi := peak(10), peak(100); lo >= hi {
		t.Errorf("energy at volume 10 (%d) is not below volume 100 (%d)", lo, hi)
	}
}

func TestSpectrumIsDeterministic(t *testing.T) {
	run := func(seed int64) [][]int {
		p := playing(t, seed, "t07")
		var frames [][]int
		for i := 1; i <= 50; i++ {
			p.step(clockAt(i * 100))
			frames = append(frames, append([]int(nil), p.bands...))
		}
		return frames
	}
	if !reflect.DeepEqual(run(9), run(9)) {
		t.Error("two runs with one seed differ")
	}
	if reflect.DeepEqual(run(9), run(10)) {
		t.Error("two seeds gave one spectrum")
	}
}

func TestIsinIsASine(t *testing.T) {
	if isin(0) != 0 || isin(256) != 1000 || isin(512) != 0 || isin(768) != -1000 {
		t.Errorf("isin at the quarter points = %d %d %d %d", isin(0), isin(256), isin(512), isin(768))
	}
	for x := int64(-2000); x < 2000; x++ {
		v := isin(x)
		if v < -1000 || v > 1000 {
			t.Fatalf("isin(%d) = %d, out of range", x, v)
		}
		if isin(x+1024) != v {
			t.Fatalf("isin is not periodic at %d", x)
		}
		if isin(x+512) != -v {
			t.Fatalf("isin(%d+512) is not the negation", x)
		}
	}
}
