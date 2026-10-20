# examples/2048 — the sliding-tiles game on Tuimark

A playable 2048, built to show nested container `each`, `class:` guards, and
`@media` doing the work a Go layout loop would do elsewhere. The whole view is
`app.tui` + `theme.tcss`; the Go code holds the rules, publishes plain data
through Tuimark's public package (`Load`, `Bind`, `Set`, `Batch`, `On`,
`Every`, `Derive`, `RunWith`, `Dump`, `Play`) and has no layout, width, or
colour code.

The board is `-size` × `-size` tiles (3 to 6, default 4). Tiles are 7 × 3
cells, coloured by value from 2 up to 2048 and a separate colour for anything
bigger. A side panel shows the score with the last gain (`+16`), the best
score, the move count, the largest tile, and a sparkline of the score over the
moves.

## Run it

```
go build -o bin/2048 ./examples/2048
./bin/2048                    # play
./bin/2048 -autopilot         # watch the expectimax autopilot play
./bin/2048 -size 5            # a 5x5 board
./bin/2048 -seed 7            # another run of tiles
./bin/2048 -theme light       # Set("@theme", "light") before Run
./bin/2048 -dump 90x24        # print Dump() as JSON and exit
./bin/2048 -demo 90x24        # the scripted run, one {"ms","dump"} JSON line per frame
```

| Flag | Default | Does |
|---|---|---|
| `-dir` | `examples/2048` | directory with `app.tui` and `theme.tcss` |
| `-theme dark\|light` | the document's | `Set("@theme", …)` before `Run` |
| `-size N` | 4 | board side, 3 to 6 |
| `-seed N` | 2 | seed of the tile spawns; the default opens on a board where every slide moves something |
| `-autopilot` | off | start with the autopilot playing |
| `-speed MS` | 150 | milliseconds between autopilot moves, 50 to 5000; it moves on the 50 ms tick, so the time rounds up to a multiple of 50 (70 plays like 100) |
| `-flash MS` | 200 | milliseconds the highlights of a move last, 0 to 2000; they clear on the 50 ms tick, so this rounds up to a multiple of 50 too |
| `-depth N` | 2 | autopilot search depth, 1 to 3 |
| `-chance4 P` | 0.1 | chance that a new tile is a 4, 0 to 1 |
| `-dump COLSxROWS` | | print `Dump()` as JSON and exit; each side is 1 to 1000 |
| `-demo COLSxROWS` | | print the scripted demo and exit; it plays a fixed game, so it takes only `-theme` and `-dir` |

A value out of range, NaN, or infinite (`-size 7`, `-chance4 NaN`, `-speed 5`),
a size that is not exactly `COLSxROWS` (`80x24junk`, `+80x+24`, `2000x2000`),
and `-demo` with a game flag (`-size`, `-seed`, `-autopilot`, `-speed`,
`-flash`, `-depth`, `-chance4`) are usage errors: the message names the flag, nothing starts, and the exit
status is 2.

## Keys

| Key | Does |
|---|---|
| `up` `down` `left` `right`, `h` `j` `k` `l`, `w` `a` `s` `d` | slide |
| `u` | undo the last move (one level) |
| `r` | new game (the best score is kept) |
| `A`, `t` | toggle the autopilot |
| `?` | help; `?` or `esc` closes it |
| `q`, `ctrl+c` | quit |

`a` is **left** (WASD), so the autopilot is on `A` (shift+a) and `t`, as in
`examples/snake`.

The rules:

- Tiles slide as far as they can; two equal tiles that meet merge, and a tile
  made by a merge does not merge again in the same move (a row `2 2 2 2`
  slides to `4 4`, and `2 2 4` to `4 4`).
- The score is the sum of the tiles the merges made.
- A move that changes nothing spawns nothing and does not count.
- Every move that changes something spawns one tile: a 2, or with chance
  `-chance4` a 4, from a generator seeded by `-seed`.
- `u` takes back the last move only: the board, the score, the move count, and
  the sparkline, and hands the wheel back from the autopilot. The best score stays. Undo does not rewind the generator, so a
  move played again can spawn a different tile. After the game is over, `u`
  takes back the move that ended it.
- Making 2048 opens a modal with *keep going* and *new game*; `esc` is keep
  going. It opens once per game, even if the winning move is undone and made
  again. A move that makes 2048 and also ends the game opens only the game
  over modal, so *undo* stays reachable. Nothing slides while a modal is open,
  and `?` does not open over one.
- With no move left the game ends and a modal offers *undo* and *new game*;
  `esc` closes it to look at the board, and `u` and `r` still work.
- Sliding by hand while the autopilot plays takes the wheel back. The autopilot
  also switches itself off when the game ends.

## The animation

A move marks the tiles it merged (`pop`: the tile is drawn reversed) and the
tile it spawned (`fresh`: a colour of its own) in the store. The store carries
them as `class:` guards on the tiles. `App.Every` ticks every 50 ms, and a tick
clears them once `-flash` milliseconds have passed since the move (200 ms by
default). Each move restarts the timer, so the highlights of the last move are
the ones that show. `-flash 0` clears them on the next tick. Tiles do not
slide between cells.

## Size

The board needs, per side `n`:

| | full tiles (7 × 3) | small tiles (6 × 1) | says "too small" at or below |
|---|---|---|---|
| rows | `4n + 3` | `n + 4` | `n + 3` |
| columns | `9n` (84 for `n = 6`, which has to leave room for the panel) | `7n + 1` | `7n` |

Between those the tiles shrink to one row; below them the board is replaced
by a line saying so. A test sweeps every board size over a wide range of
terminal sizes and checks the tiles are full, small, or absent and the dump is
clean at every one. The side panel shows from 80 columns and 15 rows; without
it, the status line carries `score N best N`, and it gives up the `◆ 2048`
brand, then the mode as columns run out; the score is the flexible part, so
a six- or seven-digit score is shortened with an ellipsis before the state
label (`● PLAYING`, `✖ GAME OVER`) is touched, and below 39 columns it goes.
The status line, the "too small" line and the board are clean down to 15
columns.

The help, win and game over modals are clean (no diagnostics, buttons on
screen) at 40x14, 60x20, 80x24, 100x30, and 120x30 in both themes.

## The autopilot

Expectimax over the real rules: it tries each slide, averages over every place
a 2 (weight 9) or a 4 (weight 1) could spawn, and looks `-depth` plies ahead
(one more once the board has three empty cells or fewer). Boards are scored by
empty cells, merges available, rows and columns that rise one way, and small
tiles. It is integer arithmetic only and never reads the clock, so a board
always gets the same answer; ties go in the order left, down, right, up. A test
checks it reaches a 256 tile within 600 moves on the first three seeds and
scores at least twice what a fixed left-down-right-up cycle does.

It moves on the first tick after `A`, then every `-speed` milliseconds. It
waits while a modal is open.

## Preview without Go

`mock.json` gives each action literal values, so the view can be tried with no
host program at all (the arrows move the board to the state after one slide,
`u` puts the opening board back, `r` deals a new one, `A` shows the autopilot, `?` the help).
A mock action is one literal list of writes, so the toggles only go one way:
`A` always turns the autopilot on and `?` always opens the help (`esc` closes
it). Only paths `app.tui` reads are written, which a test checks:

```
./bin/tuimark run examples/2048/app.tui \
    --data examples/2048/sample.json --mock examples/2048/mock.json
```

Nothing ticks there, so the highlights of a move stay until the next action,
and no mock action opens the win or game over modal (only the Go host knows
when a move ends the game). Use the Go build for the real thing.

`sample.json` and `mock.json` are generated from the code. `TestSampleInSync`
fails when they differ from what the app builds, and
`UPDATE_SAMPLE=1 go test ./examples/2048 -run TestSampleInSync` rewrites them.

## The demo

`./bin/2048 -demo 90x24` prints 88 frames, 10 800 ms in all, the last held for
1 300 ms. A fresh game with the default seed, the autopilot switched on with
the real `A` key: the first 26 moves get a frame each at 150 ms so the
highlights read, then each frame is four moves later and shows for 85 ms (at
150 ms a move, 12 seconds would play only 80 moves), so the score climbs to
about 4 000 and a 512 tile. Then the autopilot is switched off, the highlights
clear, and the last frame holds. The clock is a fake that advances tick by
tick and the only randomness is the seeded spawns, so the bytes are the same
on every run (`TestDemo` runs it twice and compares).

## How it works

- **The board is a nested container `each`.** `board` is a list of rows, each
  `{cells: […]}`; `<col id="board" each="board as line">` repeats a `<row>` per
  line and that row repeats a `<col class="tile">` per cell. A template holds
  no ids, so the cells are found by class. Each tile has one guard per value
  (`class:v2="c.v2"` … `class:v2048`, `class:vbig` for anything above 2048,
  `class:empty`) plus `class:pop` and `class:fresh`.
- **The size is a class.** `class:n4="dims.n4"` on the stage lets the
  stylesheet state each board size's breakpoints: `@media (max-rows: 18)
  { #stage.n4 > #board > .line > .tile { width: 6; height: 1; } }`. TCSS has no
  descendant combinator, so the selectors walk down with `>`.
- **The host publishes data, the runtime derives the rest.** The raw half of
  the store (`tiles`, `pop`, `fresh`, `score`, `gain`, `best`, `moves`,
  `largest`, `hist`, the modal flags, `notice`) is written by the host in one
  `Batch`; `board`, `dims`, `size_label`, `gain_label`, `state` and
  `mode_label` are `Derive`d, so `app.tui` holds no expression.
- **Time is injected.** `tick(b, now)` is the `Every` function; handlers stamp
  a move's highlights with `app.clock()`. Tests and the demo pass a fake.
- **Errors do not stop the game.** `OnError` shows the message in the top line
  in place of the mode label. The next action that works clears it, and so
  does a tick once it is three seconds old (clearing on the very next 50 ms
  tick would not leave time to read it). A quit still quits.
- **Keymap order.** `?` and `q` are the first rows, so `<hints>` keeps them
  when it runs out of room.

## Tuimark features used

No `version` (the current language), container `each` (nested), `class:NAME` guards, `@media`
on columns, rows and theme, `row-gap`/`column-gap`, `<sparkline>`, `<hints>`
(`scope="all"` in the help), `<modal>` with `<button>`s, keymap `label`/`keycap`,
`App.Every`, `App.Derive`, `App.Batch`, `App.Play`, `RunWith` with `OnError`.

## Tests

`go test ./examples/2048` covers the rules (merging once, the score, no-op
moves, spawns and their odds, undo, win and game over detection), the search
(it simulates the game's own rules, picks only moves that change the board,
beats a fixed cycle), every key through `Play`, the highlights and their
timing, both modals and their buttons, the layout sweep, no diagnostics at
every size for every board size in many states in both themes, the flags (also
through the real command line), the sample and mock files, and the demo.
