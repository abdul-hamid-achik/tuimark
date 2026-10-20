# examples/logs — a live log viewer on Tuimark

A small `tail -f` / lnav: an API service's logs stream into a scroll pane that
follows the end until you scroll up, with a filter box, level toggles, per-level
counts, and a lines-per-second sparkline. The whole view is `app.tui` +
`theme.tcss`; the Go code only keeps a ring buffer and publishes data through
Tuimark's public package (`Load`, `Bind`, `Set`, `Get`, `Batch`, `On`, `Every`,
`Derive`, `RunWith`, `Dump`, `Play`). It has no layout, width, color, or
truncation code.

By default the lines come from a seeded generator (requests through a gateway,
database queries, cache hits and misses, retries, warnings, and errors with a
stack), so the view is the same on every machine. `-follow FILE` tails a real
file instead.

## Run it

```
go build -o bin/logs ./examples/logs
./bin/logs                      # the deterministic demo stream
./bin/logs -seed 42             # another deterministic stream
./bin/logs -follow app.log      # tail a real file
./bin/logs -theme light         # Set("@theme", "light") before Run
./bin/logs -dump 120x30         # print Dump() as JSON and exit
```

Flags: `-dir` (default `examples/logs`), `-dump COLSxROWS`, `-theme dark|light`,
`-seed N`, `-follow FILE`, `-buffer N` (lines kept, default 2000), and
`-interval D` (time between ticks, default 250ms).

## Keys

| Key | Does |
|---|---|
| `/` | focus the filter; `enter` keeps it and returns to the log, `esc` clears it |
| `1` `2` `3` `4` | toggle error, warn, info, debug |
| `p` | pause or resume the stream (a followed file catches up on resume) |
| `c` | clear the buffer |
| `g` / `G` | jump to the top / bottom (`G` follows the stream again) |
| `up` `down` `pgup` `pgdn` `home` `end` | scroll the focused log; scrolling up stops it following |
| `m` | turn the mouse on or off (wheel scrolling) |
| `?` | all keys |
| `q`, `ctrl+c` | quit |

The filter takes words that must all appear in a line's service, level, or
message (case does not matter); `-word` excludes. A stack trace is kept or
dropped with the error it belongs to. Try `billing -retry`.

## Preview without Go

`mock.json` gives every action literal values, so the view can be tried with no
host program at all (the level toggles swap in a pre-filtered list):

```
./bin/tuimark run examples/logs/app.tui \
    --data examples/logs/sample.json --mock examples/logs/mock.json
```

Nothing streams there and the filter box does not filter, since filtering is the
host's `Derive`. Use the Go build for the real thing.

## What it shows

- **`<scroll stick="bottom">`**: follows a growing log, holds still
  once you scroll up, and sticks again after `G`.
- **`App.Every`**: a 250ms tick appends lines in one `*Batch`; a 1s tick feeds the
  sparkline. Neither fires in `Dump` or `Play`, so the tests call them directly.
- **`App.Derive`**: `visible` (filtered by the level toggles and the query),
  `stats` (counts per level, rows shown), `state_label` (`LIVE` / `PAUSED`) and
  `rate_label`. Markup never computes anything.
- **`App.RunWith`** with `OnError`: a tick that fails (the followed file
  vanished) shows its error in the status line, in place of the source name,
  and the stream goes on.
- **`class:NAME` guards** colour lines by level, dim stack frames, and grey out a
  switched-off level chip; **`<sparkline>`**; **`<hints/>`** from labeled keymap
  rows, in groups with `<keymap when>`; a `?` modal with `<hints scope="all"/>`.
- **Responsive without code**: `@media (max-cols: N)` drops the sparkline, the
  timestamp and the service, and tightens the level chips, so `validate` is clean
  at 40, 80, and 120 columns in both themes.
- **Light and dark palettes** from `:root` tokens and `@media (theme: light)`.
- **Provenance**: `sample.json` and `mock.json` are generated from the generator;
  `go test` fails if they drift
  (`UPDATE_SAMPLE=1 go test ./examples/logs -run TestSampleInSync` rewrites them).

## Files

```
examples/logs/
  app.tui, theme.tcss   the whole view: layout, style, focus, keymap
  sample.json           the opening state, for `tuimark dump`/`validate --data`
  mock.json             literal action effects for `tuimark run --mock`
  main.go               flags, the ring buffer, handlers, Every ticks, RunWith
  derive.go             the Derive functions and the filter (pure, tested)
  gen.go                the seeded log generator and its simulated clock
  follow.go             -follow: tail a file, parse levels, strip ANSI
  main_test.go          Validate, Dump, Play flows, tick and stick behaviour
  logic_test.go         filtering, generator determinism, the -follow parser
  sample_test.go        keeps sample.json and mock.json in sync with the generator
```

## Tests

```
go test ./examples/logs/
go test -race ./examples/logs/
```
