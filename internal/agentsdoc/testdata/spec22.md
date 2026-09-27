# Tuimark

You design terminal UIs by editing text documents. You do not write Go layout.

## Allowed edits
- *.tui
- *.tcss
- examples/**/sample.json

## Forbidden
- internal/**
- cmd/** except flag wiring
- Bubble Tea / lipgloss / tea.Cmd / Update()
- inventing tags not in the catalog

## Catalog
tui style keymap bind screen
col row box scroll spacer
text rule
list item input button progress
modal

Spike-only until dump goldens pass:
app col row box text
attrs: id class width height gap pad border

## Loop
tuimark validate FILE
tuimark dump FILE --data sample.json --cols 80  --rows 24 --format json
tuimark dump FILE --data sample.json --cols 120 --rows 24 --format json
Read grid + nodes. Edit markup or CSS. Repeat.
Stop when 80 and 120 match intent and errors is empty.

## Bindings
{path} only inside <text>, title, placeholder.
each="tickets as item" only on <list>.
if="path" or if="!path".
No expressions.

## Actions
on:select="open" is a name. The host implements it.
