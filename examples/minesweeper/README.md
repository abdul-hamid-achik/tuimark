# examples/minesweeper — Minesweeper on Tuimark

Minesweeper for the mouse and the keyboard. The whole view is `app.tui`,
`theme.tcss`, and `sizes.tcss`; the Go code holds the rules, publishes plain
data through Tuimark's public package (`Load`, `Bind`, `Set`, `Get`, `Batch`,
`On`, `Every`, `Derive`, `RunWith`, `Dump`, `Play`) and has no layout, width, or
colour code.

Three difficulties — beginner 9 × 9 with 10 mines, intermediate 16 × 16 with 40,
expert 30 × 16 with 99 — and custom boards from the command line. Every cell is
two terminal columns wide, so a cell is roughly square and the expert board is
60 columns plus its frame. A side panel shows the mines left, the clock, the
face, the click mode, and the best times.

## Run it

```
go build -o bin/minesweeper ./examples/minesweeper
./bin/minesweeper                       # beginner
./bin/minesweeper -level expert         # beginner, intermediate, or expert
./bin/minesweeper -cols 20 -rows 12 -mines 40    # a custom board
./bin/minesweeper -seed 7               # another set of mines
./bin/minesweeper -theme light          # Set("@theme", "light") before Run
./bin/minesweeper -dump 90x24           # print Dump() as JSON and exit
./bin/minesweeper -demo 90x24           # the solver's scripted game, one {"ms","dump"} JSON line per frame
```

Flags: `-dir` (default `examples/minesweeper`), `-theme dark|light`, `-seed N`
(default 1), `-level`, `-cols`, `-rows`, `-mines`, `-dump COLSxROWS`,
`-demo COLSxROWS`.

A custom board needs all three of `-cols` (9 to 30), `-rows` (9 to 16), and
`-mines` (1 to cells − 10, so the first cell and its neighbors can always be
left clear), and cannot go with `-level`. A bad value is a usage error (exit
2). `-dump` and `-demo` take `COLSxROWS` exactly (digits, an `x`, digits, 1 to
1000; `90x 24` is an error), a flag given empty (`-theme ""`, `-dump ""`) is a
usage error, not an absent flag, and `-demo` needs at least 36 by 20, the room
for its intermediate board, and says so up front. `-demo` plays its own fixed
game, whatever `-seed`, `-level`, and the custom flags say (they are still
checked).

## Keys and mouse

| Key | Does |
|---|---|
| `?` | help, open or closed (it stops the clock) |
| `q`, `ctrl+c` | quit, from the game, the help, and the "too small" screen alike |
| arrows, `h` `j` `k` `l` | move the cursor |
| `enter`, `space` | reveal the cell under the cursor; on an open number, chord |
| `f` | flag mode on or off: a click (or `enter`) flags instead of revealing |
| `g` | flag the cursor cell directly: flag, question mark, none |
| `n` | new game on the same board |
| `1` `2` `3` | beginner, intermediate, expert (a new game) |
| `p` | pause: hides the board, stops the clock, and freezes the cursor |
| `m` | mouse on or off |

The mouse has one useful button, the left one, so flags need a mode: press `f`
and clicks flag (and cycle the flag, `?`, and the empty cell); press it again to
reveal. A click moves the cursor to the cell it lands on, and keys and clicks
act the same way:

- **Reveal.** The first reveal is never a mine, and it opens a region: the cell
  and its eight neighbors are always clear. A zero opens its neighbors, and
  those open theirs (flood fill), stopping at flags.
- **Chord.** On an open number whose adjacent flags equal the number, a click
  (or `enter`, `space`) opens the rest of its covered neighbors. With the wrong
  flag in the way that opens a mine and loses the game. It works in either mode.
- **Flags.** A flagged cell is covered and is not opened by a click or a flood
  fill; `g` or flag mode takes it to a `?` and then to a covered cell. The
  counter is mines minus flags and goes negative with too many flags.
- **Win** when every safe cell is open: the mines are flagged, the face turns,
  the clock stops, and the time may be the new best for the level.
- **Loss** on opening a mine: every mine is shown (`*`), a flag on a safe cell is
  marked wrong (`x`), and the mine that went off is highlighted.

The clock starts at the first reveal, and stops when the game ends, when
paused, and while the help is open. Best times are kept in memory per
difficulty, for the session; a custom board keeps none.

## The panel and small terminals

The face is drawn with two ASCII characters, one column each (`:)` playing,
`:(` lost, `B)` won, `:|` paused): emoji are two columns wide, and the text
faces such as U+263A are emoji code points that a font may draw that way, which
would shift the line.

With room, the panel sits beside the board. When it does not fit
(the board's width, at least 36, plus 30 columns), its counters move into the
status line above the board: the face, the state, the mines left, the clock, and
the best time of the level, which stay whole down to 44 columns (the best time
goes below that, the rest down to 36). The brand goes with the panel, and the
level label up to 75 columns. What the panel also shows (the other levels' best
times and the click mode word) is not in the status line: only the mode's `flag`
tag is, while flag mode is on. There is no spare row at the smallest height for a
second line.

If the terminal is too small for the current board — narrower than the board in
its frame (and than 36 columns) or shorter than the board plus the status line
and the key hints — the board is replaced by a notice that names the size it
needs, instead of being cut. The edges are exact for every board: for example the
beginner board needs 36 by 13, intermediate 36 by 20, expert 62 by 20. The
board is also the focus, so with it gone the board keys (move, reveal, flag,
new, levels, pause, mouse) do nothing and cannot play a game you cannot see;
`?`, `q`, and `ctrl+c` still work, and the keys play again as soon as the
terminal is big enough. (A running clock keeps running.) Below 36 columns the
notice stands alone; the view is clean down to 10 columns.

The help fills a short terminal and scrolls, so its close button is always
on the screen (from 40 by 14 down to 30 by 9). It takes the focus on its body,
so the arrow keys, `pgup`, `pgdn`, `home`, and `end` scroll it at once; `enter`
or `esc` closes it.

Covered cells come in two shades like a checkerboard (a static `alt` class in
the generated cells), so neighbors can be told apart, each drawn as `▒▒`; a
flag, `?`, number, mine, and wrong flag are all right-aligned in their two
columns (` ►`, ` ?`, ` 3`, ` *`, ` x`), so a mark beside a number is not
cramped. The cursor is a cell of its own color (blue, bold) on covered and open
cells alike, in both themes; it is not drawn once the game is over.

Number colors are the classic ones, adjusted for each theme so they stay
readable on the open cells (contrast 4.5:1 or better): 1 blue, 2 green, 3 red,
4 navy (a light periwinkle on dark), 5 maroon (an orange-brown on dark), 6 teal,
7 black (white on dark), 8 gray.

If a tick or an action fails, the message replaces the level label in the
status line until the next action or tick that works (`OnError` keeps the game
going).

## How the board is built

`on:click` is not allowed inside an `each` template, so a board you can click is
one static element per cell. `app.tui` holds the 16 rows and 480 cells of the
largest grid between two comments, written by `gen_test.go`:

```
UPDATE_TUI=1 go test ./examples/minesweeper -run TestGenerated
```

which also writes `sizes.tcss`, the `@media` rules that need one block per board
size. `TestGenerated` fails when either file differs from what the generator
writes, and the generated `app.tui` is what `tuimark fmt` writes.

- **Cells are named by screen position**, `c0312` for row 3, column 12 of the
  largest grid, not by board coordinate: a new difficulty changes data, never
  the document. A cell outside the current grid has `if` false (and so do its
  rows). A click fires `cell` with `source` = the cell's id, which the host maps
  back to the board.
- **A cell of the grid has every class guard in the store**, because a missing
  path is a diagnostic: `hid` (covered, also true under a flag), `n1`…`n8`,
  `flag`, `q`, `mine`, `boom`, `wrong`, `cur`. An open empty cell has none. A
  position outside the grid is just `{"on": false}`: its `if` is false, so its
  guards are never read, and `sample.json` stays small (30 KB, not 100).
- **Cells are not focusable; the board is.** The cursor is host data drawn with
  `class:cur`; the board `col` is the screen's focus, and the keymap for the arrows,
  `enter`, `space`, and the rest has `when="#board"`. Hiding `#stage` (too
  small) or opening the help (the modal takes the focus) turns those keys off
  with no check in Go; the first keymap (`?`, `q`, `ctrl+c`) has no `when`, so
  it works everywhere. Closing the help or growing the terminal gives the
  focus back to the board.
- **`mouse="settings.mouse"`**, with `m` toggling it, so terminal text selection
  works when it is off.
- **Pausing is a class**: `#board.paused` makes its cells `visibility: hidden`,
  which also stops clicks.
- **The size rules**: the host publishes `fit.nc<N>` and `fit.nh<N>` (the
  columns and rows the board needs) and `#app` carries those classes;
  `sizes.tcss` has one `@media (max-cols)` block per width and one
  `@media (max-rows)` block per height that hides `#stage` and shows
  `#toosmall`, and per width a block that moves the panel into the status
  line (taking the brand out) and one that takes the level label out of it.
  The CSS here has no descendant combinator, so the rules are `>` chains.
- **`App.Derive`** keeps the labels and class flags (`mines_label`,
  `time_label`, `face`, `state_label`, `mode_label`, `level_label`,
  `need_label`, `is.*`,
  `lvl.*`, `best_label.*`, `best_now`, `fit.*`) so the document has no expressions. The
  board itself is published from the host in one `Batch`, only the cells that
  changed.
- **One `App.Every`** (200 ms) keeps the clock. Time comes from `app.now`, which
  the tests and the demo replace with a fake, and the mines come from a
  `rand.Rand` seeded by `-seed`.

## Preview without Go

`mock.json` gives each action literal values, so the view can be tried with no
host program at all (a click or `enter` reveals a region, `g` flags, `f` shows
the flag mode, `p` hides the board, `1` `2` `3` change the board):

```
./bin/tuimark run examples/minesweeper/app.tui \
    --data examples/minesweeper/sample.json --mock examples/minesweeper/mock.json
```

Each answer is what the real handler writes from the start state (a fresh
beginner game), found by playing it, and only for what the document reads:
`run` has no `Derive`, so the answers carry the derived labels and flags, and
none of the raw paths only `Derive` reads. They replay the same reveal each
time, a cell is written field by field (so the cursor is one `true` and one
`false`), and the clock does not run: the game loop is the Go host.

- `1` `2` `3` write the whole fresh board of their level, from any board: the
  cells that come onto the grid whole, `on: false` for the positions the level
  lacks, and the rows, size classes, and labels in full. `n` and the levels also
  take a pause off, so `p` never hides the board for the rest of the session.
- `f`, `m`, and `?` are one way here: a literal write cannot toggle, so `f`
  turns flag mode on, `m` turns the mouse off, and `?` opens the help (`esc`
  closes it).
- Apart from the level keys, an answer is a difference from the start state, so
  replayed after other answers it is only as exact as that difference.
`sample.json` and `mock.json` are written by the tests
(`UPDATE_SAMPLE=1 go test ./examples/minesweeper -run TestSampleInSync`), and
`TestSampleInSync` fails when they drift from the host.

## The demo

`-demo 90x24` plays an intermediate game on seed 1 with the single-point solver
of `solver.go`: it flags the cells that a number proves to be mines (turning flag
mode on with `f`), chords numbers whose flags are all in place, and otherwise
guesses the covered cell with the lowest chance of a mine, leaving flag mode
before it clicks. It reads only what the player sees, in cell order, with exact
fractions, so the game and the output are the same on every run, OS, and CPU
architecture. It prints under 120 frames over about 11 seconds and ends on the
win, held for 1.4 seconds. Clicks are aimed at cells found in the dump, one clock
second passes per move, and the last frame shows the win face and the time.

## Tests

`go test ./examples/minesweeper` runs:

- the rules on hand-made boards (first reveal, flood fill, chord, flag cycle,
  win, loss) and the solver (it flags only mines and loses only on a guess);
- play tests that press every key and click cells, corners included, at every
  level, with the help open and closed, paused, and with the mouse off;
- the clock (start, pause, help, stall, win, loss, the 99:59 cap) and the best
  times; the paused board ignores a click, `enter`, `space`, `g`, and the
  arrows on a covered cell, and the same key works once resumed;
- layout: no diagnostics in every look (fresh, started, flag mode, paused, lost,
  won, notice, cursor in a corner) on four boards, in both themes, with the help
  open, at seven sizes; every width from 10 to 40 columns on tiny heights; the
  exact edges of "too small" and of the panel for every board; the board keys off
  on the "too small" screen and back on after; the help at every size from 40 by
  14 down, scrolling at once; the faces and glyphs are one column, all
  right-aligned;
- colors read from `Dump` styles: contrast and family of each number in both
  themes, the two covered shades, flag, question, mine, wrong-flag, and exploded
  cells, the cursor's color on covered and open cells (and gone when the game
  ends), and the state colors;
- `sample.json`, `mock.json`, `app.tui`, and `sizes.tcss` in sync with the code,
  every mock action is a document action with a handler, the close actions
  write `help_open` false, the answers write only what the document reads, level
  keys replayed in every order give one coherent board, and a pause does not
  stick;
- the demo: two runs give the same bytes, 15 to 120 frames, 5 to 12 seconds,
  the last frame held 1 to 1.5 seconds, every dump clean, and it ends on the
  win.
