# Changelog

All notable changes to Tuimark are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
follows [Semantic Versioning](https://semver.org/) for its pre-1.0 release
line.

## [Unreleased]

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

[Unreleased]: https://github.com/abdul-hamid-achik/tuimark/compare/v0.3.1...HEAD
[0.3.1]: https://github.com/abdul-hamid-achik/tuimark/releases/tag/v0.3.1
[0.3.0]: https://github.com/abdul-hamid-achik/tuimark/releases/tag/v0.3.0
[0.2.0]: https://github.com/abdul-hamid-achik/tuimark/releases/tag/v0.2.0
