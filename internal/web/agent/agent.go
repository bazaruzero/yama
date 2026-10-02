// Package agent provides the frontend's HTTP client for a YAMA agent's
// read-only metric API.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Point is one collected metric data point.
type Point struct {
	Timestamp time.Time
	Value     float64
}

// Error is the single error type for any failure to obtain data from the
// agent: unreachable host, timeout, non-2xx response, or unparseable body.
type Error struct {
	Metric string
	Err    error
}

func (e *Error) Error() string {
	return fmt.Sprintf("agent request for metric %q failed: %v", e.Metric, e.Err)
}

// Unwrap allows errors.Is/As to inspect the underlying cause.
func (e *Error) Unwrap() error { return e.Err }

// Client talks to a YAMA agent's metric API. It is safe for concurrent use.
type Client struct {
	baseURL string
	hc      *http.Client
}

// NewClient returns a client for the agent at host:port with the given
// per-request timeout.
func NewClient(host string, port int, timeout time.Duration) *Client {
	return &Client{
		baseURL: fmt.Sprintf("http://%s:%d", host, port),
		hc:      &http.Client{Timeout: timeout},
	}
}

type rawPoint struct {
	Timestamp string  `json:"timestamp"`
	Value     float64 `json:"value"`
}

// Points fetches the metric's data points in [from, to] from the agent,
// ordered by timestamp ascending as the agent returns them. An empty
// result means the metric has no data in the window.
func (c *Client) Points(ctx context.Context, name string, from, to time.Time) ([]Point, error) {
	u := fmt.Sprintf("%s/api/v1/metrics/%s/data", c.baseURL, url.PathEscape(name))
	q := url.Values{}
	q.Set("from", from.Format(time.RFC3339))
	q.Set("to", to.Format(time.RFC3339))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u+"?"+q.Encode(), nil)
	if err != nil {
		return nil, &Error{Metric: name, Err: err}
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, &Error{Metric: name, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &Error{Metric: name, Err: fmt.Errorf("agent returned status %s", resp.Status)}
	}
	var raw []rawPoint
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, &Error{Metric: name, Err: fmt.Errorf("decode agent response: %w", err)}
	}
	points := make([]Point, 0, len(raw))
	for _, p := range raw {
		ts, err := time.Parse(time.RFC3339Nano, p.Timestamp)
		if err != nil {
			return nil, &Error{Metric: name, Err: fmt.Errorf("decode point timestamp %q: %w", p.Timestamp, err)}
		}
		points = append(points, Point{Timestamp: ts, Value: p.Value})
	}
	return points, nil
}
