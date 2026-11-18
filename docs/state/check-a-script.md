---
id: check-a-script
title: Check a script
kind: flow
status: implemented
summary: reel check reads a script, applies the theme, expands components and sequences, styles every scene and reports problems.
parent: reel
sources:
  - internal/resolve/resolve.go
  - internal/resolve/resolve_test.go
  - internal/app/check.go
  - cmd/reel/main.go
diagramLinks:
  Parsed: script-language
  Styled: cascade
diagramDetails:
  - diagram: 0
    node: Parsed
    section: Reading the lines
  - diagram: 0
    node: Resolved
    section: Building the scenes
  - diagram: 0
    node: Styled
    section: Styling every scene
  - diagram: 0
    node: Validated
    section: Final checks
  - diagram: 0
    edge:
      from: Validated
      to: Failed
      label: any error
    section: When the check fails
---

# Check a script

`reel check` reads a script and reports every problem it can find in one run,
each with a line, a column and a hint. It exits with 0 when there are no
errors, even if there are warnings, and 1 otherwise. Nothing is drawn.

```mermaid
stateDiagram-v2
    [*] --> Parsed: read file
    Parsed --> Resolved: theme loaded
    Resolved --> Styled: rules applied
    Styled --> Validated: targets checked
    Validated --> Passed: no errors
    Validated --> Failed: any error
```

## Reading the lines

The [script language](script-language.md) parser turns text into nested
lines. Mistakes here do not stop the check. The rest of the steps run on
whatever could be read, so later problems are still reported.

## Building the scenes

The theme named in the `video` block is loaded first. Its style rules become
the lowest cascade layer and its `define` blocks become components such as
`server` and `laptop`. Then the script's own style rules, components and
sequences are collected.

Each scene starts with what was on screen at the end of the previous scene.
A scene that begins with `clear` starts empty, or keeps only what matches
`clear keep=…`. Each line then becomes one of these:

- A step, such as `focus #api` or `say "…"`.
- A sequence call, which expands into its steps and connectors.
- A part address such as `#api/procs`, whose indented lines are added inside
  that part.
- An element. A component expands into its parts. Inside a component, `#name`
  names a part, `{param}` is replaced, `repeat` copies lines with `{i}`, and
  `slot` is where the caller's indented lines go.

Unknown kinds, properties, themes and parameters are reported with the
closest valid name.

## Styling every scene

Every element gets its final property values from the [cascade](cascade.md).
Rules are matched after the whole scene is built, so a rule can target parts
inside components.

## Final checks

Each scene is checked for ids used twice, including ids still on screen from
an earlier scene. Every step target and connector end must match something on
screen. Inside a component, connector ends refer to that component's own parts.
Values with a fixed vocabulary, such as `enter`, `exit`, `state` and
durations, are checked where they are written.

## When the check fails

Any error makes `reel check` exit with 1. Warnings, such as an unknown
property or a style rule that names no known kind, are printed but do not fail
the check. The same report is available as JSON with `--json`.

Repeated problems from elements carried into later scenes are reported once.
