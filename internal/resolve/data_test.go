package resolve

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestChartDataSteps(t *testing.T) {
	res := Script(`video "t"
scene one
  chart #c bar horizontal unit=ms focus=b threshold="5 Limit" cross="over at {x}"
    k, v
    a, 3
    b, 4
  set #c a=7
  sort #c asc
  mark #c row=b
`)
	if len(res.Diags) > 0 {
		t.Fatalf("diags: %v", res.Diags)
	}
	cb := res.Video.Scenes[0].Root.Children[0].Chart
	if !cb.Horizontal || cb.Unit != "ms" || cb.Focus != "b" || !cb.HasThreshold || cb.Threshold != 5 || cb.ThresholdLabel != "Limit" || cb.Cross != "over at {x}" {
		t.Fatalf("chart = %+v", cb)
	}
}

func TestChartDataErrors(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"  set #c zz=4", `no row "zz"`},
		{"  set #c a=x", `bad value`},
		{"  set #c a=1,2", `give one, or 1`},
		{"  sort #b", `not a chart`},
		{"  sort #c sideways", `unknown sort order`},
		{"  mark #c row=zz", `no row "zz"`},
		{"  mark #c span=a", `not a timeline`},
		{"  set #b a=4", `needs a chart`},
	} {
		res := Script("video \"t\"\nscene one\n  box #b \"b\"\n  chart #c bar\n    k, v\n    a, 3\n" + tc.src + "\n")
		if !res.Diags.HasErrors() || !strings.Contains(fmt.Sprint(res.Diags), tc.want) {
			t.Errorf("%s: want an error with %q, got %v", tc.src, tc.want, res.Diags)
		}
	}
}

func TestDataMotionExampleResolves(t *testing.T) {
	src, err := os.ReadFile("../../examples/data-motion.reel")
	if err != nil {
		t.Fatal(err)
	}
	if res := Script(string(src)); len(res.Diags) > 0 {
		t.Fatalf("diags: %v", res.Diags)
	}
}
