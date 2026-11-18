---
id: theme-packs
title: Theme packs
kind: contract
status: mixed
summary: A theme supplies tokens, default style rules and components written in the script language.
parent: reel
sources:
  - internal/theme/theme.go
  - internal/theme/hairline/theme.reel
diagramLinks: {}
---

# Theme packs

A theme sets the look: colours, line widths, fonts, how things enter and
leave, and what a laptop or server looks like. Its rules and components are
written in the [script language](script-language.md), so reel treats a theme
like a script that runs before yours. Your rules beat theme rules in the
[cascade](cascade.md).

Status: the hairline theme is built in and used by `reel check`. Loading theme
folders from disk, `extends`, and per-orientation settings for 9:16 are
proposed in the [design spec](../spec.html), not built.

## What a theme holds

- **Tokens**: named colours, stroke width, radius and fonts, such as
  `line-1 = #6A6E7A`. Today they are defined in Go. Nothing reads them until
  rendering is built.
- **Default rules**: a `style` block. Hairline makes connectors and shapes
  draw in, components and text fade in, and titles rise.
- **Components**: `define` blocks for laptop, phone, server, process, db and
  queue. A server is a column of `unit` boxes, a `procs` column with a slot
  for your processes, and a label.

## Hairline

Near-black ground, 1px grey lines and one grey accent. The visual rules come
from the mocks the theme was designed with: no pill shapes, no uppercase
labels above titles, key words emphasized by brightness rather than a box
behind them, and only straight or right-angled arrows.
