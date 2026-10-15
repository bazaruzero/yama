// Package collect schedules and executes metric queries and stores results.
package collect

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bazaruzero/yama/internal/config"
	"github.com/bazaruzero/yama/internal/store"
)

// Querier executes metric queries. QueryValue serves counter/gauge metrics
// (one row, one numeric column); QueryRow serves multi metrics (one row,
// one or more numeric columns keyed by column name).
type Querier interface {
	QueryValue(ctx context.Context, query string) (float64, error)
	QueryRow(ctx context.Context, query string) (map[string]float64, error)
}

// Collector executes one metric query per run and persists the result.
// A run only counts as successful once its point is stored.
type Collector struct {
	q             Querier
	st            store.Store
	globalTimeout time.Duration
}

// NewCollector creates a Collector with globalTimeout as the fallback query
// timeout for metrics that do not set their own query_timeout.
func NewCollector(q Querier, st store.Store, globalTimeout time.Duration) *Collector {
	return &Collector{q: q, st: st, globalTimeout: globalTimeout}
}

// CollectOnce executes the metric's query once and stores the data point(s)
// stamped with ts (the cycle start time shared by all metrics in a cycle).
// The run is bounded by the metric's effective query timeout. A multi
// metric's single row is exploded into one point per declared column, named
// <metric>.<column>.
func (c *Collector) CollectOnce(ctx context.Context, m config.Metric, ts time.Time) error {
	runCtx, cancel := context.WithTimeout(ctx, m.EffectiveQueryTimeout(c.globalTimeout))
	defer cancel()
	if m.Type == config.MetricTypeMulti {
		return c.collectMulti(runCtx, m, ts)
	}
	v, err := c.q.QueryValue(runCtx, m.Query)
	if err != nil {
		return fmt.Errorf("metric %q: %w", m.Name, err)
	}
	p := store.Point{
		Name:      m.Name,
		Type:      string(m.Type),
		Value:     v,
		Timestamp: ts.UTC(),
	}
	if err := c.st.Write(p); err != nil {
		return fmt.Errorf("metric %q: store: %w", m.Name, err)
	}
	return nil
}

// collectMulti executes a multi metric's query and stores one point per
// declared column, sharing ts across all of them. The returned column set
// must exactly match the declared one; any mismatch fails the run without
// writing any points.
func (c *Collector) collectMulti(ctx context.Context, m config.Metric, ts time.Time) error {
	row, err := c.q.QueryRow(ctx, m.Query)
	if err != nil {
		return fmt.Errorf("metric %q: %w", m.Name, err)
	}
	if err := matchColumns(m.Columns, row); err != nil {
		return fmt.Errorf("metric %q: %w", m.Name, err)
	}
	// Sorted order keeps writes deterministic.
	cols := make([]string, 0, len(m.Columns))
	for col := range m.Columns {
		cols = append(cols, col)
	}
	sort.Strings(cols)
	for _, col := range cols {
		p := store.Point{
			Name:      m.Name + "." + col,
			Type:      string(m.Columns[col]),
			Value:     row[col],
			Timestamp: ts.UTC(),
		}
		if err := c.st.Write(p); err != nil {
			return fmt.Errorf("metric %q: store: %w", m.Name, err)
		}
	}
	return nil
}

// matchColumns verifies the query's returned column set exactly matches the
// declared columns, in both directions, naming both sorted sets on mismatch.
func matchColumns(declared map[string]config.MetricType, returned map[string]float64) error {
	missing := make([]string, 0)
	for col := range declared {
		if _, ok := returned[col]; !ok {
			missing = append(missing, col)
		}
	}
	extra := make([]string, 0)
	for col := range returned {
		if _, ok := declared[col]; !ok {
			extra = append(extra, col)
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return nil
	}
	sort.Strings(missing)
	sort.Strings(extra)
	var b strings.Builder
	b.WriteString("column mismatch: ")
	if len(missing) > 0 {
		fmt.Fprintf(&b, "declared but not returned %v", missing)
	}
	if len(missing) > 0 && len(extra) > 0 {
		b.WriteString("; ")
	}
	if len(extra) > 0 {
		fmt.Fprintf(&b, "returned but not declared %v", extra)
	}
	return fmt.Errorf("%s", b.String())
}
