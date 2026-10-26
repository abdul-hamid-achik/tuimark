# Tuimark kit (v2)

A copy-and-paste component kit for Tuimark, in the spirit of shadcn: you do not
install it, you copy it into your project and own it.

- `kit.tui`: the kit as one `<fragment>`. It loads the two sheets below, holds
  the overlays' keys, and has one `<template>` per component that takes text,
  data or content (card, key / value, app bar, alert, stat, meter, empty state,
  form field, breadcrumb, log, toast, confirm dialog, command palette, help,
  and, for a chat, the composer, a transcript block, and a tool receipt).
  Include it once and copy a component with `<use>`.
- `theme.tcss`: the design tokens (dark and light) and the base typography.
- `kit.tcss`: the component classes.
- `components/NAME.tui`: one small, valid document per component. For a
  template it shows the `<use>`; for a class component (badge, button, status
  bar, key hints, data table, menu, tabs, split pane) it holds the markup to
  copy. A `NAME.sample.json` sits beside it when it binds data.
  Each includes `../kit.tui`, so it validates and dumps on its own:

  ```sh
  tuimark validate kit/kit.tui      # ok (fragment checked on its own)
  tuimark validate kit/components/table.tui --data kit/components/table.sample.json
  tuimark dump kit/components/table.tui --data kit/components/table.sample.json --cols 80 --rows 24 --format json
  tuimark run kit/components/confirm.tui --data kit/components/confirm.sample.json
  ```

- `examples/kit-gallery`: every component in one app, on six tabs, with a Go
  host. Start there to see them together.
- `go/` and `ts/`: host helpers (toasts, filter, sort and pages, forms, dialogs,
  a palette, async loads, formatting) to copy the same way. See
  [Helpers](#helpers).

Nothing here adds a tag, an attribute, or a CSS property: the kit is ordinary
templates and markup, plus class names, ids, and action names that follow a
few conventions (below).

## Using it

Copy `kit.tui`, `theme.tcss` and `kit.tcss` into one folder of your project
(`kit.tui` loads the two sheets from its own folder), include the kit as the
first child of your `<tui>`, and put your own sheet after it:

```xml
<tui theme="auto">
  <include src="kit/kit.tui"/>
  <style src="app.tcss"/>   <!-- yours: token overrides, app-specific sizes -->
  <screen id="main">
    <col width="1fr" height="1fr" pad="1">
      <use template="k-alert" class="warn" title="Quota" icon="!" text="92% of the build minutes used."/>
    </col>
    <use id="confirm" template="k-confirm" class="danger" with="confirm as d"/>
  </screen>
</tui>
```

The sheets are cascaded together; where two rules tie, the later one wins, so a
token re-declared in your sheet rebrands the whole kit. A `<tui>` needs no
`version` attribute.

**Templates.** `<use template="k-NAME" …/>` copies a component where it
stands. A `<use>` passes three kinds of arguments:

- **Props** are literal text: a title, a label, a placeholder, an icon, the
  sparkline's limits (`title="CPU"`, `min="0"`). A prop is text, never a path
  (`title="{card.title}"` is V003); a title that comes from data is the class
  markup (`<box class="k-card" title="{PATH}">`); only the confirm dialog and
  the log read their title from `with`. A prop
  you leave out is an attribute the copy does not write, or an empty text that
  the kit hides (`:empty`: the alert's icon, the empty state's text).
- **Data** comes through the `with` alias: values, arrays, the flags in `if`,
  `open` and `class:NAME` guards, and the paths a widget writes. A copy reads
  `x.value` as `PATH.value`, and a widget that writes (a field's input, the
  palette's cursor) writes under `PATH`. A template that reads two objects
  takes two aliases (`with="palette as p, found as f"`), and `k-kv` takes a
  scalar (`with="svc.owner as v"`). Flags are optional members of the alias: a
  missing one reads as false. The elements of an array the template repeats (a
  log's lines) still carry every flag (B002 otherwise).
- **Content** is what you write between `<use>` and `</use>`, for the
  templates that have a `<slot/>`: card (the body), app bar (the right side),
  alert (more body after `text`), empty state (the call to action) and confirm
  (more body under `d.body`). Content stays in the place where you wrote it:
  its paths are the document's (the alias of the `with` does not exist there:
  write `{confirm.detail}`, not `{d.detail}`), its ids are yours (you write
  the full id, e.g. `deploy__create`: the `<use>` adds no prefix), and its
  actions are the app's. Two uses of one template with the same content would repeat its ids
  (V004). Content in a template with no slot is V013.

Besides:

- **A variant** is a static `class` on the `<use>` (`class="danger"`). It lands
  on the copy's root, and the kit's rules style the parts from there
  (`.k-alert.danger > .k-alert-row > .k-alert-icon`). It is a modifier, not a
  prop.
- **`style`** is a prop of every template. It lands on the copy's root, which a
  rule cannot name (unless the template gives it an id: the overlays' `U__dialog`,
  the log's `U__view`), so it is how an instance gets a size
  (`<use template="k-card" title="Empty state" style="height: 10">`); a prop
  you do not pass writes no `style` at all.
- **Ids** come from the use: a template that holds ids needs an `id` on its
  `<use>`, which prefixes them. `<use id="confirm" template="k-confirm">` makes
  `confirm__dialog`, `confirm__no` and `confirm__yes`. A `<use>` is not a node:
  its own id names nothing, and a template with no ids of its own needs none.
- **Sizes**: every template's root writes its own size, so a copy is the same
  size in a screen, a tab, or a `col`.

**Classes.** A component stays a class, not a template, when a template would
save nothing or cannot express it:

- **table, menu, split**: their columns, rows or panes read a row alias, which
  would be a slot with scope. A `<slot/>` inside an `each`, a list `<item>` or a
  table is V001. A menu's list id (`menu`, named by keymaps and hosts) would
  also become `U__list`.
- **badge, button, tabs, hints**: one element each, and each carries an id or a
  handler of its own (`{$}` cannot go in an id).
- **status bar**: two sides, and a template has one slot.
- **a card with a data title, an app bar with a data title**: the class markup
  (`<box class="k-card" title="{PATH}">`) stays valid next to the templates.

Copy the markup from `components/NAME.tui` and keep its classes. There are no
named slots, no scoped slots, and a template has at most one `<slot/>`.

Each component file starts with a **contract**, an XML comment that says what
the component needs from the store and the host:

```
k-NAME · Contract: <the props, the with alias and the paths it reads and writes, the slot> · Actions: <named actions> · Keys: <keys> · Requires: <other kit classes>
```

## Upgrading from kit v1

Kit v2 reads literal text as props, and a use that still passes the v1 `with`
members shows no title or label, with no diagnostic. Move each one as follows.

| v1 | v2 |
|---|---|
| `k-alert` with `a.title`, `a.icon`, `a.text` | props `title`, `icon`, `text`; a line that comes from data goes in the content |
| `k-stat` with `s.title` | prop `title` (and `min`, `max`); `value`, `delta`, `hist`, `good`, `bad` stay in `with` |
| `k-meter` with `m.label` | prop `label`; `text`, `pct` and the flags stay in `with` |
| `k-empty` with `e.icon`, `e.title`, `e.text`, `e.action`, the button `U__create` and the action `empty_create` | props `icon`, `title`, `text`; your own button, with your id and your action, inside the `<use>` |
| `k-field` with `f.label`, `f.placeholder`, `f.help` | props `label`, `placeholder`, `help`; `f.value` and `f.error` stay in `with` |
| `k-palette` with `p.title`, `p.placeholder` | props `title`, `placeholder`; the palette helpers no longer write defaults |
| `k-help` with `h.title` | prop `title`; drop any app height rule on `#U__dialog` |
| `k-confirm` | unchanged; optional content under the body |
| `<box class="k-card" title=…>` | `<use template="k-card" title=…>…</use>`; the class form stays valid, and it is the one for a title that comes from data |
| `<row class="k-appbar">` | `<use template="k-appbar" brand=… title=…>`, the right side as content |
| `k-card-row` rows | `<use template="k-kv" label=… with="PATH as v"/>` |
| an app rule on a copy's id | `style="…"` on the `<use>` |

## Tokens

All color lives in `:root` of `theme.tcss`, read as `$name`. Edit the values to
retheme the whole kit; `@media (theme: light)` holds the light values. The
first ten tokens are the runtime's own required tokens (the same names its
built-in themes define); the rest are the kit's. Every text token reads at
WCAG AA (4.5:1) on `bg`, `panel`, `raised` and `sel`, and every `on-*` token on
the fill it sits on, in both palettes; `TestPaletteContrast` in
`examples/kit-gallery` fails a palette that does not.

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
`current`, `muted`, and the kinds of a `k-block`: `code`, `head`, `quote`,
`item`). It is only ever written next to the component class
(`.k-btn.danger`, `.k-badge.ok`), or as a `<use>`'s variant, which lands on a
template's `k-` root (`.k-alert.warn`). No rule targets a bare
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
included. A template writes only the parts (`dialog`, `no`, `yes`), and the
use's id is the instance, so `<use id="confirm" …>` gives `confirm__dialog`,
`confirm__yes`, `confirm__no`; `<use id="palette" …>` gives `palette__dialog`,
`palette__q`, `palette__list`; `<use id="help" …>` gives `help__dialog`; the
field `<use id="profile__email" …>` gives the input `profile__email__input`.
Only app-level singletons keep a plain id (`app`, `status`, `nav`), and so do
the ids of your own `tab` elements, which are the values of the path the tabs
bind to.

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
before any global keymap. `kit.tui` holds the dialog's and the help's, so they
come first in every document that includes it:

```xml
<keymap when=".k-dialog">
  <bind keys="y" action="dialog_confirm"/>
  <bind keys="n" action="dialog_cancel"/>
</keymap>
```

The dialog's rows have no `label`: its buttons name the choices, and `y` and
`n` work while it is open, so no `<hints>` (the help overlay included) lists
them.

The first matching row, in document order, wins, so a global `q` row after it
cannot steal the overlay's keys, and an open modal traps the keymap, so a key
that must also work under one is repeated in a `<keymap when="modal">`. The
kit's keymap rows name classes only, never ids, since they land in every
document that includes it; a row that names a copy's id (the palette's arrows,
`to="#palette__list"`) belongs in your document.

**Sizes are explicit.** Every template's root writes its own size, a component
that must fill a space says `width: 1fr` or `height: 1fr` in its class, a modal
writes its size (the dialog `width: auto; max-width: 60; height: auto`, so it
takes its content's size up to 60 columns), and a `tabs` writes `focusable`, so
no component depends on what a bare `row`, `col`, `modal`, or `tabs` defaults
to. Text that must shrink to its content is `width: auto`. A test keeps this
true of every document and template in the kit.

**Other notes.**

- A node can only change `border-color` if it has its own `border`, so inputs and
  panels that show focus set one (and switch it to `double`).
- Presentational attributes lose to author CSS: a `height="9"` attribute does
  not beat `.k-table { height: 1fr }`. To size a component in your app, add a
  rule to your own sheet (`#svc { height: 9; }`); to size a copy of a
  template, which has no id, pass `style` (`style="height: 9"`), which is
  author CSS on the root.
- Narrow terminals: every kit document validates clean at 40, 80 and 120
  columns. `@media (max-cols: 60)` hides the least important part (the app bar's
  meta, a status bar's muted segment, a breadcrumb's older crumbs), and
  `@media (max-cols: 70/90)` stacks the split pane and `k-cols` rows.
- A container has at most one bottom-docked child (L005).

---

## Components

Each section: what it is, the template (with what it reads) or the classes,
the actions it names (the host implements them), and the keys. The named file
shows it in use; a template's own markup is in `kit.tui`.

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
its idle look. A `disabled` button is dimmed. The label may be a `{path}`,
resolved every frame (`label="{d.yes}"` in the confirm dialog's template).

```xml
<button id="buttons__save" class="k-btn primary" label="Save" on:click="button_save"/>
```

Action: whatever `on:click` names. Keys: `enter` / `space` press the focused
button, `tab` / `shift+tab` move focus.

### card / panel (`components/card.tui`, template `k-card`)

`<use template="k-card" title="Deploy">…</use>`: a `box` with class `k-card`
whose title is the `title` prop and whose body is the use's content. Parts:
`k-card-title`, `k-card-text`, `k-card-muted`, `k-card-row` (a one-row group of
badges or text), `k-card-actions` (a one-row button group). The border takes
the focus color while anything inside holds focus (`:focus-within`).
Side-by-side cards: a `row` of cards, each `1fr`. A card whose title comes from
data is the class markup, `<box class="k-card" title="{PATH}">`. `style` sizes
one copy (`style="height: 10"`).

### key / value row (`components/kv.tui`, template `k-kv`)

`<use template="k-kv" label="Owner" with="svc.owner as v"/>`: a one-row `row`
with class `k-kv` of the `label` prop in a muted, 8-column cell and the value
`{v}` after it, for a card of facts. A row that mixes a badge and text uses the
classes `k-kv`, `k-kv-label` and `k-kv-value` without the template.

### alert / callout (`components/alert.tui`, template `k-alert`)

`<use template="k-alert" class="warn" title="Quota" icon="!" text="92% used."/>`:
a `box` with class `k-alert`, the `title` prop as the box title, and a row with
the glyph `icon` and a body of the wrapping `text`. Every prop is optional (no
icon, no glyph; the kit hides what is empty). The use's content goes in the
body after the text: a line from data (`<text>{build.retry}</text>`) or a
button. The variant (`info`, `ok`, `warn`, `danger`) colors the frame and the
glyph.

### app bar (`components/appbar.tui`, template `k-appbar`)

`<use template="k-appbar" brand="◆ orbit" title="Deployments">…</use>`: a
one-row `row` with class `k-appbar`, docked to the top of its parent
(`dock: top`) so the content below keeps all the remaining space. The `brand`
and `title` props are `k-appbar-brand` and `k-appbar-title`; the use's content
is the right side, after a `spacer`: `k-appbar-meta` (hidden under 60
columns), then any badge. Put one top-docked bar in a container. A title that
comes from data is the class markup (`<row class="k-appbar">` with its own
`k-appbar-title` text).

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

### confirm dialog (`components/confirm.tui`, template `k-confirm`)

`<use id="confirm" template="k-confirm" class="danger" with="PATH as d"/>`, the
last child of the screen: a `modal` with class `k-dialog` showing `d.body`
under the title `d.title`, and an action row `k-dialog-actions` of
`height: 1` with the buttons `d.no` and `d.yes`. `d.open` is the path the host
flips. The labels are data, not props, because one dialog serves several
operations (stop, restart, delete): the [dialog helper](#dialog) writes all of
them at the use's id on every open, so with it `PATH` is that id
(`with="confirm as d"`), and the labels are `Confirm` and `Cancel` unless the
request names its own. The use's content goes under `d.body` (a list, a
checkbox), with its paths written in full. The variant `danger` (or the flag
`d.danger`) makes a destructive dialog. The dialog takes its content's size, up
to 60 columns where the text wraps, and at most the screen. `focus` puts the
cursor on the safe button so a stray `enter` cancels; `on:escape` cancels. Keep
the labels short so the row fits 40 columns.

Ids: `confirm__dialog`, `confirm__no`, `confirm__yes` (and your trigger, such
as `confirm__open`). Actions: `dialog_confirm`, `dialog_cancel`, and your
trigger's `dialog_open`. Keys: `y` confirms and `n` cancels (the kit's keymap
`when=".k-dialog"`), `esc` cancels.

### form field (`components/field.tui`, template `k-field`)

`<use id="profile__email" template="k-field" label="Email"
placeholder="you@example.com" help="We never share it."
with="forms.profile.email as f"/>`: a `col` with class `k-field` holding the
`label` prop, an `input` with class `k-field-input` bound to `f.value` (the
`placeholder` prop), the `help` line, and the error line `f.error`. The input
has its own border and `height: 3`, which is what lets `:focus` change
`border-color`. While `f.error` is non-empty the label and the input are red and
the error line replaces the help.

A form is named by the id prefix of its fields, and its state lives under that
name: the fields `profile__name` and `profile__email` read
`forms.profile.name` and `forms.profile.email`, their inputs
(`profile__name__input`, `profile__email__input`) write `.value`, and the host
writes `forms.profile.<field>.error` ("" when fine) and `forms.profile.valid`.
The label, the placeholder and the help are props of the use, not data.

Ids: `profile__name__input`, `profile__email__input`, and your Save button
(`profile__save`). Actions: `on:change` (`form_validate`: the host checks the
value and writes the field's `.error`), `on:submit` / the Save button
(`form_submit`). Keys: type; `enter` submits; `ctrl+s`.

### empty state (`components/empty.tui`, template `k-empty`)

`<use template="k-empty" icon="◌" title="No deployments yet" text="Push a
commit." if="!items.any">`, with your button as its content: a `col` with class
`k-empty` that fills the space left and centers the `icon`, `title` and `text`
props (the icon and the text are optional), then the call to action. The
button, its id and its action are the app's:
`<button id="deploy__create" class="k-btn primary" label="New deployment"
on:click="deploy_create"/>`, so the use needs no id and the kit has no action
of its own here. Show it in place of the list it replaces with the use's `if`.
It reads no data.

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

### labeled progress (`components/meter.tui`, template `k-meter`)

`<use template="k-meter" class="ok" label="Tests" with="PATH as m"/>`: a `col`
with class `k-meter`, a head row (the `label` prop, a `spacer`, `m.text`) over a
`progress` with
class `k-meter-bar` bound to `m.pct`, a number from 0 to 100. The variant `ok`,
`warn` or `danger` colors the bar, or the flags `m.ok`, `m.warn` and
`m.danger` pick it from data. The host updates the number and the text; no
actions.

### stat / KPI card (`components/stat.tui`, template `k-stat`)

`<use template="k-stat" title="CPU" min="0" with="PATH as s"/>`: a `box` with
class `k-stat` titled by the `title` prop: `s.value` in bold, then a row with
the change `s.delta` (the flags `s.good` or `s.bad` make it green or red,
because whether a rise is good depends on the metric) and a `sparkline` bound
to `s.hist`, an array of numbers. `min` and `max` (integers) are the
sparkline's range: leave them out and the range comes from the values. No
actions.

### toast (`components/toast.tui`, template `k-toast`)

`<use template="k-toast" with="toast as t"/>`: a `k-toast-slot` row that is
always one row high, with a `k-toast` row inside shown while `t.show` is true,
so the page never jumps. It shows `t.text`; info is the default, and the flags
`t.is_ok`, `t.is_warn` and `t.is_danger` pick another level. The **host clears
it**: it sets `t.show` back to `false` from an `Every` tick (see
`examples/kit-gallery/main.go`, where a once-a-second tick counts the toast
down). The slot is not docked, so it can sit above a docked status bar.
Actions: `toast_show` (your trigger), `toast_dismiss` (`esc`, bound in your
keymap).

### command palette (`components/palette.tui`, template `k-palette`)

`<use id="palette" template="k-palette" title="Command palette"
placeholder="type a command…" with="palette as p, found as f"/>`, the last
child of the screen: a `modal` with class `k-palette` titled by the `title`
prop, an `input` (`k-palette-input`, bound to `p.query`, with the `placeholder`
prop) over a `list` (`k-palette-list`) of `f.items`, whose cursor is `p.cur`.
The markup has no expressions, so the **host filters**: it keeps `found` (the
matching commands, plus `any`) with `Derive` over `palette.query` and
`palette.commands`, and the list reads it. `focusable="false"` on the list keeps
focus in the input; while the palette is open, `up`/`down` (and
`ctrl+n`/`ctrl+p`) are not consumed by the input, so they reach a keymap
`when=".k-palette"` in your document, where `move-next`/`move-prev` with
`to="#palette__list"` move the cursor (these rows name your copy's id, so the
kit cannot hold them). Enter is the input's `on:submit`.

Ids: `palette__dialog`, `palette__q`, `palette__list`. Actions: `palette_open`
(your `ctrl+k`), `palette_close` (`esc`), `palette_run` (the host does the
command whose id is `palette.cur`, or the first match) and `palette_query` (the
input's `on:change`: a Go host that keeps `found` with `Derive` has nothing to
do for it; a TypeScript parent, which has no `Derive`, recomputes `found`
there).

### help overlay (`components/help.tui`, template `k-help`)

`<use id="help" template="k-help" title="Keys" with="help as h"/>`, the last
child of the screen: a `modal` with class `k-help` titled by the `title` prop,
open while `h.open`, holding a `? close help` line and, under it, a scroll
(`help__keys`, focusable, so the arrows move it) around
`<hints class="k-hints k-help-keys" scope="all"/>`: every labeled keymap
row, stacked. A keymap longer than the screen scrolls, and the app needs no
height rule. Ids: `help__dialog`, `help__keys`. Actions: `help_open` (your `?`),
`help_close` (`q`, `?` from the kit's keymap, and `esc`). The list holds your
keymap's labeled rows. The kit's own overlay keys have no label, so no
`<hints>` lists them, and a document that includes the kit without using an
overlay shows none of them: the help names its way out in its own `? close
help` line, and the confirm dialog's buttons name its choices (its `y` and
`n` work while it is open).

### split pane / master-detail (`components/split.tui`)

A `row` with class `k-split`: a `list` (classes `k-menu k-split-master`) and a
card (`k-card k-split-detail`). The list `bind` is the selected id; the **host
keeps `detail`** with `Derive` over the items and the cursor, so the right side
follows the cursor with no handler. Under 70 columns the panes stack. Action:
`menu_select`.

### log panel (`components/log.tui`, template `k-log`)

`<use id="log" template="k-log" with="PATH as l"/>`: a `scroll` with class
`k-log` titled `l.title`, `focusable="true"` and `stick="bottom"`, whose id is
`log__view`: it follows the end as lines are appended, until you scroll up to
read, and `move-last` makes it follow again. The lines are `l.lines`, an array
of `{n, text, debug, info, warn, err}`, in a `col` (`k-log-lines`,
`height: auto`) with `each` and a `key`; the flags color a line with the
modifiers `muted` / `info` / `warn` / `danger`. Every line must carry all the
flags (a missing path is B002). The host appends to the array (`log_append`).
Keys, in your keymap `when="#log__view"`: `j`/`k` scroll, `G` follows.

### breadcrumb (`components/breadcrumb.tui`, template `k-breadcrumb`)

`<use template="k-breadcrumb" with="PATH as b"/>`, where `PATH` is an array of
`{id, label, last}`: a `row` with class `k-breadcrumb` and `each` over it, each
crumb a `k-breadcrumb-item` (plus `current` for the last, by guard) and a
`k-breadcrumb-sep`. Under 60 columns only the current crumb shows. Display only
(a template cannot hold anything focusable); `backspace` in your keymap fires
`breadcrumb_up`, and the host trims the array.

## Helpers

Markup has no expressions, so some behavior has to live in the host: expiring a
toast, filtering the palette, validating a form, routing a dialog's answer. The
helpers are small host-side files, in Go and in TypeScript, that do those jobs
against the store paths the components read. Like the components, you **copy
them and own them**; they are not a package. `kit/go` is a nested module named
`tuimarkkit` (no dot in the name, so `go get` cannot fetch it) and `kit/ts` has
no published name.

**Copy.** In Go, copy the file (`kit/go/toasts.go`) into your project and change
`package kit` to your package; it imports only
`github.com/abdul-hamid-achik/tuimark`. In TypeScript, copy the file
(`kit/ts/toasts.ts`) next to your parent; it imports only types from
`@thelacanians/tuimark`. A file that needs another says so in its header: `dialog` and
`form` need `route`, and `palette` needs `filter`. Each file has its test beside
it (`go test ./...` in `kit/go`, `bun test` in `kit/ts`). The Go tests for
`toasts`, `form`, `dialog`, `palette` and `sortpage` load the real component
documents from `components/` and drive them with `Play` and `Dump`; the others,
and all the TypeScript tests, run against the store alone (the TypeScript ones
also check that the paths a helper writes appear in the markup of the component
that reads them). `kit/testdata/` holds the tables the Go and TypeScript
formatting and filter tests share, so both give the same strings.

**Go and TypeScript differ in two places**, because the Go API has `Derive` and
`Every` and the TypeScript host does not. A Go `filter` or `sortpage` is a
`Derive`, so it follows every store write with no handler; in TypeScript you call
`recompute()` after a write, and the palette's input fires `palette_query` on
every change so the parent can. A Go toast expires from an `Every` tick; in
TypeScript `start()` runs the tick on a timer.

### route

`Instance(source)` is everything before the last `__` of an event's `source`,
and `Part(source)` is the rest. `confirm__yes` is the part `yes` of the instance
`confirm`; an id with no `__` is its own instance. `dialog` and `form` use it.

```go
inst := kit.Instance("confirm__yes") // "confirm"; in a handler, kit.Instance(ev.Source)
```
```ts
const inst = instance("confirm__yes"); // "confirm"; in a handler, instance(ev.source)
```

### toasts

A queue behind `toast.tui`, which is one row: the first toast shows at once, the
next wait their turn, each shows for its time to live, and the queue is capped
(the oldest waiting toast is dropped). It writes `toast.show`, `.text`, `.is_ok`,
`.is_warn` and `.is_danger`, and `Register` installs `toast_dismiss` (esc).

```go
ts, _ := kit.NewToasts(app, kit.ToastOptions{Max: 5, TTL: 3 * time.Second})
ts.Register()
stop := ts.Start(time.Second)         // app.Every(time.Second, ts.Tick)
defer stop()
ts.Push("ok", "Settings saved", 0)    // kind: info, ok, warn or danger; ttl 0 = the default
ts.PushTo(batch, "warn", "92% used", 0) // inside a Batch you are already writing
```
```ts
const toasts = createToasts(ui, { max: 5, ttl: 3000 });
toasts.register();
const stop = toasts.start(500);       // setInterval(tick); stop() clears it
await toasts.push("ok", "Settings saved");
```

### filter

Keeps a filtered copy of an array in the store: `{items, any, count, total}`.
`Substring` matches every word of the query in any order; `Fuzzy` matches the
query's letters in order and puts tighter matches first. Fields are keys or
dotted paths. `FilterItems` is the pure function behind it.

```go
kit.Filter(app, kit.FilterOptions{
    Source: "palette.commands", Query: "palette.query", Out: "found",
    Fields: []string{"label", "id"}, Mode: kit.Fuzzy,
})
```
```ts
const filter = createFilter(ui, { source: "palette.commands", query: "palette.query", out: "found", fields: ["label", "id"], mode: "fuzzy" });
await filter.recompute(); // after the source or the query changed
```

### sortpage

Sort-by-column cycling and pagination for a table. It derives the sorted rows of
the current page (`Out`) and a `Meta` object: `page`, `pages`, `size`, `total`,
`label` (`1–50 of 340`), `has_prev`, `has_next`, and `titles`, where the sorted
column's title carries an arrow (`SERVICE ▲`). The table reads `Out`, its column
titles read `Meta.titles.NAME` (`title="{svc_meta.titles.name}"` on a `column`),
and a column's `on:click` calls `Cycle` with the column id's part (`svc__name`:
`name`). A column's `on:click` fires only with the mouse on (`<tui mouse="…">`);
for the keyboard, bind keymap rows to your own action (say `s` for the focused
column) that call `Cycle` too.

```go
sp, _ := kit.NewSortPage(app, kit.SortPageOptions{
    Source: "all", Out: "services", Meta: "svc_meta", Size: 50,
    Columns: []kit.Column{{Key: "name", Title: "SERVICE"}, {Key: "rps", Title: "REQ/S"}},
})
app.On("table_sort", func(ev tuimark.Event) error { return sp.Cycle(kit.Part(ev.Source)) })
sp.NextPage(); sp.SetSize(25)
```
```ts
const sp = createSortPage(ui, { source: "all", out: "services", meta: "svc_meta", columns: [{ key: "name", title: "SERVICE" }] });
ui.on("table_sort", (ev) => sp.cycle(part(ev.source)));
await sp.recompute(); // after "all" changes
```

### form

Per-field rules for the `k-field` template (`field.tui`). A form is named by the
id prefix of its fields (`<use id="profile__name" template="k-field">`, whose
input `profile__name__input` fires the form's actions), and writes
`forms.profile.<field>.error` and `forms.profile.valid`; the inputs bind
`forms.profile.<field>.value`. Rules are
functions from the value to a message (`""` is fine): `Required`, `Pattern`,
`MinLen`, `MaxLen`, or your own. `form_validate` checks the field that changed,
and does not turn red a field the user has not reached; `form_submit` checks
every field, focuses the first invalid input, and otherwise calls your
`OnSubmit`. `Reset` writes only each field's `forms.profile.<field>.value` and
`.error` and the form's `forms.profile.valid`; a field's `label`,
`placeholder` and `help` are props of its `<use>`, not data.

```go
forms := kit.NewForms(app)
forms.Add("profile").
    Field("name", kit.Required("Name is required.")).
    Field("email", kit.Required("Email is required."), kit.Pattern(emailRE, "Enter a full address.")).
    OnSubmit(func(v map[string]string) error { return save(v["name"], v["email"]) }).
    Reset(map[string]string{"name": "Ada"}) // writes .value, .error and valid
forms.Register()                           // form_validate, form_submit
```
```ts
const forms = createForms(ui);
const profile = forms.add("profile")
  .field("name", required("Name is required."))
  .field("email", required("Email is required."), pattern(emailRE, "Enter a full address."))
  .onSubmit((v) => save(v.name ?? "", v.email ?? ""));
await profile.reset({ name: "Ada" });
forms.register();
```

### dialog

`Confirm(inst, title, body, onConfirm)` opens a `k-confirm` copy (it writes
`<inst>.title`, `.body`, the button labels `.yes` and `.no`, `.open` and
`.result`) and routes `dialog_confirm` and `dialog_cancel` by instance, so one
pair of handlers serves every dialog. The result is `confirmed` or
`cancelled`. `Open` takes a `Request` with an `OnCancel` too, and its own
labels (`Yes` and `No` in Go, `yes` and `no` in TypeScript). An empty label
means `Confirm` or `Cancel`: both labels are written on every open, so the
labels of an earlier question never stay on the buttons. You open the dialog
from your own action (the trigger button's).

The helper writes a dialog's data at the path named by its instance, and finds
the instance of an answer as the part of the event's source before the last
`__` (`confirm__yes` is the instance `confirm`), which is the `<use>`'s id. So
the use's `with` path is its own id: `<use id="confirm" template="k-confirm"
with="confirm as d"/>`. With `with="dialogs.confirm as d"` the helper would
write `confirm.open` while the copy reads `dialogs.confirm.open`.

```go
d := kit.NewDialogs(app)
d.Register() // dialog_confirm, dialog_cancel
app.On("dialog_open", func(ev tuimark.Event) error {
    return d.Open(kit.Instance(ev.Source), kit.Request{ // confirm__open: confirm
        Title: "Delete project", Body: "This cannot be undone.",
        Yes: "Delete", OnConfirm: deleteProject,
    })
})
```
```ts
const dialogs = createDialogs(ui);
dialogs.register();
ui.on("dialog_open", (ev) =>
  dialogs.open(instance(ev.source), {
    title: "Delete project", body: "This cannot be undone.",
    yes: "Delete", onConfirm: () => deleteProject(),
  }));
```

Its labels are the ones `components/confirm.sample.json` holds: the buttons
read `Cancel` and `Delete`.

### palette

A command registry behind `palette.tui`: commands have an id, a title, a key hint
and a function. It writes `palette.commands`, keeps `found` with `filter`, and
implements `palette_open` (resets the query and cursor), `palette_close`,
`palette_run` (runs the command under the cursor, or the first match, and writes
`palette.last`) and `palette_query`. When the store has none of them yet, it
also writes `palette.open`, `.query`, `.cur` and `.last`, so what it writes is
all the palette needs. The title and the placeholder are props of the use, not
data: it writes neither.

```go
p, _ := kit.NewPalette(app, kit.PaletteOptions{Mode: kit.Fuzzy},
    kit.Command{ID: "deploy", Title: "Deploy to production", Key: "d", Run: deploy},
    kit.Command{ID: "logs", Title: "Open the logs", Key: "l", Run: openLogs})
p.Register()
```
```ts
const palette = await createPalette(ui, { mode: "fuzzy" },
  { id: "deploy", title: "Deploy to production", key: "d", run: deploy },
  { id: "logs", title: "Open the logs", key: "l", run: openLogs });
palette.register();
```

### async

`Load(path, fn)` runs slow work off the handler (a handler's return is the
event's reply, and a slow one delays every key) and keeps `<path>.loading`,
`<path>.error` and `<path>.data`. A later load of the same path cancels the
earlier one and drops its result; `data` is kept while a reload runs. A task
can be canceled, which puts `loading` back to false. A value the store refuses
(NaN, a channel) ends the load with the reason in `error`; the TypeScript
`createLoader` takes an `onError` for failures of the final write.

```go
loader := kit.NewLoader(app)
loader.Load("repos", func(ctx context.Context) (any, error) { return fetchRepos(ctx) })
```
```ts
const loader = createLoader(ui);
await loader.load("repos", (signal) => fetchRepos(signal));
```

### format

Strings ready for `{path}`: `Bytes` (`1.5 MiB`), `Duration` (`2m 5s`),
`Relative` (`5 minutes ago`, `in 2 days`), `Percent` (`26%`) and `Thousands`
(`1,234,567`). TypeScript spells them `bytes`, `duration` (milliseconds),
`relative`, `percent` and `thousands`.

```go
app.Set("disk.text", kit.Bytes(used)+" of "+kit.Bytes(total))
```
```ts
await ui.set("disk.text", `${bytes(used)} of ${bytes(total)}`);
```

---

## Next

These are plans, not promises.

- **Helpers as Tuimark Script.** The helpers above are host code in Go and
  TypeScript. When the language gains scripting, the same API (`on`, `set`,
  `derive`, `every`) can ship as scripts.
- **`tuimark add`.** A command that copies a component, with the sheets it
  needs, into your project.
