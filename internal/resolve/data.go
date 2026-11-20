package resolve

import (
	"fmt"
	"strings"

	c "github.com/nessalabs/nessa-video/internal/composition"
)

// checkData checks the steps that change charts and timelines: set on a
// chart row, sort, and mark with row= or span=. A timeline that a mark
// singles a span out of gets room for its readout line.
func (r *resolver) checkData(s *c.Scene) {
	for _, st := range s.Steps {
		if st.Target == nil {
			continue
		}
		targets := c.Select(s.Root, st.Target)
		switch st.Verb {
		case "set":
			data := c.DataDecls(st.Decls)
			if len(data) == 0 {
				continue
			}
			for _, e := range targets {
				if e.Chart == nil {
					r.diags.Errorf(st.Pos, `use set #id "text" for text`, "set %s=… needs a chart; %s is a %s", data[0].Key, st.Target.Src, e.Kind())
					continue
				}
				for _, d := range data {
					if e.Chart.Row(d.Key) < 0 {
						r.diags.Errorf(d.Pos, "rows: "+rowNames(e.Chart), "chart %s has no row %q", st.Target.Src, d.Key)
						continue
					}
					if _, err := c.ChartValues(d.Value, len(e.Chart.Series)); err != nil {
						r.diags.Errorf(d.Pos, fmt.Sprintf("e.g. %s=240, or one value per series: %s", d.Key, strings.Join(e.Chart.Series, ",")), "%v", err)
					}
				}
			}
		case "sort":
			for _, e := range targets {
				if e.Chart == nil {
					r.diags.Errorf(st.Pos, "sort reorders a chart's bars", "%s is a %s, not a chart", st.Target.Src, e.Kind())
				}
			}
		case "mark":
			if v, ok := st.Lookup("row"); ok {
				for _, e := range targets {
					if e.Chart == nil {
						r.diags.Errorf(st.Pos, "mark row= picks a chart row", "%s is a %s, not a chart", st.Target.Src, e.Kind())
					} else if e.Chart.Row(v) < 0 {
						r.diags.Errorf(st.Pos, "rows: "+rowNames(e.Chart), "chart %s has no row %q", st.Target.Src, v)
					}
				}
			}
			if v, ok := st.Lookup("span"); ok {
				for _, e := range targets {
					if e.Timeline == nil {
						r.diags.Errorf(st.Pos, "mark span= picks a timeline span", "%s is a %s, not a timeline", st.Target.Src, e.Kind())
						continue
					}
					found := false
					for l, ln := range e.Timeline.Lanes {
						for k := range ln.Spans {
							found = found || e.Timeline.Picks(v, l, k)
						}
					}
					if !found {
						r.diags.Errorf(st.Pos, "span= names a span's label or a lane's", "timeline %s has no span %q", st.Target.Src, v)
					}
					e.Timeline.Readout = true
				}
			}
		}
	}
}

func rowNames(cb *c.ChartBlock) string {
	var out []string
	for _, r := range cb.Rows {
		out = append(out, r.Label)
	}
	return strings.Join(out, ", ")
}
