---
title: Examples
description: Complete Tuimark apps from the repository - a system monitor studio, an inbox, a deploy dashboard, a coding-agent chat, Unicode text and the phase-0 spike - with screenshots rendered from tuimark play.
---

# Examples

Every example lives in the repository's [`examples/`](https://github.com/abdul-hamid-achik/tuimark/tree/main/examples) directory as a `.tui`, a `.tcss`, and sample data, most with a small Go host. Each picture is rendered from `tuimark play` on those files (the command is in its title bar), so you can reproduce it exactly. Screens that come in two palettes follow this site's light or dark mode.

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
