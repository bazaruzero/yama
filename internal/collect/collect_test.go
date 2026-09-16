package collect

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bazaruzero/yama/internal/config"
	"github.com/bazaruzero/yama/internal/store"
)

// fakeQuerier returns scripted results for QueryValue.
type fakeQuerier struct {
	mu    sync.Mutex
	value float64
	err   error
	calls int
}

func (f *fakeQuerier) QueryValue(_ context.Context, _ string) (float64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.value, f.err
}

func (f *fakeQuerier) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
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

	if err := c.CollectOnce(context.Background(), metric("select_1", config.MetricTypeGauge)); err != nil {
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

	if err := c.CollectOnce(context.Background(), metric("m", config.MetricTypeGauge)); err == nil {
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
	s := NewScheduler(c, []config.Metric{m}, 50*time.Millisecond, nil)

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

	s := NewScheduler(c, []config.Metric{fast, slow}, time.Second, nil)
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
	s := NewScheduler(c, []config.Metric{m}, time.Second, nil)

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
	if err := c.CollectOnce(context.Background(), m); err != nil {
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
