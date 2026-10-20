# Tuimark kit (v1)

A copy-and-paste component kit for Tuimark, in the spirit of shadcn: you do not
install it, you copy it into your project and own it.

- `kit.tui`: the kit as one `<fragment>`. It loads the two sheets below, holds
  the overlays' keys, and has one `<template>` per component that takes data
  (alert, stat, meter, empty state, form field, breadcrumb, log, toast, confirm
  dialog, command palette, help). Include it once and copy a component with
  `<use>`.
- `theme.tcss`: the design tokens (dark and light) and the base typography.
- `kit.tcss`: the component classes.
- `components/NAME.tui`: one small, valid document per component. For a
  template it shows the `<use>`; for a class component (badge, button, card,
  app bar, status bar, key hints, data table, menu, tabs, split pane) it holds
  the markup to copy. A `NAME.sample.json` sits beside it when it binds data.
  Each includes `../kit.tui`, so it validates and dumps on its own:

  ```sh
  tuimark validate kit/kit.tui      # ok (fragment checked on its own)
  tuimark validate kit/components/table.tui --data kit/components/table.sample.json
  tuimark dump kit/components/table.tui --data kit/components/table.sample.json --cols 80 --rows 24 --format json
  tuimark run kit/components/confirm.tui --data kit/components/confirm.sample.json
  ```

- `examples/kit-gallery`: every component in one app, on five tabs, with a Go
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
      <use template="k-alert" class="warn" with="quota as a"/>
    </col>
    <use id="confirm" template="k-confirm" class="danger" with="confirm as d"/>
  </screen>
</tui>
```

The sheets are cascaded together; where two rules tie, the later one wins, so a
token re-declared in your sheet rebrands the whole kit. A `<tui>` needs no
`version` attribute.

**Templates.** `<use template="k-NAME" with="PATH as x"/>` copies a component
where it stands:

- **Data** comes through the `with` alias: every label, title, and value a copy
  shows is a path under it, a button's label included. A copy reads `x.title`
  as `PATH.title`, and a widget that writes (a field's input, the palette's
  cursor) writes under `PATH`. A template that reads two objects takes two
  aliases (`with="palette as p, found as f"`).
- **Flags** (the paths a template reads in `if`, `open`, and `class:NAME`
  guards) are optional members of the alias: a missing one reads as false.
  The elements of an array the template repeats (a log's lines) still carry
  every flag (B002 otherwise).
- **A variant** is a static `class` on the `<use>` (`class="danger"`). It lands
  on the copy's root, and the kit's rules style the parts from there
  (`.k-alert.danger > .k-alert-row > .k-alert-icon`).
- **Ids** come from the use: a template that holds ids needs an `id` on its
  `<use>`, which prefixes them. `<use id="confirm" template="k-confirm">` makes
  `confirm__dialog`, `confirm__no` and `confirm__yes`.
- **Sizes**: every template's root writes its own size, so a copy is the same
  size in a screen, a tab, or a `col`.

**Classes.** A component whose content is yours (a card's body, a table's
columns, a menu's rows, a row of buttons) is not a template, since a template
has no slots: copy its markup from `components/NAME.tui` and keep its classes.

Each component file starts with a **contract**, an XML comment that says what
the component needs from the store and the host:

```
k-NAME · Contract: <the with alias and the paths it reads and writes> · Actions: <named actions> · Keys: <keys> · Requires: <other kit classes>
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
`current`, `muted`). It is only ever written next to the component class
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
  rule to your own sheet (`#svc { height: 9; }`).
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
resolved every frame (`label="{e.action}"` in the empty state's template).

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

### alert / callout (`components/alert.tui`, template `k-alert`)

`<use template="k-alert" class="warn" with="PATH as a"/>`: a `box` with class
`k-alert`, `a.title` as the box title, and a row with the glyph `a.icon`
(optional: no icon, no glyph) and the wrapping `a.text`. The variant (`info`,
`ok`, `warn`, `danger`) colors the frame and the glyph.

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

### confirm dialog (`components/confirm.tui`, template `k-confirm`)

`<use id="confirm" template="k-confirm" class="danger" with="PATH as d"/>`, the
last child of the screen: a `modal` with class `k-dialog` showing `d.body`
under the title `d.title`, and an action row `k-dialog-actions` of
`height: 1` with the buttons `d.no` and `d.yes`. `d.open` is the path the host
flips. The variant `danger` (or the flag `d.danger`) makes a destructive
dialog. The dialog takes its content's size, up to 60 columns where the text
wraps, and at most the screen. `focus` puts the cursor on the safe button so a
stray `enter` cancels; `on:escape` cancels. Keep the labels short so the row
fits 40 columns.

Ids: `confirm__dialog`, `confirm__no`, `confirm__yes` (and your trigger, such
as `confirm__open`). Actions: `dialog_confirm`, `dialog_cancel`, and your
trigger's `dialog_open`. Keys: `y` confirms and `n` cancels (the kit's keymap
`when=".k-dialog"`), `esc` cancels.

### form field (`components/field.tui`, template `k-field`)

`<use id="profile__email" template="k-field" with="forms.profile.email as f"/>`:
a `col` with class `k-field` holding the label `f.label`, an `input` with
class `k-field-input` bound to `f.value` (placeholder `f.placeholder`), the
help line `f.help`, and the error line `f.error`. The input has its own border
and `height: 3`, which is what lets `:focus` change `border-color`. While
`f.error` is non-empty the label and the input are red and the error line
replaces the help.

A form is named by the id prefix of its fields, and its state lives under that
name: the fields `profile__name` and `profile__email` read
`forms.profile.name` and `forms.profile.email`, their inputs
(`profile__name__input`, `profile__email__input`) write `.value`, and the host
writes `forms.profile.<field>.error` ("" when fine) and `forms.profile.valid`.

Ids: `profile__name__input`, `profile__email__input`, and your Save button
(`profile__save`). Actions: `on:change` (`form_validate`: the host checks the
value and writes the field's `.error`), `on:submit` / the Save button
(`form_submit`). Keys: type; `enter` submits; `ctrl+s`.

### empty state (`components/empty.tui`, template `k-empty`)

`<use id="empty" template="k-empty" with="PATH as e" if="!items.any"/>`: a
`col` with class `k-empty` that fills the space left and centers `e.icon`,
`e.title`, `e.text`, then a call to action labeled `e.action` (none when
`e.action` is absent). Show it in place of the list it replaces with the use's
`if`. Action: the button's `empty_create` (source `empty__create`).

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

`<use template="k-meter" class="ok" with="PATH as m"/>`: a `col` with class
`k-meter`, a head row (`m.label`, a `spacer`, `m.text`) over a `progress` with
class `k-meter-bar` bound to `m.pct`, a number from 0 to 100. The variant `ok`,
`warn` or `danger` colors the bar, or the flags `m.ok`, `m.warn` and
`m.danger` pick it from data. The host updates the number and the text; no
actions.

### stat / KPI card (`components/stat.tui`, template `k-stat`)

`<use template="k-stat" with="PATH as s"/>`: a `box` with class `k-stat` titled
`s.title`: `s.value` in bold, then a row with the change `s.delta` (the flags
`s.good` or `s.bad` make it green or red, because whether a rise is good
depends on the metric) and a `sparkline` bound to `s.hist`, an array of
numbers. No actions.

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

`<use id="palette" template="k-palette" with="palette as p, found as f"/>`, the
last child of the screen: a `modal` with class `k-palette` titled `p.title`,
an `input` (`k-palette-input`, bound to `p.query`) over a `list`
(`k-palette-list`) of `f.items`, whose cursor is `p.cur`. The markup has no
expressions, so the **host filters**: it keeps `found` (the matching commands,
plus `any`) with `Derive` over `palette.query` and `palette.commands`, and the
list reads it. `focusable="false"` on the list keeps focus in the input; while
the palette is open, `up`/`down` (and `ctrl+n`/`ctrl+p`) are not consumed by
the input, so they reach a keymap `when=".k-palette"` in your document, where
`move-next`/`move-prev` with `to="#palette__list"` move the cursor (these rows
name your copy's id, so the kit cannot hold them). Enter is the input's
`on:submit`.

Ids: `palette__dialog`, `palette__q`, `palette__list`. Actions: `palette_open`
(your `ctrl+k`), `palette_close` (`esc`), `palette_run` (the host does the
command whose id is `palette.cur`, or the first match) and `palette_query` (the
input's `on:change`: a Go host that keeps `found` with `Derive` has nothing to
do for it; a TypeScript parent, which has no `Derive`, recomputes `found`
there).

### help overlay (`components/help.tui`, template `k-help`)

`<use id="help" template="k-help" with="help as h"/>`, the last child of the
screen: a `modal` with class `k-help` titled `h.title`, open while `h.open`,
holding `<hints class="k-hints k-help-keys" scope="all"/>`: every labeled
keymap row, stacked. Id: `help__dialog`. Actions: `help_open` (your `?`),
`help_close` (`q`, `?` from the kit's keymap, and `esc`). The list starts
with the kit's own labeled row, the help's `? close help`, then your keymap's
labeled rows. The confirm dialog's `y` and `n` are not in it: the dialog's
buttons name its choices, and the keys work while it is open.

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
`OnSubmit`.

```go
forms := kit.NewForms(app)
forms.Add("profile").
    Field("name", kit.Required("Name is required.")).
    Field("email", kit.Required("Email is required."), kit.Pattern(emailRE, "Enter a full address.")).
    OnSubmit(func(v map[string]string) error { return save(v["name"], v["email"]) }).
    Reset(map[string]string{"name": "Ada"}) // creates the paths
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
labels (`Yes` and `No` in Go, `yes` and `no` in TypeScript; `Confirm` and
`Cancel` when empty). You open the dialog from your own action (the trigger
button's).

```go
d := kit.NewDialogs(app)
d.Register() // dialog_confirm, dialog_cancel
app.On("dialog_open", func(ev tuimark.Event) error {
    return d.Confirm(kit.Instance(ev.Source), "Delete project", "This cannot be undone.", func() error { return deleteProject() })
})
```
```ts
const dialogs = createDialogs(ui);
dialogs.register();
ui.on("dialog_open", (ev) => dialogs.confirm(instance(ev.source), "Delete project", "This cannot be undone.", () => deleteProject()));
```

### palette

A command registry behind `palette.tui`: commands have an id, a title, a key hint
and a function. It writes `palette.commands`, keeps `found` with `filter`, and
implements `palette_open` (resets the query and cursor), `palette_close`,
`palette_run` (runs the command under the cursor, or the first match, and writes
`palette.last`) and `palette_query`.

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

- **Fragments.** Today a component is markup you copy, and the repetition shows:
  the gallery spells out the same `k-meter`, `k-stat`, `k-field` and `k-alert`
  blocks several times. When the language gains composition (a `template` and
  a `use` with an `id`), each component can become a fragment written once. The
  conventions above are chosen for that: a `use id="confirm"` would generate the
  ids `confirm__dialog`, `confirm__yes`, `confirm__no`, so hosts written today
  would not change. The class vocabulary and both stylesheets stay as they are.
- **Helpers as Tuimark Script.** The helpers above are host code in Go and
  TypeScript. When the language gains scripting, the same API (`on`, `set`,
  `derive`, `every`) can ship as scripts.
- **`tuimark add`.** A command that copies a component, with the sheets it
  needs, into your project.
