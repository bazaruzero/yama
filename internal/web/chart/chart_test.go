package chart

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bazaruzero/yama/internal/web/agent"
)

var (
	winFrom = time.Date(2026, 10, 2, 11, 0, 0, 0, time.UTC)
	winTo   = time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
)

func at(min, sec int) time.Time {
	return time.Date(2026, 10, 2, 11, min, sec, 0, time.UTC)
}

func TestRenderEmpty(t *testing.T) {
	if got := Render(nil, Options{From: winFrom, To: winTo}); got != "" {
		t.Errorf("Render(nil) = %q, want empty string", got)
	}
	empty := []Series{{Name: "a", Points: nil}}
	if got := Render(empty, Options{From: winFrom, To: winTo}); got != "" {
		t.Errorf("Render(no points) = %q, want empty string", got)
	}
	out := []Series{{Name: "a", Points: []agent.Point{{Timestamp: winFrom.Add(-time.Hour), Value: 1}}}}
	if got := Render(out, Options{From: winFrom, To: winTo}); got != "" {
		t.Errorf("Render(out-of-window) = %q, want empty string", got)
	}
	allEmpty := []Series{
		{Name: "a", Points: []agent.Point{{Timestamp: winFrom.Add(-time.Hour), Value: 1}}},
		{Name: "b", Points: nil},
	}
	if got := Render(allEmpty, Options{From: winFrom, To: winTo}); got != "" {
		t.Errorf("Render(all-series-empty) = %q, want empty string", got)
	}
}

func TestRenderExcludesOutOfWindowPoints(t *testing.T) {
	series := []Series{{Name: "m", Points: []agent.Point{
		{Timestamp: winFrom.Add(-10 * time.Minute), Value: 99}, // before window
		{Timestamp: at(10, 0), Value: 1},
		{Timestamp: at(20, 0), Value: 2},
		{Timestamp: winTo.Add(time.Minute), Value: 98}, // after window
	}}}
	got := Render(series, Options{From: winFrom, To: winTo})
	if strings.Count(got, "<circle") != 2 {
		t.Errorf("circles = %d, want 2 (out-of-window points must be excluded)\n%s", strings.Count(got, "<circle"), got)
	}
	if strings.Contains(got, "99.00") || strings.Contains(got, "98.00") {
		t.Errorf("out-of-window values leaked into chart:\n%s", got)
	}
}

func TestRenderSinglePoint(t *testing.T) {
	series := []Series{{Name: "m", Points: []agent.Point{{Timestamp: at(30, 0), Value: 1}}}}
	got := Render(series, Options{From: winFrom, To: winTo})
	if !strings.Contains(got, "<svg") {
		t.Errorf("missing svg element")
	}
	if strings.Count(got, "<circle") != 1 {
		t.Errorf("circles = %d, want 1", strings.Count(got, "<circle"))
	}
	if !strings.Contains(got, "<title>2026-10-02 11:30:00 — 1.00</title>") {
		t.Errorf("missing tooltip title element:\n%s", got)
	}
}

func TestRenderFlatSeries(t *testing.T) {
	series := []Series{{Name: "m", Points: []agent.Point{
		{Timestamp: at(10, 0), Value: 5},
		{Timestamp: at(20, 0), Value: 5},
		{Timestamp: at(30, 0), Value: 5},
	}}}
	got := Render(series, Options{From: winFrom, To: winTo})
	if !strings.Contains(got, `<path class="line"`) {
		t.Errorf("missing line path")
	}
	if strings.Contains(got, "NaN") {
		t.Errorf("chart contains NaN:\n%s", got)
	}
	if !strings.Contains(got, "min 5.00 · max 5.00 · current 5.00") {
		t.Errorf("missing flat legend values:\n%s", got)
	}
}

func TestRenderLineIsSmoothedWithRoundJoins(t *testing.T) {
	series := []Series{{Name: "m", Points: []agent.Point{
		{Timestamp: at(0, 0), Value: 0},
		{Timestamp: at(30, 0), Value: 5},
		{Timestamp: at(59, 0), Value: 1},
	}}}
	got := Render(series, Options{From: winFrom, To: winTo})
	if !strings.Contains(got, `stroke-linejoin="round"`) || !strings.Contains(got, `stroke-linecap="round"`) {
		t.Errorf("line missing round join/cap attributes:\n%s", got)
	}
	if !strings.Contains(got, `d="M `) || !strings.Contains(got, " C ") {
		t.Errorf("line is not drawn as a smooth curve path:\n%s", got)
	}
	if strings.Contains(got, "polyline") {
		t.Errorf("sharp polyline must not be used:\n%s", got)
	}
}

// TestRenderMultiSeries verifies one path per series with distinct palette
// colors, a per-series legend, and a shared y-domain across all series.
func TestRenderMultiSeries(t *testing.T) {
	series := []Series{
		{Name: "commits", Points: []agent.Point{
			{Timestamp: at(0, 0), Value: 0},
			{Timestamp: at(30, 0), Value: 4},
		}},
		{Name: "reads", Points: []agent.Point{
			{Timestamp: at(0, 0), Value: 1},
			{Timestamp: at(30, 0), Value: 9},
		}},
	}
	got := Render(series, Options{From: winFrom, To: winTo})

	if n := strings.Count(got, `<path class="line"`); n != 2 {
		t.Errorf("line paths = %d, want 2:\n%s", n, got)
	}

	// Two distinct stroke colors via inline styles.
	strokes := regexp.MustCompile(`style="stroke:([^"]+)"`).FindAllStringSubmatch(got, -1)
	if len(strokes) != 2 {
		t.Fatalf("stroke styles = %d, want 2:\n%s", len(strokes), got)
	}
	if strokes[0][1] == strokes[1][1] {
		t.Errorf("series share stroke color %s", strokes[0][1])
	}
	if strokes[0][1] != palette[0] || strokes[1][1] != palette[1] {
		t.Errorf("strokes = [%s, %s], want palette order [%s, %s]", strokes[0][1], strokes[1][1], palette[0], palette[1])
	}

	// Dots carry matching fills.
	fills := regexp.MustCompile(`style="fill:([^"]+)"`).FindAllStringSubmatch(got, -1)
	if len(fills) != 4 {
		t.Errorf("dot fills = %d, want 4 (one per point):\n%s", len(fills), got)
	}

	// Per-series legend: name + current value for each series.
	if !strings.Contains(got, "commits: 4.00 · reads: 9.00") {
		t.Errorf("missing per-series legend:\n%s", got)
	}
	if strings.Contains(got, "min ") && strings.Contains(got, "max ") {
		t.Errorf("multi-series legend must not be min/max/current:\n%s", got)
	}

	// Shared y-domain spans both series: max 9 with 10% pad gives ymax 9.9.
	if !strings.Contains(got, ">9.90<") {
		t.Errorf("y axis does not span both series (want top label 9.90):\n%s", got)
	}
}

// TestRenderMultiSeriesWithEmptySeries verifies a series with no in-window
// points draws no path but still appears in the legend.
func TestRenderMultiSeriesWithEmptySeries(t *testing.T) {
	series := []Series{
		{Name: "alive", Points: []agent.Point{
			{Timestamp: at(10, 0), Value: 2},
		}},
		{Name: "dead", Points: nil},
	}
	got := Render(series, Options{From: winFrom, To: winTo})

	if n := strings.Count(got, `<path class="line"`); n != 1 {
		t.Errorf("line paths = %d, want 1 (empty series draws nothing):\n%s", n, got)
	}
	if !strings.Contains(got, "alive: 2.00") {
		t.Errorf("missing legend for populated series:\n%s", got)
	}
	if !strings.Contains(got, "dead: —") {
		t.Errorf("missing legend entry for empty series:\n%s", got)
	}
}

// TestRenderPaletteCycles verifies the palette repeats after six series.
func TestRenderPaletteCycles(t *testing.T) {
	var series []Series
	for i := 0; i < 7; i++ {
		series = append(series, Series{
			Name:   fmt.Sprintf("s%d", i),
			Points: []agent.Point{{Timestamp: at(10, 0), Value: float64(i)}},
		})
	}
	got := Render(series, Options{From: winFrom, To: winTo})
	strokes := regexp.MustCompile(`style="stroke:([^"]+)"`).FindAllStringSubmatch(got, -1)
	if len(strokes) != 7 {
		t.Fatalf("stroke styles = %d, want 7:\n%s", len(strokes), got)
	}
	if strokes[0][1] != strokes[6][1] {
		t.Errorf("series 0 and 6 should share a color (palette cycles): %s vs %s", strokes[0][1], strokes[6][1])
	}
	uniq := map[string]bool{}
	for _, s := range strokes {
		uniq[s[1]] = true
	}
	if len(uniq) != 6 {
		t.Errorf("distinct colors = %d, want 6", len(uniq))
	}
}

func TestSmoothPath(t *testing.T) {
	tests := []struct {
		name  string
		verts [][2]float64
		want  string
	}{
		{
			name:  "empty",
			verts: nil,
			want:  "",
		},
		{
			name:  "single point draws nothing",
			verts: [][2]float64{{4, 8}},
			want:  "M 4 8",
		},
		{
			name:  "two points are a straight segment",
			verts: [][2]float64{{0, 0}, {3, 3}},
			want:  "M 0 0 C 1 1 2 2 3 3",
		},
		{
			// Peak at (3,3): monotone tangent 0 there, so both control
			// points touching the peak sit at its exact y — rounded
			// without overshooting the data.
			name:  "peak has zero tangent and no overshoot",
			verts: [][2]float64{{0, 0}, {3, 3}, {6, 0}},
			want:  "M 0 0 C 1 1 2 3 3 3 C 4 3 5 1 6 0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := smoothPath(tt.verts); got != tt.want {
				t.Errorf("smoothPath = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSmoothPathSkipsDuplicateX(t *testing.T) {
	verts := [][2]float64{{0, 0}, {0, 0}, {3, 3}}
	got := smoothPath(verts)
	// The zero-width first segment draws nothing; the path stays valid and
	// ends at the final vertex (no NaN, no panic).
	if !strings.HasPrefix(got, "M 0 0 C ") || !strings.HasSuffix(got, "3 3") || strings.Contains(got, "NaN") {
		t.Errorf("smoothPath with duplicate x = %q", got)
	}
}

func TestRenderLegendAxesAndScaling(t *testing.T) {
	series := []Series{{Name: "m", Points: []agent.Point{
		{Timestamp: at(0, 0), Value: 0},
		{Timestamp: at(30, 0), Value: 5},
		{Timestamp: at(59, 0), Value: 1},
	}}}
	got := Render(series, Options{From: winFrom, To: winTo})

	if !strings.Contains(got, "min 0.00 · max 5.00 · current 1.00") {
		t.Errorf("missing min/max/current legend:\n%s", got)
	}

	// Single series must not carry inline color overrides (CSS accent look).
	if strings.Contains(got, `style="stroke:`) || strings.Contains(got, `style="fill:`) {
		t.Errorf("single-series chart must keep the stylesheet accent, no inline colors:\n%s", got)
	}

	// Value padding of 10% gives the y domain [-0.5, 5.5]; the five axis
	// labels are therefore 5.50, 4.00, 2.50, 1.00, -0.50.
	for _, want := range []string{">5.50<", ">4.00<", ">2.50<", ">1.00<", ">-0.50<"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing y label %s:\n%s", want, got)
		}
	}

	// X scaling: the plot area spans padL..width-padR = 48..788. The point
	// at 11:00 sits at the left edge, the point at 11:30 halfway across.
	if !strings.Contains(got, fmt.Sprintf(`<circle class="dot" cx="48" `)) {
		t.Errorf("first point not at left edge:\n%s", got)
	}
	midX := padL + (width-padR-padL)/2
	if !strings.Contains(got, fmt.Sprintf(`cx="%g" `, midX)) {
		t.Errorf("mid point not at cx=%g:\n%s", midX, got)
	}

	// The max point (5) must render above the min point (0) on screen.
	cyFirst := parseFloat(t, extractCy(t, got, `<circle class="dot" cx="48" `))
	cyMax := parseFloat(t, extractCy(t, got, fmt.Sprintf(`<circle class="dot" cx="%g" `, midX)))
	if cyMax >= cyFirst {
		t.Errorf("max point cy=%v should be above min point cy=%v", cyMax, cyFirst)
	}

	// Time labels are UTC wall-clock strings.
	if !strings.Contains(got, ">11:00:00<") || !strings.Contains(got, ">12:00:00<") {
		t.Errorf("missing x time labels:\n%s", got)
	}

	// The legend sits below the chart: after all points in document order,
	// centered, and lower than the x axis labels.
	legendIdx := strings.Index(got, `<text class="legend"`)
	if legendIdx < 0 {
		t.Fatalf("missing legend element:\n%s", got)
	}
	if lastDot := strings.LastIndex(got, "<circle"); legendIdx < lastDot {
		t.Errorf("legend must be placed below the chart (after the last point)")
	}
	if !strings.Contains(got, `text-anchor="middle"`) {
		t.Errorf("legend must be centered under the chart:\n%s", got)
	}
	legendY := extractY(t, got, `<text class="legend"`)
	labelY := extractY(t, got, `<text class="label label-x"`)
	if legendY <= labelY {
		t.Errorf("legend y=%v must be below x label y=%v", legendY, labelY)
	}
}

func extractCy(t *testing.T, svg, prefix string) string {
	t.Helper()
	i := strings.Index(svg, prefix)
	if i < 0 {
		t.Fatalf("prefix %q not found in chart", prefix)
	}
	rest := svg[i+len(prefix):]
	j := strings.Index(rest, `cy="`)
	if j < 0 {
		t.Fatalf("no cy attribute after %q", prefix)
	}
	rest = rest[j+len(`cy="`):]
	k := strings.IndexByte(rest, '"')
	if k < 0 {
		t.Fatalf("no closing quote for cy after %q", prefix)
	}
	return rest[:k]
}

func extractY(t *testing.T, svg, prefix string) float64 {
	t.Helper()
	i := strings.Index(svg, prefix)
	if i < 0 {
		t.Fatalf("prefix %q not found in chart", prefix)
	}
	rest := svg[i:]
	j := strings.Index(rest, ` y="`)
	if j < 0 {
		t.Fatalf("no y attribute in %q", prefix)
	}
	rest = rest[j+len(` y="`):]
	k := strings.IndexByte(rest, '"')
	if k < 0 {
		t.Fatalf("no closing quote for y in %q", prefix)
	}
	return parseFloat(t, rest[:k])
}

func parseFloat(t *testing.T, s string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		t.Fatalf("parse %q as float: %v", s, err)
	}
	return v
}
