# reel

reel makes short explainer videos about software from a plain text script.
You, or an AI agent, describe each scene: the code, diagrams and boxes to
show, and what to say about them. reel draws and animates it, reads the
narration aloud and saves an MP4.

A voice is built in and runs on your computer, so videos can be narrated
with no account and no internet connection.

## What it's for

- **Pull request descriptions.** Ask your coding agent to explain its PR as
  a video. A two-minute walkthrough of the problem, the fix and the edge
  cases is easier to review than a wall of text, and it can be attached to
  the PR.
- **Explaining how something works.** Request flows, data structures,
  caches, queues, load balancers and algorithms, shown moving step by step.
- **Design notes and onboarding.** A short narrated tour of a system for
  someone new to it.

## How fast it is

Measured on an Apple M5 laptop (10 cores), 1920×1080 at 30 fps:

| Video | Length | Narration | Time to render |
| --- | --- | --- | --- |
| Pull request walkthrough, 11 scenes | 2 min 33 s | 22 lines, spoken fresh | 17 s |
| Same video, rendered again | 2 min 33 s | lines reused | 9 s |
| Data structures demo, 5 scenes | 58 s | 8 lines, reused | 3.2 s |
| Code review example | 19 s | none | 1.3 s |

Video renders at about 17 times real time, and speech adds about 3 seconds
per minute of narration the first time. A re-render only speaks the lines
that changed, so editing a script and rendering again takes seconds. The
two-and-a-half-minute PR video is a 4.7 MB MP4.

> **AI agents:** read [docs/agents/README.md](docs/agents/README.md) first.
> Once reel is installed, `reel help` explains the workflow and
> `reel skill` prints everything needed to write a good script.

## Install with Homebrew

```sh
brew install nessalabs/tap/reel
```

## Or download a release

Every version is on the [releases page](https://github.com/nessalabs/nessa-video/releases),
for macOS (Apple silicon and Intel) and Linux. To install the latest one in
a single step:

```sh
curl -fsSL https://raw.githubusercontent.com/nessalabs/nessa-video/main/install.sh | sh
```

## Make a video

```sh
reel render examples/cache-refresh.reel
```

Run `reel help` to see every command. To teach your agent to write scripts,
run `reel skill install`, or point it at [the reel skill](skills/reel/SKILL.md).

## License

GPL-2.0-or-later. Bundled fonts and libraries are listed in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).
