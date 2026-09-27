# Changelog

All notable changes to Tuimark are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
follows [Semantic Versioning](https://semver.org/) for its pre-1.0 release
line.

## [Unreleased]

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

[Unreleased]: https://github.com/abdul-hamid-achik/tuimark/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/abdul-hamid-achik/tuimark/releases/tag/v0.2.0
