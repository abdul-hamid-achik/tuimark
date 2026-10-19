# examples/tetris — falling blocks on Tuimark

A playable Tetris with an autopilot, built to show `App.Every` driving a game
whose clock is counted, not scheduled, and a board drawn from data. The whole
view is `app.tui` + `theme.tcss`; the Go code holds the rules, publishes plain
data through Tuimark's public package (`Load`, `Bind`, `Set`, `Get`, `Batch`,
`On`, `Every`, `Derive`, `RunWith`, `Dump`, `Play`) and has no layout, width, or
colour code.

The playfield is 10 × 20 cells with two hidden rows above it where pieces
spawn, two terminal columns per cell so a cell looks square. Beside it: hold,
the next five pieces, score, best, level, lines, pieces placed, combo, the
gravity, the last clear, and a sparkline of the stack height after each of the
last 20 pieces.

## Run it

```
go build -o bin/tetris ./examples/tetris
./bin/tetris                   # play
./bin/tetris -autopilot        # watch the autopilot play
./bin/tetris -seed 7           # another piece sequence
./bin/tetris -level 5          # start at level 5
./bin/tetris -pace 3           # an autopilot three times as fast
./bin/tetris -theme light      # Set("@theme", "light") before Run
./bin/tetris -dump 100x24      # print Dump() as JSON and exit
./bin/tetris -demo 100x24      # the scripted run, one {"ms","dump"} JSON line per frame
```

Flags: `-dir` (default `examples/tetris`), `-theme dark|light`, `-seed N`
(default 56), `-level 1..15` (default 1), `-pace 0.1..10` (the autopilot's
speed as a multiple of the normal one, default 1), `-autopilot`, `-dump
COLSxROWS`, `-demo COLSxROWS`. A bad value is a usage error: a message naming
the flag, the usage, exit status 2, and nothing starts. That covers a value the
flag package cannot parse (`-level abc`, `-pace 1e309`, `-seed 9999999999999999999`)
and one the game cannot run with: a level outside 1 to 15, a pace that is not a
finite number from 0.1 to 10 (NaN and infinities included), an unknown theme, a
size that is not two whole numbers from 1 to 1000 joined by `x`, or `-dump`
with `-demo`. A failure at run time (a missing `-dir`) is `tetris: …` and exit 1.

The board needs 24 columns by 24 rows: the top line, the 22 rows of the field
and its wall, and the hints. Below that the view says the terminal is too small.
The view is clean (no `L008`/`L009`) in both themes with the help or the game
over modal open (and the help over the game over modal), the board paused, rows
flashing, an error in the top line, and numbers as long as an 8-digit score,
5-digit lines and pieces, and the 5-digit level that goes with them. The tests
check every width from 24 to 90 (and every tenth up to 140) at 14 and 24 rows,
the named sizes from 40x14 to 200x50, and both modals at the heights 14 to 20, 24, and 30
and every width from 24 to 60. The modals' buttons are on screen from 40x14 up.
The layout by width:

| columns | what shows |
|---|---|
| 74 and up | stats and trend on the left, the field, hold and next on the right |
| 70 to 73 | the field with hold and next; score, lines, and level move to the top line |
| 48 to 69 | the same, with only the score in the top line |
| 24 to 47 | the field alone; the top line keeps the score (from 32 columns) and the state |

If a tick or an action fails, the message replaces the mode label in the top
line until the next tick or action that works (`OnError` keeps the game going).

## Keys

| Key | Does |
|---|---|
| `left` `right`, `h` `l` | move |
| `down`, `j` | soft drop one row, 1 point |
| `up`, `x`, `k` | rotate clockwise |
| `z` | rotate counter-clockwise |
| `space` | hard drop, 2 points a row, locks at once |
| `c` | hold (once per piece) |
| `p` | pause: the clock stops and the board is hidden |
| `r` | restart (the best score is kept) |
| `a` | toggle the autopilot |
| `?` | help; it pauses the game, `esc` or `?` or the close button closes it and resumes |
| `q`, `ctrl+c` | quit |

Playing by hand takes the wheel from the autopilot.

Under a modal the game keys are not in the keymap at all: the hints show only
`q quit` and `? help`, and a key that would do nothing fires nothing. The game
over modal shows the final numbers and three buttons. The first, **board**, has
focus, so a stray `space` or `enter` from a player still hammering hard drop
only puts the modal away to show the final board (the top line says GAME OVER);
it cannot restart the game and hide the score. `r` restarts, from the modal or
without it; `tab` then `enter` presses restart, `tab tab enter` quit, and `q`
quits. A new game over brings the modal back. The help modal can be opened over
the game over modal; `esc` closes it. Below 40 columns the game over buttons
stand in a column, and the numbers are four short lines that fit down to 24
columns with a 6-digit score.

## The rules

- **Pieces and rotation**: the seven tetrominoes with the Super Rotation System
  (SRS): four rotation states each, five wall-kick tests per turn, and the
  separate kick table of the I piece. The O does not turn.
- **Randomizer**: a seeded 7-bag; every run of seven pieces from a bag's edge
  holds each piece once. The next panel shows five.
- **Spawn**: centred in the hidden rows, one row down at once so it shows. A
  spawn that overlaps the stack ends the game (block out), and so does a piece
  that locks wholly above the visible field (lock out).
- **Ghost**: the shaded cells (`░░`) show where the piece lands.
- **Gravity** by level, in ms per row: 1000, 793, 618, 473, 355, 262, 190, 135,
  94, 64, 43, 28, 18, 11, 7 for levels 1 to 15, and 7 beyond: the guideline's
  curve (0.8 − (L−1) × 0.007)^(L−1) seconds, rounded. The level is the start
  level plus one per 10 lines, and a clear scores at the level before its own
  lines count.
- **Lock delay**: a piece at rest locks after 500 ms. A successful move or turn
  made while the delay runs restarts it, up to 15 times, whether or not the
  piece is still at rest afterwards; the count refills only when the piece
  reaches a row lower than any before. Landing again on the same row with the
  restarts used up locks the piece at once, so lifting it and letting it fall
  cannot be repeated for ever. Hard drop locks at once.
- **Hold**: swaps the falling piece with the held one (the first time, with the
  next). Once per piece: the held piece is dimmed until the next lock.
- **Line clear**: full rows flash (`▓▓`) for 200 ms, then collapse and the next
  piece comes. Nothing can be moved during the flash.
- **Scoring**, all times the level: single 100, double 300, triple 500, tetris
  800; a T-spin (the T was last turned, and 3 of the 4 corners of its box are
  filled, wall, or floor; a corner above the field is open space and does not
  count) 400 with no line, 800, 1200, 1600 for 1 to 3 lines; a clear
  that is a tetris or a T-spin right after another is back-to-back, 1.5 ×;
  each clear after the first in a row adds 50 × the combo × the level. Soft
  drop 1 a row, hard drop 2 a row. The best score lasts until the program quits.

## The autopilot

For the falling piece it tries every distinct rotation in every column, drops
it straight down, clears what is full, and scores the field with the El-Tetris
weights: aggregate height −0.510066, completed lines +0.760666, holes
−0.35663, bumpiness −0.184483. The weights are used as integers (× 10⁶) so the
choice does not depend on how a CPU rounds floating point; rotations and
columns are tried in a fixed order and only a strictly better score replaces
the best, so a seed always plays out the same way. It does not use hold.

It then executes the plan visibly: each act turns the piece one quarter and
shifts it one column toward the target, and once the piece is in place it
drops it 8 rows an act (the soft drop scores, as for a player) and locks it
when it rests. An act comes every 100 ms at `-pace 1`. A blocked turn or shift
makes it drop the piece where it is. It plays 250 pieces from several seeds,
at `-pace 1` and at `-pace 2`, without losing, clearing about 95 lines.

## How the game loop works

- **The loop is one `App.Every` at a fixed 50 ms.** Gravity, the lock delay,
  the flash, and the autopilot's pace all count milliseconds on the game's own
  clock, which `step` advances by the time since the previous tick. So a level
  change takes effect on the very next tick and nothing re-arms the ticker; at
  level 15, where a row takes 8 ms, several rows fall in one tick. (Snake, whose
  speed is the tick, re-arms instead.)
- **`step(b, now)` is the body of the loop**, a function of the app and the
  clock it is given. The `Every` callback passes `time.Now()`; the tests and
  `-demo` pass a fake clock. A stalled tick (a suspended terminal) counts as at
  most one second, and a pause is not play time. All randomness is
  `rand.New(rand.NewSource(seed))`.
- **The store** has a raw half the host writes and a derived half the runtime
  keeps:

  | raw | |
  |---|---|
  | `field` | 22 strings of 10 characters: `.` or a piece letter (`IOTSZJL`) |
  | `cur` | `{kind, cells: [{x,y}×4]}` of the falling piece; empty while rows flash |
  | `ghost`, `flash` | the landing cells; the rows about to clear |
  | `queue`, `hold`, `hold_used` | five letters; a letter or `""`; a boolean |
  | `score`, `best`, `level`, `lines`, `pieces`, `combo`, `gravity_ms` | numbers |
  | `last` | the last clear: `TETRIS`, `B2B T-SPIN DOUBLE`, … |
  | `height_hist` | the stack height after each of the last 20 locks (`null` before) |
  | `paused`, `over`, `auto`, `help_open` | booleans |
  | `over_modal` | the game over modal is up (`over`, unless the player put it away) |
  | `notice` | the last error a tick or an action returned, or `""` |

  | derived (`App.Derive`) | |
  |---|---|
  | `board` | 20 rows of `{runs: […]}` |
  | `hold_rows`, `next_rows` | 2 rows, and 14 rows (five pieces, a blank row between) of runs |
  | `state` | `{label, playing, paused, over}` for the status line |
  | `mode_label`, `speed_label` | `manual` or `autopilot`; `1000 ms/row` |

- **The board is a nested container `each`.** Each of the 20 rows is a list of
  runs: consecutive cells of one kind (empty, one of the seven pieces, ghost,
  flash) are merged into one run. A run is `{t, empty, ki, ko, kt, ks, kz, kj,
  kl, ghost, flash, note, dim}`: `t` is the text (`██` a block, `░░`, `▓▓`, `· `
  an empty cell) and the booleans are the `class:NAME` guards the stylesheet
  colours. Pieces are colour on colour, so they are solid blocks. The hold and
  next panels are the same kind of rows, built four cells wide. A paused board
  is empty but for a `‖ PAUSED ‖` note: the stack is not in the store's board.

```xml
<col id="field" class:dead="over" each="board as line">
  <row class="line" each="line.runs as r">
    <text class="cell" class:empty="r.empty" class:ki="r.ki" … class:ghost="r.ghost"
          class:flash="r.flash" class:note="r.note">{r.t}</text>
  </row>
</col>
```

The game screen's only focusable node is the `#app` column, which takes no
keys, so every key reaches the keymap; its game rows have `when="#app"`, so
they match only while focus is on the board. A modal moves focus into itself,
which is what silences them (and their hints) behind it. The modals' buttons
take `enter` and `space` while focused.

## The demo

`-demo COLSxROWS` prints the frames the docs site turns into an animated SVG,
one `{"ms": N, "dump": D}` line each (`-seed`, `-level`, and `-pace` do not
apply to it): a fresh game with the default seed, then
the `a` key through `App.Play` (so the handler runs exactly as in `Run`), then
one frame per two ticks (100 ms) with the autopilot playing at twice the
normal pace, on a fake UTC clock. The last frame is held 1.2 s so a loop
restarts cleanly. At 100 × 24 that is 93 frames and 10.9 s; the autopilot
places 27 pieces, makes a tetris at frame 42 (the four rows flash for two
frames, then collapse), and ends on 7 lines and 1622 points. The output is the
same bytes on every run, OS, and CPU architecture (the heuristic is integer
arithmetic).

```
go run ./examples/tetris -demo 100x24 [-theme light]
```

## What it shows

- **`App.Every` as a game loop with a counted clock**, the clock injected into
  `step`.
- **`App.Derive`** for the board, the previews, the status, and the labels, so
  the markup has no expression.
- **Nested container `each`** over runs, with distinct aliases and no `id`
  inside the template; **`class:NAME` guards** for every kind of cell.
- **`<sparkline>`** with literal `min`/`max`, **`<hints/>`** from the labeled
  keymap rows (and `scope="all"` in the help modal), **`<modal>`** for game over
  and help with buttons, keymap rows with `keycap`.
- **`App.Play`** to script keys in the tests and the demo, **`App.RunWith`** with
  `OnError`.
- **Responsive without code**: `@media (max-cols: …)` and `(max-rows: …)`.
- **Light and dark palettes** from `:root` tokens and `@media (theme: light)`.
- **Provenance**: `sample.json` and `mock.json` are generated from the code;
  `go test` fails if they drift
  (`UPDATE_SAMPLE=1 go test ./examples/tetris -run TestSampleInSync` rewrites
  them). `sample.json` is the store a fresh app builds with the default seed,
  derived values included, so `tuimark validate`, `dump`, and `play` (which run
  no Go host and no `Derive`) see the real start state. `mock.json` is each
  action's measured effect from the state just before it (restart and
  over_hide from a finished game, so they clear the game over flag; help close
  after help open; hard drop after a hold, so it undims the held piece), kept
  to the paths app.tui reads: a raw path only `Derive` reads would do nothing
  under `run --mock`.

## Preview without Go

```
./bin/tuimark run examples/tetris/app.tui \
    --data examples/tetris/sample.json --mock examples/tetris/mock.json
```

`mock.json` gives each action literal values, so the view can be tried with no
host program. Nothing falls there: the game loop is the Go host's `Every`. The
mock is stateless, so `p` and `a` only switch on (pause shows PAUSED, `auto`
shows the autopilot label) and a second press writes the same thing again;
`hold` dims the held piece, and `space` undims it and moves the board on from
the opening position. Use the Go build for the real thing.

## Files

```
examples/tetris/
  app.tui, theme.tcss   the whole view: layout, style, keymap
  sample.json           the opening store, for `tuimark dump`/`validate --data`
  mock.json             literal action effects for `tuimark run --mock`
  main.go               flags, the app, handlers, Every, step, RunWith
  pieces.go             the tetrominoes, rotation states, SRS kick tables
  game.go               the rules: field, bag, gravity, lock delay, scoring
  auto.go               the autopilot: placement search and the visible moves
  store.go              the raw store, Derive functions, board and preview runs
  demo.go               -demo: the scripted autopilot run
  game_test.go          the rules, with no Tuimark: SRS, kicks, scoring, locks
  auto_test.go          the autopilot: the choice, the moves, survival
  main_test.go          Play flows: every key, pause, help, game over, the loop
  edge_test.go          review findings: the ceiling T-spin, lock delay cap, kick
                        tables, level boundary, hints under modals, the command line
  store_test.go         the store's paths and shapes; space in a modal
  clean_test.go         no diagnostics in any state at any size; modal buttons fit
  demo_test.go          TestDemo: determinism, frames, a multi-line clear
  sample_test.go        keeps sample.json and mock.json in sync with the code
```

## Tests

```
go test ./examples/tetris/
go test -race ./examples/tetris/
```
