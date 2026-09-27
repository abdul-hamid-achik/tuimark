---
title: TCSS
description: Tuimark's cell CSS - selectors, specificity, colors and theme tokens, dark, light and auto themes, and @media queries on columns, rows and theme.
---

# TCSS

TCSS is CSS for cells, not for browsers. It keeps the parts of CSS that make sense on a terminal grid (selectors, a cascade, tokens, media queries) and drops the rest. Everything is measured in cells, and every property and value comes from a [closed list](/reference/css): anything else is `V003`.

```tcss
:root { --accent: #e0443e; }

screen { color: $fg; background: $bg; }
#sidebar { width: 32; border: rounded; border-color: $border; }
#sidebar:focus-within { border-color: $focus; }
.muted { color: $muted; }
list > item:selected { reverse: true; }

@media (max-cols: 80) {
  #body { layout: column; }
  #sidebar { width: 100%; height: 8; }
}
```

## Where styles come from

From lowest to highest priority:

1. **The built-in sheet**: the defaults every document starts with (below).
2. **Presentational attributes**: `width`, `height`, `gap`, `pad`, and `border` written on a tag.
3. **Stylesheets**: `<style src="theme.tcss"/>` and inline `<style>` blocks, in document order.
4. **Inline styles**: `style="…"` on a tag.

Prefer classes in a stylesheet. Inline styles are an escape hatch, and presentational attributes lose to any stylesheet rule that sets the same property.

## Selectors

| Selector | Example | Matches |
|---|---|---|
| type | `box` | every `<box>` |
| id | `#sidebar` | the node with `id="sidebar"` |
| class | `.muted` | nodes with the class, from `class` or a [`class:NAME` guard](/guide/bindings#conditional-classes) |
| child | `row > box` | a `box` whose parent is a `row` |
| pseudo-class | `list:focus` | see below |
| list | `#header, #status` | either selector |

Compounds combine them: `#procs > item > text.hot`, `button:focus`.

| Pseudo-class | Matches |
|---|---|
| `:focus` | the focused node |
| `:selected` | the cursor row of a list or table, the active tab's label |
| `:disabled` | a node with a truthy `disabled` |
| `:empty` | a container with no children laid out |
| `:checked` | a list or table row whose key is in the widget's `checked` array (version 2) |
| `:focus-within` | the focused node and every ancestor up to the screen (version 2) |

Not supported: the descendant combinator (a space), `*`, attribute selectors, `nth-*`, `:hover`, nesting, and `&`. Use a class or a chain of `>` instead.

## Specificity

A type counts 1, a class 10, an id 100, and each pseudo-class 10; an inline `style=""` counts 1000. The highest wins, and a later rule wins a tie. So `#tasks:focus` (110) beats `#tasks` (100):

```tcss
#tasks { border: rounded; border-color: $border; }
#tasks:focus { border-color: $focus; }
```

When a cell does not look the way you expect, [`tuimark inspect`](/guide/tools#inspect) prints, for every property, the rule that won and the ones it beat.

Text properties (`color`, `bold`, `dim`, `italic`, `underline`, `reverse`, `wrap`, `visibility`) are inherited from the parent. Layout properties are not.

## Colors and tokens

A color is one of:

- a **token**: `$accent`, or the synonym `var(--accent)`;
- one of the 16 **ANSI names**: `black red green yellow blue magenta cyan white`, and each with `bright-` in front;
- a hex color, `#rgb` or `#rrggbb`;
- `default`, the terminal's own foreground or background.

Every theme defines ten tokens, and a stylesheet can override them or add its own in `:root`:

```tcss
:root {
  --accent: #e0443e;
  --brand: #c8102e;   /* a new token, used as $brand */
}
```

| Token | Built-in dark | Built-in light |
|---|---|---|
| `$bg` | `#0d1117` | `#ffffff` |
| `$fg` | `#e6edf3` | `#1f2328` |
| `$muted` | `#8b949e` | `#656d76` |
| `$accent` | `cyan` | `blue` |
| `$ok` | `green` | `green` |
| `$warn` | `yellow` | `yellow` |
| `$danger` | `red` | `red` |
| `$border` | `#30363d` | `#d0d7de` |
| `$focus` | `cyan` | `blue` |
| `$panel` | `#161b22` | `#f6f8fa` |

A token that no theme and no `:root` rule defines is `V003`. Colors stay exact all the way to the dump (`#rgb` becomes `#rrggbb`, in lowercase); they are converted to what the terminal supports (truecolor, 256, or 16 colors) only when `Run()` or `preview` paints them.

## Themes

The document's `theme` picks the base set of tokens: `dark` (the default) or `light`. In version 2, `theme="auto"` asks the terminal for its background color when the app starts (falling back to the `COLORFGBG` variable, then to dark), so the first frame is already in the right palette.

The effective theme comes from, highest first:

1. `TUIMARK_THEME=dark|light` in the environment, for a running app;
2. the host's `Set("@theme", "dark"|"light"|"auto")`;
3. `--theme dark|light` on a CLI command;
4. the document's `theme`.

The tools never probe a terminal, so `auto` renders as `dark` in `dump`, `play`, `inspect`, and the goldens; `validate` checks an `auto` document under both themes.

`:root` tokens apply whatever the theme, so a stylesheet that defines every token looks the same in both. To give each theme its own palette, use `@media (theme: …)` (version 2), after the base `:root` it refines, because tokens apply in document order:

```tcss
:root {
  --bg: #1a1614;
  --fg: #f2ede8;
  --accent: #e0443e;
}

@media (theme: light) {
  :root {
    --bg: #fbfaf8;
    --fg: #1f1a17;
    --accent: #c8102e;
  }
}

.brand { color: $accent; bold: true; }
```

Check the other theme without touching the document: `tuimark dump app.tui --theme light --styles --format json`.

## Media queries

`@media` applies rules by the size of the terminal, or by theme:

```tcss
@media (max-cols: 80) { #body { layout: column; } }
@media (min-cols: 81) { #sidebar { width: 32; } }
@media (max-rows: 20) { #header { display: none; } }
@media (theme: light) { .brand { color: $accent; } }
```

The features are `min-cols`, `max-cols`, `min-rows`, `max-rows`, and, in version 2, `theme` (`dark` or `light`). Each `@media` block takes exactly one feature: there is no `and` or `or`, so write two blocks when you need two conditions. Blocks do not nest.

Good breakpoints are 40, 80, and 120 columns, the widths `validate` checks. A layout that stacks its panes below 80 columns and hides secondary chrome below 40 covers most terminals.

## The built-in sheet

Every document starts from these rules; yours override them.

```tcss
screen { layout: column; width: 100%; height: 100%; }
col    { layout: column; width: 1fr; height: 1fr; }
row    { layout: row;    width: 1fr; height: 1fr; }
box    { layout: column; }
text   { width: auto; height: auto; }
scroll { layout: column; }
list   { layout: column; }
item   { layout: column; }
modal  { layout: column; border: single; }
spacer { flex: 1; }
input, button, progress, rule { width: auto; height: auto; }
list > item:selected { reverse: true; }
table  { layout: column; wrap: truncate; }
table > item:selected { reverse: true; }
tabs   { layout: column; }
tab    { layout: column; }
sparkline { width: auto; height: auto; }
hints  { layout: row; gap: 2; }
hints > row { gap: 1; }
```

## Things that surprise people

- **Only `modal` has a border by default.** `list:focus { border-color: $focus; }` changes nothing on a list without `border: single` (or `rounded`, `double`, `thick`).
- **A border needs room.** It takes a cell on each side, so an `<input>` sized `height: 1` cannot show one. Leave its height `auto` (three rows with a border), or show focus with `color`, `background`, `bold`, or `reverse`.
- **`reverse` paints the whole rect.** A focused button stretched to a tall row paints a tall block. Give action rows `height: 1`.
- **Rules on a `column` style its header cell only.** Body cells carry the column's `class`, so style them with `.num` or `#table > item > text`.
