// Command reel turns .reel scripts into explainer videos.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nessalabs/nessa-video/internal/app"
	c "github.com/nessalabs/nessa-video/internal/composition"
	"github.com/nessalabs/nessa-video/internal/encode"
	"github.com/nessalabs/nessa-video/internal/encode/aac"
	"github.com/nessalabs/nessa-video/internal/encode/x264"
	"github.com/nessalabs/nessa-video/internal/render/canvas"
	"github.com/nessalabs/nessa-video/internal/theme"
	"github.com/nessalabs/nessa-video/internal/voice"
	"github.com/nessalabs/nessa-video/internal/voice/elevenlabs"
)

const usage = `reel turns .reel scripts into explainer videos.

Usage:
  reel check [--json] [--timeline] [--explain SELECTOR] FILE
  reel still [--scene NAME | --at SECONDS] [-o PATH] FILE
  reel render [-o OUT.mp4] [--crf N] [--preset NAME] [--mute] FILE
  reel themes
  reel theme export NAME [DIR]
  reel ref

Commands:
  check   Parse and resolve a script; report problems with line numbers.
  still   Render how each scene looks once its steps have run, as PNG.
  render  Render the whole script to an H.264 MP4 video.
  themes  List the built-in theme packs.
  theme export  Copy a built-in pack to DIR (default themes/NAME) to edit it.
  ref     Print the built-in kinds, steps and properties.
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
	fmt.Println()
	sec("Steps", c.Verbs)
	sec("States", c.States)
	sec("Enter and exit modes", c.EnterModes)
	sec("Properties", c.Props)
	fmt.Println("Connectors\n  a -> b   a -| b   a ~> b   a ..> b   a <-> b   a -- b")
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
				el, err := elevenlabs.New(spec.ID, spec.Model)
				if err != nil {
					return nil, err
				}
				return voice.Cached{Voice: el, Dir: voice.CacheDir()}, nil
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
		return voice.Cached{Voice: voice.Offline{K: elevenlabs.KeyFor(spec.ID, spec.Model)}, Dir: voice.CacheDir()}
	case "silent":
		return voice.Silent{}
	}
	return nil
}
