# Explaining a change

What helps people understand a narrated diagram video, and the rules the
skill takes from it. Evidence comes first and practice second; each rule
names where it comes from.

## What reviewers need

- The reason for a change is what engineers most need, and the change
  description is where they look for it (Tao et al., FSE 2012).
- Reviewing is mostly understanding the change; tools support that poorly
  (Bacchelli & Bird, ICSE 2013).
- People understand code top-down: a guess at its purpose, confirmed by
  familiar cues (Brooks 1983). They hold two models, the code's control flow
  and what it does in the world (Pennington 1987). A diagram carries the
  second far better than a listing.
- Large changes that mix several concerns are hard to review; splitting them
  helps (Barnett et al., ICSE 2015; Ram et al., ESEC/FSE 2018).

## How people learn from narrated pictures

- **Segmenting**: short meaningful chunks beat one stream (Rey et al. 2019,
  meta-analysis of 56 studies). Pauses help, and so do the boundaries
  themselves. For experts, keep pauses short (Spanjers et al. 2010–2012).
- **Transient information**: speech disappears as it plays, so long spoken
  passages overload memory; shorter segments fix it (Leahy & Sweller 2011;
  Wong et al. 2012). Keep the picture still while it is spoken about.
- **Modality and redundancy**: pictures with narration beat pictures with
  on-screen text, and the full narration printed beside a picture hurts.
  Short key phrases next to the picture are fine (Mayer; Noetel et al. 2022,
  overview of 29 reviews).
- **Signalling**: highlighting what is being talked about helps (Richter,
  Scheiter & Eitel 2016), and so does showing words and pictures together in
  place and time (contiguity, Ginns 2006).
- **Coherence**: leaving out extra material helps; decoration and tangents
  cost (Mayer & Fiorella 2014).
- **Pre-training**: naming the parts first helps people follow a process
  (Mayer, Mathias & Wetzell 2002).
- **Expertise reversal**: support novices need slows experts down (Kalyuga
  et al. 2003). Engineers know programming but not this codebase: explain
  its own terms, skip generic ones, keep it brisk.
- **Animation**: helps most when the change over time is what must be
  learnt; animate only what changes, simply enough to follow (Höffler &
  Leutner 2007; Berney & Bétrancourt 2016; Tversky et al. 2002).
- **Concrete first**: one concrete case before the general rule improves
  transfer (Fyfe et al. 2014; Bret Victor, "Up and Down the Ladder of
  Abstraction").
- **Speech rate**: comprehension holds up to about twice normal lecture
  speed (Murphy et al. 2022); faster speakers keep attention (Guo, Kim &
  Rubin 2014). Dense diagrams need more care than lectures.
- **Length**: engagement falls off past about six minutes and is highest
  under three (Guo, Kim & Rubin 2014; correlational).

## Plain language, for the ear

- **Jargon costs even when defined.** Technical terms made text feel harder,
  less interesting and less persuasive, and adding definitions did not
  remove the cost (Bullock et al. 2019; Shulman et al. 2020). Leaving a term
  out beats defining it.
- **The writer cannot judge.** Experts badly underestimate what others do
  not know, and knowing about the bias barely helps (Camerer, Loewenstein &
  Weber 1989; Hinds 1999). An agent that has read the code is the expert
  here, so check the words mechanically, not by feel.
- **Plain words read as clearer and smarter.** Needlessly complex words
  lower ratings of the text and the author (Oppenheimer 2006); readers,
  judges included, understand plain versions faster and better (Kimble).
- **Listening is harder than reading.** A listener cannot go back, so
  working memory limits what they follow (Jiang & Farquharson 2018).
  Broadcasters write one new idea per sentence, subject and verb first, no
  stacked clauses (NPR Training). Announcing structure ("in three cases…
  one… two…") helps recall (Richards & Chaudron 1986). Passive and unusual
  word orders are misheard more (Ferreira 2003).
- **Concrete beats abstract.** Concrete text is understood, enjoyed and
  remembered better (Sadoski, Goetz & Fritz 1993; Sadoski et al. 2000).
- **Analogies need care.** Comparing cases helps when the mapping is
  explicit (Gentner, Loewenstein & Thompson 2003); a single loose analogy
  can plant misconceptions (Spiro et al. 1989).

### Writing rules for narration

1. One new idea per sentence; about 15 words, never more than 25.
2. Subject and verb in the first six words. Not "With a cursor, it…" but
   "Your gateway sends its bookmark."
3. Active voice, and name who acts: your gateway, the peer, the user.
4. At most two new terms in a whole video. Everything else is described.
5. Describe the idea first, then name it once: "it remembers the last
   change it read; we'll call that its bookmark." Then always that word.
6. Never read code names aloud: no `grants.subscribe`, `GrantLogFloor`,
   `incarnation`, `head`, `tombstone`. Say what they do.
7. Give every abstract claim a concrete case: "say it last read change 40".
8. Count lists out loud: "in three cases. One… Two… Three…".
9. Open with the point, end by saying it again.
10. Use everyday words: "started over" not "new incarnation", "newest" not
    "head", "removed" not "tombstoned", "connects" not "subscribes".
11. Labels on screen follow the same rules; a code name may appear once,
    small, beside its plain description, if a reviewer needs it.
12. Vary sentence length so the voiceover doesn't drone, and skip stock
    "it's not X, it's Y" contrasts, aphorisms, and punchline tag endings
    ("That's the bug.").

Before: "With a cursor, it sends only later records. Another incarnation, a
position past the head, or one older than the log is refused."

After: "Your gateway sends its bookmark. The peer sends only what came
after it. A bookmark is turned down in three cases. One: the log has
started over since. Two: it points past the newest change. Three: it points
at changes the log has already dropped."

## Rules

| Rule | From | Checked by |
| --- | --- | --- |
| Order: hook on one concrete input, predict-and-run failure, name parts right before mechanism, fix on the same input, edge cases, evidence | Tao; Pennington; pre-training; concrete first | the writer |
| Carry one named concrete input across the arc | concrete first; coherence | the writer |
| One idea per scene, at most about 45 spoken words | segmenting; transient information | `reel check` warns |
| Headings of 8 words or fewer; the sentence goes in `speak=` and shows as subtitles | redundancy | `reel check` warns |
| Narration between 130 and 210 words a minute | speech rate | `reel check` warns, with a voice |
| Highlight what is named, as it is named, one at a time | signalling; contiguity | `focus #x on="the words"` after the `say` |
| Animate change, keep structure still, about 7 labelled things at most | animation; coherence | the writer |
| Show code only when its exact text is the point, at most 12 rows | coherence; Pennington | `reel check` warns |
| Never read code names aloud; describe them | jargon; curse of knowledge | `reel check` warns |
| Spoken sentences under 25 words, varied cadence, one idea each | listening; NPR | `reel check` warns |
| At most two new terms per video, named once after a plain description | jargon; pre-training | the writer |
| Two to four minutes; split a PR that does several things | length; Barnett | the writer |
| No music under the voice | coherence | not supported, on purpose |
