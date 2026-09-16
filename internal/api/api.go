// Package api exposes collected metric data over HTTP.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/bazaruzero/yama/internal/store"
)

// defaultWindow is applied to the time range start when the from parameter
// is omitted: the last 3 hours up to now.
const defaultWindow = 3 * time.Hour

// Server serves the metric data REST API.
type Server struct {
	store store.Store
	log   *slog.Logger
	// now supplies the current time; injectable for tests.
	now func() time.Time
}

// NewServer creates the API server.
func NewServer(st store.Store, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{store: st, log: log, now: func() time.Time { return time.Now().UTC() }}
}

// Handler returns the routed HTTP handler. Non-GET requests on the metric
// data endpoint receive 405 Method Not Allowed from the method pattern.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/metrics/{name}/data", s.handleMetricData)
	return mux
}

type dataPoint struct {
	Timestamp string  `json:"timestamp"`
	Value     float64 `json:"value"`
}

func (s *Server) handleMetricData(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	now := s.now()

	from, err := parseTimeParam(r, "from", now.Add(-defaultWindow))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	to, err := parseTimeParam(r, "to", now)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	points, err := s.store.Query(name, from, to)
	if err != nil {
		s.log.Error("store query failed", "metric", name, "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := make([]dataPoint, 0, len(points))
	for _, p := range points {
		resp = append(resp, dataPoint{
			Timestamp: p.Timestamp.Format(time.RFC3339Nano),
			Value:     p.Value,
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// parseTimeParam parses an RFC3339 query parameter, returning def when the
// parameter is absent.
func parseTimeParam(r *http.Request, param string, def time.Time) (time.Time, error) {
	raw := r.URL.Query().Get(param)
	if raw == "" {
		return def, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, &paramError{param: param, value: raw}
	}
	return t, nil
}

type paramError struct {
	param string
	value string
}

func (e *paramError) Error() string {
	return "invalid " + e.param + " parameter " + strconv.Quote(e.value) + ": expected RFC3339 timestamp"
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
