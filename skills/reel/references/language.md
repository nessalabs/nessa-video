# reel language reference

## Shape of a script

```text
video "PR 482 · one refresh per key"
  size 1920x1080  fps 30          # 1080x1920 for phones
  theme hairline                  # default; or a folder such as themes/brand
  voice phonon                    # optional narration, local, Harper unless named; or: voice elevenlabs VOICE_ID (key from ELEVENLABS_API_KEY or .env)

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
| `box`, `card`, `circle`, `decision`, `store`, `stack` | shapes; text inside with `"…"`. A `card` wraps its text (about 250 wide, or `w=`) and takes `title="file.rs"` for a header above a rule |
| `title`, `p`, `label` | text; `title "a ==b=="` brightens b |
| `metric "1024" label="records kept" note="GRANT_LOG_RETAINED, share_grants.rs"` | a big number with a caption and where it comes from; `focus` underlines it |
| `row`, `col`, `grid cols=N`, `layer`, `place` | layout: `gap`, `align`, `justify`, `pad`, `w`, `h`, `x`, `y`; `flow=live` gives room only to children on screen, so `show`/`leave` inside it make the rest glide; `sides=left,right,bottom` (or `all`) draws those edges, e.g. an open bin |
| `table` | rows of cells: each child is a `row`, its children the cells (`label` for a header, `p` for values). Columns line up, rules part the rows, and rows are live: `show`/`leave` a row, `set` a cell, `focus` a row to highlight it |
| `tree kind=binary` | each child is a node (`circle "50"`), a node's children its children; nodes never move when one is shown later, so `show #n90` grows the tree in place. In a binary tree a lone child goes left unless it has `side=right`. Lines from parent to child draw in with each node |
| `meter "2" max=5` | load or fill as slots (`kind=bar` for a bar, `kind=dots r=5` for round tokens that grow back as they refill, `unit="tokens"` for a count under it); `set #m "4"` fills it smoothly |
| `badge "leader"` | a small tag pinned to the top right of the element it is written inside |
| `timer "TTL"` | a ring that empties while `play #t for=5s` runs, full again at each play; `idle=hide` shows it only while it runs |
| `status "Closed" tone=good` | a toned dot and a label; `set #s "Open" tone=bad` changes both smoothly, `play #s for=4s` shows a small countdown ring beside the label |
| `aside #why layout=left` | a column shown only during an `isolate` step that names it; `layout=left` puts the isolated element left and the aside right, `center` puts the aside below, `right` mirrors left |
| `laptop`, `phone`, `server`, `process`, `db` | theme components; parts like `#api/unit1`, `#api/procs` |
| `breaker "Closed"`, `limiter tokens=6`, `cache count=3`, `entry "user:3" value="Ada"` | system components: a breaker with its state above and failure dots (`#cb/fails`) below; a limiter with token dots (`#rl/tokens`) and a count; a cache list holding `entry` rows, newest first, with `#c/count`. Connectors meet their framed part (`anchor=true`); `sides=all r=12` draws such a rounded frame on any row or col |
| `list`, `node`, `queue`, `cell`, `lifo`, `block`, `bucket`, `balancer` | collections: put items inside (`node "3"`, `cell "A"`, servers in a `balancer`); `show` adds an item where it is written, `leave` removes it, the rest glide. `lifo` is a stack: an open bin that fills from the bottom, holding `block "x"`s, top item first. Show a hash map as a `table` (key, bucket, value), or as `bucket "0"` rows holding `node`s for its chains |
| `arrow "label" dir=right len=40` | a short arrow on its own, `dir=left|down|up` |
| `a -> b "label"` | connector; also `-|`, `~>`, `..>` (dashed), `<->`, `--`. An end matching several elements (`#lb -> .server`) draws one connector to each |
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
| `git` | indented commands, one per line: `commit a1`, `branch feature` (or `switch -c feature`), `checkout main`, `merge feature` (`merge feature ff` fast-forwards), `rebase main`; Mermaid `gitGraph` lines work too. Each branch gets its own lane and colour; `play #g` runs the commands one by one, `play #g to=4` up to the fourth |

Elements stay on screen into the next scene unless the scene starts with
`clear` (or `clear keep=#id`). Ids must be unique within what is on screen.

`reel ref` lists every kind with the properties it takes.

## Steps

Steps run one after another after the scene's elements have entered.

| Step | Does |
| --- | --- |
| `focus`, `dim`, `active`, `select`, `trace`, `rest` `SELECTOR` | change state |
| `leave SELECTOR` | play its exit |
| `show SELECTOR` | draw it in now instead of when the scene opens; with `on="words"`, as it is named |
| `say "heading"` | the scene's heading at the top; spoken when the video has a voice, with the spoken words as subtitles at the bottom; `say "short heading" speak="the spoken sentence"` is the usual form. The next step waits until the line is spoken; use `with` to run a step during it |
| `camera zoom SELECTOR`, `camera reset` | move the view |
| `send SELECTOR` | dot along the connector starting there; `send #lb to=#s2` picks the one ending at `#s2`. Add `reply` for a dot that comes back after a moment, or `drop` for one that turns red and falls where it arrives. `send #a "req 1" reply="{good:200}"` sends a card that turns into the reply at the far end; a connector a reply travels draws two lanes, requests above and replies below (`lanes=1|2` on the connector to choose). `send #a to=#db …` passes through every element between; `spend=#rl` takes a token from a meter or limiter |
| `ping #lb to=#s2` | a health check: a small dot that turns green as it arrives, or red if the target is down |
| `fail SELECTOR` / `recover SELECTOR` | put an element in the down state (red tint, red text; connectors to it turn red and dashed) and back; colours change smoothly |
| `isolate SELECTOR aside=#why backdrop=dim` | lift one element out of the scene: it grows and moves to the spotlight while the rest dims (`backdrop=hide` hides it); a dashed outline marks where it was, and the aside comes in beside it. Steps that follow can show things in the aside |
| `return SELECTOR` | end the isolation: the aside leaves (each part with its own `exit=`; `keep=true` parts stay; `order=together` or `order=cut`), and the element glides back |
| `move SELECTOR to=#y` | any element travels to `#y`'s place and stays; `move #x to=#z` continues from there, `move #x` goes home. `move #e first` (or `last`) moves a child to the front of its list among those on screen; the rest glide |
| `send … carry=#req` | any element rides the connector instead of the dot (and comes back with `reply`); give it `travel=true` so it takes no room and is seen only while it rides |
| `show SELECTOR from=#src` | the element flies out of `#src` as it appears (a value copied out of a tree into a list); it flies back on `return`. `leave #x to=#y` flies to `#y` as it goes |
| `set SELECTOR "text"` | change an element's text in place (a counter, a value), cross-fading; `set #n "4"` on a `node` changes its value. `tone=` and `outline=` change colours smoothly (`set #cb "Open" tone=bad`, `set #b outline=good`); on a component a tone goes to the parts toned like its first toned part |
| `type SELECTOR` | type out a terminal, code or text block |
| `mark SELECTOR lines=…` / `cells=…` | move a code or array focus; `lines=+12..+14,+20` takes several ranges |
| `ptr #a lo=2` / `swap #a 0 3` | move pointers, swap cells |
| `walk #flow A B C` | light diagram nodes in order; on a `mermaid #flow reveal=steps`, bring them in one by one as well |
| `play #tl` | sweep a timeline playhead, or reveal a sequence diagram's rows; after a `say`, it unfolds over the rest of that line. `play #seq to=2 on="…"` reveals up to the second row, so each message appears as it is told |
| `wait 1s`, `hold 2s` | pause; extend the end |

Timing: `on="phrase"` starts a step just before the last `say` line
speaks those words, so a highlight lands as it is named:

```reel
say "Two checks" speak="First the cache, then the database answers."
focus #cache on="cache"
focus #db on="database"
```

The words come from the voice's own timings, or are estimated from their
place in the line when there is no voice. `reel check` reports a phrase the
line never says. `with` before a step runs it alongside the previous one, `at=2s` or
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
