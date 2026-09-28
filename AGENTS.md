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

Only in a `<tui version="2">` document (V002 with `(requires version="2")` in a version="1" one): the additions to the tags above, and the full attribute sets of the new tags:

- `bind`: keycap, label
- `box`: each, key
- `col`: each, key
- `column`: class, id, on:click, style, title, width
- `hints`: border, class, disabled, focusable, gap, height, hidden, id, if, on:click, on:focus, pad, scope, style, title, width
- `list`: checked, mark, on:change
- `row`: each, key
- `sparkline`: bind, border, class, disabled, focusable, gap, height, hidden, id, if, max, min, on:click, on:focus, pad, style, title, width
- `tab`: border, class, disabled, focus, gap, hidden, id, if, label, pad, short, style, title
- `table`: bind, border, checked, class, disabled, each, focusable, gap, height, hidden, id, if, key, mark, on:change, on:click, on:focus, on:select, pad, placeholder, style, title, width
- `tabs`: bind, border, class, disabled, focusable, gap, height, hidden, id, if, mark, on:click, on:focus, on:select, pad, style, title, width
- `tui`: mouse
- `item` inside a `table`: class only (plus `class:NAME`)
- every tag except `tui`, `style`, `keymap`, and `bind`: `class:NAME` (the name is open and lowercase, `[a-z_][a-z0-9_-]*`; the value is `path` or `!path`)

Only in a `<tui version="3">` document (V002 with `(requires version="3")` in a version="1" or version="2" one; on a 0.2b tag in a version="1" document only that tag's V001 is reported): further additions to the tags above:

- `column`: priority
- `sparkline`: scale

### CSS properties

- `align`: start | center | end | stretch
- `background`: $token | var(--token) | ansi-name | #rgb | #rrggbb | default
- `bar`: block | eighths (version="2" stylesheets only)
- `bold`: true | false
- `border`: none | single | double | rounded | thick
- `border-color`: $token | var(--token) | ansi-name | #rgb | #rrggbb | default
- `color`: $token | var(--token) | ansi-name | #rgb | #rrggbb | default
- `column-gap`: 0-4 (version="3" stylesheets only)
- `content-align`: start | center | end
- `dim`: true | false
- `display`: flex | none
- `dock`: top | right | bottom | left
- `flex`: number >= 0
- `gap`: 0-4
- `grid-columns`: integer 1-12 (version="2" stylesheets only)
- `grid-min-width`: integer >= 1 (version="2" stylesheets only)
- `height`: N | N% | Nfr | auto
- `italic`: true | false
- `justify`: start | center | end | space-between
- `layout`: column | row | grid (version="2")
- `margin`: 1-4 cell values (T R B L)
- `max-height`: N | N% | Nfr | auto
- `max-width`: N | N% | Nfr | auto
- `min-height`: N | N% | Nfr | auto
- `min-width`: N | N% | Nfr | auto
- `overflow`: hidden | scroll
- `padding`: 1-4 cell values (T R B L)
- `reverse`: true | false
- `row-gap`: 0-4 (version="3" stylesheets only)
- `scrollbar`: none | auto (version="2" stylesheets only)
- `title-color`: $token | var(--token) | ansi-name | #rgb | #rrggbb | default
- `underline`: true | false
- `visibility`: visible | hidden
- `width`: N | N% | Nfr | auto
- `wrap`: wrap | nowrap | truncate | truncate-start | truncate-middle (version="3")

### Key tokens

Named: enter, esc, tab, backspace, space, up, down, left, right, home, end, pgup, pgdn, shift+tab

Also valid: `ctrl+<a-z>` (`ctrl+i`, `ctrl+j`, and `ctrl+m` arrive as `tab`/`enter` and never match), and any single printable ASCII character except space (write `space`) and `,` (the `keys=` list separator; a token of `,` binds nothing).

### Diagnostic codes

| Code | Pass | When |
|---|---|---|
| V001 | parse | unknown tag; a version="2" tag in a version="1" document (message ends with `(requires version="2")`); `input`, `button`, `list`, `modal`, `table`, `tabs`, or `tab` inside a container `each` template |
| V002 | parse | unknown attribute; a version="2" attribute in a version="1" document (with the same hint); `focusable`, `on:click`, or `on:focus` inside a container `each` template; `priority` on `column` or `scale` on `sparkline` in a version="1" or version="2" document (with `(requires version="3")`; on a 0.2b tag in a version="1" document only that tag's V001 is reported) |
| V003 | parse | bad unit / color / token / CSS property; a `version` other than 1, 2, or 3; a version="2" value, property, pseudo-class, media feature, or built-in action in a version="1" document (with the hint); `switch-to` without `to`; a hyphenated built-in in an `on:*` attribute; a `tab` without a non-empty `label`; `{path}` in `label`, `short`, `mark`, or `keycap`; a `sparkline` without `bind`; a bad `scope`, `min`, or `max`; `row-gap`/`column-gap`, or the `wrap` values `truncate-start`/`truncate-middle`, in a version="1" or version="2" document (with `(requires version="3")`; an invalid `wrap` value lists the two only in a version="3" document); a `priority` that is not a non-negative integer; a `scale` that is not a plain identifier |
| V004 | parse | duplicate id; any `id` inside a container `each` template (version="2") |
| V005 | parse | not well-formed XML |
| V006 | parse | `style src` include cycle |
| V007 | parse | control character or ANSI in text (strip + error) |
| V008 | parse | native widget name not registered (v1.1+) |
| V010 | parse | `dock` and `fr` on the same axis |
| V011 | parse | `each` / `if` missing path; an `each` alias equal to an enclosing one (version="2") |
| V012 | parse | list/input/button/modal without `id`; a `table`, `tabs`, or `tab` without `id` (version="2") |
| V013 | parse | `<text>` has element children |
| V014 | parse | missing `version` on `<tui>` (phase 1+) |
| L001 | layout | `fr` child of non-flex parent |
| L002 | layout | `%` child of `auto` parent on that axis |
| L003 | layout | fixed + min exceeds parent (warning; clip); also a table whose visible columns' cell and `%` widths and `auto`/`fr` min-widths, plus the gaps, exceed its width; in a version="3" document, evaluated on the columns left once `priority` has hidden some |
| L004 | layout | modal is not last child of screen |
| L005 | layout | more than one bottom-docked status (warning) |
| L006 | layout | a scroll/list/table/overflow: scroll viewport can never show part of its content (warning); for a table, a body viewport of 0 rows while it has rows |
| B001 | bind | `each` path is missing or not an array (a `list`, or a `col`/`row`/`box`/`table` in version="2") |
| B002 | bind | `if` path missing; also a `class:NAME` guard path |
| B003 | bind | bind path missing (`--strict` upgrades to error); also a table's `bind`, `placeholder`, row `key`, and column cell templates, reported once per template and path whatever rows are visible; a `tabs` or `sparkline` `bind` |
| B004 | bind | action not in catalog |
| B005 | bind | keymap `to`/`when` id missing; a `tab focus=` that names no node, a node inside a list item, or a node outside that tab (version="2") |
| B006 | bind | `list` + `each` without `key` (warning); also `table` + `each` without `key` |
| V015 | parse | a `class:NAME` whose NAME is not `[a-z_][a-z0-9_-]*` or whose value is not `path` / `!path` (version="2") |
| V016 | parse | 0.2b structure (version="2"): a `column` whose parent is not a `table`; a `table` child other than `column` and one `item`, or a second `item`; text directly in a `table`; a table `item` with children or text; an element inside a `column`; a `tab` whose parent is not a `tabs`; a `tabs` child other than `tab`, or text directly in a `tabs`; children or text in a `sparkline` or a `hints` |
| V017 | parse | a `table` without `each` or without a `column` child; a `tabs` without a `tab` child (version="2") |
| V018 | parse | `checked` on a `list`/`table` without both `each` and `key` (or on a list of static items); a `mark` that is not 1 or 2 columns wide; `mark` without `checked`; `on:change` on a `list` without `checked` (version="2") |
| B007 | bind | a built-in action whose target is known without data to be incompatible (from `to=`, or a `when` that is exactly `#id:focus`); static, error (version="2") |
| B008 | bind | the value at a `checked` path is present and not an array (error; version="2") |
| B009 | bind | the value at a `sparkline` `bind` is present and not an array, or holds an element that is neither a number nor `null` (warning; version="2") |
| B010 | bind | the value at a `tabs` `bind` is present, not `null`, and not the id of a visible tab; the first visible tab is active and the store is not written (warning; version="2") |
| L007 | layout | in a `layout: grid` container, a child with a document-written `width`, `min-width`, `max-width`, or `flex` (ignored), or an `fr` or `%` `height` (treated as `auto`) (warning; version="2") |
| L008 | layout | in a `version="3"` document, a painted `text` (not `visibility: hidden`, not a table cell/tab label/hint item) that loses columns (a document-unwritten `nowrap` line wider than its content box) or lines (more lines than its content box has rows, with `overflow: hidden` not document-written) without saying so (warning; version="3") |
| L009 | layout | in a `version="3"` document, a container that cuts a child, in flow or docked, on an axis it does not itself scroll, when `overflow: hidden` is not document-written on it; not for a `list`, a `table`, or a `hints`, for a node `L003` already reports, or for a child `L006` already reports (warning; version="3") |

### v0.2a: interaction, styles, and theme

- `tuimark play FILE --data sample.json --input "STEPS" --format json` replays keys, `text:`, `paste:`, `set:`, `focus:`, and `resize:` steps against the document without a TTY (or `--script FILE.ndjson` for steps that need embedded spaces or explicit JSON) and prints the final dump plus every action that fired (`events`), so an agent can check that typing into a search box, or moving a list selection, fires the right `on:` action before wiring a Go handler. `--frames` adds one settled frame per applied step.
- `tuimark dump FILE --styles --format json` (also `play --styles`) adds `theme` and per-row `styles` spans to the dump, so `:focus`, `:selected`, theme tokens, and a `reverse` selection are checked by diffing JSON instead of eyeballing `preview`.
- `--theme dark|light` on `dump`, `validate`, `preview`, `play`, and `inspect` overrides the document's `theme` for that render (`Run`'s equivalent is `TUIMARK_THEME`). `auto` and any other value the flag is explicitly given — including `""` — are usage errors (no tool probes the terminal); only leaving `--theme` off keeps the document's theme. A document's `theme="auto"` (version="2") is `dark` in every tool.
- `preview --color truecolor|256|16|none` picks the ANSI grid's color profile; without it, `preview` falls back to `TUIMARK_COLOR`, then to the same detection `Run()` uses (SPEC §26.3) — read only when stdout is a TTY, resolved once per `preview`.
- `Run()` also reads `TUIMARK_THEME` and `TUIMARK_SYNC` (forces synchronized-output framing off/`0`/on/`1`); `dump`, `validate`, `play`, `ir`, `fmt`, and `test` never read any `TUIMARK_*` variable, so their output never depends on the environment.
- `L006` (new in 0.2a) means a `scroll`/`list`/`overflow: scroll` viewport can never show part of its content: 0 cells on its scroll axis while it has content there, or clipped by an ancestor that does not itself scroll on that axis. Give it a size, or put it in a `<scroll>`.
- `"wide": true` on a dump means some row holds a cluster wider than one column or made of more than one code point (an emoji, CJK, a combining mark): index `cells`, not `grid` by rune, when it is set.
- `tuimark test --update` refuses to rewrite a non-frozen JSON golden that is not a superset of the one on disk (same fields, same values, nothing removed or changed; new members/nodes may only be added when arrays keep their length) and exits 2 naming the first differing JSON Pointer; `--allow-breaking` writes it anyway, for a reviewed, deliberate change.

### version="2" (0.2b)

- `<tui version="2">` opts a document into the 0.2b vocabulary; everything a version="1" document accepts keeps its meaning there. In a version="1" document each 0.2b tag is V001, each 0.2b attribute V002, and each 0.2b value, property, pseudo-class, media feature, or built-in action V003, with a message ending in `(requires version="2")`. A `.tcss` file has no version of its own: it is checked against the version of the document that loads it. Any `version` other than `1`, `2`, or `3` is V003, and the document is read as version="1" (`version="3"` is accepted too, see "version=\"3\"" below).
- Tags only version="2" accepts: table column tabs tab sparkline hints. This build lays out and paints all six (below).
- `class:NAME="path"` (or `!path`) adds the class NAME while the guard is truthy; a missing path is B002 and counts as null, as for `if` (`class:x="path"` adds nothing, `class:x="!path"` adds x). The JSON dump of a version="2" document lists each node's `classes` (its `class` names, then its truthy guards, each name once: `class="a b a"` gives `a b`); a version="1" dump never has `classes`. The names `tab-label`, `hint-key`, and `hint-label` are reserved for classes the runtime gives generated nodes.
- `theme="auto"`: `Run()` asks the terminal for its background (OSC 11, waiting at most 250 ms before the first frame), falls back to `COLORFGBG`, then to `dark`. Every command and `Dump()`/`Validate()` treat `auto` as `dark`; `validate` checks an `auto` document under both themes, dark first, and reports each diagnostic once.
- `@media (theme: dark)` and `@media (theme: light)` (version="2" stylesheets) hold per-theme rules and `:root` palettes. Tokens apply in document order, so put a `@media (theme: light) { :root { … } }` block after the base `:root` it refines. `@media (theme: auto)` is V003.
- `Set("@theme", "dark"|"light"|"auto")` (a reserved path, both versions) is the host's theme: it beats the document's theme and `--theme`, and `TUIMARK_THEME` beats it in `Run()`. `play` sets it with `set:@theme="light"` or the script step `{"theme":"light"}`.
- Properties only version="2" stylesheets accept: `grid-columns`, `grid-min-width`, `scrollbar`, `bar`, and the value `layout: grid`. This build applies all of them (below). Pseudo-classes (+10 each, like the others): `:focus-within` matches the focused node and each ancestor up to the screen, through a modal, and nothing while nothing is focused; `:checked` matches a list or table row whose key is in the widget's `checked` array.
- `<table id="t" each="rows as r" key="r.id" bind="cur">` holds `<column title="Name" width="1fr">{r.name}</column>` elements (the body is the cell template, resolved per row) and at most one attribute-only `<item class="…" class:NAME="…"/>` row template. It shows one row per array element; it is focusable and keeps a cursor like a list (it consumes `up`, `down`, `home`, `end`, `pgup`, `pgdn` while it has rows, paging by its body height; `bind` receives the cursor row's key with its JSON type; `on:select` fires with `keys` = `{alias: key}`), its scroll offset follows the cursor, and it lays out, paints, and dumps only the rows in its body viewport (`scroll.h` is the row count, the header excluded). The header row (the `column` nodes, whose dump `text` is the resolved `title`) is shown when a visible column has a `title` attribute. Column widths: `width` in cells; `%` of the width left after the mark channel; `fr` shares what is left, then `min-width`/`max-width` clamp it; unset or `auto` is the widest of the title and the cell texts over every row. Columns that do not fit are clipped (no horizontal scroll), and a column with `display: none` (say under `@media`) hides its header and its cells. Cells never wrap (`wrap: wrap` truncates). A rule on a `column` styles its header cell only; the body cells (`table > item > text`, no id) carry the column's `class` and its `class:NAME` guards evaluated on their row, so style them with a class or `#t > item > text`. The rows are `table > item` (the built-in sheet reverses the cursor row) and ignore their own sizes, borders, and spacing. `placeholder` is painted dim and centered on the first body row when the array is empty. `checked`/`mark` and the `move-*` and `check-*` built-ins work on a table as on a list. A table needs `id` (V012), `each` and a `column` (V017), and without `key` reports B006; misplaced children are V016.
- `<tabs id="nav" bind="view" mark="▸" gap="2" on:select="changed">` holds `<tab id="overview" label="1 overview" short="1 ovr" focus="#x">…</tab>` elements: a one-row strip of labels, and below it the content of the active tab only (inactive tabs are not inflated or laid out; their widgets keep their state by id). With `bind`, the active tab is the visible tab whose id is the bound string (`null` or a missing path, B003, give the first visible tab; any other value too, with B010; fallbacks never write the store); without `bind` the runtime remembers it. `switch-to` a tab, `move-next`/`move-prev` (wrapping over the enabled tabs)/`move-first`/`move-last` on the tabs, `left`/`right` on a focused `focusable="true"` tabs, and `action="focus"` into an inactive tab (not a nested tab already active by a fallback) activate a tab: they write its id to `bind` (or remember it) and fire `on:select` with `value` = the id; a host `Set` activates without `on:select`. Labels are generated `text` nodes (class `tab-label`, `key` = the tab id, `:selected` for the active tab, `:disabled` for a disabled one, which the keys skip); they show every `label` when they fit, else every `short` (an empty `short` counts as absent), else only the active tab's as `‹ label ›` (then `‹ short ›`, then truncated), each after a mark slot of `width(mark)` columns, `gap` apart; their own sizes, spacing, borders, and `display` are ignored. When the active tab changes and focus was on the strip, inside the old tab, or nowhere, focus moves to the new tab's `focus=`, else its first focusable node, else the screen's rule (`on:select` first, then that node's `on:focus`); when one frame changes several tabs while focus is nowhere, only the first in document order moves it; on a screen's first frame the first tabs whose active tab has a focusable `focus=` supplies the focus when `screen@focus` does not. `tabs` and `tab` need an `id` (V012), a tabs a `tab` (V017) and only tabs (V016), a tab a non-empty `label` (V003), and `tab focus=` a node inside it (B005).
- `<sparkline bind="hist" min="0" max="100"/>` draws the last values of a numeric array as bars, one column per value, eight levels per row, right-aligned (`null` is a gap; without `min`/`max` the shown values give the range; a flat series is half high). It is as wide as the array and one row high by default. A non-array or a non-number element is B009; a missing path B003.
- `<hints/>` shows the keymap rows with a `label` (their `keycap` when it is not empty, else the key, then the label): `scope="active"` (the default) only those one of whose keys would fire now through the key dispatch (a key the focused input takes, a row shadowed by an earlier row with the same key, an `esc` row under a modal with `on:escape`, a built-in whose target does not match: hidden), `scope="all"` every one. Items are laid out along the row (or down with `layout: column`), `gap` apart (`hints { gap: 2 }`), and the first that does not fit is dropped with every item after it. Each item is a generated `row` (`hints > row`, gap 1) with a `.hint-key` and a `.hint-label` text.
- `layout: grid` on `screen`, `box`, `col`, `row`, `scroll`, `item`, `tab`, or `modal` places the in-flow children in equal columns, row by row: `grid-columns: K` columns, or, with `grid-min-width: M`, `clamp(floor((W + gap) / (M + gap)), 1, K)` for the content width W the docked children leave (they are pulled out first, in a `scroll` too, where they scroll with its content), so no `@media` is needed; without `grid-min-width`, an auto-width grid is as wide as K columns of its widest child. `gap` separates columns and rows; a row is as tall as its tallest child; `align` places shorter ones. A child's `width`, `min-width`, `max-width`, `flex`, and `fr`/`%` `height` are ignored (L007 when the document wrote them). A grid box never shrinks; a `<scroll>` (or `overflow: scroll`) grid fits the space left and scrolls its rows.
- `scrollbar: auto` on a viewport (a `scroll`, `list`, `table`, or `overflow: scroll` box) that scrolls on y, has a border, is at least 3 rows tall, and has more content than fits paints a thumb over its right border (`┃`, or `█` on `double` and `thick` borders), sized and placed by the offset; it never takes layout space. `scrollbar: none` is the default.
- `bar: eighths` on a `progress` fills in eighths of a cell (`▏▎▍▌▋▊▉` for the partial one) instead of whole cells (`bar: block`, the default).
- `each="path as alias"` (and an optional `key="path"`) also goes on `col`, `row`, and `box`: every child element is the template, inflated once per array element, in order, with the alias in scope (`{path}`, `if`, `hidden`, `disabled`, `class:NAME`, and a `progress` `bind` resolve per element). The container's own `if`, `hidden`, `disabled`, `title`, and guards use the enclosing scope, and it is not repeated. Each generated top-level node dumps the element's `key` (the index without `key=`). A missing or non-array path is B001; an empty array leaves the container empty (`:empty`). The template holds nothing that takes focus or keeps state: `input`, `button`, `list`, `modal`, `table`, `tabs`, `tab` are V001, `focusable`/`on:click`/`on:focus` V002, and any `id` V004. An alias equal to an enclosing one is V011.
- Multi-select: `checked="path"` on a `list` or a `table` (with `each` and `key`, else V018) holds the keys of the checked rows, an array the runtime reads and writes (a missing path counts as `[]`, B003; a non-array is B008, shown as nothing checked and never overwritten, and the `check-*` rows skip that widget; so does a missing path under a value that is not an object, such as `sel.marked` with `sel` a number, which the runtime could not write). A list's rows are the elements of its array even when `if`/`hidden` removes their item: the cursor can rest on such a row and `check-toggle`/`check-all` include it, so filter the array in the host rather than hide items. `mark="✓"` (1 or 2 columns, needs `checked`: V018) reserves `width(mark) + 1` columns at the left of every row and paints the mark on checked rows. `on:change` on a `list` (needs `checked`: V018) fires with `value` = the new array when `check-toggle`, `check-all`, or `check-none` changed it. Checked rows dump `"checked": true` and end their text node line with ` checked`. Bind the keys yourself (`space`, `ctrl+a`, … are not implicit).
- Keymap rows of a version="2" document take `label` and `keycap` (literal text, carried in `tuimark ir`; a row with a `label` is a key hint that `<hints>` shows) and the hyphenated built-in actions (move-next, move-prev, move-first, move-last, move-page-down, move-page-up, check-toggle, check-all, check-none, switch-to), which are built in (never B004; `Catalog()` lists them with `builtin: true`) and allowed only in keymap rows (in `on:*` they are V003). A built-in acts on the node named by `to=`, else on the focused node: `move-next`/`move-prev`/`move-first`/`move-last`/`move-page-down`/`move-page-up` move a list's or a table's cursor (writing `bind`, firing `on:select` when it moved), a viewport's offset, or a tabs' active tab (not `move-page-*`); `check-toggle`/`check-all`/`check-none` change a list's or a table's `checked` array; `switch-to` (needs `to=`, V003 otherwise) activates a visible, enabled tab whose tabs is in the frame, or switches to a screen. A row whose target is missing from the frame, disabled, or incompatible (an empty list or table, one without `checked`, a button, …) does not match, and the key goes on to the next rows; a matching row takes its key even when nothing changes (`move-next` on the last row). B007 (an error, reported without data) names a target known to be incompatible from `to=` or from a `when` that is exactly `#id:focus`. `play` records the events they fire, not the actions.
- `when` in a version="2" keymap matches over the focus chain: the focused node and each ancestor up to the screen (a modal's parent is its screen), or, with nothing focused, the top open modal and then the screen. So `when="#pane"` holds while focus is anywhere inside `#pane`, and `when="#kill"` while it is inside the modal `kill`. A host action fired by such a row still names the focused node as its `source`. In a version="1" document `when` matches the focused node only, and nothing while nothing is focused.
- One key dispatch serves `Run()`, `play`, and `<hints>`: (1) `esc` fires the top modal's `on:escape`; (2) a focused, enabled widget consumes its keys (an input: printable keys, `space`, `backspace`, `ctrl+h`, `ctrl+u`, `left`, `right`, `home`, `end`, `ctrl+a`, `ctrl+e`, and `enter` with `on:submit`; a list or a table with rows: `up`, `down`, `home`, `end`, `pgup`, `pgdn`; a focusable tabs with an enabled tab: `left`, `right`; another viewport: the arrows, `pgup`, `pgdn`, `home`, `end`; any other node with `on:click`, except a list or a table: `enter` and `space`); (3) the first keymap row, in document order, whose keys, `when`, and built-in target match; (4) `tab`/`shift+tab` cycle focus and `ctrl+c` quits.
- `<tui version="2" mouse="path">` turns the mouse on while its flag (`true`, `false`, a path, or `!path`; the default is `false`; a missing path is B002 and counts as null, so `mouse="path"` is off and `mouse="!path"` is on) is truthy; it is evaluated on every frame, so a host (or a Settings screen) switches it with `Set`, and `Run()` turns the terminal's mouse reporting (SGR) on and off with it. Only the left button and the wheel act, against the frame on the screen; while a modal is open only events inside the top modal count, and there only the modal and its nodes receive them, even when it paints nothing (`visibility: hidden`). A click acts on release over the same node (for a list or table row, the same row by its index and key: when the table scrolls or the host reorders the array between press and release, so that another row lies under the pointer, nothing happens): walking up from the node under the pointer, a disabled or `visibility: hidden` node stops it; a tab label activates its tab; a row of a list or a table focuses the widget (`on:focus`) and moves the cursor there (`on:select`); a `column` with `on:click` fires it without moving focus; a node with `on:click` takes focus if it can, then fires it; any other focusable node takes focus. The wheel moves a list's or a table's cursor by one (`on:select`, focus stays), a tab strip by one tab (with wrap), or a viewport by one row (one without an `id` too). `play` replays the mouse with `click:X,Y`, `wheel-up:X,Y`, and `wheel-down:X,Y` (cells 0-based as in the dump; a cell outside the grid is a usage error; with the mouse off the step does nothing) or the script steps `{"click":[X,Y]}` and `{"wheel":"down","at":[X,Y]}`; `tuimark inspect --at X,Y` names the same node a click there starts from.
- `tuimark inspect FILE --at X,Y --json` (or `--id ID`) answers "why does this cell look like this?": the node that cell belongs to, its layout path, its pseudo-classes, every class it could have (with guards, and whether any rule names it), and every property with the rule that won and the rules that lost. It renders exactly as `dump` does with the same flags.
- `tuimark ir` prints `"version": "0.2"` for a version="2" document (validated by `schema/ir.v0.2.json`): the new kinds, `app.mouse`, the keymap's `label`/`keycap`, and `class:NAME` attributes under their full names in `attrs`.
- `TUIMARK_LOG=FILE` makes `Run()` write an NDJSON log of the session (a regular file is created or truncated with mode 0600; `/dev/null` or a pipe is written as it is; a terminal is refused): start, capabilities and the resolved theme, keys, pastes, mouse events and mouse mode changes, actions, frames, resizes, end. While a `secret` input has focus, key and paste records are redacted, and that input's events never carry their value. Only `Run()` reads it.

### version="3" (0.3a)

- `<tui version="3">` accepts everything a `version="2"` document does, with the same meaning (every version="2" rule above holds for it too, `when` over the focus chain and the dump's `classes`/`checked` included); a `.tcss` it loads is checked as for version="2". 0.3a gives it no vocabulary of its own: only the layout warnings `L008`/`L009` and the dump's `clipped` are new, and only for a version="3" document. A version="2" document is a valid version="3" document, so upgrading is one attribute.
- `L008` (warning) is a painted `text` that loses content without saying so: (a) its computed `wrap` is `nowrap` (the initial value, or the literal `nowrap`), that value is not document-written, and its widest line is wider than its content box, so columns are cut with no ellipsis; (b) its lines after wrapping and truncation outnumber its content box's rows, and `overflow: hidden` is not document-written on it. Silence (a) by writing `wrap: nowrap` (an author's own choice, not the default) or `wrap: truncate`; silence (b) by writing `overflow: hidden` or giving it more room. It never fires on a table cell, a tab label, or a hint item (each has its own cut rule already), or on `visibility: hidden` text.
- `L009` (warning) is a container that cuts a child, in flow or docked, on an axis it does not itself scroll (a `<scroll axis="y">` is still checked on `x`): part or all of the child's outer rect falls outside the container's content box there. It is silenced by writing `overflow: hidden` on the container, and it is never reported for a `list`, a `table`, or a `hints` (they show part of their content by design), for a node `L003` already reported this frame (that message already explains the cut), or for a child `L006` already reported this frame (its own warning explains it); a child with `visibility: hidden` or an empty rect does not count.
- "Document-written" (both codes) means a presentational attribute, a stylesheet rule, or `style=""`, on the node itself or — `wrap` is inherited — on an ancestor it inherits it from; the initial value and the built-in sheet's own `table { wrap: truncate; }` never count. Each is reported once per source element and frame (a template's repeated rows collapse into one diagnostic), while the dump's `clipped` (`"text"` for `L008`, `"children"` for `L009`) marks every node that meets the condition, generated rows included.
- `tuimark ir` prints `"version": "0.3"` for a version="3" document, validated by `schema/ir.v0.3.json` (the same shape as IR 0.2 in 0.3a).

### Notes

Clarifications of behavior the SPEC states loosely or not at all (the SPEC itself is not edited; see README.md's "Language notes" section for the same points in more detail):

- **`<text>`/`<button>` body normalization.** Each line of the body is trimmed of XML whitespace (space, tab, CR) and leading/trailing blank lines are dropped; interior spaces are kept. Width and paint both see the trimmed content, so `<text> sync</text>` measures and paints as `sync`, not ` sync`. There is no XML-level way to force a leading/trailing ASCII space through the body (CDATA is trimmed the same way and numeric character references are rejected as V005); U+00A0 (NBSP) and other non-ASCII spaces are not XML whitespace and survive as content, if a literal non-breaking space is an acceptable stand-in. Otherwise use `gap`, `padding`, or `margin` on a parent for spacing.
- **Built-in defaults beyond §10.6, and `list`/`input` have no border.** §10.6 only lists defaults for `screen col row box text`; the runtime fills the rest with `scroll`/`list`/`item { layout: column }`, `modal { layout: column; border: single }`, `spacer { flex: 1 }`, `input, button, progress, rule { width: auto; height: auto }`, and `list > item:selected { reverse: true }`. Only `modal` gets a default `border`, so a `border-color` rule on `list`/`input` — a `:focus`/`:selected` variant included — changes nothing unless that element also sets `border: single|double|rounded|thick`. This is why the SPEC §17 inbox fixture's `theme.tcss` (`list:focus`/`input:focus { border-color: $focus; }`) has no visible effect: `dump --styles` and `play --input tab --styles` give byte-identical `styles` regardless of focus. Show focus with `color`/`background`/`bold`/`reverse` instead (as `button:focus { reverse: true; }` does in `examples/agent`/`examples/dashboard`), or give the element its own `border: single` first (as `examples/agent`'s `#composer`/`#transcript` and `examples/dashboard`'s bordered panels do).
- **Modal visibility (`open` vs `bind`).** `open` takes `true`, `false`, a bare path, or `!path` (never `{path}`; truthiness is the §7 table). `bind` on a `modal` is shorthand for the same thing: a path whose truthiness opens it. When a modal has both attributes, `open` wins outright and `bind` is ignored for visibility, even when `open` is a falsy path and `bind`'s path is truthy — prefer setting only one of the two on a given modal.
- **Cross-axis stretch inside a modal.** `col`/`row` default to `1fr` on both the main and the cross axis, and children stretch to fill the cross axis by default (`align: stretch`). A `<row>` of buttons placed inside a `<modal>` (default size 80%×80%, or any column-laid-out ancestor) therefore stretches to the modal's full remaining height, and its buttons stretch with it — most visible when a focused button has `reverse`, which paints its whole rect. Give an action row an explicit `height: 1` or `height: auto` (and consider `justify: center` / `gap` for spacing); `align: start` on the row alone shrinks the buttons but leaves the row itself still filling the height.
- **Focused `list` key handling.** A focused, non-empty `list` (or, in version="2", `table`) consumes `up`, `down`, `home`, `end`, `pgup`, and `pgdn` itself (moving the selection and firing `on:select` on change); these never reach the keymap while the list has focus. `left`/`right` are not consumed and fall through normally. `j`/`k` are never implicit for a list, same as they are not implicit anywhere else (§8.1).
- **List rows are not independently focusable.** Inside `<item>`, nothing takes focus — the list itself is the focusable unit and navigates its rows. Handle a per-row action through the list's `on:select`, or a keymap row scoped with `when="#list:focus"` that reads the event's `keys`/`value` for which row was selected.
- **`Dump` is a side-effect-free snapshot.** It never moves focus, queues events, or mutates scroll/list/input state; `Validate` has the same property. Only the internal frame used by `Run` is "live".
- **`Run` and signals.** On SIGTERM, SIGHUP, or SIGINT, `Run` restores the terminal (cooked mode, main screen, visible cursor) and returns a non-nil error naming the signal, so the process can exit with a failure status instead of exiting clean.
- **Built-in quit.** `ctrl+c` fires the built-in `quit` action when no focused widget or keymap row claims it. With no `On("quit", ...)` handler registered, that action stops `Run`; with a handler registered, `Run` stops only if the handler returns `ErrQuit`. Bind `ctrl+c` in the keymap to route it elsewhere.
- **Escape latency.** A trailing `esc` byte at the end of a read waits briefly (about 25ms) for more bytes before it is delivered as the `esc` key, so it can be told apart from the start of a longer escape/CSI sequence.
- **Keys of one read.** In a version="2" document each key or paste of one terminal read (or of one `play` `text:` step) is dispatched on a frame rendered again after the previous one changed anything the frame shows (an input's text or cursor, a list or table cursor, an offset, a checked array, a tab), so `when`, `class:NAME` guards, and `if` see it: `text:ab\r` fires an `enter` row whose `when` reads a class the typing turned on, exactly as `text:ab enter` does. A version="1" document keeps the 0.1 loop, rendering again between keys only when events fired or focus moved.
- **A `tabs` whose `bind` path cannot be written** (a value along it is not an object, say `ui.view` with `ui` a number) cannot change its active tab: `switch-to`, `move-*`, `left`/`right`, clicks, and the wheel change nothing there and fire no `on:select`, though a keymap row naming it still takes its key.
- **A dropped mouse event also ends a pending press.** A left release always ends the press before it; a press or release the mouse ignores (outside the top modal, outside the grid, or while `mouse` is false) acts on nothing and leaves no press pending. The wheel over a tab strip with no enabled tab stops there and changes nothing.
- **A viewport without an `id` keeps the offset the wheel gives it.** Only the wheel can move one (it cannot take focus, and `move-*` needs an id), and so it is for a viewport inside a list row or an `each` template, where an id repeats. It keeps that offset under its element and the key of the row it is in, so the offset stays with it when siblings before it come and go and when the rows are reordered; it starts at 0.

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
go build -o bin/monitor ./examples/monitor
go build -o bin/agent ./examples/agent
./bin/tuimark test
glyph run specs/glyphrun/*.yml --format md
go test -race ./internal/host/
```

### Package boundaries

- `cmd/tuimark` — the CLI: flag parsing, output formatting, the golden-manifest runner behind `tuimark test` (including its v0.2a superset check), `tuimark play`'s `--input`/`--script` step parsing and `--frames`/`--strict` wiring (the replay engine itself is `internal/play`, shared with `tuimark test` and `tuimark.Play`), `tuimark inspect` (`Frame` plus `Inspect`), and the theme/`:root` token table for `tuimark ir`. Parsing, cascade, layout, and paint rules live in `internal/**`.
- `tuimark.go` (package `tuimark`, repo root) — the only public surface: the package functions `Load`, `LoadFS`, and `Parse`, `App`'s methods `Bind, Set, Get, Batch, On, Catalog, Dump, Validate, Play, Run` (SPEC §18, §18.1), the `App` and `Batch` types, and the aliases `Event`, `Handler`, `ActionSpec`, `Dump`, `DumpNode`, `Diagnostic`, `PlayEvent`, `PlayOptions`, `PlayResult`, and `ErrQuit`. Nothing else is exported from it.
- `internal/ir` — node/scalar/diagnostic/binding-grammar/key-token types, the tag-kind catalog (`Kinds`, `SpikeKinds`), the spike attribute whitelist (`SpikeAttrs`), and the key-token catalog (`NamedKeys`/`ValidKey`).
- `internal/parse` — the XML tokenizer, the IR builder (including the per-tag attribute catalog, `TagAttrs`), the formatter (`fmt`), and IR-as-JSON (`ir`).
- `internal/css` — the TCSS parser, selectors, cascade, tokens/themes, and `@media`.
- `internal/layout` — the integer flex engine, with `layout: grid`, the tab strip, and the hint items (`widgets.go`) and the table (`table.go`).
- `internal/paint` — the cell grid, borders, widget painters, and the ANSI frame diff.
- `internal/dump` — the text and JSON frame dump.
- `internal/host` — the JSON store, `Load`/`LoadFS` (a shared `styleResolver` between an OS directory and an `fs.FS`), `Get`'s reserved paths and deep copies, `Batch`'s atomic apply, the `Run`/`Play` mode guard (SPEC v0.3 §18.1 rule 8), inflation (`each` on lists and containers, multi-select, table rows with their measure cache in `table.go`), cascade application, focus, the SPEC §8.6 key dispatch with the built-in actions (`dispatch.go`, one procedure for `Run`, `play`, and the hints), tabs (`tabs.go`), hints and sparklines (`hints.go`), the mouse hit test and click/wheel rules (`mouse.go`), events, and the `Run` loop (with the capability/theme probe and `TUIMARK_LOG`). A frame is built in the SPEC §18 order: inflate, cascade, tab activation, focus, hints, layout, table rows, paint.
- `internal/play` (v0.3) — the headless replay engine of SPEC §15.4 (`Session`, step parsing): the one place `tuimark play`, `tuimark test`, and `tuimark.Play` apply steps, so their parity holds by construction.
- `internal/agentsdoc` — generates this file from the catalogs above.
- Application code — `examples/**`, or any external module — uses only the root `tuimark` package. Inside the repo, `internal/layout` is imported by `internal/paint`, `internal/dump`, and `internal/host`; `internal/paint` is imported by `internal/dump`, `internal/host`, and `cmd/tuimark` (for `preview`'s ANSI frame); `internal/play` is imported by `cmd/tuimark` and by `tuimark.go`. SPEC §3 forbids importing `internal/layout`/`internal/paint` from application code outside this repo, not from other packages inside it.

### Hard rules

- The SPEC is the normative specification. It is maintained outside this repository, in the maintainer's notes (`~/notes/projects/tuimark/SPEC.md`): never copy it into the repository, and never change it without the owner's approval. Record interpretations and clarifications as design decisions in the project notes (see "Documentation boundary" below) and, when users need them, in README's "Language notes" and the generated Notes above.
- `testdata/golden/spike/**` is frozen: never regenerate or hand-edit those goldens (`tuimark test --update` skips frozen entries by design). A geometry change there is a runtime regression, not a fixture to update — add a new fixture and manifest entry instead if you need new coverage.
- `examples/inbox/{app.tui,theme.tcss,sample.json}` are verbatim SPEC §17 fixtures: keep them byte-identical to the SPEC's text.
- Closed vocabulary: never invent a tag, attribute, CSS property, unit, or selector. Extending the catalog is a deliberate, catalog-first change (see "Adding things" below), not something a parser special-case should do quietly.
- No Bubble Tea, Lipgloss, `tea.Cmd`/`Update()`, or any other TUI toolkit, as or inside the authoring surface.
- No new dependencies beyond the three ADR-approved ones: `golang.org/x/term`, `golang.org/x/sys`, and, since ADR 0002 (v0.2), `github.com/rivo/uniseg` — used only for grapheme-cluster segmentation; Tuimark's own width function decides column width, ambiguous-width runes fixed at 1.
- Every behavior change ships with a regression test and a design-decision entry in the project notes explaining it (see "Documentation boundary" below).

### Documentation boundary

- `docs/` is the public Tuimark website (VitePress, deployed from `main`). Put only publishable content there: landing page, user guides, public reference, static assets. Its reference pages and screenshots are generated: run `task docs-gen` after changing the CLI, the catalogs, or an example, and `task docs-check` to confirm nothing is stale.
- Design decisions, ADRs, specs in progress, plans, handoffs, review findings, and progress logs live outside this repository, in the maintainer's Obsidian vault at `~/notes/projects/tuimark/` (ADRs under `adrs/NNNN-title.md`, the topic-organized decision record in `design-decisions.md`). Never add them to the repo, and never link public docs to those private paths.
- The SPEC itself lives there too (ADR 0012), not in this repository. The repository keeps only a frozen copy of its §22 block, `internal/agentsdoc/testdata/spec22.md`, for the AGENTS.md prefix test; set `TUIMARK_SPEC` to the SPEC's path to check that copy against the source.
- In the repo, `README.md` is the front page (user-facing clarifications go in its "Language notes") and `AGENTS.md` is generated from `internal/agentsdoc`.

### Adding things

Only when the SPEC itself adds one: SPEC §1 forbids inventing tags, CSS properties, units, or selectors, and SPEC §14 is the single diagnostic taxonomy. These recipes are the mechanics for a change the SPEC has already made, not license to extend the catalog on the runtime side.

- **A diagnostic code.** Implement the check where it belongs (`internal/ir`, `internal/parse`, `internal/css`, `internal/layout`, or `internal/host`), add its row to the `diagCodes` table in `internal/agentsdoc/agentsdoc.go`, add a probe/regression test that triggers it, then regenerate this file.
- **A CSS property.** Add it to `internal/css`'s property table (parsing, `PropertyNames`/`PropertyValues`, and the cascade), add a test in `internal/css`, then regenerate this file so "### CSS properties" above picks it up.
- **A tag attribute.** Add it to `internal/parse`'s `TagAttrs` catalog and whatever in `internal/ir`/`internal/host` needs to read it, add a parser/build test, then regenerate this file so "### Attributes per tag" above picks it up.
- **Regenerating this file.** `go run ./cmd/tuimark agents --repo > AGENTS.md`, then `go test ./internal/agentsdoc`: `TestAGENTSDoesNotDrift` and `TestAGENTSStartsWithSpec22Verbatim` must stay green. Never hand-edit `AGENTS.md`.

### Glyphrun conventions (`specs/glyphrun/`)

- After a deliberate change to a spec's `intent` or `outcomes`, run `glyph spec verify <spec> --stamp` to refresh its `contractHash`; a stale hash otherwise means the contract drifted without review. A spec with no `contractHash` line at all is not checked by this convention (`glyph spec verify` reports `contractHashValid: false` for it but `glyph run` still passes it) — stamp every new spec once its contract is settled.
- Assertions wait for state — `wait: {screen: {contains|notContains: ...}}` or `wait: {process: {exitCode: N}}`, each with a `timeoutMs` — before a snapshot or an outcome is checked. Specs never sleep and hope, but presses whose in-between state doesn't matter (e.g. `press: tab` then `press: down`) are sent back to back with no wait between them. The `preview_watch` fixture script uses `sleep` itself, outside the harness, to space out the file edit it makes.
- `examples/agent`'s specs launch `bin/agent --demo-workspace .glyphrun/ws/<name>` so each spec gets its own throwaway agent workspace; that workspace directory is wiped and reseeded on every run. `.glyphrun/runs/` keeps one timestamped directory per run, pruned to the last `retention.keepRuns`. Both `.glyphrun/ws/` and `.glyphrun/runs/` are gitignored.
- Transient states (a modal mid-open, a progress bar mid-run) are captured with `snapshot: <name>` at the moment they're true and asserted from `$GLYPHRUN_RUN_DIR/snapshots/<name>.txt` in a `script` verifier, not from the live end-of-run screen, which has since moved on.

See `README.md` for the user-facing quickstart, CLI reference, and "Language notes"; the reasoning behind specific interpretation calls is in the design-decision notes described under "Documentation boundary".
