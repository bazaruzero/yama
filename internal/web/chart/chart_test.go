package chart

import (
	"fmt"
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
	if got := Render([]agent.Point{{Timestamp: winFrom.Add(-time.Hour), Value: 1}}, Options{From: winFrom, To: winTo}); got != "" {
		t.Errorf("Render(out-of-window) = %q, want empty string", got)
	}
}

func TestRenderExcludesOutOfWindowPoints(t *testing.T) {
	points := []agent.Point{
		{Timestamp: winFrom.Add(-10 * time.Minute), Value: 99}, // before window
		{Timestamp: at(10, 0), Value: 1},
		{Timestamp: at(20, 0), Value: 2},
		{Timestamp: winTo.Add(time.Minute), Value: 98}, // after window
	}
	got := Render(points, Options{From: winFrom, To: winTo})
	if strings.Count(got, "<circle") != 2 {
		t.Errorf("circles = %d, want 2 (out-of-window points must be excluded)\n%s", strings.Count(got, "<circle"), got)
	}
	if strings.Contains(got, "99.00") || strings.Contains(got, "98.00") {
		t.Errorf("out-of-window values leaked into chart:\n%s", got)
	}
}

func TestRenderSinglePoint(t *testing.T) {
	got := Render([]agent.Point{{Timestamp: at(30, 0), Value: 1}}, Options{From: winFrom, To: winTo})
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
	points := []agent.Point{
		{Timestamp: at(10, 0), Value: 5},
		{Timestamp: at(20, 0), Value: 5},
		{Timestamp: at(30, 0), Value: 5},
	}
	got := Render(points, Options{From: winFrom, To: winTo})
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
	points := []agent.Point{
		{Timestamp: at(0, 0), Value: 0},
		{Timestamp: at(30, 0), Value: 5},
		{Timestamp: at(59, 0), Value: 1},
	}
	got := Render(points, Options{From: winFrom, To: winTo})
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
	points := []agent.Point{
		{Timestamp: at(0, 0), Value: 0},
		{Timestamp: at(30, 0), Value: 5},
		{Timestamp: at(59, 0), Value: 1},
	}
	got := Render(points, Options{From: winFrom, To: winTo})

	if !strings.Contains(got, "min 0.00 · max 5.00 · current 1.00") {
		t.Errorf("missing min/max/current legend:\n%s", got)
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
