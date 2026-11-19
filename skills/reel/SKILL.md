---
name: reel
description: Write a .reel script and render a short explainer video with the reel CLI. Use when asked to make a video, animation or visual walkthrough of a code change, pull request, algorithm, request flow or system design, or to update an existing .reel script.
---

# reel

reel turns a short text script into an explainer video from one binary.
You write the script; `reel check` finds mistakes, `reel still` shows each
scene as a picture, and `reel render` writes the MP4. Read
[the language reference](references/language.md) before writing a script.

## Plan the story first

The video exists so a person understands the change with less effort than
reading it. Explain with pictures of what the code does, not with the code.
Read [explaining a change](references/explaining.md) for the research behind
these rules. Read the diff and the code around it, then tell the story in
this order, skipping a beat rather than padding it:

1. Names: the two to four project-specific things the rest relies on, each
   with a plain name and a one-line meaning. Skip generic ideas such as
   what a cache is.
2. Problem: one concrete case of what went wrong or was missing, and how the
   system behaved before, as a diagram.
3. Mechanism: the new behaviour, traced with one concrete example first
   (real values moving through an array or a sequence), then the rule.
   Reuse the before diagram's shape and highlight only what changed.
4. Edge cases: only the ones the change handles or a reviewer would ask
   about, each as a concrete input.
5. Evidence: tests and numbers from the PR or a real run, as metrics or
   cards.

If the PR does two independent things, say so up front and give each its
own short arc. Leave out renames, formatting and moved files.

Say only what the source shows. Never invent numbers, timings or test output
for a chart, timeline or terminal; a timeline that shows order only takes
`unit=none`.

## Show, then say

- One idea per scene, at most about 45 spoken words. `reel check` warns past
  that; split the scene.
- Captions are key phrases of eight words or fewer. Put the full sentence in
  `speak="…"`, never on screen.
- Narration sentences: about 15 words, one idea each, active voice. Use one
  term for one thing the whole way through.
- Name a thing in plain words first. Show an identifier at most once, next
  to its plain name (`card "the position is past the head" title="CursorAhead"`).
- Highlight exactly what the voice names, when it names it, one thing at a
  time: after the `say` line, start each highlight with `on="the words"`
  (`focus #log on="grant log"`) and dim what was shown before.
- Animate what changes (a pointer moves, a message is sent, a step runs).
  Keep structure still. At most about seven labelled things on screen.
- Show code only when its exact text is the point: an API shape, a config
  key, a one-line fix. Then at most a few focused lines, in one scene.
  `reel check` warns when more than 12 rows of code are on screen.
- Aim for two to four minutes.

## Write the script

- Narration is optional. If the person wants a voice, add
  `voice elevenlabs VOICE_ID stability=0.4 style=0.3` with a conversational
  built-in voice (Chris is `iP95p4xoKVk53GoZ742B`). Write every line as
  `say "short heading" speak="what you would say to a colleague"`: the
  heading shows at the top, and the spoken words show as subtitles at the
  bottom. Repeat a scene's heading on each of its lines.
- Write the narration as one talk, not a list of slides: link scenes ("So
  here's how it worked before.", "Now the follower side."), use "you" and
  contractions, and keep sentences short. Put a scene's first `say` right
  after `clear` so the voice starts as the scene draws in. A `say` holds the next step until it has been spoken;
  start a step with `with` to show it while the line is spoken
  (`say "…"` then `with mark #diff lines=+12..+14`). Never
  write an API key into a script; it comes from `ELEVENLABS_API_KEY` or a
  local `.env`. If no key is available, render with `--mute`.
- Use `==…==` for the two or three words of a heading that matter.
- Use `camera zoom` or a new scene instead of crowding.
- Keep one picture across scenes when the story continues: leave out
  `clear` to keep everything, or `clear keep="#flow"` to keep one diagram.
  Kept elements glide to their new place while new ones draw in beside
  them. Use `clear` alone only when the next idea needs a new picture.
- Prefer components (`laptop`, `server`, `process`, `db`, `queue`) and
  blocks (`mermaid`, `array`, `timeline`, `metric`, `card`) over
  hand-placed boxes.
- When code is the point, read it with `file=` rather than pasting it, so
  line numbers match, and focus the lines with `focus=` or `mark`; long
  hunks fold down to those lines automatically.
- Show where a number comes from: `metric "1024" label="records kept"
  note="GRANT_LOG_RETAINED in share_grants.rs"`.
- Keep the theme's look. Change a token only when asked, in a `tokens` block.
- For a phone video, set `size 1080x1920`. Check the stills; add a
  `style portrait` block only where the automatic fit is not good enough.

## Check, look, render

```sh
reel check video.reel                 # problems, plus each scene's length
reel still -o stills video.reel       # one PNG per scene, after its steps
reel check --timeline video.reel      # when every step starts and ends
reel still --at 42.5 -o f.png video.reel  # the frame at a moment
reel render -o video.mp4 video.reel
```

Fix every error that `reel check` reports; each one names the line and
suggests a fix. It also prints how long each scene plays, so you can aim for
a length before rendering (`--json` has the same numbers). If you can view images, look at the stills before rendering
and fix overlaps, empty scenes or text that runs off the frame.
`reel check --explain '#id' video.reel` shows why an element looks the way
it does. `reel ref` lists every kind, step and property. With narration,
check the timeline: each highlight should start with the line about it.

## Report

Tell the person where the video is, how long it is, and what each scene
shows in one line each. Say which numbers or outputs came from real runs and
which, if any, are examples.
