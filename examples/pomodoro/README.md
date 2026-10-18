# examples/pomodoro — a big-digit clock and Pomodoro timer on Tuimark

A Pomodoro timer drawn in big block-character digits, plus a plain clock with a
small world-clock table. Two tabs:

- **Pomodoro**: work 25:00, short break 5:00, long break 15:00 after four work
  phases; cycle dots (`● ◉ ○ ○`), a progress bar that fills in eighths of a cell,
  the task you are on (an `<input>`), and the list of sessions you finished, with
  their start times.
- **Clock**: the time as HH:MM:SS in big digits, the date, and a table of world
  clocks. The cities use fixed standard-time UTC offsets (no daylight saving, no
  time zone database, so the rows are the same on every machine, Windows
  included; the table title says "standard time"). Only the `Local` row follows
  your machine's zone, daylight saving included, so a city that is on summer
  time right now (New York, London, Berlin, Sydney, depending on the date) reads
  an hour behind its wall clock.

The whole view is `app.tui` + `theme.tcss`; the Go code keeps the timer and
publishes data through Tuimark's public package (`Load`, `Bind`, `Set`, `Get`,
`Batch`, `On`, `Every`, `Derive`, `RunWith`, `Dump`, `Play`). It has no layout,
width, color, or truncation code, and the big digits are plain strings.

The phase sets the color: tomato for work, green for a short break, teal for the
long one, muted when paused. In the last ten seconds the color pulses on
alternate half-seconds, the colon blinks every second, and when a phase ends the
digits flash (reverse video) before the next phase starts by itself.

## Run it

```
go build -o bin/pomodoro ./examples/pomodoro
./bin/pomodoro                          # 25m / 5m / 15m, press space to start
./bin/pomodoro -start -work 50m -short 10m -long 30m
./bin/pomodoro -auto=false              # wait for space after each phase
./bin/pomodoro -theme light             # Set("@theme", "light") before Run
./bin/pomodoro -dump 90x24              # print Dump() as JSON and exit
./bin/pomodoro -now 2026-10-17T14:34:07Z -start   # a fixed starting clock (UTC)
./bin/pomodoro -demo 90x24              # the scripted demo, one JSON object per line
```

Flags: `-dir` (default `examples/pomodoro`), `-theme dark|light`, `-dump COLSxROWS`,
`-demo COLSxROWS`, `-now RFC3339` (the clock starts at that instant, in UTC, and
still advances), `-work`, `-short`, `-long` (each 1s to 99m59s), `-auto` (start
the next phase by itself, default true), `-flash D` (the done flash, default
1.5s), `-start` (start the timer at once), and `-tick D` (clock tick, default
100ms).

## Keys

| Key | Does |
|---|---|
| `space` | start or pause |
| `r` | reset the phase to its planned length (stopped) |
| `s` | skip to the next phase (nothing is recorded; a running timer keeps running) |

During the done flash at the end of a phase, `r` and `s` first finish the flash
(the phase that ended is already recorded and the next one begins), then act on
that next phase: `r` resets the break, `s` skips it. The cycle dots never go
backwards. `+` / `-` and `space` do nothing until the flash is over.

| `+` / `-` | add or remove a minute from this phase |
| `1` `2` | switch to the Pomodoro / Clock tab |
| `[` `]` | previous / next tab (`left` / `right` on the tab strip switch once too, but the switch moves focus into the tab, so use `[` `]` or `1` `2` to keep going) |
| `t` | focus the task input (from any tab); `enter` keeps the name, `esc` leaves |
| `?` | all keys |
| `q`, `ctrl+c` | quit |

The task name is stamped on each work phase that ends, and shows in the list of
completed sessions.

## Preview without Go

`mock.json` gives every action literal values, so the view can be tried with no
host program at all:

```
./bin/tuimark run examples/pomodoro/app.tui \
    --data examples/pomodoro/sample.json --mock examples/pomodoro/mock.json
```

Nothing counts down there: the timer, the clock, and the big digits are the
host's `Every` and `Derive`. Each action swaps in what the real one would
visibly do to the sample state (pause, reset, skip, plus and minus a minute, open
and close the help, and the focus move that `enter` in the task input makes).
`?` opens the help and `esc`, `enter`, or `q` closes it. Use the Go build for
the real thing.

## What it shows

- **`App.Every` animation**: one 100ms tick steps the timer with the clock and
  publishes the raw state (phase position, time left, the instant) in a single
  `*Batch`. The colon blink and the last-seconds pulse are functions of the
  published instant, so they animate with no extra state. `Every` never fires in
  `Dump` or `Play`, so the tests and the demo call the same `tick(b, now)` with a
  fake clock.
- **`App.Derive`**: `timer` (the big digits, the label, the title, the cycle
  dots, the progress), `is` (the booleans the `class:` guards read), `clock`,
  `world` (the world-clock rows), and `stats`. Markup never computes anything.
- **Big digits in one `<text>`**: a value with newlines renders as several rows,
  so `{timer.big}` is a five-row string built from a block-character font
  (`font.go`). A blinking colon is drawn blank, not removed, so the digits never
  move. The text is centered by a `justify: center` row.
- **`class:NAME` guards** on one container (`#tone`) color the digits, title, dots
  and progress together by phase, paused, pulse and flash. Selectors are the
  child-combinator kind, so the colors inherit from that container.
- **`<tabs>`** with `switch-to` / `move-next` / `move-prev` keymap rows and a
  `focus=` on the tab; `t` is a plain `focus` action into an inactive tab.
- **`<progress>` with `bar: eighths`**, **`<input>`** with `on:submit`,
  **`<list each key>`** for the sessions, **`<table>`** with `priority` columns
  (the City column keeps a 24-column floor with `min-width`, so when the table
  gets narrow the Offset column drops below 48 columns and the Day column
  below 38, and they come back as it widens), **`<hints/>`**
  from labeled keymap rows, and a `?` modal with `<hints scope="all"/>`.
- **`App.RunWith`** with `OnError`: a failing handler shows its message in the top
  row and the clock goes on; the next tick or action that works clears it.
- **Responsive without code**: `@media (max-cols)` hides the "next" hint and
  tightens padding; `@media (max-rows)` hides the sessions list. `validate` is
  clean from 40x14 up, on both tabs and in both themes (the help modal hides its
  note at 14 rows so the close button stays on screen).
- **Light and dark palettes** from `:root` tokens and `@media (theme: light)`.
- **Provenance**: `sample.json` and `mock.json` are generated from the real timer;
  `go test` fails if they drift
  (`UPDATE_SAMPLE=1 go test ./examples/pomodoro -run TestSampleInSync` rewrites
  them).

## The demo

`-demo COLSxROWS [-theme light]` prints one `{"ms": N, "dump": D}` object per
line, deterministic on every machine (a fake clock, in UTC): the last seven
seconds of a work phase in half-second steps (colon blinking, color pulsing), the
done flash, and the short break starting to count. D is a full `Dump` with
`styles` and `cells`. The docs site turns it into an animated SVG; 90x24 reads
well.

## Files

```
examples/pomodoro/
  app.tui, theme.tcss   the whole view: layout, style, focus, keymap
  sample.json           the whole store at a documented moment, derived paths included
  mock.json             literal action effects for `tuimark run --mock`
  main.go               flags, the app, publish, handlers, RunWith
  timer.go              the Pomodoro state machine (pure; the clock is an argument)
  font.go               the big-digit font
  derive.go             the Derive functions (pure, tested)
  demo.go               the scripted -demo
  *_test.go             timer, font, derive, Play flows, sample drift, demo
```

## Tests

```
go test ./examples/pomodoro/
go test -race ./examples/pomodoro/
```
