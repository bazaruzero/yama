// Package chart renders metric data points as server-side SVG line charts.
package chart

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bazaruzero/yama/internal/web/agent"
)

// SVG canvas dimensions.
const (
	width  = 800.0
	height = 320.0
	padTop = 14.0
	padBot = 58.0 // room for x axis labels and the legend line below them
	padL   = 48.0 // room for y axis labels
	padR   = 12.0
)

// Options controls the rendering of one chart.
type Options struct {
	// From and To define the visible time window (the x axis domain).
	// Points outside the window are excluded.
	From, To time.Time
}

// Clip returns only the points that fall within the window in opts, in
// their original order. Render draws exactly these points; callers use the
// clipped length to decide between a chart and a no-data state.
func Clip(points []agent.Point, opts Options) []agent.Point {
	in := make([]agent.Point, 0, len(points))
	for _, p := range points {
		if p.Timestamp.Before(opts.From) || p.Timestamp.After(opts.To) {
			continue
		}
		in = append(in, p)
	}
	return in
}

// Render returns an inline SVG line chart of the points for the window in
// opts, with UTC time and value axis labels, a min/max/current legend below
// the chart, and a native SVG tooltip on every point. It returns an empty
// string when there are no points in the window; callers render a no-data
// state.
func Render(points []agent.Point, opts Options) string {
	in := Clip(points, opts)
	if len(in) == 0 {
		return ""
	}

	from, to := opts.From, opts.To
	if to.Equal(from) {
		to = from.Add(time.Second)
	}

	vmin, vmax := in[0].Value, in[0].Value
	for _, p := range in[1:] {
		if p.Value < vmin {
			vmin = p.Value
		}
		if p.Value > vmax {
			vmax = p.Value
		}
	}
	ymin, ymax := vmin, vmax
	if ymin == ymax {
		ymin, ymax = vmin-1, vmax+1
	} else {
		pad := (vmax - vmin) * 0.1
		ymin, ymax = vmin-pad, vmax+pad
	}

	plotX0, plotX1 := padL, width-padR
	plotY0, plotY1 := padTop, height-padBot
	spanX := float64(to.Sub(from))
	spanY := ymax - ymin

	xOf := func(ts time.Time) float64 {
		frac := float64(ts.Sub(from)) / spanX
		return plotX0 + frac*(plotX1-plotX0)
	}
	yOf := func(v float64) float64 {
		frac := (v - ymin) / spanY
		return plotY1 - frac*(plotY1-plotY0)
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="chart" viewBox="0 0 %g %g" xmlns="http://www.w3.org/2000/svg" role="img">`, width, height)

	// Y axis: grid lines and labels at 5 positions.
	for k := 0; k <= 4; k++ {
		y := plotY0 + float64(k)*(plotY1-plotY0)/4
		v := ymax - float64(k)*spanY/4
		fmt.Fprintf(&b, `<line class="grid" x1="%g" y1="%g" x2="%g" y2="%g"/>`, plotX0, y, plotX1, y)
		fmt.Fprintf(&b, `<text class="label label-y" x="%g" y="%g" text-anchor="end">%s</text>`, plotX0-6, y+4, fmtVal(v))
	}

	// X axis: 7 grid ticks and UTC time labels across the window.
	ticks := 6
	for k := 0; k <= ticks; k++ {
		x := plotX0 + float64(k)*(plotX1-plotX0)/float64(ticks)
		ts := from.Add(time.Duration(float64(k) * float64(to.Sub(from)) / float64(ticks)))
		fmt.Fprintf(&b, `<line class="grid" x1="%g" y1="%g" x2="%g" y2="%g"/>`, x, plotY0, x, plotY1)
		anchor := "middle"
		if k == 0 {
			anchor = "start"
		} else if k == ticks {
			anchor = "end"
		}
		fmt.Fprintf(&b, `<text class="label label-x" x="%g" y="%g" text-anchor="%s">%s</text>`, x, plotY1+18, anchor, ts.UTC().Format("15:04:05"))
	}

	verts := make([][2]float64, len(in))
	for i, p := range in {
		verts[i] = [2]float64{xOf(p.Timestamp), yOf(p.Value)}
	}
	fmt.Fprintf(&b, `<path class="line" fill="none" stroke-linejoin="round" stroke-linecap="round" d="%s"/>`, smoothPath(verts))
	for _, p := range in {
		fmt.Fprintf(&b, `<circle class="dot" cx="%g" cy="%g" r="2.5"><title>%s — %s</title></circle>`,
			xOf(p.Timestamp), yOf(p.Value), p.Timestamp.UTC().Format("2006-01-02 15:04:05"), fmtVal(p.Value))
	}

	// Legend sits below the chart, under the x axis labels.
	fmt.Fprintf(&b, `<text class="legend" x="%g" y="%g" text-anchor="middle">min %s · max %s · current %s</text>`,
		width/2, plotY1+40, fmtVal(vmin), fmtVal(vmax), fmtVal(in[len(in)-1].Value))
	b.WriteString(`</svg>`)
	return b.String()
}

// smoothPath returns a smooth SVG path through the vertices: a monotone
// cubic Bézier (Fritsch–Carlson tangents converted to Bézier control
// points). The curve passes exactly through every vertex and never
// overshoots it, giving rounded joins without data distortion. Zero-width
// segments (duplicate x) draw nothing.
func smoothPath(v [][2]float64) string {
	if len(v) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "M %g %g", v[0][0], v[0][1])
	if len(v) == 1 {
		return b.String()
	}
	n := len(v) - 1
	deltas := make([]float64, n)
	for k := 0; k < n; k++ {
		dx := v[k+1][0] - v[k][0]
		if dx != 0 {
			deltas[k] = (v[k+1][1] - v[k][1]) / dx
		}
	}
	tangents := make([]float64, len(v))
	tangents[0] = deltas[0]
	tangents[n] = deltas[n-1]
	for k := 1; k < n; k++ {
		if deltas[k-1]*deltas[k] <= 0 {
			tangents[k] = 0
		} else {
			tangents[k] = (deltas[k-1] * deltas[k]) / (deltas[k-1] + deltas[k])
		}
	}
	for k := 0; k < n; k++ {
		dx := v[k+1][0] - v[k][0]
		if dx == 0 {
			continue
		}
		fmt.Fprintf(&b, " C %g %g %g %g %g %g",
			v[k][0]+dx/3, v[k][1]+tangents[k]*dx/3,
			v[k+1][0]-dx/3, v[k+1][1]-tangents[k+1]*dx/3,
			v[k+1][0], v[k+1][1])
	}
	return b.String()
}

// fmtVal formats a chart value compactly with two decimals.
func fmtVal(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}
