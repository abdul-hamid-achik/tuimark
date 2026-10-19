# chess

A full chess game for the terminal: a clickable board, a computer opponent
with six levels, clocks, undo and redo, hints, a review mode, and PGN in and
out. It is an example of Tuimark as a view language: the whole screen is
`app.tui` and `theme.tcss`; the Go host holds the rules, the engine, and the
named actions, and has no layout code.

```
go build -o bin/chess ./examples/chess && ./bin/chess
```

| Command                                        | What it does                                           |
|------------------------------------------------|--------------------------------------------------------|
| `go run ./examples/chess`                      | you (white) against the computer at level 3            |
| `go run ./examples/chess -side black -level 5` | play black against level 5                             |
| `go run ./examples/chess -mode 2p`             | two players at one keyboard                            |
| `go run ./examples/chess -mode cvc -level 2`   | watch the computer play itself                         |
| `go run ./examples/chess -clock 3+2`           | a 3 minute clock with 2 seconds added per move         |
| `go run ./examples/chess -fen "8/8/8/4k3/8/4K3/4P3/8 w - - 0 1"` | start from a position              |
| `go run ./examples/chess -pgn game.pgn`        | step through a saved game                              |
| `go run ./examples/chess -save game.pgn`       | `p` writes the game to that file                       |
| `go run ./examples/chess -letters`             | letters (`K Q R B N P` / `k q r b n p`) for fonts without chess figurines |
| `go run ./examples/chess -theme light`         | `Set("@theme", "light")` before `Run`                  |
| `go run ./examples/chess -dump 100x24`         | print `Dump()` as JSON and exit                        |
| `go run ./examples/chess -demo 100x24`         | the Opera Game, one `{"ms","dump"}` JSON line per frame |

Flags: `-dir`, `-theme dark|light`, `-seed N` (the random side), `-mode
hvc|2p|cvc`, `-side white|black|random`, `-level 1..6`, `-clock
MINUTES+SECONDS|none`, `-fen`, `-pgn`, `-save`, `-letters`, `-dump`, `-demo`.
Numbers are checked: a level outside 1 to 6, a clock that is NaN, infinite, or
outside 0.1 to 180 minutes and 0 to 60 seconds, a size that is not `COLSxROWS`
from 1 to 1000, and `-fen` together with `-pgn` are usage errors.

## Playing

Click a piece, then a square (click the piece again to put it down, another of
your pieces to switch; a click also leaves the move box). Or use the keyboard:
the arrows move the cursor, `enter` or `space` selects and moves. Or press `:` (or `/`) and type a move: `Nf3`,
`exd5`, `O-O`, `e8=Q`, `e2e4`, `e7e8q`; `enter` plays it, and an illegal move
shows a notice and leaves the text to fix. A pawn reaching the last rank asks
what it becomes (`q r b n`, `enter` takes the queen).

The board is 64 static cells with their own ids (`v00` … `v77`, by screen
position, so flipping or resizing only changes data). Light and dark squares,
the cursor, the selected piece, legal targets (a dot on an empty square, red on
a capture), the last move, the king in check, and the hinted move are `class:`
guards on those cells. Cells are 5 columns by 2 rows with room for the side
panel from 80 columns and 20 rows; below that they are 3 by 1, and under 60
columns the side panel gives way to a status (and the clocks, when there are
any) in the top line. The smallest size the layout supports is 35 columns by 12
rows; below 12 rows the board is cut.

Both sides use the filled figurines, told apart by colour: white pieces are
bright and black pieces dark, on mid-tone squares where both keep at least 3:1
contrast in every state (a test measures it, for both themes). A square shows
its states in a fixed order, the first of these winning: the cursor, check, the
selected piece, a capture target, the hint, the last move.

The side panel shows both players with their clocks, what each has captured
and the material difference, the move list as numbered pairs (the move on the
board highlighted, the list following it), the status (whose turn, check, the
result and why it ended), an evaluation bar, and what the computer found: depth,
score, nodes, and the principal variation in SAN.

### Keys

| Key                         | Action                                                        |
|-----------------------------|---------------------------------------------------------------|
| `?`                         | help                                                          |
| `q`, `ctrl+c`               | quit                                                          |
| `up` `down` `left` `right`, `k` `j` `h` `l` | move the cursor (the first press shows it where it moves); in a review, `left`/`right` (`h`/`l`) step through the game and `up`/`down` do nothing |
| `home` `end`                | the cursor to the edge of its rank; in a review, the first or last move |
| `enter`, `space`            | select the piece under the cursor, or move it; in a review, play on from here |
| `:`, `/`                    | type a move (`esc` leaves)                                    |
| `u`, `U`                    | undo, redo (against the computer: your move and its reply)    |
| `n`                         | new game: mode, side, level, clock                            |
| `f`                         | flip the board                                                |
| `H`                         | hint: the computer's best move, highlighted                   |
| `r`                         | resign (asks first)                                           |
| `d`                         | claim a draw (fifty-move rule, threefold repetition)         |
| `v`                         | review the game with the arrows, `home`, and `end`            |
| `p`                         | export the PGN: to the `-save` file, or into a dialog         |
| `m`                         | mouse on or off (off lets the terminal select text)           |
| `L`                         | letters or figurines                                          |
| `tab`                       | between the board and the move box                            |

`hjkl` move the cursor as in vim, so the hint is `H`. In the dialogs: `m s l c`
cycle the new game's mode, side, level, and clock and `enter` starts; `y`/`n`
answer the resignation; `q r b n` choose the promotion; `esc` closes any of them.

## Rules

Full legal move generation: castling (rights, not out of, through, or into
check), en passant, promotion to a queen, rook, bishop, or knight, check,
checkmate, stalemate, insufficient material, the fifty-move rule and threefold
repetition (claimable with `d`), and the seventy-five-move rule and fivefold
repetition (automatic). Repetition counts positions by a Zobrist key with a
fixed seed, where an en passant square counts only if a legal capture exists.

Move generation is checked by perft against the counts of the Chess
Programming Wiki ([Perft Results](https://www.chessprogramming.org/Perft_Results)).
`go test` runs the depths below in under a second; `TUIMARK_PERFT_DEEP=1 go test
./examples/chess -run Perft` runs all of them (about 25 seconds).

| Position            | Depth | Nodes       | Run by default |
|---------------------|-------|-------------|----------------|
| start               | 1–4   | 20 … 197,281 | yes           |
| start               | 5, 6  | 4,865,609; 119,060,324 | deep |
| Kiwipete            | 1–3   | 48 … 97,862  | yes           |
| Kiwipete            | 4, 5  | 4,085,603; 193,690,690 | deep |
| position 3          | 1–5   | 14 … 674,624 | yes           |
| position 3          | 6, 7  | 11,030,083; 178,633,661 | deep |
| position 4 (and its mirror) | 1–4 | 6 … 422,333 | yes     |
| position 4          | 5     | 15,833,292   | deep           |
| position 5          | 1–3   | 44 … 62,379  | yes            |
| position 5          | 4, 5  | 2,103,487; 89,941,194 | deep |
| position 6          | 1–3   | 46 … 89,890  | yes            |
| position 6          | 4, 5  | 3,894,594; 164,075,551 | deep |

SAN is written with the least disambiguation, `x`, `=Q`, `+`, `#`, `O-O` and
`O-O-O`, and read leniently (case, `x`, `=`, marks, zeros for castling,
UCI). Morphy's Opera Game goes through the parser and the writer move by move
in the tests. PGN export writes the Seven Tag Roster, the moves (wrapped at 80
columns), and the result; import skips comments, numeric annotation glyphs, and
variations, and reads a `FEN` tag.

## The computer

Negamax with alpha-beta pruning inside iterative deepening, a quiescence
search, MVV-LVA, killer, and history move ordering, a check extension, and
piece-square tables. Everything is integer arithmetic, and the limits are
counts of plies and nodes, never times, so the same position and level always
give the same move.

| Level | Depth | Node budget |
|-------|-------|-------------|
| 1     | 1     | 50,000      |
| 2     | 2     | 150,000     |
| 3     | 3     | 400,000     |
| 4     | 4     | 250,000     |
| 5     | 6     | 1,000,000   |
| 6     | 8     | 3,000,000   |

The depth is a ceiling: levels 4 to 6 usually end on the node budget one or two
plies short (the new game dialog says "up to depth N"). When the budget runs
out, the iteration in progress is dropped and the one before answers; a budget
that runs out inside the first iteration (a position built to blow up the
search) plays the best-ordered move instead, so the computer always answers. A side short of time (under ten seconds on its clock) thinks at
most three plies. The computer claims a draw when one is available and it is no
better than equal.

The search runs in a goroutine on a copy of the position, tagged with a
generation number; its result is applied with `App.Batch`. An undo, a new game,
a resignation, a flag fall, or a review bumps the generation (and sets a stop
flag), so a result that arrives late is dropped. A reply is never faster than
a third of a second in live play. While it thinks, a spinner animated by
`App.Every` shows in the top line and the status says whose move it is.

## Clocks

`-clock 5+0`, `3+2`, `10+5` (or the new game dialog's choices): minutes each
and seconds added after every move. The minutes are a plain number with at most
one decimal place (`0.5`), the seconds a whole number; anything else is refused.
They tick with `App.Every`; each side's clock starts after its own first move
(so the first move of each side is free and earns no increment), and the time
since the last tick is charged to the side that moves, at the move. A flag fall
loses the game on time unless the other side could not possibly mate (FIDE 6.9,
decided conservatively): it has only a king, or a lone knight or bishop against
a bare king, and the game is drawn. A long stall (a suspended terminal) costs one
second at most. The clocks stop in a review, while a dialog is open, in the
pause the viewer gets between two moves of a game the computer plays, and after
the game ends; the pause a quick level adds to look natural is given back.
An undo (or playing on from a review) puts the time back as it stood.

## How it uses Tuimark

- `version="3"` with `mouse="mouse"` and an `m` key, like `examples/logs`.
- 64 click targets as static elements (`on:click` is not allowed in an `each`
  template), with `class:` guards, generated by `gen_test.go` between the
  `board:begin` and `board:end` comments (`UPDATE_TUI=1 go test ./examples/chess
  -run TestBoardMarkupInSync` rewrites them; the same test fails on a drift).
- `<keymap when="#board">`, `#cmd`, `#help`, `#newgame`, `#resign`, `#promo`,
  `#pgn`: the keys depend on where the focus is, and the board's keys are dead
  behind a dialog. `?` and `q` come first, so `<hints/>` keeps them when it runs
  out of room.
- Five dialogs (`<modal focus=…>`), each with its escape action, closed by
  writing its flag false; the handlers also set `@focus` to the board, which
  the mock needs to say too.
- `App.Derive` for the cells, labels, move rows, player strips, status,
  evaluation bar, and labels, so the document has no expressions.
- `App.Every` for the clocks, the spinner, and the computer's pace when it plays
  itself; `App.Batch` for a search's result; `RunWith` and `OnError` for a notice
  that clears on the next action or after three seconds.
- `<list bind="mcur" focusable="false">` for the move list: the list's offset
  follows the cursor row, so the move on the board stays in view.
- `<progress>` for the evaluation bar, `@media (max-cols: …)` and `(max-rows: …)`
  for the compact board.

## Tests

`go test ./examples/chess` covers the engine (perft, SAN, FEN, PGN, the Opera
Game, results, repetition, search), the interface through `Play` (clicks, keys,
the move box, promotion, undo and redo, review, clocks, hints, resignation,
claims, the new game dialog, PGN in a dialog and a file), every key of every
keymap context, every dialog at sizes from 35x12 up in both themes, the
document and its dumps over many game states, sizes, and themes, the flags, and
the demo. `sample.json` and `mock.json` are generated (`UPDATE_SAMPLE=1 go test
./examples/chess -run TestSampleInSync`): `sample.json` is the whole store of a
fresh game (derived paths included, since the CLI runs no `Derive`), and
`mock.json` answers each action of `tuimark run --mock` with the writes the
real handler makes, found by diffing the store across the action. Mocks are
literal values, so toggles are one-way (`flip`, `m`, `L`, and `v` write the new
state and nothing writes it back), a move is the one recorded from the sample
state (with the computer's reply, as the real handler gives it), and the
promotion mocks start from a position with a pawn about to promote, because the
sample has no promotion to open the dialog from.

```
tuimark validate examples/chess/app.tui --data examples/chess/sample.json
tuimark run examples/chess/app.tui --data examples/chess/sample.json --mock examples/chess/mock.json
```

The demo is the Opera Game (Morphy, Paris 1858, public domain) typed move by
move through the move box, 34 frames of about 280 ms, ending on `17.Rd8#` with
checkmate and 1–0, held for 1.3 seconds. It is identical on every run.
