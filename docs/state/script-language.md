---
id: script-language
title: Script language
kind: contract
status: implemented
summary: Every line is a kind followed by arguments, quoted text and key=value pairs, nested by indentation.
parent: reel
sources:
  - internal/script/parse.go
  - internal/script/parse_test.go
  - internal/selector/selector.go
  - examples/cache-refresh.reel
diagramLinks: {}
---

# Script language

A `.reel` script is plain text. Every line names a kind, then any arguments,
quoted text and `key=value` pairs. Indented lines belong to the line above.
The parser does not decide what a word means; [checking a script](check-a-script.md)
does that. This keeps new components and plugins from needing parser changes.

```text
scene request
  #web -> #api "GET /v1/items" for=1.5s
  focus #api/unit1 at=2s
  with dim process:not(.hot)
```

## Lines

- The first word is the kind, such as `box`, `scene` or `focus`.
- `"…"` is visible text. `\"` and `\n` work inside it.
- `key=value` and `key="a value"` set properties.
- A line whose second word is `->`, `-|`, `~>`, `..>`, `<->`, `--` or `..`
  is a connector.
- `with` at the start runs a step together with the previous one.
- `# ` starts a comment. `#name` without a space is an id.
- `terminal`, `code`, `diff`, `chart`, `timeline`, `mermaid` and `math` keep
  their indented lines exactly as written.
- In quoted text, `\"` is a quote, `\\` a backslash and `\n` a new line. On a
  `math` line other backslashes are left for TeX.
  So does a block between two ```` ``` ```` lines, such as Mermaid.

## Mistakes the parser reports

Unclosed quotes, tabs used for indentation, uneven indentation, a connector
without a target, `with` on its own and an unclosed ```` ``` ```` block. Each
report has a line, a column and a hint. Parsing continues after a mistake so
that one run reports everything.

## Selectors

Selectors pick elements in style rules, steps, connectors and the camera.

| Selector | Picks |
| --- | --- |
| `process` | every element of that kind |
| `#api` | the element with that id |
| `.hot` | elements with that class |
| `#api/unit1` | a part inside a component |
| `#api process` | processes anywhere inside `#api` |
| `row > box` | boxes directly inside a row |
| `[state=dim]` | elements whose own line sets that value |
| `:first`, `:last`, `:nth(2)`, `:not(…)` | position and negation |
| `#a, #b` | either |

`[state=dim]` reads only what is written on the element's line, not the
styled result. Otherwise a rule could change whether it applies to itself.
