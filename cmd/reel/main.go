// Command reel turns .reel scripts into explainer videos.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nessalabs/nessa-video/internal/app"
	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/encode"
	"github.com/nessalabs/nessa-video/internal/encode/aac"
	"github.com/nessalabs/nessa-video/internal/encode/x264"
	"github.com/nessalabs/nessa-video/internal/render/canvas"
	"github.com/nessalabs/nessa-video/internal/theme"
	"github.com/nessalabs/nessa-video/internal/voice"
	"github.com/nessalabs/nessa-video/internal/voice/elevenlabs"
	"github.com/nessalabs/nessa-video/internal/voice/phonon"
	skill "github.com/nessalabs/nessa-video/skills/reel"
)

// version is set at build time: make build VERSION=v0.1.0.
var version = "dev"

const usage = `reel turns .reel scripts into explainer videos (MP4).

Workflow for agents and people:
  1. Learn the script format:   reel skill          (how to plan and write a script)
                                reel ref            (every element, step and property)
  2. Write a .reel file. Start from the minimal script at the end of this help.
  3. Check it:                  reel check FILE     exit 0 = ok, 1 = problems; each
                                problem has a line number and a suggested fix.
                                Add --json for machine-readable output.
  4. Look at it:                reel still -o DIR FILE   one PNG per scene; read them.
  5. Render it:                 reel render -o OUT.mp4 FILE
  Fix what check reports and re-run until it passes before rendering.

Commands:
  check [--json] [--timeline] [--explain SELECTOR] FILE
                Parse and resolve a script; report problems with line numbers.
                --timeline lists when every step runs; --explain '#id' shows
                where each property of an element came from.
  still [--scene NAME | --at SECONDS] [-o PATH] FILE
                Render how each scene looks once its steps have run, as PNG.
  render [-o OUT.mp4] [--crf N] [--preset NAME] [--mute] FILE
                Render the whole video. --mute skips narration.
  skill         Print the reel skill: how to plan and write a good script.
  skill install [DIR]
                Install the skill for an agent (default ~/.claude/skills/reel).
  ref           Print the built-in kinds, steps and properties.
  themes        List the built-in theme packs.
  theme export NAME [DIR]
                Copy a built-in pack to DIR (default themes/NAME) to edit it.
  version       Print the version.

System design: the hairline theme has breaker, limiter and cache (with
entry rows). Requests travel as labelled cards on their own lane and
answers come back on another: send #client to=#svc "req 1" reply="200";
spend=#rl takes a limiter token; set #cb "Open" tone=bad changes a state
light; play #cb for=4s runs its countdown; move #e first reorders a list.
See examples/systems.reel and reel ref.

Narration: put "voice phonon" under the video line to narrate with the
built-in local voice (Harper; no account or network needed), or
"voice elevenlabs VOICE_ID" with ELEVENLABS_API_KEY set or in a .env file.

Minimal script:
  video "What a cache does"
    voice phonon

  scene hit
    row gap=40
      box #app "app"
      box #cache "cache"
    #app -> #cache
    say "A ==cache hit==" speak="The app asks the cache first."
    focus #cache on="cache first"

More: https://github.com/nessalabs/nessa-video (docs/agents/README.md)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	switch os.Args[1] {
	case "check":
		os.Exit(check(os.Args[2:]))
	case "still":
		os.Exit(still(os.Args[2:]))
	case "render":
		os.Exit(renderCmd(os.Args[2:]))
	case "themes":
		for _, n := range theme.Names() {
			def := ""
			if n == theme.Default {
				def = " (default)"
			}
			fmt.Println(n + def)
		}
	case "theme":
		os.Exit(themeCmd(os.Args[2:]))
	case "ref":
		ref()
	case "skill":
		os.Exit(skillCmd(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Println("reel " + version)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "reel: unknown command %q\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
}

func check(args []string) int {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print JSON")
	explain := fs.String("explain", "", "show where each property of matching elements came from")
	timeline := fs.Bool("timeline", false, "list when every step runs")
	fs.Parse(args)
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: reel check [--json] [--explain SELECTOR] FILE")
		return 2
	}
	file := fs.Arg(0)
	src, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "reel:", err)
		return 2
	}
	rep := app.CheckWith(string(src), filepath.Dir(file), offlineVoice)

	if *explain != "" {
		ex, err := rep.Explain(*explain)
		if err != nil {
			fmt.Fprintln(os.Stderr, "reel:", err)
			return 1
		}
		if *asJSON {
			printJSON(ex)
		} else {
			for _, e := range ex {
				fmt.Printf("%s  (scene %s, defined at %s)\n", e.Element, e.Scene, e.Where)
				for _, p := range e.Props {
					detail := ""
					if p.Detail != "" {
						detail = ", " + p.Detail
					}
					fmt.Printf("  %-8s %-10s %s at %s%s\n", p.Key, p.Value, p.From, p.Where, detail)
				}
			}
		}
		return 0
	}

	if *asJSON {
		printJSON(rep)
	} else {
		if *timeline && rep.OK {
			fmt.Print(rep.FormatTimeline())
		}
		fmt.Print(rep.FormatText(file))
	}
	if !rep.OK {
		return 1
	}
	return 0
}

func ref() {
	sec := func(title string, items []string) {
		fmt.Printf("%s\n  %s\n\n", title, strings.Join(items, " "))
	}
	fmt.Println("Kinds and what they take (every element also takes state= enter= exit= hidden= w= h=)")
	for _, group := range [][]string{c.Primitives, c.Layouts, c.Blocks} {
		for _, k := range group {
			fmt.Printf("  %-9s %s\n", k, c.KindProps[k])
		}
	}
	fmt.Println("  laptop, phone, server, process, db, queue: theme components; run `reel theme export hairline` to see their parts")
	fmt.Println("  breaker \"Closed\" label= tone= limit=3, limiter tokens=6 unit=, cache count= size=3 (holds entry \"key\" value=): system components")
	fmt.Println()
	sec("Steps", c.Verbs)
	sec("States", c.States)
	sec("Enter and exit modes", c.EnterModes)
	sec("Properties", c.Props)
	fmt.Println("Connectors\n  a -> b   a -| b   a ~> b   a ..> b   a <-> b   a -- b")
}

// skillCmd prints the skill, or with `install [DIR]` writes it where an
// agent loads skills from.
func skillCmd(args []string) int {
	if len(args) == 0 {
		main, _ := skill.Files.ReadFile("SKILL.md")
		os.Stdout.Write(main)
		refs, _ := fs.Glob(skill.Files, "references/*.md")
		for _, r := range refs {
			b, _ := skill.Files.ReadFile(r)
			fmt.Printf("\n\n<!-- %s -->\n\n", r)
			os.Stdout.Write(b)
		}
		return 0
	}
	if args[0] != "install" || len(args) > 2 {
		fmt.Fprintln(os.Stderr, "usage: reel skill [install [DIR]]")
		return 2
	}
	dir := ""
	if len(args) == 2 {
		dir = args[1]
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "reel:", err)
			return 1
		}
		dir = filepath.Join(home, ".claude", "skills", "reel")
	}
	err := fs.WalkDir(skill.Files, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := skill.Files.ReadFile(p)
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "reel:", err)
		return 1
	}
	fmt.Println("installed the reel skill in", dir)
	return 0
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func still(args []string) int {
	fs := flag.NewFlagSet("still", flag.ExitOnError)
	scene := fs.String("scene", "", "render only this scene")
	at := fs.Float64("at", -1, "render the frame at this many seconds into the video instead")
	out := fs.String("o", ".", "output directory, or a .png file with --scene")
	fs.Parse(args)
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: reel still [--scene NAME] [-o PATH] FILE")
		return 2
	}
	file := fs.Arg(0)
	src, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "reel:", err)
		return 2
	}
	cv, err := canvas.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "reel:", err)
		return 1
	}
	sa := app.Stills{Raster: cv, Measure: cv, Dir: filepath.Dir(file), Voice: offlineVoice}
	if *at >= 0 {
		img, rep, err := sa.RenderAt(string(src), *at)
		if err != nil {
			if !rep.OK {
				fmt.Fprint(os.Stderr, rep.FormatText(file))
			}
			fmt.Fprintln(os.Stderr, "reel:", err)
			return 1
		}
		path := *out
		if !strings.HasSuffix(strings.ToLower(path), ".png") {
			stem := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
			os.MkdirAll(path, 0o755)
			path = filepath.Join(path, fmt.Sprintf("%s-at-%.2fs.png", stem, *at))
		}
		f, err := os.Create(path)
		if err == nil {
			err = png.Encode(f, img)
			f.Close()
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "reel:", err)
			return 1
		}
		fmt.Println(path)
		return 0
	}
	stills, rep, err := sa.Render(string(src), *scene)
	if err != nil {
		if !rep.OK {
			fmt.Fprint(os.Stderr, rep.FormatText(file))
		}
		fmt.Fprintln(os.Stderr, "reel:", err)
		return 1
	}
	stem := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	dir := *out
	asFile := strings.HasSuffix(strings.ToLower(*out), ".png")
	if asFile {
		// -o a.png writes a.png for one scene, or a-1-intro.png and so on beside it
		dir = filepath.Dir(*out)
		stem = strings.TrimSuffix(filepath.Base(*out), filepath.Ext(*out))
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "reel:", err)
		return 1
	}
	for _, st := range stills {
		path := filepath.Join(dir, fmt.Sprintf("%s-%d-%s.png", stem, st.Index, st.Scene))
		if asFile && len(stills) == 1 {
			path = *out
		}
		f, err := os.Create(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, "reel:", err)
			return 1
		}
		if err := png.Encode(f, st.Image); err != nil {
			f.Close()
			fmt.Fprintln(os.Stderr, "reel:", err)
			return 1
		}
		f.Close()
		fmt.Println(path)
	}
	return 0
}

func renderCmd(args []string) int {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	out := fs.String("o", "", "output file (default: the script name with .mp4)")
	crf := fs.Float64("crf", 18, "quality: lower is better and larger, 14 to 28 is useful")
	preset := fs.String("preset", "veryfast", "x264 speed preset: ultrafast to slow")
	mute := fs.Bool("mute", false, "skip narration even if the script names a voice")
	fs.Parse(args)
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: reel render [-o OUT.mp4] [--crf N] [--preset NAME] FILE")
		return 2
	}
	file := fs.Arg(0)
	src, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "reel:", err)
		return 2
	}
	loadDotEnv(dotEnvPaths(file)...)
	if *out == "" {
		*out = strings.TrimSuffix(file, filepath.Ext(file)) + ".mp4"
	}
	cv, err := canvas.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "reel:", err)
		return 1
	}
	tmp := *out + ".partial"
	f, err := os.Create(tmp)
	if err != nil {
		fmt.Fprintln(os.Stderr, "reel:", err)
		return 1
	}
	vs := app.Videos{
		Raster: cv, Measure: cv, CRF: *crf, Preset: *preset, Dir: filepath.Dir(file),
		NewEncoder:  func(w io.Writer, o encode.Options) (encode.Encoder, error) { return x264.New(w, o) },
		Mute:        *mute,
		EncodeAudio: aac.Encode,
		Log:         func(m string) { fmt.Fprintln(os.Stderr, m) },
		NewVoice: func(spec *c.VoiceSpec) (voice.Voice, error) {
			switch spec.Engine {
			case "elevenlabs":
				el, err := elevenlabs.New(spec.ID, spec.Model, spec.Tune)
				if err != nil {
					return nil, err
				}
				return voice.Cached{Voice: el, Dir: voice.CacheDir()}, nil
			case "phonon":
				return localVoice(spec)
			case "silent":
				return voice.Silent{}, nil
			}
			return nil, fmt.Errorf("unknown voice engine %q", spec.Engine)
		},
	}
	last := -1
	stats, rep, err := vs.Render(string(src), f, func(done, total int) {
		if pct := done * 100 / total; pct/10 != last/10 && isTerminal() {
			last = pct
			fmt.Fprintf(os.Stderr, "\rrendering %3d%%", pct)
		}
	})
	if isTerminal() {
		fmt.Fprint(os.Stderr, "\r")
	}
	f.Close()
	if err != nil {
		os.Remove(tmp)
		if !rep.OK {
			fmt.Fprint(os.Stderr, rep.FormatText(file))
		}
		fmt.Fprintln(os.Stderr, "reel:", err)
		return 1
	}
	if err := os.Rename(tmp, *out); err != nil {
		fmt.Fprintln(os.Stderr, "reel:", err)
		return 1
	}
	spoken := ""
	if stats.Spoken > 0 {
		spoken = fmt.Sprintf(", %d narrated lines", stats.Spoken)
	}
	fmt.Printf("%s: %d frames, %.1fs of video%s in %.2fs (%.1fx real time)\n",
		*out, stats.Frames, stats.Duration.Seconds(), spoken, stats.Took.Seconds(), stats.Duration.Seconds()/stats.Took.Seconds())
	return 0
}

func isTerminal() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func themeCmd(args []string) int {
	if len(args) < 2 || args[0] != "export" || len(args) > 3 {
		fmt.Fprintln(os.Stderr, "usage: reel theme export NAME [DIR]")
		return 2
	}
	name := args[1]
	src, ok := theme.Builtin(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "reel: no built-in theme %q; built-in: %s\n", name, strings.Join(theme.Names(), ", "))
		return 1
	}
	dir := filepath.Join("themes", name)
	if len(args) == 3 {
		dir = args[2]
	}
	path := filepath.Join(dir, "theme.reel")
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(os.Stderr, "reel: %s already exists; remove it or pick another folder\n", path)
		return 1
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "reel:", err)
		return 1
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "reel:", err)
		return 1
	}
	fmt.Printf("%s\nuse it with `theme %s` in a script's video block\n", path, filepath.ToSlash(dir))
	return 0
}

// dotEnvPaths lists .env files in the script's folder and its parents, then
// the current folder and its parents, nearest first.
func dotEnvPaths(file string) []string {
	var out []string
	seen := map[string]bool{}
	for _, start := range []string{filepath.Dir(file), "."} {
		dir, err := filepath.Abs(start)
		if err != nil {
			continue
		}
		for {
			p := filepath.Join(dir, ".env")
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return out
}

// loadDotEnv reads KEY=VALUE lines from the first files that exist and sets
// variables that are not already set, so API keys can live in a local,
// git-ignored .env file instead of the shell.
func loadDotEnv(paths ...string) {
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
			if !ok {
				continue
			}
			k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
			if _, set := os.LookupEnv(k); !set {
				os.Setenv(k, v)
			}
		}
	}
}

// offlineVoice answers narration from the cache only, never the network, so
// check and still can use the lengths a previous render spoke.
func offlineVoice(spec *c.VoiceSpec) voice.Voice {
	switch spec.Engine {
	case "elevenlabs":
		return voice.Cached{Voice: voice.Offline{K: elevenlabs.KeyFor(spec.ID, spec.Model, spec.Tune)}, Dir: voice.CacheDir()}
	case "phonon":
		return voice.Cached{Voice: voice.Offline{K: phonon.KeyFor(spec.Model, spec.ID, spec.Tune)}, Dir: voice.CacheDir()}
	case "silent":
		return voice.Silent{}
	}
	return nil
}

// localVoice loads the Phonon model, fetching the pinned checkpoint and the
// voice into the cache first unless model= names a folder of its own. The
// model loads only when a line is not in the voice cache.
func localVoice(spec *c.VoiceSpec) (voice.Voice, error) {
	if !phonon.Available {
		return nil, phonon.ErrNotBuilt
	}
	key := phonon.KeyFor(spec.Model, spec.ID, spec.Tune)
	return voice.Cached{Voice: &lazyPhonon{spec: spec, key: key}, Dir: voice.CacheDir()}, nil
}

// lazyPhonon loads the model on the first line that needs speaking.
type lazyPhonon struct {
	spec *c.VoiceSpec
	key  string
	once sync.Once
	v    *phonon.Voice
	err  error
}

func (l *lazyPhonon) Key() string { return l.key }

func (l *lazyPhonon) Speak(ctx context.Context, text string) (voice.Spoken, error) {
	l.once.Do(func() {
		dir, name := l.spec.Model, l.spec.ID
		if dir == "" {
			dir = phonon.DefaultDir()
			f := phonon.Fetcher{Log: func(m string) { fmt.Fprintln(os.Stderr, m) }}
			if name, l.err = f.Ensure(ctx, dir, name); l.err != nil {
				return
			}
		}
		if name == "" {
			name = "default"
		}
		l.v, l.err = phonon.New(dir, name, l.spec.Tune, l.key)
	})
	if l.err != nil {
		return voice.Spoken{}, l.err
	}
	return l.v.Speak(ctx, text)
}
