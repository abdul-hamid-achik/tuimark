# Tuimark kit (v0)

A copy-and-paste component kit for Tuimark, in the spirit of shadcn: you do not
install it, you copy what you need into your project and own it.

- `theme.tcss`: the design tokens (dark and light) and the base typography.
- `kit.tcss`: the component classes.
- `components/NAME.tui`: one small, valid, self-contained document per
  component, with the markup to copy. A `NAME.sample.json` sits beside it
  when it binds data. Each loads `../theme.tcss` and `../kit.tcss`, so it
  validates and dumps on its own:

  ```sh
  tuimark validate kit/components/table.tui --data kit/components/table.sample.json
  tuimark dump kit/components/table.tui --data kit/components/table.sample.json --cols 80 --rows 24 --format json
  tuimark run kit/components/table.tui --data kit/components/table.sample.json
  ```

- `examples/kit-gallery`: every component in one app, on five tabs, with a Go
  host. Start there to see them together.

The kit is a vocabulary of **classes**, not a new language. Nothing here adds a
tag, an attribute, or a CSS property; a component is ordinary markup plus
class names, ids, and action names that follow three conventions (below).

## Using it

Copy `theme.tcss` and `kit.tcss` into your project, load them first, and put
your own sheet last:

```xml
<tui theme="auto">
  <style src="theme.tcss"/>
  <style src="kit.tcss"/>
  <style src="app.tcss"/>   <!-- yours: token overrides, app-specific sizes -->
  ...
```

There is no `@import` in TCSS, so every document lists the sheets it uses. The
sheets are cascaded together; where two rules tie, the later one wins, so a
token re-declared in your sheet rebrands the whole kit. A `<tui>` needs no
`version` attribute.

Then copy the markup of the component you want from `components/NAME.tui`.
Its comments say what to keep and what to change. Each component file starts
with a **contract**, an XML comment that says what the component needs from the
store and the host:

```
k-NAME · Contract: <store paths it reads and writes> · Actions: <named actions> · Keys: <keys> · Requires: <other kit classes>
```

## Tokens

All color lives in `:root` of `theme.tcss`, read as `$name`. Edit the values to
retheme the whole kit; `@media (theme: light)` holds the light values. The
first ten tokens are the runtime's own required tokens (the same names its
built-in themes define); the rest are the kit's.

| Token | Use |
|---|---|
| `bg` | the screen |
| `fg` | body text |
| `muted` | secondary text and labels |
| `border` | idle borders |
| `panel` | cards, panels, modals, inputs |
| `accent` | the brand color: primary actions, the active tab, focus titles, the active cursor bar |
| `focus` | the border or fill of the focused thing |
| `ok` `warn` `danger` | status colors |
| `subtle` | tertiary text: shortcuts, separators |
| `raised` | raised or inset things on a panel: ghost fills, `k-kbd`, neutral badges, bars |
| `info` | the info status color |
| `sel` | the cursor row of a list or table that does not have focus |
| `on-accent` `on-ok` `on-warn` `on-danger` | text drawn on that filled color (`on-accent` also goes on `info`) |
| `code` | inline code |

Selection and focus never rely on a background alone: the cursor row is bold,
the focused viewport has a double border, the focused cursor row is a solid
accent bar, a focused button is inverted and underlined. They read under 16
colors.

## Conventions

**Classes.** Every kit class starts with `k-`. A component is a root class
(`k-card`) plus parts that carry the root's name (`k-card-title`,
`k-card-text`): TCSS has no descendant combinator (only `>`), so a component
cannot style "text inside a card", and every part carries its own class. A
**modifier** is a second, short class on the same node (`primary`, `secondary`,
`danger`, `ghost`, `ok`, `warn`, `info`, `accent`, `good`, `bad`, `invalid`,
`current`, `muted`). It is only ever written next to the component class:
`.k-btn.danger`, `.k-badge.ok`, `.k-alert.warn`. No rule targets a bare
modifier. A compound selector outranks a single class, so a modifier wins over
its base wherever it sits in the file. Choose a modifier from data with
`class:NAME="path"` guards. The runtime's generated classes (`tab-label`,
`hint-key`, `hint-label`) keep their names and are styled only under a
component (`.k-tabs > .tab-label`).

Typography and layout utilities are `k-title`, `k-subtitle`, `k-muted`,
`k-strong`, `k-code`, `k-kbd`, `k-num`, `k-text-accent` (and `-ok`, `-warn`,
`-danger`, `-info`), `k-page`, `k-page-body`, `k-section`, `k-cols`.

**Ids** are `instance` for the node that is the component (`svc`, `prefs`) or
`instance__part` (two underscores) for a part, the overlay node itself
included: `confirm__dialog`, `confirm__yes`, `confirm__no`, `palette__dialog`,
`palette__q`, `palette__list`, `help__dialog`, `field__email`. Only app-level
singletons keep a plain id (`app`, `status`, `nav`), and so do the ids of your
own `tab` elements, which are the values of the path the tabs bind to.

**Actions** are `component_verb`, a plain identifier (dots and hyphens are
invalid): `dialog_confirm`, `dialog_cancel`, `palette_open`, `palette_close`,
`palette_run`, `toast_show`, `toast_dismiss`, `help_open`, `help_close`,
`form_submit`, `form_validate`, `menu_select`, `table_select`, `log_append`,
`breadcrumb_up`. No generic names (`cancel`, `open`, `save`). `quit` is the
built-in and stays. An action does not say which instance fired it: a host
routes by the **prefix of the event's `source` before the last `__`**
(`confirm__yes` belongs to the instance `confirm`), and keeps that instance's
state under its own name (`confirm.open`). The gallery does exactly that, in
`instanceOf` in `examples/kit-gallery/main.go`.

**Overlays** are `modal`s, last in the screen (L004), with `on:escape`, a size
written by their class, and a keymap scoped to their own class that comes
before any global keymap:

```xml
<keymap when=".k-dialog">
  <bind keys="y" action="dialog_confirm" label="confirm"/>
  <bind keys="n" action="dialog_cancel" label="cancel"/>
</keymap>
```

The first matching row, in document order, wins, so a global `q` row after it
cannot steal the overlay's keys.

**Sizes are explicit.** A component that must fill a space says `width: 1fr` or
`height: 1fr` in its class, a modal writes its size (`width: 52; height: auto`),
and a `tabs` writes `focusable`, so no component depends on what a bare `row`,
`col`, `modal`, or `tabs` defaults to. Text that must shrink to its content is
`width: auto`. A test keeps this true of every document in the kit.

**Other notes.**

- A node can only change `border-color` if it has its own `border`, so inputs and
  panels that show focus set one (and switch it to `double`).
- Presentational attributes lose to author CSS: a `height="9"` attribute does
  not beat `.k-table { height: 1fr }`. To size a component in your app, add a
  rule to your own sheet (`#svc { height: 9; }`).
- Narrow terminals: every kit document validates clean at 40, 80 and 120
  columns. `@media (max-cols: 60)` hides the least important part (the app bar's
  meta, a status bar's muted segment, a breadcrumb's older crumbs), and
  `@media (max-cols: 70/90)` stacks the split pane and `k-cols` rows.
- A container has at most one bottom-docked child (L005).

---

## Components

Each section: what it is, the classes, the actions it names (the host
implements them), and the keys. The full markup is in the named file.

### badge (`components/badge.tui`)

A short status label. One `text` with `k-badge` and a modifier: `accent`, `ok`,
`warn`, `danger`, `info` (none is neutral).

```xml
<text class="k-badge ok">passing</text>
<text class="k-badge" class:ok="build.ok" class:danger="!build.ok">{build.status}</text>
```

No actions, no keys.

### button (`components/button.tui`)

`button` with class `k-btn` and a modifier: `primary`, `secondary` (the neutral
filled one), `danger`, `ghost` (no background at all). The focused button is
drawn inverted, bold and underlined, and no modifier shares its focus look with
its idle look. A `disabled` button is dimmed. The label is literal text (a
`{path}` in `label` is V003).

```xml
<button id="buttons__save" class="k-btn primary" label="Save" on:click="button_save"/>
```

Action: whatever `on:click` names. Keys: `enter` / `space` press the focused
button, `tab` / `shift+tab` move focus.

### card / panel (`components/card.tui`)

A `box` with class `k-card` and a `title`. Parts: `k-card-title`, `k-card-text`,
`k-card-muted`, `k-card-row` (a one-row group of badges or text),
`k-card-actions` (a one-row button group). The border takes the focus color
while anything inside holds focus (`:focus-within`). Side-by-side cards: a `row`
of cards, each `1fr`.

### alert / callout (`components/alert.tui`)

A `box` with class `k-alert` and a modifier (`info`, `ok`, `warn`, `danger`),
the title as the box title, and a row with a glyph (`k-alert-icon` plus the
same modifier) and the wrapping `k-alert-text`.

### app bar (`components/appbar.tui`)

A one-row `row` with class `k-appbar`, docked to the top of its parent
(`dock: top`) so the content below keeps all the remaining space. Parts:
`k-appbar-brand`, `k-appbar-title`, `k-appbar-meta` (hidden under 60 columns),
then any badge. Put one top-docked bar in a container.

### status bar (`components/statusbar.tui`)

A one-row `row` with class `k-statusbar`, docked to the bottom. Left segments, a
`spacer`, right segments. Parts: `k-statusbar-seg` (a filled chip; modifiers
`ok`, `warn`, `danger`), `k-statusbar-text`, `k-statusbar-muted` (hidden under
60 columns). One bottom-docked bar per container (L005): put the key hints and
the toast above it, not docked.

### key-hint bar (`components/hints.tui`)

`<hints class="k-hints"/>`. Every keymap row with a `label` is a hint: the key
(or its `keycap`) in the accent color, the label muted. `scope="active"` (the
default) shows only the keys that would fire now; hints that do not fit drop
off the right. A built-in like `move-next` is hidden while its target does not
exist, so bind host actions you want listed unconditionally.

### confirm dialog (`components/confirm.tui`, class `k-dialog`)

A `modal` with class `k-dialog` (add `danger` for a destructive one), a
`k-dialog-text`, and an action row `k-dialog-actions` of `height: 1`. The class
writes the size (52 wide, as tall as its content, at most the screen). `open` is
a path the host flips; `focus` puts the cursor on the safe button so a stray
`enter` cancels; `on:escape` cancels. Keep the labels short so the row fits 40
columns.

Ids: `confirm__open` (the trigger), `confirm__dialog`, `confirm__no`,
`confirm__yes`. Actions: `dialog_open`, `dialog_confirm`, `dialog_cancel`. The
host sets `confirm.open`. Keys: `y` confirms and `n` cancels (the keymap
`when=".k-dialog"`), `esc` cancels.

### form field (`components/field.tui`)

A `col` with class `k-field`: `k-field-label`, an `input` with class
`k-field-input`, `k-field-help`, `k-field-error`. The input has its own border
and `height: 3`, which is what lets `:focus` change `border-color`.
`class:invalid="form.x_error"` on the label and the input turns them red while
the path is non-empty; the help line shows `if="!form.x_error"` and the error
`if="form.x_error"`.

Ids: `field__name`, `field__email`, `field__save`. Actions: `on:change`
(`form_validate`: the host checks the value and writes `form.x_error`),
`on:submit` / the Save button (`form_submit`). Keys: type; `enter` submits;
`ctrl+s`.

### empty state (`components/empty.tui`)

A `col` with class `k-empty` that fills the space left and centers:
`k-empty-icon`, `k-empty-title`, `k-empty-text`, then the call to action. Show
it next to the list it replaces with `if="!items.any"`. Action: the button's
`on:click` (`empty_create`).

### data table (`components/table.tui`)

A `table` with class `k-table` and `column` children. The header is bold and
muted; the cursor row is bold on a quiet fill, and a solid accent bar while the
table has focus. The class `k-num` on a `column` right-aligns its header and
cells. `priority` hides columns lowest first when the table is too narrow, and
a `min-width` (class `k-table-name`) on the `1fr` column makes columns hide
before it is squeezed. `k-table-muted`, and `k-table-state` with the modifiers
`ok` / `warn` / `danger`, color a column; guard the modifiers with `class:NAME`
on the column.

Actions: `table_select` (`on:select`, fired as the cursor moves, `keys` holds
the row key under the alias) and `table_open` (a keymap `enter`). `bind`
receives the cursor key. Keys: `up`/`down`/`home`/`end`/`pgup`/`pgdn` are the
table's; `j`/`k` via `move-next`/`move-prev`.

### menu / nav list (`components/menu.tui`)

A `list` with class `k-menu`; each item is a `k-menu-row` of `k-menu-icon`,
`k-menu-label`, `k-menu-key`. `checked` + `mark="✓"` add multi-select marks.
The cursor row is bold on a quiet fill until the menu has focus, then a solid
accent bar. Actions: `menu_select` (`on:select`), `menu_change` (`on:change`,
the new checked array), and the keymap `enter` for `menu_open`. Keys: `space`
toggles the mark (`check-toggle`).

### tabs (`components/tabs.tui`)

A `tabs` with class `k-tabs`, `focusable`, `bind` to the active tab's id, and
`tab` children (each needs an `id` and a `label`; `short` is the label when the
strip does not fit). The generated labels are styled by `.k-tabs > .tab-label`
and its `:selected` variant. Only the active tab's content is laid out, so
validate it by switching the bound path. Keys: `left`/`right` on the focused
strip, `]`/`[` (`move-next`/`move-prev` `to="#nav"`), digits (`switch-to`).
Optional action: `on:select`.

### labeled progress (`components/meter.tui`)

A `col` with class `k-meter`: a `k-meter-head` row (`k-meter-label`, `spacer`,
`k-meter-value`) over a `progress` with class `k-meter-bar`. `bind` is a number
from 0 to 100. Add `ok`, `warn` or `danger` to the bar, or guard them from
data. The host updates the number and the text; no actions.

### stat / KPI card (`components/stat.tui`)

A `box` with class `k-stat` (the label is the box title): `k-stat-value` in
bold, then a `k-stat-row` with `k-stat-delta` (`good` or `bad` for green or red,
chosen by `class:good`/`class:bad` because whether a rise is good depends on the
metric) and a `sparkline` with class `k-stat-spark` bound to an array of
numbers. No actions.

### toast (`components/toast.tui`)

A `k-toast-slot` row that is always one row high, with a `k-toast` row inside
shown by `if="toast.show"`, so the page never jumps. Info is the default; guard
`ok`, `warn`, `danger`. The **host clears it**: it sets `toast.show` back to
`false` from an `Every` tick (see `examples/kit-gallery/main.go`, where a
once-a-second tick counts the toast down). The slot is not docked, so it can sit
above a docked status bar. Actions: `toast_show` (the trigger), `toast_dismiss`
(`esc`).

### command palette (`components/palette.tui`)

A `modal` with class `k-palette`: an `input` (`k-palette-input`, bound to the
query) over a `list` (`k-palette-list`) of the filtered commands. The markup has
no expressions, so the **host filters**: it keeps `found` (the matching
commands, plus `any`) with `Derive` over `palette.query` and
`palette.commands`, and the list reads it. `focusable="false"` on the list keeps
focus in the input; while the palette is open, `up`/`down` (and
`ctrl+n`/`ctrl+p`) are not consumed by the input, so they reach the keymap
`when=".k-palette"`, where `move-next`/`move-prev` with `to="#palette__list"`
move the cursor. Enter is the input's `on:submit`.

Ids: `palette__dialog`, `palette__q`, `palette__list`. Actions: `palette_open`
(`ctrl+k`), `palette_close` (`esc`), `palette_run` (the host does the command
whose id is `palette.cur`, or the first match).

### help overlay (`components/help.tui`)

A `modal` with class `k-help` holding `<hints class="k-hints k-help-keys"
scope="all"/>`: every labeled keymap row, stacked. Id: `help__dialog`.
Actions: `help_open` (`?`), `help_close` (`q`, `?`, `esc`).

### split pane / master-detail (`components/split.tui`)

A `row` with class `k-split`: a `list` (classes `k-menu k-split-master`) and a
card (`k-card k-split-detail`). The list `bind` is the selected id; the **host
keeps `detail`** with `Derive` over the items and the cursor, so the right side
follows the cursor with no handler. Under 70 columns the panes stack. Action:
`menu_select`.

### log panel (`components/log.tui`)

A `scroll` with class `k-log`, an `id`, `focusable="true"` and
`stick="bottom"`: it follows the end as lines are appended, until you scroll up
to read, and `move-last` makes it follow again. The lines are a `col`
(`k-log-lines`, `height: auto`) with `each` and a `key`; color a line with the
modifiers `muted` / `info` / `warn` / `danger`. Every line must carry all the
flags (a missing path is B002). The host appends to the array (`log_append`).
Keys: `j`/`k` scroll, `G` follows.

### breadcrumb (`components/breadcrumb.tui`)

A `row` with class `k-breadcrumb` and `each` over `{id, label, last}`; its
children are the template: a `k-breadcrumb-item` (plus `current` for the last,
by guard) and a `k-breadcrumb-sep`. Under 60 columns only the current crumb
shows. Display only (a template cannot hold anything focusable); `backspace`
fires `breadcrumb_up`, and the host trims the array.

---

## Next

These are plans, not promises.

- **Fragments.** Today a component is markup you copy, and the repetition shows:
  the gallery spells out the same `k-meter`, `k-stat`, `k-field` and `k-alert`
  blocks several times. When the language gains composition (a `template` and
  a `use` with an `id`), each component can become a fragment written once. The
  conventions above are chosen for that: a `use id="confirm"` would generate the
  ids `confirm__dialog`, `confirm__yes`, `confirm__no`, so hosts written today
  would not change. The class vocabulary and both stylesheets stay as they are.
- **Host helpers.** A host today writes its own small helpers (see
  `instanceOf` and the toast, dialog and palette handlers in
  `examples/kit-gallery`). They are meant to ship as files you copy, in Go and
  in TypeScript, not as a public package.
- **`tuimark add`.** A command that copies a component, with the sheets it
  needs, into your project.
