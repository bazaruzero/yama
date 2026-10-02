package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func clientFor(t *testing.T, srvURL string, timeout time.Duration) *Client {
	t.Helper()
	u, err := url.Parse(srvURL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return NewClient(u.Hostname(), port, timeout)
}

func TestPointsSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"timestamp":"2026-10-02T12:00:00Z","value":1},{"timestamp":"2026-10-02T12:00:02Z","value":2.5}]`)
	}))
	defer ts.Close()

	points, err := clientFor(t, ts.URL, 2*time.Second).Points(context.Background(), "select_1", time.Now().Add(-time.Hour), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 {
		t.Fatalf("points = %d, want 2", len(points))
	}
	if !points[0].Timestamp.Equal(time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("points[0].timestamp = %s", points[0].Timestamp)
	}
	if points[0].Value != 1 || points[1].Value != 2.5 {
		t.Errorf("values = %v, %v", points[0].Value, points[1].Value)
	}
}

func TestPointsSendsExplicitRangeParams(t *testing.T) {
	var gotFrom, gotTo, gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotFrom = r.URL.Query().Get("from")
		gotTo = r.URL.Query().Get("to")
		fmt.Fprint(w, "[]")
	}))
	defer ts.Close()

	from := time.Date(2026, 10, 2, 11, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	if _, err := clientFor(t, ts.URL, 2*time.Second).Points(context.Background(), "select_1", from, to); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/metrics/select_1/data" {
		t.Errorf("path = %q", gotPath)
	}
	if gotFrom != "2026-10-02T11:00:00Z" {
		t.Errorf("from = %q, want RFC3339 2026-10-02T11:00:00Z", gotFrom)
	}
	if gotTo != "2026-10-02T12:00:00Z" {
		t.Errorf("to = %q, want RFC3339 2026-10-02T12:00:00Z", gotTo)
	}
}

func TestPointsNon2xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer ts.Close()

	_, err := clientFor(t, ts.URL, 2*time.Second).Points(context.Background(), "m", time.Now().Add(-time.Hour), time.Now())
	var aerr *Error
	if !errors.As(err, &aerr) {
		t.Fatalf("err = %v, want *agent.Error", err)
	}
	if aerr.Metric != "m" {
		t.Errorf("metric = %q, want m", aerr.Metric)
	}
}

func TestPointsTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		fmt.Fprint(w, "[]")
	}))
	defer ts.Close()

	_, err := clientFor(t, ts.URL, 20*time.Millisecond).Points(context.Background(), "m", time.Now().Add(-time.Hour), time.Now())
	var aerr *Error
	if !errors.As(err, &aerr) {
		t.Fatalf("err = %v, want *agent.Error", err)
	}
}

func TestPointsMalformedBody(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "not json")
	}))
	defer ts.Close()

	_, err := clientFor(t, ts.URL, 2*time.Second).Points(context.Background(), "m", time.Now().Add(-time.Hour), time.Now())
	var aerr *Error
	if !errors.As(err, &aerr) {
		t.Fatalf("err = %v, want *agent.Error", err)
	}
}

func TestPointsUnreachable(t *testing.T) {
	// A closed server simulates an agent that is down.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Close()

	_, err := clientFor(t, ts.URL, 100*time.Millisecond).Points(context.Background(), "m", time.Now().Add(-time.Hour), time.Now())
	var aerr *Error
	if !errors.As(err, &aerr) {
		t.Fatalf("err = %v, want *agent.Error", err)
	}
}
