// Package agentsdoc generates AGENTS.md from the runtime's own catalogs
// (tag kinds, attributes, CSS properties, key tokens, diagnostic codes) so
// the document agents read cannot drift from what the code actually
// accepts. `tuimark agents` prints Markdown(); a test in this package
// asserts the repo's AGENTS.md equals it.
package agentsdoc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/abdul-hamid-achik/tuimark/internal/css"
	"github.com/abdul-hamid-achik/tuimark/internal/host"
	"github.com/abdul-hamid-achik/tuimark/internal/ir"
	"github.com/abdul-hamid-achik/tuimark/internal/parse"
)

// diagCode is one row of the SPEC §14 diagnostic taxonomy. The table itself
// has no catalog in code (the pass/when prose isn't data the runtime
// carries at run time), so it is spelled out here once; the code column
// order matches SPEC §14 and is otherwise deterministic.
type diagCode struct {
	Code, Pass, When string
}

var diagCodes = []diagCode{
	{"V001", "parse", "unknown tag; a version=\"2\" tag in a version=\"1\" document (message ends with `(requires version=\"2\")`)"},
	{"V002", "parse", "unknown attribute; a version=\"2\" attribute in a version=\"1\" document (with the same hint)"},
	{"V003", "parse", "bad unit / color / token / CSS property; a `version` other than 1 or 2; a version=\"2\" value, property, pseudo-class, media feature, or built-in action in a version=\"1\" document (with the hint)"},
	{"V004", "parse", "duplicate id"},
	{"V005", "parse", "not well-formed XML"},
	{"V006", "parse", "`style src` include cycle"},
	{"V007", "parse", "control character or ANSI in text (strip + error)"},
	{"V008", "parse", "native widget name not registered (v1.1+)"},
	{"V010", "parse", "`dock` and `fr` on the same axis"},
	{"V011", "parse", "`each` / `if` missing path"},
	{"V012", "parse", "list/input/button/modal without `id`"},
	{"V013", "parse", "`<text>` has element children"},
	{"V014", "parse", "missing `version` on `<tui>` (phase 1+)"},
	{"L001", "layout", "`fr` child of non-flex parent"},
	{"L002", "layout", "`%` child of `auto` parent on that axis"},
	{"L003", "layout", "fixed + min exceeds parent (warning; clip)"},
	{"L004", "layout", "modal is not last child of screen"},
	{"L005", "layout", "more than one bottom-docked status (warning)"},
	{"L006", "layout", "a scroll/list/overflow: scroll viewport can never show part of its content (warning)"},
	{"B001", "bind", "`each` path is not an array"},
	{"B002", "bind", "`if` path missing; also a `class:NAME` guard path"},
	{"B003", "bind", "bind path missing (`--strict` upgrades to error)"},
	{"B004", "bind", "action not in catalog"},
	{"B005", "bind", "keymap `to`/`when` id missing"},
	{"B006", "bind", "`list` + `each` without `key` (warning)"},
	{"V015", "parse", "a `class:NAME` whose NAME is not `[a-z_][a-z0-9_-]*` or whose value is not `path` / `!path` (version=\"2\")"},
}

// kindGroups is the SPEC §22 Catalog block, grouped exactly as it appears
// there (document/layout/content/interactive/overlay, the same grouping as
// §6.1-§6.5 and the §13.1 kind enum): one line per group, five lines
// total. It is kept as its own list, rather than flattening ir.Kinds onto
// one line, so `## 22. AGENTS.md` stays a verbatim copy (SPEC §3) instead
// of just the same 18 tokens in a different shape; TestKindGroupsMatchIRKinds
// checks the two lists cannot drift apart.
var kindGroups = [][]string{
	{"tui", "style", "keymap", "bind", "screen"},
	{"col", "row", "box", "scroll", "spacer"},
	{"text", "rule"},
	{"list", "item", "input", "button", "progress"},
	{"modal"},
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Markdown generates AGENTS.md: SPEC §22 verbatim, followed by generated
// reference sections built from the code's own catalogs.
func Markdown() string {
	var b strings.Builder

	b.WriteString("# Tuimark\n\n")
	b.WriteString("You design terminal UIs by editing text documents. You do not write Go layout.\n\n")

	b.WriteString("## Allowed edits\n")
	b.WriteString("- *.tui\n- *.tcss\n- examples/**/sample.json\n\n")

	b.WriteString("## Forbidden\n")
	b.WriteString("- internal/**\n- cmd/** except flag wiring\n- Bubble Tea / lipgloss / tea.Cmd / Update()\n- inventing tags not in the catalog\n\n")

	b.WriteString("## Catalog\n")
	for _, g := range kindGroups {
		b.WriteString(strings.Join(g, " ") + "\n")
	}
	b.WriteString("\n")
	b.WriteString("Spike-only until dump goldens pass:\n")
	b.WriteString(strings.Join(ir.SpikeKinds, " ") + "\n")
	b.WriteString("attrs: " + strings.Join(ir.SpikeAttrs, " ") + "\n\n")

	b.WriteString("## Loop\n")
	b.WriteString("tuimark validate FILE\n")
	b.WriteString("tuimark dump FILE --data sample.json --cols 80  --rows 24 --format json\n")
	b.WriteString("tuimark dump FILE --data sample.json --cols 120 --rows 24 --format json\n")
	b.WriteString("Read grid + nodes. Edit markup or CSS. Repeat.\n")
	b.WriteString("Stop when 80 and 120 match intent and errors is empty.\n\n")

	b.WriteString("## Bindings\n")
	b.WriteString("{path} only inside <text>, title, placeholder.\n")
	b.WriteString(`each="tickets as item" only on <list>.` + "\n")
	b.WriteString(`if="path" or if="!path".` + "\n")
	b.WriteString("No expressions.\n\n")

	b.WriteString("## Actions\n")
	b.WriteString(`on:select="open" is a name. The host implements it.` + "\n")
	builtins := sortedKeys(host.Builtins)
	b.WriteString("Built-in actions: " + strings.Join(builtins, ", ") + "\n\n")

	writeV02aLoopAddition(&b)

	b.WriteString("## Reference\n\n")
	b.WriteString("`tuimark` in the Loop above is `./bin/tuimark` after `go build -o bin/tuimark ./cmd/tuimark`, or `go run ./cmd/tuimark` without building.\n\n")
	writeAttrsPerTag(&b)
	writeCSSProperties(&b)
	writeKeyTokens(&b)
	writeDiagnosticCodes(&b)
	writeV02aSection(&b)
	writeV02bSection(&b)
	writeNotes(&b)
	writeRuntimeSection(&b)

	return strings.TrimRight(b.String(), "\n") + "\n"
}

// writeV02bSection documents what the 0.2b foundations add for an
// authoring agent (SPEC §22's version="2" section): the version gate, the
// vocabulary it accepts, class:NAME and the dump's classes, the theme
// work, and the tools (`inspect`, `TUIMARK_LOG`). It states plainly what
// the version gate accepts that this build does not lay out yet.
func writeV02bSection(b *strings.Builder) {
	b.WriteString("### version=\"2\" (0.2b)\n\n")
	b.WriteString("- `<tui version=\"2\">` opts a document into the 0.2b vocabulary; everything a version=\"1\" document accepts keeps its meaning there. In a version=\"1\" document each 0.2b tag is V001, each 0.2b attribute V002, and each 0.2b value, property, pseudo-class, media feature, or built-in action V003, with a message ending in `(requires version=\"2\")`. A `.tcss` file has no version of its own: it is checked against the version of the document that loads it. Any `version` other than `1` or `2` is V003, and the document is read as version=\"1\".\n")
	b.WriteString("- Tags only version=\"2\" accepts: " + strings.Join(ir.KindsV2, " ") + ". This build parses them but does not lay them out or paint them yet: each dumps as one empty node.\n")
	b.WriteString("- `class:NAME=\"path\"` (or `!path`) adds the class NAME while the guard is truthy; a missing path is B002 and adds nothing. The JSON dump of a version=\"2\" document lists each node's `classes` (its `class` names, then its truthy guards, no repeats); a version=\"1\" dump never has `classes`. The names `tab-label`, `hint-key`, and `hint-label` are reserved for classes the runtime gives generated nodes.\n")
	b.WriteString("- `theme=\"auto\"`: `Run()` asks the terminal for its background (OSC 11, waiting at most 250 ms before the first frame), falls back to `COLORFGBG`, then to `dark`. Every command and `Dump()`/`Validate()` treat `auto` as `dark`; `validate` checks an `auto` document under both themes, dark first, and reports each diagnostic once.\n")
	b.WriteString("- `@media (theme: dark)` and `@media (theme: light)` (version=\"2\" stylesheets) hold per-theme rules and `:root` palettes. Tokens apply in document order, so put a `@media (theme: light) { :root { … } }` block after the base `:root` it refines. `@media (theme: auto)` is V003.\n")
	b.WriteString("- `Set(\"@theme\", \"dark\"|\"light\"|\"auto\")` (a reserved path, both versions) is the host's theme: it beats the document's theme and `--theme`, and `TUIMARK_THEME` beats it in `Run()`. `play` sets it with `set:@theme=\"light\"` or the script step `{\"theme\":\"light\"}`.\n")
	b.WriteString("- Properties only version=\"2\" stylesheets accept: `grid-columns`, `grid-min-width`, `scrollbar`, `bar`, and the value `layout: grid`. This build computes them (`inspect` shows them) but layout and paint do not apply them yet. Pseudo-classes: `:focus-within` matches the focused node and each ancestor up to the screen, through a modal, and nothing while nothing is focused; `:checked` is accepted but matches nothing until multi-select lands.\n")
	b.WriteString("- Keymap rows of a version=\"2\" document take `label` and `keycap` (literal text, carried in `tuimark ir`) and the hyphenated built-in actions (" + strings.Join(ir.BuiltinActionsV2, ", ") + "), which this build accepts (built in: never B004) but does not run yet: a row naming one never matches, so its key goes on to the next row. `when` still matches the focused node only.\n")
	b.WriteString("- `tuimark inspect FILE --at X,Y --json` (or `--id ID`) answers \"why does this cell look like this?\": the node that cell belongs to, its layout path, its pseudo-classes, every class it could have (with guards, and whether any rule names it), and every property with the rule that won and the rules that lost. It renders exactly as `dump` does with the same flags.\n")
	b.WriteString("- `tuimark ir` prints `\"version\": \"0.2\"` for a version=\"2\" document (validated by `schema/ir.v0.2.json`): the new kinds, `app.mouse`, the keymap's `label`/`keycap`, and `class:NAME` attributes under their full names in `attrs`.\n")
	b.WriteString("- `TUIMARK_LOG=FILE` makes `Run()` write an NDJSON log of the session (mode 0600): start, capabilities and the resolved theme, keys, pastes, actions, frames, resizes, end. While a `secret` input has focus, key and paste records are redacted, and that input's events never carry their value. Only `Run()` reads it.\n\n")
}

// writeNotes documents runtime behavior that the SPEC leaves implicit or
// ambiguous. The SPEC is not edited to add these (it is the user's source of
// truth); the clarifications live here and in README.md's "Language notes"
// section instead, in the runtime's own words rather than SPEC's, so this
// section can never be mistaken for a verbatim SPEC quote.
func writeNotes(b *strings.Builder) {
	b.WriteString("### Notes\n\n")
	b.WriteString("Clarifications of behavior the SPEC states loosely or not at all (the SPEC itself is not edited; see README.md's \"Language notes\" section for the same points in more detail):\n\n")
	b.WriteString("- **`<text>`/`<button>` body normalization.** Each line of the body is trimmed of XML whitespace (space, tab, CR) and leading/trailing blank lines are dropped; interior spaces are kept. Width and paint both see the trimmed content, so `<text> sync</text>` measures and paints as `sync`, not ` sync`. There is no XML-level way to force a leading/trailing ASCII space through the body (CDATA is trimmed the same way and numeric character references are rejected as V005); U+00A0 (NBSP) and other non-ASCII spaces are not XML whitespace and survive as content, if a literal non-breaking space is an acceptable stand-in. Otherwise use `gap`, `padding`, or `margin` on a parent for spacing.\n")
	b.WriteString("- **Built-in defaults beyond §10.6, and `list`/`input` have no border.** §10.6 only lists defaults for `screen col row box text`; the runtime fills the rest with `scroll`/`list`/`item { layout: column }`, `modal { layout: column; border: single }`, `spacer { flex: 1 }`, `input, button, progress, rule { width: auto; height: auto }`, and `list > item:selected { reverse: true }`. Only `modal` gets a default `border`, so a `border-color` rule on `list`/`input` — a `:focus`/`:selected` variant included — changes nothing unless that element also sets `border: single|double|rounded|thick`. This is why the SPEC §17 inbox fixture's `theme.tcss` (`list:focus`/`input:focus { border-color: $focus; }`) has no visible effect: `dump --styles` and `play --input tab --styles` give byte-identical `styles` regardless of focus. Show focus with `color`/`background`/`bold`/`reverse` instead (as `button:focus { reverse: true; }` does in `examples/agent`/`examples/dashboard`), or give the element its own `border: single` first (as `examples/agent`'s `#composer`/`#transcript` and `examples/dashboard`'s bordered panels do).\n")
	b.WriteString("- **Modal visibility (`open` vs `bind`).** `open` takes `true`, `false`, a bare path, or `!path` (never `{path}`; truthiness is the §7 table). `bind` on a `modal` is shorthand for the same thing: a path whose truthiness opens it. When a modal has both attributes, `open` wins outright and `bind` is ignored for visibility, even when `open` is a falsy path and `bind`'s path is truthy — prefer setting only one of the two on a given modal.\n")
	b.WriteString("- **Cross-axis stretch inside a modal.** `col`/`row` default to `1fr` on both the main and the cross axis, and children stretch to fill the cross axis by default (`align: stretch`). A `<row>` of buttons placed inside a `<modal>` (default size 80%×80%, or any column-laid-out ancestor) therefore stretches to the modal's full remaining height, and its buttons stretch with it — most visible when a focused button has `reverse`, which paints its whole rect. Give an action row an explicit `height: 1` or `height: auto` (and consider `justify: center` / `gap` for spacing); `align: start` on the row alone shrinks the buttons but leaves the row itself still filling the height.\n")
	b.WriteString("- **Focused `list` key handling.** A focused, non-empty `list` consumes `up`, `down`, `home`, `end`, `pgup`, and `pgdn` itself (moving the selection and firing `on:select` on change); these never reach the keymap while the list has focus. `left`/`right` are not consumed and fall through normally. `j`/`k` are never implicit for a list, same as they are not implicit anywhere else (§8.1).\n")
	b.WriteString("- **List rows are not independently focusable.** Inside `<item>`, nothing takes focus — the list itself is the focusable unit and navigates its rows. Handle a per-row action through the list's `on:select`, or a keymap row scoped with `when=\"#list:focus\"` that reads the event's `keys`/`value` for which row was selected.\n")
	b.WriteString("- **`Dump` is a side-effect-free snapshot.** It never moves focus, queues events, or mutates scroll/list/input state; `Validate` has the same property. Only the internal frame used by `Run` is \"live\".\n")
	b.WriteString("- **`Run` and signals.** On SIGTERM, SIGHUP, or SIGINT, `Run` restores the terminal (cooked mode, main screen, visible cursor) and returns a non-nil error naming the signal, so the process can exit with a failure status instead of exiting clean.\n")
	b.WriteString("- **Built-in quit.** `ctrl+c` fires the built-in `quit` action when no focused widget or keymap row claims it. With no `On(\"quit\", ...)` handler registered, that action stops `Run`; with a handler registered, `Run` stops only if the handler returns `ErrQuit`. Bind `ctrl+c` in the keymap to route it elsewhere.\n")
	b.WriteString("- **Escape latency.** A trailing `esc` byte at the end of a read waits briefly (about 25ms) for more bytes before it is delivered as the `esc` key, so it can be told apart from the start of a longer escape/CSI sequence.\n\n")
}

// writeRuntimeSection documents the other audience for this file: an agent
// changing the Go runtime itself, not authoring a .tui/.tcss/sample.json UI
// against it. Everything above (Allowed edits, Forbidden, Catalog, Loop,
// Bindings, Actions, the Reference tables, Notes) is written for the
// authoring task; this section is deliberately separate and last so an
// authoring agent can ignore it, and a runtime-changing agent can find it
// without reading past unrelated tables.
func writeRuntimeSection(b *strings.Builder) {
	b.WriteString("## Working on the Tuimark runtime\n\n")
	b.WriteString("Everything above (Allowed edits, Forbidden, the Catalog/Loop/Bindings/Actions sections, and the Reference and Notes) is written for UI-authoring tasks: editing `.tui`/`.tcss`/`sample.json` against the runtime as it stands. This section is for the other kind of change: editing the Go runtime itself (`internal/**`, `cmd/tuimark`, or `tuimark.go`).\n\n")

	b.WriteString("### Gate\n\n")
	b.WriteString("`task verify` runs, in order: the gofmt check (`gofmt -l .` must print nothing — `-l` only lists unformatted files, so empty output is the pass condition), `go vet ./...`, `go test ./...`, the `bin/` build, `./bin/tuimark test`, and every Glyphrun spec. It does not run the race detector, so also run `go test -race ./internal/host/` yourself.\n\n")
	b.WriteString("```\n")
	b.WriteString("task verify\n")
	b.WriteString("go test -race ./internal/host/\n")
	b.WriteString("```\n\n")
	b.WriteString("The manual equivalent, without `task verify` (useful for a faster loop on one piece):\n\n")
	b.WriteString("```\n")
	b.WriteString("gofmt -l .\n")
	b.WriteString("go vet ./...\n")
	b.WriteString("go test ./...\n")
	b.WriteString("go build -o bin/tuimark ./cmd/tuimark\n")
	b.WriteString("go build -o bin/inbox ./examples/inbox\n")
	b.WriteString("go build -o bin/dashboard ./examples/dashboard\n")
	b.WriteString("go build -o bin/agent ./examples/agent\n")
	b.WriteString("./bin/tuimark test\n")
	b.WriteString("glyph run specs/glyphrun/*.yml --format md\n")
	b.WriteString("go test -race ./internal/host/\n")
	b.WriteString("```\n\n")

	b.WriteString("### Package boundaries\n\n")
	b.WriteString("- `cmd/tuimark` — the CLI: flag parsing, output formatting, the golden-manifest runner behind `tuimark test` (including its v0.2a superset check), the `tuimark play` step parser and headless session engine (built only from `internal/host`'s exported `App` methods — `Frame`, `HandleKeyRun`, `HandlePaste`, `Dispatch`, `TakePending`, `Focus`, `SetTheme`), `tuimark inspect` (`Frame` plus `Inspect`), and the theme/`:root` token table for `tuimark ir`. Parsing, cascade, layout, and paint rules live in `internal/**`.\n")
	b.WriteString("- `tuimark.go` (package `tuimark`, repo root) — the only public surface: the package functions `Load` and `Parse`, `App`'s methods `Bind, Set, On, Catalog, Dump, Validate, Run` (SPEC §18), the `App` type itself, the aliases `Event`, `Handler`, `ActionSpec`, `Dump`, `DumpNode`, `Diagnostic`, and `ErrQuit`. Nothing else is exported from it.\n")
	b.WriteString("- `internal/ir` — node/scalar/diagnostic/binding-grammar/key-token types, the tag-kind catalog (`Kinds`, `SpikeKinds`), the spike attribute whitelist (`SpikeAttrs`), and the key-token catalog (`NamedKeys`/`ValidKey`).\n")
	b.WriteString("- `internal/parse` — the XML tokenizer, the IR builder (including the per-tag attribute catalog, `TagAttrs`), the formatter (`fmt`), and IR-as-JSON (`ir`).\n")
	b.WriteString("- `internal/css` — the TCSS parser, selectors, cascade, tokens/themes, and `@media`.\n")
	b.WriteString("- `internal/layout` — the integer flex engine.\n")
	b.WriteString("- `internal/paint` — the cell grid, borders, widget painters, and the ANSI frame diff.\n")
	b.WriteString("- `internal/dump` — the text and JSON frame dump.\n")
	b.WriteString("- `internal/host` — the JSON store, inflation, cascade application, focus, events, and the `Run` loop (with the capability/theme probe and `TUIMARK_LOG`). A frame is built in the SPEC §18 order: inflate, cascade, tab activation, focus, hints, layout, table rows, paint; the stages for the widgets not built yet are in place and do nothing.\n")
	b.WriteString("- `internal/agentsdoc` — generates this file from the catalogs above.\n")
	b.WriteString("- Application code — `examples/**`, or any external module — uses only the root `tuimark` package. Inside the repo, `internal/layout` is imported by `internal/paint`, `internal/dump`, and `internal/host`; `internal/paint` is imported by `internal/dump`, `internal/host`, and `cmd/tuimark` (for `preview`'s ANSI frame). SPEC §3 forbids importing `internal/layout`/`internal/paint` from application code outside this repo, not from other packages inside it.\n\n")

	b.WriteString("### Hard rules\n\n")
	b.WriteString("- The SPEC is the normative specification. It is maintained outside this repository, in the maintainer's notes (`~/notes/projects/tuimark/SPEC.md`): never copy it into the repository, and never change it without the owner's approval. Record interpretations and clarifications as design decisions in the project notes (see \"Documentation boundary\" below) and, when users need them, in README's \"Language notes\" and the generated Notes above.\n")
	b.WriteString("- `testdata/golden/spike/**` is frozen: never regenerate or hand-edit those goldens (`tuimark test --update` skips frozen entries by design). A geometry change there is a runtime regression, not a fixture to update — add a new fixture and manifest entry instead if you need new coverage.\n")
	b.WriteString("- `examples/inbox/{app.tui,theme.tcss,sample.json}` are verbatim SPEC §17 fixtures: keep them byte-identical to the SPEC's text.\n")
	b.WriteString("- Closed vocabulary: never invent a tag, attribute, CSS property, unit, or selector. Extending the catalog is a deliberate, catalog-first change (see \"Adding things\" below), not something a parser special-case should do quietly.\n")
	b.WriteString("- No Bubble Tea, Lipgloss, `tea.Cmd`/`Update()`, or any other TUI toolkit, as or inside the authoring surface.\n")
	b.WriteString("- No new dependencies beyond the three ADR-approved ones: `golang.org/x/term`, `golang.org/x/sys`, and, since ADR 0002 (v0.2), `github.com/rivo/uniseg` — used only for grapheme-cluster segmentation; Tuimark's own width function decides column width, ambiguous-width runes fixed at 1.\n")
	b.WriteString("- Every behavior change ships with a regression test and a design-decision entry in the project notes explaining it (see \"Documentation boundary\" below).\n\n")

	b.WriteString("### Documentation boundary\n\n")
	b.WriteString("- `docs/` is reserved for the public Tuimark website (to be built, likely VitePress). Put only publishable content there: landing page, user guides, public reference, static assets. It does not exist yet; do not create it for anything else.\n")
	b.WriteString("- Design decisions, ADRs, specs in progress, plans, handoffs, review findings, and progress logs live outside this repository, in the maintainer's Obsidian vault at `~/notes/projects/tuimark/` (ADRs under `adrs/NNNN-title.md`, the topic-organized decision record in `design-decisions.md`). Never add them to the repo, and never link public docs to those private paths.\n")
	b.WriteString("- The SPEC itself lives there too (ADR 0012), not in this repository. The repository keeps only a frozen copy of its §22 block, `internal/agentsdoc/testdata/spec22.md`, for the AGENTS.md prefix test; set `TUIMARK_SPEC` to the SPEC's path to check that copy against the source.\n")
	b.WriteString("- In the repo, `README.md` is the front page (user-facing clarifications go in its \"Language notes\") and `AGENTS.md` is generated from `internal/agentsdoc`.\n\n")

	b.WriteString("### Adding things\n\n")
	b.WriteString("Only when the SPEC itself adds one: SPEC §1 forbids inventing tags, CSS properties, units, or selectors, and SPEC §14 is the single diagnostic taxonomy. These recipes are the mechanics for a change the SPEC has already made, not license to extend the catalog on the runtime side.\n\n")
	b.WriteString("- **A diagnostic code.** Implement the check where it belongs (`internal/ir`, `internal/parse`, `internal/css`, `internal/layout`, or `internal/host`), add its row to the `diagCodes` table in `internal/agentsdoc/agentsdoc.go`, add a probe/regression test that triggers it, then regenerate this file.\n")
	b.WriteString("- **A CSS property.** Add it to `internal/css`'s property table (parsing, `PropertyNames`/`PropertyValues`, and the cascade), add a test in `internal/css`, then regenerate this file so \"### CSS properties\" above picks it up.\n")
	b.WriteString("- **A tag attribute.** Add it to `internal/parse`'s `TagAttrs` catalog and whatever in `internal/ir`/`internal/host` needs to read it, add a parser/build test, then regenerate this file so \"### Attributes per tag\" above picks it up.\n")
	b.WriteString("- **Regenerating this file.** `go run ./cmd/tuimark agents > AGENTS.md`, then `go test ./internal/agentsdoc`: `TestAGENTSDoesNotDrift` and `TestAGENTSStartsWithSpec22Verbatim` must stay green. Never hand-edit `AGENTS.md`.\n\n")

	b.WriteString("### Glyphrun conventions (`specs/glyphrun/`)\n\n")
	b.WriteString("- After a deliberate change to a spec's `intent` or `outcomes`, run `glyph spec verify <spec> --stamp` to refresh its `contractHash`; a stale hash otherwise means the contract drifted without review. A spec with no `contractHash` line at all is not checked by this convention (`glyph spec verify` reports `contractHashValid: false` for it but `glyph run` still passes it) — stamp every new spec once its contract is settled.\n")
	b.WriteString("- Assertions wait for state — `wait: {screen: {contains|notContains: ...}}` or `wait: {process: {exitCode: N}}`, each with a `timeoutMs` — before a snapshot or an outcome is checked. Specs never sleep and hope, but presses whose in-between state doesn't matter (e.g. `press: tab` then `press: down`) are sent back to back with no wait between them. The `preview_watch` fixture script uses `sleep` itself, outside the harness, to space out the file edit it makes.\n")
	b.WriteString("- `examples/agent`'s specs launch `bin/agent --demo-workspace .glyphrun/ws/<name>` so each spec gets its own throwaway agent workspace; that workspace directory is wiped and reseeded on every run. `.glyphrun/runs/` keeps one timestamped directory per run, pruned to the last `retention.keepRuns`. Both `.glyphrun/ws/` and `.glyphrun/runs/` are gitignored.\n")
	b.WriteString("- Transient states (a modal mid-open, a progress bar mid-run) are captured with `snapshot: <name>` at the moment they're true and asserted from `$GLYPHRUN_RUN_DIR/snapshots/<name>.txt` in a `script` verifier, not from the live end-of-run screen, which has since moved on.\n\n")

	b.WriteString("See `README.md` for the user-facing quickstart, CLI reference, and \"Language notes\"; the reasoning behind specific interpretation calls is in the design-decision notes described under \"Documentation boundary\".\n")
}

func writeAttrsPerTag(b *strings.Builder) {
	b.WriteString("### Attributes per tag\n\n")
	for _, tag := range sortedKeys(parse.TagAttrs) {
		attrs := sortedKeys(parse.TagAttrs[tag])
		if len(attrs) == 0 {
			fmt.Fprintf(b, "- `%s`: (no attributes)\n", tag)
			continue
		}
		fmt.Fprintf(b, "- `%s`: %s\n", tag, strings.Join(attrs, ", "))
	}
	b.WriteString("\n")
	b.WriteString("Inside a list `<item>` nothing takes focus (the list does and navigates its rows): no `input`, `button`, `list`, or `modal` (V001), and no `focusable`, `on:click`, or `on:focus` (V002) anywhere in the row. Handle a row with the list's `on:select`, or a keymap row such as `<bind keys=\"d\" action=\"delete\" when=\"#rows:focus\"/>` for `<list id=\"rows\">` (its event identifies the selected row).\n\n")
	b.WriteString("Bindings contain no spaces: `{path}`, `if=\"path\"`, `if=\"!path\"`, and exactly one space on each side of `as` in `each=\"path as alias\"`.\n\n")
	b.WriteString("Only in a `<tui version=\"2\">` document (V002 with `(requires version=\"2\")` in a version=\"1\" one): the additions to the tags above, and the full attribute sets of the new tags:\n\n")
	for _, tag := range sortedKeys(parse.TagAttrsV2) {
		fmt.Fprintf(b, "- `%s`: %s\n", tag, strings.Join(sortedKeys(parse.TagAttrsV2[tag]), ", "))
	}
	b.WriteString("- `item` inside a `table`: class only (plus `class:NAME`)\n")
	b.WriteString("- every tag except `tui`, `style`, `keymap`, and `bind`: `class:NAME` (the name is open and lowercase, `[a-z_][a-z0-9_-]*`; the value is `path` or `!path`)\n\n")
}

func writeCSSProperties(b *strings.Builder) {
	b.WriteString("### CSS properties\n\n")
	names := css.PropertyNames()
	sort.Strings(names)
	for _, name := range names {
		v2 := ""
		if css.PropertyV2(name) {
			v2 = " (version=\"2\" stylesheets only)"
		}
		fmt.Fprintf(b, "- `%s`: %s%s\n", name, css.PropertyValues(name), v2)
	}
	b.WriteString("\n")
}

func writeKeyTokens(b *strings.Builder) {
	b.WriteString("### Key tokens\n\n")
	b.WriteString("Named: " + strings.Join(ir.NamedKeys, ", ") + "\n\n")
	b.WriteString("Also valid: `ctrl+<a-z>` (`ctrl+i`, `ctrl+j`, and `ctrl+m` arrive as `tab`/`enter` and never match), and any single printable ASCII character except space (write `space`) and `,` (the `keys=` list separator; a token of `,` binds nothing).\n\n")
}

func writeDiagnosticCodes(b *strings.Builder) {
	b.WriteString("### Diagnostic codes\n\n")
	b.WriteString("| Code | Pass | When |\n")
	b.WriteString("|---|---|---|\n")
	for _, d := range diagCodes {
		fmt.Fprintf(b, "| %s | %s | %s |\n", d.Code, d.Pass, d.When)
	}
	b.WriteString("\n")
}

// writeV02aLoopAddition extends the §22 Loop (verbatim above, and not
// itself editable) with the v0.2a tools an authoring agent needs for a
// document that has input, a keymap, or `on:` actions. The §24 trial run
// against the SPEC v0.2 draft reported this as friction: the generated
// Loop only ever mentioned `validate`/`dump`, so an agent working from
// AGENTS.md alone had no prompt to reach for `play`, `--styles`, or
// `--theme` while iterating.
func writeV02aLoopAddition(b *strings.Builder) {
	b.WriteString("## Loop, v0.2a addition\n\n")
	b.WriteString("The §22 Loop above predates v0.2a and only shows `validate`/`dump`. For a document with an `<input>`, a `<keymap>`, or any `on:` action, extend it with `play` before wiring a Go handler:\n\n")
	b.WriteString("tuimark play FILE --data sample.json --input \"STEPS\" --styles --format json\n\n")
	b.WriteString("Read `events` to confirm the right action fired (and with what `keys`/`value`) for the input you replayed; read `styles` (added by `--styles`) to confirm `:focus`/`:selected`/theme-token rules actually change a cell's look, instead of eyeballing `preview`. Add `--theme light` (or `dark`) to check the other theme without touching the document. Repeat `dump`/`play` at 80 and 120 columns as the §22 Loop already does.\n\n")
}

// writeV02aSection documents the v0.2a interaction/inspection loop: `play`,
// `dump --styles`, `--theme`, and the two flags/fields an agent reading a
// dump must already know about (`wide`, `L006`). It follows the reference
// tables (SPEC §22's own note: "The generated part that `tuimark agents`
// appends after it SHOULD add the v0.2a loop...").
func writeV02aSection(b *strings.Builder) {
	b.WriteString("### v0.2a: interaction, styles, and theme\n\n")
	b.WriteString("- `tuimark play FILE --data sample.json --input \"STEPS\" --format json` replays keys, `text:`, `paste:`, `set:`, `focus:`, and `resize:` steps against the document without a TTY (or `--script FILE.ndjson` for steps that need embedded spaces or explicit JSON) and prints the final dump plus every action that fired (`events`), so an agent can check that typing into a search box, or moving a list selection, fires the right `on:` action before wiring a Go handler. `--frames` adds one settled frame per applied step.\n")
	b.WriteString("- `tuimark dump FILE --styles --format json` (also `play --styles`) adds `theme` and per-row `styles` spans to the dump, so `:focus`, `:selected`, theme tokens, and a `reverse` selection are checked by diffing JSON instead of eyeballing `preview`.\n")
	b.WriteString("- `--theme dark|light` on `dump`, `validate`, `preview`, `play`, and `inspect` overrides the document's `theme` for that render (`Run`'s equivalent is `TUIMARK_THEME`). `auto` and any other value the flag is explicitly given — including `\"\"` — are usage errors (no tool probes the terminal); only leaving `--theme` off keeps the document's theme. A document's `theme=\"auto\"` (version=\"2\") is `dark` in every tool.\n")
	b.WriteString("- `preview --color truecolor|256|16|none` picks the ANSI grid's color profile; without it, `preview` falls back to `TUIMARK_COLOR`, then to the same detection `Run()` uses (SPEC §26.3) — read only when stdout is a TTY, resolved once per `preview`.\n")
	b.WriteString("- `Run()` also reads `TUIMARK_THEME` and `TUIMARK_SYNC` (forces synchronized-output framing off/`0`/on/`1`); `dump`, `validate`, `play`, `ir`, `fmt`, and `test` never read any `TUIMARK_*` variable, so their output never depends on the environment.\n")
	b.WriteString("- `L006` (new in 0.2a) means a `scroll`/`list`/`overflow: scroll` viewport can never show part of its content: 0 cells on its scroll axis while it has content there, or clipped by an ancestor that does not itself scroll on that axis. Give it a size, or put it in a `<scroll>`.\n")
	b.WriteString("- `\"wide\": true` on a dump means some row holds a cluster wider than one column or made of more than one code point (an emoji, CJK, a combining mark): index `cells`, not `grid` by rune, when it is set.\n")
	b.WriteString("- `tuimark test --update` refuses to rewrite a non-frozen JSON golden that is not a superset of the one on disk (same fields, same values, nothing removed or changed; new members/nodes may only be added when arrays keep their length) and exits 2 naming the first differing JSON Pointer; `--allow-breaking` writes it anyway, for a reviewed, deliberate change.\n\n")
}
