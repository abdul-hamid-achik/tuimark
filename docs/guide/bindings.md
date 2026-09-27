---
title: Bindings and data
description: How Tuimark markup reads a JSON store - {path} interpolation, bind, each, if, truthiness and class:NAME guards - and why it has no expressions.
---

# Bindings and data

A Tuimark app has one data store: a JSON value, usually an object. The host fills it with `Bind` and changes it with `Set`; the CLI loads it from a file with `--data sample.json`. Markup reads it through **paths** and never computes anything: every value it shows is already in the store.

## Paths

A path is a dotted chain of names: `title`, `ticket.body`, `cpu.hist`. Each name is a letter or `_` followed by letters, digits, or `_` (no hyphens, no spaces, no indexes). Inside an `each` template, the alias comes first: in `each="tickets as t"`, `t.title` is the current ticket's title.

Arrays are reached only through `each`; `tickets.0.title` is not a path markup can use.

## Interpolation

`{path}` inserts a value into text. It works in four places: the body of a `<text>`, a `title`, a `placeholder`, and the body of a `<column>` (the cell template of a [table](/guide/widgets#table)).

```tui
<text>{done} of {total} done ({done})</text>
<box title="{ticket.id}">…</box>
<input id="q" bind="query" placeholder="search {folder}"/>
```

The value is formatted as text:

| Value | Shows as |
|---|---|
| a string | itself, with control characters and ANSI escapes removed |
| an integer-valued number | `3`, `1024` |
| another number | the shortest form that reads back the same: `2.5`, `1e+20` |
| `true`, `false` | `true`, `false` |
| `null` | nothing |
| an array or object | compact JSON, object members sorted: `[1,"a"]`, `{"a":2,"b":1}` |

A path that does not resolve shows nothing and reports `B003`, a warning (`--strict` makes it an error). Anything between braces that is not a plain path, such as `{ folder }` or `{a + b}`, is `V003`. A `{` with no closing `}` is just text.

## Widgets that write back

`bind` connects a widget's own state to a path, in both directions:

| Widget | `bind` holds | Written when |
|---|---|---|
| `input` | its text | the user types |
| `list`, `table` | the key of the cursor row | the cursor moves |
| `tabs` | the id of the active tab | a tab is activated |
| `progress` | a number from 0 to 100 | never (read only) |
| `sparkline` | an array of numbers | never (read only) |
| `modal` | truthy while open | never (read only) |

So the host never has to track what is typed or selected: it is in the store, and the `on:change` or `on:select` event says when it moved. Setting the path from the host moves the widget.

## Repeating with each

`each="path as alias"` repeats a template once per element of an array, in order. On a `list` (and in version 2 a `table`), the template is the `<item>` and each element is a row:

```tui
<list id="tickets" each="tickets as t" key="t.id" bind="selected" on:select="open">
  <item>
    <text>{t.id} {t.title}</text>
  </item>
</list>
```

- `key="path"` names each row. The cursor, `bind`, checked rows, and the `keys` of every event use it, so a row keeps its identity when the array is re-sorted or filtered. Without `key`, rows are numbered by index and you get `B006`.
- The path must be an array: a missing or non-array value is `B001`.
- Nothing inside a list row can take focus. The list is the focusable unit and moves its own cursor; act on a row through `on:select` or a keymap row with `when="#tickets:focus"`, whose event names the row.

In version 2, `each` also works on `col`, `row`, and `box`. The container is not repeated; its children are the template, inflated once per element:

```tui
<row id="tags" each="tags as tag" key="tag">
  <text class="tag">{tag}</text>
</row>
```

Such a template holds nothing that takes focus or keeps state (no `input`, `button`, `list`, `modal`, `table`, `tabs`, `tab`, no `focusable`, `on:click`, or `on:focus`) and no `id`, since it would repeat. An alias that shadows an enclosing one is `V011`. An empty array leaves the container empty, so it matches `:empty`.

## Conditions

`if="path"` keeps a node only while the value is truthy; `if="!path"` only while it is not:

```tui
<box if="selected_ticket">
  <text>{selected_ticket.title}</text>
</box>
<text if="!selected_ticket" class="muted">no selection</text>
```

| Value | Truthy? |
|---|---|
| `null`, `false`, `0`, `""`, `[]` | no |
| everything else, `{}` included | yes |

A path that does not resolve reports `B002` and counts as `null`: `if="missing"` removes its node and `if="!missing"` keeps it.

`hidden` and `disabled` take the same `path` and `!path`, or a literal `true` or `false`. A hidden node is not laid out; a disabled one stays visible but cannot take focus, and keys, actions, and clicks skip it. A modal's `open` and the document's `mouse` flag work the same way.

## Conditional classes

In version 2, `class:NAME="path"` adds the class `NAME` while the path is truthy (or `class:NAME="!path"` while it is not). It is how data changes the look of a node without any expression in markup:

```tui
<text class="status" class:live="live" class:stale="stale">{status_label}</text>
```

```tcss
.live  { color: $ok; }
.stale { color: $danger; }
```

The guard is evaluated on every frame, in the node's scope, so inside a template it reads the alias: `<item class:blocked="p.protected"/>` colors the rows of protected processes. `NAME` is lowercase: a letter or `_`, then letters, digits, `_`, or `-` (`V015` otherwise). A missing path is `B002` and adds nothing.

The JSON dump of a version 2 document lists each node's `classes`, the static ones first, so you can check that a guard fired without reading colors.

## No expressions

Markup cannot compute: no arithmetic, comparisons, string formatting, or filtering. When a view needs a derived value (a count, a label like `cpu 23%`, a flag like `cpu_hot`, the filtered rows), the host computes it and puts it in the store. This keeps every `.tui` file readable at a glance, keeps the runtime small, and gives an agent exactly one place to look when a value is wrong: the data.

```json
{
  "count": 3,
  "cpu": { "label": "cpu 23%", "pct": 23, "hot": false },
  "tickets": [{ "id": "t-12", "title": "login loop on staging" }]
}
```

Keep a `sample.json` next to each document with realistic data, including the awkward cases (long strings, empty arrays, wide characters). The CLI renders with it, `validate --data` checks every path against it, and golden tests pin it.
