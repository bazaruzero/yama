// Package collect schedules and executes metric queries and stores results.
package collect

import (
	"context"
	"fmt"
	"time"

	"github.com/bazaruzero/yama/internal/config"
	"github.com/bazaruzero/yama/internal/store"
)

// Querier executes a SQL query and returns its single numeric value.
type Querier interface {
	QueryValue(ctx context.Context, query string) (float64, error)
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

// CollectOnce executes the metric's query once and stores the data point
// stamped with ts (the cycle start time shared by all metrics in a cycle).
// The run is bounded by the metric's effective query timeout.
func (c *Collector) CollectOnce(ctx context.Context, m config.Metric, ts time.Time) error {
	runCtx, cancel := context.WithTimeout(ctx, m.EffectiveQueryTimeout(c.globalTimeout))
	defer cancel()
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
