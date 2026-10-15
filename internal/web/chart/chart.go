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

// palette is the fixed series color cycle for multi-series charts. Index 0
// matches the stylesheet accent so a single-series chart keeps its look.
var palette = []string{"#d97d45", "#3f72af", "#5aa469", "#8e6bbf", "#c44e52", "#4c9a9f"}

// seriesColorAt returns the palette color for series index i, cycling.
func seriesColorAt(i int) string {
	return palette[i%len(palette)]
}

// Options controls the rendering of one chart.
type Options struct {
	// From and To define the visible time window (the x axis domain).
	// Points outside the window are excluded.
	From, To time.Time
}

// Series is one named metric series rendered as a single line.
type Series struct {
	Name   string
	Points []agent.Point
}

// Clip returns only the points that fall within the window in opts, in
// their original order, for each series. Render draws exactly these points;
// callers use the clipped lengths to decide between a chart and a no-data
// state.
func Clip(series []Series, opts Options) []Series {
	out := make([]Series, len(series))
	for i, s := range series {
		in := make([]agent.Point, 0, len(s.Points))
		for _, p := range s.Points {
			if p.Timestamp.Before(opts.From) || p.Timestamp.After(opts.To) {
				continue
			}
			in = append(in, p)
		}
		out[i] = Series{Name: s.Name, Points: in}
	}
	return out
}

// Render returns an inline SVG line chart with one line per series for the
// window in opts, with UTC time and value axis labels and a legend below
// the chart. All series share one absolute y-axis (no normalization).
// A single series keeps the stylesheet accent color and shows a
// min/max/current legend; multiple series get distinct palette colors and a
// per-series legend (name and current value). It returns an empty string
// when no series has points in the window; callers render a no-data state.
func Render(series []Series, opts Options) string {
	in := Clip(series, opts)

	vmin, vmax := 0.0, 0.0
	first := true
	for _, s := range in {
		for _, p := range s.Points {
			if first {
				vmin, vmax = p.Value, p.Value
				first = false
				continue
			}
			if p.Value < vmin {
				vmin = p.Value
			}
			if p.Value > vmax {
				vmax = p.Value
			}
		}
	}
	if first {
		return ""
	}

	from, to := opts.From, opts.To
	if to.Equal(from) {
		to = from.Add(time.Second)
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

	single := len(in) == 1
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

	for i, s := range in {
		if len(s.Points) == 0 {
			continue
		}
		var colorAttr string
		if !single {
			colorAttr = fmt.Sprintf(` style="stroke:%s"`, seriesColorAt(i))
		}
		verts := make([][2]float64, len(s.Points))
		for j, p := range s.Points {
			verts[j] = [2]float64{xOf(p.Timestamp), yOf(p.Value)}
		}
		fmt.Fprintf(&b, `<path class="line" fill="none" stroke-linejoin="round" stroke-linecap="round"%s d="%s"/>`, colorAttr, smoothPath(verts))
		var dotColorAttr string
		if !single {
			dotColorAttr = fmt.Sprintf(` style="fill:%s"`, seriesColorAt(i))
		}
		for _, p := range s.Points {
			fmt.Fprintf(&b, `<circle class="dot" cx="%g" cy="%g" r="2.5"%s><title>%s — %s</title></circle>`,
				xOf(p.Timestamp), yOf(p.Value), dotColorAttr, p.Timestamp.UTC().Format("2006-01-02 15:04:05"), fmtVal(p.Value))
		}
	}

	// Legend sits below the chart, under the x axis labels.
	var legend string
	if single {
		last := in[0].Points[len(in[0].Points)-1].Value
		legend = fmt.Sprintf("min %s · max %s · current %s", fmtVal(vmin), fmtVal(vmax), fmtVal(last))
	} else {
		parts := make([]string, 0, len(in))
		for _, s := range in {
			if len(s.Points) == 0 {
				parts = append(parts, s.Name+": —")
				continue
			}
			parts = append(parts, s.Name+": "+fmtVal(s.Points[len(s.Points)-1].Value))
		}
		legend = strings.Join(parts, " · ")
	}
	fmt.Fprintf(&b, `<text class="legend" x="%g" y="%g" text-anchor="middle">%s</text>`,
		width/2, plotY1+40, legend)
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
