---
title: The .tui language
description: How a Tuimark document is structured, the closed set of tags, common attributes, text, screens and modals, and document versions.
---

# The .tui language

A `.tui` file is a strict subset of XML that describes what is on the screen: containers, text, and widgets, with their data bindings and the names of the actions they fire. It holds no logic. How things look lives in [TCSS](/guide/tcss), the data in JSON, and what actions do in the host program.

## A document

```tui
<tui version="2" theme="auto" mouse="mouse_enabled">
  <style src="theme.tcss"/>
  <style>
    #title { bold: true; }
  </style>
  <keymap>
    <bind keys="q,ctrl+c" action="quit"/>
  </keymap>
  <screen id="main" focus="#query">
    <!-- containers, text and widgets -->
    <modal id="confirm" open="confirm_open">…</modal>
  </screen>
  <screen id="logs">…</screen>
</tui>
```

- **`<tui>`** is the root. `version` is required (`V014` without it). `theme` is `dark` (the default), `light`, or, in `version="2"`, `auto`. `mouse` (version 2) turns mouse input on; see [the mouse](/guide/interaction#the-mouse).
- **`<style>`** loads a stylesheet with `src` (relative to the document), or holds one inline. Several are fine; they apply in order.
- **`<keymap>`** holds the `<bind>` rows: which keys fire which actions. See [Keymap, actions and focus](/guide/interaction).
- **`<screen>`** is a full terminal surface. A document can have several; exactly one is active, the first by default, and the host switches with `Set("@screen", "logs")`. `focus="#id"` picks the node focused when the screen appears.
- **`<modal>`** is a layer over its screen. Modals come last in their screen (`L004` otherwise).

## XML rules

Tuimark reads well-formed XML and nothing fancier:

- UTF-8, one root, lowercase tag and attribute names, values in double quotes.
- Comments are fine. No namespaces, DTDs, or processing instructions, and no entities beyond `&lt; &gt; &amp; &quot; &apos;`.
- A document that is not well-formed is `V005`; `tuimark fmt` refuses it and every other command reports it.

Control characters and ANSI escapes are removed from bound data before it is painted, so data can never repaint the terminal; in the document's own text they are an error (`V007`).

## Versions

Every document says which vocabulary it uses:

- **`version="1"`** is the original vocabulary. Its meaning, and its dumps, never change.
- **`version="2"`** accepts everything `version="1"` does, with the same meaning, plus the 0.2 additions: the tags `table`, `column`, `tabs`, `tab`, `sparkline`, and `hints`; `each` on containers; multi-select; `class:NAME`; `theme="auto"`; `@media (theme: …)`; `layout: grid`; scrollbars; `bar: eighths`; the built-in `move-*`, `check-*`, and `switch-to` actions; the mouse; and `when` matching over the focus chain.
- **`version="3"`** (0.3) is `version="2"` plus two layout warnings about content a dump shows as gone:
  - `L008`: a text cut without an ellipsis;
  - `L009`: a node that cuts a child on an axis it does not scroll.

  The dump marks the node with `clipped`. When a cut is intended, write `wrap: nowrap` or `overflow: hidden` and the warning goes away. A document that clips nothing dumps exactly as it would as `version="2"`.

Use `version="3"` for new documents. In a `version="1"` document each 0.2 addition is still recognized, and reported with a message that ends in `(requires version="2")`, so an agent knows exactly what to change. A `.tcss` file has no version of its own: it is checked against the document that loads it.

## The tags

The vocabulary is closed: an unknown tag is `V001`, not a new widget. The [tags reference](/reference/tags) lists every attribute each tag takes.

| Role | Tag | What it is |
|---|---|---|
| Document | `tui` | the root |
| | `style` | a stylesheet, from `src` or inline |
| | `keymap`, `bind` | global keys and one key row |
| | `screen` | a terminal surface; one is active |
| Layout | `col`, `row` | vertical and horizontal flex containers |
| | `box` | a generic container, a column by default |
| | `scroll` | a clipping viewport that scrolls its content (`axis="y\|x\|both"`) |
| | `spacer` | an empty item that takes the free space |
| | `tabs`, `tab` | a strip of tab labels over the active tab (version 2) |
| Content | `text` | text, with `{path}` interpolation |
| | `rule` | a one-cell separator line (`axis="x\|y"`) |
| | `sparkline` | a bar chart of a numeric array (version 2) |
| | `hints` | key hints generated from the keymap (version 2) |
| Interactive | `list`, `item` | a focusable list with a cursor; `item` is its row |
| | `input` | a single-line text field |
| | `button` | a focusable action |
| | `progress` | a 0–100 bar |
| | `table`, `column` | rows of an array in columns, with a cursor (version 2) |
| Overlay | `modal` | a layer above the screen that traps focus |

[Widgets](/guide/widgets) shows each interactive tag with a rendered example.

## Common attributes

Most tags share these:

| Attribute | Meaning |
|---|---|
| `id` | a unique name (`V004` on a duplicate). Required on focusable widgets: `list`, `input`, `button`, `modal`, and in version 2 `table`, `tabs`, `tab` (`V012`) |
| `class` | space-separated class names for [TCSS](/guide/tcss) selectors |
| `title` | text painted on the node's top border, when it has one; may interpolate `{path}` |
| `if` | `path` or `!path`: the node exists only while it is truthy |
| `hidden`, `disabled` | `true`, `false`, `path`, or `!path` |
| `focusable` | `true` lets any node take focus |
| `on:click`, `on:focus`, … | the name of an action to fire; see [actions](/guide/interaction#actions-are-names) |
| `class:NAME` | adds class `NAME` while a path is truthy (version 2); see [bindings](/guide/bindings#conditional-classes) |
| `style` | inline TCSS declarations; an escape hatch, prefer classes |
| `width`, `height`, `gap`, `pad`, `border` | presentational shortcuts for the same CSS properties; any stylesheet rule beats them |

An attribute a tag does not take is `V002`.

## Text

`<text>` holds a template: literal text with any number of `{path}` interpolations.

```tui
<text>{done} of {total} tasks</text>
<text wrap="wrap">{ticket.body}</text>
```

- A `<text>` has no element children (`V013`).
- Each line of the body is trimmed, and blank lines at the start and end are dropped, so indentation in the source never reaches the screen. Interior spaces are kept: `mail  {folder}` keeps its two spaces. For spacing around text, use `gap`, `padding`, or `margin` on a container.
- `wrap` is `nowrap` (the default: the line is cut at the edge), `wrap` (word-wrap to the width), or `truncate` (cut with `…`).
- Width is measured in terminal columns by grapheme cluster: CJK and emoji take two columns, combining accents take none, and a cluster is never split.

`<button>` takes its label from `label` or from its body; `title` and `placeholder` are templates too. See [bindings](/guide/bindings) for how paths resolve.

## Screens and modals

A document with several screens shows one at a time. Only the active screen and its open modals are laid out; the host switches with `Set("@screen", "id")`, and in version 2 a keymap row can do it with `action="switch-to" to="#id"`.

A modal is open while its `open` attribute is truthy: `true`, `false`, a path, or `!path` (never `{path}`). `bind` on a modal is a shorthand for the same thing; if both are present, `open` wins. An open modal:

- is laid out above its screen, centered, 80% × 80% unless you size it, with a single border by default;
- traps focus: `tab` cycles inside it, and a focus request for a node outside it does nothing;
- fires `on:open` when it opens and `on:close` when it closes, and `on:escape` when `esc` is pressed;
- gives focus back to where it was when it closes.

## What is not in the language

On purpose, and permanently:

- **No expressions.** No arithmetic, comparisons, filters, function calls, or array indexes. Markup has `{path}`, `each`, `if`, and guards; anything computed comes from the host as data.
- **No pixels.** One unit is one terminal cell. `px`, `em`, `vh` and friends are `V003`.
- **No open vocabulary.** Tags and properties are a fixed catalog, so an agent cannot invent a widget that silently does nothing.
- **No generated code.** A `.tui` file is interpreted at run time; nothing is compiled to Go.

## Formatting

`tuimark fmt` rewrites a document in one canonical shape: two-space indentation, one element per line, attributes in a fixed order (`id`, `class`, the `class:NAME` guards, then the rest), self-closing empty elements, and comments kept in place.

```sh
tuimark fmt app.tui            # print the formatted document
tuimark fmt app.tui --write    # rewrite the file
tuimark fmt app.tui --check    # exit 2 when it is not formatted
```

It works on any well-formed document, even one with diagnostics, so it is safe to run before anything else.
