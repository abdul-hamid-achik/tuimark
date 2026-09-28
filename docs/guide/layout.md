---
title: Layout
description: How Tuimark lays out a frame on an integer cell grid - units, the box model, fr and percent sizing, docks, scrolling, auto-fit and grids.
---

# Layout

Tuimark lays out every frame itself, on a grid of whole terminal cells, with one measuring pass and one allocating pass. There are no pixels and nothing is fractional, so a layout is exact and the same on every machine: what `tuimark dump` reports is what the terminal shows.

Every frame on this page is real `tuimark dump` output.

## Units

| Unit | Example | Means |
|---|---|---|
| `N` | `width: 12` | N cells: columns for widths, rows for heights |
| `N%` | `width: 30%` | a percent of the parent's content box on that axis, rounded down |
| `Nfr` | `width: 1fr` | a share of the space left after everything else |
| `auto` | `height: auto` | the content's own size |

`flex: N` is accepted as another way to write a grow weight on the parent's main axis; it shares the leftover space with `fr`. `px`, `em`, `ch`, `vw`, `vh`, and `min-content` are `V003`.

<<< @/snippets/layout/units.tui{tui}

<<< @/snippets/out/units.txt{grid}

At 60 columns, `#a` takes 12 and `#b` takes 30% of 60, 18. The 30 cells left are shared one part to `#c` and two parts to `#d`. The last `fr` item gets any remainder, so no cell is lost to rounding.

## The box model

`width` and `height` are the **outer** size, border included. Inside it:

```
outer    the allocated rect: x, y, w, h
border   one cell on each side, when there is one
padding  inside the border
content  what is left for children or text
```

`padding` and `margin` take one to four cell values, in the CSS order (top, right, bottom, left). `gap` (0–4) puts empty cells between a container's children. A size that would leave a negative content box is clamped to zero, and anything that does not fit is clipped, never wrapped onto the next line.

In `version="3"`, `row-gap` and `column-gap` (0–4) split `gap` into its two axes: `column-gap` between siblings side by side (a `layout: row` container, grid and table columns, a tab strip, a `hints` laid out along a row), `row-gap` between siblings stacked (a `layout: column` container, grid rows, a `hints` with `layout: column`). `gap` stays their shorthand in the cascade — it sets both, with its own specificity and order — so a document that writes only `gap` lays out exactly as before.

## Direction and alignment

`col` stacks its children vertically, `row` horizontally, and `box` is a column unless you set `layout: row`. Any container takes `layout: column | row` (and, in version 2, `grid`).

| Property | Axis | Values |
|---|---|---|
| `justify` | main | `start`, `center`, `end`, `space-between` |
| `align` | cross | `stretch` (the default), `start`, `center`, `end` |
| `content-align` | text inside its box | `start`, `center`, `end` |

With the default `align: stretch`, children fill the cross axis. That is usually what you want, but it is also why a row of buttons in a tall modal grows tall: give such a row `height: 1`.

## Default sizes

| Tag | Width | Height |
|---|---|---|
| `screen` | 100% | 100% |
| `col`, `row` | `1fr` | `1fr` |
| `box` in a `row` | `auto` | `1fr` |
| `box` in a `col` | `1fr` | `auto` |
| `text`, `input`, `button`, `progress`, `rule` | `auto` | `auto` |
| `list`, `scroll`, `table` in a `col` | `1fr` | `auto`, fit to the space left |
| `sparkline` | one column per value | 1 |
| `modal` | 80% of the screen | 80% of the screen |

If any sibling on an axis has an `fr` size, siblings with no size on that axis become `1fr` too.

## How space is shared

Along the main axis, a container hands out its content size in this order:

1. fixed sizes (`N`);
2. percentages, of the parent's full content size (not of what is left);
3. `auto` children, at their content size;
4. viewports sized `auto`, at their content size but capped by what is left ([auto-fit](#auto-fit));
5. what remains, split between the `fr` children by weight.

`min-width`, `max-width`, `min-height`, and `max-height` clamp a child after its percentage and before the `fr` split. There is no flex-wrap: a row that does not fit is clipped.

## Docks

`dock: top | bottom | left | right` pulls a child out of the flow and pins it to an edge of its parent. Top and bottom strips are cut first, then left and right, and the other children share what is left.

<<< @/snippets/layout/docks.tui{tui}

<<< @/snippets/out/docks.txt{grid}

Give a docked child a size across its edge: a `height` for `top` and `bottom`, a `width` for `left` and `right`. An `fr` size on the same axis as a dock is `V010`, and more than one bottom-docked status line is warned about (`L005`).

## Scrolling

A **viewport** clips its content and scrolls it: a `scroll`, a `list`, a `table`, or any container with `overflow: scroll`.

- `<scroll axis="y">` (the default) scrolls vertically, `axis="x"` horizontally, `axis="both"` both ways. `list`, `table`, and `overflow: scroll` scroll on `y`.
- A `scroll` takes the arrow and paging keys when it has focus; give it `focusable="true"` and an `id`. A list or table follows its cursor instead.
- In version 2, `scrollbar: auto` paints a thumb over the right border of a bordered viewport at least 3 rows tall whose content does not fit. It never takes layout space.
- In `version="3"`, `<scroll id="log" stick="bottom">` keeps a growing viewport showing its end, like a log or a chat: while the offset is already at the bottom it keeps sticking as content arrives, but scrolling up to read back stops it from jumping, until you scroll (or `move-last`) back to the end. It needs an `id` and a `y` axis, and cannot go inside a list `<item>` or a container `each` template.

The dump reports each viewport's `scroll` state (offset and extent), and a list reports every item, including the ones scrolled out of view.

## Auto-fit

A viewport sized `auto` on its scroll axis takes its content size, capped by the space its siblings leave. A short list stays short; a long one fills what is left and scrolls, and whatever comes after it stays on screen.

<<< @/snippets/layout/autofit.tui{tui}

<<< @/snippets/out/autofit.txt{grid}

Twelve lines of log do not fit in 8 rows, so the list takes the 6 rows the title and the status line leave, and scrolls to keep its cursor (the last line) in view. Viewports are served in document order: the first may leave nothing for the next. To share space between two viewports, give both an `fr` size.

`L006` warns when a viewport can never show any of its content: zero rows while it has some, or clipped away by an ancestor that does not scroll. The fix is a size (`N`, `%`, or `fr`) or more room in the parent.

## Grids

In version 2, `layout: grid` places a container's children in equal columns, row by row, which is what a dashboard of panels needs:

<<< @/snippets/layout/grid.tui{tui}

`grid-columns: K` is the most columns (1 to 12). With `grid-min-width: M`, the count adapts to the width: as many columns of at least `M` cells as fit, up to `K`, so the grid is responsive without any `@media`. The same document at 64 and 36 columns:

<div class="side-by-side">

<<< @/snippets/out/grid-wide.txt{grid}

<<< @/snippets/out/grid-narrow.txt{grid}

</div>

`gap` separates both columns and rows. A row is as tall as its tallest child, and `align` places shorter ones. A child's `width`, `min-width`, `max-width`, `flex`, and any `fr` or `%` height are ignored in a grid (`L007` warns). A grid never shrinks to fit: put it in a `<scroll>` (or make it `overflow: scroll`) to scroll its rows when they do not fit.

## Responsive layouts

`@media` switches rules by terminal size. This is the tour document from the [home page](/): at 56 columns its panes stack.

<<< @/snippets/tour/theme.tcss{tcss}

<<< @/snippets/out/tour-56.txt{grid}

`display: none` removes a node from the layout entirely (the monitor example hides table columns that way on narrow terminals); `visibility: hidden` keeps its space and paints nothing.

## Checking a layout

Dump at the widths you care about and read the nodes:

```sh
tuimark dump app.tui --data sample.json --cols 80 --rows 24
tuimark dump app.tui --data sample.json --cols 120 --rows 24 --format json
```

Layout problems are diagnostics with an `L` code. The ones you will meet:

| Code | Meaning |
|---|---|
| `L001` | an `fr` size or `flex` on a child along its viewport's scroll axis, where there is no leftover to share |
| `L002` | a `%` size inside a parent whose size on that axis is `auto` |
| `L003` | fixed and minimum sizes that add up to more than the parent (a warning; the rest is clipped) |
| `L004` | a modal that is not the last child of its screen |
| `L006` | a viewport that can never show its content |
| `L007` | a size on a grid cell, which the grid ignores |

The full list is in the [diagnostics reference](/reference/diagnostics#layout).
