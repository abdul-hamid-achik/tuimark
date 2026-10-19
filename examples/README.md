# Examples

Each directory is a Tuimark app: a `.tui` document, a `.tcss` stylesheet,
sample data, and, for most, a small Go host. The host supplies data and
implements named actions; it has no layout code. Build any of them from the
repository root:

```sh
go build -o bin/tuimark ./cmd/tuimark
go build -o bin/logs ./examples/logs && ./bin/logs
```

## Apps

| Example | What it is | What it shows |
|---|---|---|
| [`logs`](logs) | A live log viewer, like `tail -f`; `-follow FILE` tails a real file | `<scroll stick="bottom">`, `Every` streaming lines in one `Batch`, `Derive` for the filter and the counts, `<sparkline>`, level colors with `class:` guards |
| [`tasks`](tasks) | A kanban board (Todo, Doing, Done); `-file` keeps it in a JSON file | three focusable lists, `checked` multi-select, chained `Derive`s, a form modal with inputs, a confirm modal, `RunWith`/`OnError` |
| [`gitui`](gitui) | A git client: status, log, branches; `-repo PATH` reads a real repository (read-only without `-write`) | `<tabs>`, tables with `<column priority>`, a colored diff in a `<scroll>`, `<keymap when>` per context, modals |
| [`http`](http) | An HTTP API client, like Postman; a fake API runs in process unless `-base URL` | inputs and a form, a request in a goroutine applied with `Batch`, a spinner on `Every`, cancel with `esc`, response tabs with a scroll and a table |
| [`snake`](snake) | A playable Snake; `-autopilot` watches a small search play it | `Every` as a game loop that re-arms itself faster as the level rises, a board drawn with nested `each` and `class:` guards, a game-over modal |
| [`chess`](chess) | Chess against the computer, a friend, or itself: an alpha-beta engine with six levels, clocks, SAN or UCI move entry, PGN and FEN in and out | 64 static clickable cells with `class:` guards for the selection, legal targets, last move and check; the engine in a goroutine applied with `Batch`; promotion, new-game and resign modals; `<keymap when>` per dialog; perft tests |
| [`tetris`](tetris) | Falling blocks with SRS rotation, hold, a ghost piece and an autopilot | one fixed `Every` tick driving gravity, lock delay and the line-clear flash; the board, hold and next queue as nested `each` runs; a `<sparkline>` of the stack height |
| [`minesweeper`](minesweeper) | Minesweeper on beginner, intermediate, expert or custom boards, by mouse or keyboard | 480 generated static cells with `on:click`, shown per board size with `if`; generated `@media` blocks for the too-small notice; a timer on `Every` |
| [`2048`](2048) | The sliding-tiles game, 3×3 to 6×6, with undo and an expectimax autopilot | tiles as nested `each` with a `class:` guard per value, pop and new-tile highlights cleared by an `Every` tick, win and game-over modals |
| [`sudoku`](sudoku) | Sudoku with a generator graded by solving technique, notes, hints and a clickable number pad | 81 static clickable cells, notes as a 3×3 grid in the side panel, conflict and same-digit highlights from `Derive`, undo and redo, a timer that hides the board when paused |
| [`pipeline`](pipeline) | A CI pipeline run: parallel jobs, a flaky test, retry and rerun | spinners and progress bars driven by one simulation tick, the selected job's log in `<scroll stick="bottom">`, a confirm modal |
| [`music`](music) | A music player with no audio: library, queue, albums, now playing | a six-row `<sparkline>` spectrum visualizer, `bar: eighths` position and volume, `<tabs>` with live counts in their labels, tables with `<column priority>` |
| [`pomodoro`](pomodoro) | A Pomodoro timer and a clock in big block digits | a derived multi-line `<text>` for the digits and their blinking colon, phase colors from `class:` guards, a task `<input>`, a world-clock table |
| [`monitor`](monitor) | A system monitor's studio view | nine tabs, a panel grid with gauges and sparklines, a process table with sorting and multi-select, settings that switch the mouse and the theme |
| [`agent`](agent) | A coding-agent chat on a scripted provider or a local Ollama model | a transcript, a composer, approval modals, a headless mode |
| [`dashboard`](dashboard) | A deploy dashboard | two screens, a progress bar, buttons, a confirm modal |
| [`inbox`](inbox) | A ticket inbox, the SPEC's reference fixture (`version="1"`) | a search input, a bound list, a detail pane, `@media` |
| [`host-ts`](host-ts) | A minimal counter driven by a Bun parent through `@tuimark/host` | `tuimark host` and its fd 3/fd 4 protocol, from TypeScript |

`logs`, `tasks`, `gitui`, `http`, the six games (`snake`, `chess`, `tetris`,
`minesweeper`, `2048`, `sudoku`), `pipeline`, `music`, and `pomodoro` also
ship a `mock.json`, so you can try the view with no Go at all (the mock
answers each action with fixed writes; the Go host is the real app, and
nothing moves on its own without it):

```sh
tuimark run examples/tasks/app.tui --data examples/tasks/sample.json --mock examples/tasks/mock.json
```

The six games, `pipeline`, `music`, and `pomodoro` also have a
`-demo COLSxROWS` flag: the app replays a scripted session on a fake clock
with a fixed seed and prints each frame as a JSON line. The docs site turns
those frames into the animations on its Examples page.

## Fixtures

These feed the tests rather than show an app:

- [`unicode`](unicode): CJK, emoji, and combining marks in lists, wrapped text, and next to borders.
- [`stacked`](stacked): the inbox layout with static text, for the `@media` goldens.
- [`spike`](spike): the first Tuimark document. Its goldens are frozen.

`tuimark validate` reports nothing for any app here with its sample data.
`logs`, `tasks`, `gitui`, `http`, the six games, `pipeline`, `music`,
`pomodoro`, and `monitor` also have their own Go tests
(`go test ./examples/logs/`, …), which replay keys with `Play` and check the
store and the dump; `agent` has Go tests of its own too.
