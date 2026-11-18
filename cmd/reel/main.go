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
	"github.com/nessalabs/nessa-video/internal/encode/x264"
	"github.com/nessalabs/nessa-video/internal/render/canvas"
	"github.com/nessalabs/nessa-video/internal/theme"
)

const usage = `reel turns .reel scripts into explainer videos.

Usage:
  reel check [--json] [--explain SELECTOR] FILE
  reel still [--scene NAME] [-o PATH] FILE
  reel render [-o OUT.mp4] [--crf N] [--preset NAME] FILE
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
	rep := app.CheckIn(string(src), filepath.Dir(file))

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
	sec("Primitives", c.Primitives)
	sec("Layouts", c.Layouts)
	sec("Blocks", c.Blocks)
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
	stills, rep, err := app.Stills{Raster: cv, Measure: cv, Dir: filepath.Dir(file)}.Render(string(src), *scene)
	if err != nil {
		if !rep.OK {
			fmt.Fprint(os.Stderr, rep.FormatText(file))
		}
		fmt.Fprintln(os.Stderr, "reel:", err)
		return 1
	}
	stem := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	for _, st := range stills {
		path := filepath.Join(*out, fmt.Sprintf("%s-%d-%s.png", stem, st.Index, st.Scene))
		if *scene != "" && strings.HasSuffix(*out, ".png") {
			path = *out
		} else if err := os.MkdirAll(*out, 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "reel:", err)
			return 1
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
		NewEncoder: func(w io.Writer, o encode.Options) (encode.Encoder, error) { return x264.New(w, o) },
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
	fmt.Printf("%s: %d frames, %.1fs of video in %.2fs (%.1fx real time)\n",
		*out, stats.Frames, stats.Duration.Seconds(), stats.Took.Seconds(), stats.Duration.Seconds()/stats.Took.Seconds())
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
