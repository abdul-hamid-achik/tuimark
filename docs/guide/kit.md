---
title: The kit
description: A copy-paste component kit for Tuimark - design tokens, k- component classes and 21 small documents you copy into your project and own, with a gallery app that shows them all.
---

# The kit

The kit is a set of ready-made components for Tuimark: buttons, cards, alerts, a data table, a command palette, dialogs, toasts, and more. It follows the shadcn way: you do not install it, you **copy what you need into your project and own it**. There is nothing to import and nothing to keep in sync; if a card should look different, you edit your copy.

The kit adds no tag, attribute, or CSS property to the language. A component is ordinary `.tui` markup plus class names, ids, and action names that follow a few conventions, styled by two stylesheets. It lives in the repository's [`kit/`](https://github.com/abdul-hamid-achik/tuimark/tree/main/kit) directory.

| File | Holds |
|---|---|
| `kit/theme.tcss` | the design tokens, dark and light, and the base typography |
| `kit/kit.tcss` | the component classes |
| `kit/components/NAME.tui` | one small, valid document per component, with the markup to copy and a `NAME.sample.json` beside it when it binds data |
| `examples/kit-gallery` | every component in one app, on five tabs, with a Go host |

## Using it

Copy `theme.tcss` and `kit.tcss` into your project and load them first, then your own sheet **last**. There is no `@import` in TCSS, so each document lists the sheets it uses:

```tui
<tui theme="auto">
  <style src="theme.tcss"/>
  <style src="kit.tcss"/>
  <style src="app.tcss"/>
  <screen>
    <col width="1fr" height="1fr" pad="1">
      <box class="k-card" title="Deploy">
        <text class="k-card-title">api-gateway 2.14.0</text>
        <row class="k-card-actions">
          <button id="deploy__go" class="k-btn primary" label="Deploy" on:click="button_deploy"/>
          <button id="deploy__skip" class="k-btn ghost" label="Not now" on:click="button_skip"/>
        </row>
      </box>
    </col>
  </screen>
</tui>
```

The sheets are cascaded together, and where two rules tie the later one wins. That is how you rebrand the kit: every color is a token in `:root`, so your own sheet only re-declares the ones you want to change.

```tcss
/* app.tcss, loaded after theme.tcss and kit.tcss */
:root { --accent: #e8590c; --focus: #e8590c; }
@media (theme: light) {
  :root { --accent: #c2410c; --focus: #c2410c; }
}
```

Every document in the kit validates on its own, so you can try a component before copying it:

```sh
tuimark validate kit/components/table.tui --data kit/components/table.sample.json
tuimark run kit/components/table.tui --data kit/components/table.sample.json
```

Each component file starts with a **contract**, an XML comment that says what it needs from your store and your host:

```text
k-NAME · Contract: <store paths it reads and writes> · Actions: <named actions> · Keys: <keys> · Requires: <other kit classes>
```

## Tokens

All color lives in `:root` of `theme.tcss`, read as `$name`. The first ten names are the runtime's own required tokens (the same names its built-in themes define); the kit adds the rest.

| Token | Use |
|---|---|
| `bg` `fg` `muted` `border` `panel` | the screen, body text, secondary text, idle borders, cards and modals |
| `accent` `focus` | the brand color, and the border or fill of the focused thing |
| `ok` `warn` `danger` `info` | status colors |
| `subtle` `raised` `sel` | tertiary text, raised or inset fills, the cursor row of an unfocused list |
| `on-accent` `on-ok` `on-warn` `on-danger` | text drawn on a filled color |
| `code` | inline code |

Selection and focus never rely on a background color alone. A cursor row is also bold, a focused list or table gets a double border and a solid accent bar, and a focused button is inverted and underlined, so all of it still reads under 16 colors.

## Conventions

**Classes.** Every kit class starts with `k-`: `k-card` for a component and `k-card-title` for a part of it. (TCSS has no descendant combinator, only `>`, so a part cannot be styled as "text inside a card"; each part carries its own class.) A **modifier** is a second, short class on the same node, such as `primary`, `ok`, `warn`, `danger`, and it is only ever used next to its component class: `k-btn danger`, `k-badge ok`, `k-alert warn`. Pick a modifier from your data with `class:NAME="path"` guards.

**Ids** are `instance__part`, with two underscores: `confirm__dialog`, `confirm__yes`, `confirm__no`, `field__email`. A node that is the whole component keeps the instance name (`svc` for a table), and so do the singletons of your app (`app`, `status`, `nav`).

**Actions** are `component_verb`, a plain identifier: `dialog_confirm`, `palette_run`, `toast_show`, `form_submit`, `table_select`. Generic names such as `cancel` or `save` are avoided. An action does not say which instance fired it; a host routes by the prefix of the event's `source` before the last `__`, so `confirm__yes` belongs to the instance `confirm`, and one handler serves every dialog.

**Overlays** (dialogs, the palette, the help) are modals that close on `esc`, write their own size, and declare their keys in a keymap scoped to their own class, listed before any global keymap:

```tui
<keymap when=".k-dialog">
  <bind keys="y" action="dialog_confirm" label="confirm"/>
  <bind keys="n" action="dialog_cancel" label="cancel"/>
</keymap>
```

**Sizes are explicit.** A component that fills a space says so in its class, and a `tabs` writes `focusable`, so nothing depends on the default size of a bare `row`, `col`, or modal.

## Components

| Component | Class | In one line |
|---|---|---|
| badge | `k-badge` | a short status label, with a modifier for its color |
| button | `k-btn` | `primary`, `secondary`, `danger` and `ghost`, inverted when focused |
| card | `k-card` | a bordered panel with a title that takes the focus color while it holds focus |
| alert | `k-alert` | a callout with an icon, in four status colors |
| app bar | `k-appbar` | a one-row bar docked to the top |
| status bar | `k-statusbar` | a one-row bar docked to the bottom, with filled segments |
| key hints | `k-hints` | the keymap's labeled rows as a row of hints |
| confirm dialog | `k-dialog` | a modal that puts the cursor on the safe button |
| form field | `k-field` | a label, an input, help text and an error line, with invalid styling |
| empty state | `k-empty` | an icon, a title, a line of help and a call to action, centered |
| data table | `k-table` | a table with a header, right-aligned numbers and columns that hide by priority |
| menu | `k-menu` | a navigation list with icons, shortcuts and optional check marks |
| tabs | `k-tabs` | a tab strip bound to a path |
| labeled progress | `k-meter` | a label, a value and a bar in eighths |
| stat | `k-stat` | a KPI card with a change and a sparkline |
| toast | `k-toast` | a one-row notice that never moves the page |
| command palette | `k-palette` | a modal with an input over a filtered list |
| help overlay | `k-help` | a modal that lists every labeled key |
| split pane | `k-split` | a list beside a detail card, stacked on a narrow terminal |
| log panel | `k-log` | a scrolling log that follows its end |
| breadcrumb | `k-breadcrumb` | the current path, down to the last crumb on a narrow terminal |

Markup has no expressions, so a few components lean on the host: the palette's filtered list and the split pane's detail are computed with `Derive`, and a toast is cleared by an `Every` tick. The contract comment of each file says which.

## The gallery

`examples/kit-gallery` shows every component together, with a Go host that implements the actions and routes them by instance. See it on the [Examples](/examples#kit-gallery) page, or run the view alone with fixed answers for every action:

```sh
tuimark run examples/kit-gallery/app.tui --data examples/kit-gallery/sample.json --mock examples/kit-gallery/mock.json
```

The mock opens and closes the dialogs and shows toasts; filtering the palette, validating the form and appending to the log need the Go host (`go run ./examples/kit-gallery`).
