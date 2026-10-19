# examples/sudoku — Sudoku on Tuimark

A playable Sudoku with a puzzle generator, pencil marks, hints, undo and redo, a
clock, and a click-or-type interface. The whole view is `app.tui` +
`theme.tcss`; the Go code holds the rules and publishes plain data through
Tuimark's public package (`Load`, `Bind`, `Set`, `Get`, `Batch`, `On`, `Every`,
`Derive`, `RunWith`, `Dump`, `Play`) and has no layout, width, or colour code.

The board is 81 static cells, three columns wide and one row tall, in nine boxes.
A side panel shows the clock, mistakes, hints, how many cells are filled, the
cursor cell's pencil marks as a 3×3 grid, a number pad, and six tools, all
clickable.

## Run it

```
go build -o bin/sudoku ./examples/sudoku
./bin/sudoku                          # a medium puzzle
./bin/sudoku -difficulty hard         # easy, medium, hard, or expert
./bin/sudoku -seed 7                  # another puzzle, the same one every time
./bin/sudoku -puzzle 53..7....6..195....98....6.8...6...34..8.3..17...2...6.6....28....419..5....8..79
./bin/sudoku -theme light             # Set("@theme", "light") before Run
./bin/sudoku -dump 80x24              # print Dump() as JSON and exit
./bin/sudoku -demo 90x24              # the logic solver plays, one {"ms","dump"} JSON line per frame
```

Flags: `-dir` (default `examples/sudoku`), `-theme dark|light`, `-seed N` (default
1), `-difficulty easy|medium|hard|expert` (default medium), `-puzzle STRING`,
`-dump COLSxROWS`, `-demo COLSxROWS`. A bad value is a usage error before the
terminal is touched: an unknown theme or difficulty, a size that is not two plain
numbers from 1 to 1000 (`NaN`, `Inf`, a sign, a fraction, `0`, and `1001x24` are
all refused), a `-seed` that is not an integer, `-puzzle` together with
`-difficulty` (a typed puzzle is graded, not generated), and `-demo` with a size
under 40x14 or together with `-seed`, `-difficulty`, `-puzzle`, or `-dump` (the
demo plays its own puzzle). `-demo` checks all of this before it prints a frame.

`-puzzle` takes 81 characters, `1`-`9` for a given and `0` or `.` for a blank,
and refuses a puzzle unless it has at least one blank and **exactly one
solution**: it says "no blanks" (a complete grid has nothing to play), "no
solution", "more than one solution", or that the givens repeat a digit in a row,
column, or box. The check has a budget of 200000 search nodes, not a time: a grid
that spends it (Norvig's unsolvable puzzle is one, or nearly) is refused with a
message saying the search gave up. The puzzle is then graded by the logic solver (below), and the
panel title shows that grade.

## Keys

| Key | Does |
|---|---|
| `?` | help |
| `q`, `ctrl+c` | quit |
| `up` `down` `left` `right`, `k` `j` `h` `l` | move the cursor (it stops at the edges) |
| `1`-`9` | put a digit in the cursor cell; in notes mode, toggle that pencil mark |
| `0`, `backspace`, `x` | erase the cell's digit and pencil marks |
| `n` | notes mode on or off |
| `u`, `U` | undo, redo |
| `H` | hint: fill one correct cell (counted) |
| `c` | check: mark the wrong entries |
| `p` | pause or resume; paused, the board is hidden |
| `N` | new game (choose a difficulty) |
| `m` | mouse on or off (on by default) |

Clicks: a cell moves the cursor there, a pad number puts that digit, and
`erase`, `notes`, `undo`, `redo`, `hint`, `check` do what their keys do. In the
help, `enter`, `q`, `?`, or `esc` closes it. `?`, `q`, and `ctrl+c` are in a
keymap with no `when`, so they work wherever the focus is, and on a screen too
small for the board (where nothing can take the focus); `q` and `ctrl+c` quit
from under the new-game and win modals too.

**`h` moves left, so the hint is `H`.** Both hjkl and `h` for a hint are natural,
but only the first matching keymap row fires, so one has to give way. The vim
keys win and the hint is shift+h. A test pins both.

**Digits are `put1` … `put9`.** A key event does not say which key fired it, so
each digit is its own action; the number pad uses the same names, and the event's
`source` tells a click from a key.

The keymap starts with `?` and `q`, so the hints bar keeps them when it runs out
of room (it does at 40 columns).

## Rules in play

- **Givens** are bold and fixed. Entries are blue. The cursor cell is the accent
  colour; its row, column, and box are tinted; every cell holding the same digit
  is tinted more; the cursor's box has an accent border.
- **Conflicts** (a digit repeated in a row, column, or box) turn both digits red.
  That is only a duplicate, not a verdict: a wrong digit that clashes with
  nothing stays calm until you press `c`.
- **Mistakes** counts entries that differ from the solution, once each, at the
  time they are placed. Undoing one does not take it back.
- **Notes.** In notes mode a digit toggles a pencil mark on an empty cell. A cell
  that holds notes shows a dim `·`; the cursor cell's marks are the 3×3 grid in
  the panel. Placing a digit removes it from the notes of its row, column, and
  box, and a single undo takes back the digit **and** the notes it removed.
  (Cells are one row tall, so marks cannot be drawn inside them; there is no
  in-cell notes view.)
- **Undo and redo** cover digits, erasing, notes, and hints. A new move empties
  redo. After the win they do nothing.
- **Hint** fills the cursor cell when it is not right yet. Otherwise it goes to a
  cell the logic solver can fill with a naked or hidden single from the entries
  that are right (wrong entries are ignored), moves the cursor there, and says
  which technique in the top line: `hint: r4c5 = 7, hidden single`. If no single
  is left it fills the first cell that is not right. A hint is counted, and can be
  undone; the count stays.
- **Check** marks every entry that differs from the solution with a red
  background and says how many. A mark goes away when its cell changes.
- **Pause** hides the digits (the middle box says PAUSED) and stops the clock.
  Digits, erase, notes, undo, redo, hint, check, the arrows, clicks on cells, the
  pad, and the tools all do nothing. `?` (help), `N` (new game, which also
  unpauses), `m` (mouse), `q`, and `ctrl+c` still work. The help and the
  new-game modal stop the clock too. A finished board cannot be paused, and takes
  no digits, undo, redo, hint, check, erase, or notes: only the cursor keys and
  `N`.
- **Win.** When the board equals the solution the clock stops, every cell is
  highlighted for 0.7 s, and the win modal opens with the time, hints used, and
  mistakes. `new game` goes to the difficulty choice; `close` leaves the finished
  board on screen.
- **New game** asks for a difficulty and starts a puzzle from the next seed
  (`seed+1`, then `seed+2`, …), so a session is reproducible.

## Generator, solvers, and difficulty

All randomness comes from `rand.New(rand.NewSource(seed))`; the same seed and
difficulty give the same puzzle on every machine.

1. **A complete grid** by randomized backtracking: always branch on the empty
   cell with the fewest candidates (ties broken at random), digits in shuffled
   order.
2. **Removal.** Visit the cells in a seeded random order. A clue is removed when
   the puzzle still has exactly one solution (a backtracking solver with
   constraint propagation counts solutions up to 2, within a budget of 200000
   nodes; a count that runs out of budget keeps the clue) **and** the logic solver still grades it no harder than the
   difficulty asked. Removal stops at the clue floor of the difficulty, or when no
   clue can go.
3. **Grade.** The puzzle is kept when the logic solver grades it exactly at the
   difficulty. Otherwise another removal order is tried (40 per grid, up to 4
   grids); if none gives the grade the closest puzzle is used, and the panel shows
   the grade it really has.

The logic solver (`logic.go`) keeps the candidates of every empty cell and
applies named techniques, the cheapest first. A puzzle's grade is the hardest
technique it cannot do without:

| Difficulty | Techniques | Clue floor |
|---|---|---|
| easy | naked single, hidden single | 38 |
| medium | + locked candidates (pointing, claiming), naked pair | 30 |
| hard | + hidden pair, X-wing | 26 |
| expert | anything beyond the techniques above: this solver gets stuck (some such puzzles fall to triples, swordfish, or XY-wing; many need a guess) | none (usually 23-27) |

The grade is found by running the solver to the end, so it has no clock and no
node budget and is the same on every run. A new game is generated inside its
handler, so a hard one can hold the loop for a moment (a few hundred
milliseconds at worst on the seeds tried).

## Sizes

The board is 33×15 with a border around each box (from 20 rows), or 29×11 with
the boxes told apart by a gap and a tint (below 20 rows). The side panel needs 64
columns and 20 rows; without it the top line carries the clock and the mistake
count. Below 40 columns or 14 rows the view says the terminal is too small (a
shorter sentence below 30 columns, and without the status label below 20), and is
clean down to 10x3. A test lays the
view out at every row count from 14 to 30 at the column counts on and beside each
breakpoint of the stylesheet (40, 55, 56, 63, 64, 80, 120), closed and
with the help, the win, and the new-game modal open, with no diagnostic; another
runs seven states (new, mid-game, a conflict, paused, won, a long error notice,
a clock past an hour) at eight sizes in both themes, with each modal's buttons on
screen.

If a handler or a tick fails, the message replaces the hint line until the next
action or tick that works (`OnError` keeps the game going).

## Preview without Go

`mock.json` answers each action with the literal writes its handler makes, so
the view can be tried with no host program at all:

```
./bin/tuimark run examples/sudoku/app.tui \
    --data examples/sudoku/sample.json --mock examples/sudoku/mock.json
```

Each answer is a fixed list of writes recorded from the sample state, so they
only look right one at a time: any click moves the cursor to r5c5, any digit goes
into the first blank, `H` fills it, and `c` says there is nothing wrong. Three
kinds of answer are recorded differently: the arrows from a cursor on r5c5, the
close actions with their modal open, and `new game` always shows the one puzzle
it was recorded with (a medium game from seed 2). `erase`, `undo`, and `redo` have
nothing to act on in a new game, so they answer with nothing. So do `pause` and
`mouse`: a toggle that can only be turned one way would trap the session on a
PAUSED board or with the mouse off, so the mock leaves them out. `n` switches
notes mode on and nothing switches it off. Only paths the document reads are
written (`run` has no `Derive`). There is no clock and no generator. Use the Go
build for the real thing.

`sample.json` is the whole store of a new game at its documented point (seed 1,
medium, cursor on the first blank, time 0:00), derived paths included, because
the CLI runs no `Derive`. Both files are written by a test from the real code:

```
UPDATE_SAMPLE=1 go test ./examples/sudoku -run TestSampleInSync
```

## The store

The host writes the raw half; `Derive` keeps the rest, so the document has no
expressions.

| Path | Holds |
|---|---|
| `puzzle`, `vals`, `marks` | 81 characters: the givens, the current digits, the check marks |
| `notes` | 81 pencil-mark sets (9-bit masks) |
| `cur`, `pencil` | the cursor cell, notes mode |
| `diff`, `clues`, `time`, `mistakes`, `hints`, `undo_n`, `redo_n` | status |
| `paused`, `won`, `help_open`, `win_open`, `new_open`, `mouse` | flags |
| `info`, `notice` | the last hint or check message, the last error |
| `cells.cRC` | derived: `t` (the text) and `cur peer same given bad wrong note done` |
| `boxes.bN`, `sel`, `mini`, `pad`, `filled`, `state`, `diff_label`, `mode_label` | derived |

## How it uses Tuimark

- **81 static clickable cells.** `on:click` is not allowed inside an `each`
  template, so each cell is its own element, `<text id="c34" class="cell"
  class:cur="cells.c34.cur" … on:click="cell">`. A click fires `cell` with
  `source` = `c34`. Ids are by screen position (`c` + row + column), so the data
  decides what a cell shows and nothing in the markup depends on the puzzle.
- **The markup is generated.** The board, the pad, and the notes grid sit between
  `<!-- generated: NAME -->` markers in `app.tui`; `gen_test.go` writes them
  (`UPDATE_TUI=1 go test ./examples/sudoku -run TestAppTuiInSync`) and a drift
  test fails when the file differs from the generator. The output is already what
  `tuimark fmt` writes.
- **Cells take no focus.** A focused node with `on:click` would swallow `enter`
  and `space`, and `tab` would visit 81 stops. The cursor lives in the host's
  data and is drawn by a `class:cur` guard. The board `col` is `focusable` and is
  the screen's `focus`, so `<keymap when="#board">` is in force on the board and
  out of force under a modal; after a modal closes the focus is back on the
  board.
- **`class:` guards** colour cells, the pad, the tools, and the box borders from
  derived booleans, with the CSS order doing the precedence (cursor over same
  digit over peer; conflict red over entry blue).
- **`<tui mouse="mouse">`** with `m` toggling it, so terminal text selection works
  when it is off.
- **`@media`** by columns and rows, in the stylesheet only: the box borders, the
  side panel, the brand, and the too-small notice.
- **`App.Every`** is the clock. Its function is `step(setter, now)`, a pure
  function of the app and the time it is handed; tests and the demo drive it with
  a fake clock. A stall longer than a second counts as one second.
- **`App.Derive`** keeps the cells, the box marks, the notes grid, the pad, the
  counts, and the labels from the raw paths.
- **Three modals**, each with `focus=` on its default button, `on:escape`, and a
  close action that writes its flag false.
- **`<hints/>`** shows the keymap's labelled rows.

## The demo

`go run ./examples/sudoku -demo 90x24` prints about 65 frames in about 10 s (a
test bounds the count to 15-120 frames and the total to 5-12 s, with the last
frame held 1-1.5 s). A generated
easy puzzle with 52 clues is solved by the logic solver through the app's own
inputs: it pencils in the candidates of one cell with `n`, then for each digit
clicks the cell and types the digit. The finished board shows highlighted for 0.7
s, then the win modal is held for 1.3 s. The clock is a fake that advances by what
each frame shows, so the output is the same bytes on every run, and `-theme light`
works too.

## Tests

`go test ./examples/sudoku` covers the geometry, the solvers (every technique on a
hand-built position), the generator (unique solution, the grade asked, the same
puzzle from the same seed), the rules, every keymap row, clicks on cells, the pad,
and the tools, the clock, the win flow, the modals, the flags, the sample and mock
files, the generated markup, and a sweep of sizes and states for diagnostics.
