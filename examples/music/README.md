# examples/music — a music player on Tuimark

A terminal music player in the spirit of cmus, with no audio. A library of 25
invented tracks, a queue, an album list, a now-playing panel with a position
bar, and a spectrum visualizer that bounces while a track plays and falls to
rest when it is paused. Playback is a clock: a position that advances while
"playing" and moves on at the end of a track by the repeat and shuffle modes.
The whole view is `app.tui` + `theme.tcss`; the Go code keeps a small player
model and implements named actions through Tuimark's public package. There is
no layout, width, padding, color, or truncation code in it.

## Files

```
examples/music/
  app.tui, theme.tcss   the whole view: panel, visualizer, tabs, tables, help, both themes
  sample.json           the whole store at a documented point (derived values included)
  mock.json             canned answers for `tuimark run --mock` (no Go needed)
  main.go               flags, load/run, the Every tick, the OnError policy
  player.go             the player model: clock, queue, repeat/shuffle, spectrum (pure)
  catalog.go            the 25 invented tracks
  derive.go             the Derive functions: rows, counts, now-playing fields
  handlers.go           every named action
  demo.go               the scripted -demo run behind the docs animation
  *_test.go             Play flows, player logic, the sample drift test, the demo
```

## Run it

```
go build -o bin/music ./examples/music
./bin/music                          # starts playing the sample's track
./bin/music -theme light             # Set("@theme", "light") before Run
./bin/music -seed 9                  # another spectrum and another shuffle
./bin/music -speed 8                 # the playback clock at 8x, to see tracks end
./bin/music -paused                  # start paused
./bin/music -dump 100x24             # print Dump() as JSON and exit
./bin/music -demo 100x24             # the scripted demo: one {"ms","dump"} JSON object per line
```

Flags: `-dir` (default `examples/music`, where `app.tui` lives), `-theme
dark|light`, `-seed`, `-speed`, `-paused`, `-dump COLSxROWS`, `-demo
COLSxROWS`. The opening state is fixed (see "The sample" below), so the first
screen is the same on every machine.

## Keys

| Key | Does |
|---|---|
| `space` | play / pause (starts a stopped player) |
| `n` / `p` | next track (always moves on, wrapping) / previous: restart if more than 3 s in, else the track played before |
| `enter` | play the highlighted row: the track (library), the queue entry (queue tab), or the album (albums tab: it plays the first track and queues the rest) |
| `a` | add the track under the cursor to the queue (albums tab: the whole album) |
| `x`, `c` | queue tab: remove the entry under the cursor, clear the queue |
| `s` | shuffle on / off |
| `r` | repeat: off, all, one |
| `+` (`=`) / `-` | volume up / down by 5 |
| `←` / `→` | seek 5 s back / forward (the tables do not consume left and right) |
| `/` | focus the filter (every word must match title, artist, or album); `enter` keeps it, `esc` clears it |
| `1` `2` `3` | library, queue, albums tabs |
| `up`, `down`, `home`, `end`, `pgup`, `pgdn` | move the cursor |
| `?` | the help modal (all keys); `esc`, `?`, `enter`, or `q` closes it |
| `q`, `ctrl+c` | quit |

The player keys are quiet while the filter has the focus (typing wins) and
while the help is open.

At the end of a track: repeat one plays it again (and "up next" says so); otherwise the queue comes
first, then a random track when shuffle is on, then the next track in library
order. After the last track the player stops, unless repeat is `all`.

## Try the view without Go

```
./bin/tuimark run examples/music/app.tui --data examples/music/sample.json --mock examples/music/mock.json
```

`run` answers every non-built-in action itself from `mock.json`, which holds
the literal writes each real handler makes from the sample state, so `space`,
`n`, `s`, `r`, `+`, `←`, `enter`, `a` and the rest all change something
visible. That includes the focus: `enter` in the filter moves it to the
library (a write to `@focus`), `esc` clears the filter and does the same, and
the help's close actions write `help_open` false. With no Go host nothing ticks, so the bars and the position stand
still, and the filter does not narrow the library (the rows are derived in Go).
To see the whole thing move without a terminal, use `-demo`.

## What it shows

- **`App.Every` for animation**: one 100 ms tick calls `player.step(now)` and
  writes the position, the spectrum, and the loudness history in a single
  `Batch`, so each frame is one redraw. The step function takes the time as an
  argument; the Every callback passes `time.Now()`, tests and `-demo` pass a
  fake clock.
- **A tall `<sparkline>` as a spectrum analyzer**: 40 bands bound to
  `spectrum`, drawn 6 rows high with `min="0" max="100"` (eight levels per row,
  so 48 levels), and a second one-row `<sparkline>` for a loudness history (the
  mean level, one sample every three ticks, so it scrolls and empties while
  paused). Both have literal `min`/`max`: a `scale` group of all-zero values
  would draw the resting bars half high, and a paused player must draw nothing.
- **A deterministic fake spectrum**: per band, a sum of integer sines (a slow
  wave across the bands, a faster one, and the track's beat on the low end)
  plus noise from a seeded `rand.Rand`. A band rises most of the way to its
  target at once and falls a fixed fraction per tick, so it bounces while
  playing and settles to zero when paused. All arithmetic is integer, so the
  bars are the same on every machine, CPU architecture included.
- **`<progress>` with `bar: eighths`** for the track position and the volume.
- **`<tabs>` with v3 templated labels** (`label="queue {counts.queue}"`) and a
  `short` form for narrow terminals; `switch-to` on `1 2 3`.
- **`<table>`** for the library, queue, and albums, with `<column priority>`
  so a narrow terminal drops the album column first and then the artist, a
  `placeholder` for the empty states, and `class:on` guards that color the
  playing row.
- **`App.Derive`**: `lib_rows` (filtered by the `filter` input, which just
  binds a path), `queue_rows`, `album_rows`, `counts`, `now`, and `vol` are
  recomputed inside every store write, so the markup has no expressions.
- **Several `<keymap when>` groups** over the focus chain (`#lib`, `#q`,
  `#albums`, `#filter`, `#app`, `#help`), with `<hints>` that change with the
  focus. The hints drop from the end when the bar is narrow, so `? help` and
  `q quit` come right after the first player keys, ahead of the less urgent
  ones. A keymap row on a table fires its action with the highlighted row's
  key in the event (`ev.Keys["r"]`), and the handlers act on that row; they do
  not read the bound `cur.*` path, which is only written when the cursor moves.
  The help modal lists the keys in a `<scroll>`, which fills the screen on a
  short terminal (11 rows or fewer) so the close button always stays on it.
- **`<modal>`** help, `@media (max-cols: …)` that drops the visualizer below 77
  columns, `@media (max-rows: 16)` that keeps only the track and its position, and both themes (`theme="auto"`, a dark
  `:root` and a light one under `@media (theme: light)`).
- **`RunWith` with `OnError`**: a failing tick or handler lands in the
  now-playing panel and the music goes on. The message goes away with the next
  action that succeeds, or after 3 s of ticks, and "up next" comes back.

## The sample

`sample.json` is the whole store the app builds at one documented point:
default seed (7), "Tessellate" playing from 1:04, "Window Seat Aurora" and
"Soft Alloy" queued, then 80 ticks of 100 ms of a fake clock from
2026-10-19 21:00:00 UTC (so 1:12 in), derived values included. The Go host
builds the same state itself and does not read the file; `tuimark validate`,
`dump`, and `run` have no Go host and no `Derive`, so they read the literal
values. `TestSampleInSync` keeps it, and `mock.json`, from drifting:

```
UPDATE_SAMPLE=1 go test ./examples/music -run TestSampleInSync
```

## The demo

`-demo COLSxROWS` prints 80 frames, one JSON object per line,
`{"ms":N,"dump":D}` (each dump with `styles` and `cells`): the track plays and
the bars bounce, `a` queues a track (the tab label counts up), `n` skips to the
queued track, `space` pauses so the bars fall to nothing, `space` resumes, and
the last frame holds for 1.3 s. The playback clock runs 6x so the position bar
visibly moves. The output is byte-identical on every run and OS: a fixed seed,
a fake clock, and keys applied through `Play` so the handlers run as in `Run`.
The docs site turns it into an animated SVG; `-theme light` renders the light
palette.

## Tests

```
go test ./examples/music/
go test -race ./examples/music/
```

They cover a clean `Validate()` in both themes, every action having a handler,
`Dump` at five sizes (no wide clusters), `Play` flows (play/pause, next and
previous, the queue, shuffle and repeat cycling, volume, seek, the filter,
tabs, the queue and albums tabs, row actions acting on the highlighted row
after a filter or a removal, the help modal at short heights, the notice
clearing, quit), the tick advancing the
position and ending tracks, the player model alone (track end under each repeat
mode, queue before shuffle, seeded shuffle, seek bounds, the spectrum bouncing
while playing and decaying to rest when paused, determinism), the sample and mock
drift test (mock answers close the modal and move the focus as the handlers do), and the demo (two runs identical, frame and time bounds, every dump
ok with no warnings, the story it tells).
