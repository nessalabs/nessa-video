---
name: reel
description: Write a .reel script and render a short explainer video with the reel CLI. Use when asked to make a video, animation or visual walkthrough of a code change, pull request, system design, algorithm, or incident analysis, or to update an existing .reel script.
---

# reel

reel turns a plain text script into a narrated explainer video from one binary.
You write the `.reel` script; `reel check` catches mistakes and timing issues,
`reel still` renders each scene as a PNG so you can inspect the layout, and
`reel render` writes the MP4. Read [the language reference](references/language.md)
for element and step syntax, and [explaining a change](references/explaining.md)
for the cognitive-load rules behind these guidelines.

## How to tell an interactive-feeling story

A linear video feels interactive when the viewer is constantly predicting what
will happen next and watching the system prove them right or wrong.

1. **Start with a hook, skip the glossary.** Open with the visible failure,
   surprising behavior, or hard constraint in the first 5–10 seconds. Introduce
   project-specific names only right before the viewer needs them.
2. **Carry one concrete example through the whole video.** Pick one named
   request, user, or record (like `order #42` or `req 1`) and follow that same
   item from failure to fix to edge cases. Switching examples every scene breaks
   continuity.
3. **Use a predict → run → explain loop.** Before animating a failure or a
   tricky case, pose the concrete question ("Two requests hit an empty cache at
   once — what hits the database?"), pause a beat, run the animation so the
   viewer watches it happen, then `zoom` or `isolate` to show why.
4. **Reuse the same layout across scenes.** Leave out `clear` (or use
   `clear keep="#id"`) when the story continues so elements glide into place
   and the viewer can see what changed without relearning the diagram.
5. **Stick to the source.** Never invent numbers, timestamps, latencies, or
   test output. When a timeline only shows ordering, set `unit=none`.

## Pick the arc for the job

Skip any beat that does not apply, and split a video if it tries to cover two
unrelated topics. Aim for two to four minutes.

### Code review / pull request
1. **Hook:** Show the bug or missing behavior on one concrete input.
2. **Before:** Animate how the old code handles that input and watch it fail.
3. **Fix:** Keep the same diagram shape, highlight what changed, and replay the
   exact same input through the new path.
4. **Edge cases:** Run one or two tricky inputs a reviewer would ask about
   (retries, restarts, empty state) on the same setup.
5. **Evidence:** Show real test results or benchmark numbers as `metric` or
   `card` blocks, noting where each number came from.

### Explaining a system or algorithm
1. **Hook:** Show the tension the design has to solve (load spike, race
   condition, memory limit, or slow lookup).
2. **Naive attempt:** Run one concrete input through the obvious approach until
   it breaks visibly (`fail`, `{bad:503}`, overflowing queue).
3. **The mechanism:** Name the 2–3 key parts right before they act, then step
   the same input through the real design while data visibly moves (`send`,
   `move`, `set`, `walk`, `play`).
4. **Tradeoff or limit:** Push the design with one stress case (burst traffic,
   cold cache, node failure) so the viewer sees where its boundaries are.

### Incident analysis / postmortem
1. **Impact first:** Start at the moment users or alerts felt the incident,
   with real timestamps and symptoms.
2. **The trigger:** Trace one real failing request or event into the system.
3. **Why safeguards missed it:** Pose the expectation ("The circuit breaker
   should have tripped here — why didn't it?"), then `isolate` or `zoom` into
   the exact condition that bypassed it.
4. **The fix under replay:** Replay the same fault against the fixed system and
   show the safeguard catching it.
5. **Numbers and follow-ups:** Close with verified duration, blast radius, and
   the concrete guardrail added.

## Visual pacing and narration

- **One idea per scene**, at most about 45 spoken words. `reel check` warns
  past that; split the scene instead of speeding up.
- **Short headings, spoken detail.** Keep on-screen headings to eight words or
  fewer, using `==…==` on the two or three words that matter. Put the full
  spoken sentence in `speak="…"`, and repeat the scene's heading on each `say`
  line in that scene.
- **Draw and highlight as you speak.** Put a scene's first `say` right after
  `clear` so narration starts as the scene draws in. Sync highlights and
  reveals to spoken phrases with `on="the words"` (`focus #log on="grant log"`,
  `show #x on="…"`, `play #seq to=2 on="…"`), and dim what you already covered.
  Use `with` when a step should run during a `say` line.
- **Animate what changes, keep structure still.** Keep at most about seven
  labelled things on screen at once. Use `camera zoom`, `isolate` + `aside`, or
  a new scene instead of crowding.
- **Reserve colour for outcomes.** Use `{bad:…}` / `tone=bad` for failures and
  bugs, `{good:…}` for passes and fixes, `{warn:…}` for risks, and leave
  everything else neutral grey.
- **Show code sparingly.** Only put code on screen when its exact text is the
  point (an API shape, config key, or short fix), loaded via `file=` with
  `focus=` or `mark` so line numbers stay real. Keep it to 12 rows or fewer.
- **Write narration for the ear.** Talk to one teammate using "you" and
  contractions, link scenes smoothly ("Now watch what happens on retry."), keep
  sentences short with subject and verb up front, and never read code
  identifiers aloud. Mix sentence lengths so the voiceover doesn't drone, and
  skip stock "it's not X, it's Y" contrasts, aphorisms, and punchline tag
  endings ("That's the bug.").

## Choosing visual primitives

Prefer built-in components and live structures over hand-placed boxes; see
[the language reference](references/language.md) for full syntax and `reel ref`
for every built-in kind, step, and property.

- **Data structures & flows:** `list`, `queue`, `lifo`, `table`, `tree`, and
  `git` blocks with `show`, `leave`, `move`, `walk`, and `play`.
- **Services & resilience:** `laptop`, `server`, `process`, `db`, `balancer`,
  `breaker`, `limiter`, `cache`, `meter`, `send … reply`, `ping`, `fail` /
  `recover`, and `carry=`.
- **Numbers & math:** bar/line charts and `timeline` animated via `set`,
  `sort`, `mark`, and `play`; `plot` blocks for curves, `zoom`, and `note`.
- **Narration & sizing:** Use `voice phonon Harper` for local offline speech
  (or `voice elevenlabs VOICE_ID` with `ELEVENLABS_API_KEY` from the
  environment or `.env`, never hardcoded). Set `size 1080x1920` for phone
  videos.

## Check, look, render

```sh
reel check video.reel                     # errors, warnings, and scene lengths
reel check --timeline video.reel          # exact start/end time of every step
reel still -o stills video.reel           # one PNG per scene, after its steps
reel still --at 42.5 -o f.png video.reel  # inspect a single moment
reel render -o video.mp4 video.reel
```

Fix every error `reel check` reports and check the timeline so each highlight
lands on the words that name it. If you can view images, inspect the stills
before rendering to catch overlaps, clipped text, or empty frames.

## Report

Tell the user where the video is, how long it is, and what each scene covers in
one line. State which numbers or outputs came from real runs and which are
illustrative examples.
