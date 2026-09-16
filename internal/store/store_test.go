package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreWriteQueryRoundTrip(t *testing.T) {
	s, err := OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	base := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	want := []Point{
		{Name: "select_1", Type: "gauge", Value: 1, Timestamp: base},
		{Name: "select_1", Type: "gauge", Value: 1, Timestamp: base.Add(10 * time.Second)},
		{Name: "select_1", Type: "gauge", Value: 1, Timestamp: base.Add(20 * time.Second)},
	}
	for _, p := range want {
		if err := s.Write(p); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	got, err := s.Query("select_1", base, base.Add(20*time.Second))
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d points, want 3", len(got))
	}
	for i, p := range got {
		if !p.Timestamp.Equal(want[i].Timestamp) || p.Value != want[i].Value || p.Type != want[i].Type {
			t.Errorf("point %d: got %+v, want %+v", i, p, want[i])
		}
	}
}

func TestStoreQueryRangeFiltering(t *testing.T) {
	s, err := OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	base := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	// Write out of chronological order to verify query ordering.
	times := []time.Duration{30 * time.Second, 0, 20 * time.Second, 10 * time.Second, 40 * time.Second}
	for _, d := range times {
		if err := s.Write(Point{Name: "m", Type: "gauge", Value: 1, Timestamp: base.Add(d)}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.Query("m", base.Add(10*time.Second), base.Add(30*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d points in [10s,30s], want 3", len(got))
	}
	for i, want := range []time.Duration{10, 20, 30} {
		if !got[i].Timestamp.Equal(base.Add(want * time.Second)) {
			t.Errorf("point %d: got %s", i, got[i].Timestamp)
		}
	}
	// Inclusive boundaries: a point exactly at from or to is included.
	got, err = s.Query("m", base, base.Add(40*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("full range: got %d points, want 5", len(got))
	}
}

func TestStoreQueryUnknownMetric(t *testing.T) {
	s, err := OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	got, err := s.Query("nope", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("unknown metric: got %d points, want 0", len(got))
	}
}

func TestStoreQueryOrdering(t *testing.T) {
	s, err := OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	base := time.Now().UTC().Truncate(time.Second)
	// Insert in reverse order; query must return ascending.
	for i := 9; i >= 0; i-- {
		if err := s.Write(Point{Name: "ord", Type: "gauge", Value: float64(i), Timestamp: base.Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Query("ord", base, base.Add(9*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 10 {
		t.Fatalf("got %d, want 10", len(got))
	}
	for i, p := range got {
		if p.Value != float64(i) {
			t.Errorf("index %d: value %v out of order", i, p.Value)
		}
	}
}

func TestOpenBadgerUncreatableDir(t *testing.T) {
	// A regular file where a directory is expected makes open fail.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := OpenBadger(filepath.Join(blocker, "data"))
	if err == nil {
		t.Fatal("expected error opening storage under a regular file")
	}
}

func TestStorePersistenceAcrossReopen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")

	s, err := OpenBadger(dir)
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	if err := s.Write(Point{Name: "select_1", Type: "gauge", Value: 1, Timestamp: ts}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := OpenBadger(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	got, err := s2.Query("select_1", ts.Add(-time.Minute), ts.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].Timestamp.Equal(ts) || got[0].Value != 1 || got[0].Type != "gauge" {
		t.Errorf("after reopen: got %+v", got)
	}
}
