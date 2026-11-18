---
id: theme-packs
title: Theme packs
kind: contract
status: implemented
summary: A theme pack is one .reel file with tokens, style rules and components. Hairline is built in and the default; packs on disk can extend it.
parent: reel
sources:
  - internal/theme/theme.go
  - internal/theme/hairline/theme.reel
  - internal/resolve/resolve.go
diagramLinks: {}
---

# Theme packs

A theme sets the look: colours, line widths, type sizes, timings, how things
enter and leave, and what a laptop or server looks like. A pack is one
`theme.reel` file written in the [script language](script-language.md), so
reel treats a theme like a script that runs before yours. Your rules beat
theme rules in the [cascade](cascade.md).

Hairline is the one built-in pack and the default. It is embedded in the
binary, and it is the same file you get with `reel theme export hairline`.

## What a pack holds

```text
extends hairline          # optional: start from another pack

tokens                    # name value, one per line
  line-2   #D4D7DE
  size-title  40
  motion-draw 0.6

style                     # default rules, lowest in the cascade
  box  enter=draw

define server units=3     # components built from primitives
  col #units gap=0
    repeat units
      box #unit{i} w=160 h=60
  ...
```

- **Tokens** are colours, sizes and timings. The stage, layout and motion
  read them by name; nothing about the look is hard-coded in the engine.
  Colours must be `#rrggbb`.
- **Style** sets defaults per kind: hairline makes shapes and connectors draw
  in, components and text fade in, and titles rise.
- **Components** are `define` blocks: laptop, phone, server, process, db and
  queue in hairline. A server is a column of `unit` boxes, a `procs` column
  with a slot for your processes, and a label.

## Using a pack

`theme NAME` in the video block picks a pack. A built-in name is used as is;
otherwise reel looks for `themes/NAME/theme.reel` and then
`NAME/theme.reel` beside the script, or a path such as `theme ./brand`.

`extends` loads another pack first, so a pack can change three tokens and
keep everything else. Extending in a loop is reported. A script can also
override tokens for itself with its own `tokens` block, without a pack.

```sh
reel themes                         # list built-in packs
reel theme export hairline          # writes themes/hairline/theme.reel
```

## Hairline

Near-black ground, 1px grey lines and one grey accent. The visual rules come
from the mocks the theme was designed with: no pill shapes, no uppercase
labels above titles, key words emphasized by brightness rather than a box
behind them, and only straight or right-angled arrows.

## Not built

Fonts in packs: hairline's fonts are embedded, and a pack cannot bring its
own yet.
