---
title: Examples
description: Complete Tuimark apps from the repository - six games (Snake, chess, Tetris, Minesweeper, Sudoku, 2048), a CI pipeline runner, a music player, a Pomodoro timer, a container-engine dashboard, a system monitor studio, a live log viewer, a kanban board, a git client, an HTTP client, an inbox, a deploy dashboard, a coding-agent chat, Unicode text and the phase-0 spike - with screenshots rendered from tuimark play.
---

# Examples

Every example lives in the repository's [`examples/`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples) directory as a `.tui`, a `.tcss`, and sample data, most with a small Go host. Each picture is rendered from `tuimark play` on those files (the command is in its title bar), so you can reproduce it exactly. Screens that come in two palettes follow this site's light or dark mode.

`tuimark play` runs no host program, so these pictures show the sample data and what the built-in keys do. The apps' own actions, such as sending a request or moving a card, need the Go host.

The first nine examples move on their own, so their pictures are animations: each Go app has a `-demo` flag that replays a scripted session on a fake clock with a fixed seed and prints every frame, and the site turns those frames into an animated SVG. Use the button in the title bar to pause one; with reduced motion turned on in your system settings, they start paused.

To run them, clone the repository and build:

```sh
git clone https://github.com/abdul-hamid-achik/tuimark
cd tuimark
go build -o bin/tuimark ./cmd/tuimark
go build -o bin/monitor ./examples/monitor
./bin/monitor
```

## snake

[`examples/snake`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/snake) is a playable Snake. The game loop is an `Every` tick whose interval is the snake's speed: each food raises the level, and the tick re-arms itself faster. `-autopilot` (or `A` while playing) hands the wheel to a small search that steers to the food.

<TermShot name="snake-demo">
The autopilot playing from a fresh game. The board is a nested <code>each</code>: one row per board line, one text per run of same-kind cells, colored by <code>class:</code> guards. Nothing on the screen takes focus, so the arrow keys reach the keymap.
</TermShot>

```sh
go build -o bin/snake ./examples/snake
./bin/snake                      # arrows, hjkl or wasd steer; space pauses
./bin/snake -autopilot           # watch it play
```

## chess

[`examples/chess`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/chess) is chess with a real engine behind it. Move generation is checked against the published perft counts, moves go in by mouse, by cursor, or typed as SAN or UCI, and games load and save as FEN and PGN. The engine is a small alpha-beta search with six levels; it thinks in a goroutine and hands its move back through `Batch`, so the board never freezes while it searches. You can play it, give it a friend's seat, let it play itself, or step through a game in review.

<TermShot name="chess-demo">
Morphy's Opera Game (Paris, 1858) typed into the app's own move box. The 64 squares are static elements with ids, so a click names its square; the selection, the legal targets, the last move, and the king in check are <code>class:</code> guards on them.
</TermShot>

```sh
go build -o bin/chess ./examples/chess
./bin/chess                      # you play White against level 3
./bin/chess -mode 2p -clock 5+0  # two players, five minutes each
./bin/chess -pgn game.pgn        # step through a game
```

## tetris

[`examples/tetris`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/tetris) is a falling-blocks game with the modern rules: SRS rotation with wall kicks, a 7-bag, a hold slot, a ghost piece, lock delay, T-spins, and combos. One fixed `Every` tick drives gravity, the lock delay, and the line-clear flash on a game clock, so a level change takes effect on the next tick. `a` hands the game to an autopilot that weighs every placement.

<TermShot name="tetris-demo">
The autopilot playing from a fixed seed, with a four-line clear that flashes before it collapses. The board, the hold slot, and the next queue are nested <code>each</code> runs of same-kind cells, colored by <code>class:</code> guards.
</TermShot>

```sh
go build -o bin/tetris ./examples/tetris
./bin/tetris                     # arrows or hjkl move, space drops, x/z rotate, c holds
./bin/tetris -autopilot          # watch it play
```

## minesweeper

[`examples/minesweeper`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/minesweeper) is Minesweeper on the three classic boards or a custom one. The first reveal always opens a region, a revealed number chords its neighbors, and `f` switches the left click to flagging, since only the left button reaches the document. The 480 cells of the largest board are static elements generated into `app.tui`, so a click names its cell, and `if` shows the ones the current board uses.

<TermShot name="minesweeper-demo">
A solver clearing an intermediate board: it flags the cells it can prove are mines, chords the numbers they complete, and guesses only when it has to.
</TermShot>

```sh
go build -o bin/minesweeper ./examples/minesweeper
./bin/minesweeper                       # beginner; 1, 2, 3 switch boards
./bin/minesweeper -level expert
./bin/minesweeper -cols 20 -rows 12 -mines 30
```

## sudoku

[`examples/sudoku`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/sudoku) generates puzzles with exactly one solution and grades them by the techniques they need, from singles to X-wings. It has notes, hints, a check, undo and redo, and a number pad you can click. The notes of the cell under the cursor show as a 3×3 grid in the side panel, since a one-row cell has no room for them.

<TermShot name="sudoku-demo">
The logic solver filling an easy puzzle through the app's own clicks and keys, then the solved board and the win dialog. The same-digit, conflict, and peer highlights are <code>class:</code> guards computed by <code>Derive</code>.
</TermShot>

```sh
go build -o bin/sudoku ./examples/sudoku
./bin/sudoku -difficulty hard
./bin/sudoku -puzzle 53..7....6..195....98....6.8...6...34..8.3..17...2...6.6....28....419..5....8..79
```

## 2048

[`examples/2048`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/2048) is the sliding-tiles game on a 3×3 to 6×6 board, with undo, a win dialog that lets you keep going, and an autopilot that searches a couple of moves ahead. Merged tiles pop and the new tile lights up for a moment: the move sets the highlight, and an `Every` tick clears it.

<TermShot name="2048-demo">
The autopilot playing from a fixed seed until a 512 tile, the score and its sparkline climbing.
</TermShot>

```sh
go build -o bin/2048 ./examples/2048
./bin/2048                       # arrows, hjkl or wasd slide; u undoes
./bin/2048 -size 5 -autopilot
```

## pipeline

[`examples/pipeline`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/pipeline) is a CI run in the terminal: stages of parallel jobs with spinners, progress bars and timers, and the selected job's log streaming into a `<scroll stick="bottom">`. A flaky test fails on the first run; `r` retries it and the later stages go on.

<TermShot name="pipeline-demo">
The run from the start: build and test jobs in parallel, the integration log streaming, the failure, a retry with <code>r</code>, and every stage passing. The spinner frame, the bars, and the counts are <code>Derive</code>d from one simulation clock that a tick advances.
</TermShot>

```sh
go build -o bin/pipeline ./examples/pipeline
./bin/pipeline                   # j/k pick a job, r retries, R reruns, c cancels
./bin/pipeline -speed 8          # simulated seconds per real second
```

## music

[`examples/music`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/music) is a music player with no audio: a library table with a filter, a queue, albums, and a now-playing panel whose spectrum visualizer is a six-row `<sparkline>` the tick feeds.

<TermShot name="music-demo">
A track playing, a skip with <code>n</code> to the queued track, then <code>space</code> to pause, which lets the bars fall to rest. The position and volume bars use <code>bar: eighths</code>, and the tab labels carry live counts (<code>label="queue {counts.queue}"</code>).
</TermShot>

```sh
go build -o bin/music ./examples/music
./bin/music                      # space plays and pauses, n/p skip, a queues
./bin/music -speed 8             # run the playback clock faster
```

## pomodoro

[`examples/pomodoro`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/pomodoro) is a Pomodoro timer and a clock in big block digits. The digits are one derived string per frame shown in a `<text>` (the blinking colon is drawn into that string), and the phase colors and the pulse of the last ten seconds are `class:` guards on values the tick computes.

<TermShot name="pomodoro-demo">
The last seconds of a work session, the flash at zero, and the short break starting in green. The second tab is a clock with a small world-clock table whose offset columns drop out on narrow terminals through <code>&lt;column priority&gt;</code>.
</TermShot>

```sh
go build -o bin/pomodoro ./examples/pomodoro
./bin/pomodoro -start            # space starts and pauses, s skips, 2 shows the clock
./bin/pomodoro -work 50m -short 10m
```

## monitor studio

[`examples/monitor`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/monitor) is a system monitor's studio view, and the showcase for everything `version="2"` adds: nine tabs, a panel grid with gauges and sparklines, a scrolling grid of per-core bars, a process table with sortable columns and multi-select, a filter input, kill confirmations, a detail modal, settings that switch the mouse and the theme, and a hint bar built from the keymap. `theme="auto"` picks its palette from the terminal.

<TermShot name="monitor-processes">
The processes tab after <code>space down space down</code>: two rows marked with <code>▸</code> (<code>check-toggle</code>), protected processes in red through a row <code>class:blocked</code> guard, hot CPU cells through a cell guard, and a scrollbar on the table's border.
</TermShot>

<TermShot name="monitor-overview">
The overview tab: <code>layout: grid</code> with <code>grid-min-width</code> places the panels in two columns here and one on a narrow terminal. Gauges use <code>bar: eighths</code>.
</TermShot>

<TermShot name="monitor-cpu">
The cpu tab: a grid of cores inside a <code>scroll</code> with <code>scrollbar: auto</code>, four per row at this width.
</TermShot>

<TermShot name="monitor-narrow">
At 56 columns: the nine labels no longer fit even in their <code>short</code> form, so the strip shows only the active tab as <code>‹ 7 processes ›</code>; <code>@media</code> rules hide the four table columns that no longer fit, and the hint bar drops the hints it has no room for.
</TermShot>

The Go host has no layout, width, padding, truncation, or column code. It runs a seeded fake collector and handles the named actions: sorting (the sort arrow in the header is data), filtering, kill and force kill, the pinned detail, and the settings.

```sh
./bin/monitor                    # run it
./bin/monitor --theme light      # Set("@theme", "light") before Run
./bin/monitor --dump 120x30      # print Dump() as JSON instead
```

## logs

[`examples/logs`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/logs) is a live log viewer, like `tail -f`: lines stream in, colored by level, and the view follows the end until you scroll up to read. A filter input and the `1`-`4` level toggles narrow what is shown, and a sparkline tracks lines per second.

<TermShot name="logs-live">
The log is a <code>&lt;scroll stick="bottom"&gt;</code>: it keeps showing the newest line as the content grows, and stops following while you read further up. Levels are colored with <code>class:</code> guards on each line.
</TermShot>

The host streams lines with `Every`, which appends them in one `Batch` per tick, and `Derive` keeps the filtered lines and the counts, so the markup has no expressions. `-follow FILE` tails a real file instead of the built-in generator.

```sh
go build -o bin/logs ./examples/logs
./bin/logs                       # a seeded fake API service
./bin/logs -follow app.log       # tail a real file
```

## tasks

[`examples/tasks`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/tasks) is a kanban board: three lanes of cards that you move, reorder, mark, create, edit, and delete from the keyboard, with a filter over every lane.

<TermShot name="tasks-board">
Three focusable lists bound to lanes that <code>Derive</code> computes from one <code>cards</code> array and the filter; the counts in the titles are bound too.
</TermShot>

<TermShot name="tasks-marked">
After <code>right space down space</code>: focus moved to Doing and two cards are marked (<code>checked</code> with <code>mark="✓"</code>), ready to move together with <code>H</code>/<code>L</code>.
</TermShot>

New and edited cards go through a form modal; deleting asks first. `-file board.json` keeps the board in a file, and a failed save shows in the status line through `RunWith`'s `OnError`.

```sh
go build -o bin/tasks ./examples/tasks
./bin/tasks -file board.json
```

## gitui

[`examples/gitui`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/gitui) is a small git client in the spirit of tig and lazygit: staged and unstaged files with a diff preview, the log with each commit's detail, and the branches with a checkout confirmation.

<TermShot name="gitui-status">
The status tab: two lists of changed files with colored status letters, and the selected file's diff, colored line by line with <code>class:</code> guards.
</TermShot>

<TermShot name="gitui-commit">
A commit's detail over the log tab. <code>enter</code> on a commit opens it through the Go host; <code>tuimark play</code> runs no host, so this picture sets the same flag with a <code>set:</code> step.
</TermShot>

The log is a table whose `<column priority>` values decide which columns go first on a narrow terminal, and each tab has its own keys through `<keymap when>`. By default it shows a sample repository; `-repo PATH` reads a real one with `git`, read-only unless you also pass `-write`.

```sh
go build -o bin/gitui ./examples/gitui
./bin/gitui                      # the sample repository
./bin/gitui -repo .              # this repository, read-only
```

## http

[`examples/http`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/http) is an HTTP API client, like Postman: a collection of saved requests, an editor for the method, URL, headers, and body, and the response with its status, timing, body, and headers.

<TermShot name="http-client">
A POST request in the editor and its 201 response: the body is a <code>&lt;scroll&gt;</code> of indented JSON, and the status line is colored by class.
</TermShot>

<TermShot name="http-headers">
After <code>h</code>: the response pane's Headers tab, a <code>&lt;table&gt;</code> of the response headers.
</TermShot>

Requests run in a goroutine and land with one `Batch`; while one is in flight, an `Every` tick animates the spinner and the elapsed time, and `esc` cancels it. Without `-base`, the collection targets a fake API the host serves in process, so it works offline.

```sh
go build -o bin/http ./examples/http
./bin/http                       # against the built-in fake API
./bin/http -base https://api.example.com
```

## kit-gallery

[`examples/kit-gallery`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/kit-gallery) shows every component of the [kit](/guide/kit) on five tabs: badges, buttons, alerts, cards, stats and progress; a form and a menu; a data table, a log panel and an empty state; a confirm dialog, a command palette, a help overlay and toasts; and a split pane.

<TermShot name="kit-gallery">
The basics tab under the kit's two stylesheets and a short <code>app.tcss</code>. Every color is a token, so the same document follows the site's light or dark mode.
</TermShot>

<TermShot name="kit-gallery-data">
The data tab after <code>3 focus:#svc down</code>: the cursor row of the focused table is a solid accent bar, and the log panel below follows its end.
</TermShot>

The kit's actions are generic (<code>dialog_confirm</code>, <code>toast_show</code>), and the Go host routes them by the prefix of the event's <code>source</code> before the last <code>__</code>. <code>Derive</code> filters the palette's commands and picks the split pane's detail, and an <code>Every</code> tick clears the toast.

```sh
go run ./examples/kit-gallery
go run ./examples/kit-gallery -theme light
tuimark run examples/kit-gallery/app.tui --data examples/kit-gallery/sample.json --mock examples/kit-gallery/mock.json
```

## containers

[`examples/containers`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/containers) is a container-engine dashboard in the spirit of Docker Desktop or k9s, built only from the [kit](/guide/kit) over a simulated engine (no Docker is touched). A filterable, sortable, multi-select table of containers with `<column priority>`; a detail pane with an overview card, a CPU and a memory `sparkline`, and nested tabs for the log (`<scroll stick="bottom">`), the environment and the ports; images, volumes, and an events log.

<TermShot name="containers">
The containers tab: two marked rows (<code>space</code>), the sort in the table title, and the status bar showing the engine degraded because one container exited with a failure.
</TermShot>

<TermShot name="containers-detail">
The detail tab after <code>2</code>: the overview card, the CPU and memory sparklines, and the log below them, which keeps following its end as lines arrive.
</TermShot>

<TermShot name="containers-demo">
A session: filter the table with <code>/</code>, mark two rows and stop them through the confirm dialog, start them again, sort by CPU, open the detail and flip through its tabs, restart from the command palette, then watch a scripted crash and its restart in the events before the help opens.
</TermShot>

The kit's actions are generic (<code>dialog_confirm</code>, <code>palette_run</code>) and the host routes them by the prefix of the event's <code>source</code>. <code>Every</code> advances a seeded simulation once a second; <code>Derive</code> keeps the visible rows (the filter and the sort), the counts, the container under the cursor with its log, and the palette's matches, so typing in the filter updates every pane; no handler recomputes the views (one only keeps the cursor on a visible row).

```sh
go run ./examples/containers
go run ./examples/containers -theme light
tuimark run examples/containers/app.tui --data examples/containers/sample.json --mock examples/containers/mock.json
```

## inbox

[`examples/inbox`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/inbox) is a ticket inbox: a search input, a list bound to an array, and a detail pane that shows the selected ticket, in a `version="1"` document.

<TermShot name="inbox">
After <code>tab</code>: focus moved from the search box to the list, whose cursor row is reversed.
</TermShot>

<TermShot name="inbox-narrow">
At 60 columns an <code>@media (max-cols: 80)</code> block stacks the list above the detail pane.
</TermShot>

The host wires three actions: `open` shows the selected ticket, `search` filters the list, and `quit` stops.

## dashboard

[`examples/dashboard`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/dashboard) is a deploy dashboard with two screens, a pipeline list, a progress bar, buttons, a scrolling log, and a confirm modal. Header and footer are docked.

<TermShot name="dashboard" />

The Logs and Back buttons switch screens with `Set("@screen", …)`; Deploy (or `d` on it) opens the confirmation. Once confirmed, the host runs a fake deploy in a goroutine that advances the steps and the progress bar with `Set`, which is safe from any goroutine.

## agent

[`examples/agent`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/agent) is a small, real coding-agent chat: a transcript, a composer, tool activity and touched files, plan and build modes, and approval modals for edits and commands. It talks to a scripted provider or a local Ollama model, and has a headless mode.

<TermShot name="agent" />

The whole view is `agent.tui` and `agent.tcss`; the Go code supplies data and named actions only. See its [README](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/agent) for the providers, the workspace sandbox, and the flags.

```sh
go build -o bin/agent ./examples/agent
./bin/agent --demo-workspace ./demo-workspace
```

## unicode

[`examples/unicode`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/unicode) checks the hard part of terminal text: CJK characters two columns wide, emoji, and combining accents, in a list, in wrapped text, and next to borders.

<TermShot name="unicode" />

Widths are measured by grapheme cluster, a cluster is never split when text wraps or is cut, and borders stay aligned. Its dump has `"wide": true`, and `--cells` maps every column.

## spike

[`examples/spike`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples/spike) is where Tuimark started: five tags, two bordered boxes, and a status line, with no stylesheet. Its goldens are frozen, so a change to its geometry is a runtime regression, never an update.

<TermShot name="spike" />

## Write your own

Start from [getting started](/guide/getting-started), borrow freely from these files, and keep the [widgets](/guide/widgets) page open.

`logs`, `tasks`, `gitui`, `http`, `snake`, `pipeline`, `music`, `pomodoro`, and `containers` also ship a `mock.json`, so you can try their views in the terminal with no Go at all through [`tuimark run`](/guide/run):

```sh
tuimark run examples/tasks/app.tui --data examples/tasks/sample.json --mock examples/tasks/mock.json
```
