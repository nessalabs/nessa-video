// Command reel turns .reel scripts into explainer videos.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"reel/internal/app"
	c "reel/internal/composition"
)

const usage = `reel turns .reel scripts into explainer videos.

Usage:
  reel check [--json] [--explain SELECTOR] FILE
  reel ref

Commands:
  check   Parse and resolve a script; report problems with line numbers.
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
	rep := app.Check(string(src))

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
