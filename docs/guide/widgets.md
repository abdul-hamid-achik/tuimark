---
title: Widgets
description: Tuimark's built-in widgets - list, input, button, progress, sparkline, table, tabs, hints and modal - with rendered examples, their events and the keys they take.
---

# Widgets

Tuimark has a small, fixed set of widgets. Each one below is shown with a document you can copy and the frame it renders; the pictures come from `tuimark play` on exactly these files. All of them share one `data.json`:

::: details data.json

<<< @/snippets/widgets/data.json{json}

:::

## list

A focusable list with a cursor. Its rows come from `each` over an array, or from static `<item>` children.

<<< @/snippets/widgets/list.tui{tui}

<TermShot name="w-list" />

- `bind` holds the cursor row's key; `on:select` fires when the cursor moves. A focused list takes `up`, `down`, `home`, `end`, `pgup`, and `pgdn`.
- The cursor row is `:selected`, reversed by the built-in sheet (`list > item:selected { reverse: true; }`).
- **Multi-select** (version 2): `checked="path"` holds the keys of the checked rows, an array the runtime reads and writes; `mark="✓"` reserves room for the mark at the start of every row; `on:change` fires with the new array. Checked rows match `:checked`. Bind the keys yourself with the `check-toggle`, `check-all`, and `check-none` built-ins.
- Rows do not take focus: a row cannot hold an `input`, `button`, `list`, or `modal`. Act on the cursor row with `on:select`, or with a keymap row scoped to the list, whose event names the row.
- Without a size, a list fits its rows into the space left and scrolls ([auto-fit](/guide/layout#auto-fit)); `scrollbar: auto` shows where you are.

## input

A single-line text field. `bind` holds its text.

<<< @/snippets/widgets/input.tui{tui}

<TermShot name="w-input" />

- `placeholder` shows, dimmed, while it is empty. `secret="true"` paints one `•` per character, and keeps the text out of `TUIMARK_LOG`.
- `on:change` fires with the new text as the event's value; `on:submit` fires on `enter`.
- A focused input takes printable keys, `space`, `backspace`, `left`, `right`, `home`, `end`, `ctrl+a`, `ctrl+e`, `ctrl+h`, and `ctrl+u` before the keymap sees them, so typing never triggers shortcuts. A paste arrives as one edit and one `on:change`.
- An input is one row of text. With a border it is three rows tall, as above; a `height: 1` would leave the border no room.

## button

```tui
<button id="deploy" label="Deploy" on:click="ask_deploy"/>
<button id="approve" on:click="approve">Approve</button>
```

The label comes from `label` or the body, painted as `[ Deploy ]`. A focused button fires `on:click` on `enter` or `space`, and on a mouse click. Show focus with `button:focus { reverse: true; }`.

## progress and sparkline

A `progress` is a bar from 0 to 100, from `bind` or a literal `value`. A `sparkline` (version 2) draws the last values of a numeric array as bars, one column per value and eight levels per row.

<<< @/snippets/widgets/progress.tui{tui}

<TermShot name="w-progress" />

- `bar: eighths` (version 2) fills eighths of a cell for a smoother bar; `bar: block`, the default, fills whole cells.
- A sparkline is naturally one column per value and one row tall; give it a `height` for more levels. Values are right-aligned in the width it gets, `null` is a gap, and `min` and `max` fix the range (otherwise the shown values set it).
- The bar color is the widget's `color`.

## table

Rows of an array, in columns, with a cursor (version 2). Each `<column>` has a header `title` and a body that is the cell template.

<<< @/snippets/widgets/table.tui{tui}

<TermShot name="w-table" />

- It keeps a cursor like a list: `bind` holds the cursor row's key, `on:select` fires with the row in `keys`, and a focused table takes the arrow and paging keys. `checked`, `mark`, and the `check-*` built-ins work as on a list.
- **Column widths**: a number of cells; a `%` of the table; an `fr` share of what is left, clamped by `min-width` and `max-width`; or, by default, the widest of the title and every cell.
- **Styling**: a rule on a column styles its header cell. Body cells carry the column's `class` and its `class:NAME` guards, evaluated per row, so `.num { content-align: end; }` right-aligns a whole column and `#procs > item > text.hot` colors only the hot cells. One attribute-only `<item class:blocked="p.protected"/>` styles whole rows.
- Only the rows in view are laid out, painted, and dumped, so a table of thousands of rows costs what its visible rows cost.
- Cells never wrap. `placeholder` shows on the first row when the array is empty.
- `display: none` on a column hides it. Under `@media`, that is how a table drops columns on a narrow terminal. The same document at 30 columns:

<<< @/snippets/out/w-table-narrow.txt{grid}

A `column` with `on:click` fires when its header is clicked, the natural place for sorting.

## tabs

A strip of tab labels over the content of the active tab (version 2).

<<< @/snippets/widgets/tabs.tui{tui}

<TermShot name="w-tabs" />

- With `bind`, the active tab is the one whose id is the bound string, and activating a tab writes its id there; `on:select` fires with the id as the value. Without `bind`, the runtime remembers the tab.
- Tabs switch with the `switch-to`, `move-next`, and `move-prev` built-ins, with `left` and `right` on a `focusable="true"` tabs, with a click on a label, and when focus moves into an inactive tab.
- Only the active tab is laid out; widgets in the others keep their state (a list's cursor, an input's text).
- When a tab becomes active while focus was in the old tab, on the strip, or nowhere, focus moves to the new tab's `focus="#id"` target, or its first focusable node.
- **Labels always fit.** Every `label` when there is room, else every `short`, else only the active tab as `‹ label ›`. The same strip at 24 and 16 columns:

<div class="side-by-side">

<<< @/snippets/out/w-tabs-narrow.txt{grid}

<<< @/snippets/out/w-tabs-tiny.txt{grid}

</div>

Labels are generated `text` nodes with the class `tab-label`, `:selected` on the active one; style them with `.tab-label`.

## hints

Key hints generated from the keymap (version 2). Every `<bind>` row with a `label` is a hint; `keycap` replaces the key text shown.

<<< @/snippets/widgets/hints.tui{tui}

<TermShot name="w-hints" />

By default (`scope="active"`), a hint shows only while one of its keys would actually fire, through the same key handling the app uses. Press `/` and focus moves to the input, which takes `q`, `/`, `space`, and `ctrl+a` for itself, so only the hint that still applies is left:

<<< @/snippets/out/w-hints-filter.txt{grid}

`scope="all"` lists every hint, for a help screen. Items are laid out along the row (or down it with `layout: column`), and the ones that do not fit are dropped from the end. Style them with `.hint-key` and `.hint-label`.

## modal

A layer over the screen, open while `open` is truthy.

<<< @/snippets/widgets/modal.tui{tui}

<TermShot name="w-modal" />

- A modal is centered, 80% by 80% of the screen unless sized, with a single border by default and its `title` on it.
- It traps focus while open, gives it back when it closes, and fires `on:open`, `on:close`, and, on `esc`, `on:escape`.
- A modal must be the last child of its screen (`L004`). Several can be open; the last one in the document is on top.
- Give the button row `height: 1`. Rows stretch across the modal by default, and a focused `reverse` button would paint the whole stretched rect.

## scroll, rule, spacer

```tui
<scroll id="log" axis="y" focusable="true" title="log">
  <text wrap="wrap">{log}</text>
</scroll>
<rule/>
<row>
  <text>left</text>
  <spacer/>
  <text>right</text>
</row>
```

- `scroll` clips and scrolls its content; with `focusable="true"` it takes the arrow and paging keys. See [scrolling](/guide/layout#scrolling).
- `rule` is a one-cell line, horizontal by default (`axis="y"` for vertical).
- `spacer` takes the free space in a row or column, pushing its siblings apart.
