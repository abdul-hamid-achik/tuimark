package main

import (
	"math/rand"
	"strconv"
	"time"
)

// Repeat modes, in the order the r key cycles them.
const (
	repeatOff = "off"
	repeatAll = "all"
	repeatOne = "one"
)

const (
	bandCount = 40 // bars in the spectrum
	loudCount = 24 // samples in the loudness history
	loudEvery = 3  // ticks per loudness sample
	maxStepMs = 1000
	seekMs    = 5000
	volStep   = 5
	histCap   = 64
)

// qitem is one entry of the play queue. QID is unique, so the same track
// can be queued twice and each entry still has a key.
type qitem struct {
	QID string
	ID  string
}

// player is the whole playback model: what is playing, the queue, and the
// fake spectrum. Nothing in it reads a real clock or the global rand: the
// time arrives through step, the randomness from the seeded source, and
// all spectrum arithmetic is integer, so a run is the same on every
// machine. Not safe for concurrent use; the runtime calls handlers and
// Every functions one at a time.
type player struct {
	rng *rand.Rand

	cur     int  // index into catalog
	playing bool // false: paused, or stopped when posMs is 0
	shuffle bool
	repeat  string
	volume  int // 0 to 100
	posMs   int64
	speed   int64 // playback clock multiplier; 1 is real time

	queue   []qitem
	nextQID int
	history []int // catalog indexes played before cur, most recent last

	last    time.Time // the time of the previous step
	audioMs int64     // time spent playing: the phase of the spectrum
	ticks   int
	bands   []int // 0 to 100 per band
	loud    []int // 0 to 100, oldest first
}

func newPlayer(seed int64) *player {
	return &player{
		rng:    rand.New(rand.NewSource(seed)),
		repeat: repeatOff,
		volume: 70,
		speed:  1,
		bands:  make([]int, bandCount),
		loud:   make([]int, loudCount),
	}
}

func (p *player) durMs() int64 { return int64(catalog[p.cur].Secs) * 1000 }

// jump starts the track at index i from its beginning.
func (p *player) jump(i int) {
	if i != p.cur {
		p.history = append(p.history, p.cur)
		if len(p.history) > histCap {
			p.history = append([]int(nil), p.history[len(p.history)-histCap:]...)
		}
	}
	p.cur, p.posMs, p.playing = i, 0, true
}

// playID starts the track with this id; false when there is none.
func (p *player) playID(id string) bool {
	i := trackIndex(id)
	if i < 0 {
		return false
	}
	p.jump(i)
	return true
}

// toggle plays or pauses; a stopped player starts its track again.
func (p *player) toggle() { p.playing = !p.playing }

// advance moves to the next track. A track that ran out (manual false)
// honors repeat one and stops after the last track unless repeat is all;
// the n key (manual true) always moves on, wrapping around the library.
// The queue goes first, then shuffle's random pick, then library order.
func (p *player) advance(manual bool) {
	switch {
	case !manual && p.repeat == repeatOne:
		p.posMs = 0
	case len(p.queue) > 0:
		id := p.queue[0].ID
		p.queue = p.queue[1:]
		p.jump(trackIndex(id))
	case p.shuffle && len(catalog) > 1:
		r := p.rng.Intn(len(catalog) - 1)
		if r >= p.cur {
			r++
		}
		p.jump(r)
	default:
		n := p.cur + 1
		if n >= len(catalog) {
			if !manual && p.repeat != repeatAll {
				p.playing, p.posMs = false, 0 // the end of the library
				return
			}
			n = 0
		}
		p.jump(n)
	}
}

// next is the n key.
func (p *player) next() { p.advance(true) }

// prev is the p key: more than three seconds in, it restarts the track;
// otherwise it goes back to the track played before, or to the one before
// in library order when nothing was played yet.
func (p *player) prev() {
	if p.posMs > 3000 {
		p.posMs, p.playing = 0, true
		return
	}
	if n := len(p.history); n > 0 {
		i := p.history[n-1]
		p.history = p.history[:n-1]
		p.cur, p.posMs, p.playing = i, 0, true
		return
	}
	i := p.cur - 1
	if i < 0 {
		i = len(catalog) - 1
	}
	p.cur, p.posMs, p.playing = i, 0, true
}

// seek moves the position by delta, staying inside the track.
func (p *player) seek(delta int64) {
	p.posMs += delta
	if p.posMs < 0 {
		p.posMs = 0
	}
	if max := p.durMs() - 1000; p.posMs > max {
		p.posMs = max
	}
}

func (p *player) setVolume(v int) {
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	p.volume = v
}

func (p *player) cycleRepeat() {
	switch p.repeat {
	case repeatOff:
		p.repeat = repeatAll
	case repeatAll:
		p.repeat = repeatOne
	default:
		p.repeat = repeatOff
	}
}

// enqueue appends a track to the queue.
func (p *player) enqueue(id string) bool {
	if trackIndex(id) < 0 {
		return false
	}
	p.nextQID++
	p.queue = append(p.queue, qitem{QID: qidName(p.nextQID), ID: id})
	return true
}

func qidName(n int) string { return "q" + strconv.Itoa(n) }

// dequeue removes the entry with this QID.
func (p *player) dequeue(qid string) bool {
	for i, q := range p.queue {
		if q.QID == qid {
			p.queue = append(append([]qitem(nil), p.queue[:i]...), p.queue[i+1:]...)
			return true
		}
	}
	return false
}

// step is the clock: it advances the playback position by the time since
// the previous step (times the speed), moves on when a track ends, and
// animates the spectrum one frame. It is the only place time enters; the
// Every tick passes time.Now(), a test or the demo a fake clock. The first
// call only sets the reference time.
func (p *player) step(now time.Time) {
	var dt int64
	if !p.last.IsZero() {
		dt = now.Sub(p.last).Milliseconds()
		if dt < 0 {
			dt = 0
		}
		if dt > maxStepMs {
			dt = maxStepMs
		}
	}
	p.last = now
	if p.playing && dt > 0 {
		p.audioMs += dt
		p.posMs += dt * p.speed
		for guard := 0; p.playing && p.posMs >= p.durMs() && guard < 64; guard++ {
			carry := p.posMs - p.durMs()
			p.advance(false)
			if !p.playing {
				break
			}
			p.posMs += carry
		}
	}
	p.animate(dt)
}

// animate moves every band toward its target level for this instant. The
// target is a sum of integer sines per band (a slow wave across the
// bands, a faster one, and the track's beat on the low end) plus seeded
// noise. A band jumps up most of the way at once and falls by a fixed
// fraction per tick, so it bounces while playing and settles to zero
// when the target drops to nothing.
func (p *player) animate(dt int64) {
	n := len(p.bands)
	bpm := int64(catalog[p.cur].BPM)
	beat := isin(p.audioMs*bpm*1024/60000+256) + 1000 // cosine, 0 to 2000
	beat = beat / 2
	beat = beat * beat / 1000 * beat / 1000 // sharpen the pulse
	vol := int64(350 + 650*p.volume/100)
	falls := int((dt + 45) / 90)
	if falls < 1 {
		falls = 1
	}
	if falls > 6 {
		falls = 6
	}
	sum := 0
	for b := 0; b < n; b++ {
		noise := int64(p.rng.Intn(1000)) // drawn even when paused: one stream
		var target int64
		if p.playing {
			bb := int64(b)
			f := bb * 1000 / int64(n-1)
			tilt := 1000 - 600*f/1000
			w1 := (isin(p.audioMs*(600+37*bb)*1024/1000000+bb*97) + 1000) / 2
			w2 := (isin(p.audioMs*(1700+61*bb)*1024/1000000+bb*bb*3) + 1000) / 2
			pulse := beat * (1000 - f/2) / 1000
			mix := (300*pulse + 400*w1 + 200*w2 + 100*noise) / 1000
			mix = mix * mix / 1000 // contrast: a few tall bars among low ones
			target = mix * tilt / 1000 * vol * 300 / 1000 / 1000
			if target > 100 {
				target = 100
			}
		}
		v := int64(p.bands[b])
		if target >= v {
			v += ((target-v)*7 + 9) / 10
		} else {
			for i := 0; i < falls && v > target; i++ {
				v = v * 70 / 100
			}
			if v < target {
				v = target
			}
		}
		p.bands[b] = int(v)
		sum += int(v)
	}
	p.ticks++
	if p.ticks%loudEvery == 0 {
		p.loud = append(p.loud[1:], sum/n)
	}
}

// isin is a sine with a period of 1024 and a range of -1000 to 1000, from
// Bhaskara's rational approximation in integers: the same bits everywhere.
func isin(phase int64) int64 {
	phase %= 1024
	if phase < 0 {
		phase += 1024
	}
	sign := int64(1)
	if phase >= 512 {
		sign, phase = -1, phase-512
	}
	m := phase * (512 - phase)
	return sign * 4000 * m / (327680 - m)
}
