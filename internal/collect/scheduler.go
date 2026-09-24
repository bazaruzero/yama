package collect

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/bazaruzero/yama/internal/config"
)

// collectJob is one due metric plus the timestamp of the cycle that
// enqueued it. Every job from the same cycle carries the identical ts.
type collectJob struct {
	metric config.Metric
	ts     time.Time
}

// dispatch is a FIFO job queue with per-metric dedup: a metric whose
// collection is already queued or running is not enqueued again. Because of
// this rule the number of in-flight jobs never exceeds the number of enabled
// metrics, so a buffered channel of that capacity never blocks the producer.
type dispatch struct {
	ch      chan collectJob
	mu      sync.Mutex
	pending map[string]bool
}

func newDispatch(capacity int) *dispatch {
	if capacity < 1 {
		capacity = 1
	}
	return &dispatch{
		ch:      make(chan collectJob, capacity),
		pending: make(map[string]bool),
	}
}

// tryEnqueue adds the job unless one for the same metric is in flight.
// It reports whether the job was enqueued.
func (d *dispatch) tryEnqueue(job collectJob) bool {
	d.mu.Lock()
	if d.pending[job.metric.Name] {
		d.mu.Unlock()
		return false
	}
	d.pending[job.metric.Name] = true
	d.mu.Unlock()
	d.ch <- job
	return true
}

// finish marks the metric's job as complete, allowing future enqueues.
func (d *dispatch) finish(name string) {
	d.mu.Lock()
	delete(d.pending, name)
	d.mu.Unlock()
}

// Scheduler runs collection cycles: at each wake it collects every enabled
// metric that is due (elapsed time since its last collection >= its
// effective interval), sharing one timestamp — the wake time — across the
// whole cycle. Jobs execute on a bounded worker pool of at most maxConns
// workers, so concurrent database connections never exceed maxConns.
type Scheduler struct {
	collector *Collector
	metrics   []config.Metric
	global    time.Duration
	maxConns  int
	log       *slog.Logger
}

// NewScheduler creates a Scheduler for metrics, using global as the default
// collection interval and maxConns (>= 1) as the concurrent connection
// ceiling.
func NewScheduler(c *Collector, metrics []config.Metric, global time.Duration, maxConns int, log *slog.Logger) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	if maxConns < 1 {
		maxConns = 1
	}
	return &Scheduler{collector: c, metrics: metrics, global: global, maxConns: maxConns, log: log}
}

// Run starts the collection cycles and blocks until ctx is cancelled and all
// workers have finished. The first cycle fires immediately at startup.
// Pending-but-unstarted jobs at cancellation are discarded; those metrics
// resume on their next due cycle.
func (s *Scheduler) Run(ctx context.Context) {
	enabled := make([]config.Metric, 0, len(s.metrics))
	for _, m := range s.metrics {
		if !m.IsEnabled() {
			s.log.Info("metric disabled, skipping", "metric", m.Name)
			continue
		}
		enabled = append(enabled, m)
	}
	if len(enabled) == 0 {
		return
	}

	d := newDispatch(len(enabled))

	workers := s.maxConns
	if workers > len(enabled) {
		workers = len(enabled)
	}

	var workerWG sync.WaitGroup
	for i := 0; i < workers; i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-d.ch:
					if !ok {
						return
					}
					s.collectAndLog(ctx, job)
					d.finish(job.metric.Name)
				}
			}
		}()
	}

	last := make(map[string]time.Time, len(enabled))
	runCycle := func() {
		x := time.Now().UTC()
		enqueued, skipped := 0, 0
		for _, m := range enabled {
			lastT, seen := last[m.Name]
			if seen && x.Sub(lastT) < m.EffectiveInterval(s.global) {
				continue // not due this cycle
			}
			// Phase-align at enqueue time even if dedup drops the job:
			// prevents unbounded catch-up bursts under sustained backlog.
			last[m.Name] = x
			if d.tryEnqueue(collectJob{metric: m, ts: x}) {
				enqueued++
			} else {
				skipped++ // previous collection still in flight
			}
		}
		if enqueued > 0 {
			s.log.Info("collection cycle", "wake", x, "enqueued", enqueued, "skipped_in_flight", skipped)
		} else {
			s.log.Debug("collection cycle idle", "wake", x, "skipped_in_flight", skipped)
		}
	}

	// nextDue returns the earliest time any enabled metric is next due.
	// Metrics never collected are due immediately.
	nextDue := func() time.Time {
		now := time.Now()
		var next time.Time
		for _, m := range enabled {
			due := now
			if lastT, seen := last[m.Name]; seen {
				due = lastT.Add(m.EffectiveInterval(s.global))
			}
			if next.IsZero() || due.Before(next) {
				next = due
			}
		}
		return next
	}

	runCycle() // immediate first cycle
	for {
		// Sleep until the earliest next due instant. Waking exactly at a
		// metric's due time (instead of on a fixed grid) keeps every metric
		// at its own cadence plus only timer jitter — never a full wake
		// step late. A non-positive delay (cycle overran an interval) fires
		// the timer immediately.
		timer := time.NewTimer(time.Until(nextDue()))
		select {
		case <-ctx.Done():
			timer.Stop()
			close(d.ch)
			workerWG.Wait()
			return
		case <-timer.C:
			runCycle()
		}
	}
}

func (s *Scheduler) collectAndLog(ctx context.Context, job collectJob) {
	start := time.Now()
	if err := s.collector.CollectOnce(ctx, job.metric, job.ts); err != nil {
		s.log.Error("collection failed", "metric", job.metric.Name, "error", err)
		return
	}
	s.log.Info("collected metric", "metric", job.metric.Name, "duration_ms", time.Since(start).Milliseconds())
}
