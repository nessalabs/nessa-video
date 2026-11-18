---
id: domain-language
title: Shared words and areas
kind: guide
status: reference
summary: The terms reel's code and docs use, which area owns each one, and which way dependencies point.
parent: reel
sources:
  - internal/diag/diag.go
  - internal/selector/selector.go
  - internal/composition/catalog.go
diagramLinks:
  Script: script-language
  Composition: cascade
  Resolve: check-a-script
  Theme: theme-packs
  Render: render-pipeline
---

# Shared words and areas

reel is split into areas that each own one job and one set of words. A word
means the same thing in the code, the script, the CLI output and these pages.
Code in one area may use the areas it points to, never the reverse.

```mermaid
flowchart RL
    Resolve[Resolve] --> Script[Script]
    Resolve --> Composition[Composition]
    Resolve --> Theme[Theme]
    Theme --> Composition
    Composition --> Kernel[Shared kernel: positions, problems, selectors]
    Script --> Kernel
    Render[Render] --> Kernel
    App[Use cases] --> Resolve
    CLI[reel command] --> App
```

Composition is the core. It does not know script syntax, drawing libraries or
video formats. Resolve translates a script into composition terms, so the
script language can change without touching the core. Render only sees a
display list, so a different drawing library is a new adapter, not a rewrite.

## Areas

| Area | Package | Owns |
| --- | --- | --- |
| Script | `internal/script` | Reading `.reel` text into lines, nesting and tokens |
| Shared kernel | `internal/diag`, `internal/selector` | Positions, problem reports, the selector language |
| Composition | `internal/composition` | Video, scenes, elements, steps, style rules, the cascade |
| Theme | `internal/theme` | Theme packs: tokens, default rules, components |
| Resolve | `internal/resolve` | Turning script lines into composition, with checks |
| Render | `internal/render` | Display lists and the rasterizer port and adapters |
| Use cases | `internal/app` | Check and explain today; still and render later |
| Interface | `cmd/reel` | The command line |

Motion and Encode will be their own areas between Composition and Render, and
after Render.

## Words

| Word | Meaning |
| --- | --- |
| Video | One script's result: title, size, frame rate, theme, scenes |
| Scene | One shot. Holds what is on screen and the steps that happen |
| Element | One thing on screen: a primitive, a component instance, a part, a layout or a connector |
| Primitive | A built-in drawable kind such as box, circle or label |
| Component | A kind defined from other kinds with `define`, such as server |
| Part | A named element inside a component instance, addressed as `#api/unit1` |
| Connector | An element joining two others, written `a -> b` |
| Step | Something that happens during a scene, such as focus or leave |
| Sequence | A named, reusable list of steps |
| Selector | A pattern that picks elements, such as `#api process:not(.hot)` |
| Rule | A selector with property values, from a theme or a style block |
| Cascade | The fixed order that decides which value wins when rules disagree |
| Theme pack | Tokens, default rules and components that set the look |
| Display list | The drawing operations for one frame, independent of any library |
| Rasterizer | A port that turns a display list into pixels |
| Diagnostic | A problem with a position, severity and fix hint |
