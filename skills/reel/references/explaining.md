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

## Plain language

Define a term before using it and keep using exactly that term (Julia
Evans, "Patterns in confusing explanations"). Keep the real name so people
can search for it, next to a plain description. Sentences average about 15
words, at most 25 in writing and less when spoken (GOV.UK; Content Design
London).

## Rules

| Rule | From | Checked by |
| --- | --- | --- |
| Order: names, problem, concrete case, mechanism, edge cases, evidence | Tao; Pennington; pre-training; concrete first | the writer |
| One idea per scene, at most about 45 spoken words | segmenting; transient information | `reel check` warns |
| Captions of 8 words or fewer; the sentence goes in `speak=` | redundancy | `reel check` warns |
| Narration between 130 and 210 words a minute | speech rate | `reel check` warns, with a voice |
| Highlight what is named, as it is named, one at a time | signalling; contiguity | `focus #x on="the words"` after the `say` |
| Animate change, keep structure still, about 7 labelled things at most | animation; coherence | the writer |
| Show code only when its exact text is the point, at most 12 rows | coherence; Pennington | `reel check` warns |
| Name identifiers once, next to a plain name | pre-training; Evans | the writer |
| Two to four minutes; split a PR that does several things | length; Barnett | the writer |
| No music under the voice | coherence | not supported, on purpose |
