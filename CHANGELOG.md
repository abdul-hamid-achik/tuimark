# Changelog

All notable changes to Tuimark are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
follows [Semantic Versioning](https://semver.org/) for its pre-1.0 release
line.

## [Unreleased]

## [0.4.3] - 2026-10-19

The 0.4b release. It extends the current language, the language of a
`<tui>` without `version`, in three directions:

- reuse and split markup: `<include>` and `<fragment>` put a document in
  several files, and `<template>` and `<use>` stamp a piece of markup with
  its own ids and its own data;
- form controls: `<checkbox>`, `<select>`, radio groups (`single`), a
  spinner (`<progress indeterminate>`), and `{path}` in a button's label;
- defaults for people coming from the web: content-sized `col` and `row`,
  an `esc` that closes a modal, a modal that traps the keymap, a focus look
  on buttons.

There is no `version="4"`: writing it is `V003`, and the document is read as
`version="1"`. Tools that need a number for the current language use 4. A
document that writes `version="1"`, `"2"`, or `"3"`, and the spike `<app>`
root, keeps exactly the meaning, diagnostics, and dumps it had in 0.4.1. A
tag, attribute, or `{path}` that 0.4.3 adds is `V001`, `V002`, or `V003` in
such a document, with a message ending in `(requires a <tui> without
version)`; an include's file is never read there.

### Changed — versionless documents

A `<tui>` without `version` changes layout and keys with 0.4.3. To keep
0.4.1's behavior, write `<tui version="3">`; nothing else in the document
needs to change. To see what a document gains or loses, copy it with
`version="3"`, run `tuimark dump --format json` on both at 80 and 120
columns, and diff the results. Measured this way on the 15 examples that
write no version, at five sizes, none changes its base layout, because each
writes the sizes it relies on; with a modal open, some show the focus look,
the trapped keymap's shorter hint bar, or content-sized hints. Documents that
leaned on the old defaults change in these ways.

**Layout.**

- **`col` and `row` are content-sized** (`auto` on both axes) except as
  direct children of a `screen`, a `modal`, or a `tab`, where they stay
  `1fr` by `1fr`. A sidebar `col` beside a body `col` is as wide as its text,
  and the body needs `width: 1fr` to take the rest; panes that are wider than
  their row push the last ones out (`L009`), and a `wrap: wrap` text in a
  content-sized pane is measured unwrapped. The `col` and `row` children of an
  `each` container take their content size instead of equal shares, `justify`
  acts in containers that `1fr` children no longer fill, and a `%` child of a
  content-sized container is `L002`. A header or status `row` written directly
  in a `screen` is still `1fr`, so give it `height: 1`. `box` is unchanged (it
  was never `1fr` on its main axis).
- **The sibling-`fr` rule applies to viewports only.** An unsized sibling of
  an `fr` child becomes `1fr` only when it is a `scroll`, a `list`, a `table`,
  or has `overflow: scroll`; a `text`, `button`, `box`, `tabs`, or `hints` next
  to an `fr` pane stays at its content size. A filter `row` above a list in a
  `tab`, a `screen`, or a `modal` is `1fr` and splits the space with the list:
  give it `height: 1`.
- **`fr` meets `min-*` and `max-*` as CSS does.** A child that a `max` clamps
  (or a `min` lifts) is frozen at that size, and the other `fr` children share
  what it frees: three `1fr` columns in 40 cells, the first with `max-width:
  10`, are 10, 15, and 15, where a pinned document gives 10, 13, and 14 and
  leaves 3 cells empty. Table columns keep their old split.
- **`modal` and `hints` are content-sized.** A `modal` with no size is as big
  as its content, wide enough for its title, capped by a written `max-width`
  or `max-height` and otherwise by 80% of the screen (a pinned document
  gives 80% by 80%). A `hints` is as big as its items along their axis and
  drops the items that do not fit, with no diagnostic, so a status row of a
  text, a `spacer`, and `<hints/>` keeps the text and the hints whole.
- **An `input` is `max(width(placeholder) + 1, 20)` columns wide** when it
  has no `width`, whatever its value, so it no longer grows while you type.

**Keys and focus.**

- **`esc` closes the top modal** that has no `on:escape`, by writing `false`
  at its `open` path (its `bind` path when it has no `open`); the next frame
  fires `on:close` and gives focus back. A modal whose `open` is `true`,
  `false`, or `!path`, or whose path a `Derive` computes, stays open, because
  there is nothing to write: use one of these, or `on:escape`, for a dialog
  that must not be dismissed (the game-over modals of `snake` and `tetris`).
- **A modal traps the keymap.** While one is open, a keymap row fires only
  when its effective `when` matches the focus chain up to the top modal, so a
  `q` quit row without `when` no longer quits under a confirmation, and a
  `ctrl+c` row without `when` gives way to the built-in quit. `tab`,
  `shift+tab`, `ctrl+c`, and the keys the focused widget takes still work, and
  `<hints/>` shows only the rows that fire. A key that must also work under a
  modal is repeated in `<keymap when="modal">`, a group whose rows all carry
  that `when`.
- **A `tabs` is focusable** without `focusable="true"`: `tab` reaches it,
  `left` and `right` switch the tab, and focus stays on the strip when the
  active tab changes. `focusable="false"` turns it off. Specs that count `tab`
  presses change; a `screen focus=` keeps the old initial focus.
- **A focused `button`, `checkbox`, or `select` is reversed** by the built-in
  sheet (`button:focus, checkbox:focus, select:focus { reverse: true; }`).
  A document whose buttons have another focus style adds `reverse: false` to
  it; the kit's `.k-btn:focus` now does.
- **`ctrl+i`, `ctrl+j`, and `ctrl+m` in a keymap are `V003`**, once per
  token, because a terminal delivers them as `tab` and `enter` and they never
  matched. Write `tab` or `enter`. Pinned documents accept them as before.

**Messages and output.**

- **`L002` and `L009` name the fix.** `L002` under a `col` or `row` ends with
  `(width: 1fr fills it)` (or `height`), and `L009` on a content-sized `col`,
  `row`, or `box` says that the child is sized by its content and does not
  shrink, and to give it `width: 1fr` (or `flex: 1`), or to write `overflow:
  hidden` to keep the cut.
- **`tuimark ir` prints `"version": "0.4"` for a versionless document** (it
  printed `"0.3"`), validated by the new `schema/ir.v0.4.json`, and `tuimark
  inspect` ends with a `source` line (`--json`: a `source` member) naming the
  element a node was built from. Pinned documents print neither.

### Added

- **`<include src>` and `<fragment>`.** `<include src="parts/forms.tui"/>`
  places the elements of another file where it stands: directly under
  `<tui>`, in a `<screen>`, in a container, or in a `<keymap>`. `src` is
  relative to the file that writes the include. The file's root is
  `<fragment>`, which takes no attributes and holds any number of elements,
  `<template>`s, a `<style>`, a `<keymap>`, or further includes. Ids, styles,
  and data are shared and the result is checked as if written where the
  include stands; a diagnostic about a placed element ends with `, placed by
  <include> at FILE:LINE:COL`. A fragment is named by its path relative to the
  document, and the same fragment may be included twice (its ids then repeat:
  `V004`). Failures insert nothing and the rest of the document is still
  checked:
  - an include cycle is `V006`, judged by file identity, so a symbolic link
    cannot hide one; so are a file outside the file system `LoadFS` was
    given, one that cannot be read, and one that is not a regular file of at
    most 64 MiB;
  - a stylesheet, or a file whose root is not `<fragment>`, is the new `V020`.

  A fragment is not a document: `dump`, `play`, `preview`, `ir`, and
  `inspect` print `V005` for it ("check it through the document that includes
  it") and exit 2, while `validate` checks it on its own (`ok (fragment
  checked on its own)`, with or without `--data`) and `fmt` formats it.
- **`<template>` and `<use>`.** `<template id="field">` holds a piece of
  markup that is never drawn by itself, and `<use id="email" template="field"
  with="form.email as f"/>` is replaced, when the document is read, by a copy
  built where the use stands. A template is a direct child of `<tui>` or
  `<screen>`, in the document or in a fragment, and may come after its uses.
  A `<use>` is not a node, so it takes no space and holds no content (no
  slots).
  - **Scoped ids.** The copy's ids are prefixed with the use's id and two
    underscores: the `input` of `<use id="email">` is `email__input`, and a
    use inside a template adds its own prefix (`billing__street__input`).
    Outside the copy (`to=`, `when=`, `focus=`, `#id` selectors, `focus:`
    steps, an event's `source`) you write the scoped id; inside the template
    `#x` means the element of the same copy. A use needs an `id` when its
    template holds any (`V012`), and naming an unscoped id from outside is
    `B005` with the scoped form in the message.
  - **`with="path as alias"`** makes the alias stand for the path inside the
    copy, so `{f.label}` reads `form.email.label` and `bind="f.value"` reads
    and writes `form.email.value`; clauses are separated by exactly `, `. A
    copy also reads the aliases in scope where its use stands (a list row's
    `each` alias). A `with` path that is missing is one `B003` at the use and
    nothing more from the copy, and a flag read through an alias that stands
    for an object (`if`, `hidden`, `disabled`, `open`, `class:NAME`) is `null`
    when the member is missing, so a field shows an error only when the host
    wrote one.
  - **Guards and classes.** `if` and `hidden` on a use are read where the use
    stands and are decided per row in a list `<item>`, an `each` container, or
    a table. A static `class` on a use is added to every top-level element of
    the copy, after the template's own classes, so
    `<use template="confirm" class="danger">` styles one copy with an ordinary
    class rule. To reuse a table column or its row `<item>`, make it the
    template and put the use directly in the `<table>`.
  - **Errors.** A use without `template` or naming a missing template, and each
    use on a cycle of templates, is the new `V019`; a cycle is named as a
    simple loop from the use's own template (up to 8 templates whole, else the
    first 8, `… (N more)`, and the template it returns to), and a long ring
    is found in near-linear time. A malformed `with`, a repeated alias, or one
    already in scope is `V011`; any other attribute on a use is `V002`, naming
    the class as the way to style a copy.
  - **Messages about a copy** say which copy they mean. A path that went
    through an alias ends with `(P via <use id="email">)`, naming the path it
    stands for (a use without an id is `<use template="badge">`, with ` at
    FILE:LINE:COL` when the path starts with an `each` alias); any other
    diagnostic ends with `, placed by <use …> at FILE:LINE:COL`. Each is
    reported once per written element, and an id, path, or template name a
    message quotes is cut at 64 bytes, at a character boundary, followed by
    `…`.
- **Expansion limits, `V021`.** Includes and uses share a budget per
  document: 100000 elements inserted (counted once per copy, at every depth),
  16 MiB of placed source (fragment files, copied templates, scoped-id
  prefixes, and the paths `with` substitutes), and 256 levels of nesting. The
  include or use that would go over inserts nothing, no later one expands,
  and the rest of the document is still reported. A fragment included inside a
  `<template>` has a budget of its own, so a template whose include fans out
  is reported whether or not any use copies it.
- **`single` on `list` and `table`.** With `checked`, `single="true"` makes
  the path hold one row key, or `null`, instead of an array: a radio group.
  `space` on the cursor row chooses it (choosing the checked row again keeps
  it), `check-none` clears it, and `check-all` never applies (`B007`).
  Without `checked` it is `V018`; a checked path that is an array or an object
  is `B008`.
- **`<checkbox>`.** `<checkbox id="dark" bind="settings.dark" label="Dark
  mode" on:change="dark_changed"/>` paints `[x] Dark mode` or `[ ] Dark mode`
  (`:checked` and `:focus` match). `space` toggles it, writing a boolean to
  `bind` (without `bind` the runtime keeps the value by `id`) and firing
  `on:change` with it; `enter` goes on to the keymap, so a form's submit row
  fires from it. A click toggles it too, and its `label` is a template like a
  `<text>` body. A bound value that is not a boolean is the new `B011`, a
  warning.
- **`<select>`.** `<select id="tz" bind="tz" each="zones as z" key="z.id"
  label="{z.name}" placeholder="zone" on:change="tz_changed"/>` is one row: the
  chosen option's label or the dim placeholder, then `▾`. `enter`, `space`, or a
  click opens its option list below it (above it when there is more room
  there), drawn over the screen and over every modal; the arrows, `home`,
  `end`, `pgup`, and `pgdn` move its cursor, `enter`, `space`, or a click picks
  (writing the key and firing `on:change` only when it changed), and `esc`
  closes it. `tab`, `shift+tab`, and `ctrl+c` close it and then act as usual.
  Style it with `select > list` and `select > list > item`. It needs `id`,
  `each`, and `key` (`V012`, `V017`); a bound value that is no option's key is
  the new `B012`, a warning. `validate` without `--data` skips `B011` and
  `B012` with the other data checks.
- **`<progress indeterminate="true">`** is a one-glyph spinner (`⠋⠙⠹…`, ASCII
  under the 16-color profile) that turns every 100 ms. In a live session it
  redraws on a session clock, painting only; it builds no frame, fires no
  `Every`, and writes no `TUIMARK_LOG` record. Combined with `bind` or `value`
  it is `V003`.
- **`tick:DUR` in `play`** advances a virtual clock by `DUR` (`250ms`, `1.5s`,
  `1m30s`; the units `ms`, `s`, `m`, and `h`, from `1ms` to `1h`), which turns
  a spinner and calls a Go host's `Every` functions that fall due, so
  time-based state can be tested. It is also `{"tick":"250ms"}` in a
  `--script`, `"input": "tick:300ms"` in a golden manifest, and a step of
  `tuimark_play` and `Play`. A spinner paints glyph `floor(V / 100ms)` at
  virtual time `V`, and `Dump` paints the first. Every document accepts the
  step; `play` without it never calls an `Every` function.
- **`{path}` in a button.** A button's `label`, or its body when it has no
  label, is a template resolved every frame like a `<text>` body: a missing
  path is `B003` at the button, and an auto-width button follows its data.
- **Tools.**
  - `inspect` ends with the element a node was built from and, for a copy or
    a placed element, one `via` line per use or include that placed it,
    innermost first (`--json`: `source` with `file`, `line`, `col`, and `via`).
  - `tuimark ir` keeps the composition as written: an `include` or `use` node
    holds the nodes it placed, in the template's own words, with scoped ids,
    each with a `from` member (`FILE:LINE`).
  - `tuimark catalog` gains `current` (4), `since: 4` on the new tags and
    attributes, and `keys.rejected_since` for the `ctrl+i`, `ctrl+j`, and
    `ctrl+m` rule; `versions` still lists 1, 2, and 3.
  - `run` and `preview --watch` watch every fragment the last load read, at any
    depth, including one that does not exist yet, so creating it reloads.
  - `tuimark lsp` gives a fragment the diagnostics of the documents that
    include it (found among the open documents and the `.tui` files of its
    directory and its parent), republishes them when either side changes,
    checks a fragment no document reads on its own, follows `<include src>`,
    `template=`, and scoped ids for go to definition, and completes template
    ids and scoped ids, by the language of the document.
  - `tuimark mcp` returns stderr as the result when a command exits 2 with
    nothing on stdout (`tuimark_inspect` of a fragment), still not as an
    error.
- **VS Code extension:** highlighting and snippets for `include`, `fragment`,
  `template`, `use`, `checkbox`, `select`, `single`, and `indeterminate`.
- **Go API.** No function, type, or option is added. `Load`, `LoadFS`, and
  `Parse` expand includes and templates, `Play` takes the `tick:` step and
  calls `Every` functions on its virtual clock (up to 10000 per step, not when
  `PlayOptions.NoHandlers` is set), and an `Every` function's `ErrQuit` ends the
  call with `Quit: true`.
- **Docs.** New guides, "Templates and composition" and "Tuimark for frontend
  devs" (a map from the web, with the differences between the current language
  and a pinned `version="3"`), and the layout, widgets, interaction, language,
  testing, and tools guides cover the 0.4b behavior.

### Changed

- **Go API: `tuimark.Diagnostic` gained an unexported field.** It is an alias
  of the internal type, which now keeps what a message cut at an element of a
  copy, whole, so that two diagnostics whose messages read the same once cut
  stay two. Unkeyed struct literals of `Diagnostic` no longer compile (write
  the field names), and `==` also compares the hidden field, so two values
  with equal exported fields can differ. The fields, their JSON, and
  `String()` are unchanged. Before 1.0 this ships as a patch.
- **The examples that opened modals follow the new keys.** `2048`,
  `minesweeper`, `snake`, `sudoku`, and `tetris` repeat the keys that act
  under a modal in a `<keymap when="modal">`, `snake`'s game-over modal reads
  a derived path so `esc` leaves it open, and `tetris`'s has an `on:escape`.
  The kit's `.k-btn:focus` writes `reverse: false`, since its colors are
  already inverted. `schema/dump.v0.2.json` lists `checkbox` and `select` among
  the node tags.
- **The website moves to [tuimark.dev](https://tuimark.dev)** and is rebuilt
  with Next.js and Fumadocs in place of VitePress. Every page keeps its URL
  (the old `NAME.html` addresses redirect), and so does `tuimark.vercel.app`,
  which redirects to the new domain. New: a landing page whose hero is a
  `tuimark play` session of a three-file document, a page that pictures
  every kit component in both themes with its markup (`/kit`), a blog with
  an RSS feed, a static search, Open Graph images for every page, a sitemap,
  and pages for agents: each page as Markdown at its URL plus `.md`,
  `/llms.txt`, and `/llms-full.txt`. Screenshots follow the site's light or
  dark mode, except the documents that paint their own dark palette. The
  agents guide becomes "Use Tuimark with AI agents", with the `tuimark mcp`
  setup for Claude Code and Cursor. `task docs` serves the site, `task
  docs-build` builds it and checks every link, and the generated pages are
  still `task docs-gen` and `task docs-check`.

## [0.4.2] - 2026-10-19

A patch on 0.4a with no change to the language. Tuimark ships on npm as
`@thelacanians/tuimark` (the TypeScript SDK and a `tuimark` command that
downloads the verified release binary), the component kit gains copyable Go
and TypeScript host helpers, `examples/` gains a container-engine dashboard
built on the kit, and the runtime builds for js/wasm for the website's
playground.

### Added

- **The npm package `@thelacanians/tuimark`** (`sdk/ts`): the SDK and the CLI
  in one package. `import { spawnHost } from "@thelacanians/tuimark"` is the
  TypeScript client that was `@tuimark/host` (never published), and its new
  `tuimark` command (`npx @thelacanians/tuimark new my-app`) runs the Go
  binary of the release with the same version, with the same arguments,
  stdio, exit code, and signals (and fd 3 and fd 4 of `tuimark host` when a
  parent starts the installed command directly; parents should prefer
  `spawnHost`, or run the binary themselves). The launcher is plain JS for
  Node 18 or later and Bun, with no dependencies and no `postinstall` script.
  It finds the binary in `TUIMARK_BINARY`, else the cache (`$XDG_CACHE_HOME`
  or `~/.cache`, `~/Library/Caches`, `%LOCALAPPDATA%`, then
  `tuimark/<version>/<os>_<arch>/tuimark`), else it downloads the release's
  `tuimark_<version>_<os>_<arch>[.exe]`, checks its SHA-256 against a table
  built into the package from the release's `checksums.txt` (a mismatch is
  discarded and never run), and caches it; a cached binary is checked against
  the same hash on every run. `TUIMARK_RELEASE_URL` points it at a mirror,
  checked by the mirror's own `checksums.txt` and cached apart.
  `spawnHost` uses the same lookup when `bin` is not given, and
  `resolveBinary` is exported.
- **Raw binaries in the release.** GoReleaser now also uploads
  `tuimark_<version>_<os>_<arch>[.exe]` (Go's own OS and arch names) for the
  six platforms, listed in `checksums.txt`, next to the archives. The Homebrew
  cask still uses only the archives.
- **npm publishing in `release.yml`.** After GoReleaser, the `npm-pack` job
  (no OIDC token) builds the package at the tag's version, embeds the six
  checksums, and checks the release has the six binaries, and the
  `npm-publish` job (GitHub environment `npm`, which allows only `v*` tags)
  publishes it with npm trusted publishing and provenance. Run by hand from a
  release tag ("Use workflow from"), the workflow publishes that tag's
  package.

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
  `@thelacanians/tuimark`. Both run in CI and in `task verify`. For the helpers to be
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
- **A js/wasm build target, for the website's playground.** The core packages
  (the root package and `internal/**`) now build and pass their tests for
  `GOOS=js GOARCH=wasm`; the one blocker was `syscall.SIGWINCH` in
  `internal/host/resize_unix.go`, now retagged `unix` (with `resize_other.go`
  for everything else). `cmd/tuimark-wasm` is the entry that compiles to
  `tuimark.wasm` (about 1.2 MB with brotli): a private, unversioned JS API
  (`open`, `render`, `step`, `validate`, `reload`, `close`) over the same
  runtime, whose frames are the CLI's dumps (tests compare them byte for byte
  for every example and kit component). It is not a command, is not published,
  and promises no compatibility. `task wasm` builds it, `task wasm-test` runs
  the core tests under js with Node, and `task verify` checks its size budget
  and a Node smoke test. The CLI and `Run` are not part of the js target.

### Changed

- `@tuimark/host` is now `@thelacanians/tuimark`, in `tuimark new --ts`
  (whose `package.json` depends on `^<the running version>`), `examples/host-ts`,
  the generated `AGENTS.md`, and the guides. The package version is the Tuimark
  release it runs (it was `1.0.0`), and `sdk-ts-publish.yml` and its `sdk-ts-v*`
  tags are gone.

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

[Unreleased]: https://github.com/abdul-hamid-achik/tuimark/compare/v0.4.3...HEAD
[0.4.3]: https://github.com/abdul-hamid-achik/tuimark/releases/tag/v0.4.3
[0.4.2]: https://github.com/abdul-hamid-achik/tuimark/releases/tag/v0.4.2
[0.4.1]: https://github.com/abdul-hamid-achik/tuimark/releases/tag/v0.4.1
[0.4.0]: https://github.com/abdul-hamid-achik/tuimark/releases/tag/v0.4.0
[0.3.2]: https://github.com/abdul-hamid-achik/tuimark/releases/tag/v0.3.2
[0.3.1]: https://github.com/abdul-hamid-achik/tuimark/releases/tag/v0.3.1
[0.3.0]: https://github.com/abdul-hamid-achik/tuimark/releases/tag/v0.3.0
[0.2.0]: https://github.com/abdul-hamid-achik/tuimark/releases/tag/v0.2.0
