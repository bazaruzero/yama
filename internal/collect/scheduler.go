package collect

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/bazaruzero/yama/internal/config"
)

// Scheduler runs one ticker goroutine per enabled metric, all sharing the
// caller's context (typically from signal.NotifyContext), and blocks until
// the context is cancelled and all goroutines have drained.
type Scheduler struct {
	collector *Collector
	metrics   []config.Metric
	global    time.Duration
	log       *slog.Logger
}

// NewScheduler creates a Scheduler for metrics, using global as the default
// collection interval for metrics without their own interval.
func NewScheduler(c *Collector, metrics []config.Metric, global time.Duration, log *slog.Logger) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{collector: c, metrics: metrics, global: global, log: log}
}

// Run starts collection for every enabled metric and blocks until ctx is
// cancelled and all metric goroutines have finished.
func (s *Scheduler) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for _, m := range s.metrics {
		if !m.IsEnabled() {
			s.log.Info("metric disabled, skipping", "metric", m.Name)
			continue
		}
		wg.Add(1)
		go func(m config.Metric) {
			defer wg.Done()
			interval := m.EffectiveInterval(s.global)
			s.log.Info("scheduling metric", "metric", m.Name, "interval", interval)
			s.collectAndLog(ctx, m)
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					s.collectAndLog(ctx, m)
				}
			}
		}(m)
	}
	wg.Wait()
}

func (s *Scheduler) collectAndLog(ctx context.Context, m config.Metric) {
	start := time.Now()
	if err := s.collector.CollectOnce(ctx, m); err != nil {
		s.log.Error("collection failed", "metric", m.Name, "error", err)
		return
	}
	s.log.Info("collected metric", "metric", m.Name, "duration_ms", time.Since(start).Milliseconds())
}
