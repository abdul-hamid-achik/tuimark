# Tuimark

You design terminal UIs by editing text documents. You do not write Go layout.

## Allowed edits
- *.tui
- *.tcss
- examples/**/sample.json

## Forbidden
- internal/**
- cmd/** except flag wiring
- Bubble Tea / lipgloss / tea.Cmd / Update()
- inventing tags not in the catalog

## Catalog
tui style keymap bind screen
col row box scroll spacer
text rule
list item input button progress
modal

Spike-only until dump goldens pass:
app col row box text
attrs: id class width height gap pad border

## Loop
tuimark validate FILE
tuimark dump FILE --data sample.json --cols 80  --rows 24 --format json
tuimark dump FILE --data sample.json --cols 120 --rows 24 --format json
Read grid + nodes. Edit markup or CSS. Repeat.
Stop when 80 and 120 match intent and errors is empty.

## Bindings
{path} only inside <text>, title, placeholder.
each="tickets as item" only on <list>.
if="path" or if="!path".
No expressions.

## Actions
on:select="open" is a name. The host implements it.
Built-in actions: focus, quit

## Loop, v0.2a addition

The §22 Loop above predates v0.2a and only shows `validate`/`dump`. For a document with an `<input>`, a `<keymap>`, or any `on:` action, extend it with `play` before wiring a Go handler:

tuimark play FILE --data sample.json --input "STEPS" --styles --format json

Read `events` to confirm the right action fired (and with what `keys`/`value`) for the input you replayed; read `styles` (added by `--styles`) to confirm `:focus`/`:selected`/theme-token rules actually change a cell's look, instead of eyeballing `preview`. Add `--theme light` (or `dark`) to check the other theme without touching the document. Repeat `dump`/`play` at 80 and 120 columns as the §22 Loop already does.

## Reference

`tuimark` in the Loop above is `./bin/tuimark` after `go build -o bin/tuimark ./cmd/tuimark`, or `go run ./cmd/tuimark` without building.

### Attributes per tag

- `bind`: action, keys, to, when
- `box`: border, class, disabled, focusable, gap, height, hidden, id, if, on:click, on:focus, pad, style, title, width
- `button`: border, class, disabled, focusable, gap, height, hidden, id, if, label, on:click, on:focus, pad, style, title, width
- `col`: border, class, disabled, focusable, gap, height, hidden, id, if, on:click, on:focus, pad, style, title, width
- `input`: bind, border, class, disabled, focusable, gap, height, hidden, id, if, on:change, on:click, on:focus, on:submit, pad, placeholder, secret, style, title, width
- `item`: border, class, disabled, gap, height, hidden, id, if, pad, style, title, width
- `keymap`: (no attributes)
- `list`: bind, border, class, disabled, each, focusable, gap, height, hidden, id, if, key, on:click, on:focus, on:select, pad, style, title, width
- `modal`: bind, border, class, disabled, focusable, gap, height, hidden, id, if, on:click, on:close, on:escape, on:focus, on:open, open, pad, style, title, width
- `progress`: bind, border, class, disabled, focusable, gap, height, hidden, id, if, on:click, on:focus, pad, style, title, value, width
- `row`: border, class, disabled, focusable, gap, height, hidden, id, if, on:click, on:focus, pad, style, title, width
- `rule`: axis, border, class, disabled, focusable, gap, height, hidden, id, if, on:click, on:focus, pad, style, title, width
- `screen`: class, focus, id, style, title
- `scroll`: axis, border, class, disabled, focusable, gap, height, hidden, id, if, on:click, on:focus, pad, style, title, width
- `spacer`: border, class, disabled, focusable, gap, height, hidden, id, if, on:click, on:focus, pad, style, title, width
- `style`: src
- `text`: border, class, disabled, focusable, gap, height, hidden, id, if, on:click, on:focus, pad, style, title, width, wrap
- `tui`: theme, version

Inside a list `<item>` nothing takes focus (the list does and navigates its rows): no `input`, `button`, `list`, or `modal` (V001), and no `focusable`, `on:click`, or `on:focus` (V002) anywhere in the row. Handle a row with the list's `on:select`, or a keymap row such as `<bind keys="d" action="delete" when="#rows:focus"/>` for `<list id="rows">` (its event identifies the selected row).

Bindings contain no spaces: `{path}`, `if="path"`, `if="!path"`, and exactly one space on each side of `as` in `each="path as alias"`.

### CSS properties

- `align`: start | center | end | stretch
- `background`: $token | var(--token) | ansi-name | #rgb | #rrggbb | default
- `bold`: true | false
- `border`: none | single | double | rounded | thick
- `border-color`: $token | var(--token) | ansi-name | #rgb | #rrggbb | default
- `color`: $token | var(--token) | ansi-name | #rgb | #rrggbb | default
- `content-align`: start | center | end
- `dim`: true | false
- `display`: flex | none
- `dock`: top | right | bottom | left
- `flex`: number >= 0
- `gap`: 0-4
- `height`: N | N% | Nfr | auto
- `italic`: true | false
- `justify`: start | center | end | space-between
- `layout`: column | row
- `margin`: 1-4 cell values (T R B L)
- `max-height`: N | N% | Nfr | auto
- `max-width`: N | N% | Nfr | auto
- `min-height`: N | N% | Nfr | auto
- `min-width`: N | N% | Nfr | auto
- `overflow`: hidden | scroll
- `padding`: 1-4 cell values (T R B L)
- `reverse`: true | false
- `title-color`: $token | var(--token) | ansi-name | #rgb | #rrggbb | default
- `underline`: true | false
- `visibility`: visible | hidden
- `width`: N | N% | Nfr | auto
- `wrap`: wrap | nowrap | truncate

### Key tokens

Named: enter, esc, tab, backspace, space, up, down, left, right, home, end, pgup, pgdn, shift+tab

Also valid: `ctrl+<a-z>` (`ctrl+i`, `ctrl+j`, and `ctrl+m` arrive as `tab`/`enter` and never match), and any single printable ASCII character except space (write `space`) and `,` (the `keys=` list separator; a token of `,` binds nothing).

### Diagnostic codes

| Code | Pass | When |
|---|---|---|
| V001 | parse | unknown tag |
| V002 | parse | unknown attribute |
| V003 | parse | bad unit / color / token / CSS property |
| V004 | parse | duplicate id |
| V005 | parse | not well-formed XML |
| V006 | parse | `style src` include cycle |
| V007 | parse | control character or ANSI in text (strip + error) |
| V008 | parse | native widget name not registered (v1.1+) |
| V010 | parse | `dock` and `fr` on the same axis |
| V011 | parse | `each` / `if` missing path |
| V012 | parse | list/input/button/modal without `id` |
| V013 | parse | `<text>` has element children |
| V014 | parse | missing `version` on `<tui>` (phase 1+) |
| L001 | layout | `fr` child of non-flex parent |
| L002 | layout | `%` child of `auto` parent on that axis |
| L003 | layout | fixed + min exceeds parent (warning; clip) |
| L004 | layout | modal is not last child of screen |
| L005 | layout | more than one bottom-docked status (warning) |
| L006 | layout | a scroll/list/overflow: scroll viewport can never show part of its content (warning) |
| B001 | bind | `each` path is not an array |
| B002 | bind | `if` path missing |
| B003 | bind | bind path missing (`--strict` upgrades to error) |
| B004 | bind | action not in catalog |
| B005 | bind | keymap `to`/`when` id missing |
| B006 | bind | `list` + `each` without `key` (warning) |

### v0.2a: interaction, styles, and theme

- `tuimark play FILE --data sample.json --input "STEPS" --format json` replays keys, `text:`, `paste:`, `set:`, `focus:`, and `resize:` steps against the document without a TTY (or `--script FILE.ndjson` for steps that need embedded spaces or explicit JSON) and prints the final dump plus every action that fired (`events`), so an agent can check that typing into a search box, or moving a list selection, fires the right `on:` action before wiring a Go handler. `--frames` adds one settled frame per applied step.
- `tuimark dump FILE --styles --format json` (also `play --styles`) adds `theme` and per-row `styles` spans to the dump, so `:focus`, `:selected`, theme tokens, and a `reverse` selection are checked by diffing JSON instead of eyeballing `preview`.
- `--theme dark|light` on `dump`, `validate`, `preview`, and `play` overrides the document's `theme` for that render (`Run`'s equivalent is `TUIMARK_THEME`). `auto` and any other value the flag is explicitly given — including `""` — are usage errors in 0.2a; only leaving `--theme` off keeps the document's theme. `theme="auto"` is 0.2b.
- `preview --color truecolor|256|16|none` picks the ANSI grid's color profile; without it, `preview` falls back to `TUIMARK_COLOR`, then to the same detection `Run()` uses (SPEC §26.3) — read only when stdout is a TTY, resolved once per `preview`.
- `Run()` also reads `TUIMARK_THEME` and `TUIMARK_SYNC` (forces synchronized-output framing off/`0`/on/`1`); `dump`, `validate`, `play`, `ir`, `fmt`, and `test` never read any `TUIMARK_*` variable, so their output never depends on the environment.
- `L006` (new in 0.2a) means a `scroll`/`list`/`overflow: scroll` viewport can never show part of its content: 0 cells on its scroll axis while it has content there, or clipped by an ancestor that does not itself scroll on that axis. Give it a size, or put it in a `<scroll>`.
- `"wide": true` on a dump means some row holds a cluster wider than one column or made of more than one code point (an emoji, CJK, a combining mark): index `cells`, not `grid` by rune, when it is set.
- `tuimark test --update` refuses to rewrite a non-frozen JSON golden that is not a superset of the one on disk (same fields, same values, nothing removed or changed; new members/nodes may only be added when arrays keep their length) and exits 2 naming the first differing JSON Pointer; `--allow-breaking` writes it anyway, for a reviewed, deliberate change.

### Notes

Clarifications of behavior the SPEC states loosely or not at all (the SPEC itself is not edited; see README.md's "Language notes" section for the same points in more detail):

- **`<text>`/`<button>` body normalization.** Each line of the body is trimmed of XML whitespace (space, tab, CR) and leading/trailing blank lines are dropped; interior spaces are kept. Width and paint both see the trimmed content, so `<text> sync</text>` measures and paints as `sync`, not ` sync`. There is no XML-level way to force a leading/trailing ASCII space through the body (CDATA is trimmed the same way and numeric character references are rejected as V005); U+00A0 (NBSP) and other non-ASCII spaces are not XML whitespace and survive as content, if a literal non-breaking space is an acceptable stand-in. Otherwise use `gap`, `padding`, or `margin` on a parent for spacing.
- **Built-in defaults beyond §10.6, and `list`/`input` have no border.** §10.6 only lists defaults for `screen col row box text`; the runtime fills the rest with `scroll`/`list`/`item { layout: column }`, `modal { layout: column; border: single }`, `spacer { flex: 1 }`, `input, button, progress, rule { width: auto; height: auto }`, and `list > item:selected { reverse: true }`. Only `modal` gets a default `border`, so a `border-color` rule on `list`/`input` — a `:focus`/`:selected` variant included — changes nothing unless that element also sets `border: single|double|rounded|thick`. This is why the SPEC §17 inbox fixture's `theme.tcss` (`list:focus`/`input:focus { border-color: $focus; }`) has no visible effect: `dump --styles` and `play --input tab --styles` give byte-identical `styles` regardless of focus. Show focus with `color`/`background`/`bold`/`reverse` instead (as `button:focus { reverse: true; }` does in `examples/agent`/`examples/dashboard`), or give the element its own `border: single` first (as `examples/agent`'s `#composer`/`#transcript` and `examples/dashboard`'s bordered panels do).
- **Modal visibility (`open` vs `bind`).** `open` takes `true`, `false`, a bare path, or `!path` (never `{path}`; truthiness is the §7 table). `bind` on a `modal` is shorthand for the same thing: a path whose truthiness opens it. When a modal has both attributes, `open` wins outright and `bind` is ignored for visibility, even when `open` is a falsy path and `bind`'s path is truthy — prefer setting only one of the two on a given modal.
- **Cross-axis stretch inside a modal.** `col`/`row` default to `1fr` on both the main and the cross axis, and children stretch to fill the cross axis by default (`align: stretch`). A `<row>` of buttons placed inside a `<modal>` (default size 80%×80%, or any column-laid-out ancestor) therefore stretches to the modal's full remaining height, and its buttons stretch with it — most visible when a focused button has `reverse`, which paints its whole rect. Give an action row an explicit `height: 1` or `height: auto` (and consider `justify: center` / `gap` for spacing); `align: start` on the row alone shrinks the buttons but leaves the row itself still filling the height.
- **Focused `list` key handling.** A focused, non-empty `list` consumes `up`, `down`, `home`, `end`, `pgup`, and `pgdn` itself (moving the selection and firing `on:select` on change); these never reach the keymap while the list has focus. `left`/`right` are not consumed and fall through normally. `j`/`k` are never implicit for a list, same as they are not implicit anywhere else (§8.1).
- **List rows are not independently focusable.** Inside `<item>`, nothing takes focus — the list itself is the focusable unit and navigates its rows. Handle a per-row action through the list's `on:select`, or a keymap row scoped with `when="#list:focus"` that reads the event's `keys`/`value` for which row was selected.
- **`Dump` is a side-effect-free snapshot.** It never moves focus, queues events, or mutates scroll/list/input state; `Validate` has the same property. Only the internal frame used by `Run` is "live".
- **`Run` and signals.** On SIGTERM, SIGHUP, or SIGINT, `Run` restores the terminal (cooked mode, main screen, visible cursor) and returns a non-nil error naming the signal, so the process can exit with a failure status instead of exiting clean.
- **Built-in quit.** `ctrl+c` fires the built-in `quit` action when no focused widget or keymap row claims it. With no `On("quit", ...)` handler registered, that action stops `Run`; with a handler registered, `Run` stops only if the handler returns `ErrQuit`. Bind `ctrl+c` in the keymap to route it elsewhere.
- **Escape latency.** A trailing `esc` byte at the end of a read waits briefly (about 25ms) for more bytes before it is delivered as the `esc` key, so it can be told apart from the start of a longer escape/CSI sequence.

## Working on the Tuimark runtime

Everything above (Allowed edits, Forbidden, the Catalog/Loop/Bindings/Actions sections, and the Reference and Notes) is written for UI-authoring tasks: editing `.tui`/`.tcss`/`sample.json` against the runtime as it stands. This section is for the other kind of change: editing the Go runtime itself (`internal/**`, `cmd/tuimark`, or `tuimark.go`).

### Gate

`task verify` runs, in order: the gofmt check (`gofmt -l .` must print nothing — `-l` only lists unformatted files, so empty output is the pass condition), `go vet ./...`, `go test ./...`, the `bin/` build, `./bin/tuimark test`, and every Glyphrun spec. It does not run the race detector, so also run `go test -race ./internal/host/` yourself.

```
task verify
go test -race ./internal/host/
```

The manual equivalent, without `task verify` (useful for a faster loop on one piece):

```
gofmt -l .
go vet ./...
go test ./...
go build -o bin/tuimark ./cmd/tuimark
go build -o bin/inbox ./examples/inbox
go build -o bin/dashboard ./examples/dashboard
go build -o bin/agent ./examples/agent
./bin/tuimark test
glyph run specs/glyphrun/*.yml --format md
go test -race ./internal/host/
```

### Package boundaries

- `cmd/tuimark` — the CLI: flag parsing, output formatting, the golden-manifest runner behind `tuimark test` (including its v0.2a superset check), the `tuimark play` step parser and headless session engine (built only from `internal/host`'s exported `App` methods — `Frame`, `HandleKeyRun`, `HandlePaste`, `Dispatch`, `TakePending`, `Focus`, `SetTheme`), and the theme/`:root` token table for `tuimark ir`. Parsing, cascade, layout, and paint rules live in `internal/**`.
- `tuimark.go` (package `tuimark`, repo root) — the only public surface: the package functions `Load` and `Parse`, `App`'s methods `Bind, Set, On, Catalog, Dump, Validate, Run` (SPEC §18), the `App` type itself, the aliases `Event`, `Handler`, `ActionSpec`, `Dump`, `DumpNode`, `Diagnostic`, and `ErrQuit`. Nothing else is exported from it.
- `internal/ir` — node/scalar/diagnostic/binding-grammar/key-token types, the tag-kind catalog (`Kinds`, `SpikeKinds`), the spike attribute whitelist (`SpikeAttrs`), and the key-token catalog (`NamedKeys`/`ValidKey`).
- `internal/parse` — the XML tokenizer, the IR builder (including the per-tag attribute catalog, `TagAttrs`), the formatter (`fmt`), and IR-as-JSON (`ir`).
- `internal/css` — the TCSS parser, selectors, cascade, tokens/themes, and `@media`.
- `internal/layout` — the integer flex engine.
- `internal/paint` — the cell grid, borders, widget painters, and the ANSI frame diff.
- `internal/dump` — the text and JSON frame dump.
- `internal/host` — the JSON store, inflation, cascade application, focus, events, and the `Run` loop.
- `internal/agentsdoc` — generates this file from the catalogs above.
- Application code — `examples/**`, or any external module — uses only the root `tuimark` package. Inside the repo, `internal/layout` is imported by `internal/paint`, `internal/dump`, and `internal/host`; `internal/paint` is imported by `internal/dump`, `internal/host`, and `cmd/tuimark` (for `preview`'s ANSI frame). SPEC §3 forbids importing `internal/layout`/`internal/paint` from application code outside this repo, not from other packages inside it.

### Hard rules

- The SPEC is the normative specification. It is maintained outside this repository, in the maintainer's notes (`~/notes/projects/tuimark/SPEC.md`): never copy it into the repository, and never change it without the owner's approval. Record interpretations and clarifications as design decisions in the project notes (see "Documentation boundary" below) and, when users need them, in README's "Language notes" and the generated Notes above.
- `testdata/golden/spike/**` is frozen: never regenerate or hand-edit those goldens (`tuimark test --update` skips frozen entries by design). A geometry change there is a runtime regression, not a fixture to update — add a new fixture and manifest entry instead if you need new coverage.
- `examples/inbox/{app.tui,theme.tcss,sample.json}` are verbatim SPEC §17 fixtures: keep them byte-identical to the SPEC's text.
- Closed vocabulary: never invent a tag, attribute, CSS property, unit, or selector. Extending the catalog is a deliberate, catalog-first change (see "Adding things" below), not something a parser special-case should do quietly.
- No Bubble Tea, Lipgloss, `tea.Cmd`/`Update()`, or any other TUI toolkit, as or inside the authoring surface.
- No new dependencies beyond the three ADR-approved ones: `golang.org/x/term`, `golang.org/x/sys`, and, since ADR 0002 (v0.2), `github.com/rivo/uniseg` — used only for grapheme-cluster segmentation; Tuimark's own width function decides column width, ambiguous-width runes fixed at 1.
- Every behavior change ships with a regression test and a design-decision entry in the project notes explaining it (see "Documentation boundary" below).

### Documentation boundary

- `docs/` is reserved for the public Tuimark website (to be built, likely VitePress). Put only publishable content there: landing page, user guides, public reference, static assets. It does not exist yet; do not create it for anything else.
- Design decisions, ADRs, specs in progress, plans, handoffs, review findings, and progress logs live outside this repository, in the maintainer's Obsidian vault at `~/notes/projects/tuimark/` (ADRs under `adrs/NNNN-title.md`, the topic-organized decision record in `design-decisions.md`). Never add them to the repo, and never link public docs to those private paths.
- The SPEC itself lives there too (ADR 0012), not in this repository. The repository keeps only a frozen copy of its §22 block, `internal/agentsdoc/testdata/spec22.md`, for the AGENTS.md prefix test; set `TUIMARK_SPEC` to the SPEC's path to check that copy against the source.
- In the repo, `README.md` is the front page (user-facing clarifications go in its "Language notes") and `AGENTS.md` is generated from `internal/agentsdoc`.

### Adding things

Only when the SPEC itself adds one: SPEC §1 forbids inventing tags, CSS properties, units, or selectors, and SPEC §14 is the single diagnostic taxonomy. These recipes are the mechanics for a change the SPEC has already made, not license to extend the catalog on the runtime side.

- **A diagnostic code.** Implement the check where it belongs (`internal/ir`, `internal/parse`, `internal/css`, `internal/layout`, or `internal/host`), add its row to the `diagCodes` table in `internal/agentsdoc/agentsdoc.go`, add a probe/regression test that triggers it, then regenerate this file.
- **A CSS property.** Add it to `internal/css`'s property table (parsing, `PropertyNames`/`PropertyValues`, and the cascade), add a test in `internal/css`, then regenerate this file so "### CSS properties" above picks it up.
- **A tag attribute.** Add it to `internal/parse`'s `TagAttrs` catalog and whatever in `internal/ir`/`internal/host` needs to read it, add a parser/build test, then regenerate this file so "### Attributes per tag" above picks it up.
- **Regenerating this file.** `go run ./cmd/tuimark agents > AGENTS.md`, then `go test ./internal/agentsdoc`: `TestAGENTSDoesNotDrift` and `TestAGENTSStartsWithSpec22Verbatim` must stay green. Never hand-edit `AGENTS.md`.

### Glyphrun conventions (`specs/glyphrun/`)

- After a deliberate change to a spec's `intent` or `outcomes`, run `glyph spec verify <spec> --stamp` to refresh its `contractHash`; a stale hash otherwise means the contract drifted without review. A spec with no `contractHash` line at all is not checked by this convention (`glyph spec verify` reports `contractHashValid: false` for it but `glyph run` still passes it) — stamp every new spec once its contract is settled.
- Assertions wait for state — `wait: {screen: {contains|notContains: ...}}` or `wait: {process: {exitCode: N}}`, each with a `timeoutMs` — before a snapshot or an outcome is checked. Specs never sleep and hope, but presses whose in-between state doesn't matter (e.g. `press: tab` then `press: down`) are sent back to back with no wait between them. The `preview_watch` fixture script uses `sleep` itself, outside the harness, to space out the file edit it makes.
- `examples/agent`'s specs launch `bin/agent --demo-workspace .glyphrun/ws/<name>` so each spec gets its own throwaway agent workspace; that workspace directory is wiped and reseeded on every run. `.glyphrun/runs/` keeps one timestamped directory per run, pruned to the last `retention.keepRuns`. Both `.glyphrun/ws/` and `.glyphrun/runs/` are gitignored.
- Transient states (a modal mid-open, a progress bar mid-run) are captured with `snapshot: <name>` at the moment they're true and asserted from `$GLYPHRUN_RUN_DIR/snapshots/<name>.txt` in a `script` verifier, not from the live end-of-run screen, which has since moved on.

See `README.md` for the user-facing quickstart, CLI reference, and "Language notes"; the reasoning behind specific interpretation calls is in the design-decision notes described under "Documentation boundary".
