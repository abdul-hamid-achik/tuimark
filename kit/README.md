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
class names.

## Using it

Copy `theme.tcss` and `kit.tcss` into your project, load them first, and put
your own sheet last:

```xml
<tui version="3" theme="auto">
  <style src="theme.tcss"/>
  <style src="kit.tcss"/>
  <style src="app.tcss"/>   <!-- yours: overrides and app-specific sizes -->
  ...
```

There is no `@import` in TCSS, so every document lists the sheets it uses. The
sheets are cascaded together; where two rules tie, the later one wins. (On a
0.4.0 build `version="3"` is required on `<tui>`; from 0.4.1 it is optional and
the attribute can be dropped.)

Then copy the markup of the component you want from `components/NAME.tui`.
Its comments say what to keep and what to change.

### Tokens

All color lives in `:root` of `theme.tcss`, read as `$name`. Edit the values to
retheme the whole kit; `@media (theme: light)` holds the light values.

| Token | Use |
|---|---|
| `bg` | the screen |
| `surface` | cards, panels, modals, inputs |
| `surface-2` | raised or inset things on a surface: buttons, `kbd`, neutral badges, bars |
| `selected` | the cursor row of a list or table |
| `fg` | body text |
| `muted` | secondary text and labels |
| `border` | idle borders |
| `accent` | the brand color: primary actions, the active tab, focus titles |
| `accent-fg` | text drawn on `accent`, `success`, `warning`, `danger`, `info` |
| `success` `warning` `danger` `info` | status colors |
| `focus` | the border or fill of the focused thing |

Typography classes: `.title`, `.subtitle`, `.muted`, `.strong`, `.kbd`,
`.code`, and the colors `.text-accent`, `.text-success`, `.text-warning`,
`.text-danger`, `.text-info`. `button:disabled` and `input:disabled` are dimmed.

### Conventions

- **Root class plus prefixed parts.** TCSS has no descendant combinator (only
  `>`), so a component cannot style "text inside a card". Every part carries
  its own class, named after the root: `.card`, `.card-title`, `.card-text`.
- **Variants are a second class** on the same node: `badge success`,
  `btn danger`, `alert warning`. A compound selector outranks a single class, so
  the variant wins wherever it sits in the file. Choose a variant from data
  with `class:NAME="path"` guards.
- **Sizes are explicit.** A component that must fill a space says `width: 1fr`
  or `height: 1fr` in its class, so it does not depend on what a bare `row` or
  `col` defaults to. Text that must shrink to its content is `width: auto`.
- **Focus is shown with color.** A node can only change `border-color` if it
  has its own `border`, so inputs and panels that show focus set one; buttons
  and menus show it with a fill.
- **Presentational attributes lose to author CSS.** A `height="9"` attribute
  does not beat `.dtable { height: 1fr }`. To size a component in your app, add a
  rule to your own sheet (`#svc { height: 9; }`) or use `style="height: 9"`.
- **Narrow terminals.** `tuimark validate` checks 40, 80 and 120 columns, and
  every kit document is clean there. Where a component would not fit it
  degrades: `@media (max-cols: 60)` hides the least important part (the app
  bar's meta, a status bar's muted segment, a breadcrumb's older crumbs), and
  `@media (max-cols: 70/90)` stacks the split pane and `cols` rows.
- **Modals are the last children of the screen** (L004), and a container has
  at most one bottom-docked child (L005).

### Layout helpers

`kit.tcss` also has three layout classes the gallery uses: `.page` (a `scroll`
that fills a tab; its child `.page-body` is a `col` as tall as its content),
`.cols` (a row of panels that stacks into a column under 90 columns), and
`.section` (a small heading).

---

## Components

Each section: what it is, the classes, the actions it names (the host
implements them), and the keys. The full markup is in the named file.

### badge (`components/badge.tui`)

A short status label. One `text` with `badge` and a variant: `accent`,
`success`, `warning`, `danger`, `info` (none is neutral).

```xml
<text class="badge success">passing</text>
<text class="badge" class:success="build.ok" class:danger="!build.ok">{build.status}</text>
```

No actions, no keys.

### button (`components/button.tui`)

`button` with class `btn` and a variant: `primary`, `secondary`, `danger`,
`ghost`. The focused button is drawn inverted and underlined, and no variant
shares its focus look with its idle look. A `disabled` button is dimmed. The
label is literal text (a `{path}` in `label` is V003).

```xml
<button id="save" class="btn primary" label="Save" on:click="save"/>
```

Action: whatever `on:click` names. Keys: `enter` / `space` press the focused
button, `tab` / `shift+tab` move focus.

### card / panel (`components/card.tui`)

A `box` with class `card` and a `title`. Parts: `card-title`, `card-text`,
`card-muted`, `card-actions` (a one-row button row). The border takes the focus
color while anything inside holds focus (`:focus-within`). Side-by-side cards:
a `row` of cards, each `1fr`.

```xml
<box class="card" title="Deploy">
  <text class="card-title">api-gateway 2.14.0</text>
  <row class="card-actions"><button .../></row>
</box>
```

### alert / callout (`components/alert.tui`)

A `box` with class `alert` and a variant (`info`, `success`, `warning`,
`danger`), the title as the box title, and a row with a glyph (`alert-icon`
plus the same variant) and the wrapping `alert-text`.

### app bar (`components/appbar.tui`)

A one-row `row` with class `appbar`, docked to the top of its parent
(`dock: top`) so the content below keeps all the remaining space. Parts:
`appbar-brand`, `appbar-title`, `appbar-meta` (hidden under 60 columns), then
any badge. Put one top-docked bar in a container.

### status bar (`components/statusbar.tui`)

A one-row `row` with class `statusbar`, docked to the bottom. Left segments, a
`spacer`, right segments. Parts: `statusbar-seg` (a filled chip; variants
`success`, `warning`, `danger`), `statusbar-text`, `statusbar-muted` (hidden
under 60 columns). One bottom-docked bar per container (L005): put the key hints
and the toast above it, not docked.

### key-hint bar (`components/hintbar.tui`)

`<hints class="hintbar"/>`. Every keymap row with a `label` is a hint: the key
(or its `keycap`) in the accent color, the label muted. `scope="active"` (the
default) shows only the keys that would fire now; hints that do not fit drop
off the right. A built-in like `move-next` is hidden while its target does not
exist, so bind host actions you want listed unconditionally.

### confirm dialog (`components/confirm.tui`)

A `modal` with class `dialog` (add `danger` for a destructive one), a
`dialog-text`, and an action row `dialog-actions` of `height: 1`. It must be the
last child of the screen. `open` is a path the host flips; `focus` puts the
cursor on the safe button so a stray `enter` cancels; `on:escape` cancels.
Keep the labels short so the row fits 40 columns.

Actions: `confirm_open` (the trigger), `confirm_accept`, `confirm_cancel`. The
host sets `confirm.open` to `true` or `false` and does the work on accept. Keys:
`y` accepts and `n` cancels (a `<keymap when="#confirm">`), `esc` cancels.

### form field (`components/field.tui`)

A `col` with class `field`: `field-label`, an `input` with class `field-input`,
`field-help`, `field-error`. The input has its own border and `height: 3`, which
is what lets `:focus` change `border-color`. `class:invalid="form.x_error"` on
the label and the input turns them red while the path is non-empty; the help
line shows `if="!form.x_error"` and the error `if="form.x_error"`.

Actions: `on:change` (`form_validate`: the host checks the value and writes
`form.x_error`), `on:submit` / the Save button (`form_submit`). Keys: type;
`enter` submits; `ctrl+s` can be bound as in the file.

### empty state (`components/empty.tui`)

A `col` with class `empty` that fills the space left and centers: `empty-icon`,
`empty-title`, `empty-text`, then the call to action. Show it next to the list
it replaces with `if="!items.any"`. Action: the button's `on:click`.

### data table (`components/table.tui`)

A `table` with class `dtable` and `column` children. The header is bold and
muted; the cursor row uses `$selected`, stronger while the table has focus. The
class `num` on a `column` right-aligns its header and cells. `priority` hides
columns lowest first when the table is too narrow, and a `min-width` (class
`cell-name`) on the `1fr` column makes columns hide before it is squeezed.
`cell-muted`, `cell-success`, `cell-warning`, `cell-danger` color a column;
guard them with `class:NAME` on the column.

Actions: `on:select` (`svc_select`, fired as the cursor moves, `keys` holds the
row key under the alias) and a keymap `enter` for `svc_open`. `bind` receives
the cursor key. Keys: `up`/`down`/`home`/`end`/`pgup`/`pgdn` are the table's;
`j`/`k` via `move-next`/`move-prev`.

### menu / nav list (`components/menu.tui`)

A `list` with class `menu`; each item is a `menu-row` of `menu-icon`,
`menu-label`, `menu-key`. `checked` + `mark="✓"` add multi-select marks. The
cursor row is a quiet highlight until the menu has focus, then a solid accent
bar. Actions: `on:select`, `on:change` (the new checked array), and the keymap
`enter` for `menu_open`. Keys: `space` toggles the mark (`check-toggle`).

### tabs (`components/tabs.tui`)

A `tabs` with class `tabs`, `bind` to the active tab's id, and `tab` children
(each needs an `id` and a `label`; `short` is the label when the strip does not
fit). The generated labels are styled by `.tab-label` and
`.tab-label:selected`, which apply to every tabs in the document. Only the
active tab's content is laid out, so validate it by switching the bound path.
Keys: `left`/`right` on a `focusable` strip, `]`/`[` (`move-next`/`move-prev`
`to="#nav"`), digits (`switch-to`). Optional action: `on:select`.

### labeled progress (`components/meter.tui`)

A `col` with class `meter`: a `meter-head` row (`meter-label`, `spacer`,
`meter-value`) over a `progress` with class `meter-bar`. `bind` is a number from
0 to 100. Add `success`, `warning` or `danger` to the bar, or guard them from
data. The host updates the number and the text; no actions.

### stat / KPI card (`components/stat.tui`)

A `box` with class `stat` (the label is the box title): `stat-value` in bold,
then a `stat-row` with `stat-delta` (`good` or `bad` for green or red, chosen
by `class:good`/`class:bad` because whether a rise is good depends on the
metric) and a `sparkline` with class `stat-spark` bound to an array of numbers.
No actions.

### toast (`components/toast.tui`)

A `toast-slot` row that is always one row high, with a `toast` row inside shown
by `if="toast.show"`, so the page never jumps. Info is the default; guard
`success`, `warning`, `danger`. The **host clears it**: it sets `toast.show` back
to `false` from an `Every` tick (see `examples/kit-gallery/main.go`, where a
once-a-second tick counts the toast down). The slot is not docked, so it can sit
above a docked status bar. Actions: `toast_show` (the trigger), `toast_dismiss`
(`esc`).

### command palette (`components/palette.tui`)

A `modal` with class `palette`: an `input` (`palette-input`, bound to the query)
over a `list` (`palette-list`) of the filtered commands. The markup has no
expressions, so the **host filters**: it keeps `found` (the matching commands,
plus `any`) with `Derive` over `palette.query` and `palette.commands`, and the
list reads it. `focusable="false"` on the list keeps focus in the input; while
the palette is open, `up`/`down` (and `ctrl+n`/`ctrl+p`) are not consumed by the
input, so they reach the keymap, where `move-next`/`move-prev` with
`to="#palette-list"` move the cursor. Enter is the input's `on:submit`.

Actions: `palette_open` (`ctrl+k`), `palette_close` (`esc`), `palette_run` (the
host does the command whose id is `palette.cur`, or the first match).

### help overlay (`components/help.tui`)

A `modal` with class `help` holding `<hints class="help-keys" scope="all"/>`:
every labeled keymap row, stacked. Actions: `help_open` (`?`), `help_close`
(`q`, `?`, `esc`).

### split pane / master-detail (`components/split.tui`)

A `row` with class `split`: a `list` (classes `menu split-master`) and a card
(`card split-detail`). The list `bind` is the selected id; the **host keeps
`detail`** with `Derive` over the items and the cursor, so the right side
follows the cursor with no handler. Under 70 columns the panes stack. Action:
`on:select`.

### log panel (`components/log.tui`)

A `scroll` with class `logpanel`, an `id`, `focusable="true"` and
`stick="bottom"`: it follows the end as lines are appended, until you scroll up
to read, and `move-last` makes it follow again. The lines are a `col`
(`log-lines`, `height: auto`) with `each` and a `key`; color a line with
`class:muted` / `info` / `warning` / `danger`. Every line must carry all the
flags (a missing path is B002). The host appends to the array (`log_append`).
Keys: `j`/`k` scroll, `G` follows.

### breadcrumb (`components/breadcrumb.tui`)

A `row` with class `crumbs` and `each` over `{id, label, last}`; its children
are the template: a `crumb` (plus `current` for the last, by guard) and a
`crumb-sep`. Under 60 columns only the current crumb shows. Display only (a
template cannot hold anything focusable); `backspace` can fire `crumb_up`, and
the host trims the array.

---

## What changes with `<template>` and `<use>` (0.4b)

Today a component is markup you copy, and the repetition shows: the gallery
spells out the same `meter`, `stat`, `field` and `alert` blocks several times.
Composition will turn each component into a **fragment**: a `<template>` that
takes parameters, written once, and a one-line `<use>` where you need it.

What stays: the class vocabulary and both stylesheets are unchanged, because
the fragments are built from the same classes. What moves: the markup under
`components/` becomes the template definitions, and `<!-- COPY -->` blocks
become `<use name="field" .../>`. The things the kit works around today (the
button label that cannot be a `{path}`, the unrepeatable ids in an `each`
template) are the first candidates for parameters. Until then, copy.
