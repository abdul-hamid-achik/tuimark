# Changelog

All notable changes to Tuimark are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
follows [Semantic Versioning](https://semver.org/) for its pre-1.0 release
line.

## [Unreleased]

### Added

- **The Tuimark kit (v0).** `kit/` is a copy-paste component kit in the
  spirit of shadcn: two stylesheets (`theme.tcss`, the dark and light design
  tokens on top of the runtime's ten required ones; `kit.tcss`, the `k-`
  component classes) and 21 small documents under `kit/components/` (badge,
  button, card, alert, app bar, status bar, key hints, confirm dialog, form
  field, empty state, data table, menu, tabs, labeled progress, stat, toast,
  command palette, help overlay, split pane, log panel, breadcrumb), each with
  a contract comment, markup to copy, and a sample. Classes are `k-<component>`
  and `k-<component>-<part>` with short modifiers used only next to them, ids
  are `instance__part`, and actions are `component_verb`. It adds no tag,
  attribute, or property.
- **`examples/kit-gallery`**: every component in one app on five tabs, with a
  Go host that routes by the prefix of an event's source, a `mock.json`, and
  tests that hold the kit's conventions to every document.
- More fuzz targets (internal; `task fuzz` runs each briefly, outside
  `task verify` and CI): the document parser (diagnostic codes, positions,
  determinism, `fmt` keeps the diagnostics), the binding, key-token and
  size grammars, the stylesheet cascade, `play` step parsing, and a
  whole-runtime render of any document or any stylesheet (grid shape,
  snapshot independence, no hang). About an hour of local fuzzing found no
  crash.
- **Kit host helpers (`kit/go`, `kit/ts`).** The behavior markup cannot
  express, as files you copy into your project and own, not a
  public package: `route` (an event's instance and part), `toasts` (a capped
  queue that expires), `filter` (substring or fuzzy, into a store path with
  counts), `sortpage` (column sort cycling with arrows, and pages), `form`
  (per-field rules writing `forms.<form>.<field>.error` and
  `forms.<form>.valid`), `dialog` (`Confirm`, routed by instance), `palette`
  (a command registry), `async` (`<path>.loading`, `.error`, `.data`) and
  `format` (bytes, durations, relative time, percentages, thousands). `kit/go`
  is a nested module named `tuimarkkit` (no dot, so `go get` cannot fetch it)
  whose tests drive the real kit components; `kit/ts` is typed against
  `@tuimark/host`. Both run in CI and in `task verify`. For the helpers to be
  sound, two kit contracts moved: a form is named by the id prefix of its
  inputs (`profile__name`) with its state at `forms.<form>.<field>.{value,
  error}`, and the palette's input fires a `palette_query` action as the query
  changes (a host without `Derive`, such as a TypeScript parent, recomputes
  `found` there).
- **`examples/containers`**: a container-engine dashboard built only from the
  kit, over a simulated engine (a seeded state machine on an injected clock; no
  Docker). A filterable, sortable, multi-select table with `<column priority>`,
  a detail pane (overview card, CPU and memory `sparkline` pair, nested tabs for
  a `stick="bottom"` log, the environment and the ports), images, volumes and an
  events log; stop, restart and remove (a danger dialog) and the prunes through
  the kit's confirm dialog, a command palette filtered by `Derive`, a help
  overlay, toasts, a status bar and key hints. It ships `sample.json` and
  `mock.json` (generated snapshots, kept in step by a test), tests for the
  view at 40, 80 and 120 columns in both themes with every tab and modal open,
  the kit's conventions and the `Play` flows, and a `-demo COLSxROWS` mode that
  the docs site turns into an animated SVG. `loadfs_test.go` now gives any
  example that loads `../../kit` the repository root as its file system.

## [0.4.1] - 2026-10-18

A patch on 0.4a. `version` on `<tui>` becomes optional, and `examples/`
grows by thirteen apps: four tools, four animated apps (Snake among them),
and five more games, with animated demos of nine of them on the docs site.

### Changed

- **`version` on `<tui>` is optional.** A `<tui>` without it is the
  current language, `version="3"`: it accepts the whole vocabulary, and
  `L008`/`L009` and the dump's `clipped` apply to it. `V014` is never
  reported any more; its row stays in the catalog, marked retired. A
  document that writes `version="1"`, `"2"`, or `"3"` keeps exactly the
  meaning it had, and any other value is still `V003` and read as
  `version="1"`. `tuimark new`, the examples, and the docs write no
  version, except the SPEC fixtures (`examples/inbox`, `examples/monitor`)
  and the examples that are `version="1"` on purpose. The language server
  completes a document without `version` with the current vocabulary.

### Added

- **Four example apps** in `examples/`, each a document, a
  stylesheet with dark and light palettes, sample data, a `mock.json` for
  `tuimark run --mock`, and a Go host on the 0.4 API with its own tests:
  - `logs`: a live log viewer. Lines stream in with `Every`, the view
    follows the end with `<scroll stick="bottom">`, and `Derive` keeps the
    filter and the counts. `-follow FILE` tails a real file.
  - `tasks`: a kanban board with three lanes, multi-select, a form modal,
    and a delete confirmation. `-file` keeps the board in a JSON file.
  - `gitui`: a git client with status and a diff preview, the log with
    each commit's detail, and the branches. `-repo PATH` reads a real
    repository, read-only unless `-write` is also given.
  - `http`: an HTTP API client with a collection, a request editor, and
    the response's body and headers. It runs against an in-process fake
    API unless `-base URL` is given.
- `examples/README.md` lists every example and what it shows. The docs
  site's Examples page has screenshots of the four new apps.
- **Four animated example apps**, built the same way:
  - `snake`: a playable Snake whose `Every` tick re-arms faster as the
    level rises; `-autopilot` plays it.
  - `pipeline`: a CI run with parallel jobs, spinners, progress bars, the
    selected job's log streaming into a `<scroll stick="bottom">`, a flaky
    test, retry, rerun, and cancel.
  - `music`: a music player with no audio, with a library, a queue,
    albums, and a spectrum visualizer drawn by a six-row `<sparkline>`.
  - `pomodoro`: a Pomodoro timer and a clock in big block digits drawn
    from one derived string, with phase colors from `class:` guards.

  Each has a `-demo COLSxROWS` flag that replays a scripted session on a
  fake clock and prints its frames as JSON lines.
- **Five more games**, built the same way and with the same `-demo` flag:
  - `chess`: legal move generation checked by perft, SAN, UCI, FEN and PGN,
    an alpha-beta engine with six levels that thinks in a goroutine, four
    modes (against the computer, two players, computer against computer,
    review), clocks, undo and redo, hints, promotion, resign and draw
    claims, and a board of 64 static clickable cells.
  - `tetris`: SRS rotation with wall kicks, a 7-bag, hold, a ghost piece,
    lock delay, a line-clear flash, T-spins, combos, and an autopilot.
  - `minesweeper`: beginner, intermediate, expert and custom boards, a safe
    first reveal, flood fill, chording, flags and question marks, and a
    flag mode for the mouse.
  - `2048`: a 3×3 to 6×6 board, undo, win and game-over modals, highlights
    for merged and new tiles, and an expectimax autopilot.
  - `sudoku`: a generator whose puzzles have one solution and are graded by
    the solving techniques they need, notes, hints, a check, conflicts,
    undo and redo, and a clickable number pad.
- The docs site's Examples page shows those nine as animations: the
  generator renders the `-demo` frames as one CSS-animated SVG per theme
  (only the cells that change are stored per frame), and each has a pause
  button. Readers who prefer reduced motion start on the last frame.

## [0.4.0] - 2026-10-17

0.4a is the "frontend loop" release. It adds the tools that let you build
a terminal app the way you build a web front end:

- see the document live and edit it while it runs (`tuimark run`);
- start a project from a template (`tuimark new`);
- get diagnostics and completion in the editor (`tuimark lsp` and a VS
  Code extension);
- drive the view from TypeScript (`@tuimark/host`);
- keep timers and computed values out of the markup (`Every`, `Derive`).

The vocabulary does not change: `version="3"` is still the newest, and
every document keeps its meaning and its dumps.

### Added

- **Go API.**
  - `RunContext(ctx, w)` and `RunWith(ctx, w, RunOptions{OnError})` run
    the app under a context. A cancellation ends the session as a quit
    and restores the terminal.
  - `OnError` decides whether an error from a handler, an `Every`, or a
    `Derive` ends the session or is shown and survived.
  - `Quit()` ends the active session from any goroutine.
  - `Every(d, fn)` runs `fn` every `d` on the loop's goroutine, as one
    `Batch` with one redraw. Late ticks are coalesced, and `stop` is safe
    from anywhere.
  - `Derive(path, fn)` keeps a computed value (a filtered list, a count)
    in the store. It is recomputed inside every write, so nothing ever
    sees a stale value, and it is read-only for every writer: a widget
    bound to it does not change.
- **`tuimark run FILE`** shows a document live in the terminal, with no
  Go and no parent program, and reloads it in place when you save.
  - Host actions are answered by `run` itself: `--mock FILE.json` gives
    each one fixed store writes, so a prototype can react without a
    backend.
  - `F12` (or `--strip`) shows a strip of the last events, and
    `--events FILE.ndjson` writes them as the host protocol's `event`
    lines. A `secret` input's value is redacted in both.
  - A panel lists the document's diagnostics, or why a reload failed.
  - A reload keeps the store, the focus, cursors, scroll offsets, inputs,
    and the active tab for every node whose `id` and tag are unchanged.
  - Two `ctrl+c` within one second always end the session.
- **`tuimark catalog`** prints the whole vocabulary as JSON: tags,
  attributes and their keywords, CSS properties and values,
  pseudo-classes, media features, actions, key tokens, and diagnostic
  codes, each with the `version` that introduced it. The format is
  described by `schema/catalog.v1.json`.
- **`tuimark new NAME [--go | --ts]`** writes a project that validates
  cleanly at 40, 80, and 120 columns under both themes: `app.tui`,
  `theme.tcss`, `sample.json`, and an `AGENTS.md` briefing for coding
  agents (`tuimark agents --project`). `--go` adds a `main.go` host and
  `--ts` a `parent.ts` built on `@tuimark/host`. The Go module is named
  after the directory, and a name Go refuses is rewritten: `log` or
  `time` (a standard-library root), `std`, `cmd`, and a Windows device
  name such as `aux` get `-app`, so `tuimark new log --go` builds.
- **`tuimark lsp`** is a Language Server Protocol server for `.tui` and
  `.tcss`.
  - Diagnostics are exactly `validate`'s. They are routed to the
    stylesheet they belong to, and a sibling `sample.json` is used as the
    data.
  - Completion, hover, go to definition, and formatting with `tuimark
    fmt`.
  - It reads only regular files, at most 64 MiB each (a `sample.json`,
    a `.tui` or a `<style src>` that is a link to `/dev/zero` or
    `/dev/stdin`, or a FIFO, counts as absent). Its diagnostics for one
    document never arrive out of order, and a save or a watched file
    refreshes those of a stylesheet nobody has open.
- **VS Code extension** (`editors/vscode`): TextMate grammars that mark
  anything outside the closed vocabulary, snippets, and a client for
  `tuimark lsp`. Without the binary, highlighting and snippets still
  work. The server starts only in a trusted workspace, since a workspace
  can set `tuimark.path`, and only from a binary whose `version` prints a
  tuimark version.
- **`@tuimark/host`** (`sdk/ts`), the TypeScript client of the host
  protocol, for Bun. `spawnHost` starts `tuimark host`, and `on`, `set`,
  `batch`, `get`, and `exited` do the rest. It follows every rule for
  parents: signal forwarding, `SIGTERM` then `SIGKILL`, and never
  touching the terminal. A `SIGTERM` or `SIGHUP` is forwarded after a
  1.5 s grace, and only if the child is still running, so a signal sent
  to the whole process group does not reach the child twice. Messages of
  any size go through fd 3. `examples/host-ts` now uses it.
- **`TUIMARK_LOG`** writes a `{"ev":"rounds","queued":N}` record when the
  lifecycle rounds hit their cap.

### Changed

- **Faster frames for big lists.** Moving the cursor in a 20 000-row
  list now takes about 5.5 ms per key instead of about 120 ms (Apple M5).
  A one-path `Batch` over a 20 000-row store, with the frame it draws,
  takes about 5.5 ms instead of 64 ms. Six things do it:
  - an unchanged frame is not rebuilt;
  - a list's rows are reused while the data they read is unchanged;
  - equal rows share one computed style;
  - a frame's rows are built in memory kept from the frame before last;
  - a `Batch` copies only the parts of the store it writes;
  - layout measures a list's height once a frame, and checks each box
    as it places it instead of in one more pass.

  Dumps are byte for byte the same. `task bench` prints the numbers.
- **Lifecycle events past the 8th round are kept**, not dropped. They are
  dispatched first at the next settle, whatever causes it: a key, a
  resize, a tick, or a `Play` step.
- **`tuimark validate` says when it did not check bindings.** Without
  `--data` the text output ends with `ok (bindings not checked: no
  --data)`, and the JSON output has a `data` member (the path, or
  `null`).
- **`tuimark host` bounds fd 3 lines.** A line of 64 MiB or more ends the
  session with `{"type":"error","error":"fd 3 line of 64 MiB or more"}`,
  instead of growing without limit. `tuimark mcp` already stopped there.

### Fixed

- **A list or table whose `key` is an object or an array** no longer
  shares that value between the row and the paths the runtime writes it
  to (`bind`, `checked`) or the events it fires: a `Set` through one of
  them, or a handler changing its event, changed the row too.

## [0.3.2] - 2026-10-17

A patch: fixes and friendlier diagnostics from a full review of 0.3.1.
No vocabulary changes; documents keep their meaning and their dumps,
except that a few invalid ones now say so.

### Changed

- **Diagnostics suggest a fix.** An unknown tag, attribute, CSS property,
  or theme within two edits of a known one ends its message with `; did
  you mean <list>?` (or `"width"`, naming the version a newer name needs).
  A common HTML name gets the Tuimark way to say it instead: `<div>` → use
  `<box>`, `<col>`, or `<row>`; `<span>`/`<p>` → `<text>`; `<checkbox>` →
  a `<list>` with `checked=`; `<include>` → there are no components or
  includes. The codes are unchanged.
- **`tuimark play --input`** explains a token that fails right after a
  `text:` or `paste:` step: `--input` splits steps at spaces, so text with
  a space needs `--script`.
- **`App.Dump`** refuses a size above 1000 in either dimension, the limit
  `Play` and the CLI already had, instead of trying to allocate the grid
  (a huge size panicked).
- **The viewport key clamp of 0.3.1 applies to every document version.**
  It fixed an `up` lost after `end` in the same terminal read, so it is
  kept for `version="1"` and `version="2"` too.

### Fixed

- **`@media` error recovery.** A selector with no `{` inside an `@media`
  block no longer swallows the block's closing `}`, which made every rule
  after the block conditional on the media query.
- **Selectors that can never match are `V003`.** `#1`, `.9x`, and `.-x`
  name an id or class no element can have (§7's grammar), in stylesheets
  and in keymap `when` alike.
- **A leading `+` is not a cell count.** `gap: +3`, `padding: +1`,
  `gap="+3"`, and `@media (max-cols: +80)` are `V003`.
- **`tuimark fmt` keeps the XML declaration** (`<?xml version="1.0"?>`)
  instead of dropping it.
- **Clearer messages.** `V014` names `version="3"`; a bare `&` followed
  later by a `;` says "bare '&'" instead of naming a garbled entity; a
  DOCTYPE or processing instruction after the root gets its own message;
  text inside an unknown tag no longer adds a `V013`; `box:` says the
  pseudo-class is empty.
- **`tuimark mcp` accepts valid JSON with numbers beyond a double**
  (`1e400`, a 400-digit literal): such a line was answered `-32700` (parse
  error), although only a line that is not JSON gets it. An `id` is echoed
  as written.
- **`tuimark fmt --write --check`** is a usage error instead of silently
  running only the check, and `tuimark version` refuses arguments.
- **`tuimark test`** rejects a manifest entry whose `name` is empty, not a
  single path segment, or already used (ignoring case), so no entry can
  write into another's goldens.

### Docs and tests

- The README's language notes say that `wrap: wrap` collapses runs of
  spaces and breaks at an NBSP, and that store numbers are doubles
  (integers beyond ±2^53 lose precision; pass them as strings). The Go
  docs of `Bind`, `Set`, and `Batch` and the host fd comments are
  corrected.
- New fuzz targets for the stylesheet parser and `fmt` idempotence, a
  property test of the ANSI frame diff, and tests of `internal/play`'s
  step grammar. CI cross-compiles and vets for Windows (amd64, arm64).

## [0.3.1] - 2026-09-28

0.3b rounds out `version="3"`'s layout vocabulary and adds two ways to
drive a document from outside Go: `tuimark host`, a wire protocol for a
parent process in any language, and `tuimark mcp`, a Model Context
Protocol server for coding agents. Everything is additive: `version="1"`
and `version="2"` documents keep their meaning and their dumps.

### Added

- **Layout and CSS (`version="3"`).** `row-gap` and `column-gap` (0-4)
  split `gap` into its two axes — column siblings, grid rows/columns, and
  tab/hint strips on one, stacked siblings on the other — while `gap`
  stays their shorthand in the cascade, so a document that sets only `gap`
  lays out as before. `wrap: truncate-start` and `wrap: truncate-middle`
  join `truncate`, keeping the end (`…efgh`) or both ends
  (`/Syste…/Data`) of a cut line instead of the start. `<column
  priority="N">` hides a table column, lowest priority first, once the
  visible columns no longer fit, and brings it back once they do, in
  place of `@media` rules for column hiding. `<sparkline scale="NAME">`
  shares one value range across every sparkline of a frame with the same
  name, so a group of gauges reads on the same scale.
- **Several `<keymap>` elements** are now allowed, in every version: their
  rows form one keymap in document order. `<keymap when="SEL">`
  (`version="3"`) gives every row inside it that `when`, unless a row
  writes its own, which replaces it outright.
- **`<scroll id="log" stick="bottom">`** (`version="3"`) keeps a growing
  viewport — a log, a chat — showing its end as content arrives, and stops
  sticking while you scroll up to read back, resuming once you scroll (or
  `move-last`) back to the end.
- **`<modal focus="#id">`** (`version="3"`) names the node that takes
  focus when the modal becomes the top one, and the fallback when the
  node focused inside it is lost (it leaves the frame or can no longer
  take focus).
- **Tab label templates.** In `version="3"`, a `<tab>`'s `label` and
  `short` are templates, resolved every frame like a `<text>` body
  (`{path}`, no expressions); in `version="1"`/`version="2"` a `{path}` in
  either stays `V003`.
- **`tuimark host FILE [--data FILE.json] [--theme dark|light]
  [--reply-timeout 5s]`** runs a document in the terminal for a parent
  process written in any language: the terminal stays on fd 0/1/2 exactly
  as with `Run()`, the parent writes JSON lines to fd 3 (`set`, `bind`,
  `batch`, `get`, `reply`) and reads JSON lines from fd 4 (`ready` first,
  then `event`/`ack`/`error`, `exit` last). Each host action waits for the
  parent's reply up to `--reply-timeout` (default `5s`), and the fd 4
  queue is bounded, so a parent that stops reading ends the session
  instead of freezing the terminal. POSIX only; a usage error on Windows.
  `examples/host-ts` is a Bun parent that drives it, forwarding
  `SIGTERM`/`SIGHUP` to the child and never reading the terminal itself.
- **`tuimark mcp`** is a Model Context Protocol server on stdin/stdout
  (JSON-RPC 2.0, one message per line, standard library only) with five
  read-only tools that run the authoring loop: `tuimark_validate`,
  `tuimark_dump`, `tuimark_play`, `tuimark_inspect`, and `tuimark_agents`.
  Each returns exactly what the matching CLI command prints to stdout,
  byte for byte; a result is an error only when the command itself would
  exit 1 (a validation failure, exit 2, is a normal result). Register it
  with `{"command": "tuimark", "args": ["mcp"]}`.
- **`PlayOptions.Frames`** makes `Play` also return one `PlayFrame` per
  applied step, in `PlayResult.Frames` — the same frames
  `tuimark play --frames` prints.

### Changed

- **`Batch`'s returned error is now every failed part joined with
  `errors.Join`**, first each failed `Set`'s own error in call order,
  then `fn`'s own error when it differs. This is a **behavior change**:
  `errors.Is` still finds a sentinel error inside it, but comparing the
  result with `==` against a sentinel, or a type switch on it, no longer
  matches, since the returned value is always a joined `error`, even for
  one failure. Callers that compared `Batch`'s error directly need
  `errors.Is`/`errors.As`, or `Unwrap() []error` to walk every part.
- **`examples/monitor`** moves to the 0.3b vocabulary: its one `<keymap>`
  splits into several `<keymap when="…">` groups, each row keeping its
  place in dispatch order, `<hints>`, and the IR `keymap` array; `#cores`
  gains `row-gap: 0` so its rows touch while `gap: 1` still separates the
  columns; and the Processes table's columns hide by `priority` — `c-io`
  and `c-user` first, then `c-thr`, then `c-mem` — in place of the
  `@media` rules that hid them before. Its goldens and
  `monitor_resize.yml` are updated to match, and `scale` and modal
  `focus=` are deliberately left unused there.
- The `V002`/`V003` diagnostics for a 0.3b tag, attribute, or value
  (`stick`, `focus` on `modal`, `when` on `keymap`, `priority`, `scale`,
  the two new `wrap` values, `row-gap`/`column-gap`, a tab's `{path}`
  `label`/`short`) used in a `version="1"` or `version="2"` document now
  end in `(requires version="3")`, matching the existing hint for
  `version="2"` items used in `version="1"`.

### Fixed

- **A focused viewport's keys clamp at its end.** `down` or `end` at the
  end of a focused `<scroll>` stored an offset past it, so an `up` in the
  same terminal read was lost and the scroll ended one row off. Its keys
  now clamp to the viewport's maximum, as `move-*` and the wheel already
  did.

## [0.3.0] - 2026-09-28

The host API grows for real embedding and testing, and a new document
version, `version="3"`, reports content that a view cuts without saying
so. Everything is additive: `version="1"` and `version="2"` documents
keep their meaning and their dumps.

### Added

- **`LoadFS(fsys, name)`** loads a document and its stylesheets from an
  `fs.FS`, so a view embedded with `go:embed` loads without being copied to
  disk. A `<style src>` that climbs out of the file system is a `V006`
  diagnostic.
- **`Get(path)`** returns a copy of a store value. `@focus`, `@screen`, and
  `@theme` return the app's current focus, screen, and theme.
- **`Batch(fn)`** applies several `Set`s as one change, all or nothing, with
  one redraw. No frame, dump, or handler sees half of it.
- **`Play(opts, steps...)`** drives the app without a terminal, for tests.
  It uses the step grammar of `tuimark play`, calls the registered
  handlers, keeps the state for the next call, and returns the events and
  the final dump. One engine now serves `tuimark play`, `tuimark test`, and
  `Play`, and the CLI output is unchanged.
- **`<tui version="3">`** is a `version="2"` document plus two layout
  warnings:
  - `L008`: a text loses columns or lines without an ellipsis.
  - `L009`: a node cuts a child on an axis it does not scroll.

  Dumps mark the node with `clipped`. A cut the author intends is silenced
  by writing `wrap: nowrap` or `overflow: hidden`. `tuimark ir` prints IR
  `"0.3"` for these documents (`schema/ir.v0.3.json`), and
  `schema/dump.v0.2.json` gains `clipped`.

### Changed

- **`examples/monitor`** is a `version="3"` document. Its overview grid
  now scrolls instead of being cut at 40–58 columns.
- The `V003` message for an invalid `version` now lists `"1"`, `"2"`, or
  `"3"`.

### Fixed

- **Release.** The release workflow now publishes the Homebrew cask. For
  v0.2.0 the cask upload failed on a template function GoReleaser does not
  define, and the cask was published by hand from the release's checksums.
- **`examples/monitor`.** Enter or space no longer confirms a kill. The
  confirmation's buttons are `focusable="false"`: focus rests on the
  modal, so only `y`, `n`, and `esc` act there, and a click on either
  button still works. Before, focus went to the first button, "yes (y)",
  and enter or space fired it. Views that copied the example should make
  the same change.

## [0.2.0] - 2026-09-27

The first public release. It implements version 0.2 of the language in two
parts: runtime, terminal, and tooling improvements that apply to every
document, and a new `version="2"` vocabulary that documents opt into.
Existing `version="1"` documents keep their meaning, and their dumps are
unchanged.

### Added

- **Installation.** Release archives for macOS, Linux, and Windows (amd64
  and arm64), a Homebrew cask (`brew install --cask
  abdul-hamid-achik/tap/tuimark`), and `go install
  github.com/abdul-hamid-achik/tuimark/cmd/tuimark@latest`.
- **`version="2"` documents.** The new vocabulary below is available only
  in `<tui version="2">`. In a `version="1"` document each new tag,
  attribute, value, property, pseudo-class, media feature, or built-in
  action is a diagnostic that ends in `(requires version="2")`.
- **New widgets.** `<table>` with `<column>` (header, per-row cell
  templates, column widths in cells, `%`, `fr`, or auto, and virtualized
  rows so only visible rows are laid out), `<tabs>`/`<tab>` (a tab strip
  with short labels for narrow terminals, and focus on activation),
  `<hints>` (a key-hint bar built from the keymap, with `label` and
  `keycap`), and `<sparkline>`.
- **Layout and style.** `layout: grid`, `scrollbar`, `bar: eighths` for
  smooth progress bars, `class:NAME="path"` guards, the `:checked` and
  `:focus-within` pseudo-classes, `theme="auto"` (asks the terminal for its
  background color, then falls back to `COLORFGBG`, then to dark), and
  `@media (theme: dark|light)`.
- **Interaction.** `each` on containers, multi-select on `list` and
  `table` (a `checked` array), built-in actions for moving, checking, and
  switching tabs, `when` guards evaluated over the focus chain, and mouse
  input (SGR reports: click on release and the wheel).
- **`tuimark play`.** Drives a document with simulated keys, text, paste,
  resizes, and mouse clicks without a terminal, and prints the resulting
  frames as text or JSON.
- **`tuimark inspect`.** Explains one node, by id or by cell: its path,
  classes, pseudo-classes, and each style property with the rule that set
  it and the rules it overrode.
- **More dump and CLI options.** `dump --styles`, `--theme dark|light` on
  `dump`, `validate`, `preview`, and `play`, `preview --color`, and a
  superset check in `tuimark test --update` so a golden only gains fields.
- **`TUIMARK_LOG`.** Writes an NDJSON log of a `Run()` session (start,
  keys, pastes, mouse reports, actions, and frames) and redacts input
  while a secret input has focus.
- **Environment variables.** `TUIMARK_COLOR`, `TUIMARK_THEME`, and
  `TUIMARK_SYNC` override color, theme, and synchronized-output detection.
- **Schemas.** `schema/ir.v0.2.json` and `schema/dump.v0.2.json`; the JSON
  dump of a `version="2"` document lists each node's `classes`.
- **Examples.** `examples/unicode` (CJK text and emoji) and
  `examples/monitor`, a process-monitor studio built with tables, tabs,
  hints, sparklines, the grid layout, and the mouse.
- **`tuimark version`** now also prints the commit and build date for
  release builds.

### Changed

- **Unicode.** Text is measured, cut, wrapped, painted, and edited by
  grapheme cluster with built-in Unicode 15 width tables, so wide
  characters and emoji no longer break borders or alignment. The JSON dump
  marks wide cells with `"wide": true`.
- **Auto-fit scrolling.** A `list`, `scroll`, or `overflow: scroll`
  container without a size on its scroll axis now fits the available space
  and scrolls (also in `version="1"` documents), with a new `L006` warning;
  dump nodes carry their `scroll` state.
- **Terminal output.** `Run()` probes the terminal once at start-up, picks
  a color profile (truecolor, 256, 16, or none) and downsamples colors to
  it, uses synchronized output when the terminal supports it, handles
  bracketed paste as a single edit, and never mistakes terminal replies or
  mouse reports for key presses.

## [0.1.0] - 2026-09-26

The first working version of the language and runtime. It was not
published as a release.

### Added

- **The view language.** UIs are written as `.tui` markup (XML), styled
  with `.tcss` (a reduced CSS with tokens, themes, and `@media`), bound to
  JSON data with `{path}` interpolation, `each`, and `if`, and wired to the
  host through named actions such as `on:select="open"`.
- **Widgets.** `screen`, `col`, `row`, `box`, `scroll`, `spacer`,
  `text`, `rule`, `list`, `item`, `input`, `button`, `progress`, `modal`,
  plus `keymap` and `bind` for key bindings, with focus handling.
- **A Go runtime with a small public API.** `Load`, `Parse`, `Bind`, `Set`,
  `On`, `Catalog`, `Dump`, `Validate`, and `Run`: an integer cell-grid
  layout engine, Unicode painting, and a side-effect-free snapshot for
  tests and tools.
- **The `tuimark` CLI.** `dump` (text or JSON, with `--data` and
  `--cells`), `validate` (`--strict`, `--catalog`), `preview` (with
  `--watch`), `fmt`, `ir`, `test` (golden dumps), and `agents` (generates an
  `AGENTS.md` for coding agents from the catalog).
- **Examples.** An inbox, a dashboard, a stacked responsive layout, and a
  local coding-agent harness with scripted, Ollama, and offline fixture
  providers.
- **Tests.** Conformance tests, golden dumps, and end-to-end terminal specs
  run with [Glyphrun](https://github.com/abdul-hamid-achik/glyphrun).

[Unreleased]: https://github.com/abdul-hamid-achik/tuimark/compare/v0.4.1...HEAD
[0.4.1]: https://github.com/abdul-hamid-achik/tuimark/releases/tag/v0.4.1
[0.4.0]: https://github.com/abdul-hamid-achik/tuimark/releases/tag/v0.4.0
[0.3.2]: https://github.com/abdul-hamid-achik/tuimark/releases/tag/v0.3.2
[0.3.1]: https://github.com/abdul-hamid-achik/tuimark/releases/tag/v0.3.1
[0.3.0]: https://github.com/abdul-hamid-achik/tuimark/releases/tag/v0.3.0
[0.2.0]: https://github.com/abdul-hamid-achik/tuimark/releases/tag/v0.2.0
