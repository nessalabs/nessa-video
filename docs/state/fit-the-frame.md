---
id: fit-the-frame
title: Fit the frame
kind: operation
status: implemented
summary: The same script renders for 16:9 and 9:16. The engine turns rows and diagrams and scales what still does not fit; style blocks per orientation adjust the rest.
parent: reel
sources:
  - internal/layout/layout.go
  - internal/resolve/resolve.go
  - internal/stage/stage.go
  - examples/responsive.reel
diagramLinks:
  Layout: draw-a-still
diagramDetails:
  - diagram: 0
    node: Rules
    section: Orientation styles
  - diagram: 0
    node: Measure
    section: Room for labels
  - diagram: 0
    node: Flip
    section: Turning rows and diagrams
  - diagram: 0
    node: Scale
    section: Scaling what is left
---

# Fit the frame

A script names one size, and changing `size 1920x1080` to `size 1080x1920`
is enough to get a phone cut. The engine fits each scene to the frame the
way a responsive page does. Style blocks for one orientation adjust whatever
the automatic fit does not get right.

```mermaid
flowchart LR
    Rules[Orientation styles] --> Measure[Measure the scene]
    Measure --> Fits{Fits the frame?}
    Fits -->|yes| Layout[Place everything]
    Fits -->|too wide or tall| Flip[Turn a row or diagram]
    Flip --> Measure
    Fits -->|nothing left to turn| Scale[Scale down to fit]
    Scale --> Layout
```

## Orientation styles

`style portrait` and `style landscape` hold rules that apply only in that
orientation, like a CSS media query. A frame taller than it is wide is
portrait. Rules for the other orientation are still checked, so a typo shows
up whichever size you render.

```text
style portrait
  #stack  dir=col        # this row becomes a column on phones
  .aside  hidden=true    # leave the terminal out of the phone cut
```

`dir=row` or `dir=col` turns any row or column the other way; on a Mermaid
flowchart `dir=LR` or `dir=TD` does the same. `hidden=true` removes an element
from layout and drawing; connectors to it are not drawn either.

## Room for labels

Before measuring, the engine finds labelled connectors between neighbouring
elements in a row and widens the gap between them so the label fits.

## Turning rows and diagrams

While the scene is wider than the frame, the widest row is turned into a
column, and a left-to-right flowchart into a top-to-bottom one. While it is
taller than the frame, the tallest column becomes a row if its children
would then fit across. Rows and columns with their own `dir`, or with
`fit=false`, are left alone; a flowchart's `LR` or `TD` header is only a
preference.

## Scaling what is left

If the scene still does not fit, everything is scaled down evenly around the
centre of the stage. Captions are not scaled. The camera still zooms into a
target at full size.
