# containers

A container-engine dashboard in the spirit of Docker Desktop or k9s, built
only from the [Tuimark kit](../../kit/README.md). Twelve simulated containers
(no Docker is touched) with live CPU and memory, logs that grow, and the odd
crash. The view is `app.tui`, which includes `kit/kit.tui` (the kit's sheets,
its overlay keys and its templates: the app bar, key / value rows, stat cards,
the toast, the confirm dialog, the palette and the help) over a short
`app.tcss`; the Go host simulates the engine and implements the named actions.

```sh
go build -o bin/tuimark ./cmd/tuimark
go run ./examples/containers                   # from the repo root
go run ./examples/containers -theme light
go run ./examples/containers -seed 3           # other CPU, memory and log noise
go run ./examples/containers -dump 120x40      # Dump() as JSON, no terminal
go run ./examples/containers -demo 100x24      # scripted frames as JSON lines (ignores -seed)

# the view alone, with no Go: a mock answers every action with fixed writes
tuimark run examples/containers/app.tui --data examples/containers/sample.json --mock examples/containers/mock.json
```

## Tabs

| Tab | What it shows |
|---|---|
| 1 containers | a table (name, image, status, CPU %, memory, uptime, ports) with a filter field, marks, a sort, and `priority` columns that drop on a narrow terminal |
| 2 detail | the container under the cursor: an overview card, a CPU and a memory `sparkline` pair, and nested tabs for its log (a `stick="bottom"` scroll that grows), environment and ports |
| 3 images | a table of images with sizes and how many containers use each |
| 4 volumes | a table of volumes and who mounts them |
| 5 events | the engine's event log, following its end |

## Keys

`1`-`5` / `[` `]` switch tabs, `ctrl+k` opens the command palette, `?` the
keys, `t` toggles the theme, `q` quits.

On the containers tab: `/` filters (`enter` hands the keys back to the table,
`esc` clears the filter), `space` marks a row (`a` marks all, `c` clears),
`o` cycles the sort, `enter` opens the detail, and `s` stop, `u` start, `r`
restart, `d` remove act on the marked rows, or on the cursor row when none is
marked. Stop, restart and remove ask first in a confirm dialog (`y` / `n`;
remove is a danger dialog with the cursor on Cancel).

On the detail tab: `l` / `e` / `p` switch the log, environment and ports,
`j` / `k` scroll, `G` follows the end, `esc` goes back. On images and volumes,
`p` prunes the unused ones.

## The host

`main.go`, `handlers.go`, `derive.go`, `sim.go` and `view.go` have no layout
code.

- `sim.go` is the engine: a state machine whose clock and random numbers are
  injected (a seeded `rand.Rand`, a time passed to each step), so a run is the
  same every time.
- `Every(time.Second, tick)` advances the engine and writes the containers,
  their logs, the events, the images and the volumes in one `Batch`.
- `Derive` keeps `rows` (the filter and the sort), `counts`, `detail` (the
  container under the cursor, with its log) and `found` (the palette's
  matches), so typing in the filter or moving the cursor updates every pane;
  no handler recomputes the views (one only keeps the cursor on a visible row).
- The kit's actions are generic (`dialog_confirm`, `palette_run`); the dialog's
  instance is the prefix of the event's `source` before the last `__`. One
  dialog serves every operation, so the host writes its title, body and button
  labels (`confirm.yes`, `confirm.no`) on every open; the palette's and the
  help's titles are props of their `<use>`, not data.

`sample.json` and `mock.json` are snapshots of the real app twelve simulated
seconds in, derived paths included, so `validate`, `dump` and `run --mock` work
with no Go. A test keeps them from drifting
(`UPDATE_SAMPLE=1 go test ./examples/containers -run TestSampleInSync`
rewrites them).

## Tests

`go test ./examples/containers/` checks that:

- the document has no diagnostic (warnings included) at 40, 80 and 120 columns
  under both themes, with every tab and nested tab visited and every modal open,
  for several kinds of container and for an empty engine;
- the kit's conventions hold (`kit_test.go`): `k-` classes and short modifiers,
  styling by id in `app.tcss`, `instance__part` ids, `component_verb` actions,
  tokens defined for both themes, overlays that are kit templates with
  `on:escape` and a scoped keymap that comes first, sized containers,
  `focusable` on every `tabs`, no `version`;
- `Play` flows work: filtering, marks, sort, opening the detail and its nested
  tabs, stop / start / restart / remove through the dialog, pruning, the palette,
  the help overlay, the theme toggle;
- the log grows and follows its end, a scripted crash shows in the events and the
  container comes back, and the toast clears on the tick;
- the simulation is deterministic, and `sample.json` and `mock.json` match the app.
