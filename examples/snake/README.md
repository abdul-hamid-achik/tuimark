# examples/snake — Snake on Tuimark

A playable Snake, built to show `App.Every` as a game loop. The whole view is
`app.tui` + `theme.tcss`; the Go code holds the rules, publishes plain data
through Tuimark's public package (`Load`, `Bind`, `Set`, `Get`, `Batch`, `On`,
`Every`, `Derive`, `RunWith`, `Dump`, `Play`) and has no layout, width, or
colour code.

The board is 24 × 16 cells inside a wall, two terminal columns per cell so a
cell looks square. A side panel shows the score, best score, length, level,
speed, play time, and a sparkline of the snake's pace.

## Run it

```
go build -o bin/snake ./examples/snake
./bin/snake                    # play
./bin/snake -autopilot         # watch the BFS autopilot play
./bin/snake -seed 7            # another food sequence
./bin/snake -theme light       # Set("@theme", "light") before Run
./bin/snake -dump 90x24        # print Dump() as JSON and exit
./bin/snake -demo 90x24        # the scripted run, one {"ms","dump"} JSON line per frame
```

Flags: `-dir` (default `examples/snake`), `-theme dark|light`, `-seed N`
(default 1), `-autopilot`, `-dump COLSxROWS`, `-demo COLSxROWS`.

The board needs 52 columns by 20 rows. Below 80 columns the side panel goes
and the status line carries the score; below 52 columns or 20 rows the view
says the terminal is too small. The view is clean (no `L008`/`L009`) at every
size from 52x20 up with 5-digit scores, with the help or the game over modal
open; the status line drops the brand below 64 columns to keep the score and the
state whole.

If a tick or an action fails, the message replaces the mode label in the top
line until the next tick or action that works (`OnError` keeps the game going).

## Keys

| Key | Does |
|---|---|
| `up` `down` `left` `right`, `h` `j` `k` `l`, `w` `a` `s` `d` | steer |
| `space`, `p` | pause or resume |
| `r` | restart (the best score is kept) |
| `A`, `t` | toggle the autopilot |
| `?` | help; it pauses the game, `esc` or `?` closes it and resumes |
| `q`, `ctrl+c` | quit |

Notes on steering:

- A turn that reverses the snake into its own body, or repeats its heading, is
  ignored.
- Up to two turns are queued between two moves, so a quick double tap (`up`,
  then `left`) makes both turns, one per move.
- Steering by hand while the autopilot plays takes the wheel back.
- `a` is **left** (WASD), so the autopilot is on `A` (shift+a) and `t`.

The autopilot is a breadth-first search to the food that treats a body cell as
free once the tail will have moved past it. It takes that first step unless it
would leave the snake less room than its length (a flood fill), and otherwise
the move with the most free room. Ties go in the fixed order up, right, down,
left, so a seed always plays out the same way.

## Preview without Go

`mock.json` gives each action literal values, so the view can be tried with no
host program at all (the arrows swing the snake round its head, `space` shows
PAUSED, `A` shows the autopilot, `r` resets):

```
./bin/tuimark run examples/snake/app.tui \
    --data examples/snake/sample.json --mock examples/snake/mock.json
```

Nothing moves there: the game loop is the Go host's `Every`. Use the Go build
for the real thing.

## How the game loop works

- **The loop is one `App.Every`.** Its interval is the snake's speed: 140 ms at
  level 1, 7 ms faster for each food, 70 ms at level 11. `Every` has a fixed
  period, so when the level changes the host calls `stop()` and arms a new
  registration at the new interval (`app.rearm`, from inside the tick, which
  `Every` allows; `startTicker` holds the stop-then-arm closure, and a test drives
  it with a fake `Every`). There is no fast base tick.
- **`step(b, now)` is the body of the loop**, a pure function of the app and
  the clock it is given. The `Every` callback passes `time.Now()`; the tests
  and `-demo` pass a fake clock. The clock only measures play time (paused time
  does not count). All randomness is `rand.New(rand.NewSource(seed))`: the
  seed names the sequence of foods, and the autopilot has no randomness.
- **The store** has a raw half the host writes and a derived half the runtime
  keeps:

  | raw | |
  |---|---|
  | `snake` | `[{x,y}, …]`, head first |
  | `food` | `{x,y}` |
  | `score`, `best`, `level`, `interval_ms` | numbers |
  | `elapsed` | play time, `m:ss` |
  | `speed_hist` | the last 22 paces in moves per second (`null` before a move) |
  | `paused`, `over`, `won`, `auto`, `help_open` | booleans |
  | `notice` | the last error a tick or an action returned, or `""` |

  | derived (`App.Derive`) | |
  |---|---|
  | `board` | 16 rows of `{runs: […]}` |
  | `length`, `speed_label`, `level_label` | counts and labels |
  | `state` | `{label, playing, paused, over}` for the status line |
  | `mode_label` | `manual` or `autopilot` |
  | `over_title` | `GAME OVER`, or `BOARD CLEARED` when the snake fills the board |

- **The board is a nested container `each`.** `board` has one entry per row and
  each row is a list of runs: consecutive cells of one kind (empty, body, head,
  food) are merged into one run, so a row is a handful of nodes whatever its
  width. A run is `{t, empty, snake, head, food, dead}`: `t` is two spaces per
  cell (or the head's eyes, or the food's `▐▌`), and the booleans are the
  `class:NAME` guards the stylesheet colours. The cells use `background`, so
  they are solid blocks with no seams. After a game over every snake cell also
  has `dead`, and the wall turns red.

```xml
<col id="field" class:dead="state.over" each="board as line">
  <row class="line" each="line.runs as r">
    <text class="cell" class:empty="r.empty" class:snake="r.snake" class:head="r.head"
          class:food="r.food" class:dead="r.dead">{r.t}</text>
  </row>
</col>
```

Nothing on the game screen takes focus, so the arrow keys reach the keymap
instead of being consumed by a focused list or scroll.

## The demo

`-demo COLSxROWS` prints the frames the docs site turns into an animated SVG,
one `{"ms": N, "dump": D}` line each: a fresh game, then the `A` key through
`App.Play` (so the handler runs exactly as in `Run`), then one frame per move
with the autopilot playing, on a fake clock that advances by the game's own
interval. The last frame is held 1.3 s so a loop restarts cleanly. At 90 × 24
that is 57 frames and 8.3 s, the snake eats six times and the pace sparkline
climbs. The output is the same bytes on every run and every OS.

```
go run ./examples/snake -demo 90x24 [-theme light]
```

## What it shows

- **`App.Every` as a game loop**, re-armed at the speed of the level, with the
  clock injected into `step`.
- **`App.Derive`** for the board, the status, and the labels, so the markup has
  no expression.
- **Nested container `each`** over runs, with distinct aliases and no `id`
  inside the template; **`class:NAME` guards** for every cell kind.
- **`<sparkline>`** with literal `min`/`max`, **`<hints/>`** from the labeled
  keymap rows (and `scope="all"` in the help modal), **`<modal>`** for game over
  and help, keymap rows with `keycap`.
- **`App.Play`** to script keys in the tests and the demo, **`App.RunWith`** with
  `OnError`.
- **Responsive without code**: `@media (max-cols: 79)` and `(max-rows: 19)`.
- **Light and dark palettes** from `:root` tokens and `@media (theme: light)`.
- **Provenance**: `sample.json` and `mock.json` are generated from the code;
  `go test` fails if they drift
  (`UPDATE_SAMPLE=1 go test ./examples/snake -run TestSampleInSync` rewrites
  them). `sample.json` is the store a fresh app builds with the default seed,
  derived values included, so `tuimark validate`, `dump`, and `play` (which run
  no Go host and no `Derive`) see the real start state.

## Files

```
examples/snake/
  app.tui, theme.tcss   the whole view: layout, style, keymap
  sample.json           the opening store, for `tuimark dump`/`validate --data`
  mock.json             literal action effects for `tuimark run --mock`
  main.go               flags, the app, handlers, Every, step, RunWith
  game.go               the rules: grid, snake, turn queue, food, speed
  auto.go               the autopilot: BFS to the food, then most room
  store.go              the raw store, Derive functions, the board runs
  demo.go               -demo: the scripted autopilot run
  main_test.go          Play flows: steering, pause, eating, game over, help, …
  demo_test.go          TestDemo: determinism, frame count, total time
  sample_test.go        keeps sample.json and mock.json in sync with the code
```

## Tests

```
go test ./examples/snake/
go test -race ./examples/snake/
```
