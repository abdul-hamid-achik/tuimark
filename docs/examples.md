---
title: Examples
description: Complete Tuimark apps from the repository - a system monitor studio, a live log viewer, a kanban board, a git client, an HTTP client, an inbox, a deploy dashboard, a coding-agent chat, Unicode text and the phase-0 spike - with screenshots rendered from tuimark play.
---

# Examples

Every example lives in the repository's [`examples/`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples) directory as a `.tui`, a `.tcss`, and sample data, most with a small Go host. Each picture is rendered from `tuimark play` on those files (the command is in its title bar), so you can reproduce it exactly. Screens that come in two palettes follow this site's light or dark mode.

`tuimark play` runs no host program, so these pictures show the sample data and what the built-in keys do. The apps' own actions, such as sending a request or moving a card, need the Go host.

To run them, clone the repository and build:

```sh
git clone https://github.com/abdul-hamid-achik/tuimark
cd tuimark
go build -o bin/tuimark ./cmd/tuimark
go build -o bin/monitor ./examples/monitor
./bin/monitor
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

`logs`, `tasks`, `gitui`, and `http` also ship a `mock.json`, so you can try their views in the terminal with no Go at all through [`tuimark run`](/guide/run):

```sh
tuimark run examples/tasks/app.tui --data examples/tasks/sample.json --mock examples/tasks/mock.json
```
