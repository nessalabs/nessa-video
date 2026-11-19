package voice

import (
	"math"
	"sort"
	"strings"
)

// Timing words from the audio itself, for voices that report none. Speech
// has quiet stretches where the speaker pauses: before the first word,
// after the last, and at commas and full stops. Spreading words evenly over
// the whole clip ignores those, so the highlight runs ahead of the voice
// before a pause and falls behind after it. Instead the speech is found,
// its pauses are matched to the text's punctuation in order, and words are
// spread by length only within each phrase.

const (
	alignHop      = 0.01 // seconds per energy frame
	minPause      = 0.12 // a quiet stretch this long is a pause
	quietFraction = 0.06 // frames below this share of the loud level are quiet
)

// EstimateWordsFromAudio times the words of text against the speech in pcm.
func EstimateWordsFromAudio(text string, pcm Speech) []Word {
	fields := strings.Fields(text)
	if len(fields) == 0 || len(pcm) == 0 {
		return nil
	}
	quiet := quietFrames(pcm)
	n := len(quiet)
	first, last := 0, n-1
	for first < n && quiet[first] {
		first++
	}
	for last > first && quiet[last] {
		last--
	}
	if first >= last {
		return EstimateWords(text, pcm.Seconds())
	}
	start, end := float64(first)*alignHop, float64(last+1)*alignHop

	// pauses inside the speech, as [from, to) in seconds
	type pause struct{ from, to float64 }
	var pauses []pause
	for i := first; i <= last; {
		if !quiet[i] {
			i++
			continue
		}
		j := i
		for j <= last && quiet[j] {
			j++
		}
		if d := float64(j-i) * alignHop; d >= minPause {
			pauses = append(pauses, pause{float64(i) * alignHop, float64(j) * alignHop})
		}
		i = j
	}

	// Phrases end at punctuation. Boundaries are matched to pauses in
	// order, each near where an even spread would put it, choosing the
	// matching with the least total distance; a boundary may go without a
	// pause (speakers run on through commas), at a cost.
	weight := func(w string) float64 { return float64(len([]rune(w))) + 2 }
	total := 0.0
	for _, w := range fields {
		total += weight(w)
	}
	type boundary struct {
		after  int     // index of the last word before it
		expect float64 // where an even spread puts it
		skip   float64 // cost of giving it no pause
	}
	var bounds []boundary
	acc := 0.0
	for i, w := range fields[:len(fields)-1] {
		acc += weight(w)
		switch w[len(w)-1:] {
		case ",", ";", ":", "—":
			bounds = append(bounds, boundary{i, start + (end-start)*acc/total, 0.25})
		case ".", "?", "!":
			bounds = append(bounds, boundary{i, start + (end-start)*acc/total, 0.6})
		}
	}
	const far = 1.2 // a pause further than this from its boundary never matches
	B, P := len(bounds), len(pauses)
	// cost[i][j]: best cost for boundaries i.. using pauses j..
	cost := make([][]float64, B+1)
	pick := make([][]int, B+1) // -1 skip, else the pause taken
	for i := range cost {
		cost[i] = make([]float64, P+1)
		pick[i] = make([]int, P+1)
	}
	for i := B - 1; i >= 0; i-- {
		for j := P; j >= 0; j-- {
			best, choice := bounds[i].skip+cost[i+1][j], -1
			for k := j; k < P; k++ {
				mid := (pauses[k].from + pauses[k].to) / 2
				d := math.Abs(mid - bounds[i].expect)
				if d > far {
					continue
				}
				if c := d + cost[i+1][k+1]; c < best {
					best, choice = c, k
				}
			}
			cost[i][j], pick[i][j] = best, choice
		}
	}
	type cut struct {
		after    int
		from, to float64
	}
	var cuts []cut
	for i, j := 0, 0; i < B; i++ {
		if k := pick[i][j]; k >= 0 {
			cuts = append(cuts, cut{bounds[i].after, pauses[k].from, pauses[k].to})
			j = k + 1
		}
	}

	// spread each phrase's words over its stretch of speech
	out := make([]Word, len(fields))
	lo, from := 0, start
	spread := func(hi int, to float64) {
		sum := 0.0
		for _, w := range fields[lo : hi+1] {
			sum += weight(w)
		}
		t := from
		for k := lo; k <= hi; k++ {
			d := (to - from) * weight(fields[k]) / sum
			out[k] = Word{Text: fields[k], Start: t, End: t + d*0.9}
			t += d
		}
	}
	for _, c := range cuts {
		spread(c.after, c.from)
		lo, from = c.after+1, c.to
	}
	spread(len(fields)-1, end)
	return out
}

// quietFrames marks each 10 ms frame as quiet or not, against the loudness
// of the speech around it.
func quietFrames(pcm Speech) []bool {
	hop := int(alignHop * Rate)
	n := len(pcm) / hop
	rms := make([]float64, n)
	peak := 0.0
	for i := range rms {
		var s float64
		for _, v := range pcm[i*hop : (i+1)*hop] {
			s += float64(v) * float64(v)
		}
		rms[i] = math.Sqrt(s / float64(hop))
		peak = math.Max(peak, rms[i])
	}
	// the loud level is a high percentile, so one click does not set it
	sorted := append([]float64(nil), rms...)
	sort.Float64s(sorted)
	loud := sorted[int(float64(len(sorted)-1)*0.9)]
	if loud == 0 {
		loud = peak
	}
	quiet := make([]bool, n)
	for i, r := range rms {
		quiet[i] = r < loud*quietFraction
	}
	return quiet
}
