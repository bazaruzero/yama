package collect

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bazaruzero/yama/internal/config"
	"github.com/bazaruzero/yama/internal/store"
)

// fakeQuerier returns scripted results for QueryValue. When sleep is set it
// holds each call for that duration, tracking concurrent executions.
type fakeQuerier struct {
	mu     sync.Mutex
	value  float64
	err    error
	calls  int
	sleep  time.Duration
	active int64
	maxAct int64
}

func (f *fakeQuerier) QueryValue(_ context.Context, _ string) (float64, error) {
	cur := atomic.AddInt64(&f.active, 1)
	for {
		max := atomic.LoadInt64(&f.maxAct)
		if cur <= max || atomic.CompareAndSwapInt64(&f.maxAct, max, cur) {
			break
		}
	}
	if f.sleep > 0 {
		time.Sleep(f.sleep)
	}
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	atomic.AddInt64(&f.active, -1)
	return f.value, f.err
}

func (f *fakeQuerier) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeQuerier) maxConcurrent() int {
	return int(atomic.LoadInt64(&f.maxAct))
}

func metric(name string, typ config.MetricType) config.Metric {
	return config.Metric{Name: name, Query: "SELECT 1", Type: typ}
}

func TestCollectOnceSuccess(t *testing.T) {
	st, err := store.OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	q := &fakeQuerier{value: 1}
	c := NewCollector(q, st, 3*time.Second)

	if err := c.CollectOnce(context.Background(), metric("select_1", config.MetricTypeGauge), time.Now().UTC()); err != nil {
		t.Fatalf("collect: %v", err)
	}
	points, err := st.Query("select_1", time.Now().Add(-time.Minute), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 {
		t.Fatalf("got %d points, want 1", len(points))
	}
	if points[0].Value != 1 || points[0].Type != "gauge" || points[0].Name != "select_1" {
		t.Errorf("point: %+v", points[0])
	}
	if points[0].Timestamp.Location() != time.UTC {
		t.Errorf("timestamp not UTC: %s", points[0].Timestamp.Location())
	}
}

func TestCollectOnceQueryError(t *testing.T) {
	st, err := store.OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	q := &fakeQuerier{err: errors.New("syntax error in SQL")}
	c := NewCollector(q, st, 3*time.Second)

	if err := c.CollectOnce(context.Background(), metric("m", config.MetricTypeGauge), time.Now().UTC()); err == nil {
		t.Fatal("expected query error")
	}
	// Failed run produces no data point.
	points, err := st.Query("m", time.Now().Add(-time.Minute), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 0 {
		t.Errorf("failed run stored %d points, want 0", len(points))
	}
}

// TestSchedulerDisabledMetricNeverRuns verifies disabled metrics are skipped.
func TestSchedulerDisabledMetricNeverRuns(t *testing.T) {
	st, err := store.OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	disabled := false
	q := &fakeQuerier{value: 1}
	c := NewCollector(q, st, time.Second)
	m := config.Metric{Name: "off", Query: "SELECT 1", Type: config.MetricTypeGauge, Enabled: &disabled}
	s := NewScheduler(c, []config.Metric{m}, 50*time.Millisecond, 1, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	s.Run(ctx)

	if q.callCount() != 0 {
		t.Errorf("disabled metric ran %d times", q.callCount())
	}
}

// TestSchedulerPerMetricIntervals verifies each metric runs on its own
// schedule: with a global 1s interval, a 60ms metric must fire several times
// while a disabled-by-omission 1s metric fires only its initial run.
func TestSchedulerPerMetricIntervals(t *testing.T) {
	st, err := store.OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	q := &fakeQuerier{value: 1}
	c := NewCollector(q, st, time.Second)

	fast := config.Metric{
		Name:     "fast",
		Query:    "SELECT 1",
		Type:     config.MetricTypeGauge,
		Interval: config.Duration{Duration: 60 * time.Millisecond},
	}
	slow := config.Metric{Name: "slow", Query: "SELECT 2", Type: config.MetricTypeGauge} // inherits 1s global

	s := NewScheduler(c, []config.Metric{fast, slow}, time.Second, 1, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Millisecond)
	defer cancel()
	s.Run(ctx)

	fastPts, err := st.Query("fast", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	slowPts, err := st.Query("slow", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// fast: initial run + ~5 ticks in 350ms. slow: initial run only (1s interval).
	if len(fastPts) < 3 {
		t.Errorf("fast metric collected %d times, want >= 3", len(fastPts))
	}
	if len(slowPts) != 1 {
		t.Errorf("slow metric collected %d times, want 1 (initial run)", len(slowPts))
	}
}

// TestSchedulerQueryErrorDoesNotStopCollection verifies a failing metric does
// not block its own later runs or other metrics.
func TestSchedulerQueryErrorDoesNotStopCollection(t *testing.T) {
	st, err := store.OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	q := &fakeQuerier{err: errors.New("boom")}
	c := NewCollector(q, st, time.Second)
	m := config.Metric{
		Name:     "flaky",
		Query:    "SELECT bad",
		Type:     config.MetricTypeGauge,
		Interval: config.Duration{Duration: 40 * time.Millisecond},
	}
	s := NewScheduler(c, []config.Metric{m}, time.Second, 1, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	s.Run(ctx)

	if n := q.callCount(); n < 3 {
		t.Errorf("failing metric ran %d times, want >= 3 (collection must continue)", n)
	}
}

// TestCollectorStoreIntegration is the 4.4 integration check: points written
// by the collector are retrievable from the store afterwards.
func TestCollectorStoreIntegration(t *testing.T) {
	dir := t.TempDir()
	st, err := store.OpenBadger(dir)
	if err != nil {
		t.Fatal(err)
	}

	q := &fakeQuerier{value: 42}
	c := NewCollector(q, st, time.Second)
	m := metric("integration_metric", config.MetricTypeCounter)

	var ok int32
	if err := c.CollectOnce(context.Background(), m, time.Now().UTC()); err != nil {
		t.Fatalf("collect: %v", err)
	}
	atomic.AddInt32(&ok, 1)

	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st2, err := store.OpenBadger(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st2.Close()
	points, err := st2.Query("integration_metric", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 1 || points[0].Value != 42 || points[0].Type != "counter" {
		t.Errorf("after reopen: %+v", points)
	}
}

// TestSchedulerBoundedConcurrency (task 3.2): with 10 metrics ticking every
// 50ms, 50ms queries, and maxConns=2, peak concurrent executions must never
// exceed 2 while every metric still produces points.
func TestSchedulerBoundedConcurrency(t *testing.T) {
	st, err := store.OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	q := &fakeQuerier{value: 1, sleep: 50 * time.Millisecond}
	c := NewCollector(q, st, 5*time.Second)

	var metrics []config.Metric
	for i := 0; i < 10; i++ {
		metrics = append(metrics, config.Metric{
			Name:     fmt.Sprintf("m%02d", i),
			Query:    "SELECT 1",
			Type:     config.MetricTypeGauge,
			Interval: config.Duration{Duration: 50 * time.Millisecond},
		})
	}

	s := NewScheduler(c, metrics, time.Second, 2, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	s.Run(ctx)

	if peak := q.maxConcurrent(); peak > 2 {
		t.Errorf("peak concurrent executions = %d, want <= 2", peak)
	}
	if peak := q.maxConcurrent(); peak < 2 {
		t.Logf("note: observed peak %d (< cap 2); serial execution also valid", peak)
	}
	for _, m := range metrics {
		pts, err := st.Query(m.Name, time.Now().Add(-time.Hour), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if len(pts) == 0 {
			t.Errorf("metric %s produced no points", m.Name)
		}
	}
}

// TestSchedulerDedupUnderContention (task 3.3): a single metric with 300ms
// queries ticking every 30ms on one worker must not pile up duplicate jobs —
// executions are bounded by elapsed/sleep, not by tick count.
func TestSchedulerDedupUnderContention(t *testing.T) {
	st, err := store.OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	q := &fakeQuerier{value: 1, sleep: 300 * time.Millisecond}
	c := NewCollector(q, st, 5*time.Second)
	m := config.Metric{
		Name:     "slowpoke",
		Query:    "SELECT 1",
		Type:     config.MetricTypeGauge,
		Interval: config.Duration{Duration: 30 * time.Millisecond},
	}

	runFor := 1000 * time.Millisecond
	s := NewScheduler(c, []config.Metric{m}, time.Second, 1, nil)
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), runFor)
	defer cancel()
	s.Run(ctx)
	elapsed := time.Since(start)

	allowed := int(elapsed/(300*time.Millisecond)) + 2 // ceil + slack for the in-flight run
	if n := q.callCount(); n > allowed {
		t.Errorf("executions = %d, want <= %d (dedup failed: ticks piled up)", n, allowed)
	}
	if n := q.callCount(); n < 2 {
		t.Errorf("executions = %d, want >= 2 in %s", n, elapsed)
	}
}

// TestCollectOnceUsesProvidedTimestamp (task 6.2): the stored point carries
// exactly the timestamp passed in, not the completion time.
func TestCollectOnceUsesProvidedTimestamp(t *testing.T) {
	st, err := store.OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	q := &fakeQuerier{value: 7, sleep: 20 * time.Millisecond}
	c := NewCollector(q, st, time.Second)
	ts := time.Date(2026, 9, 24, 12, 0, 0, 123456789, time.UTC)
	if err := c.CollectOnce(context.Background(), metric("stamped", config.MetricTypeGauge), ts); err != nil {
		t.Fatalf("collect: %v", err)
	}
	pts, err := st.Query("stamped", ts.Add(-time.Minute), ts.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 1 || !pts[0].Timestamp.Equal(ts) {
		t.Errorf("stored timestamp %v, want exactly %v", pts[0].Timestamp, ts)
	}
}

// TestCycleSharedTimestamp (task 6.1): with maxConns=1 and sequential 20ms
// queries, the initial cycle's three metrics all share one identical
// timestamp (the cycle wake time), not staggered completion times.
func TestCycleSharedTimestamp(t *testing.T) {
	st, err := store.OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	q := &fakeQuerier{value: 1, sleep: 20 * time.Millisecond}
	c := NewCollector(q, st, 5*time.Second)
	metrics := []config.Metric{
		{Name: "a", Query: "SELECT 1", Type: config.MetricTypeGauge},
		{Name: "b", Query: "SELECT 1", Type: config.MetricTypeGauge},
		{Name: "c", Query: "SELECT 1", Type: config.MetricTypeGauge},
	}
	s := NewScheduler(c, metrics, 100*time.Millisecond, 1, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	s.Run(ctx)

	first := func(name string) time.Time {
		pts, err := st.Query(name, time.Now().Add(-time.Hour), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if len(pts) == 0 {
			t.Fatalf("metric %s produced no points", name)
		}
		return pts[0].Timestamp
	}
	ta, tb, tc := first("a"), first("b"), first("c")
	if !ta.Equal(tb) || !ta.Equal(tc) {
		t.Errorf("cycle timestamps differ: a=%v b=%v c=%v (must be identical cycle wake time)", ta, tb, tc)
	}
	if n := q.callCount(); n < 3 {
		t.Fatalf("only %d executions, want >= 3", n)
	}
}

// TestShortIntervalCollectedMoreOften (task 6.3): a 100ms metric inside a
// 500ms global cadence is collected ~5x as often — neither skipped nor
// delayed to the global cadence.
func TestShortIntervalCollectedMoreOften(t *testing.T) {
	st, err := store.OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	q := &fakeQuerier{value: 1}
	c := NewCollector(q, st, time.Second)
	short := config.Metric{
		Name: "short", Query: "SELECT 1", Type: config.MetricTypeGauge,
		Interval: config.Duration{Duration: 100 * time.Millisecond},
	}
	long := config.Metric{Name: "global", Query: "SELECT 1", Type: config.MetricTypeGauge}

	s := NewScheduler(c, []config.Metric{short, long}, 500*time.Millisecond, 1, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 620*time.Millisecond)
	defer cancel()
	s.Run(ctx)

	count := func(name string) int {
		pts, err := st.Query(name, time.Now().Add(-time.Hour), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return len(pts)
	}
	shortN, globalN := count("short"), count("global")
	if shortN < 5 {
		t.Errorf("short-interval metric collected %d times in 620ms, want >= 5 (delayed to global cadence?)", shortN)
	}
	if globalN > 2 {
		t.Errorf("global metric collected %d times in 620ms, want <= 2", globalN)
	}
}

// TestLongIntervalNoRedundantCollection (task 6.4): a 200ms metric inside a
// 50ms global cadence is collected only when due, not on every wake.
func TestLongIntervalNoRedundantCollection(t *testing.T) {
	st, err := store.OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	q := &fakeQuerier{value: 1}
	c := NewCollector(q, st, time.Second)
	slow := config.Metric{
		Name: "slow", Query: "SELECT 1", Type: config.MetricTypeGauge,
		Interval: config.Duration{Duration: 200 * time.Millisecond},
	}
	fast := config.Metric{Name: "fast", Query: "SELECT 1", Type: config.MetricTypeGauge}

	s := NewScheduler(c, []config.Metric{slow, fast}, 50*time.Millisecond, 1, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	s.Run(ctx)

	count := func(name string) int {
		pts, err := st.Query(name, time.Now().Add(-time.Hour), time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return len(pts)
	}
	slowN := count("slow")
	if slowN < 3 || slowN > 5 {
		t.Errorf("200ms metric collected %d times in 700ms, want 3-5 (due at 0,200,400,600; tolerant of dropped ticker wakes)", slowN)
	}
	if n := count("fast"); n < 6 {
		t.Errorf("50ms metric collected %d times in 700ms, want >= 6", n)
	}
}

// TestExactCadenceNoGridSlip (task 7.2): with next-due timing, a 200ms
// metric fires every ~200ms. Under the old GCD-grid design a boundary wake
// could land microseconds short of the full interval and slip a whole grid
// step, producing a ~2x gap; here every gap must stay within 1.3x.
func TestExactCadenceNoGridSlip(t *testing.T) {
	st, err := store.OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	q := &fakeQuerier{value: 1}
	c := NewCollector(q, st, time.Second)
	m := config.Metric{
		Name: "steady", Query: "SELECT 1", Type: config.MetricTypeGauge,
		Interval: config.Duration{Duration: 200 * time.Millisecond},
	}
	s := NewScheduler(c, []config.Metric{m}, 500*time.Millisecond, 1, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 1050*time.Millisecond)
	defer cancel()
	s.Run(ctx)

	pts, err := st.Query("steady", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) < 5 || len(pts) > 6 {
		t.Fatalf("collected %d times in 1050ms, want 5-6", len(pts))
	}
	worst := time.Duration(0)
	for i := 1; i < len(pts); i++ {
		gap := pts[i].Timestamp.Sub(pts[i-1].Timestamp)
		if gap > worst {
			worst = gap
		}
	}
	if worst > 260*time.Millisecond {
		t.Errorf("largest gap between collections = %s, want <= 260ms (grid-step slip detected)", worst)
	}
	if worst < 190*time.Millisecond {
		t.Errorf("implausibly small gap %s — cadence broken", worst)
	}
}
