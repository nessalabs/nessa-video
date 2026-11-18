# reel language reference

## Shape of a script

```text
video "PR 482 · one refresh per key"
  size 1920x1080  fps 30          # 1080x1920 for phones
  theme hairline                  # default; or a folder such as themes/brand
  voice elevenlabs VOICE_ID       # optional narration; key from ELEVENLABS_API_KEY or .env

tokens                            # optional: override theme tokens
  line-2 #D4D7DE

style                             # optional: selector then key=value
  .hot  state=focus

define queue items=3 label=""     # optional: your own component
  col gap=8
    repeat items
      box #slot w=120 h=32
    label #label "{label}"

sequence request from to text="GET"   # optional: reusable steps
  {from} -> {to} "{text}"
  send {from}
  focus {to}

scene problem                     # scene NAME [LENGTH]
  ...elements and steps...
```

Every line is `kind #id .class "text" key=value`. Indented lines belong to
the line above. `# ` starts a comment. In quoted text, `\"` is a quote and
`\n` a new line; on `math` lines backslashes are left for TeX.

## Elements

| Kind | Notes |
| --- | --- |
| `box`, `card`, `circle`, `decision`, `store`, `stack` | shapes; text inside with `"…"` |
| `title`, `p`, `label` | text; `title "a ==b=="` brightens b |
| `metric "1024" label="records kept" note="GRANT_LOG_RETAINED, share_grants.rs"` | a big number with a caption and where it comes from; `focus` underlines it |
| `row`, `col`, `grid cols=N`, `layer`, `place` | layout: `gap`, `align`, `justify`, `pad`, `w`, `h`, `x`, `y` |
| `laptop`, `phone`, `server`, `process`, `db`, `queue` | theme components; parts like `#api/unit1`, `#api/procs` |
| `a -> b "label"` | connector; also `-|`, `~>`, `..>` (dashed), `<->`, `--` |
| `code LANG` | indented lines or `file=PATH lines=11..15`; `focus=3..4` |
| `diff` | indented `git diff` or `file=PR.diff path=FILE hunk=N`; `focus=+12..+14` (new) or `-12` (old) |

Line numbers in `focus=` and `mark lines=` are the file's own numbers, the ones
`reel still` shows in the gutter. A long block is folded to the lines around
its focus and every line a `mark` in the scene names (`context=3` lines either
side by default), so it stays readable; folded runs show as "N lines".
`reel check` warns when a focus names lines the block does not show.
| `terminal` | indented lines; `$ ` starts a command |
| `array 1 3 4` | `ptr="lo=0 hi=2"` (0-based) |
| `chart bar` / `chart line` | indented CSV: header row, then `label, v1, v2` |
| `timeline unit=ms` | indented lanes: `label | 0-18 wait | *18-22 done` (`*` = bright) |
| `mermaid` | indented `flowchart LR`/`TD` (nodes become parts `#flow/ID`) or `sequenceDiagram` with `participant … as …`, `->>`, `-->>` (reply), `-x`, `Note over A: …`, `alt`/`else`/`opt`/`loop`/`par` … `end`, `autonumber` |
| `math "TeX"` | formula |

Elements stay on screen into the next scene unless the scene starts with
`clear` (or `clear keep=#id`). Ids must be unique within what is on screen.

`reel ref` lists every kind with the properties it takes.

## Steps

Steps run one after another after the scene's elements have entered.

| Step | Does |
| --- | --- |
| `focus`, `dim`, `active`, `select`, `trace`, `rest` `SELECTOR` | change state |
| `leave SELECTOR` | play its exit |
| `say "caption"` | caption at the bottom; spoken when the video has a voice, highlighting each word as it is said; `say speak="…"` shows and speaks the same words; `say "short" speak="longer"` speaks more than it shows. The next step waits until the line is spoken; use `with` to run a step during it |
| `camera zoom SELECTOR`, `camera reset` | move the view |
| `send SELECTOR` | dot along the connector starting there |
| `type SELECTOR` | type out a terminal, code or text block |
| `mark SELECTOR lines=…` / `cells=…` | move a code or array focus; `lines=+12..+14,+20` takes several ranges |
| `ptr #a lo=2` / `swap #a 0 3` | move pointers, swap cells |
| `walk #flow A B C` | light diagram nodes in order |
| `play #tl` | sweep a timeline playhead, or reveal a sequence diagram's rows |
| `wait 1s`, `hold 2s` | pause; extend the end |

Timing: `with` before a step runs it alongside the previous one, `at=2s` or
`at=+0.5s` sets its start, `for=1.5s` its length, `scene name 6s` the
scene's length. Otherwise everything is automatic.

## Selectors and style

`kind`, `#id`, `.class`, `#id/part`, `a b` (inside), `a > b` (child),
`[state=dim]`, `:first`, `:last`, `:nth(2)`, `:not(…)`, `a, b`. Steps take
groups too: `focus #a, #b`.

Values come from, lowest to highest: theme defaults, theme rules, the
script's `style` block (more specific selectors win), the element's own line.
`enter` and `exit` take `draw`, `fade`, `rise`, `type` or `none`.

## Phones and other shapes

`size 1080x1920` renders a vertical video from the same script. The engine
turns rows into columns, turns wide flowcharts top-to-bottom and scales what
still does not fit. To adjust by hand:

```text
style portrait
  #stack  dir=col
  .aside  hidden=true
```

`dir=row|col` turns a container, `hidden=true` leaves an element out, and
`fit=false` stops the engine from turning a container.

## Themes

`reel themes` lists built-in packs; hairline is the default.
`reel theme export hairline` copies it to `themes/hairline/theme.reel` to edit.
A pack can start with `extends hairline` and change only some tokens, rules
or components.
