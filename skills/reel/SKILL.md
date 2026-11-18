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

A good video makes one point. For a code review, read the diff and the code
around it, then pick the story in four beats or fewer:

1. The problem, shown with the code or flow that has it.
2. The change, shown as a diff with the important lines focused.
3. Why it works: a flow, a timeline, an array or a chart.
4. Proof: a test run or a measured number, only if you have a real one.

Say only what the source shows. Never invent numbers, timings or test output
for a chart, timeline or terminal. If you have no measurement, leave that
beat out or label it plainly as an example.

## Write the script

- Narration is optional. If the person wants a voice, add
  `voice elevenlabs VOICE_ID` and keep captions short, putting the full
  sentence in `say "short caption" speak="the full spoken sentence"`. Never
  write an API key into a script; it comes from `ELEVENLABS_API_KEY` or a
  local `.env`. If no key is available, render with `--mute`.
- One idea per scene and one caption per scene. Use `==…==` for the two or
  three words that matter.
- About six elements per scene at most. Use `camera zoom` or a new scene
  instead of crowding.
- Start a scene with `clear` when its content is unrelated to the last one.
- Prefer components (`laptop`, `server`, `process`, `db`, `queue`) and
  blocks (`diff`, `code`, `mermaid`, `timeline`) over hand-placed boxes.
- Read diffs and source with `file=` rather than pasting them, so line
  numbers match. Focus the lines you talk about with `focus=` or `mark`;
  long hunks fold down to those lines automatically.
- Show where a number comes from: `metric "1024" label="records kept"
  note="GRANT_LOG_RETAINED in share_grants.rs"`.
- Keep the theme's look. Change a token only when asked, in a `tokens` block.
- For a phone video, set `size 1080x1920`. Check the stills; add a
  `style portrait` block only where the automatic fit is not good enough.

## Check, look, render

```sh
reel check video.reel                 # problems, plus each scene's length
reel still -o stills video.reel       # one PNG per scene, after its steps
reel render -o video.mp4 video.reel
```

Fix every error that `reel check` reports; each one names the line and
suggests a fix. It also prints how long each scene plays, so you can aim for
a length before rendering (`--json` has the same numbers). If you can view images, look at the stills before rendering
and fix overlaps, empty scenes or text that runs off the frame.
`reel check --explain '#id' video.reel` shows why an element looks the way
it does. `reel ref` lists every kind, step and property.

## Report

Tell the person where the video is, how long it is, and what each scene
shows in one line each. Say which numbers or outputs came from real runs and
which, if any, are examples.
