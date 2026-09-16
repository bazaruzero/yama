package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bazaruzero/yama/internal/store"
)

// fixedNow is the clock for deterministic tests.
var fixedNow = time.Date(2026, 9, 15, 15, 0, 0, 0, time.UTC)

func newTestServer(t *testing.T, st store.Store) *Server {
	t.Helper()
	s := NewServer(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.now = func() time.Time { return fixedNow }
	return s
}

func seed(t *testing.T, st store.Store) {
	t.Helper()
	base := fixedNow.Add(-4 * time.Hour) // 11:00 — outside the 3h default window
	for i := 0; i < 5; i++ {
		p := store.Point{
			Name:      "select_1",
			Type:      "gauge",
			Value:     1,
			Timestamp: base.Add(time.Duration(i) * time.Hour), // 11:00, 12:00, 13:00, 14:00, 15:00
		}
		if err := st.Write(p); err != nil {
			t.Fatal(err)
		}
	}
}

func getData(t *testing.T, s *Server, url string) (int, []dataPoint) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	if rec.Code == http.StatusOK {
		var points []dataPoint
		if err := json.Unmarshal(rec.Body.Bytes(), &points); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return rec.Code, points
	}
	return rec.Code, nil
}

func TestMetricDataExplicitRange(t *testing.T) {
	st, err := store.OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	seed(t, st)
	s := newTestServer(t, st)

	from := fixedNow.Add(-5 * time.Hour).Format(time.RFC3339) // 10:00
	to := fixedNow.Add(-2 * time.Hour).Format(time.RFC3339)   // 13:00
	code, points := getData(t, s, "/api/v1/metrics/select_1/data?from="+from+"&to="+to)
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if len(points) != 3 {
		t.Fatalf("got %d points, want 3 (11:00, 12:00, 13:00)", len(points))
	}
	if points[0].Timestamp >= points[1].Timestamp || points[1].Timestamp >= points[2].Timestamp {
		t.Errorf("points not ascending: %+v", points)
	}
}

func TestMetricDataDefaultWindow(t *testing.T) {
	st, err := store.OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	seed(t, st)
	s := newTestServer(t, st)

	// No params: window is [now-3h, now] = [12:00, 15:00] → points at 12,13,14,15.
	code, points := getData(t, s, "/api/v1/metrics/select_1/data")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if len(points) != 4 {
		t.Fatalf("got %d points, want 4 (points within the last 3h)", len(points))
	}
	first, _ := time.Parse(time.RFC3339Nano, points[0].Timestamp)
	if first.Before(fixedNow.Add(-3 * time.Hour)) {
		t.Errorf("point before default window start: %s", first)
	}
}

func TestMetricDataStartOnlyDefaultsEndToNow(t *testing.T) {
	st, err := store.OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	seed(t, st)
	s := newTestServer(t, st)

	// from=13:30, to omitted → window [13:30, now=15:00] → points at 14:00, 15:00.
	from := fixedNow.Add(-90 * time.Minute).Format(time.RFC3339)
	code, points := getData(t, s, "/api/v1/metrics/select_1/data?from="+from)
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if len(points) != 2 {
		t.Fatalf("got %d points, want 2", len(points))
	}
}

func TestMetricDataEmptyResultIsEmptyArray(t *testing.T) {
	st, err := store.OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := newTestServer(t, st)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/metrics/nothing/data", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if body != "[]\n" {
		t.Errorf("body = %q, want empty JSON array", body)
	}
}

func TestMetricDataInvalidTimeParam(t *testing.T) {
	st, err := store.OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := newTestServer(t, st)

	for _, url := range []string{
		"/api/v1/metrics/m/data?from=not-a-time",
		"/api/v1/metrics/m/data?to=2026-13-99",
	} {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", url, rec.Code)
		}
		var errResp map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
			t.Fatalf("error body not JSON: %v", err)
		}
		if errResp["error"] == "" {
			t.Errorf("%s: missing error message in body", url)
		}
	}
}

func TestMetricDataMethodNotAllowed(t *testing.T) {
	st, err := store.OpenBadgerInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := newTestServer(t, st)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/metrics/m/data", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status %d, want 405", method, rec.Code)
		}
	}
}
