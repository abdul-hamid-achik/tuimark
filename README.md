# Tuimark

Tuimark is a *view language for terminals*. UI is authored as text —
structure in `.tui` (XML), style in `.tcss` (a reduced CSS), data as JSON,
behavior as named actions (`on:select="open"`) — and a Go runtime
interprets those files: it lays out an integer cell grid, paints Unicode,
and dumps a machine-readable snapshot.

```
agent edits app.tui + theme.tcss + sample.json
        ↓
tuimark dump --format json --cols 80
        ↓
grid + node geometry
        ↓
agent edits again
```

The Tuimark specification (the SPEC, currently v0.2) is the source of truth
for the language and the runtime. It is maintained by the author outside
this repository; "SPEC §N" references in code and docs point to it. This
file is the map of what's in the repo and how to drive it.

## What Tuimark is not

Rules the runtime holds to (SPEC §1):

- Not a compiler. `.tui` is interpreted at runtime; nothing generates Go.
- Not Bubble Tea, Lipgloss, tview, Textual, Ink, or Ratatui — none of those
  are used as the authoring surface or the public API.
- Not pixels. One unit is one terminal cell; there is no `px`/`em`/`vh`.
- Not an open vocabulary. Tags and CSS properties are a closed catalog;
  an unknown one is a diagnostic (`V001`/`V003`), not a new widget.
- Not a place for logic. No expressions, filters, function calls, or Go in
  markup — only literal values, `{path}` interpolation, `each`, and `if`.

## Requirements

- Go 1.22+, no CGO (`internal/layout` and `internal/paint` are the only
  layout/paint engines involved; there is no C dependency to build).
- [go-task](https://taskfile.dev) — optional, for the `task verify` /
  `task build` / `task glyph` shortcuts in `Taskfile.yml`. Each task is a
  short shell command; without `task`, use the equivalent commands shown
  under Testing (for example the four `go build` lines), since a couple of
  tasks use Taskfile template variables (like `{{.AGENT_DIR}}`) that a
  plain shell won't expand.
- [Glyphrun](https://github.com/abdul-hamid-achik/glyphrun) (the `glyph`
  CLI) — optional, only needed to run the terminal end-to-end specs under
  `specs/glyphrun/`. `go install
  github.com/abdul-hamid-achik/glyphrun/cmd/glyph@latest`, then run
  `glyph docs` for its own documentation.

## Build / install

From a checkout (this module is not published yet — there is no
`go get github.com/abdul-hamid-achik/tuimark` to run against a released
version):

```sh
git clone <this-repo>
cd tuimark
go build -o bin/tuimark ./cmd/tuimark
```

`go install` also works against a local checkout:

```sh
go install ./cmd/tuimark
```

The module is not published yet. To use it from another module, add
`replace github.com/abdul-hamid-achik/tuimark => /path/to/your/checkout`
(or a `go.work`) and import `github.com/abdul-hamid-achik/tuimark`.

## Quickstart

```sh
go build -o bin/tuimark ./cmd/tuimark
./bin/tuimark validate examples/spike/inbox.tui
./bin/tuimark dump examples/spike/inbox.tui --cols 80 --rows 24
./bin/tuimark dump examples/spike/inbox.tui --cols 80 --rows 24 --format json
./bin/tuimark preview examples/spike/inbox.tui --cols 80 --rows 24
```

`dump` prints one frame: the text dump by default, or `--format json` for
the machine-readable shape (`grid` + `nodes` + `errors`). `preview` writes
the same text dump, plus an ANSI-painted grid first when stdout is a TTY.
Bind a JSON file as the data store with `--data`:

```sh
./bin/tuimark dump examples/inbox/app.tui --data examples/inbox/sample.json --cols 80 --rows 24
```

Or use it as a library:

```go
ui, err := tuimark.Load("app.tui")
if err != nil { return err }
_ = ui.Bind("tickets", tickets)
ui.On("open", handleOpen)
ui.On("quit", func(tuimark.Event) error { return tuimark.ErrQuit })
return ui.Run(os.Stdout)
```

## Language tour

A minimal complete app: one list bound to an array, one detail pane bound
to the selected item, and a media query that stacks them on a narrow
terminal. Three files:

`app.tui`:

```xml
<tui version="1" theme="dark">
  <style src="theme.tcss"/>
  <keymap>
    <bind keys="q,ctrl+c" action="quit"/>
  </keymap>
  <screen id="main" focus="#items">
    <col id="root">
      <row id="header">
        <text>{title}</text>
      </row>
      <row id="body">
        <list id="items" each="tickets as t" key="t.id" bind="selected" on:select="open">
          <item>
            <text>{t.title}</text>
          </item>
        </list>
        <box id="detail" if="selected_ticket">
          <text>{selected_ticket.title}</text>
        </box>
        <box id="empty" if="!selected_ticket">
          <text>no selection</text>
        </box>
      </row>
    </col>
  </screen>
</tui>
```

`theme.tcss`:

```css
#header { height: 1; }
#body { layout: row; gap: 1; }
#items { width: 30%; border: single; }
#detail, #empty { width: 1fr; border: single; }

@media (max-cols: 60) {
  #body { layout: column; }
  #items { width: 100%; height: 8; }
}
```

`sample.json`:

```json
{
  "title": "tickets",
  "selected": "t-1",
  "selected_ticket": { "id": "t-1", "title": "fix the login loop" },
  "tickets": [
    { "id": "t-1", "title": "fix the login loop" },
    { "id": "t-2", "title": "cannot deploy" }
  ]
}
```

```sh
./bin/tuimark validate app.tui --data sample.json
./bin/tuimark dump app.tui --data sample.json --cols 80 --rows 24
./bin/tuimark dump app.tui --data sample.json --cols 60 --rows 24
```

`validate` reports `ok`, and the two `dump`s show the same layout side by
side at 80 columns and stacked at 60 (the `@media (max-cols: 60)` block
takes over), proving the loop end to end. This one example touches:

- **Layout units** (SPEC §9): `N` (`height: 1`, `height: 8`), `N%`
  (`width: 30%`), `Nfr` (`width: 1fr`, the implicit `1fr` on `col`/`row`),
  and `auto` (the default for `text`).
- **TCSS selectors/`@media`** (SPEC §10): `#id` and a comma-list of ids as
  selectors, and one feature per `@media` block (`max-cols`).
- **Bindings** (SPEC §7): `{path}` in `<text>` bodies, `each="tickets as
  t"` on `<list>`, and `if="selected_ticket"` / `if="!selected_ticket"`.
- **Keymap and actions** (SPEC §8): a global `<bind>` for `quit`, and
  `on:select="open"` as a named action the host would register with `On`.

## Document versions: `version="1"` and `version="2"`

Every `<tui>` document declares `version="1"` or `version="2"`. A
`version="1"` document keeps the vocabulary and meaning it has always had,
and its dumps stay byte-identical. `version="2"` opts into the 0.2b
vocabulary (SPEC §5.1); this build implements all of it:

- **The version gate.** In a `version="1"` document each 0.2b tag is
  `V001`, each 0.2b attribute `V002`, and each 0.2b value, property,
  pseudo-class, media feature, or built-in action `V003`, with a message
  that ends in `(requires version="2")`. A `.tcss` file is checked against
  the version of the document that loads it, so one stylesheet can serve
  both. Any other `version` is `V003`, and the document is read as
  `version="1"`.
- **`class:NAME="path"`** (or `!path`) on any rendered element adds the
  class `NAME` while the guard is truthy, evaluated every frame in the
  element's scope (a missing path is `B002` and counts as null, as for
  `if`: `class:x="path"` adds nothing, `class:x="!path"` adds `x`).
  `NAME` is lowercase, `[a-z_][a-z0-9_-]*` (`V015` otherwise; an
  uppercase letter is `V002`).
  The JSON dump of a `version="2"` document lists each node's `classes`:
  its `class` names, then its truthy guards, without repeats (a name that
  is already there, from `class` or a guard, is not added again, so
  `class="a b a"` gives `a b`).
- **`theme="auto"`.** `Run()` asks the terminal for its background color
  (OSC 11, waiting at most 250 ms, so the first frame is already in the
  right theme), falls back to `COLORFGBG`, then to `dark`. Every command
  and `Dump()`/`Validate()` treat `auto` as `dark`, and `validate` checks an
  `auto` document under both themes.
- **`@media (theme: dark)` / `@media (theme: light)`** hold per-theme rules
  and `:root` palettes. Tokens apply in document order, so a light block
  goes after the base `:root` it refines:

  ```css
  :root { --brand: #e0443e; }
  @media (theme: light) { :root { --brand: #c8102e; --bg: #fbfaf8; } }
  #title { color: $brand; }
  ```

- **`:focus-within`** matches the focused node and each ancestor up to the
  screen (through a modal); nothing while nothing is focused. **`:checked`**
  matches a list or table row whose key is in the widget's `checked` array.
  Both count +10, like the other pseudo-classes.
- **`<table>` and `<column>`.** A table shows one row per element of an
  array, in columns. Each `<column>`'s body is its cell template, resolved
  on each row; its `title` is the header text. At most one attribute-only
  `<item class="…" class:NAME="…"/>` is the row template.

  ```xml
  <table id="procs" each="procs as p" key="p.pid" bind="cursor_pid"
         checked="marked" mark="▸" placeholder="no results" on:select="moved">
    <item class:blocked="p.protected"/>
    <column id="c-pid" class="num" title="PID" width="7">{p.pid}</column>
    <column id="c-name" title="Name" width="1fr">{p.name}</column>
    <column id="c-cpu" class="num" title="CPU%" class:hot="p.cpu_hot">{p.cpu}</column>
  </table>
  ```

  It is focusable and keeps a cursor like a list: it consumes `up`,
  `down`, `home`, `end`, `pgup`, and `pgdn` while it has rows (a page is
  its body height), writes the cursor row's key, with its JSON type, to
  `bind`, and fires `on:select` with `keys` = `{alias: key}`. The bound
  value selects the first row whose key equals it (`7` and `"7"` differ);
  when none does, the cursor keeps its index. The scroll offset follows
  the cursor whatever moved it (a key, a `Set`, a re-sort, a resize), and
  only the rows in the body viewport are laid out, painted, and dumped:
  `scroll.h` is the row count, the header excluded. Inside a `<scroll>`,
  an unsized table lays out every row. Column widths: `width` in cells;
  `%` of the width left after the mark channel; `fr` shares what is left
  and is then clamped by `min-width`/`max-width`; unset or `auto` is the
  widest of the title and the cell texts on every row. Columns that do not
  fit are clipped; `display: none` on a column (under `@media`, say) hides
  its header and its cells. The header row is shown when a visible column
  has a `title` attribute, and the `column` nodes are its cells in the dump
  (their `text` is the resolved title). Cells never wrap: `wrap: wrap`
  truncates. A rule on a `column` styles its header cell only; body cells
  are `table > item > text` nodes without an id that carry the column's
  `class` plus its `class:NAME` guards evaluated on their row, so
  `.num { content-align: end; }` aligns a whole column and
  `#procs > item > text.hot` colors only the hot cells. Rows and cells
  ignore their own sizes, borders, padding, and margins. `placeholder` is
  painted dim and centered on the first body row when the array is empty
  (it is not a node); an unsized empty table with a `placeholder` keeps
  that one body row (and is at least as wide as the text), so it never
  auto-fits down to its header and hides it. `checked`/`mark` and the
  `move-*`/`check-*` built-ins work as on a list. Diagnostics: `V012`
  without `id`, `V017` without `each` or a `column`, `V016` for misplaced
  children or text, `B006` without `key`, `L003` when the columns' cell
  and `%` widths and min-widths exceed the table, `L006` for a body of 0
  rows that has rows.
  Cell and guard paths are resolved on every row and reported once per
  path, so the diagnostics never depend on the scroll offset; the resolved
  rows are cached until a store path they read is set again, so every
  change to the array costs one pass over its rows whatever the column
  widths. The bound cursor row and the checked keys are looked up in key
  sets kept with the rows and the checked array, so `check-all` is linear
  and a frame with thousands of checked rows costs what one with none
  does.
- **`scrollbar: auto`** on a viewport (`scroll`, `list`, `table`, or an
  `overflow: scroll` box) that scrolls on y, has a border, is at least 3
  rows tall, and has more content than fits paints a thumb over its right
  border: `┃` on `single`/`rounded` borders, `█` on `double`/`thick`. With
  `track = h − 2`, `length = max(1, track·view/content)` and
  `pos = (track − length)·offset/(content − view)` (integer division; for
  a table, `view` is its body height and `content` its rows). It never
  takes layout space. `scrollbar: none` is the default.
- **`bar: eighths`** on a `progress` fills eighths of a cell: `e =
  floor(W·8·v/100 + 0.5)` gives `e div 8` full cells and the partial glyph
  `▏▎▍▌▋▊▉` number `e mod 8`; `bar: block`, the default, is the v1 bar.
- **`<tabs>` and `<tab>`.** A `tabs` paints a one-row strip of labels
  and, right below it, the content of its active `tab`; inactive tabs are
  never inflated or laid out, and widgets inside them keep their state by
  id (list cursors, input text, scroll offsets, a nested `tabs`' tab).

  ```xml
  <tabs id="nav" bind="view" mark="▸" gap="2" on:select="view_changed">
    <tab id="overview" label="1 overview" short="1 ovr"> … </tab>
    <tab id="processes" label="7 processes" short="7 proc" focus="#procs"> … </tab>
  </tabs>
  ```

  With `bind`, the active tab is the visible tab whose id is the bound
  string (`null` or a missing path, `B003`, give the first visible tab;
  any other value gives it too and reports `B010`; fallbacks never write
  the store); without `bind`, the runtime remembers the tab per `tabs` id.
  A user activation (`switch-to`, `move-next`/`move-prev`/`move-first`/
  `move-last` on the `tabs`, `left`/`right` on a focused
  `focusable="true"` `tabs`, `action="focus"` into an inactive tab) writes
  the tab's id to `bind` (or remembers it) and fires `on:select` with
  `value` = the id; a host `Set` activates without `on:select`. A focus
  request into a nested tab that is already its `tabs`' active tab by a
  fallback does not activate it: nothing is written and its `on:select`
  does not fire. Disabled tabs are skipped by those keys and actions.
  Labels come in three tiers so the strip always fits: every `label` when
  they fit (with `gap` between them and a mark slot of `width(mark)` in
  front of each), else every `short` (an empty `short` counts as absent),
  else the active tab's alone as `‹ label ›` (then `‹ short ›`, then
  truncated). Labels are generated `text` nodes (class
  `tab-label`, `key` = the tab id, `:selected` on the active one,
  `:disabled` on a disabled tab), styled with `.tab-label` or
  `tabs > text`; they ignore their own sizes, spacing, borders, and
  `display`. When the active tab changes and focus was on the strip,
  inside the old tab, or nowhere, focus moves to the new tab's `focus=`
  target, else its first focusable node, else the screen's rule; the
  `on:select` comes first, then that node's `on:focus`. When one frame
  changes several `tabs` while focus is nowhere, only the first in
  document order moves it. Focus elsewhere stays. On a screen's first
  frame, `screen@focus` wins, then the first
  `tabs` whose active tab has a `focus=` that can take focus. A `tabs`
  and every `tab` need an id (`V012`), a `tabs` needs a `tab` (`V017`)
  and holds only tabs (`V016`), a `tab` needs a non-empty `label`
  (`V003`), and a `tab focus=` must name a node inside it (`B005`).
- **`<sparkline bind="cpu.hist" min="0" max="100"/>`** draws the last
  values of a numeric array as bars, one column per value, eight levels
  per row, right-aligned in its box (`null` is a gap). Without `min`/`max`
  the smallest and largest shown values are the range; a flat series sits
  at half height. It is as wide as the array by default and one row high
  (give it a `height` for more levels). A value that is not an array, or
  an element that is neither a number nor `null`, is `B009`.
- **`<hints>`** shows key hints generated from the keymap: a `<bind>` with
  a `label` is a hint row, and `keycap` replaces the key text shown (an
  empty `keycap` counts as absent).
  `scope="active"` (the default) shows the rows that one of their keys
  would fire right now, through the same key dispatch `Run()` and `play`
  use: a key the focused input takes, a row shadowed by an earlier row
  with the same key, an `esc` row under a modal with `on:escape`, and a
  built-in whose target does not match are all hidden. `scope="all"` shows
  every hint row (a help screen). Items are laid out along the row (or
  down, with `layout: column`), `gap` apart; an item that does not fit is
  left out with every item after it. Each item is a generated `row`
  (`hints > row`, `gap: 1` by default) holding a `.hint-key` and a
  `.hint-label` text.

  ```xml
  <bind keys="space" action="check-toggle" when="#procs:focus" label="mark"/>
  <bind keys="ctrl+a" action="check-all" keycap="^A" label="all"/>
  …
  <hints id="keys"/>
  ```

- **`layout: grid`** (on `screen`, `box`, `col`, `row`, `scroll`, `item`,
  `tab`, `modal`) places the in-flow children in equal columns, row by
  row. `grid-columns: K` is the column count (1–12); with
  `grid-min-width: M` the count adapts to the width,
  `clamp(floor((W + gap) / (M + gap)), 1, K)`, so a responsive grid needs
  no `@media`. Docked children are pulled out first (in a `scroll` too,
  where they scroll with its content), and `W` is the width they leave.
  Without `grid-min-width`, an auto-width grid is as wide as `K` columns
  of its widest child. `gap` separates columns and rows; a row is as tall
  as its tallest child, and `align` places a shorter one. A child's `width`,
  `min-width`, `max-width`, `flex`, and an `fr`/`%` `height` are ignored
  (`L007`, a warning, when the document wrote them). A grid box is never
  shrinkable; put it in a `<scroll>` (or make it a `scroll`/`overflow:
  scroll` grid) to fit it to the space left and scroll its rows.

  ```css
  #cores { layout: grid; grid-columns: 4; grid-min-width: 22; gap: 1; }
  ```

- **`each` on `col`, `row`, and `box`.** Every child element is the
  template, inflated once per array element, in array order, with the alias
  in scope; the container itself is not repeated, and its own `if`,
  `hidden`, `disabled`, `title`, and guards use the enclosing scope. Each
  generated top-level node dumps the element's `key` (from `key="path"`,
  else the index). A missing or non-array path is `B001`; an empty array
  leaves the container empty, so it matches `:empty`. The template holds
  nothing that takes focus or keeps state: `input`, `button`, `list`,
  `modal`, `table`, `tabs`, and `tab` are `V001`, `focusable`, `on:click`,
  and `on:focus` `V002`, any `id` `V004`; an alias equal to an enclosing
  one is `V011`.

  ```xml
  <row id="tags" each="tags as t" key="t"><text class="tag">{t}</text></row>
  ```

- **Multi-select.** `checked="path"` on a `list` or a `table` (which needs
  `each` and `key`, `V018` otherwise) holds the keys of the checked rows: an array in
  the store that the runtime reads and writes. A missing path counts as
  `[]` (`B003`); any other value is `B008`, shows nothing checked, and is
  never overwritten: the `check-*` rows skip that widget. They skip it too
  when the path is missing under a value that is not an object
  (`checked="sel.marked"` with `sel` a number), which the runtime could
  not write. A list's rows are the elements of its array, even when `if`
  or `hidden` removes their item: the cursor can rest on such a row and
  `check-toggle`/`check-all` include it, so filter the array in the host
  rather than hide items. `mark="✓"` (1 or 2 columns, and only with `checked`)
  reserves `width(mark) + 1` columns at the left of every row, checked or
  not, and paints the mark on the checked ones. `on:change` on a list
  (only with `checked`) fires with the new array as its `value` when
  `check-toggle`, `check-all`, or `check-none` changed it. Checked rows dump
  `"checked": true`, and their text node line ends with ` checked`. No key
  is implicit: bind `space`, `ctrl+a`, … yourself.
- **Built-in actions** (keymap rows only; `V003` in `on:*`): `move-next`,
  `move-prev`, `move-first`, `move-last`, `move-page-down`, and
  `move-page-up` move a list's or a table's cursor (writing its `bind`,
  firing `on:select` when it moved) or a viewport's offset, and
  `move-next`/`move-prev` (wrapping) and `move-first`/`move-last` move a
  `tabs`' active tab over its enabled visible tabs; `check-toggle`,
  `check-all`, and `check-none` change a list's or a table's `checked`
  array; `switch-to` (which needs `to=`) activates a tab (a visible,
  enabled one whose `tabs` is in the frame) or switches screens. The
  target is the node `to=` names, else the focused node. A row whose
  target is missing from the frame, disabled, or incompatible (an empty
  list or table, one without `checked`, a button, `move-page-*` on a
  `tabs`) does not match, and the key goes on to the next
  rows, so `<bind keys="ctrl+a" action="check-all"/>` never steals
  `ctrl+a` from a widget without `checked`; a matching row takes its key
  even when nothing changes. `B007` (an error, reported without data)
  flags a target known to be incompatible from `to=` or from a `when` that
  is exactly `#id:focus`. The built-ins are never `B004`, and `Catalog()`
  lists them with `Builtin: true`; `play` records the events they fire,
  not the actions.
- **`when` over the focus chain.** In a `version="2"` keymap, `when`
  matches when its selector matches the focused node or any ancestor up to
  the screen (a modal's parent is its screen), or, with nothing focused,
  the top open modal or the screen. `when="#pane"` holds while focus is
  anywhere inside `#pane`. The event of a host action still names the
  focused node as its `source`. `version="1"` keeps matching the focused
  node only.
- **One key dispatch** serves `Run()`, `play`, and `<hints>`: `esc` fires
  the top modal's `on:escape`; then the focused, enabled widget consumes
  its own keys (an input its typing and editing keys, a list or a table
  with rows `up`, `down`, `home`, `end`, `pgup`, `pgdn`, a focusable
  `tabs` with an enabled tab `left` and `right`, another viewport the
  arrows and paging keys, any other node with `on:click` except a list or
  a table `enter` and `space`); then the first keymap row whose keys,
  `when`, and built-in target match; then `tab`, `shift+tab`, and
  `ctrl+c`.
- **The mouse.** `<tui version="2" mouse="mouse_enabled">` turns mouse
  input on while its flag (`true`, `false`, a path, or `!path`; default
  `false`, so the terminal keeps its native text selection) is truthy. It
  is evaluated on every frame: a host or a Settings screen switches it
  with `Set`, and `Run()` writes `CSI ?1000h CSI ?1006h` (SGR reports) or
  `CSI ?1000l CSI ?1006l` before the next frame, and turns it off on the
  way out. A missing path is `B002` and counts as null, so
  `mouse="path"` is off and `mouse="!path"` is on. Only the left
  button and the wheel act, on the frame on the screen; while a modal is
  open only events inside the top modal count, and there only the modal
  and its nodes receive them, even when it paints nothing
  (`visibility: hidden`), so the screen nodes that show through never
  do. A click acts on the release, when press and release land on the
  same node: the same layer and path of child indices, where a list or
  table row counts as its row (its index in the array and its key). So
  two rows sharing a template `id` differ, and when a table scrolls or
  the host reorders the array between press and release, so that another
  row lies under the pointer, nothing happens.
  Walking up from the node under the pointer, the first rule that applies
  decides: a disabled or `visibility: hidden` node stops the walk; a tab
  label activates its tab; a list or table row focuses its widget
  (`on:focus`) and moves the cursor there (`on:select`); a `column` with
  `on:click` fires it without moving focus; a node with `on:click` takes
  focus if it can and fires it; any other focusable node takes focus. The
  wheel moves a list or table cursor by one (`on:select`, focus stays), a
  tab strip by one tab (wrapping), or another viewport by one row, with
  or without an `id`. The hit test is the one `tuimark inspect --at X,Y`
  uses.

## CLI reference

```
tuimark dump     FILE [--cols 80] [--rows 24] [--format text|json] [--data FILE.json]
                      [--cells] [--styles] [--strict] [--theme dark|light]
tuimark validate FILE [--json] [--strict] [--catalog FILE] [--data FILE.json] [--theme dark|light]
tuimark preview  FILE [--cols 80] [--rows 24] [--data FILE.json] [--watch] [--theme dark|light]
                      [--color truecolor|256|16|none]
tuimark play     FILE [--cols 80] [--rows 24] [--data FILE.json] [--theme dark|light]
                      [--input STEPS | --script FILE.ndjson]
                      [--format text|json] [--cells] [--styles] [--frames] [--strict]
tuimark fmt      FILE [--write] [--check]
tuimark ir       FILE
tuimark agents
tuimark test     [DIR] [--update] [--allow-breaking]
tuimark inspect  FILE (--at X,Y | --id ID) [--cols 80] [--rows 24] [--data FILE.json]
                      [--theme dark|light] [--strict] [--json]
tuimark version
```

- `dump` renders one frame: the text dump (grid + node geometry +
  diagnostics) by default, or `--format json` for the machine-readable
  shape agents parse (SPEC §13.2). `--data FILE.json` binds the file as the
  JSON store first. `--cells` (JSON only) adds per-cell `{x,y,ch,id}`
  ownership; `--strict` upgrades missing bind paths (`B003`) to errors;
  `--theme`/`--styles` are below. Exits **2** when the dump has an
  error-severity diagnostic.
- `validate` prints diagnostics (or, with `--json`, the full shape
  including the action catalog) without laying out a frame for its own
  sake — it still renders internally at 40/80/120×24 to catch
  layout/bind problems (under both themes, dark first, for a `theme="auto"`
  document without `--theme`). Without `--data`, bind-dependent checks
  (`B001`–`B003`, and the 0.2b `B008`–`B010`) are skipped, since every path
  would otherwise read as "missing". `--catalog FILE` (a JSON list of action names, or
  `{"actions":[...]}`) turns unresolved actions into `B004` warnings.
- `preview` writes the text dump to stdout, and also paints an ANSI frame
  first when stdout is a TTY, in the color profile from `--color`, else
  `TUIMARK_COLOR`, else detection (the same order and detection rule
  `Run()` uses; see "Environment variables" below) — resolved once per
  `preview`, including across `--watch` reloads. `--watch` re-renders
  whenever the document, its stylesheets, or `--data` change (polled
  every 150ms).
- `play` replays input against a document with no TTY and prints the
  resulting dump plus the actions it fired (below).
- `fmt` is the canonical formatter (below).
- `ir` prints the source IR as JSON (SPEC §13.1): a document's parsed shape
  from *before* the stylesheet cascade (SPEC §18's "parse .tui → source IR"
  step, ahead of "apply stylesheet + media") — a node's `style` map holds
  only its own presentation attributes and `style=""`, not rules matched
  from a `<style>` sheet. It exits like every other command (**2** on any
  diagnostic with error severity, including stylesheet-only errors). A
  `version="1"` document (and the spike) gives `"version": "0.1"`, valid
  against `schema/ir.v0.1.json`; a `version="2"` document gives
  `"version": "0.2"`, valid against `schema/ir.v0.2.json`, which adds the
  six new kinds, `app.mouse`, and the keymap's `label`/`keycap`
  (`class:NAME` attributes stay in `attrs` under their full names). Its
  `tokens` are resolved for the document's theme with `auto` as `dark`; a
  `:root` rule inside `@media (theme: …)` counts only for that theme.
- `inspect` explains one node of one frame (below).
- `agents` prints `AGENTS.md`, generated from the runtime's own catalogs so
  it cannot drift; `AGENTS.md` at the repo root must equal this output,
  enforced by `internal/agentsdoc`'s `TestAGENTSDoesNotDrift` (regenerate
  with `go run ./cmd/tuimark agents > AGENTS.md`).
- `test` runs the golden dump comparisons driven by
  `testdata/golden/manifest.json` (below).

### `--theme` and `dump --styles`

`--theme dark|light` (on `dump`, `validate`, `preview`, `play`, and
`inspect`) overrides the document's own `theme` attribute for that one
render — the same override `Run()`'s `TUIMARK_THEME` environment variable
applies for a live session (SPEC §26.4). Any other value the flag is
explicitly given — including `auto` (no tool probes the terminal) and `""`
— is a usage error that exits 1 before anything is rendered; only leaving
`--theme` off entirely keeps the document's own theme, and a document's
`theme="auto"` is `dark` in every tool. The golden manifest's `theme`
field (below) follows the same rule.

The host's theme, `Set("@theme", "dark"|"light"|"auto")` (a reserved path,
in both document versions), beats both the document's theme and
`--theme`; in `Run()`, `TUIMARK_THEME` beats it, and `auto` resolves from
the terminal's background. Whatever `Run()` probed never reaches `Dump()`
or `Validate()`.

### Environment variables (`Run()` and `preview --color`)

`Run()` reads three environment variables before it touches the terminal
(SPEC §26.10); an invalid value is a usage error and `Run` returns before
entering raw mode. Values are exact and case-sensitive; an empty value
counts as unset.

| Variable | Values | Meaning |
|---|---|---|
| `TUIMARK_COLOR` | `truecolor`, `256`, `16`, `none` | forces the color profile (SPEC §26.3), instead of detecting it from `TERM`/`COLORTERM`/`NO_COLOR`/`WT_SESSION` |
| `TUIMARK_THEME` | `dark`, `light` | overrides the document's `theme`, same as `--theme` (SPEC §26.4) |
| `TUIMARK_SYNC` | `0`, `1` | forces synchronized-output framing off or on, instead of probing the terminal (SPEC §26.6) |
| `TUIMARK_LOG` | a file path | writes an NDJSON log of the session (below); a path that cannot be opened, or that names a terminal, makes `Run` return an error before touching the terminal |
| `COLORFGBG` | set by some terminals | the fallback for `theme="auto"` when the terminal does not answer the background query: its last `;` field, 0–6 and 8 dark, 7 and 9–15 light |

**`TUIMARK_LOG=FILE`** (SPEC §26.12) makes `Run()` write one JSON object
per line to `FILE` — created with mode `0600`, or, when it is an existing
regular file (or a symlink to one), truncated and narrowed to `0600` — so
diagnostics never land on the terminal `Run()` owns. Anything else that
is not a terminal (`/dev/null`, a FIFO, a shell's `>(jq …)` pipe) is
written as it is, never truncated and never chmod-ed; a terminal is
refused with an error. A `Run()` that fails before its session starts
(stdin is not a terminal) leaves the file with no records. Each
record starts with `ev` and `t` (milliseconds since `Run()` started):
`start` (version and size), `caps` (whether the probe ran, the color
profile, synchronized output, grapheme mode, and the effective theme with
where it came from), then one record per `key` (by token), `paste` (its
normalized length), `mouse` (kind `press`, `release`, `wheel-up`, or
`wheel-down`, and the 0-based cell; only while the mouse is on), `mode`
(each mouse mode change written), `action` (the §8.2 payload), `frame`
(microseconds and bytes written), and `resize`, and finally `end` with its reason
(`quit`, `eof`, `signal`, or `error`). While an enabled `secret` input has
focus, `key` and `paste` records are just `{"ev", "t", "redacted": true}`,
and that input's events are logged without their `value`. A write error
stops the logging, never the app.

`preview` also reads `TUIMARK_COLOR` — but only when stdout is a TTY (so a
plain, redirected `preview` never depends on the environment), and only as
a fallback: `--color` on the command line wins first. An invalid
`TUIMARK_COLOR` makes `preview` exit 1 before writing anything, the same
way it makes `Run()` return an error. `dump`, `validate`, `play`, `ir`,
`fmt`, `test`, `Dump()`, and `Validate()` never read any `TUIMARK_*`
variable, so their output never depends on the environment.

`dump --styles` (also `play --styles`; JSON output only) adds two fields to
the dump: `"theme"`, the effective theme, and `"styles"`, one array of
spans per row. Each span (`{"x", "w", "fg", "bg", "a"}`) is a maximal run of
adjacent columns sharing a foreground, background, and attribute set; every
row's spans are contiguous and cover every column. Colors are canonical —
`"#rrggbb"` (lowercase), an ANSI name such as `"bright-cyan"`, or
`"default"` for an unset color — and are never downsampled (downsampling is
only a `Run()`/`preview` terminal-output concern). `"a"` lists whichever of
`bold`, `dim`, `italic`, `underline`, `reverse` are on, in that order, and
is omitted when none are. This is how a test asserts `:focus`, `:selected`,
and `reverse`-selection styling by diffing JSON instead of reading
`preview`'s ANSI output by eye.

### `tuimark play`

```sh
tuimark play app.tui --data sample.json \
  --input "text:log tab down" --format json
```

`play` loads a fresh app from `FILE` (`--data` binds the store root, as for
`dump`) and replays a session against it with no TTY: the same primitives
`Run()`'s event loop uses (key/paste decoding and coalescing, event
dispatch, the lifecycle-event settle loop), just never touching a real
terminal. Built-in actions (`quit`, `focus`) run; a host action registered
with `On` does not exist here, so it is only ever *recorded*, never
executed — `play` checks what the document *would* dispatch, not a Go
handler's own logic (simulate a handler's effect with a `set:` step, or
test the Go side with `go test` or Glyphrun).

Steps come from `--input "STEP STEP ..."` (space-separated; a run of spaces
is one separator) or `--script FILE.ndjson` (one JSON object per line, for
steps whose payload needs an embedded space or explicit JSON), never both.
With neither, the session is just the first frame (step 0).

| `--input` step | Meaning |
|---|---|
| `KEY` | one key: a named key (`enter esc tab backspace space up down left right home end pgup pgdn shift+tab`), `ctrl+a`..`ctrl+z`, or any other single printable ASCII character but `,` (write `/`, not `slash` — there are no aliases) |
| `text:STR` | STR's bytes, decoded like real terminal input: a run of printable characters going to a focused input is one edit and one `on:change` (the same coalescing `Run()` does for fast typing or an unbracketed paste); characters that go elsewhere reach the keymap one by one. In a `version="2"` document each key or paste of the step sees the frame the previous one left, as in `Run()`, so `text:ab\r` and `text:ab enter` fire the same events |
| `paste:STR` | one bracketed paste. Normalized (CRLF/CR/LF/TAB → space, ANSI/control bytes stripped) and delivered as one edit/`on:change` when an enabled input has focus; discarded — never reaching the keymap or a built-in — otherwise, so a paste containing `q` can never quit |
| `set:PATH=JSON` | `Set(PATH, json.Unmarshal(JSON))`; `PATH` may be empty (the store root) or the reserved `@focus`/`@screen`/`@theme` (`set:@theme="light"`) |
| `focus:#ID` | `Set("@focus", "ID")` (the `#` is optional); fires no `on:focus`, like `Set` |
| `resize:COLSxROWS` | the terminal size changes, handled like `Run()` handles `SIGWINCH` |
| `click:X,Y`, `wheel-up:X,Y`, `wheel-down:X,Y` | a left click (press and release) or one wheel report at cell (X, Y), 0-based as in the dump, on the current frame (`version="2"` mouse, SPEC §8.5). With the document's `mouse` false the step changes nothing and fires nothing; a cell outside the current grid is a usage error |

The equivalent `--script` line for each: `{"key":"..."}`, `{"text":"..."}`,
`{"paste":"..."}`, `{"set":{"path":"...","value":...}}`, `{"focus":"#..."}`,
`{"resize":[COLS,ROWS]}`. `{"theme":"light"}` (`"dark"`, `"light"`, or
`"auto"`) is `Set("@theme", value)`, like `set:@theme="light"`; since
`play` never probes the terminal, `auto` is `dark`. `{"click":[X,Y]}` is
`click:X,Y`, and `{"wheel":"up","at":[X,Y]}` (or `"down"`) is
`wheel-up:X,Y`/`wheel-down:X,Y` — the one step object with two members,
exactly those two.

A `quit` (a keymap `quit`, or `ctrl+c` with nothing else claiming it) ends
the session early: later steps are never applied. `--format json` prints
the final dump (with `--cells`/`--styles` as for `dump`) plus `"events"` —
every dispatched action, in order, each `{"step", "action", "source",
"keys", "value"}` — always present, `[]` when nothing fired. `--frames`
adds `"frames"`, one `{"step", "input", "events", "dump"}` per applied step
(step 0 included). `--format text` appends an `=== events ===` section
(`STEP ACTION SOURCE KEYS VALUE`, or the single line `none`) after the text
dump, with one such section per step when `--frames` is given.

Exit codes: **0** the final dump is ok; **2** it has an error-severity
diagnostic; **1** a usage or I/O error (an unparseable step, a mouse step
outside the grid, a `Set` error such as a bad path or an unknown screen) — nothing is
printed to stdout in that case, only `tuimark: play: step N (INPUT):
REASON` to stderr.

Exit codes, uniform across every command: **0** ok, **1** usage, I/O, or
crash, **2** validation errors (or, for `fmt --check` / `test`, "not
formatted" / "golden mismatch"). On the CLI, `--cols`/`--rows` must be
between 1 and 1000 (the underlying Go `Dump` itself accepts 0, giving an
empty grid).

### `tuimark fmt`

Canonical formatting, built on the lossless XML tree so it survives
comments and works on any well-formed document regardless of catalog or
binding errors (only not-well-formed XML, `V005`, is refused):

- 2-space indentation, one element per line.
- Attributes reordered to `id`, `class`, the `class:NAME` guards in source
  order, then the rest in source order.
- Elements with no children and no text self-close (`<box id="a"/>`).
- `<text>`/`<button>` bodies (and a `version="2"` `<column>`'s cell
  template) that fit on one line stay inline
  (`<text>hi</text>`); multi-line bodies are indented one level, one output
  line per text line.
- Inline `<style>` bodies are re-indented one level and re-escaped (`&`,
  `<`, `]]>`; CDATA is not preserved); relative indentation within the
  body is kept.
- Comments are preserved in place.
- `&lt; &gt; &amp;` are escaped in text, `&quot; &amp; &lt;` in attribute
  values.
- Output always ends with exactly one newline.

Default prints the formatted document to stdout; `--write` rewrites the
file in place; `--check` prints the file name and exits 2 if it is not
already formatted (no write). Both `examples/spike/inbox.tui` and
`examples/inbox/app.tui` already pass `fmt --check`.

### `tuimark inspect`

```sh
tuimark inspect app.tui --data sample.json --at 12,3 --json
tuimark inspect app.tui --data sample.json --id inbox
```

`inspect` renders one frame exactly as `dump` does with the same flags
(it is deterministic, like `dump`) and explains one node of it: "why does
this cell look like this?" `--at X,Y` (0-based, as in the dump) picks the
node the cell belongs to — the node whose id owns the cell in the `--cells`
map, then down through its anonymous children that contain the cell (a list
row, a text); `--id ID` picks the first laid-out node with that id. Exactly
one of the two is required.

`--json` prints `cols`, `rows`, `ok`, the `node` (as in the dump), its
layout `path` (`/screen#main/col#app/list#inbox/item[3]/text`), the `cell`
(with `--at`), the `pseudo`-classes that match it, one entry per class it
could have (`{"name", "from", "guard", "active", "used"}`: `from` is
`class` or `class:NAME`; `used` tells whether any selector of the loaded
stylesheets or of the built-in sheet names it), and one `style` entry per
TCSS property, in a fixed order: its computed `value`, its `origin`
(`ua`, `attribute`, `author`, `inline`, `inherited`, or `initial`), the
winning `rule` (file, line, column, selector as written — the whole list
for `a, b`, with the specificity of its highest matching selector — or the
attribute as written, or `style=""` — specificity, and `@media`
condition), and the declarations it `overridden`, highest priority first
(a declaration of a selector list counts once, never against itself). The text form (the
default) shows the same, one line per property that is not `initial`.

A cell outside the grid, an id no laid-out node has, or a malformed value
exits 1 with nothing on stdout; a frame with an error-severity diagnostic
exits 2 and still prints the report.

### `tuimark test` and the golden manifest

`testdata/golden/manifest.json` is a JSON array:

```json
[
  {
    "name": "spike",
    "file": "examples/spike/inbox.tui",
    "sizes": ["40x24", "80x24", "120x24"],
    "json": ["80x24"],
    "frozen": true
  }
]
```

`data` is optional. For every size in `sizes`, `tuimark test` compares the
text dump against `DIR/<name>/<size>.txt`; for sizes also listed in
`json`, it compares the JSON dump against `DIR/<name>/<size>.json`. `DIR`
defaults to `testdata/golden`. One `PASS`/`FAIL` line is printed per
golden, with a short diff hint on failure; the command exits 2 on any
mismatch, 1 on an I/O error (a document or golden file that can't be
read). `--update` (re)writes the goldens for every entry whose `frozen` is
not `true` — `spike`'s goldens are frozen and must never be regenerated;
`inbox`, `stacked`, `unicode`, and the SPEC literal fixtures under
`specs/fixtures/` are not: `table` (§6.9.5, at 30x5), `tabs` (§6.10.5, the
strip at 8, 12, 20, 30, and 40 columns), `hints` and `hints-filter`
(§6.12, the second after the `play` step `focus:#filter`), and `grid`
(§11.7, at 20 and 30 columns).
Adding another fixture just needs another
row in the manifest and a golden directory; nothing else in the runner is
fixture-specific.

An entry also takes these optional fields:

| Field | Meaning |
|---|---|
| `theme` | `"dark"` or `"light"`, as `--theme` |
| `input` | `play` steps (`--input`'s grammar, above); the goldens for this entry are then `play` output (an `=== events ===`/`"events"` section too) instead of plain `dump` output |
| `script` | a `play` script path (`--script`'s grammar), exclusive with `input` |
| `styles` | `true` to have the JSON goldens include `"theme"` and `"styles"` |
| `cells` | `true` to have the JSON goldens include `"cells"` (0.2b) |

**`--update`'s superset check.** For a non-frozen entry and a size where a
JSON golden already exists, `--update` refuses to overwrite it unless the
newly rendered JSON is a **superset** of the one on disk: every scalar that
was there keeps its exact value, every array keeps its exact length (element
by element a superset too), and an object may only gain new members. When it
is not — a value changed, an array grew or shrank, a member disappeared —
neither the `.json` nor the paired `.txt` golden is written for that size;
`tuimark test` instead prints `FAIL NAME SIZE (json): update removes or
changes POINTER` (an RFC 6901 JSON Pointer to the first offending location)
and exits 2. This catches an accidental regression at update time instead of
only at the next `git diff`. `--allow-breaking` skips the check for one run,
for a reviewed, deliberate change (a moved node from an auto-fit change, for
example) — record why in the project's decision log alongside the updated
goldens.

## Public Go API

The public surface is deliberately small (package `tuimark`, SPEC §18):

```go
func Load(path string) (*App, error)
func Parse(r io.Reader) (*App, error)

func (a *App) Bind(path string, v any) error
func (a *App) Set(path string, v any) error
func (a *App) On(action string, h Handler)
func (a *App) Catalog() []ActionSpec
func (a *App) Dump(cols, rows int) (*Dump, error)
func (a *App) Validate() []Diagnostic
func (a *App) Run(w io.Writer) error
```

`internal/layout` and `internal/paint` are not importable from outside this
module; the only supported entry points are the functions above.

- **`Load`/`Parse`** never fail on diagnostics in the document — a bad
  `.tui` loads fine; read `Validate()` or `Dump()` to see the problems.
  Only I/O (`Load`) or a read error (`Parse`) returns an error here.
- **`Bind`/`Set`** coerce `v` to JSON-shaped data (`null | bool | number |
  string | array | object`) at the boundary; `NaN`/`±Inf` are rejected, as
  are values with no JSON shape (channels, funcs). `Set` additionally
  schedules a redraw and is safe from any goroutine. Three paths are
  reserved for hosts, not markup: `Set("@focus", "#id")` moves focus,
  `Set("@screen", "id")` switches the active screen — the SPEC says only
  "the host switches by id" without naming an API, and these paths are how
  this runtime exposes it without growing the public surface — and
  `Set("@theme", "dark"|"light"|"auto")` sets the host's theme (any other
  value is an error and changes nothing), for `Run()` and for
  `Dump()`/`Validate()` alike.
- **`Event`** is the payload a handler receives: `Action` (the name),
  `Source` (the firing node's `id`), `Keys` (a snapshot of the selected
  item's `each` aliases, e.g. `{"item": "t-12"}`), and `Value` (the input's
  text for events from a focused input, such as `on:change`/`on:submit` or
  a keymap row; the selected item's id or index for a static, non-`each`
  list; the new `checked` array for a list's `on:change`; nil otherwise).
- **`ErrQuit`**, returned by a handler, stops `Run` cleanly.
- **`Dump`** is a side-effect-free snapshot: it never moves focus, queues
  events, or mutates scroll/list/input state, so dumping the same app at
  several sizes never depends on call order. A negative `cols`/`rows` is
  an error; 0 gives an empty grid. `Validate` has the same
  no-side-effects property.
- **`Run`** drives the app in the terminal (raw mode) until quit.
  `SIGTERM`, `SIGHUP`, and `SIGINT` also stop it (Unix; on other platforms
  only `os.Interrupt`): the terminal is restored first (cooked mode, main
  screen, visible cursor), and `Run` returns a non-nil error naming the
  signal, so the process can exit with a failure status instead of exiting
  clean — even when a handler had already returned `ErrQuit` as the signal
  arrived.

## Examples

- `examples/spike/inbox.tui` — the phase-0 fixture (5 tags: `app col row box
  text`). Its goldens in `testdata/golden/spike/` are **frozen**: they are
  never regenerated, by design — a geometry mismatch there is a runtime
  regression, not a fixture update.

  ```sh
  ./bin/tuimark dump examples/spike/inbox.tui --cols 80 --rows 24
  ```

- `examples/inbox/{app.tui,theme.tcss,sample.json}` — the SPEC §17 fixture
  (keymap, focus, input, list, binding, `each`/`if`, scroll, spacer, media
  queries; see `examples/dashboard` below for `button`/`modal`/`progress`/
  `rule`). These three files are kept byte-identical to the SPEC's fixture text;
  `examples/inbox/main.go` is an interactive host wiring
  `open`/`search`/`quit`.

  ```sh
  go build -o bin/inbox ./examples/inbox
  ./bin/inbox
  ```

- `examples/dashboard` — a multi-screen host (`app.tui`, `theme.tcss`,
  `sample.json`, `actions.json`) exercising screen switching and a
  confirm-style modal; driven by `specs/glyphrun/dashboard_*.yml`.

  ```sh
  go build -o bin/dashboard ./examples/dashboard
  ./bin/dashboard
  ./bin/dashboard -dump 80x24   # print the JSON dump instead of running
  ```

- `examples/agent` — an interactive host around a scripted or Ollama-backed
  chat provider (`agent.tui`, `engine.go`, `provider.go`, `tools.go`, its own
  `README.md`); driven by `specs/glyphrun/agent_*.yml` and has its own Go
  tests (`go test ./examples/agent`). The whole view is `.tui`/`.tcss`; Go
  only supplies data and named actions through the public API above.

  ```sh
  go build -o bin/tuimark ./cmd/tuimark
  go build -o bin/agent ./examples/agent

  # scripted provider (default), interactive, seeds a demo workspace
  # (--demo-workspace deletes and recreates the given directory)
  ./bin/agent --demo-workspace ./demo-workspace

  # headless, machine-readable, one shot
  ./bin/agent --headless --json --demo-workspace ./demo-workspace -p "list"

  # headless, then print the resulting UI's own JSON dump instead
  ./bin/agent --headless --dump 120x30 --demo-workspace ./demo-workspace -p "list"

  # the in-process loopback fixture: exercises the real HTTP streaming
  # client end to end with no model
  ./bin/agent --fixture --demo-workspace ./demo-workspace

  # a real local Ollama model
  ./bin/agent --provider ollama --model qwen3.5:0.8b --demo-workspace ./demo-workspace
  ```

  See `examples/agent/README.md` for the tool sandbox, the two providers,
  and the keys and main flags (`./bin/agent --help` lists all flags).

- `examples/monitor` — the 0.2b acceptance fixture (SPEC §17.4): monitor
  studio's Overview (a panel grid with sparklines and eighths gauges),
  CPU (a core grid in a scroll with a scrollbar), Processes (a table
  whose columns follow the width by `@media`, multi-select with `▸`, a
  filter input, the kill confirmation, and the process detail), and
  Settings, under a nine-label tab strip and a contextual `<hints>` bar,
  in a `version="2"` document with `theme="auto"` and the mouse on.
  `studio.tui` and `studio.tcss` are the SPEC's literal text;
  `sample.json` pins the rows the goldens assert; `main.go` is a host with
  no layout, width, padding, truncation, or column code — a seeded fake
  collector and the named actions (sorting with the arrow as data, the
  filter, kill and force kill, the pinned detail, settings that switch
  the mouse and the theme) through the public API only. Its goldens are
  the `monitor-*` entries of `testdata/golden/manifest.json`, and
  `specs/glyphrun/monitor_*.yml` drive the built binary.

  ```sh
  go build -o bin/monitor ./examples/monitor
  ./bin/monitor
  ./bin/monitor --theme light        # Set("@theme", "light") before Run
  ./bin/monitor --dump 120x30        # print Dump() as JSON instead of running
  ./bin/tuimark play examples/monitor/studio.tui --data examples/monitor/sample.json \
    --cols 120 --input "tab tab 9 tab click:70,1"
  ```

- `examples/stacked/{app.tui,theme.tcss}` — the phase-1 media fixture (SPEC
  §4, §21 test 10): the inbox layout with static text, where `@media
  (max-cols: 80)` stacks `#sidebar` above `#detail`. It only feeds the
  goldens in `testdata/golden/stacked/` (text dumps at 40/80/120 cols, not
  frozen); there is no `main.go` or `sample.json`, so it is not a runnable
  host.

## Testing

- `go test ./...` — the Go test suite: parser/CSS/layout/IR unit tests, the
  SPEC §21 conformance tests (`conformance_test.go` for tests 1-9,
  `phase_test.go` for tests 10-14, and the 0.2b literal fixtures and
  gates through the public API in `conformance_v02b_test.go`,
  `conformance_p2_test.go`, `conformance_p3_test.go`,
  `conformance_p4_test.go`, and the `examples/monitor` acceptance checks in
  `conformance_p5_test.go`), the formatter's
  idempotence/semantics-preservation tests, the IR-vs-schema test, the
  `AGENTS.md`-does-not-drift test, and the CLI's own exit-code/output tests.
  It also runs `tuimark test testdata/golden` itself
  (`cmd/tuimark/main_test.go`'s `TestTestCommandPasses`), so a golden
  mismatch fails `go test ./...` (and `task verify`) same as any other
  test. `go test -race ./...` runs the same suite with the race detector.
- `go test -run xxx -bench BenchmarkFrame ./internal/host/` — SPEC §21 test
  73 (recorded, not a gate): one 200×60 frame of a 1000-row, 7-column
  table, with a warm row cache (only the cursor moved), the same with every
  row checked and the cursor in the last rows, and a cold one (the rows
  resolved again, as after a `Set` of the array).
- `tuimark test` — the golden dump runner described above, runnable on its
  own (`./bin/tuimark test`) for a faster loop while iterating; `--update`
  regenerates the non-frozen goldens for a human to review and commit. The
  spike goldens under `testdata/golden/spike/` are frozen and never
  touched by `--update`.
- `glyph run specs/glyphrun/*.yml` — terminal end-to-end specs driven by
  Glyphrun (the `glyph` CLI) against the real interactive hosts
  (`examples/inbox`, `examples/dashboard`, `examples/monitor`,
  `examples/agent`) and the `tuimark` CLI itself. All five binaries must
  exist under `bin/` before running the specs — build them first:
  ```sh
  go build -o bin/tuimark ./cmd/tuimark
  go build -o bin/inbox ./examples/inbox
  go build -o bin/dashboard ./examples/dashboard
  go build -o bin/monitor ./examples/monitor
  go build -o bin/agent ./examples/agent
  ```
  or run `task build` (see `Taskfile.yml`), which builds all five in one
  step; `task glyph` builds and then runs every spec; `task verify` runs
  fmt, vet, test, build, golden, and glyph in order — the full local gate.
  See `glyphrun.config.yml` and `glyph docs` for the spec format.

## Architecture

The pipeline a document goes through, from source to screen (SPEC §18):

```
parse .tui → source IR
apply stylesheet + media(cols, rows, theme)
inflate bindings + each + if
layout measure / allocate
focus + hit-test
paint buffer
diff previous frame → ANSI   # Run() only
```

Each frame is built in the order SPEC §18 fixes for 0.2b: (1) inflate the
active screen and its open modals (`if`, `hidden`, text, class guards; a
`tabs`' tab nodes but not their content), (2) cascade and drop
`display: none`, (3) activate one tab per `tabs`, (4) resolve focus,
repeating 1–4 while focus or an active tab changes (at most three rounds),
(5) build the items of each `hints`, (6) measure and allocate, (7) generate
the visible rows of each `table`, (8) paint. A table resolves every row in
stage 1 (keys, guards, cell texts, the columns' measures) and gets its rows
in stage 7, once layout knows its body height and offset. Stage 3
inflates only the active tab's content, so inactive tabs cost nothing;
stage 5 runs the key dispatch without side effects to decide which hints
apply; layout then picks the tab label tier and drops the hint items that
do not fit.

Package map:

| Package | Role |
|---|---|
| `internal/ir` | node, scalar, diagnostic, binding-grammar, and key-token types shared by the other packages |
| `internal/parse` | the XML tokenizer (`xml.go`), the IR builder (`build.go`), the canonical formatter (`format.go`), and source-IR-as-JSON (`irjson.go`) |
| `internal/css` | the TCSS parser, selectors, cascade, themes/tokens, and `@media` |
| `internal/layout` | the integer flex engine (SPEC §11): box model, measure, allocate; `layout: grid`, the tab strip, and the hint items (`widgets.go`); the table (`table.go`) |
| `internal/paint` | the cell grid, borders, titles, widgets, and the ANSI frame diff |
| `internal/dump` | the text and JSON frame dump (SPEC §13) |
| `internal/host` | the JSON store, bind/each/if inflation, cascade application, focus, the key dispatch of SPEC §8.6 with the built-in actions (`dispatch.go`, shared by `Run`, `play`, and the hints), tabs (`tabs.go`), hints and sparklines (`hints.go`), tables (`table.go`), event dispatch, and the `Run` loop |
| `internal/agentsdoc` | generates `AGENTS.md` from the catalogs the packages above expose |

`cmd/tuimark` is the CLI built on top of these packages; the root package
`tuimark` (this module's public API, in `tuimark.go`) is a thin wrapper
around `internal/host`.

## Language notes (clarifications of the SPEC)

The SPEC is the source of truth and is not edited here. These are points
where its text is loose or silent about behavior the runtime actually has;
the generated `AGENTS.md` carries the same points, more tersely, in its
`### Notes` section. The full design-decision record (parser edge cases,
TCSS cascade details, layout arithmetic, focus/signal handling, the CLI) is
kept with the maintainer's project notes, outside this repository.

- **`<text>`/`<button>` body normalization** (SPEC §6.3, §12). Each line of
  the body is trimmed of XML whitespace (space, tab, CR) and leading/trailing
  blank lines are dropped; interior spaces are kept, so
  `<text>mail  {folder}</text>` keeps its two-space gap. Width (§11.5) and
  paint (§12) both see the already-trimmed content — `<text> sync</text>`
  measures 4 cells and paints `sync`, not ` sync`. There is no XML-level way
  to force a leading or trailing space into the body (CDATA is trimmed the
  same way, and numeric character references are rejected outright); use
  `gap`, `padding`, or `margin` on a parent for spacing, or a non-XML space
  such as U+00A0 (NBSP) if you need it as literal content.

- **Modal visibility: `open` vs `bind`** (SPEC §6.5, §7). A `<modal>`'s
  `open` attribute takes `true`, `false`, a bare path, or `!path` — never
  `{path}` (that gives `V003`); truthiness follows the §7 table. `bind` on a
  `modal` is shorthand for the same thing: a path whose truthiness opens it.
  Reading `internal/host/frame.go`'s `inflate` for `modal`, the two are not
  merged or OR'd: if the `open` attribute is present at all, it alone decides
  visibility and `bind` is ignored outright, even when `open`'s path is
  falsy and `bind`'s path is truthy. `bind` is only consulted when `open` is
  absent. In practice, set exactly one of the two on a given modal.

- **Cross-axis stretch inside a modal** (SPEC §11.2, §11.3 pass 4, §6.5).
  `col`/`row` default to `1fr` on *both* axes, and by default children
  stretch to fill the cross axis (`align: stretch`). A modal defaults to
  80%×80% of the screen when unsized, so a `<row>` of buttons placed after
  other content inside it — the common confirm-dialog shape — inherits all
  of the modal's leftover height on the main axis, and its buttons then
  stretch to match on the cross axis. This is easy to miss in a dump's text
  grid (a button's label still reads as one line) but is very visible once a
  focused button has `reverse: true`, which paints its *whole* rect: a tall
  reversed block instead of a one-line button. Give the action row an
  explicit `height: 1` or `height: auto` — `align: start` alone shrinks the
  buttons but leaves the row itself still filling the height. For example:

  ```xml
  <tui version="1">
    <style>
      modal { width: 30; height: auto; }
      #actions { height: 1; gap: 2; justify: center; }
    </style>
    <screen id="one" focus="#confirmDelete">
      <text>hi</text>
      <modal id="confirmModal" open="true">
        <text>Delete this item?</text>
        <row id="actions">
          <button id="confirmDelete" label="Delete"/>
          <button id="cancelDelete" label="Cancel"/>
        </row>
      </modal>
    </screen>
  </tui>
  ```

  Saved as `modal.tui`, this validates and dumps cleanly
  (`./bin/tuimark validate modal.tui` reports `ok`; `./bin/tuimark dump
  modal.tui --cols 80 --rows 24` shows `actions` and both buttons at
  height 1, not stretched to the modal's height).

- **Built-in defaults beyond §10.6, and `list`/`input` have no border.**
  §10.6 only lists defaults for `screen col row box text`. The runtime
  fills the rest of the gap with: `scroll`/`list`/`item { layout: column
  }`, `modal { layout: column; border: single }`, `spacer { flex: 1 }`,
  `input, button, progress, rule { width: auto; height: auto }`, and
  `list > item:selected { reverse: true }`. In particular, `list` and
  `input` get no default `border` (only `modal` does), so a `border-color`
  rule on either — including a `:focus`/`:selected` variant — changes
  nothing unless that same element (or selector) also sets `border:
  single|double|rounded|thick` somewhere. This is why the SPEC §17 inbox
  fixture's own `theme.tcss` (`list:focus { border-color: $focus; }`,
  `input:focus { border-color: $focus; }`) has no visible effect at all:
  `dump --styles` and `play --input tab --styles` produce byte-identical
  `styles` regardless of which of `#query`/`#inbox` is focused. `border`
  also needs 2 rows or columns of the element's own size, so a one-row
  `<input>` could not show one anyway; show focus on it with `color`,
  `background`, `bold`, or `reverse` instead — `examples/agent` and
  `examples/dashboard` both do this for `button:focus { reverse: true; }`
  — or give the element an explicit `border: single` first if a colored
  border is what you want, as `examples/agent`'s
  `#composer`/`#transcript` and `examples/dashboard`'s
  `#pipeline`/`#mainpanel`/`#logScroll` do (their `list:focus`/
  `scroll:focus { border-color: $focus; }` works precisely because those
  elements already carry `border: single`). These three files stay
  byte-identical to SPEC §17 on purpose, so this note documents the trap
  rather than fixing the fixture.

- **Focused `list` key handling** (SPEC §8.3). §8.3 says only "`list`
  consumes `up`/`down`", but a focused, non-empty `list` also consumes
  `home`, `end`, `pgup`, and `pgdn` — all six move the selection (firing
  `on:select` when it changes) and never reach the keymap while the list has
  focus. `left`/`right` are not consumed and fall through as usual. As with
  every other widget, `j`/`k` are never implicit for a list; bind them
  yourself in `<keymap>` if you want them.

- **List rows don't take focus individually.** Inside `<item>`, nothing is
  focusable (no `input`/`button`/`list`/`modal`, no `focusable`,
  `on:click`, or `on:focus`) — using them inside an `<item>` is an error
  (`V001` for those tags, `V002` for those attributes). The `<list>` itself
  is the focusable unit and navigates its own rows. Act on a row via the
  list's `on:select`, or a keymap row scoped with `when="#list:focus"` that
  reads the event's `keys`/`value` to see which row fired.
- **`Dump` is a side-effect-free snapshot.** It never moves focus, queues
  events, or mutates scroll/list/input state — same for `Validate`. Only the
  internal frame `Run` drives is "live".
- **`Run` restores the terminal on signals.** On `SIGTERM`, `SIGHUP`, or
  `SIGINT`, `Run` puts the terminal back (cooked mode, main screen, visible
  cursor) and returns a non-nil error naming the signal, so the process can
  exit with a failure status instead of exiting clean.
- **`ctrl+c` is the built-in quit** whenever no keymap binding claims it
  first; bind `ctrl+c` yourself to override or intercept it.
- **Escape latency is ~25ms.** A trailing `esc` byte at the end of a read
  waits briefly for more bytes before it's delivered as the `esc` key, so a
  bare Escape press can be told apart from the start of a longer
  escape/CSI sequence.
- **Keys of one read see each other's changes** (SPEC v0.2b §8.6). In a
  `version="2"` document, `Run()` and `play` dispatch each key or paste of
  one terminal read (or of one `text:` step) on a frame rendered again
  after the previous one changed anything the frame shows: an input's text
  or cursor, a list or table cursor, a viewport offset, a checked array, a
  tab, the screen. `when` selectors, `class:NAME` guards, and `if` see the
  change, so typing `ab` and `enter` in one read fires
  `<bind keys="enter" action="go" when="#q.filled"/>` on an input with
  `class:filled="query"` exactly as two separate reads do. A
  `version="1"` document keeps the 0.1 loop: the frame is rendered again
  between keys only when events fired or focus moved.
- **A `tabs` whose `bind` path cannot be written** (SPEC v0.2b §6.10.2).
  When a value along the path is not an object (`bind="ui.view"` with `ui`
  a number), the active tab cannot change: `switch-to`, `move-*`,
  `left`/`right`, a label click, and the wheel change nothing and fire no
  `on:select`. A keymap row that names the tabs still takes its key.
- **A dropped mouse event also ends a pending press.** A left release
  always ends the press before it; a press or release that the mouse
  ignores (outside the top modal, outside the grid, or while `mouse` is
  false) acts on nothing and leaves no press pending, so a later release
  never completes a click that started under a closed mouse or behind a
  modal. The wheel over a tab strip with no enabled tab stops there and
  changes nothing.
- **A viewport without an `id` keeps the offset the wheel gives it**
  (SPEC v0.2b §8.5, wheel rule 3). Only the wheel can move one: it cannot
  take focus, and `move-*` needs an id. The same holds for a viewport
  inside a list row or an `each` template, where an id repeats. Its
  offset is kept under its element and the key of the row it is in, so
  it stays with that element when siblings before it come and go, and
  with that row when the rows are reordered. It starts at 0.

## Status per phase (SPEC §4)

- **Phase 0 (spike)** — done: `app col row box text`, goldens frozen and
  green.
- **Phase 1 (style)** — done: TCSS parser, tokens/themes, one-feature
  `@media`, `preview --watch`.
- **Phase 2 (interaction)** — done: JSON store, `{path}`/`each`/`if`,
  `list item input button scroll spacer`, focus/tab, named actions.
- **Phase 3 (chrome)** — done: `keymap bind progress rule modal`, screen
  focus, `validate --strict --catalog`, `Run()`.
- **Phase 4 (agent polish)** — done: `fmt`, `ir`, `test`, `agents`, `dump
  --cells`, the `examples/agent` harness, this README.
- **Phase 5 (v0.2a: runtime, terminal, and tools)** — grapheme-cluster width
  (ADR 0002, `github.com/rivo/uniseg`) and the `wide`/`--cells` dump fields;
  auto-fit `scroll`/`list` sizing with the `L006` diagnostic (ADR 0007);
  `Run()`'s capability probe, color profiles, theme selection, bracketed
  paste, and input decoder (SPEC §26); and the headless tools this delivery
  adds — `tuimark play`, `dump --styles`, `--theme` on
  `dump`/`validate`/`preview`/`play`, the `tuimark test --update` superset
  check, and `schema/ir.v0.2.json`/`schema/dump.v0.2.json`. No 0.2a document
  changes version (`tuimark ir` still emits `"version": "0.1"`); `<tui
  version="2">` and the rest of the proposal's new vocabulary are 0.2b
  (ADR 0009).
- **Phase 6 (v0.2b)** — implemented; the SPEC §24 trial with a fresh
  agent (§21 test 74) is still to be run and recorded. Done: the
  `version="2"` gate for the whole 0.2b vocabulary (with the `(requires version="2")` hint in
  `version="1"` documents), IR `0.2` and the 0.2b schemas, `class:NAME` and
  the dump's `classes`, the SPEC §18 frame order, `theme="auto"` (OSC 11,
  `COLORFGBG`), `@media (theme)`, `Set("@theme")`, `tuimark inspect`,
  `TUIMARK_LOG`, the written-out key dispatch with the built-in actions and
  `when` over the focus chain, `each` on containers, multi-select on
  `list` and `table`, `:checked`/`:focus-within`, `table`/`column` (with
  the `table` golden at 30x5, the SPEC §6.9.5 fixture in
  `specs/fixtures/table.{tui,json}`), `scrollbar`, `bar: eighths`,
  `tabs`/`tab` (with focus on activation and focus requests into
  inactive tabs), `sparkline`, `hints` with `label`/`keycap`, and
  `layout: grid` (goldens `tabs`, `hints`, `hints-filter`, and `grid`, the
  SPEC literal fixtures in `specs/fixtures/`), the mouse (SGR reports,
  click on release, the wheel, `play`'s `click:`/`wheel-*:` steps), and
  the acceptance fixture `examples/monitor` with its `monitor-*` goldens
  and `specs/glyphrun/monitor_*.yml` specs.

## License

MIT — see `LICENSE`.
